package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	TextInput     bool                               `json:"text_input"`
	ImageInput    bool                               `json:"image_input"`
	HalfDuplex    bool                               `json:"half_duplex"`
	LocalWakeWord bool                               `json:"local_wake_word"`
	PreviewEvents bool                               `json:"preview_events,omitempty"`
	PlaybackAck   *sessionStartPlaybackAckCapability `json:"playback_ack,omitempty"`
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
	State          session.State       `json:"state"`
	InputState     session.InputState  `json:"input_state,omitempty"`
	OutputState    session.OutputState `json:"output_state,omitempty"`
	BargeInEnabled bool                `json:"barge_in_enabled,omitempty"`
	TurnID         string              `json:"turn_id,omitempty"`
	AcceptReason   string              `json:"accept_reason,omitempty"`
}

type clientSessionUpdatePayload struct {
	Interrupt bool `json:"interrupt"`
}

type responseStartPayload struct {
	ResponseID string   `json:"response_id"`
	Modalities []string `json:"modalities"`
	TurnID     string   `json:"turn_id,omitempty"`
	TraceID    string   `json:"trace_id,omitempty"`
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

func sessionUpdateFromSnapshot(snapshot session.Snapshot, turnID, acceptReason string) sessionUpdatePayload {
	return sessionUpdatePayload{
		State:          snapshot.State,
		InputState:     snapshot.InputState,
		OutputState:    snapshot.OutputState,
		BargeInEnabled: true,
		TurnID:         turnID,
		AcceptReason:   strings.TrimSpace(acceptReason),
	}
}

type wsPeer struct {
	conn      websocketWriteConn
	serverSeq int64
	writeMu   sync.Mutex
}

func (p *wsPeer) WriteEvent(eventType events.Type, sessionID string, payload any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	p.serverSeq++
	envelope := events.New(eventType, sessionID, p.serverSeq, payload)
	return writeWebsocketJSON(p.conn, envelope)
}

func (p *wsPeer) WriteBinary(payload []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	return writeWebsocketBinary(p.conn, payload)
}

type realtimeWSHandler struct {
	profile   RealtimeProfile
	upgrader  websocket.Upgrader
	responder voice.Responder
	logger    *slog.Logger
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
		logger:    gatewayTraceLogger(profile.Logger, "realtime"),
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
	runtime := newConnectionRuntime(conn, peer, session.NewRealtimeSession(), h.responder)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer runtime.interruptOutput(50 * time.Millisecond)
	inboundCh := startWebsocketReadPump(ctx, conn)
	var previewTicker *time.Ticker
	if h.profile.ServerEndpointEnabled {
		previewTicker = time.NewTicker(voice.InputPreviewPollInterval)
		defer previewTicker.Stop()
	}

	if err := applyReadDeadline(runtime, runtime.session.Snapshot(), h.profile); err != nil {
		return
	}

	for {
		select {
		case inbound, ok := <-inboundCh:
			if !ok {
				return
			}
			if err := h.handleInboundMessage(ctx, runtime, peer, inbound); err != nil {
				return
			}
		case <-previewTickerC(previewTicker):
			drainedInbound := false
		drainInbound:
			for {
				select {
				case inbound, ok := <-inboundCh:
					if !ok {
						return
					}
					drainedInbound = true
					if err := h.handleInboundMessage(ctx, runtime, peer, inbound); err != nil {
						return
					}
				default:
					break drainInbound
				}
			}
			if drainedInbound {
				continue
			}
			if err := h.handleServerEndpointTick(ctx, runtime, time.Now().UTC()); err != nil {
				return
			}
		}
	}
}

func (h *realtimeWSHandler) handleInboundMessage(ctx context.Context, runtime *connectionRuntime, peer *wsPeer, inbound websocketInboundMessage) error {
	if inbound.err != nil {
		logWebsocketInboundTermination(h.logger, runtime, runtime.turnTrace.Current(), inbound.err)
		if handled, handleErr := h.handleReadError(runtime, inbound.err); handled {
			return handleErr
		}
		return inbound.err
	}

	switch inbound.messageType {
	case websocket.TextMessage:
		if err := h.handleControl(ctx, runtime, inbound.payload); err != nil {
			if errors.Is(err, session.ErrNoActiveSession) || errors.Is(err, session.ErrSessionAlreadyActive) {
				return nil
			}
			return err
		}
	case websocket.BinaryMessage:
		if err := h.handleBinary(runtime, inbound.payload); err != nil {
			return err
		}
	default:
		if err := peer.WriteEvent(events.TypeError, runtime.session.Snapshot().SessionID, errorPayload{
			Code:        "unsupported_message_type",
			Message:     "only text and binary websocket frames are supported",
			Recoverable: true,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (h *realtimeWSHandler) handleReadError(runtime *connectionRuntime, err error) (bool, error) {
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		return false, nil
	}

	snapshot := runtime.session.Snapshot()
	now := time.Now().UTC()
	reason, shouldClose := deadlineCloseReason(snapshot, h.profile, now)
	trace := runtime.turnTrace.Current()
	logTurnTraceInfo(h.logger, "gateway websocket read timeout", snapshot.SessionID, trace,
		"remote_addr", runtime.remoteAddr,
		"session_state", snapshot.State,
		"idle_timeout_ms", h.profile.IdleTimeoutMs,
		"max_session_ms", h.profile.MaxSessionMs,
		"deadline_close", shouldClose,
		"close_reason", string(reason),
	)
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

func (h *realtimeWSHandler) handleServerEndpointTick(ctx context.Context, runtime *connectionRuntime, now time.Time) error {
	snapshot := runtime.session.Snapshot()
	if !h.profile.ServerEndpointEnabled || snapshot.SessionID == "" || snapshot.State != session.StateActive {
		return nil
	}
	observation := runtime.pollInputPreview(now)
	if !observation.Active {
		return nil
	}
	if observation.PartialChanged {
		logInputPreviewTraceInfo(h.logger, "gateway input preview updated", snapshot.SessionID, observation.Trace,
			"partial_text", observation.Preview.PartialText,
			"audio_bytes", observation.Preview.AudioBytes,
		)
	}
	if observation.SpeechStartedObserved {
		logInputPreviewTraceInfo(h.logger, "gateway input preview speech started", snapshot.SessionID, observation.Trace,
			"audio_bytes", observation.Preview.AudioBytes,
		)
	}
	if observation.EndpointCandidateObserved {
		logInputPreviewTraceInfo(h.logger, "gateway input preview endpoint candidate", snapshot.SessionID, observation.Trace,
			"partial_text", observation.Preview.PartialText,
			"audio_bytes", observation.Preview.AudioBytes,
			"endpoint_reason", observation.Preview.EndpointReason,
		)
	}
	if observation.CommitSuggested {
		logInputPreviewTraceInfo(h.logger, "gateway input preview commit suggested", snapshot.SessionID, observation.Trace,
			"partial_text", observation.Preview.PartialText,
			"audio_bytes", observation.Preview.AudioBytes,
			"endpoint_reason", observation.Preview.EndpointReason,
		)
	}
	if err := h.emitPreviewObservationEvents(runtime, snapshot, observation); err != nil {
		return err
	}
	if observation.CommitSuggested && snapshot.AudioBytes > 0 {
		previewTrace, previewResult, previewResultOK, previewErr := runtime.consumeInputPreview(ctx)
		if previewErr != nil {
			h.logger.Warn("gateway input preview finalize failed",
				"session_id", snapshot.SessionID,
				"preview_id", previewTrace.PreviewID,
				"error", previewErr,
			)
		}
		turn, err := runtime.session.CommitTurn()
		if err != nil {
			return err
		}
		if err := applyReadDeadline(runtime, turn.Snapshot, h.profile); err != nil {
			return err
		}
		trace := runtime.turnTrace.Begin(turn.Snapshot.SessionID, "server_endpoint")
		attrs := []any{
			"input_type", "audio",
			"audio_bytes", len(turn.AudioPCM),
			"input_codec", turn.Snapshot.InputCodec,
			"input_sample_rate_hz", turn.Snapshot.InputSampleRate,
			"input_channels", turn.Snapshot.InputChannels,
			"turn_index", turn.Snapshot.Turns,
			"endpoint_reason", observation.Preview.EndpointReason,
		}
		if previewResultOK && previewErr == nil {
			attrs = appendPreviewTranscriptionLogAttrs(attrs, &previewResult)
		}
		attrs = appendInputPreviewTraceLogAttrs(attrs, previewTrace, now)
		logTurnTraceInfo(h.logger, "gateway turn accepted", turn.Snapshot.SessionID, trace, attrs...)
		if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, turn.Snapshot.SessionID, sessionUpdateFromSnapshot(turn.Snapshot, trace.TurnID, "server_endpoint")); err != nil {
			return err
		}
		var previewTranscription *voice.TranscriptionResult
		if previewResultOK && previewErr == nil {
			previewTranscription = &previewResult
		}
		if err := h.emitTurnResponse(ctx, runtime, turn, trace, "", previewTranscription); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (h *realtimeWSHandler) handleBinary(runtime *connectionRuntime, payload []byte) error {
	if len(payload) > h.profile.MaxFrameBytes {
		return h.terminateWithError(runtime, "frame_too_large", fmt.Sprintf("binary frame exceeds %d bytes", h.profile.MaxFrameBytes))
	}

	previous := runtime.session.Snapshot()
	normalizedPayload := payload
	if runtime.inputNormalizer != nil {
		decodedPayload, err := runtime.inputNormalizer.Decode(payload)
		if err != nil {
			return h.terminateWithError(runtime, "audio_decode_failed", err.Error())
		}
		normalizedPayload = decodedPayload
	}

	if previous.State == session.StateSpeaking {
		runtime.stagePendingBargeInAudio(normalizedPayload)
		observation := runtime.previewForBargeIn(context.Background(), h.responder, previous, normalizedPayload)
		if observation.PartialChanged {
			logInputPreviewTraceInfo(h.logger, "gateway barge-in preview updated", previous.SessionID, observation.Trace,
				"partial_text", observation.Preview.PartialText,
				"audio_bytes", observation.Preview.AudioBytes,
			)
		}
		if observation.SpeechStartedObserved {
			logInputPreviewTraceInfo(h.logger, "gateway barge-in speech started", previous.SessionID, observation.Trace,
				"audio_bytes", observation.Preview.AudioBytes,
			)
		}
		if observation.EndpointCandidateObserved {
			logInputPreviewTraceInfo(h.logger, "gateway barge-in endpoint candidate", previous.SessionID, observation.Trace,
				"partial_text", observation.Preview.PartialText,
				"audio_bytes", observation.Preview.AudioBytes,
				"endpoint_reason", observation.Preview.EndpointReason,
			)
		}
		if err := h.emitPreviewObservationEvents(runtime, previous, observation); err != nil {
			return err
		}
		if observation.SpeechStartedObserved || observation.EndpointCandidateObserved {
			if previewing, stateErr := runtime.session.SetInputState(session.InputStatePreviewing); stateErr == nil {
				_ = runtime.peer.WriteEvent(events.TypeSessionUpdate, previewing.SessionID, sessionUpdateFromSnapshot(previewing, runtime.turnTrace.Current().TurnID, ""))
			}
		}
		decision := voice.EvaluateBargeIn(observation.Preview, previous.InputSampleRate, previous.InputChannels, voice.BargeInConfig{
			MinAudioMs:       h.profile.BargeInMinAudioMs,
			IncompleteHoldMs: h.profile.BargeInHoldAudioMs,
		})
		if runtime.voiceSession != nil {
			runtime.voiceSession.RecordInterruptionDecision(decision)
		}
		directive := decision.PlaybackDirective()
		if directive.ShouldDuckOutput() {
			runtime.applyOutputDirective(directive)
			attrs := appendBargeInDecisionLogAttrs(nil, decision)
			attrs = append(attrs,
				"barge_in_action", directive.Action,
				"output_gain", directive.Gain,
				"duck_hold_ms", directive.Hold.Milliseconds(),
			)
			attrs = appendInputPreviewTraceLogAttrs(attrs, observation.Trace, time.Now().UTC())
			logTurnTraceInfo(h.logger, "gateway barge-in soft directive applied", previous.SessionID, runtime.turnTrace.Current(), attrs...)
		}
		if !directive.ShouldInterruptOutput() {
			if decision.Policy != voice.InterruptionPolicyIgnore {
				attrs := appendBargeInDecisionLogAttrs(nil, decision)
				attrs = appendInputPreviewTraceLogAttrs(attrs, observation.Trace, time.Now().UTC())
				logTurnTraceInfo(h.logger, "gateway barge-in policy observed", previous.SessionID, runtime.turnTrace.Current(), attrs...)
			}
			if err := applyReadDeadline(runtime, previous, h.profile); err != nil {
				return err
			}
			return nil
		}
		attrs := appendBargeInDecisionLogAttrs(nil, decision)
		attrs = appendInputPreviewTraceLogAttrs(attrs, observation.Trace, time.Now().UTC())
		logTurnTraceInfo(h.logger, "gateway barge-in accepted", previous.SessionID, runtime.turnTrace.Current(), attrs...)
		if err := interruptSpeakingFlowWithOptions(runtime, h.profile, h.logger, interruptSpeakingOptions{
			ClearPreview: false,
			ClearPending: false,
			Decision:     &decision,
		}, nil, func(trace turnTrace, active session.Snapshot) error {
			return runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdateFromSnapshot(active, trace.TurnID, ""))
		}); err != nil {
			return err
		}
		if _, err := runtime.flushPendingBargeInAudio(); err != nil {
			return err
		}
		return nil
	}

	current, err := runtime.session.IngestOwnedAudioFrame(normalizedPayload)
	if err != nil {
		if errors.Is(err, session.ErrNoActiveSession) {
			if sendErr := runtime.peer.WriteEvent(events.TypeError, "", errorPayload{
				Code:        "session_not_started",
				Message:     "session.start must be sent before binary audio frames",
				Recoverable: true,
			}); sendErr != nil {
				return sendErr
			}
			return nil
		}
		return err
	}

	if h.profile.ServerEndpointEnabled && (previous.State == session.StateActive || previous.State == session.StateSpeaking) {
		if err := runtime.ensureInputPreview(context.Background(), h.responder, current, ""); err != nil {
			h.logger.Warn("gateway input preview start failed", "session_id", current.SessionID, "error", err)
		} else {
			observation, pushErr := runtime.pushInputPreviewAudio(context.Background(), normalizedPayload)
			if pushErr != nil {
				h.logger.Warn("gateway input preview push failed", "session_id", current.SessionID, "error", pushErr)
			} else {
				if observation.PartialChanged {
					logInputPreviewTraceInfo(h.logger, "gateway input preview updated", current.SessionID, observation.Trace,
						"partial_text", observation.Preview.PartialText,
						"audio_bytes", observation.Preview.AudioBytes,
					)
				}
				if observation.SpeechStartedObserved {
					logInputPreviewTraceInfo(h.logger, "gateway input preview speech started", current.SessionID, observation.Trace,
						"audio_bytes", observation.Preview.AudioBytes,
					)
				}
				if observation.EndpointCandidateObserved {
					logInputPreviewTraceInfo(h.logger, "gateway input preview endpoint candidate", current.SessionID, observation.Trace,
						"partial_text", observation.Preview.PartialText,
						"audio_bytes", observation.Preview.AudioBytes,
						"endpoint_reason", observation.Preview.EndpointReason,
					)
				}
				if observation.CommitSuggested {
					logInputPreviewTraceInfo(h.logger, "gateway input preview commit suggested", current.SessionID, observation.Trace,
						"partial_text", observation.Preview.PartialText,
						"audio_bytes", observation.Preview.AudioBytes,
						"endpoint_reason", observation.Preview.EndpointReason,
					)
				}
				if err := h.emitPreviewObservationEvents(runtime, current, observation); err != nil {
					return err
				}
			}
		}
	}

	if err := applyReadDeadline(runtime, current, h.profile); err != nil {
		return err
	}

	if previous.State == session.StateSpeaking {
		return runtime.peer.WriteEvent(events.TypeSessionUpdate, current.SessionID, sessionUpdateFromSnapshot(current, runtime.turnTrace.Current().TurnID, ""))
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
	case events.TypeAudioOutStarted:
		return h.handleAudioOutStarted(runtime, envelope)
	case events.TypeAudioOutMark:
		return h.handleAudioOutMark(runtime, envelope)
	case events.TypeAudioOutCleared:
		return h.handleAudioOutCleared(runtime, envelope)
	case events.TypeAudioOutCompleted:
		return h.handleAudioOutCompleted(runtime, envelope)
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
	runtime.collaboration = negotiateVoiceCollaboration(h.profile, payload.Capabilities)
	runtime.clearPlaybackAckState()

	if err := applyReadDeadline(runtime, snapshot, h.profile); err != nil {
		return err
	}

	return runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdateFromSnapshot(snapshot, "", ""))
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

	snapshot := runtime.session.Snapshot()
	if snapshot.State == session.StateSpeaking && runtime.hasPendingBargeInAudio() {
		if err := interruptSpeakingFlowWithOptions(runtime, h.profile, h.logger, interruptSpeakingOptions{
			ClearPreview: false,
			ClearPending: false,
			Policy:       voice.InterruptionPolicyHardInterrupt,
			Reason:       firstNonEmpty(strings.TrimSpace(payload.Reason), "explicit_commit_during_speaking"),
		}, nil, func(trace turnTrace, active session.Snapshot) error {
			return runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdateFromSnapshot(active, trace.TurnID, ""))
		}); err != nil {
			return err
		}
		flushedSnapshot, err := runtime.flushPendingBargeInAudio()
		if err != nil {
			return err
		}
		snapshot = flushedSnapshot
	}
	if snapshot.State != session.StateActive {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "turn_not_ready",
			Message:     "audio.in.commit is accepted only while the session is active",
			Recoverable: true,
		})
	}
	previewTrace, previewResult, previewResultOK, previewErr := runtime.consumeInputPreview(ctx)
	if previewErr != nil {
		h.logger.Warn("gateway input preview finalize failed",
			"session_id", envelope.SessionID,
			"trace_id", runtime.turnTrace.Current().TraceID,
			"error", previewErr,
		)
	}
	turn, err := runtime.session.CommitTurn()
	if err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot commit audio before session.start",
			Recoverable: true,
		})
	}
	runtime.resetPendingBargeInAudio()

	if err := applyReadDeadline(runtime, turn.Snapshot, h.profile); err != nil {
		return err
	}

	turnMeta := runtime.turnTrace.Begin(turn.Snapshot.SessionID, "audio_commit")
	attrs := []any{
		"input_type", "audio",
		"audio_bytes", len(turn.AudioPCM),
		"input_codec", turn.Snapshot.InputCodec,
		"input_sample_rate_hz", turn.Snapshot.InputSampleRate,
		"input_channels", turn.Snapshot.InputChannels,
		"turn_index", turn.Snapshot.Turns,
	}
	if previewResultOK && previewErr == nil {
		attrs = appendPreviewTranscriptionLogAttrs(attrs, &previewResult)
	}
	attrs = appendInputPreviewTraceLogAttrs(attrs, previewTrace, time.Now().UTC())
	logTurnTraceInfo(h.logger, "gateway turn accepted", turn.Snapshot.SessionID, turnMeta, attrs...)
	if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, turn.Snapshot.SessionID, sessionUpdateFromSnapshot(turn.Snapshot, turnMeta.TurnID, firstNonEmpty(strings.TrimSpace(payload.Reason), "audio_commit"))); err != nil {
		return err
	}

	var previewTranscription *voice.TranscriptionResult
	if previewResultOK && previewErr == nil {
		previewTranscription = &previewResult
	}
	return h.emitTurnResponse(ctx, runtime, turn, turnMeta, "", previewTranscription)
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
		if err := interruptSpeakingFlowWithOptions(runtime, h.profile, h.logger, interruptSpeakingOptions{
			ClearPreview: true,
			ClearPending: true,
			Policy:       voice.InterruptionPolicyHardInterrupt,
			Reason:       "text_input_during_speaking",
		}, nil, func(trace turnTrace, active session.Snapshot) error {
			return runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdateFromSnapshot(active, trace.TurnID, ""))
		}); err != nil {
			return err
		}
	}

	if _, err := runtime.session.AcceptText(); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot send text before session.start",
			Recoverable: true,
		})
	}

	runtime.clearInputPreview()
	turn, err := runtime.session.CommitTurn()
	if err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "session_not_started",
			Message:     "cannot send text before session.start",
			Recoverable: true,
		})
	}

	if err := applyReadDeadline(runtime, turn.Snapshot, h.profile); err != nil {
		return err
	}

	turnMeta := runtime.turnTrace.Begin(turn.Snapshot.SessionID, "text_input")
	logTurnTraceInfo(h.logger, "gateway turn accepted", turn.Snapshot.SessionID, turnMeta,
		"input_type", "text",
		"text_len", len(strings.TrimSpace(payload.Text)),
		"turn_index", turn.Snapshot.Turns,
	)
	if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, turn.Snapshot.SessionID, sessionUpdateFromSnapshot(turn.Snapshot, turnMeta.TurnID, "text_input")); err != nil {
		return err
	}

	return h.emitTurnResponse(ctx, runtime, turn, turnMeta, payload.Text, nil)
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
		return interruptSpeakingFlowWithOptions(runtime, h.profile, h.logger, interruptSpeakingOptions{
			ClearPreview: true,
			ClearPending: true,
			Policy:       voice.InterruptionPolicyHardInterrupt,
			Reason:       "client_interrupt_hint",
		}, nil, func(trace turnTrace, active session.Snapshot) error {
			return runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdateFromSnapshot(active, trace.TurnID, ""))
		})
	}

	return runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdateFromSnapshot(snapshot, "", ""))
}

