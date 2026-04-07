package voice

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
			Model:          "SenseVoiceSmall",
			Device:         "cpu",
			Language:       "zh",
			Emotion:        "calm",
			SpeakerID:      "speaker-a",
			AudioEvents:    []string{"speech"},
			EndpointReason: "silence_timeout",
			Partials:       []string{"打开", "打开客厅灯"},
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
	var partials []string
	if err := json.Unmarshal([]byte(input.Metadata["speech.partials"]), &partials); err != nil {
		t.Fatalf("partials should be encoded json: %v", err)
	}
	if len(partials) != 2 || partials[1] != "打开客厅灯" {
		t.Fatalf("unexpected partials %+v", partials)
	}
}
