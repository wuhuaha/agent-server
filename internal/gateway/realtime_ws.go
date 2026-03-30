package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"agent-server/internal/session"
	"agent-server/internal/voice"
	"agent-server/pkg/events"

	"github.com/gorilla/websocket"
)

type controlEnvelope struct {
	Type      events.Type      `json:"type"`
	SessionID string           `json:"session_id,omitempty"`
	Seq       int64            `json:"seq"`
	Timestamp time.Time        `json:"ts"`
	Payload   *json.RawMessage `json:"payload"`
}

type sessionStartPayload struct {
	ProtocolVersion string                   `json:"protocol_version"`
	Device          sessionStartDevice       `json:"device"`
	Audio           sessionStartAudio        `json:"audio"`
	Session         sessionStartSession      `json:"session"`
	Capabilities    sessionStartCapabilities `json:"capabilities"`
}

type sessionStartDevice struct {
	DeviceID        string `json:"device_id"`
	ClientType      string `json:"client_type"`
	FirmwareVersion string `json:"firmware_version,omitempty"`
}

type sessionStartAudio struct {
	Codec        string `json:"codec"`
	SampleRateHz int    `json:"sample_rate_hz"`
	Channels     int    `json:"channels"`
}

type sessionStartSession struct {
	Mode         string `json:"mode"`
	WakeReason   string `json:"wake_reason,omitempty"`
	ClientCanEnd bool   `json:"client_can_end"`
	ServerCanEnd bool   `json:"server_can_end"`
}

type sessionStartCapabilities struct {
	TextInput     bool `json:"text_input"`
	ImageInput    bool `json:"image_input"`
	HalfDuplex    bool `json:"half_duplex"`
	LocalWakeWord bool `json:"local_wake_word"`
}

type commitPayload struct {
	Reason string `json:"reason"`
}

type textPayload struct {
	Text string `json:"text"`
}

type sessionEndPayload struct {
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

type sessionUpdatePayload struct {
	State          session.State `json:"state"`
	BargeInEnabled bool          `json:"barge_in_enabled,omitempty"`
}

type clientSessionUpdatePayload struct {
	Interrupt bool `json:"interrupt"`
}

type responseStartPayload struct {
	ResponseID string   `json:"response_id"`
	Modalities []string `json:"modalities"`
}

type responseChunkPayload struct {
	ResponseID string `json:"response_id"`
	Text       string `json:"text,omitempty"`
}

type errorPayload struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

type wsPeer struct {
	conn      *websocket.Conn
	serverSeq int64
	writeMu   sync.Mutex
}

func (p *wsPeer) WriteEvent(eventType events.Type, sessionID string, payload any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	p.serverSeq++
	envelope := events.New(eventType, sessionID, p.serverSeq, payload)
	return p.conn.WriteJSON(envelope)
}

func (p *wsPeer) WriteBinary(payload []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	return p.conn.WriteMessage(websocket.BinaryMessage, payload)
}

type realtimeWSHandler struct {
	profile   RealtimeProfile
	upgrader  websocket.Upgrader
	responder voice.Responder
}

func NewRealtimeWSHandler(profile RealtimeProfile, responder voice.Responder) http.Handler {
	if responder == nil {
		responder = voice.NewBootstrapResponder(profile.OutputCodec, profile.OutputSampleRate, profile.OutputChannels)
	}
	return &realtimeWSHandler{
		profile: profile,
		upgrader: websocket.Upgrader{
			Subprotocols: []string{profile.Subprotocol},
			CheckOrigin: func(*http.Request) bool {
				return true
			},
		},
		responder: responder,
	}
}

func (h *realtimeWSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return
	}

	if !supportsSubprotocol(r, h.profile.Subprotocol) {
		http.Error(w, "missing required websocket subprotocol", http.StatusBadRequest)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	readLimit := 65536
	if h.profile.MaxFrameBytes > readLimit {
		readLimit = h.profile.MaxFrameBytes * 2
	}
	conn.SetReadLimit(int64(readLimit))

	peer := &wsPeer{conn: conn}
	runtime := newConnectionRuntime(conn, peer, session.NewRealtimeSession())
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer runtime.interruptOutput(50 * time.Millisecond)

	if err := applyReadDeadline(conn, runtime.session.Snapshot(), h.profile); err != nil {
		return
	}

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if handled, handleErr := h.handleReadError(runtime, err); handled {
				if handleErr != nil {
					return
				}
				continue
			}
			return
		}

		switch messageType {
		case websocket.TextMessage:
			if err := h.handleControl(ctx, runtime, payload); err != nil {
				if errors.Is(err, session.ErrNoActiveSession) || errors.Is(err, session.ErrSessionAlreadyActive) {
					continue
				}
				return
			}
		case websocket.BinaryMessage:
			if err := h.handleBinary(runtime, payload); err != nil {
				return
			}
		default:
			if err := peer.WriteEvent(events.TypeError, runtime.session.Snapshot().SessionID, errorPayload{
				Code:        "unsupported_message_type",
				Message:     "only text and binary websocket frames are supported",
				Recoverable: true,
			}); err != nil {
				return
			}
		}
	}
}