func (h *realtimeWSHandler) handleAudioOutStarted(runtime *connectionRuntime, envelope controlEnvelope) error {
	var payload audioOutStartedPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "invalid_audio_out_started",
			Message:     err.Error(),
			Recoverable: true,
		})
	}
	return h.recordPlaybackAckStarted(runtime, payload)
}

func (h *realtimeWSHandler) handleAudioOutMark(runtime *connectionRuntime, envelope controlEnvelope) error {
	var payload audioOutMarkPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "invalid_audio_out_mark",
			Message:     err.Error(),
			Recoverable: true,
		})
	}
	return h.recordPlaybackAckMark(runtime, payload)
}

func (h *realtimeWSHandler) handleAudioOutCleared(runtime *connectionRuntime, envelope controlEnvelope) error {
	var payload audioOutClearedPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "invalid_audio_out_cleared",
			Message:     err.Error(),
			Recoverable: true,
		})
	}
	return h.recordPlaybackAckCleared(runtime, payload)
}

func (h *realtimeWSHandler) handleAudioOutCompleted(runtime *connectionRuntime, envelope controlEnvelope) error {
	var payload audioOutCompletedPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return runtime.peer.WriteEvent(events.TypeError, envelope.SessionID, errorPayload{
			Code:        "invalid_audio_out_completed",
			Message:     err.Error(),
			Recoverable: true,
		})
	}
	return h.recordPlaybackAckCompleted(runtime, payload)
}

