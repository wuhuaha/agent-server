package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"agent-server/internal/agent"
)

type fakeTranscriber struct {
	result TranscriptionResult
	err    error
}

func (f fakeTranscriber) Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error) {
	if f.err != nil {
		return TranscriptionResult{}, f.err
	}
	return f.result, nil
}

type recordingHintTranscriber struct {
	result    TranscriptionResult
	lastReq   TranscriptionRequest
	callCount int
}

func (t *recordingHintTranscriber) Transcribe(_ context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	t.lastReq = req
	t.callCount++
	return t.result, nil
}

type fakeStreamingTranscriber struct {
	result TranscriptionResult
	err    error
	chunks [][]byte
}

type recordingPreviewHintTranscriber struct {
	lastReq TranscriptionRequest
}

func (t *recordingPreviewHintTranscriber) Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error) {
	return TranscriptionResult{}, nil
}

func (t *recordingPreviewHintTranscriber) StartStream(_ context.Context, req TranscriptionRequest, _ TranscriptionDeltaSink) (StreamingTranscriptionSession, error) {
	t.lastReq = req
	return &fakeStreamingTranscriptionSession{parent: &fakeStreamingTranscriber{}}, nil
}

type countingTranscriber struct {
	result TranscriptionResult
	calls  int
}

func (f *countingTranscriber) Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error) {
	f.calls++
	return f.result, nil
}

func (f *fakeStreamingTranscriber) Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error) {
	if f.err != nil {
		return TranscriptionResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeStreamingTranscriber) StartStream(_ context.Context, req TranscriptionRequest, _ TranscriptionDeltaSink) (StreamingTranscriptionSession, error) {
	return &fakeStreamingTranscriptionSession{
		parent: f,
		req:    req,
	}, nil
}

type fakeStreamingTranscriptionSession struct {
	parent *fakeStreamingTranscriber
	req    TranscriptionRequest
	buffer bytes.Buffer
}

func (s *fakeStreamingTranscriptionSession) PushAudio(_ context.Context, chunk []byte) error {
	s.parent.chunks = append(s.parent.chunks, append([]byte(nil), chunk...))
	_, err := s.buffer.Write(chunk)
	return err
}

func (s *fakeStreamingTranscriptionSession) Finish(context.Context) (TranscriptionResult, error) {
	if s.parent.err != nil {
		return TranscriptionResult{}, s.parent.err
	}
	s.req.AudioPCM = append([]byte(nil), s.buffer.Bytes()...)
	return s.parent.result, nil
}

func (s *fakeStreamingTranscriptionSession) Close() error {
	return nil
}

type previewStreamingTranscriber struct {
	text string
}

func (previewStreamingTranscriber) Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error) {
	return TranscriptionResult{}, nil
}

func (p previewStreamingTranscriber) StartStream(_ context.Context, _ TranscriptionRequest, sink TranscriptionDeltaSink) (StreamingTranscriptionSession, error) {
	return &previewStreamingSession{sink: sink, partialText: firstNonEmpty(p.text, "打开客厅灯")}, nil
}

type previewStreamingSession struct {
	sink        TranscriptionDeltaSink
	started     bool
	partialText string
}

func (s *previewStreamingSession) PushAudio(ctx context.Context, chunk []byte) error {
	if !s.started {
		s.started = true
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{Kind: TranscriptionDeltaKindSpeechStart}); err != nil {
			return err
		}
	}
	return emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{Kind: TranscriptionDeltaKindPartial, Text: s.partialText})
}

func (s *previewStreamingSession) Finish(context.Context) (TranscriptionResult, error) {
	return TranscriptionResult{}, nil
}

func (s *previewStreamingSession) Close() error {
	return nil
}

type stagedPreviewStreamingTranscriber struct {
	session *stagedPreviewStreamingSession
}

func (*stagedPreviewStreamingTranscriber) Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error) {
	return TranscriptionResult{}, nil
}

func (t *stagedPreviewStreamingTranscriber) StartStream(_ context.Context, _ TranscriptionRequest, sink TranscriptionDeltaSink) (StreamingTranscriptionSession, error) {
	t.session = &stagedPreviewStreamingSession{sink: sink}
	return t.session, nil
}