func (h *realtimeWSHandler) handleReadError(runtime *connectionRuntime, err error) (bool, error) {
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		return false, nil
	}

	snapshot := runtime.session.Snapshot()
	reason, shouldClose := deadlineCloseReason(snapshot, h.profile, time.Now().UTC())
	if !shouldClose {
		return true, applyReadDeadline(runtime.conn, snapshot, h.profile)
	}

	runtime.interruptOutput(100 * time.Millisecond)
	return true, h.endSession(runtime, reason, closeMessageForReason(reason))
}

func (h *realtimeWSHandler) handleBinary(runtime *connectionRuntime, payload []byte) error {
	if len(payload) > h.profile.MaxFrameBytes {
		return h.terminateWithError(runtime, "frame_too_large", fmt.Sprintf("binary frame exceeds %d bytes", h.profile.MaxFrameBytes))
	}

	previous := runtime.session.Snapshot()
	if previous.State == session.StateSpeaking {
		runtime.interruptOutput(100 * time.Millisecond)
	}

	normalizedPayload := payload
	if runtime.inputNormalizer != nil {
		decodedPayload, err := runtime.inputNormalizer.Decode(payload)
		if err != nil {
			return h.terminateWithError(runtime, "audio_decode_failed", err.Error())
		}
		normalizedPayload = decodedPayload
	}

	current, err := runtime.session.IngestAudioFrame(normalizedPayload)
	if err != nil {
		if sendErr := runtime.peer.WriteEvent(events.TypeError, "", errorPayload{
			Code:        "session_not_started",
			Message:     "session.start must be sent before binary audio frames",
			Recoverable: true,
		}); sendErr != nil {
			return sendErr
		}
		return err
	}

	if err := applyReadDeadline(runtime.conn, current, h.profile); err != nil {
		return err
	}

	if previous.State == session.StateSpeaking {
		return runtime.peer.WriteEvent(events.TypeSessionUpdate, current.SessionID, sessionUpdatePayload{
			State:          current.State,
			BargeInEnabled: true,
		})
	}

	return nil
}

func (h *realtimeWSHandler) handleControl(ctx context.Context, runtime *connectionRuntime, raw []byte) error {
	var envelope controlEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, "", errorPayload{
			Code:        "invalid_json",
			Message:     "control frame is not valid json",
			Recoverable: true,
		})
	}

	switch envelope.Type {
	case events.TypeSessionStart:
		return h.handleSessionStart(runtime, envelope)
	case events.TypeAudioInCommit:
		return h.handleCommit(ctx, runtime, envelope)
	case events.TypeTextIn:
		return h.handleText(ctx, runtime, envelope)
	case events.TypeSessionEnd:
		return h.handleClientEnd(runtime, envelope)
	case events.TypeSessionUpdate:
		return h.handleSessionUpdate(runtime, envelope)
	default:
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "unsupported_event",
			Message:     fmt.Sprintf("event %s is not implemented in bootstrap handler", envelope.Type),
			Recoverable: true,
		})
	}
}

