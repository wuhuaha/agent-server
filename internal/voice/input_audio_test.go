package voice

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pion/opus/pkg/oggreader"
)

func TestParseOpusPacketInfoWideband20ms(t *testing.T) {
	info, err := parseOpusPacketInfo([]byte{9 << 3})
	if err != nil {
		t.Fatalf("parseOpusPacketInfo failed: %v", err)
	}
	if info.Mode != opusConfigurationModeSilkOnly {
		t.Fatalf("expected silk-only mode, got %s", info.Mode)
	}
	if info.SampleRateHz != 16000 {
		t.Fatalf("expected 16k sample rate, got %d", info.SampleRateHz)
	}
	if info.FrameCount != 1 {
		t.Fatalf("expected 1 frame, got %d", info.FrameCount)
	}
	if info.SampleCount != 320 {
		t.Fatalf("expected 320 decoded samples, got %d", info.SampleCount)
	}
}

func TestOpusInputNormalizerDecodesSpeechPacket(t *testing.T) {
	packet := loadTestOpusPacket(t)
	normalizer, err := NewInputNormalizer("opus", 16000, 1)
	if err != nil {
		t.Fatalf("NewInputNormalizer failed: %v", err)
	}

	decoded, err := normalizer.Decode(packet)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("expected decoded pcm payload")
	}
	if len(decoded)%2 != 0 {
		t.Fatalf("expected even pcm byte length, got %d", len(decoded))
	}
	if normalizer.OutputCodec() != "pcm16le" {
		t.Fatalf("expected normalized codec pcm16le, got %s", normalizer.OutputCodec())
	}
	if normalizer.OutputSampleRate() != 16000 {
		t.Fatalf("expected normalized sample rate 16000, got %d", normalizer.OutputSampleRate())
	}
	if normalizer.OutputChannels() != 1 {
		t.Fatalf("expected normalized channels 1, got %d", normalizer.OutputChannels())
	}
}

func loadTestOpusPacket(t *testing.T) []byte {
	t.Helper()
	oggPath := filepath.Join("..", "..", "testdata", "opus-tiny.ogg")
	oggData, err := os.ReadFile(oggPath)
	if err != nil {
		t.Fatalf("read ogg testdata failed: %v", err)
	}

	reader, _, err := oggreader.NewWith(bytes.NewReader(oggData))
	if err != nil {
		t.Fatalf("oggreader init failed: %v", err)
	}

	for {
		segments, _, err := reader.ParseNextPage()
		if errors.Is(err, io.EOF) {
			t.Fatal("no opus packet found in ogg testdata")
		}
		if err != nil {
			t.Fatalf("ParseNextPage failed: %v", err)
		}
		if len(segments) == 0 || bytes.HasPrefix(segments[0], []byte("OpusTags")) {
			continue
		}
		return append([]byte(nil), segments[0]...)
	}
}
