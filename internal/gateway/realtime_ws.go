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
	DeltaType  string `json:"delta_type,omitempty"`
	Text       string `json:"text,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolStatus string `json:"tool_status,omitempty"`
	ToolInput  string `json:"tool_input,omitempty"`
	ToolOutput string `json:"tool_output,omitempty"`
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
				return
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
		if snapshot.SessionID == "" {
			return true, nil
		}

		runtime.interruptOutput(100 * time.Millisecond)
		return true, h.endSession(runtime, session.CloseReasonError, closeMessageForReason(session.CloseReasonError))
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

	turn, err := runtime.session.CommitTurn()
	if err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot commit audio before session.start",
			Recoverable: true,
		})
	}

	if err := applyReadDeadline(runtime.conn, turn.Snapshot, h.profile); err != nil {
		return err
	}

	if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, turn.Snapshot.SessionID, sessionUpdatePayload{State: session.StateThinking}); err != nil {
		return err
	}

	return h.emitTurnResponse(ctx, runtime, turn, "")
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

	turn, err := runtime.session.CommitTurn()
	if err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot send text before session.start",
			Recoverable: true,
		})
	}

	if err := applyReadDeadline(runtime.conn, turn.Snapshot, h.profile); err != nil {
		return err
	}

	if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, turn.Snapshot.SessionID, sessionUpdatePayload{State: session.StateThinking}); err != nil {
		return err
	}

	return h.emitTurnResponse(ctx, runtime, turn, payload.Text)
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

func (h *realtimeWSHandler) emitTurnResponse(ctx context.Context, runtime *connectionRuntime, turn session.CommittedTurn, text string) error {
	request := buildTurnRequest(turn, runtime, text)
	responseID := fmt.Sprintf("resp_%d", time.Now().UTC().UnixNano())

	if streamingResponder, ok := h.responder.(voice.StreamingResponder); ok {
		return h.emitStreamingTurnResponse(ctx, runtime, turn.Snapshot, responseID, request, streamingResponder)
	}

	response, err := h.responder.Respond(ctx, request)
	if err != nil {
		return h.terminateWithError(runtime, "response_generation_failed", err.Error())
	}

	responseDeltas := responseDeltasForEmission(response, true)
	if err := runtime.peer.WriteEvent(events.TypeResponseStart, turn.Snapshot.SessionID, responseStartPayload{
		ResponseID: responseID,
		Modalities: responseModalities(responseDeltas, response),
	}); err != nil {
		return err
	}

	if err := h.emitResponseDeltas(runtime, turn.Snapshot.SessionID, responseID, responseDeltas); err != nil {
		return err
	}

	return h.finalizeTurnResponse(ctx, runtime, turn.Snapshot, response)
}

func (h *realtimeWSHandler) emitStreamingTurnResponse(
	ctx context.Context,
	runtime *connectionRuntime,
	snapshot session.Snapshot,
	responseID string,
	request voice.TurnRequest,
	streamingResponder voice.StreamingResponder,
) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type streamedResponseResult struct {
		response voice.TurnResponse
		err      error
	}

	deltaCh := make(chan voice.ResponseDelta, 8)
	responseCh := make(chan streamedResponseResult, 1)

	go func() {
		defer close(deltaCh)
		response, err := streamingResponder.RespondStream(streamCtx, request, voice.ResponseDeltaSinkFunc(func(ctx context.Context, delta voice.ResponseDelta) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case deltaCh <- delta:
				return nil
			}
		}))
		responseCh <- streamedResponseResult{response: response, err: err}
	}()

	var response voice.TurnResponse
	responseReady := false
	sentResponseStart := false
	sawStreamedDelta := false
	var responseChRef <-chan streamedResponseResult = responseCh

	for !responseReady || deltaCh != nil {
		select {
		case delta, ok := <-deltaCh:
			if !ok {
				deltaCh = nil
				continue
			}
			sawStreamedDelta = true
			if !sentResponseStart {
				if err := runtime.peer.WriteEvent(events.TypeResponseStart, snapshot.SessionID, responseStartPayload{ResponseID: responseID, Modalities: []string{"text"}}); err != nil {
					cancel()
					return err
				}
				sentResponseStart = true
			}
			if err := h.emitResponseDelta(runtime, snapshot.SessionID, responseID, delta); err != nil {
				cancel()
				return err
			}
		case result := <-responseChRef:
			responseChRef = nil
			if result.err != nil {
				cancel()
				return h.terminateWithError(runtime, "response_generation_failed", result.err.Error())
			}
			response = result.response
			responseReady = true
			if !sentResponseStart {
				responseDeltas := responseDeltasForEmission(response, true)
				if err := runtime.peer.WriteEvent(events.TypeResponseStart, snapshot.SessionID, responseStartPayload{ResponseID: responseID, Modalities: responseModalities(responseDeltas, response)}); err != nil {
					cancel()
					return err
				}
				sentResponseStart = true
				if err := h.emitResponseDeltas(runtime, snapshot.SessionID, responseID, responseDeltas); err != nil {
					cancel()
					return err
				}
			} else if !sawStreamedDelta {
				responseDeltas := responseDeltasForEmission(response, true)
				if err := h.emitResponseDeltas(runtime, snapshot.SessionID, responseID, responseDeltas); err != nil {
					cancel()
					return err
				}
			}
		}
	}

	return h.finalizeTurnResponse(ctx, runtime, snapshot, response)
}

func buildTurnRequest(turn session.CommittedTurn, runtime *connectionRuntime, text string) voice.TurnRequest {
	inputCodec := turn.Snapshot.InputCodec
	inputSampleRate := turn.Snapshot.InputSampleRate
	inputChannels := turn.Snapshot.InputChannels
	if runtime.inputNormalizer != nil {
		inputCodec = runtime.inputNormalizer.OutputCodec()
		inputSampleRate = runtime.inputNormalizer.OutputSampleRate()
		inputChannels = runtime.inputNormalizer.OutputChannels()
	}

	return voice.TurnRequest{
		SessionID:       turn.Snapshot.SessionID,
		DeviceID:        turn.Snapshot.DeviceID,
		ClientType:      turn.Snapshot.ClientType,
		Text:            text,
		AudioPCM:        turn.AudioPCM,
		AudioBytes:      turn.Snapshot.AudioBytes,
		InputFrames:     turn.Snapshot.InputFrames,
		InputCodec:      inputCodec,
		InputSampleRate: inputSampleRate,
		InputChannels:   inputChannels,
	}
}

func responseDeltasForEmission(response voice.TurnResponse, allowTextFallback bool) []voice.ResponseDelta {
	if len(response.Deltas) > 0 {
		return response.Deltas
	}
	if allowTextFallback && strings.TrimSpace(response.Text) != "" {
		return []voice.ResponseDelta{{Kind: voice.ResponseDeltaKindText, Text: response.Text}}
	}
	return nil
}

func responseModalities(deltas []voice.ResponseDelta, response voice.TurnResponse) []string {
	modalities := make([]string, 0, 2)
	if len(deltas) > 0 {
		modalities = append(modalities, "text")
	}
	if len(response.AudioChunks) > 0 || response.AudioStream != nil {
		modalities = append(modalities, "audio")
	}
	if len(modalities) == 0 {
		modalities = append(modalities, "text")
	}
	return modalities
}

func (h *realtimeWSHandler) finalizeTurnResponse(ctx context.Context, runtime *connectionRuntime, snapshot session.Snapshot, response voice.TurnResponse) error {
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
		h.startAudioStream(ctx, runtime, snapshot.SessionID, audioStream, response.EndSession, response.EndReason, response.EndMessage)
		return nil
	}

	if response.EndSession {
		reason, message := responseCloseDirective(response)
		return h.endSession(runtime, reason, message)
	}

	active, err := runtime.session.SetState(session.StateActive)
	if err != nil {
		return err
	}
	if err := applyReadDeadline(runtime.conn, active, h.profile); err != nil {
		return err
	}

	return runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdatePayload{State: active.State, BargeInEnabled: true})
}

func (h *realtimeWSHandler) startAudioStream(
	ctx context.Context,
	runtime *connectionRuntime,
	sessionID string,
	audioStream voice.AudioStream,
	endSession bool,
	endReason string,
	endMessage string,
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
					_ = runtime.peer.WriteEvent(events.TypeError, sessionID, errorPayload{Code: "audio_stream_failed", Message: err.Error(), Recoverable: false})
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

		if endSession {
			reason := session.CloseReason(defaultCloseReason(endReason, string(session.CloseReasonCompleted)))
			message := strings.TrimSpace(endMessage)
			if message == "" {
				message = closeMessageForReason(reason)
			}
			_ = h.endSession(runtime, reason, message)
			return
		}

		active, err := runtime.session.SetState(session.StateActive)
		if err != nil {
			return
		}
		if err := applyReadDeadline(runtime.conn, active, h.profile); err != nil {
			return
		}
		_ = runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdatePayload{State: active.State, BargeInEnabled: true})
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

	if err := runtime.peer.WriteEvent(events.TypeSessionEnd, snapshot.SessionID, sessionEndPayload{Reason: string(reason), Message: message}); err != nil {
		return err
	}

	runtime.session.Reset()
	runtime.inputNormalizer = nil
	return applyReadDeadline(runtime.conn, runtime.session.Snapshot(), h.profile)
}

func (h *realtimeWSHandler) terminateWithError(runtime *connectionRuntime, code, message string) error {
	snapshot := runtime.session.Snapshot()
	sessionID := snapshot.SessionID

	if err := runtime.peer.WriteEvent(events.TypeError, sessionID, errorPayload{Code: code, Message: message, Recoverable: false}); err != nil {
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

func (h *realtimeWSHandler) emitResponseDeltas(runtime *connectionRuntime, sessionID, responseID string, deltas []voice.ResponseDelta) error {
	for _, delta := range deltas {
		if err := h.emitResponseDelta(runtime, sessionID, responseID, delta); err != nil {
			return err
		}
	}
	return nil
}

func (h *realtimeWSHandler) emitResponseDelta(runtime *connectionRuntime, sessionID, responseID string, delta voice.ResponseDelta) error {
	payload := responseChunkPayload{ResponseID: responseID, DeltaType: string(delta.Kind), Text: delta.Text, ToolCallID: delta.ToolCallID, ToolName: delta.ToolName, ToolStatus: delta.ToolStatus, ToolInput: delta.ToolInput, ToolOutput: delta.ToolOutput}
	if payload.DeltaType == "" {
		payload.DeltaType = string(voice.ResponseDeltaKindText)
	}
	return runtime.peer.WriteEvent(events.TypeResponseChunk, sessionID, payload)
}

func responseCloseDirective(response voice.TurnResponse) (session.CloseReason, string) {
	reason := session.CloseReason(defaultCloseReason(response.EndReason, string(session.CloseReasonCompleted)))
	message := strings.TrimSpace(response.EndMessage)
	if message == "" {
		message = closeMessageForReason(reason)
	}
	return reason, message
}

func defaultCloseReason(reason, fallback string) string {
	if strings.TrimSpace(reason) == "" {
		return fallback
	}
	return reason
}