func (h *realtimeWSHandler) handleSessionStart(runtime *connectionRuntime, envelope controlEnvelope) error {
	var payload sessionStartPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "invalid_session_start",
			Message:     err.Error(),
			Recoverable: false,
		})
	}

	if err := h.validateStartPayload(payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "unsupported_session_start",
			Message:     err.Error(),
			Recoverable: false,
		})
	}

	inputNormalizer, err := voice.NewInputNormalizer(payload.Audio.Codec, h.profile.InputSampleRate, h.profile.InputChannels)
	if err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "unsupported_session_start",
			Message:     err.Error(),
			Recoverable: false,
		})
	}

	snapshot, err := runtime.session.Start(session.StartRequest{
		RequestedSessionID: envelope.SessionID,
		DeviceID:           payload.Device.DeviceID,
		ClientType:         payload.Device.ClientType,
		FirmwareVersion:    payload.Device.FirmwareVersion,
		Mode:               payload.Session.Mode,
		InputCodec:         payload.Audio.Codec,
		InputSampleRate:    payload.Audio.SampleRateHz,
		InputChannels:      payload.Audio.Channels,
		ClientCanEnd:       payload.Session.ClientCanEnd,
		ServerCanEnd:       payload.Session.ServerCanEnd,
	})
	if err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_already_active",
			Message:     err.Error(),
			Recoverable: false,
		})
	}
	runtime.inputNormalizer = inputNormalizer

	if err := applyReadDeadline(runtime.conn, snapshot, h.profile); err != nil {
		return err
	}

	return runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdatePayload{
		State:          session.StateActive,
		BargeInEnabled: true,
	})
}

func (h *realtimeWSHandler) handleCommit(ctx context.Context, runtime *connectionRuntime, envelope controlEnvelope) error {
	var payload commitPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "invalid_commit",
			Message:     err.Error(),
			Recoverable: true,
		})
	}

	snapshot, err := runtime.session.CommitTurn()
	if err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot commit audio before session.start",
			Recoverable: true,
		})
	}

	if err := applyReadDeadline(runtime.conn, snapshot, h.profile); err != nil {
		return err
	}

	if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdatePayload{State: session.StateThinking}); err != nil {
		return err
	}

	return h.emitBootstrapResponse(ctx, runtime, snapshot, "", payload.Reason)
}

func (h *realtimeWSHandler) handleText(ctx context.Context, runtime *connectionRuntime, envelope controlEnvelope) error {
	if !h.profile.AllowTextInput {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "text_input_disabled",
			Message:     "text input is disabled on this server",
			Recoverable: true,
		})
	}

	var payload textPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "invalid_text_input",
			Message:     err.Error(),
			Recoverable: true,
		})
	}

	if runtime.session.Snapshot().State == session.StateSpeaking {
		runtime.interruptOutput(100 * time.Millisecond)
	}

	if _, err := runtime.session.AcceptText(); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot send text before session.start",
			Recoverable: true,
		})
	}

	snapshot, err := runtime.session.CommitTurn()
	if err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot send text before session.start",
			Recoverable: true,
		})
	}

	if err := applyReadDeadline(runtime.conn, snapshot, h.profile); err != nil {
		return err
	}

	if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdatePayload{State: session.StateThinking}); err != nil {
		return err
	}

	return h.emitBootstrapResponse(ctx, runtime, snapshot, payload.Text, "text_input")
}

func (h *realtimeWSHandler) handleClientEnd(runtime *connectionRuntime, envelope controlEnvelope) error {
	var payload sessionEndPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "invalid_session_end",
			Message:     err.Error(),
			Recoverable: true,
		})
	}

	runtime.interruptOutput(100 * time.Millisecond)
	reason := defaultCloseReason(payload.Reason, string(session.CloseReasonClientStop))
	if err := h.endSession(runtime, session.CloseReason(reason), payload.Message); err != nil {
		if !errors.Is(err, session.ErrNoActiveSession) {
			return err
		}
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot end a session that is not active",
			Recoverable: true,
		})
	}
	return nil
}

func (h *realtimeWSHandler) handleSessionUpdate(runtime *connectionRuntime, envelope controlEnvelope) error {
	var payload clientSessionUpdatePayload
	if envelope.Payload != nil {
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
				Code:        "invalid_session_update",
				Message:     err.Error(),
				Recoverable: true,
			})
		}
	}

	snapshot := runtime.session.Snapshot()
	if snapshot.SessionID == "" {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "session.update cannot be processed before session.start",
			Recoverable: true,
		})
	}

	if payload.Interrupt && snapshot.State == session.StateSpeaking {
		runtime.interruptOutput(100 * time.Millisecond)
		updated, err := runtime.session.SetState(session.StateActive)
		if err != nil {
			return err
		}
		if err := applyReadDeadline(runtime.conn, updated, h.profile); err != nil {
			return err
		}
		return runtime.peer.WriteEvent(events.TypeSessionUpdate, updated.SessionID, sessionUpdatePayload{
			State:          updated.State,
			BargeInEnabled: true,
		})
	}

	return runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdatePayload{
		State:          snapshot.State,
		BargeInEnabled: true,
	})
}

