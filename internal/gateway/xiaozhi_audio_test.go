//go:build system
// +build system

package gateway

import (
	"context"
	"testing"
	"time"

	"agent-server/internal/voice"
)

func TestFFmpegXiaozhiOutputEncoderProducesFirstPacket(t *testing.T) {
	encoder, ok := newDefaultXiaozhiOutputEncoder().(ffmpegXiaozhiOutputEncoder)
	if !ok || encoder.Binary == "" {
		t.Skip("ffmpeg is not available")
	}

	// 1 second of 16 kHz mono pcm16 silence is enough to force ffmpeg to emit opus pages.
	source := voice.NewStaticAudioStream([][]byte{make([]byte, 16000*2)})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := encoder.EncodePCM16(ctx, source, 16000, 1, 24000, 1, 60)
	if err != nil {
		t.Fatalf("EncodePCM16 failed: %v", err)
	}
	defer stream.Close()

	packet, err := stream.Next(ctx)
	if err != nil {
		t.Fatalf("expected first opus packet, got error: %v", err)
	}
	if len(packet) == 0 {
		t.Fatal("expected non-empty opus packet")
	}
}
