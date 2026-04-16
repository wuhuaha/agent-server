package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agent-server/internal/session"
	"agent-server/internal/voice"

	"github.com/gorilla/websocket"
)

func TestRealtimeDiscoveryIncludesWireProfile(t *testing.T) {
	handler := NewRealtimeHandler(RealtimeProfile{
		WSPath:                         "/v1/realtime/ws",
		ProtocolVersion:                "rtos-ws-v0",
		Subprotocol:                    "agent-server.realtime.v0",
		LLMProvider:                    "deepseek_chat",
		ServerEndpointAvailable:        true,
		ServerEndpointEnabled:          true,
		ServerEndpointMinAudioMs:       320,
		ServerEndpointSilenceMs:        480,
		ServerEndpointLexicalMode:      "conservative",
		ServerEndpointIncompleteHoldMs: 720,
		ServerEndpointHintSilenceMs:    160,
		AuthMode:                       "disabled",
		TurnMode:                       "client_wakeup_client_commit",
		IdleTimeoutMs:                  15000,
		MaxSessionMs:                   300000,
		MaxFrameBytes:                  4096,
		InputCodec:                     "pcm16le",
		InputSampleRate:                16000,
		InputChannels:                  1,
		OutputCodec:                    "pcm16le",
		OutputSampleRate:               16000,
		OutputChannels:                 1,
		AllowOpus:                      true,
		AllowTextInput:                 true,
		AllowImageInput:                false,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got := body["protocol_version"]; got != "rtos-ws-v0" {
		t.Fatalf("expected protocol_version rtos-ws-v0, got %v", got)
	}
	if got := body["ws_path"]; got != "/v1/realtime/ws" {
		t.Fatalf("expected ws_path /v1/realtime/ws, got %v", got)
	}
	if got := body["llm_provider"]; got != "deepseek_chat" {
		t.Fatalf("expected llm_provider deepseek_chat, got %v", got)
	}
	if got := body["turn_mode"]; got != "client_wakeup_client_commit" {
		t.Fatalf("expected turn_mode client_wakeup_client_commit, got %v", got)
	}
	serverEndpoint, ok := body["server_endpoint"].(map[string]any)
	if !ok {
		t.Fatalf("expected server_endpoint object, got %#v", body["server_endpoint"])
	}
	if got := serverEndpoint["mode"]; got != "server_vad_assisted" {
		t.Fatalf("expected server_endpoint mode server_vad_assisted, got %v", got)
	}
	if got := serverEndpoint["main_path_candidate"]; got != true {
		t.Fatalf("expected server endpoint candidate flag true, got %v", got)
	}
	voiceCollaboration, ok := body["voice_collaboration"].(map[string]any)
	if !ok {
		t.Fatalf("expected voice_collaboration object, got %#v", body["voice_collaboration"])
	}
	previewEvents, ok := voiceCollaboration["preview_events"].(map[string]any)
	if !ok {
		t.Fatalf("expected preview_events object, got %#v", voiceCollaboration["preview_events"])
	}
	if got := previewEvents["enabled"]; got != true {
		t.Fatalf("expected preview_events enabled true, got %v", got)
	}
	if got := previewEvents["mode"]; got != "preview_v1" {
		t.Fatalf("expected preview_events mode preview_v1, got %v", got)
	}
	playbackAck, ok := voiceCollaboration["playback_ack"].(map[string]any)
	if !ok {
		t.Fatalf("expected playback_ack object, got %#v", voiceCollaboration["playback_ack"])
	}
	if got := playbackAck["enabled"]; got != true {
		t.Fatalf("expected playback_ack enabled true, got %v", got)
	}
	if got := playbackAck["mode"]; got != "segment_mark_v1" {
		t.Fatalf("expected playback_ack mode segment_mark_v1, got %v", got)
	}
	notes, ok := body["notes"].([]any)
	if !ok || len(notes) == 0 {
		t.Fatalf("expected discovery notes, got %#v", body["notes"])
	}
	foundCandidateNote := false
	for _, item := range notes {
		if item == "audio turns may auto-commit through the shared server-endpoint candidate after preview silence; explicit audio.in.commit stays supported" {
			foundCandidateNote = true
			break
		}
	}
	if !foundCandidateNote {
		t.Fatalf("expected discovery notes to mention the server-endpoint candidate, got %#v", notes)
	}
}

func TestInputPreviewTraceTracksPreviewMilestones(t *testing.T) {
	var state inputPreviewTraceState
	startedAt := time.Unix(1700000000, 0).UTC()

	trace := state.ObserveAudio("sess_trace", 640, startedAt)
	if trace.PreviewID == "" {
		t.Fatal("expected preview trace id after first audio")
	}
	if trace.AudioBytes != 640 {
		t.Fatalf("expected preview audio bytes 640, got %d", trace.AudioBytes)
	}

	trace, firstPartial, speechStarted, endpointCandidate, commitSuggested := state.ObservePreview("sess_trace", voice.InputPreview{
		PartialText:   "打开客厅灯",
		AudioBytes:    1280,
		SpeechStarted: true,
	}, startedAt.Add(120*time.Millisecond))
	if !firstPartial {
		t.Fatal("expected first partial observation to be recorded")
	}
	if !speechStarted {
		t.Fatal("expected speech start observation to be recorded")
	}
	if endpointCandidate {
		t.Fatal("did not expect endpoint candidate on first partial")
	}
	if commitSuggested {
		t.Fatal("did not expect commit suggestion on first partial")
	}
	if got := trace.SpeechStartLatencyMs(); got != 120 {
		t.Fatalf("expected speech start latency 120ms, got %d", got)
	}
	if got := trace.FirstPartialLatencyMs(); got != 120 {
		t.Fatalf("expected first partial latency 120ms, got %d", got)
	}

	trace, firstPartial, speechStarted, endpointCandidate, commitSuggested = state.ObservePreview("sess_trace", voice.InputPreview{
		PartialText:     "打开客厅灯",
		AudioBytes:      1920,
		SpeechStarted:   true,
		CommitSuggested: true,
		EndpointReason:  "preview_tail_silence",
	}, startedAt.Add(420*time.Millisecond))
	if firstPartial {
		t.Fatal("expected first partial to stay one-shot")
	}
	if speechStarted {
		t.Fatal("expected speech start observation to stay one-shot")
	}
	if !endpointCandidate {
		t.Fatal("expected endpoint candidate observation to be recorded")
	}
	if !commitSuggested {
		t.Fatal("expected commit suggestion observation to be recorded")
	}
	if got := trace.EndpointCandidateLatencyMs(); got != 420 {
		t.Fatalf("expected endpoint candidate latency 420ms, got %d", got)
	}
	if got := trace.CommitSuggestedLatencyMs(); got != 420 {
		t.Fatalf("expected commit latency 420ms, got %d", got)
	}
	if trace.EndpointReason != "preview_tail_silence" {
		t.Fatalf("expected endpoint reason to persist, got %q", trace.EndpointReason)
	}

	cleared := state.Clear()
	if cleared.PreviewID == "" {
		t.Fatal("expected clear to return the previous trace")
	}
	if state.Current().PreviewID != "" {
		t.Fatal("expected preview trace state to reset after clear")
	}
}

func TestTurnTraceTracksFirstOutputMilestones(t *testing.T) {
	var state turnTraceState
	trace := state.Begin("sess_turn", "audio_commit")

	markedText, firstTextRecorded := state.MarkFirstTextDelta()
	if !firstTextRecorded {
		t.Fatal("expected first text delta milestone to be recorded")
	}
	if markedText.FirstTextDeltaAt.IsZero() {
		t.Fatal("expected first text delta timestamp to be set")
	}
	if _, firstTextRecorded = state.MarkFirstTextDelta(); firstTextRecorded {
		t.Fatal("expected first text delta milestone to be idempotent")
	}

	markedAudio, firstAudioRecorded := state.MarkFirstAudioChunk()
	if !firstAudioRecorded {
		t.Fatal("expected first audio chunk milestone to be recorded")
	}
	if markedAudio.FirstAudioChunkAt.IsZero() {
		t.Fatal("expected first audio chunk timestamp to be set")
	}
	if _, firstAudioRecorded = state.MarkFirstAudioChunk(); firstAudioRecorded {
		t.Fatal("expected first audio chunk milestone to be idempotent")
	}
	if markedAudio.FirstAudioChunkLatencyMs() < 0 || markedText.FirstTextDeltaLatencyMs() < 0 || trace.ResponseStartLatencyMs() < 0 {
		t.Fatal("expected milestone latencies to stay non-negative")
	}
}

func TestBuildTurnRequestCarriesPreviousPlaybackOutcomeMetadata(t *testing.T) {
	orchestrator := voice.NewSessionOrchestrator(nil)
	delivered := "好的，已经为你打开客厅灯，现在把亮度调到了最舒适的模式。"
	orchestrator.PrepareTurn(voice.TurnRequest{
		SessionID:  "sess-prev",
		TurnID:     "turn-prev",
		DeviceID:   "dev-prev",
		ClientType: "rtos",
	}, "打开客厅灯并调亮一点", delivered)
	orchestrator.StartPlaybackWithOptions(delivered, 200*time.Millisecond, 2*time.Second, voice.PlaybackStartOptions{
		PreferClientFacts: true,
	})
	orchestrator.ObservePlaybackStartedFact()
	orchestrator.ObservePlaybackMarkFact(600 * time.Millisecond)
	summary := orchestrator.InterruptPlaybackWithPolicy(voice.InterruptionPolicyHardInterrupt, "client_barge_in_after_mark")

	request := buildTurnRequest(session.CommittedTurn{
		Snapshot: session.Snapshot{
			SessionID:       "sess-next",
			DeviceID:        "dev-prev",
			ClientType:      "rtos",
			InputCodec:      "pcm16le",
			InputSampleRate: 16000,
			InputChannels:   1,
			AudioBytes:      3200,
			InputFrames:     5,
		},
		AudioPCM: []byte{1, 2, 3},
	}, &connectionRuntime{voiceSession: orchestrator}, turnTrace{
		TurnID:  "turn-next",
		TraceID: "trace-next",
	}, "继续", nil)

	if got := request.Metadata["voice.previous.available"]; got != "true" {
		t.Fatalf("expected previous playback context on request, got %+v", request.Metadata)
	}
	if got := request.Metadata["voice.previous.heard_text"]; got != summary.HeardText {
		t.Fatalf("expected previous heard text %q, got %q", summary.HeardText, got)
	}
	if got := request.Metadata["voice.previous.resume_anchor"]; got != summary.HeardText {
		t.Fatalf("expected previous resume anchor %q, got %q", summary.HeardText, got)
	}
	if got := request.Metadata["voice.previous.missed_text"]; got == "" {
		t.Fatalf("expected previous missed text on request, got %+v", request.Metadata)
	}
	if got := request.Metadata["voice.previous.interruption_policy"]; got != string(voice.InterruptionPolicyHardInterrupt) {
		t.Fatalf("expected previous interruption policy %q, got %q", voice.InterruptionPolicyHardInterrupt, got)
	}
}

func TestAppendWebsocketErrorLogAttrsIncludesCloseMetadata(t *testing.T) {
	attrs := appendWebsocketErrorLogAttrs(nil, &websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "bye"})
	rendered := map[string]any{}
	for i := 0; i+1 < len(attrs); i += 2 {
		key, _ := attrs[i].(string)
		rendered[key] = attrs[i+1]
	}
	if rendered["ws_close_code"] != websocket.CloseNormalClosure {
		t.Fatalf("expected ws_close_code %d, got %#v", websocket.CloseNormalClosure, rendered["ws_close_code"])
	}
	if rendered["ws_close_text"] != "bye" {
		t.Fatalf("expected ws_close_text bye, got %#v", rendered["ws_close_text"])
	}
}

func TestValidatePlaybackAckIdentityRequiresActiveMeta(t *testing.T) {
	if validatePlaybackAckIdentity(audioPlaybackMeta{}, "resp_1", "playback_1") {
		t.Fatal("expected empty playback meta to reject ack identity")
	}
	if !validatePlaybackAckIdentity(audioPlaybackMeta{ResponseID: "resp_1", PlaybackID: "playback_1"}, "resp_1", "playback_1") {
		t.Fatal("expected matching playback meta to accept ack identity")
	}
	if validatePlaybackAckIdentity(audioPlaybackMeta{ResponseID: "resp_1", PlaybackID: "playback_1"}, "resp_2", "playback_1") {
		t.Fatal("expected mismatched response id to reject ack identity")
	}
}

func TestIsExpectedWebsocketClosureRecognizesNormalClose(t *testing.T) {
	if !isExpectedWebsocketClosure(&websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "done"}) {
		t.Fatal("expected normal close to be treated as expected")
	}
	if isExpectedWebsocketClosure(errors.New("broken pipe")) {
		t.Fatal("expected broken pipe to stay unexpected")
	}
}