func (h *realtimeWSHandler) recordPlaybackAckStarted(runtime *connectionRuntime, payload audioOutStartedPayload) error {
	if runtime.session.Snapshot().SessionID == "" || !runtime.collaboration.PlaybackAckEnabled() {
		return nil
	}
	meta := runtime.playbackAckMeta()
	if !validatePlaybackAckIdentity(meta, strings.TrimSpace(payload.ResponseID), strings.TrimSpace(payload.PlaybackID)) {
		h.logger.Warn("gateway playback ack started ignored because identity mismatched",
			"session_id", runtime.session.Snapshot().SessionID,
			"remote_addr", runtime.remoteAddr,
			"response_id", payload.ResponseID,
			"playback_id", payload.PlaybackID,
			"expected_response_id", meta.ResponseID,
			"expected_playback_id", meta.PlaybackID,
		)
		return nil
	}
	runtime.recordPlaybackStarted(time.Now().UTC())
	if runtime.voiceSession != nil {
		runtime.voiceSession.ObservePlaybackStartedFact()
	}
	logPlaybackAckInfo(h.logger, "gateway playback ack started", runtime, payload)
	return nil
}

func (h *realtimeWSHandler) recordPlaybackAckMark(runtime *connectionRuntime, payload audioOutMarkPayload) error {
	if runtime.session.Snapshot().SessionID == "" || !runtime.collaboration.PlaybackAckEnabled() {
		return nil
	}
	meta := runtime.playbackAckMeta()
	if !validatePlaybackAckIdentity(meta, strings.TrimSpace(payload.ResponseID), strings.TrimSpace(payload.PlaybackID)) {
		h.logger.Warn("gateway playback ack mark ignored because identity mismatched",
			"session_id", runtime.session.Snapshot().SessionID,
			"remote_addr", runtime.remoteAddr,
			"response_id", payload.ResponseID,
			"playback_id", payload.PlaybackID,
			"expected_response_id", meta.ResponseID,
			"expected_playback_id", meta.PlaybackID,
		)
		return nil
	}
	_, totalPlayed, heardText := runtime.recordPlaybackMark(time.Now().UTC(), strings.TrimSpace(payload.SegmentID), payload.PlayedDurationMs)
	if runtime.voiceSession != nil {
		switch {
		case strings.TrimSpace(heardText) != "":
			runtime.voiceSession.ObservePlaybackMarkTextFact(totalPlayed, heardText)
		case totalPlayed > 0:
			runtime.voiceSession.ObservePlaybackMarkFact(totalPlayed)
		}
	}
	logPlaybackAckInfo(h.logger, "gateway playback ack mark", runtime, payload)
	return nil
}

