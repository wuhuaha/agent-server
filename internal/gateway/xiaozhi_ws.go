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

	"github.com/gorilla/websocket"
)

type xiaozhiJSONPeer struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (p *xiaozhiJSONPeer) WriteJSON(payload any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.conn.WriteJSON(payload)
}

func (p *xiaozhiJSONPeer) WriteBinary(payload []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.conn.WriteMessage(websocket.BinaryMessage, payload)
}

func (p *xiaozhiJSONPeer) WriteCompatBinary(payload []byte, protocolVersion int) error {
	wrapped, err := wrapXiaozhiBinaryFrame(payload, protocolVersion)
	if err != nil {
		return err
	}
	return p.WriteBinary(wrapped)
}

type xiaozhiAudioParams struct {
	Format        string `json:"format"`
	SampleRate    int    `json:"sample_rate"`
	Channels      int    `json:"channels"`
	FrameDuration int    `json:"frame_duration,omitempty"`
}

type xiaozhiHelloMessage struct {
	Type       string             `json:"type"`
	Version    int                `json:"version,omitempty"`
	Transport  string             `json:"transport,omitempty"`
	DeviceID   string             `json:"device_id,omitempty"`
	DeviceName string             `json:"device_name,omitempty"`
	DeviceMAC  string             `json:"device_mac,omitempty"`
	Token      string             `json:"token,omitempty"`
	Audio      xiaozhiAudioParams `json:"audio_params"`
	Features   map[string]any     `json:"features,omitempty"`
}

type xiaozhiHelloResponse struct {
	Type      string             `json:"type"`
	Version   int                `json:"version"`
	Transport string             `json:"transport"`
	SessionID string             `json:"session_id"`
	Audio     xiaozhiAudioParams `json:"audio_params"`
}

type xiaozhiListenMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	State     string `json:"state"`
	Mode      string `json:"mode,omitempty"`
	Text      string `json:"text,omitempty"`
}

type xiaozhiAbortMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type xiaozhiTTSMessage struct {
	Type      string `json:"type"`
	State     string `json:"state"`
	SessionID string `json:"session_id,omitempty"`
	Text      string `json:"text,omitempty"`
}

type xiaozhiTextMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Text      string `json:"text"`
}

