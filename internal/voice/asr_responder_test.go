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

type fakeStreamingTranscriber struct {
	result TranscriptionResult
	err    error
	chunks [][]byte
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

type previewStreamingTranscriber struct{}

func (previewStreamingTranscriber) Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error) {
	return TranscriptionResult{}, nil
}

func (previewStreamingTranscriber) StartStream(_ context.Context, _ TranscriptionRequest, sink TranscriptionDeltaSink) (StreamingTranscriptionSession, error) {
	return &previewStreamingSession{sink: sink}, nil
}

type previewStreamingSession struct {
	sink    TranscriptionDeltaSink
	started bool
}

func (s *previewStreamingSession) PushAudio(ctx context.Context, chunk []byte) error {
	if !s.started {
		s.started = true
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{Kind: TranscriptionDeltaKindSpeechStart}); err != nil {
			return err
		}
	}
	return emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{Kind: TranscriptionDeltaKindPartial, Text: "打开客厅灯"})
}

func (s *previewStreamingSession) Finish(context.Context) (TranscriptionResult, error) {
	return TranscriptionResult{}, nil
}

func (s *previewStreamingSession) Close() error {
	return nil
}

type capturingTurnExecutor struct {
	inputs []agent.TurnInput
}

func (e *capturingTurnExecutor) ExecuteTurn(_ context.Context, input agent.TurnInput) (agent.TurnOutput, error) {
	e.inputs = append(e.inputs, input)
	return agent.TurnOutput{Text: "handled: " + input.UserText}, nil
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
			Text:           "打开客厅灯",
			Segments:       []string{"打开客厅灯"},
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
	if response.Text != "handled: 打开客厅灯" {
		t.Fatalf("unexpected response text %q", response.Text)
	}
	if response.InputText != "打开客厅灯" {
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