type stagedPreviewStreamingSession struct {
	sink      TranscriptionDeltaSink
	started   bool
	pushCount int
}

func (s *stagedPreviewStreamingSession) PushAudio(ctx context.Context, _ []byte) error {
	s.pushCount++
	if !s.started {
		s.started = true
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{Kind: TranscriptionDeltaKindSpeechStart}); err != nil {
			return err
		}
	}
	text := "打开客厅灯然后"
	if s.pushCount == 1 {
		text = "打开客厅灯"
	}
	return emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{Kind: TranscriptionDeltaKindPartial, Text: text})
}

func (*stagedPreviewStreamingSession) Finish(context.Context) (TranscriptionResult, error) {
	return TranscriptionResult{}, nil
}

func (*stagedPreviewStreamingSession) Close() error {
	return nil
}

type capturingTurnExecutor struct {
	inputs []agent.TurnInput
}

func (e *capturingTurnExecutor) ExecuteTurn(_ context.Context, input agent.TurnInput) (agent.TurnOutput, error) {
	e.inputs = append(e.inputs, input)
	return agent.TurnOutput{Text: "handled: " + input.UserText}, nil
}

type capturingPrewarmExecutor struct {
	calls chan agent.TurnInput
}

func (e *capturingPrewarmExecutor) ExecuteTurn(_ context.Context, input agent.TurnInput) (agent.TurnOutput, error) {
	return agent.TurnOutput{Text: "handled: " + input.UserText}, nil
}

func (e *capturingPrewarmExecutor) PrewarmTurn(_ context.Context, input agent.TurnInput) {
	if e.calls != nil {
		e.calls <- input
	}
}

type fixedSemanticJudge struct {
	result SemanticTurnJudgement
}

func (j fixedSemanticJudge) JudgePreview(context.Context, SemanticTurnRequest) (SemanticTurnJudgement, error) {
	return j.result, nil
}

type fixedSemanticSlotParser struct {
	result SemanticSlotParseResult
}

func (p fixedSemanticSlotParser) ParsePreview(context.Context, SemanticSlotParseRequest) (SemanticSlotParseResult, error) {
	return p.result, nil
}

func TestASRResponderForAudio(t *testing.T) {
	responder := NewASRResponder(
		fakeTranscriber{result: TranscriptionResult{Text: "hello from local asr"}},
		"auto",
		"pcm16le",
		16000,
		1,
		true,
	)

	response, err := responder.Respond(context.Background(), TurnRequest{
		SessionID:       "sess_test",
		DeviceID:        "rtos-001",
		AudioPCM:        make([]byte, 3200),
		AudioBytes:      3200,
		InputFrames:     5,
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	})
	if err != nil {
		t.Fatalf("respond failed: %v", err)
	}
	if !strings.Contains(response.Text, "hello from local asr") {
		t.Fatalf("expected transcript in response, got %q", response.Text)
	}
	if response.InputText != "hello from local asr" {
		t.Fatalf("expected input text to carry transcript, got %q", response.InputText)
	}
	if len(response.AudioChunks) != 2 {
		t.Fatalf("expected placeholder audio, got %d chunks", len(response.AudioChunks))
	}
	if len(response.Deltas) != 1 || response.Deltas[0].Kind != ResponseDeltaKindText {
		t.Fatalf("expected one text delta, got %+v", response.Deltas)
	}
}

func TestASRResponderReturnsErrorWhenTranscriberFails(t *testing.T) {
	responder := NewASRResponder(
		fakeTranscriber{err: errors.New("transcriber unavailable")},
		"auto",
		"pcm16le",
		16000,
		1,
		false,
	)

	_, err := responder.Respond(context.Background(), TurnRequest{
		SessionID:       "sess_test",
		DeviceID:        "rtos-001",
		AudioPCM:        make([]byte, 3200),
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	})
	if err == nil || !strings.Contains(err.Error(), "transcriber unavailable") {
		t.Fatalf("expected transcriber error, got %v", err)
	}
}