func (h *realtimeWSHandler) recordPlaybackAckCleared(runtime *connectionRuntime, payload audioOutClearedPayload) error {
	if runtime.session.Snapshot().SessionID == "" || !runtime.collaboration.PlaybackAckEnabled() {
		return nil
	}
	meta := runtime.playbackAckMeta()
	if !validatePlaybackAckIdentity(meta, strings.TrimSpace(payload.ResponseID), strings.TrimSpace(payload.PlaybackID)) {
		h.logger.Warn("gateway playback ack cleared ignored because identity mismatched",
			"session_id", runtime.session.Snapshot().SessionID,
			"remote_addr", runtime.remoteAddr,
			"response_id", payload.ResponseID,
			"playback_id", payload.PlaybackID,
			"expected_response_id", meta.ResponseID,
			"expected_playback_id", meta.PlaybackID,
		)
		return nil
	}
	_, totalPlayed, heardText := runtime.recordPlaybackCleared(time.Now().UTC(), strings.TrimSpace(payload.ClearedAfterSegmentID), payload.Reason)
	if runtime.voiceSession != nil {
		switch {
		case strings.TrimSpace(heardText) != "":
			runtime.voiceSession.ObservePlaybackMarkTextFact(totalPlayed, heardText)
		case totalPlayed > 0:
			runtime.voiceSession.ObservePlaybackMarkFact(totalPlayed)
		}
	}
	logPlaybackAckInfo(h.logger, "gateway playback ack cleared", runtime, payload)
	snapshot := runtime.session.Snapshot()
	if snapshot.State == session.StateSpeaking {
		return interruptSpeakingFlowWithOptions(runtime, h.profile, h.logger, interruptSpeakingOptions{
			ClearPreview: false,
			ClearPending: false,
			Policy:       voice.InterruptionPolicyHardInterrupt,
			Reason:       firstNonEmpty(strings.TrimSpace(payload.Reason), "client_playback_cleared"),
		}, nil, func(trace turnTrace, active session.Snapshot) error {
			return runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdateFromSnapshot(active, trace.TurnID, ""))
		})
	}
	return nil
}