func (h *realtimeWSHandler) emitBootstrapResponse(ctx context.Context, runtime *connectionRuntime, snapshot session.Snapshot, text string, _ string) error {
	inputCodec := snapshot.InputCodec
	inputSampleRate := snapshot.InputSampleRate
	inputChannels := snapshot.InputChannels
	if runtime.inputNormalizer != nil {
		inputCodec = runtime.inputNormalizer.OutputCodec()
		inputSampleRate = runtime.inputNormalizer.OutputSampleRate()
		inputChannels = runtime.inputNormalizer.OutputChannels()
	}

	response, err := h.responder.Respond(ctx, voice.TurnRequest{
		SessionID:       snapshot.SessionID,
		DeviceID:        snapshot.DeviceID,
		Text:            text,
		AudioPCM:        snapshot.TurnAudio,
		AudioBytes:      snapshot.AudioBytes,
		InputFrames:     snapshot.InputFrames,
		InputCodec:      inputCodec,
		InputSampleRate: inputSampleRate,
		InputChannels:   inputChannels,
	})
	if err != nil {
		return h.terminateWithError(runtime, "response_generation_failed", err.Error())
	}

	responseID := fmt.Sprintf("resp_%d", time.Now().UTC().UnixNano())
	modalities := []string{"text"}
	if len(response.AudioChunks) > 0 || response.AudioStream != nil {
		modalities = append(modalities, "audio")
	}

	if err := runtime.peer.WriteEvent(events.TypeResponseStart, snapshot.SessionID, responseStartPayload{
		ResponseID: responseID,
		Modalities: modalities,
	}); err != nil {
		return err
	}

	if response.Text != "" {
		if err := runtime.peer.WriteEvent(events.TypeResponseChunk, snapshot.SessionID, responseChunkPayload{
			ResponseID: responseID,
			Text:       response.Text,
		}); err != nil {
			return err
		}
	}

	runtime.session.ClearTurn()

	audioStream := response.AudioStream
	if audioStream == nil && len(response.AudioChunks) > 0 {
		audioStream = voice.NewStaticAudioStream(response.AudioChunks)
	}

	if audioStream != nil {
		speaking, err := runtime.session.SetState(session.StateSpeaking)
		if err != nil {
			return err
		}

		if err := applyReadDeadline(runtime.conn, speaking, h.profile); err != nil {
			return err
		}

		if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdatePayload{State: session.StateSpeaking}); err != nil {
			return err
		}

		h.startAudioStream(ctx, runtime, snapshot.SessionID, text, audioStream)
		return nil
	}

	if shouldServerEnd(text) {
		return h.endSession(runtime, session.CloseReasonCompleted, closeMessageForReason(session.CloseReasonCompleted))
	}

	active, err := runtime.session.SetState(session.StateActive)
	if err != nil {
		return err
	}
	if err := applyReadDeadline(runtime.conn, active, h.profile); err != nil {
		return err
	}

	return runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdatePayload{
		State:          active.State,
		BargeInEnabled: true,
	})
}