func TestASRResponderInjectsStructuredSpeechMetadataIntoTurnInput(t *testing.T) {
	executor := &capturingTurnExecutor{}
	responder := NewASRResponder(
		fakeTranscriber{result: TranscriptionResult{
			Text:           "打开客厅灯。",
			Segments:       []string{"打开客厅灯。"},
			DurationMs:     1500,
			ElapsedMs:      320,
			Model:          "SenseVoiceSmall",
			Device:         "cpu",
			Language:       "zh",
			Emotion:        "calm",
			SpeakerID:      "speaker-a",
			AudioEvents:    []string{"speech"},
			EndpointReason: "silence_timeout",
			Partials:       []string{"打开", "打开客厅灯"},
			Mode:           "batch",
		}},
		"auto",
		"pcm16le",
		16000,
		1,
		false,
	).WithTurnExecutor(executor)

	response, err := responder.Respond(context.Background(), TurnRequest{
		SessionID:       "sess_test",
		DeviceID:        "rtos-001",
		ClientType:      "xiaozhi-compat",
		AudioPCM:        make([]byte, 3200),
		AudioBytes:      3200,
		InputFrames:     5,
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
		Metadata: map[string]string{
			"source": "mic",
		},
	})
	if err != nil {
		t.Fatalf("respond failed: %v", err)
	}
	if response.Text != "handled: 打开客厅灯。" {
		t.Fatalf("unexpected response text %q", response.Text)
	}
	if response.InputText != "打开客厅灯。" {
		t.Fatalf("expected input text to echo transcript, got %q", response.InputText)
	}
	if len(executor.inputs) != 1 {
		t.Fatalf("expected one executor input, got %d", len(executor.inputs))
	}
	input := executor.inputs[0]
	if got := input.Metadata["source"]; got != "mic" {
		t.Fatalf("expected base metadata to survive, got %q", got)
	}
	if got := input.Metadata["speech.language"]; got != "zh" {
		t.Fatalf("unexpected speech language %q", got)
	}
	if got := input.Metadata["speech.emotion"]; got != "calm" {
		t.Fatalf("unexpected speech emotion %q", got)
	}
	if got := input.Metadata["speech.speaker_id"]; got != "speaker-a" {
		t.Fatalf("unexpected speaker id %q", got)
	}
	if got := input.Metadata["speech.endpoint_reason"]; got != "silence_timeout" {
		t.Fatalf("unexpected endpoint reason %q", got)
	}
	if got := input.Metadata["speech.duration_ms"]; got != "1500" {
		t.Fatalf("unexpected duration metadata %q", got)
	}
	if got := input.Metadata["speech.elapsed_ms"]; got != "320" {
		t.Fatalf("unexpected elapsed metadata %q", got)
	}
	if got := input.Metadata["speech.transcriber_mode"]; got == "" {
		t.Fatal("expected speech.transcriber_mode metadata")
	}
	if got := input.Metadata["speech.text_terminal_punctuation"]; got != "strong_stop" {
		t.Fatalf("unexpected terminal punctuation metadata %q", got)
	}
	if got := input.Metadata["speech.text_clause_count"]; got != "1" {
		t.Fatalf("unexpected text clause count metadata %q", got)
	}
	if got := input.Metadata["speech.partial_count"]; got != "2" {
		t.Fatalf("unexpected partial count metadata %q", got)
	}
	var partials []string
	if err := json.Unmarshal([]byte(input.Metadata["speech.partials"]), &partials); err != nil {
		t.Fatalf("partials should be encoded json: %v", err)
	}
	if len(partials) != 2 || partials[1] != "打开客厅灯" {
		t.Fatalf("unexpected partials %+v", partials)
	}
}

func TestASRInputPreviewSessionCanFinishIntoFinalTranscription(t *testing.T) {
	streaming := &fakeStreamingTranscriber{
		result: TranscriptionResult{
			Text:           "打开客厅灯",
			EndpointReason: "server_silence_timeout",
			Mode:           "stream_preview_batch",
		},
	}
	responder := NewASRResponder(streaming, "auto", "pcm16le", 16000, 1, false)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_preview_finish",
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 3200)); err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}

	finalizer, ok := preview.(FinalizingInputPreviewSession)
	if !ok {
		t.Fatalf("expected preview session to support finalization, got %T", preview)
	}
	result, err := finalizer.Finish(context.Background())
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}
	if got := result.Text; got != "打开客厅灯" {
		t.Fatalf("expected finalized text, got %q", got)
	}
}