func (h *realtimeWSHandler) recordPlaybackAckCompleted(runtime *connectionRuntime, payload audioOutCompletedPayload) error {
	if runtime.session.Snapshot().SessionID == "" || !runtime.collaboration.PlaybackAckEnabled() {
		return nil
	}
	meta := runtime.playbackAckMeta()
	if !validatePlaybackAckIdentity(meta, strings.TrimSpace(payload.ResponseID), strings.TrimSpace(payload.PlaybackID)) {
		h.logger.Warn("gateway playback ack completed ignored because identity mismatched",
			"session_id", runtime.session.Snapshot().SessionID,
			"remote_addr", runtime.remoteAddr,
			"response_id", payload.ResponseID,
			"playback_id", payload.PlaybackID,
			"expected_response_id", meta.ResponseID,
			"expected_playback_id", meta.PlaybackID,
		)
		return nil
	}
	runtime.recordPlaybackCompleted(time.Now().UTC())
	if runtime.voiceSession != nil {
		runtime.voiceSession.ObservePlaybackCompletedFact()
	}
	logPlaybackAckInfo(h.logger, "gateway playback ack completed", runtime, payload)
	return nil
}

func (h *realtimeWSHandler) emitTurnResponse(ctx context.Context, runtime *connectionRuntime, turn session.CommittedTurn, trace turnTrace, text string, previewTranscription *voice.TranscriptionResult) error {
	request := buildTurnRequest(turn, runtime, trace, text, previewTranscription)
	logPreviousPlaybackContext(h.logger, turn.Snapshot.SessionID, trace, request)
	provisionalInputText := strings.TrimSpace(request.Text)
	if provisionalInputText == "" && request.PreviewTranscription != nil {
		provisionalInputText = strings.TrimSpace(request.PreviewTranscription.Text)
	}
	if runtime.voiceSession != nil {
		runtime.voiceSession.PrepareTurn(request, provisionalInputText, "")
	}
	result, err := executeTurnResponse(ctx, request, trace, turnExecutionOptions{
		Runtime:   runtime,
		Responder: h.responder,
		Logger:    h.logger,
		SessionID: turn.Snapshot.SessionID,
		EmitResponseStart: func(trace turnTrace, responseID string, modalities []string, _ voice.TurnResponse) error {
			if err := runtime.peer.WriteEvent(events.TypeResponseStart, turn.Snapshot.SessionID, responseStartPayload{
				ResponseID: responseID,
				Modalities: modalities,
				TurnID:     trace.TurnID,
				TraceID:    trace.TraceID,
			}); err != nil {
				logTurnTraceError(h.logger, "gateway response.start write failed", turn.Snapshot.SessionID, trace, err,
					"remote_addr", runtime.remoteAddr,
					"response_id", responseID,
					"modalities", strings.Join(modalities, ","),
					"ws_stage", "response_start",
				)
				return err
			}
			logTurnTraceInfo(h.logger, "gateway response.start sent", turn.Snapshot.SessionID, trace,
				"remote_addr", runtime.remoteAddr,
				"response_id", responseID,
				"modalities", strings.Join(modalities, ","),
			)
			return nil
		},
		EmitResponseDelta: func(responseID string, delta voice.ResponseDelta) error {
			return h.emitResponseDelta(runtime, turn.Snapshot.SessionID, responseID, delta)
		},
		OnTextDeltaCollected: func(trace turnTrace, aggregatedText string) {
			if runtime.voiceSession == nil || strings.TrimSpace(aggregatedText) == "" {
				return
			}
			if snapshot := runtime.session.Snapshot(); snapshot.OutputState == session.OutputStateSpeaking {
				runtime.voiceSession.UpdatePlayback(aggregatedText, 0)
			}
		},
		StartResponseAudio: func(trace turnTrace, responseID string, audioStart voice.ResponseAudioStart, aggregatedText string, completion *turnOutputOutcomeFuture) error {
			speaking, err := runtime.session.SetOutputState(session.OutputStateSpeaking)
			if err != nil {
				return err
			}
			trace = runtime.turnTrace.MarkSpeaking()
			logTurnTraceInfo(h.logger, "gateway turn speaking", turn.Snapshot.SessionID, trace,
				"speaking_latency_ms", trace.SpeakingLatencyMs(),
				"audio_start_source", string(audioStart.Source),
				"audio_start_incremental", audioStart.Incremental,
			)
			if runtime.voiceSession != nil {
				runtime.voiceSession.StartPlaybackWithOptions(aggregatedText, outputFrameInterval, plannedPlaybackDurationForAudioStream(audioStart.Stream, outputFrameInterval), voice.PlaybackStartOptions{
					PreferClientFacts: runtime.collaboration.PlaybackAckEnabled(),
				})
			}
			if err := applyReadDeadline(runtime, speaking, h.profile); err != nil {
				return err
			}
			if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, turn.Snapshot.SessionID, sessionUpdateFromSnapshot(speaking, trace.TurnID, "")); err != nil {
				logTurnTraceError(h.logger, "gateway speaking update write failed", turn.Snapshot.SessionID, trace, err,
					"remote_addr", runtime.remoteAddr,
					"ws_stage", "session_update_speaking",
				)
				return err
			}
			logTurnTraceInfo(h.logger, "gateway speaking update sent", turn.Snapshot.SessionID, trace,
				"remote_addr", runtime.remoteAddr,
				"audio_start_source", string(audioStart.Source),
			)
			h.startAudioStream(ctx, runtime, turn.Snapshot.SessionID, trace, newAudioPlaybackMeta(responseID, aggregatedText, plannedPlaybackDurationForAudioStream(audioStart.Stream, outputFrameInterval)), audioStart.Stream, completion)
			return nil
		},
	})
	if err != nil {
		runtime.interruptOutput(100 * time.Millisecond)
		return h.terminateWithError(runtime, "response_generation_failed", err.Error())
	}
	inputText := strings.TrimSpace(result.Response.InputText)
	if inputText == "" {
		inputText = strings.TrimSpace(text)
	}
	deliveredText := strings.TrimSpace(result.AggregatedText)
	if deliveredText == "" {
		deliveredText = strings.TrimSpace(result.Response.Text)
	}
	if runtime.voiceSession != nil {
		runtime.voiceSession.PrepareTurn(request, inputText, strings.TrimSpace(result.Response.Text))
		if result.Response.AudioStreamTransferred {
			runtime.voiceSession.UpdatePlayback(deliveredText, 0)
		}
	}
	if result.Response.AudioStreamTransferred {
		return nil
	}
	return h.finalizeTurnResponse(ctx, runtime, turn.Snapshot, result.Trace, result.ResponseID, result.Response, deliveredText)
}

