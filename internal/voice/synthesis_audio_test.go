package voice

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestSynthesizedAudioFallsBackWhenStreamingReturnsNoAudio(t *testing.T) {
	t.Parallel()

	audioPCM := bytes.Repeat([]byte{1, 0}, 320)
	synth := &stubFallbackSynthesizer{
		stream: NewStaticAudioStream(nil),
		result: SynthesisResult{
			AudioPCM:     audioPCM,
			SampleRateHz: 16000,
			Channels:     1,
			Codec:        "pcm16le",
		},
	}

	_, stream := synthesizedAudio(context.Background(), synth, TurnRequest{SessionID: "sess_1"}, "你好", "已为你打开客厅灯。")
	if stream == nil {
		t.Fatal("expected synthesized audio stream")
	}
	defer stream.Close()

	collected, err := collectAudioStream(context.Background(), stream)
	if err != nil {
		t.Fatalf("collectAudioStream failed: %v", err)
	}
	if !bytes.Equal(collected, audioPCM) {
		t.Fatalf("unexpected fallback audio length/content: got=%d want=%d", len(collected), len(audioPCM))
	}
	if synth.synthesizeCalls != 1 {
		t.Fatalf("expected fallback synthesize call, got %d", synth.synthesizeCalls)
	}
}

func TestSynthesizedAudioKeepsPrimaryStreamWhenStreamingProducesAudio(t *testing.T) {
	t.Parallel()

	audioPCM := bytes.Repeat([]byte{2, 0}, 160)
	synth := &stubFallbackSynthesizer{
		stream: NewStaticAudioStream([][]byte{audioPCM}),
		result: SynthesisResult{
			AudioPCM:     bytes.Repeat([]byte{9, 0}, 160),
			SampleRateHz: 16000,
			Channels:     1,
			Codec:        "pcm16le",
		},
	}

	_, stream := synthesizedAudio(context.Background(), synth, TurnRequest{SessionID: "sess_2"}, "你好", "好的。")
	if stream == nil {
		t.Fatal("expected synthesized audio stream")
	}
	defer stream.Close()

	collected, err := collectAudioStream(context.Background(), stream)
	if err != nil {
		t.Fatalf("collectAudioStream failed: %v", err)
	}
	if !bytes.Equal(collected, audioPCM) {
		t.Fatalf("unexpected primary stream audio length/content: got=%d want=%d", len(collected), len(audioPCM))
	}
	if synth.synthesizeCalls != 0 {
		t.Fatalf("expected no fallback synthesize call, got %d", synth.synthesizeCalls)
	}
}

type stubFallbackSynthesizer struct {
	stream          AudioStream
	streamErr       error
	result          SynthesisResult
	synthesizeErr   error
	synthesizeCalls int
}

func (s *stubFallbackSynthesizer) StreamSynthesize(context.Context, SynthesisRequest) (AudioStream, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	if s.stream != nil {
		return s.stream, nil
	}
	return nil, io.EOF
}

func (s *stubFallbackSynthesizer) Synthesize(context.Context, SynthesisRequest) (SynthesisResult, error) {
	s.synthesizeCalls++
	if s.synthesizeErr != nil {
		return SynthesisResult{}, s.synthesizeErr
	}
	return s.result, nil
}

type errAudioStream struct{}

func (errAudioStream) Next(context.Context) ([]byte, error) {
	return nil, errors.New("stream failed")
}

func (errAudioStream) Close() error {
	return nil
}