func TestASRResponderPrefersPreviewTranscriptionFastPath(t *testing.T) {
	transcriber := &countingTranscriber{result: TranscriptionResult{Text: "慢路径文本"}}
	executor := &capturingTurnExecutor{}
	responder := NewASRResponder(transcriber, "auto", "pcm16le", 16000, 1, false).WithTurnExecutor(executor)

	response, err := responder.Respond(context.Background(), TurnRequest{
		SessionID:       "sess_fast_preview",
		DeviceID:        "rtos-001",
		ClientType:      "rtos",
		AudioPCM:        make([]byte, 3200),
		AudioBytes:      3200,
		InputFrames:     5,
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
		PreviewTranscription: &TranscriptionResult{
			Text:           "快路径文本",
			EndpointReason: "server_endpoint_preview_finish",
			Mode:           "preview_finalize_fast_path",
		},
	})
	if err != nil {
		t.Fatalf("Respond failed: %v", err)
	}
	if got := response.InputText; got != "快路径文本" {
		t.Fatalf("expected preview fast-path text, got %q", got)
	}
	if transcriber.calls != 0 {
		t.Fatalf("expected transcriber to be skipped, got %d calls", transcriber.calls)
	}
	if len(executor.inputs) != 1 {
		t.Fatalf("expected one executor input, got %d", len(executor.inputs))
	}
	if got := executor.inputs[0].UserText; got != "快路径文本" {
		t.Fatalf("expected executor to receive fast-path text, got %q", got)
	}
	if got := executor.inputs[0].Metadata["speech.transcriber_mode"]; got != "preview_finalize_fast_path" {
		t.Fatalf("expected preview mode metadata, got %q", got)
	}
}

func TestASRResponderPreviewSessionTriggersPrewarmOnStableCompletePrefix(t *testing.T) {
	executor := &capturingPrewarmExecutor{calls: make(chan agent.TurnInput, 1)}
	responder := NewASRResponder(previewStreamingTranscriber{text: "明天周几"}, "auto", "pcm16le", 16000, 1, false).
		WithTurnExecutor(executor)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_prewarm",
		DeviceID:     "rtos-001",
		ClientType:   "rtos",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 3200)); err != nil {
		t.Fatalf("first PushAudio failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 3200)); err != nil {
		t.Fatalf("second PushAudio failed: %v", err)
	}

	select {
	case input := <-executor.calls:
		if got := input.UserText; got != "明天周几" {
			t.Fatalf("expected prewarm text 明天周几, got %q", got)
		}
		if got := input.Metadata["voice.preview.stable_prefix"]; got != "明天周几" {
			t.Fatalf("expected stable prefix metadata, got %q", got)
		}
		if got := input.Metadata["voice.preview.utterance_complete"]; got != "true" {
			t.Fatalf("expected utterance_complete metadata, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected preview prewarm call")
	}
}

func TestASRResponderPreviewSessionPrewarmsMatureStablePrefixBeforeUtteranceComplete(t *testing.T) {
	executor := &capturingPrewarmExecutor{calls: make(chan agent.TurnInput, 1)}
	transcriber := &stagedPreviewStreamingTranscriber{}
	responder := NewASRResponder(transcriber, "auto", "pcm16le", 16000, 1, false).
		WithTurnExecutor(executor)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_prewarm_progressive",
		DeviceID:     "rtos-001",
		ClientType:   "rtos",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 12800)); err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}
	if transcriber.session == nil || transcriber.session.pushCount < 2 {
		t.Fatalf("expected one ingress frame to be split into multiple preview pushes, got %+v", transcriber.session)
	}

	snapshot := preview.Poll(time.Now().Add((defaultPrewarmStableForMs + 80) * time.Millisecond))
	if snapshot.UtteranceComplete {
		t.Fatalf("expected live partial to stay incomplete, got %+v", snapshot)
	}
	if !snapshot.Arbitration.PrewarmAllowed {
		t.Fatalf("expected mature stable prefix to allow prewarm, got %+v", snapshot.Arbitration)
	}

	select {
	case input := <-executor.calls:
		if got := input.UserText; got != "打开客厅灯" {
			t.Fatalf("expected prewarm text 打开客厅灯, got %q", got)
		}
		if got := input.Metadata["voice.preview.utterance_complete"]; got != "false" {
			t.Fatalf("expected incomplete live preview metadata, got %q", got)
		}
		if got := input.Metadata["voice.preview.turn_stage"]; got != string(TurnArbitrationStagePrewarmAllowed) {
			t.Fatalf("expected prewarm stage metadata, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected mature stable prefix prewarm call")
	}
}

