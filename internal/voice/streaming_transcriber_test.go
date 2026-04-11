package voice

import (
	"context"
	"testing"
)

type recordingTranscriber struct {
	requests []TranscriptionRequest
	result   TranscriptionResult
	err      error
}

func (t *recordingTranscriber) Transcribe(_ context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	t.requests = append(t.requests, req)
	if t.err != nil {
		return TranscriptionResult{}, t.err
	}
	return t.result, nil
}

type recordingDeltaSink struct {
	deltas []TranscriptionDelta
}

func (s *recordingDeltaSink) EmitTranscriptionDelta(_ context.Context, delta TranscriptionDelta) error {
	s.deltas = append(s.deltas, delta)
	return nil
}

func TestBufferedStreamingTranscriberBuffersAudioAndEmitsLifecycleDeltas(t *testing.T) {
	inner := &recordingTranscriber{result: TranscriptionResult{
		Text:           "hello from buffered stream",
		EndpointReason: "buffered_finish",
	}}
	transcriber := NewBufferedStreamingTranscriber(inner)
	sink := &recordingDeltaSink{}

	stream, err := transcriber.StartStream(context.Background(), TranscriptionRequest{
		SessionID:    "sess_test",
		TurnID:       "turn_test",
		TraceID:      "trace_test",
		DeviceID:     "rtos-1",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
		Language:     "auto",
	}, sink)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}
	if err := stream.PushAudio(context.Background(), []byte{0x01, 0x02}); err != nil {
		t.Fatalf("PushAudio #1 failed: %v", err)
	}
	if err := stream.PushAudio(context.Background(), []byte{0x03, 0x04, 0x05, 0x06}); err != nil {
		t.Fatalf("PushAudio #2 failed: %v", err)
	}
	result, err := stream.Finish(context.Background())
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}
	if len(inner.requests) != 1 {
		t.Fatalf("expected one inner request, got %d", len(inner.requests))
	}
	if got := len(inner.requests[0].AudioPCM); got != 6 {
		t.Fatalf("expected 6 buffered bytes, got %d", got)
	}
	if result.Mode != "buffered_stream" {
		t.Fatalf("expected buffered_stream mode, got %q", result.Mode)
	}
	if len(sink.deltas) != 3 {
		t.Fatalf("expected 3 deltas, got %+v", sink.deltas)
	}
	if sink.deltas[0].Kind != TranscriptionDeltaKindSpeechStart {
		t.Fatalf("unexpected first delta %+v", sink.deltas[0])
	}
	if sink.deltas[1].Kind != TranscriptionDeltaKindSpeechEnd {
		t.Fatalf("unexpected second delta %+v", sink.deltas[1])
	}
	if sink.deltas[2].Kind != TranscriptionDeltaKindFinal || sink.deltas[2].Text != "hello from buffered stream" {
		t.Fatalf("unexpected final delta %+v", sink.deltas[2])
	}
}