func buildTurnRequest(turn session.CommittedTurn, runtime *connectionRuntime, trace turnTrace, text string, previewTranscription *voice.TranscriptionResult) voice.TurnRequest {
	inputCodec := turn.Snapshot.InputCodec
	inputSampleRate := turn.Snapshot.InputSampleRate
	inputChannels := turn.Snapshot.InputChannels
	if runtime.inputNormalizer != nil {
		inputCodec = runtime.inputNormalizer.OutputCodec()
		inputSampleRate = runtime.inputNormalizer.OutputSampleRate()
		inputChannels = runtime.inputNormalizer.OutputChannels()
	}
	metadata := map[string]string(nil)
	if runtime != nil && runtime.voiceSession != nil {
		metadata = runtime.voiceSession.LastPlaybackContextMetadata()
	}

	return voice.TurnRequest{
		SessionID:            turn.Snapshot.SessionID,
		TurnID:               trace.TurnID,
		TraceID:              trace.TraceID,
		DeviceID:             turn.Snapshot.DeviceID,
		ClientType:           turn.Snapshot.ClientType,
		Text:                 text,
		Metadata:             metadata,
		PreviewTranscription: clonePreviewTranscription(previewTranscription),
		AudioPCM:             turn.AudioPCM,
		AudioBytes:           turn.Snapshot.AudioBytes,
		InputFrames:          turn.Snapshot.InputFrames,
		InputCodec:           inputCodec,
		InputSampleRate:      inputSampleRate,
		InputChannels:        inputChannels,
	}
}

func clonePreviewTranscription(result *voice.TranscriptionResult) *voice.TranscriptionResult {
	if result == nil {
		return nil
	}
	cloned := *result
	cloned.Segments = append([]string(nil), result.Segments...)
	cloned.AudioEvents = append([]string(nil), result.AudioEvents...)
	cloned.Partials = append([]string(nil), result.Partials...)
	return &cloned
}

func appendPreviewTranscriptionLogAttrs(attrs []any, result *voice.TranscriptionResult) []any {
	if result == nil {
		return attrs
	}
	return append(attrs,
		"preview_finalize_fast_path", true,
		"preview_finalize_text_len", len(strings.TrimSpace(result.Text)),
		"preview_finalize_mode", strings.TrimSpace(result.Mode),
		"preview_finalize_endpoint_reason", strings.TrimSpace(result.EndpointReason),
	)
}

func appendBargeInDecisionLogAttrs(attrs []any, decision voice.BargeInDecision) []any {
	return append(attrs,
		"barge_in_policy", decision.Policy,
		"barge_in_reason", decision.Reason,
		"barge_in_audio_ms", decision.AudioMs,
		"barge_in_lexically_complete", decision.LexicallyComplete,
		"barge_in_min_audio_ms", decision.MinAudioMs,
		"barge_in_hold_audio_ms", decision.HoldAudioMs,
		"barge_in_acoustic_ready", decision.AcousticReady,
		"barge_in_semantic_ready", decision.SemanticReady,
		"barge_in_accept_candidate", decision.AcceptCandidate,
		"barge_in_accept_now", decision.AcceptNow,
		"barge_in_endpoint_hinted", decision.EndpointHinted,
		"barge_in_backchannel_likely", decision.BackchannelLikely,
		"barge_in_takeover_lexicon", decision.TakeoverLexicon,
		"barge_in_turn_stage", decision.TurnStage,
		"barge_in_stability", decision.Stability,
		"barge_in_stable_prefix_runes", decision.StablePrefixRunes,
		"barge_in_intrusion_score", decision.IntrusionScore,
		"barge_in_takeover_score", decision.TakeoverScore,
	)
}