func TestASRResponderPreviewSessionPollIncludesSemanticJudgeResult(t *testing.T) {
	responder := NewASRResponder(previewStreamingTranscriber{text: "明白"}, "auto", "pcm16le", 16000, 1, false).
		WithSemanticJudge(fixedSemanticJudge{
			result: SemanticTurnJudgement{
				UtteranceStatus:    SemanticUtteranceIncomplete,
				InterruptionIntent: SemanticIntentBackchannel,
				Confidence:         0.92,
				Reason:             "short_ack",
				Source:             "test",
			},
		}, 80*time.Millisecond, 2, 0)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_semantic_preview",
		DeviceID:     "rtos-001",
		ClientType:   "rtos",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 1280)); err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		snapshot := preview.Poll(time.Now())
		if snapshot.Arbitration.SemanticReady {
			if snapshot.Arbitration.SemanticIntent != SemanticIntentBackchannel {
				t.Fatalf("expected semantic backchannel intent, got %+v", snapshot.Arbitration)
			}
			if snapshot.Arbitration.SemanticConfidence < 0.9 {
				t.Fatalf("expected semantic confidence to propagate, got %+v", snapshot.Arbitration)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected preview poll to include semantic judgement result")
}

func TestASRResponderPreviewSessionPollIncludesSlotParserResult(t *testing.T) {
	responder := NewASRResponder(previewStreamingTranscriber{text: "把灯调亮一点"}, "auto", "pcm16le", 16000, 1, false).
		WithSlotParser(fixedSemanticSlotParser{
			result: SemanticSlotParseResult{
				Domain:        SemanticSlotDomainSmartHome,
				Intent:        "set_attribute",
				SlotStatus:    SemanticSlotStatusPartial,
				Actionability: SemanticSlotActionabilityClarifyNeeded,
				ClarifyNeeded: true,
				MissingSlots:  []string{"target"},
				Confidence:    0.9,
				Reason:        "missing_target_need_clarify",
				Source:        "test",
			},
		}, 80*time.Millisecond, 4, 0)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_slot_preview",
		DeviceID:     "rtos-001",
		ClientType:   "rtos",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 1280)); err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		snapshot := preview.Poll(time.Now())
		if snapshot.Arbitration.SlotReady {
			if snapshot.Arbitration.SlotDomain != SemanticSlotDomainSmartHome {
				t.Fatalf("expected smart_home slot domain, got %+v", snapshot.Arbitration)
			}
			if !snapshot.Arbitration.SlotClarifyNeeded {
				t.Fatalf("expected clarify-needed slot parse, got %+v", snapshot.Arbitration)
			}
			if snapshot.Arbitration.Stage != TurnArbitrationStageDraftAllowed {
				t.Fatalf("expected slot parse to promote draft_allowed, got %+v", snapshot.Arbitration)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected preview poll to include slot parser result")
}

func TestASRResponderPreviewSessionPollIncludesGroundedSlotSummary(t *testing.T) {
	responder := NewASRResponder(previewStreamingTranscriber{text: "打开客厅灯"}, "auto", "pcm16le", 16000, 1, false).
		WithSlotParser(NewGroundedSemanticSlotParser(fixedSemanticSlotParser{
			result: SemanticSlotParseResult{
				Domain:        SemanticSlotDomainSmartHome,
				Intent:        "device_control",
				SlotStatus:    SemanticSlotStatusPartial,
				Actionability: SemanticSlotActionabilityClarifyNeeded,
				ClarifyNeeded: true,
				MissingSlots:  []string{"target"},
				Confidence:    0.9,
				Reason:        "missing_target_need_clarify",
				Source:        "test",
			},
		}, NewDefaultEntityCatalogGrounder()), 80*time.Millisecond, 4, 0)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_slot_grounded_preview",
		DeviceID:     "rtos-001",
		ClientType:   "rtos",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 1280)); err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		snapshot := preview.Poll(time.Now())
		if snapshot.Arbitration.SlotGrounded {
			if snapshot.Arbitration.SlotCanonicalTarget != "客厅灯" {
				t.Fatalf("expected canonical target 客厅灯, got %+v", snapshot.Arbitration)
			}
			if snapshot.Arbitration.SlotCanonicalLocation != "客厅" {
				t.Fatalf("expected canonical location 客厅, got %+v", snapshot.Arbitration)
			}
			if snapshot.Arbitration.Stage != TurnArbitrationStageDraftAllowed {
				t.Fatalf("expected grounded slot parse to promote draft_allowed, got %+v", snapshot.Arbitration)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected preview poll to include grounded slot parser result")
}

func TestASRResponderUsesStreamingTranscriberWhenAvailable(t *testing.T) {
	executor := &capturingTurnExecutor{}
	streaming := &fakeStreamingTranscriber{result: TranscriptionResult{
		Text:           "打开客厅灯",
		EndpointReason: "stream_finish",
		Partials:       []string{"打开", "打开客厅灯"},
		Mode:           "fake_stream",
	}}
	responder := NewASRResponder(
		streaming,
		"auto",
		"pcm16le",
		16000,
		1,
		false,
	).WithTurnExecutor(executor)

	audio := make([]byte, pcmFrameBytes(16000, 1, defaultASRStreamingChunkMs)*2+160)
	response, err := responder.Respond(context.Background(), TurnRequest{
		SessionID:       "sess_stream",
		DeviceID:        "rtos-001",
		AudioPCM:        audio,
		AudioBytes:      len(audio),
		InputFrames:     3,
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	})
	if err != nil {
		t.Fatalf("respond failed: %v", err)
	}
	if response.InputText != "打开客厅灯" {
		t.Fatalf("unexpected input text %q", response.InputText)
	}
	if len(streaming.chunks) < 2 {
		t.Fatalf("expected streaming transcriber to receive multiple chunks, got %d", len(streaming.chunks))
	}
	if len(executor.inputs) != 1 {
		t.Fatalf("expected one executor input, got %d", len(executor.inputs))
	}
	if got := executor.inputs[0].Metadata["speech.transcriber_mode"]; got != "fake_stream" {
		t.Fatalf("unexpected transcriber mode metadata %q", got)
	}
}

func TestASRResponderPassesRuntimeOwnedASRHintsFromSlotParser(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	sessionID := "sess_asr_hint_propagation"
	grounder.GroundPreview(SemanticSlotParseRequest{
		SessionID:   sessionID,
		PartialText: "打开客厅灯",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "device_control",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityClarifyNeeded,
		MissingSlots:  []string{"target"},
		Confidence:    0.9,
		Reason:        "missing_target_need_clarify",
		Source:        "test",
	})

	transcriber := &recordingHintTranscriber{result: TranscriptionResult{Text: "把灯调亮一点"}}
	responder := NewASRResponder(transcriber, "auto", "pcm16le", 16000, 1, false).
		WithSlotParser(NewGroundedSemanticSlotParser(fixedSemanticSlotParser{
			result: SemanticSlotParseResult{
				Domain:        SemanticSlotDomainSmartHome,
				Intent:        "set_attribute",
				SlotStatus:    SemanticSlotStatusPartial,
				Actionability: SemanticSlotActionabilityDraftOK,
				Confidence:    0.8,
				Reason:        "value_present_target_unclear",
				Source:        "test",
			},
		}, grounder), 80*time.Millisecond, 4, 0).
		WithTurnExecutor(staticTurnExecutor{text: "好的"})

	_, err := responder.Respond(context.Background(), TurnRequest{
		SessionID:       sessionID,
		DeviceID:        "rtos-001",
		AudioPCM:        make([]byte, 3200),
		AudioBytes:      3200,
		InputFrames:     5,
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	})
	if err != nil {
		t.Fatalf("Respond failed: %v", err)
	}
	if transcriber.callCount != 1 {
		t.Fatalf("expected exactly one transcribe call, got %d", transcriber.callCount)
	}
	if len(transcriber.lastReq.Hotwords) == 0 || transcriber.lastReq.Hotwords[0] != "客厅灯" {
		t.Fatalf("expected recent-context hotwords to propagate into transcription request, got %+v", transcriber.lastReq)
	}
}

func TestASRResponderPassesRuntimeOwnedASRHintsIntoPreviewStream(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	sessionID := "sess_preview_hint_propagation"
	grounder.GroundPreview(SemanticSlotParseRequest{
		SessionID:   sessionID,
		PartialText: "打开客厅灯",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "device_control",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityClarifyNeeded,
		MissingSlots:  []string{"target"},
		Confidence:    0.9,
		Reason:        "missing_target_need_clarify",
		Source:        "test",
	})

	transcriber := &recordingPreviewHintTranscriber{}
	responder := NewASRResponder(transcriber, "auto", "pcm16le", 16000, 1, false).
		WithSlotParser(NewGroundedSemanticSlotParser(fixedSemanticSlotParser{
			result: SemanticSlotParseResult{
				Domain:        SemanticSlotDomainSmartHome,
				Intent:        "set_attribute",
				SlotStatus:    SemanticSlotStatusPartial,
				Actionability: SemanticSlotActionabilityDraftOK,
				Confidence:    0.8,
				Reason:        "value_present_target_unclear",
				Source:        "test",
			},
		}, grounder), 80*time.Millisecond, 4, 0)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    sessionID,
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	_ = preview.Close()

	if len(transcriber.lastReq.Hotwords) == 0 || transcriber.lastReq.Hotwords[0] != "客厅灯" {
		t.Fatalf("expected recent-context hotwords to propagate into preview stream request, got %+v", transcriber.lastReq)
	}
}

func TestASRResponderSpeechPlannerDoesNotDoubleSynthesizeFinalResponse(t *testing.T) {
	synth := &recordingSynthesizer{}
	responder := NewASRResponder(
		fakeTranscriber{result: TranscriptionResult{Text: "打开客厅灯"}},
		"auto",
		"pcm16le",
		16000,
		1,
		false,
	).WithTurnExecutor(staticTurnExecutor{text: "好的。"}).
		WithSynthesizer(synth).
		WithSpeechPlannerConfig(SpeechPlannerConfig{
			Enabled:          true,
			MinChunkRunes:    2,
			TargetChunkRunes: 6,
		})

	response, err := responder.RespondStream(context.Background(), TurnRequest{
		SessionID:       "sess_asr_planner_once",
		TurnID:          "turn_asr_planner_once",
		TraceID:         "trace_asr_planner_once",
		DeviceID:        "dev_asr_planner_once",
		AudioPCM:        make([]byte, 3200),
		AudioBytes:      3200,
		InputFrames:     5,
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	}, nil)
	if err != nil {
		t.Fatalf("RespondStream failed: %v", err)
	}
	if response.AudioStream == nil {
		t.Fatal("expected planned audio stream on the response")
	}

	if _, err := response.AudioStream.Next(context.Background()); err != nil {
		t.Fatalf("expected synthesized audio chunk, got %v", err)
	}
	drainTestAudioStream(t, response.AudioStream)
	_ = response.AudioStream.Close()

	synth.mu.Lock()
	defer synth.mu.Unlock()
	if len(synth.texts) != 1 {
		t.Fatalf("expected exactly one synthesis request when planner audio is available, got %+v", synth.texts)
	}
	if synth.texts[0] != "好的。" {
		t.Fatalf("unexpected synthesized text %q", synth.texts[0])
	}
}

func TestASRResponderInputPreviewSuggestsCommitAfterSilence(t *testing.T) {
	responder := NewASRResponder(
		previewStreamingTranscriber{},
		"auto",
		"pcm16le",
		16000,
		1,
		false,
	)
	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_preview",
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
		Language:     "zh",
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	first, err := preview.PushAudio(context.Background(), make([]byte, 12800))
	if err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}
	if first.CommitSuggested {
		t.Fatal("preview should not suggest commit immediately")
	}
	if first.PartialText != "打开客厅灯" {
		t.Fatalf("unexpected partial text %q", first.PartialText)
	}
	later := preview.Poll(time.Now().Add(800 * time.Millisecond))
	if !later.CommitSuggested {
		t.Fatal("expected preview to suggest commit after silence window")
	}
	if later.EndpointReason != defaultServerEndpointReason {
		t.Fatalf("unexpected endpoint reason %q", later.EndpointReason)
	}
}

