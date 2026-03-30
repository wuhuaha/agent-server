package voice

import (
	"context"
	"errors"
	"strings"
	"testing"
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
	if len(response.AudioChunks) != 2 {
		t.Fatalf("expected placeholder audio, got %d chunks", len(response.AudioChunks))
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