func logPreviousPlaybackContext(logger *slog.Logger, sessionID string, trace turnTrace, request voice.TurnRequest) {
	if logger == nil {
		return
	}
	if request.Metadata["voice.previous.available"] != "true" {
		return
	}
	logTurnTraceInfo(logger, "gateway turn request carries previous playback context", sessionID, trace,
		"previous_turn_id", request.Metadata["voice.previous.turn_id"],
		"previous_heard_boundary", request.Metadata["voice.previous.heard_boundary"],
		"previous_heard_confidence", request.Metadata["voice.previous.heard_confidence"],
		"previous_response_interrupted", request.Metadata["voice.previous.response_interrupted"],
		"previous_resume_anchor", request.Metadata["voice.previous.resume_anchor"],
		"previous_missed_text", request.Metadata["voice.previous.missed_text"],
	)
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

func (h *realtimeWSHandler) finalizeTurnResponse(ctx context.Context, runtime *connectionRuntime, snapshot session.Snapshot, trace turnTrace, responseID string, response voice.TurnResponse, deliveredText string) error {
	return finalizeTurnLifecycle(trace, response, deliveredText, turnFinalizeHooks{
		Completion: turnCompletionHooks{
			Runtime:   runtime,
			Profile:   h.profile,
			Logger:    h.logger,
			SessionID: snapshot.SessionID,
			OnReturnActive: func(trace turnTrace, active session.Snapshot) error {
				return runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdateFromSnapshot(active, trace.TurnID, ""))
			},
			OnEndSession: func(_ turnTrace, reason session.CloseReason, message string) error {
				return h.endSession(runtime, reason, message)
			},
		},
		BeforeNoAudio: func(_ turnTrace, _ voice.TurnResponse, aggregatedText string) error {
			if runtime.voiceSession != nil {
				runtime.voiceSession.FinalizeTextResponse(aggregatedText)
			}
			return nil
		},
		BeforeSpeaking: func(trace turnTrace) error {
			if runtime.voiceSession != nil {
				runtime.voiceSession.StartPlaybackWithOptions(deliveredText, outputFrameInterval, plannedPlaybackDurationForResponse(response, outputFrameInterval), voice.PlaybackStartOptions{
					PreferClientFacts: runtime.collaboration.PlaybackAckEnabled(),
				})
			}
			speaking := runtime.session.Snapshot()
			if err := runtime.peer.WriteEvent(events.TypeSessionUpdate, snapshot.SessionID, sessionUpdateFromSnapshot(speaking, trace.TurnID, "")); err != nil {
				logTurnTraceError(h.logger, "gateway speaking update write failed", snapshot.SessionID, trace, err,
					"remote_addr", runtime.remoteAddr,
					"ws_stage", "session_update_speaking",
				)
				return err
			}
			logTurnTraceInfo(h.logger, "gateway speaking update sent", snapshot.SessionID, trace,
				"remote_addr", runtime.remoteAddr,
			)
			return nil
		},
		StartAudioStream: func(trace turnTrace, audioStream voice.AudioStream, response voice.TurnResponse, _ string) error {
			h.startAudioStream(ctx, runtime, snapshot.SessionID, trace, newAudioPlaybackMeta(responseID, deliveredText, plannedPlaybackDurationForResponse(response, outputFrameInterval)), audioStream, resolvedTurnOutputOutcome(turnOutputOutcome{
				EndSession: response.EndSession,
				EndReason:  response.EndReason,
				EndMessage: response.EndMessage,
			}))
			return nil
		},
	})
}

func (h *realtimeWSHandler) startAudioStream(
	ctx context.Context,
	runtime *connectionRuntime,
	sessionID string,
	trace turnTrace,
	meta audioPlaybackMeta,
	audioStream voice.AudioStream,
	completion *turnOutputOutcomeFuture,
) {
	streamCtx, cancel := context.WithCancel(ctx)
	stream := runtime.installOutput(cancel, completion)
	_, sourceSegmented := audioStream.(voice.SegmentedAudioStream)
	audioStream = newPCM16EffectAudioStream(audioStream, &stream.effects)
	runtime.installPlaybackAckMeta(meta)

	go func() {
		defer close(stream.done)
		defer runtime.clearOutput(stream)
		defer runtime.clearPlaybackAckState()
		defer func() { _ = audioStream.Close() }()

		ticker := time.NewTicker(outputFrameInterval)
		defer ticker.Stop()
		audioChunkIndex := 0
		var segmentedStream voice.SegmentedAudioStream
		if sourceSegmented {
			segmentedStream, _ = audioStream.(voice.SegmentedAudioStream)
		}
		segmentCursor := audioPlaybackMeta{
			ResponseID: meta.ResponseID,
			PlaybackID: meta.PlaybackID,
		}

		if !sourceSegmented {
			if err := h.emitAudioOutMeta(runtime, sessionID, trace, meta); err != nil {
				return
			}
		}

		for {
			if streamCtx.Err() != nil {
				return
			}

			if sourceSegmented {
				segment, ok, err := segmentedStream.NextSegment(streamCtx)
				if err != nil {
					if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
						break
					}
					return
				}
				if ok {
					segmentCursor = nextSegmentAudioPlaybackMeta(segmentCursor, segment.Text, segment.ExpectedDuration, segment.IsLastSegment)
					if err := h.emitAudioOutMeta(runtime, sessionID, trace, segmentCursor); err != nil {
						return
					}
				}
			}

			chunk, err := audioStream.Next(streamCtx)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
					if sourceSegmented && errors.Is(err, io.EOF) {
						continue
					}
					break
				}
				if streamCtx.Err() != nil {
					return
				}
				current := runtime.session.Snapshot()
				if current.SessionID == sessionID && current.State == session.StateSpeaking {
					if runtime.voiceSession != nil {
						runtime.voiceSession.InterruptPlayback()
					}
					logTurnTraceError(h.logger, "gateway turn audio stream failed", sessionID, runtime.turnTrace.Current(), err)
					_ = runtime.peer.WriteEvent(events.TypeError, sessionID, errorPayload{Code: "audio_stream_failed", Message: err.Error(), Recoverable: false})
					_ = h.endSession(runtime, session.CloseReasonError, err.Error())
				}
				return
			}
			if len(chunk) == 0 {
				continue
			}
			audioChunkIndex++
			if err := runtime.peer.WriteBinary(chunk); err != nil {
				logTurnTraceError(h.logger, "gateway audio binary write failed", sessionID, runtime.turnTrace.Current(), err,
					"remote_addr", runtime.remoteAddr,
					"audio_chunk_index", audioChunkIndex,
					"audio_chunk_bytes", len(chunk),
					"ws_stage", "audio_binary",
				)
				return
			}
			markTurnFirstAudioChunk(runtime, h.logger, sessionID, len(chunk))
			if runtime.voiceSession != nil && !runtime.collaboration.PlaybackAckEnabled() {
				runtime.voiceSession.ObservePlaybackChunk()
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
			runtime.resetPendingBargeInAudio()
			runtime.clearInputPreview()
			return
		}
		if runtime.voiceSession != nil {
			if runtime.collaboration.PlaybackAckEnabled() {
				terminal, clearedReason, ok := runtime.waitForPlaybackAckTerminal(streamCtx, playbackAckTerminalWaitTimeout(meta))
				switch {
				case ok && terminal == playbackAckTerminalCompleted:
					runtime.voiceSession.CompletePlaybackWithSource(voice.HeardTextSourcePlaybackCompleted)
				case ok && terminal == playbackAckTerminalCleared:
					runtime.voiceSession.InterruptPlaybackWithPolicy(voice.InterruptionPolicyHardInterrupt, firstNonEmpty(strings.TrimSpace(clearedReason), "client_playback_cleared"))
				default:
					runtime.voiceSession.CompletePlaybackWithSource(voice.HeardTextSourceHeuristicBytes)
				}
			} else {
				runtime.voiceSession.CompletePlayback()
			}
		}

		outcome, err := stream.completion.Wait(streamCtx)
		if err != nil {
			return
		}

		if outcome.EndSession {
			_ = completeTurnReturnOrClose(trace, true, outcome.EndReason, outcome.EndMessage, turnCompletionHooks{
				Runtime:   runtime,
				Profile:   h.profile,
				Logger:    h.logger,
				SessionID: sessionID,
				OnEndSession: func(_ turnTrace, reason session.CloseReason, message string) error {
					return h.endSession(runtime, reason, message)
				},
			})
			return
		}

		_ = completeTurnReturnOrClose(trace, false, outcome.EndReason, outcome.EndMessage, turnCompletionHooks{
			Runtime:   runtime,
			Profile:   h.profile,
			Logger:    h.logger,
			SessionID: sessionID,
			ResolveReturnActive: func() (session.Snapshot, bool, error) {
				return runtime.resolvePostPlaybackActiveSnapshot()
			},
			OnReturnActive: func(trace turnTrace, active session.Snapshot) error {
				return runtime.peer.WriteEvent(events.TypeSessionUpdate, active.SessionID, sessionUpdateFromSnapshot(active, trace.TurnID, ""))
			},
		})
	}()
}