func (h *realtimeWSHandler) startAudioStream(
	ctx context.Context,
	runtime *connectionRuntime,
	sessionID string,
	requestText string,
	audioStream voice.AudioStream,
) {
	streamCtx, cancel := context.WithCancel(ctx)
	stream := runtime.installOutput(cancel)

	go func() {
		defer close(stream.done)
		defer runtime.clearOutput(stream)
		defer func() { _ = audioStream.Close() }()

		ticker := time.NewTicker(outputFrameInterval)
		defer ticker.Stop()

		for {
			if streamCtx.Err() != nil {
				return
			}

			chunk, err := audioStream.Next(streamCtx)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
					break
				}
				if streamCtx.Err() != nil {
					return
				}
				current := runtime.session.Snapshot()
				if current.SessionID == sessionID && current.State == session.StateSpeaking {
					_ = runtime.peer.WriteEvent(events.TypeError, sessionID, errorPayload{
						Code:        "audio_stream_failed",
						Message:     err.Error(),
						Recoverable: false,
					})
					_ = h.endSession(runtime, session.CloseReasonError, err.Error())
				}
				return
			}
			if len(chunk) == 0 {
				continue
			}
			if err := runtime.peer.WriteBinary(chunk); err != nil {
				return
			}
			select {
			case <-streamCtx.Done():
				return
			case <-ticker.C:
			}
		}

		if streamCtx.Err() != nil {
			return
		}

		current := runtime.session.Snapshot()
		if current.SessionID != sessionID || current.State != session.StateSpeaking {
			return
		}

		if shouldServerEnd(requestText) {
			_ = h.endSession(runtime, session.CloseReasonCompleted, closeMessageForReason(session.CloseReasonCompleted))
			return
		}

		active, err := runtime.session.SetState(session.StateActive)
		if err != nil {
			return
		}
		if err := applyReadDeadline(runtime.conn, active, h.profile); err != nil {
			return
		}
		_ = runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdatePayload{
			State:          active.State,
			BargeInEnabled: true,
		})
	}()
}

func (h *realtimeWSHandler) validateStartPayload(payload sessionStartPayload) error {
	if payload.ProtocolVersion != h.profile.ProtocolVersion {
		return fmt.Errorf("protocol_version must be %s", h.profile.ProtocolVersion)
	}
	if payload.Device.DeviceID == "" {
		return errors.New("device.device_id is required")
	}
	if payload.Device.ClientType == "" {
		return errors.New("device.client_type is required")
	}
	if payload.Audio.Codec == "" {
		return errors.New("audio.codec is required")
	}
	if payload.Audio.Codec != h.profile.InputCodec && !(h.profile.AllowOpus && payload.Audio.Codec == "opus") {
		return fmt.Errorf("unsupported codec %s", payload.Audio.Codec)
	}
	if payload.Audio.SampleRateHz != h.profile.InputSampleRate {
		return fmt.Errorf("unsupported sample_rate_hz %d", payload.Audio.SampleRateHz)
	}
	if payload.Audio.Channels != h.profile.InputChannels {
		return fmt.Errorf("unsupported channels %d", payload.Audio.Channels)
	}
	if payload.Session.Mode == "" {
		return errors.New("session.mode is required")
	}
	return nil
}

func (h *realtimeWSHandler) endSession(runtime *connectionRuntime, reason session.CloseReason, message string) error {
	snapshot, err := runtime.session.End(reason)
	if err != nil {
		return err
	}

	if err := runtime.peer.WriteEvent(events.TypeSessionEnd, snapshot.SessionID, sessionEndPayload{
		Reason:  string(reason),
		Message: message,
	}); err != nil {
		return err
	}

	runtime.session.Reset()
	runtime.inputNormalizer = nil
	return applyReadDeadline(runtime.conn, runtime.session.Snapshot(), h.profile)
}

func (h *realtimeWSHandler) terminateWithError(runtime *connectionRuntime, code, message string) error {
	snapshot := runtime.session.Snapshot()
	sessionID := snapshot.SessionID

	if err := runtime.peer.WriteEvent(events.TypeError, sessionID, errorPayload{
		Code:        code,
		Message:     message,
		Recoverable: false,
	}); err != nil {
		return err
	}

	if sessionID != "" {
		runtime.interruptOutput(100 * time.Millisecond)
		return h.endSession(runtime, session.CloseReasonError, message)
	}

	return nil
}

func supportsSubprotocol(r *http.Request, expected string) bool {
	for _, offered := range websocket.Subprotocols(r) {
		if strings.TrimSpace(offered) == expected {
			return true
		}
	}
	return false
}

func decodePayload(raw *json.RawMessage, target any) error {
	if raw == nil {
		return errors.New("payload is required")
	}
	if err := json.Unmarshal(*raw, target); err != nil {
		return err
	}
	return nil
}

func shouldServerEnd(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	switch normalized {
	case "/end", "bye", "goodbye", "结束", "结束对话":
		return true
	default:
		return false
	}
}

func defaultCloseReason(reason, fallback string) string {
	if strings.TrimSpace(reason) == "" {
		return fallback
	}
	return reason
}