type xiaozhiServerMessage struct {
	Type      string `json:"type"`
	Status    string `json:"status,omitempty"`
	Message   string `json:"message,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type xiaozhiCompatState struct {
	sessionID             string
	deviceID              string
	clientID              string
	binaryProtocolVersion int
	helloReceived         bool
	inputCodec            string
	inputSampleRate       int
	inputChannels         int
	inputFrameDurationMs  int
	listenMode            string
	audioTurnOpen         bool
}

type xiaozhiWSHandler struct {
	profile   XiaozhiCompatProfile
	responder voice.Responder
	encoder   xiaozhiOutputEncoder
	upgrader  websocket.Upgrader
	logger    *slog.Logger
}

func NewXiaozhiWSHandler(profile XiaozhiCompatProfile, responder voice.Responder) http.Handler {
	return newXiaozhiWSHandlerWithEncoder(profile, responder, nil)
}

func newXiaozhiWSHandlerWithEncoder(profile XiaozhiCompatProfile, responder voice.Responder, encoder xiaozhiOutputEncoder) http.Handler {
	if responder == nil {
		responder = voice.NewBootstrapResponder(firstNonEmpty(strings.TrimSpace(profile.SourceOutputCodec), "pcm16le"), profile.SourceOutputRate, profile.SourceOutputChannels)
	}
	if encoder == nil {
		encoder = newDefaultXiaozhiOutputEncoder()
	}
	return &xiaozhiWSHandler{
		profile:   profile,
		responder: responder,
		encoder:   encoder,
		logger:    gatewayTraceLogger(profile.Logger, "xiaozhi"),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

func (h *xiaozhiWSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
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

	peer := &xiaozhiJSONPeer{conn: conn}
	runtime := newConnectionRuntime(conn, nil, session.NewRealtimeSession())
	versionHeader := firstNonEmpty(strings.TrimSpace(r.Header.Get("Protocol-Version")), strings.TrimSpace(r.URL.Query().Get("protocol-version")))
	state := xiaozhiCompatState{
		sessionID:             fmt.Sprintf("sess_xiaozhi_%d", time.Now().UTC().UnixNano()),
		deviceID:              firstNonEmpty(strings.TrimSpace(r.Header.Get("Device-Id")), strings.TrimSpace(r.URL.Query().Get("device-id")), "xiaozhi-device"),
		clientID:              firstNonEmpty(strings.TrimSpace(r.Header.Get("Client-Id")), strings.TrimSpace(r.URL.Query().Get("client-id"))),
		binaryProtocolVersion: resolveXiaozhiProtocolVersion(versionHeader, 0, h.profile.WelcomeVersion),
		inputCodec:            firstNonEmpty(strings.ToLower(strings.TrimSpace(h.profile.InputCodec)), "opus"),
		inputSampleRate:       maxInt(h.profile.InputSampleRate, 16000),
		inputChannels:         maxInt(h.profile.InputChannels, 1),
		inputFrameDurationMs:  maxInt(h.profile.InputFrameDurationMs, 60),
		listenMode:            "auto",
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer runtime.interruptOutput(50 * time.Millisecond)

	if err := applyReadDeadline(runtime, runtime.session.Snapshot(), h.runtimeProfile()); err != nil {
		return
	}

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if handled, handleErr := h.handleReadError(ctx, runtime, peer, &state, err); handled {
				if errors.Is(handleErr, errContinueReadLoop) {
					continue
				}
				if handleErr != nil {
					return
				}
				return
			}
			return
		}

		switch messageType {
		case websocket.TextMessage:
			if err := h.handleControl(ctx, runtime, peer, &state, payload); err != nil {
				return
			}
		case websocket.BinaryMessage:
			if err := h.handleBinary(runtime, peer, &state, payload); err != nil {
				return
			}
		default:
			if err := h.emitServerError(peer, state.sessionID, "unsupported_message_type", "only text and binary websocket frames are supported"); err != nil {
				return
			}
		}
	}
}

func (h *xiaozhiWSHandler) handleReadError(ctx context.Context, runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, err error) (bool, error) {
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		return false, nil
	}

	snapshot := runtime.session.Snapshot()
	now := time.Now().UTC()
	if handled, handleErr := h.maybeHandleServerEndpointTimeout(ctx, runtime, peer, state, snapshot, now); handled {
		return true, handleErr
	}

	reason, shouldClose := deadlineCloseReason(snapshot, h.runtimeProfile(), now)
	if !shouldClose && snapshot.SessionID != "" {
		reason = session.CloseReasonError
	}

	runtime.interruptOutput(100 * time.Millisecond)
	_ = peer.WriteJSON(xiaozhiTTSMessage{Type: "tts", State: "stop", SessionID: state.sessionID})
	if snapshot.SessionID != "" {
		_, _ = runtime.session.End(reason)
		runtime.clearInputPreview()
		runtime.session.Reset()
	}
	return true, runtime.conn.Close()
}

func (h *xiaozhiWSHandler) maybeHandleServerEndpointTimeout(ctx context.Context, runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, snapshot session.Snapshot, now time.Time) (bool, error) {
	if !h.profile.ServerEndpointEnabled || snapshot.SessionID == "" || snapshot.State != session.StateActive || !state.audioTurnOpen {
		return false, nil
	}
	preview, active, partialChanged, commitSuggested := runtime.pollInputPreview(now)
	if !active {
		return false, nil
	}
	if partialChanged {
		logTurnTraceInfo(h.logger, "gateway input preview updated", snapshot.SessionID, runtime.turnTrace.Current(),
			"partial_text", preview.PartialText,
			"audio_bytes", preview.AudioBytes,
		)
	}
	if commitSuggested && snapshot.AudioBytes > 0 {
		state.audioTurnOpen = false
		runtime.clearInputPreview()
		turn, err := runtime.session.CommitTurn()
		if err != nil {
			return true, err
		}
		if err := applyReadDeadline(runtime, turn.Snapshot, h.runtimeProfile()); err != nil {
			return true, err
		}
		trace := runtime.turnTrace.Begin(turn.Snapshot.SessionID, "server_endpoint")
		logTurnTraceInfo(h.logger, "gateway turn accepted", turn.Snapshot.SessionID, trace,
			"input_type", "audio",
			"audio_bytes", len(turn.AudioPCM),
			"input_codec", turn.Snapshot.InputCodec,
			"input_sample_rate_hz", turn.Snapshot.InputSampleRate,
			"input_channels", turn.Snapshot.InputChannels,
			"turn_index", turn.Snapshot.Turns,
			"endpoint_reason", preview.EndpointReason,
		)
		if err := h.emitTurnResponse(ctx, runtime, peer, state, turn, trace, ""); err != nil {
			return true, err
		}
		return true, errContinueReadLoop
	}
	if deadlineReason, shouldClose := deadlineCloseReason(snapshot, h.runtimeProfile(), now); shouldClose {
		runtime.interruptOutput(100 * time.Millisecond)
		_ = peer.WriteJSON(xiaozhiTTSMessage{Type: "tts", State: "stop", SessionID: state.sessionID})
		_, _ = runtime.session.End(deadlineReason)
		runtime.clearInputPreview()
		runtime.session.Reset()
		return true, runtime.conn.Close()
	}
	if err := applyReadDeadline(runtime, snapshot, h.runtimeProfile()); err != nil {
		return true, err
	}
	return true, errContinueReadLoop
}

func (h *xiaozhiWSHandler) handleControl(ctx context.Context, runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, raw []byte) error {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return h.emitServerError(peer, state.sessionID, "invalid_json", "control frame is not valid json")
	}

	switch strings.ToLower(strings.TrimSpace(base.Type)) {
	case "hello":
		var msg xiaozhiHelloMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return h.emitServerError(peer, state.sessionID, "invalid_hello", err.Error())
		}
		return h.handleHello(runtime, peer, state, msg)
	case "listen":
		var msg xiaozhiListenMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return h.emitServerError(peer, state.sessionID, "invalid_listen", err.Error())
		}
		return h.handleListen(ctx, runtime, peer, state, msg)
	case "abort":
		var msg xiaozhiAbortMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return h.emitServerError(peer, state.sessionID, "invalid_abort", err.Error())
		}
		return h.handleAbort(runtime, peer, state, msg)
	case "mcp":
		return nil
	default:
		return h.emitServerError(peer, state.sessionID, "unsupported_event", fmt.Sprintf("event %s is not implemented in xiaozhi compatibility mode", base.Type))
	}
}

func (h *xiaozhiWSHandler) handleHello(runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, msg xiaozhiHelloMessage) error {
	audio := h.resolvedHelloAudio(msg)
	if err := h.validateHello(audio); err != nil {
		return h.emitServerError(peer, state.sessionID, "invalid_hello", err.Error())
	}
	inputNormalizer, err := voice.NewInputNormalizer(audio.Format, h.profile.InputSampleRate, h.profile.InputChannels)
	if err != nil {
		return h.emitServerError(peer, state.sessionID, "unsupported_audio_format", err.Error())
	}

	runtime.inputNormalizer = inputNormalizer
	state.binaryProtocolVersion = resolveXiaozhiProtocolVersion("", msg.Version, state.binaryProtocolVersion)
	state.helloReceived = true
	state.inputCodec = strings.ToLower(strings.TrimSpace(audio.Format))
	state.inputSampleRate = audio.SampleRate
	state.inputChannels = audio.Channels
	state.inputFrameDurationMs = audio.FrameDuration
	state.deviceID = firstNonEmpty(strings.TrimSpace(msg.DeviceID), strings.TrimSpace(msg.DeviceMAC), state.deviceID)

	return peer.WriteJSON(xiaozhiHelloResponse{
		Type:      "hello",
		Version:   firstNonEmptyInt(state.binaryProtocolVersion, normalizeXiaozhiProtocolVersion(h.profile.WelcomeVersion)),
		Transport: firstNonEmpty(strings.TrimSpace(h.profile.WelcomeTransport), "websocket"),
		SessionID: state.sessionID,
		Audio: xiaozhiAudioParams{
			Format:        firstNonEmpty(strings.TrimSpace(h.profile.OutputCodec), "opus"),
			SampleRate:    maxInt(h.profile.OutputSampleRate, 24000),
			Channels:      maxInt(h.profile.OutputChannels, 1),
			FrameDuration: maxInt(h.profile.OutputFrameDurationMs, 60),
		},
	})
}

func (h *xiaozhiWSHandler) resolvedHelloAudio(msg xiaozhiHelloMessage) xiaozhiAudioParams {
	return xiaozhiAudioParams{
		Format:        firstNonEmpty(strings.ToLower(strings.TrimSpace(msg.Audio.Format)), strings.ToLower(strings.TrimSpace(h.profile.InputCodec)), "opus"),
		SampleRate:    maxInt(msg.Audio.SampleRate, maxInt(h.profile.InputSampleRate, 16000)),
		Channels:      maxInt(msg.Audio.Channels, maxInt(h.profile.InputChannels, 1)),
		FrameDuration: maxInt(msg.Audio.FrameDuration, maxInt(h.profile.InputFrameDurationMs, 60)),
	}
}

func (h *xiaozhiWSHandler) validateHello(audio xiaozhiAudioParams) error {
	format := strings.ToLower(strings.TrimSpace(audio.Format))
	if format != "opus" && format != "pcm16le" {
		return fmt.Errorf("unsupported xiaozhi input format %s", format)
	}
	if audio.SampleRate <= 0 {
		return fmt.Errorf("audio_params.sample_rate must be positive")
	}
	if audio.Channels != 1 {
		return fmt.Errorf("only mono xiaozhi input is currently supported")
	}
	if format == "pcm16le" && audio.SampleRate != h.profile.InputSampleRate {
		return fmt.Errorf("pcm16le input sample_rate must be %d", h.profile.InputSampleRate)
	}
	return nil
}

func (h *xiaozhiWSHandler) handleListen(ctx context.Context, runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, msg xiaozhiListenMessage) error {
	state.listenMode = firstNonEmpty(strings.TrimSpace(msg.Mode), state.listenMode)
	switch strings.ToLower(strings.TrimSpace(msg.State)) {
	case "start":
		if err := h.interruptSpeaking(runtime, peer, state); err != nil {
			return err
		}
		if _, err := h.ensureSessionStarted(runtime, state); err != nil {
			return err
		}
		state.audioTurnOpen = true
		return nil
	case "stop":
		state.audioTurnOpen = false
		snapshot := runtime.session.Snapshot()
		if snapshot.SessionID == "" || snapshot.AudioBytes == 0 || snapshot.State != session.StateActive {
			return nil
		}
		runtime.clearInputPreview()
		turn, err := runtime.session.CommitTurn()
		if err != nil {
			return h.emitServerError(peer, state.sessionID, "commit_failed", err.Error())
		}
		if err := applyReadDeadline(runtime, turn.Snapshot, h.runtimeProfile()); err != nil {
			return err
		}
		trace := runtime.turnTrace.Begin(turn.Snapshot.SessionID, "listen_stop")
		logTurnTraceInfo(h.logger, "gateway turn accepted", turn.Snapshot.SessionID, trace,
			"input_type", "audio",
			"audio_bytes", len(turn.AudioPCM),
			"input_codec", turn.Snapshot.InputCodec,
			"input_sample_rate_hz", turn.Snapshot.InputSampleRate,
			"input_channels", turn.Snapshot.InputChannels,
			"turn_index", turn.Snapshot.Turns,
		)
		return h.emitTurnResponse(ctx, runtime, peer, state, turn, trace, "")
	case "detect":
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			return nil
		}
		if err := h.interruptSpeaking(runtime, peer, state); err != nil {
			return err
		}
		if _, err := h.ensureSessionStarted(runtime, state); err != nil {
			return err
		}
		if _, err := runtime.session.AcceptText(); err != nil {
			return h.emitServerError(peer, state.sessionID, "text_input_failed", err.Error())
		}
		turn, err := runtime.session.CommitTurn()
		if err != nil {
			return h.emitServerError(peer, state.sessionID, "text_input_failed", err.Error())
		}
		runtime.clearInputPreview()
		if err := applyReadDeadline(runtime, turn.Snapshot, h.runtimeProfile()); err != nil {
			return err
		}
		if err := peer.WriteJSON(xiaozhiTextMessage{Type: "stt", SessionID: state.sessionID, Text: text}); err != nil {
			return err
		}
		trace := runtime.turnTrace.Begin(turn.Snapshot.SessionID, "detect_text")
		logTurnTraceInfo(h.logger, "gateway turn accepted", turn.Snapshot.SessionID, trace,
			"input_type", "text",
			"text_len", len(text),
			"turn_index", turn.Snapshot.Turns,
		)
		return h.emitTurnResponse(ctx, runtime, peer, state, turn, trace, text)
	default:
		return nil
	}
}

func (h *xiaozhiWSHandler) handleAbort(runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, _ xiaozhiAbortMessage) error {
	return h.interruptSpeaking(runtime, peer, state)
}

func (h *xiaozhiWSHandler) handleBinary(runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	unwrapped, err := unwrapXiaozhiBinaryFrame(payload, state.binaryProtocolVersion)
	if err != nil {
		return h.emitServerError(peer, state.sessionID, "invalid_audio_frame", err.Error())
	}
	if h.profile.MaxFrameBytes > 0 && len(unwrapped) > h.profile.MaxFrameBytes {
		return h.emitServerError(peer, state.sessionID, "frame_too_large", fmt.Sprintf("binary frame exceeds %d bytes", h.profile.MaxFrameBytes))
	}
	if !state.audioTurnOpen && runtime.session.Snapshot().SessionID == "" {
		return nil
	}
	if err := h.interruptSpeaking(runtime, peer, state); err != nil {
		return err
	}
	if runtime.inputNormalizer == nil {
		inputNormalizer, err := voice.NewInputNormalizer(state.inputCodec, h.profile.InputSampleRate, h.profile.InputChannels)
		if err != nil {
			return h.emitServerError(peer, state.sessionID, "unsupported_audio_format", err.Error())
		}
		runtime.inputNormalizer = inputNormalizer
	}
	if _, err := h.ensureSessionStarted(runtime, state); err != nil {
		return err
	}

	normalized := unwrapped
	if runtime.inputNormalizer != nil {
		decoded, err := runtime.inputNormalizer.Decode(unwrapped)
		if err != nil {
			return h.emitServerError(peer, state.sessionID, "audio_decode_failed", err.Error())
		}
		normalized = decoded
	}
	if len(normalized) == 0 {
		return nil
	}

	snapshot, err := runtime.session.IngestAudioFrame(normalized)
	if err != nil {
		return h.emitServerError(peer, state.sessionID, "audio_ingest_failed", err.Error())
	}
	if h.profile.ServerEndpointEnabled && state.audioTurnOpen {
		if err := runtime.ensureInputPreview(context.Background(), h.responder, snapshot, ""); err != nil {
			h.logger.Warn("gateway input preview start failed", "session_id", snapshot.SessionID, "error", err)
		} else {
			preview, partialChanged, pushErr := runtime.pushInputPreviewAudio(context.Background(), normalized)
			if pushErr != nil {
				h.logger.Warn("gateway input preview push failed", "session_id", snapshot.SessionID, "error", pushErr)
			} else if partialChanged {
				logTurnTraceInfo(h.logger, "gateway input preview updated", snapshot.SessionID, runtime.turnTrace.Current(),
					"partial_text", preview.PartialText,
					"audio_bytes", preview.AudioBytes,
				)
			}
		}
	}
	return applyReadDeadline(runtime, snapshot, h.runtimeProfile())
}

func (h *xiaozhiWSHandler) ensureSessionStarted(runtime *connectionRuntime, state *xiaozhiCompatState) (session.Snapshot, error) {
	snapshot := runtime.session.Snapshot()
	if snapshot.SessionID != "" {
		return snapshot, nil
	}
	started, err := runtime.session.Start(session.StartRequest{
		RequestedSessionID: state.sessionID,
		DeviceID:           state.deviceID,
		ClientType:         "xiaozhi-compat",
		Mode:               "voice",
		InputCodec:         state.inputCodec,
		InputSampleRate:    state.inputSampleRate,
		InputChannels:      state.inputChannels,
		ClientCanEnd:       true,
		ServerCanEnd:       true,
	})
	if err != nil {
		return session.Snapshot{}, err
	}
	if err := applyReadDeadline(runtime, started, h.runtimeProfile()); err != nil {
		return session.Snapshot{}, err
	}
	return started, nil
}

func (h *xiaozhiWSHandler) emitTurnResponse(ctx context.Context, runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, turn session.CommittedTurn, trace turnTrace, text string) error {
	request := buildTurnRequest(turn, runtime, trace, text)
	textParts := make([]string, 0, 4)
	collectText := func(delta voice.ResponseDelta) {
		if delta.Kind != voice.ResponseDeltaKindText || strings.TrimSpace(delta.Text) == "" {
			return
		}
		textParts = append(textParts, delta.Text)
	}

	var response voice.TurnResponse
	var err error
	if streamingResponder, ok := h.responder.(voice.StreamingResponder); ok {
		response, err = streamingResponder.RespondStream(ctx, request, voice.ResponseDeltaSinkFunc(func(_ context.Context, delta voice.ResponseDelta) error {
			collectText(delta)
			return nil
		}))
	} else {
		response, err = h.responder.Respond(ctx, request)
		if err == nil {
			for _, delta := range responseDeltasForEmission(response, true) {
				collectText(delta)
			}
		}
	}
	if err != nil {
		logTurnTraceError(h.logger, "gateway turn terminated", state.sessionID, trace, err, "error_code", "response_generation_failed")
		return h.emitServerError(peer, state.sessionID, "response_generation_failed", err.Error())
	}
	trace = runtime.turnTrace.MarkResponseStart()
	logTurnTraceInfo(h.logger, "gateway turn response started", state.sessionID, trace,
		"response_start_latency_ms", trace.ResponseStartLatencyMs(),
		"has_audio", response.AudioStream != nil || len(response.AudioChunks) > 0,
	)
	if strings.TrimSpace(text) == "" && strings.TrimSpace(response.InputText) != "" {
		if err := peer.WriteJSON(xiaozhiTextMessage{Type: "stt", SessionID: state.sessionID, Text: response.InputText}); err != nil {
			return err
		}
	}

	return h.finalizeTurnResponse(ctx, runtime, peer, state, trace, response, strings.TrimSpace(strings.Join(textParts, "")))
}

func (h *xiaozhiWSHandler) finalizeTurnResponse(ctx context.Context, runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState, trace turnTrace, response voice.TurnResponse, aggregatedText string) error {
	runtime.session.ClearTurn()
	runtime.clearInputPreview()
	if aggregatedText == "" {
		aggregatedText = responseTextForXiaozhi(response)
	}

	audioStream := response.AudioStream
	if audioStream == nil && len(response.AudioChunks) > 0 {
		audioStream = voice.NewStaticAudioStream(response.AudioChunks)
	}
	if audioStream != nil {
		encoded, err := h.encodeAudioStream(ctx, audioStream)
		if err != nil {
			logTurnTraceError(h.logger, "gateway turn terminated", state.sessionID, trace, err, "error_code", "tts_encode_failed")
			if response.EndSession {
				runtime.session.Reset()
				return runtime.conn.Close()
			}
			active, setErr := runtime.session.SetState(session.StateActive)
			if setErr == nil {
				_ = applyReadDeadline(runtime, active, h.runtimeProfile())
			}
			return h.emitServerError(peer, state.sessionID, "tts_encode_failed", err.Error())
		}
		speaking, err := runtime.session.SetState(session.StateSpeaking)
		if err != nil {
			return err
		}
		trace = runtime.turnTrace.MarkSpeaking()
		logTurnTraceInfo(h.logger, "gateway turn speaking", state.sessionID, trace,
			"speaking_latency_ms", trace.SpeakingLatencyMs(),
		)
		if err := applyReadDeadline(runtime, speaking, h.runtimeProfile()); err != nil {
			return err
		}
		h.startAudioStream(ctx, runtime, peer, state, trace, aggregatedText, encoded, response.EndSession, response.EndReason, response.EndMessage)
		return nil
	}

	if strings.TrimSpace(aggregatedText) != "" {
		if err := peer.WriteJSON(xiaozhiTextMessage{Type: "llm", SessionID: state.sessionID, Text: aggregatedText}); err != nil {
			return err
		}
	}
	if response.EndSession {
		trace = runtime.turnTrace.MarkCompleted()
		logTurnTraceInfo(h.logger, "gateway turn completed", state.sessionID, trace,
			"completed_latency_ms", trace.CompletedLatencyMs(),
			"end_session", true,
			"end_reason", strings.TrimSpace(response.EndReason),
		)
		runtime.turnTrace.Clear()
		runtime.session.Reset()
		return runtime.conn.Close()
	}
	active, err := runtime.session.SetState(session.StateActive)
	if err != nil {
		return err
	}
	trace = runtime.turnTrace.MarkActive()
	trace = runtime.turnTrace.MarkCompleted()
	logTurnTraceInfo(h.logger, "gateway turn completed", state.sessionID, trace,
		"active_return_latency_ms", trace.ActiveReturnLatencyMs(),
		"completed_latency_ms", trace.CompletedLatencyMs(),
		"end_session", false,
	)
	runtime.turnTrace.Clear()
	return applyReadDeadline(runtime, active, h.runtimeProfile())
}

func (h *xiaozhiWSHandler) encodeAudioStream(ctx context.Context, audioStream voice.AudioStream) (voice.AudioStream, error) {
	if strings.ToLower(strings.TrimSpace(h.profile.SourceOutputCodec)) != "pcm16le" {
		return nil, fmt.Errorf("xiaozhi adapter currently requires pcm16le source audio, got %s", h.profile.SourceOutputCodec)
	}
	return h.encoder.EncodePCM16(
		ctx,
		audioStream,
		h.profile.SourceOutputRate,
		h.profile.SourceOutputChannels,
		h.profile.OutputSampleRate,
		h.profile.OutputChannels,
		h.profile.OutputFrameDurationMs,
	)
}

func (h *xiaozhiWSHandler) startAudioStream(
	ctx context.Context,
	runtime *connectionRuntime,
	peer *xiaozhiJSONPeer,
	state *xiaozhiCompatState,
	trace turnTrace,
	text string,
	audioStream voice.AudioStream,
	endSession bool,
	endReason string,
	endMessage string,
) {
	streamCtx, cancel := context.WithCancel(ctx)
	stream := runtime.installOutput(cancel)
	frameDuration := time.Duration(maxInt(h.profile.OutputFrameDurationMs, 60)) * time.Millisecond

	go func() {
		defer close(stream.done)
		defer runtime.clearOutput(stream)
		defer func() { _ = audioStream.Close() }()

		if err := peer.WriteJSON(xiaozhiTTSMessage{Type: "tts", State: "start", SessionID: state.sessionID}); err != nil {
			return
		}
		if strings.TrimSpace(text) != "" {
			if err := peer.WriteJSON(xiaozhiTTSMessage{Type: "tts", State: "sentence_start", SessionID: state.sessionID, Text: text}); err != nil {
				return
			}
		}

		ticker := time.NewTicker(frameDuration)
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
				logTurnTraceError(h.logger, "gateway turn audio stream failed", state.sessionID, runtime.turnTrace.Current(), err)
				_ = h.emitServerError(peer, state.sessionID, "audio_stream_failed", err.Error())
				return
			}
			if len(chunk) == 0 {
				continue
			}
			if err := peer.WriteCompatBinary(chunk, state.binaryProtocolVersion); err != nil {
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

		_ = peer.WriteJSON(xiaozhiTTSMessage{Type: "tts", State: "stop", SessionID: state.sessionID})
		if endSession {
			trace = runtime.turnTrace.MarkCompleted()
			logTurnTraceInfo(h.logger, "gateway turn completed", state.sessionID, trace,
				"completed_latency_ms", trace.CompletedLatencyMs(),
				"end_session", true,
				"end_reason", strings.TrimSpace(endReason),
			)
			runtime.turnTrace.Clear()
			runtime.session.Reset()
			_ = runtime.conn.Close()
			return
		}
		active, err := runtime.session.SetState(session.StateActive)
		if err != nil {
			return
		}
		trace = runtime.turnTrace.MarkActive()
		trace = runtime.turnTrace.MarkCompleted()
		logTurnTraceInfo(h.logger, "gateway turn completed", state.sessionID, trace,
			"active_return_latency_ms", trace.ActiveReturnLatencyMs(),
			"completed_latency_ms", trace.CompletedLatencyMs(),
			"end_session", false,
		)
		runtime.turnTrace.Clear()
		_ = applyReadDeadline(runtime, active, h.runtimeProfile())
	}()
}

func (h *xiaozhiWSHandler) interruptSpeaking(runtime *connectionRuntime, peer *xiaozhiJSONPeer, state *xiaozhiCompatState) error {
	snapshot := runtime.session.Snapshot()
	if snapshot.State != session.StateSpeaking && runtime.output == nil {
		return nil
	}
	interrupted := runtime.interruptOutput(100 * time.Millisecond)
	if interrupted {
		if err := peer.WriteJSON(xiaozhiTTSMessage{Type: "tts", State: "stop", SessionID: state.sessionID}); err != nil {
			return err
		}
	}
	if snapshot.SessionID == "" || snapshot.State != session.StateSpeaking {
		return nil
	}
	active, err := runtime.session.SetState(session.StateActive)
	if err != nil {
		return err
	}
	trace := runtime.turnTrace.MarkInterrupted()
	trace = runtime.turnTrace.MarkActive()
	logTurnTraceInfo(h.logger, "gateway turn interrupted", active.SessionID, trace,
		"active_return_latency_ms", trace.ActiveReturnLatencyMs(),
	)
	runtime.clearInputPreview()
	return applyReadDeadline(runtime, active, h.runtimeProfile())
}

func (h *xiaozhiWSHandler) emitServerError(peer *xiaozhiJSONPeer, sessionID, code, message string) error {
	rendered := strings.TrimSpace(message)
	if rendered == "" {
		rendered = code
	}
	return peer.WriteJSON(xiaozhiServerMessage{Type: "server", Status: "error", Message: rendered, SessionID: sessionID})
}

func (h *xiaozhiWSHandler) runtimeProfile() RealtimeProfile {
	return RealtimeProfile{
		IdleTimeoutMs: h.profile.IdleTimeoutMs,
		MaxSessionMs:  h.profile.MaxSessionMs,
	}
}

func responseTextForXiaozhi(response voice.TurnResponse) string {
	parts := make([]string, 0, len(response.Deltas)+1)
	for _, delta := range responseDeltasForEmission(response, true) {
		if delta.Kind != voice.ResponseDeltaKindText {
			continue
		}
		if strings.TrimSpace(delta.Text) == "" {
			continue
		}
		parts = append(parts, delta.Text)
	}
	if len(parts) == 0 && strings.TrimSpace(response.Text) != "" {
		parts = append(parts, response.Text)
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstNonEmptyInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