func (h *realtimeWSHandler) emitAudioOutMeta(runtime *connectionRuntime, sessionID string, trace turnTrace, meta audioPlaybackMeta) error {
	runtime.activatePlaybackAckSegment(meta)
	if !runtime.collaboration.PlaybackAckEnabled() {
		return nil
	}
	if err := runtime.peer.WriteEvent(events.TypeAudioOutMeta, sessionID, audioOutMetaPayload{
		ResponseID:         meta.ResponseID,
		PlaybackID:         meta.PlaybackID,
		SegmentID:          meta.SegmentID,
		Text:               meta.Text,
		ExpectedDurationMs: audioDurationMs(meta.ExpectedDuration),
		IsLastSegment:      meta.IsLastSegment,
	}); err != nil {
		logTurnTraceError(h.logger, "gateway audio.out.meta write failed", sessionID, trace, err,
			"remote_addr", runtime.remoteAddr,
			"response_id", meta.ResponseID,
			"playback_id", meta.PlaybackID,
			"segment_id", meta.SegmentID,
			"ws_stage", "audio_out_meta",
		)
		return err
	}
	logTurnTraceInfo(h.logger, "gateway audio.out.meta sent", sessionID, trace,
		"response_id", meta.ResponseID,
		"playback_id", meta.PlaybackID,
		"segment_id", meta.SegmentID,
		"segment_index", meta.SegmentIndex,
		"is_last_segment", meta.IsLastSegment,
	)
	return nil
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
	if err := validatePlaybackAckMode(payload.Capabilities.playbackAckMode()); err != nil {
		return err
	}
	return nil
}

func (h *realtimeWSHandler) endSession(runtime *connectionRuntime, reason session.CloseReason, message string) error {
	snapshot, err := runtime.session.End(reason)
	if err != nil {
		return err
	}

	if err := runtime.peer.WriteEvent(events.TypeSessionEnd, snapshot.SessionID, sessionEndPayload{Reason: string(reason), Message: message}); err != nil {
		logTurnTraceError(h.logger, "gateway session.end write failed", snapshot.SessionID, runtime.turnTrace.Current(), err,
			"remote_addr", runtime.remoteAddr,
			"end_reason", string(reason),
			"ws_stage", "session_end",
		)
		return err
	}

	runtime.session.Reset()
	runtime.inputNormalizer = nil
	runtime.clearInputPreview()
	runtime.clearPlaybackAckState()
	runtime.collaboration = collaborationNegotiation{}
	runtime.turnTrace.Clear()
	return applyReadDeadline(runtime, runtime.session.Snapshot(), h.profile)
}

func (h *realtimeWSHandler) terminateWithError(runtime *connectionRuntime, code, message string) error {
	snapshot := runtime.session.Snapshot()
	sessionID := snapshot.SessionID
	trace := runtime.turnTrace.Current()

	if err := runtime.peer.WriteEvent(events.TypeError, sessionID, errorPayload{Code: code, Message: message, Recoverable: false}); err != nil {
		return err
	}

	if sessionID != "" {
		logTurnTraceError(h.logger, "gateway turn terminated", sessionID, trace, errors.New(message), "error_code", code)
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
	return emitTurnResponseDeltas(responseID, deltas, func(responseID string, delta voice.ResponseDelta) error {
		return h.emitResponseDelta(runtime, sessionID, responseID, delta)
	})
}

func (h *realtimeWSHandler) emitResponseDelta(runtime *connectionRuntime, sessionID, responseID string, delta voice.ResponseDelta) error {
	payload := responseChunkPayload{ResponseID: responseID, DeltaType: string(delta.Kind), Text: delta.Text, ToolCallID: delta.ToolCallID, ToolName: delta.ToolName, ToolStatus: delta.ToolStatus, ToolInput: delta.ToolInput, ToolOutput: delta.ToolOutput}
	if payload.DeltaType == "" {
		payload.DeltaType = string(voice.ResponseDeltaKindText)
	}
	if err := runtime.peer.WriteEvent(events.TypeResponseChunk, sessionID, payload); err != nil {
		logTurnTraceError(h.logger, "gateway response.chunk write failed", sessionID, runtime.turnTrace.Current(), err,
			"remote_addr", runtime.remoteAddr,
			"response_id", responseID,
			"delta_type", payload.DeltaType,
			"text_len", len(payload.Text),
			"tool_name", payload.ToolName,
			"tool_status", payload.ToolStatus,
			"ws_stage", "response_chunk",
		)
		return err
	}
	return nil
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