func TestASRResponderInputPreviewHonorsCustomTurnDetectionThresholds(t *testing.T) {
	responder := NewASRResponder(
		previewStreamingTranscriber{},
		"auto",
		"pcm16le",
		16000,
		1,
		false,
	).WithTurnDetection(100, 1200)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_preview_custom",
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
		Language:     "zh",
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 6400)); err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}
	if snapshot := preview.Poll(time.Now().Add(800 * time.Millisecond)); snapshot.CommitSuggested {
		t.Fatal("preview should not suggest commit before custom silence window elapses")
	}
	if snapshot := preview.Poll(time.Now().Add(1400 * time.Millisecond)); !snapshot.CommitSuggested {
		t.Fatal("expected preview to suggest commit after custom silence window elapses")
	}
}

func TestASRResponderInputPreviewAddsLexicalHoldForIncompletePartial(t *testing.T) {
	responder := NewASRResponder(
		previewStreamingTranscriber{text: "帮我把"},
		"auto",
		"pcm16le",
		16000,
		1,
		false,
	).WithTurnDetection(100, 300).WithTurnDetectionLexicalGuard(turnDetectorLexicalModeConservative, 600)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_preview_lexical_hold",
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
		Language:     "zh",
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 6400)); err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}
	if snapshot := preview.Poll(time.Now().Add(500 * time.Millisecond)); snapshot.CommitSuggested {
		t.Fatal("preview should keep holding incomplete lexical partial")
	}
	snapshot := preview.Poll(time.Now().Add(950 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected preview to suggest commit after lexical hold elapses")
	}
	if snapshot.EndpointReason != lexicalHoldServerEndpointReason {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}

func TestASRResponderInputPreviewEmitsAcceptCandidateBeforeCommit(t *testing.T) {
	responder := NewASRResponder(
		previewStreamingTranscriber{},
		"auto",
		"pcm16le",
		16000,
		1,
		false,
	).WithTurnDetection(100, 300)

	preview, err := responder.StartInputPreview(context.Background(), InputPreviewRequest{
		SessionID:    "sess_preview_candidate",
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
		Language:     "zh",
	})
	if err != nil {
		t.Fatalf("StartInputPreview failed: %v", err)
	}
	if _, err := preview.PushAudio(context.Background(), make([]byte, 6400)); err != nil {
		t.Fatalf("PushAudio failed: %v", err)
	}
	snapshot := preview.Poll(time.Now().Add(220 * time.Millisecond))
	if snapshot.CommitSuggested {
		t.Fatal("expected accept candidate before final commit")
	}
	if snapshot.EndpointReason != defaultServerEndpointReason {
		t.Fatalf("expected endpoint candidate reason %q, got %q", defaultServerEndpointReason, snapshot.EndpointReason)
	}
	if got := snapshot.Arbitration.Stage; got != TurnArbitrationStageAcceptCandidate {
		t.Fatalf("expected accept candidate stage, got %q", got)
	}
}
