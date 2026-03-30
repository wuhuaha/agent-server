package voice

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestChunkPCM16(t *testing.T) {
	audio := make([]byte, 3200)
	chunks := chunkPCM16(audio, 16000, 1, 20)
	if len(chunks) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if len(chunk) != 640 {
			t.Fatalf("expected 640-byte chunk, got %d", len(chunk))
		}
	}
}

func TestAdaptPCM16Downsample(t *testing.T) {
	source := make([]byte, 24000*2)
	for i := 0; i < 24000; i++ {
		binary.LittleEndian.PutUint16(source[i*2:i*2+2], uint16(int16(i%2000)))
	}
	adapted, err := adaptPCM16(source, 24000, 1, 16000, 1)
	if err != nil {
		t.Fatalf("adapt failed: %v", err)
	}
	if len(adapted) != 16000*2 {
		t.Fatalf("expected 32000 bytes after resample, got %d", len(adapted))
	}
}

func TestDecodeWAVPCM16(t *testing.T) {
	var wav bytes.Buffer
	data := make([]byte, 320)
	writeTestWAV(&wav, data, 24000, 1)

	pcm, sampleRateHz, channels, err := decodeWAVPCM16(wav.Bytes())
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if sampleRateHz != 24000 || channels != 1 {
		t.Fatalf("unexpected metadata %d/%d", sampleRateHz, channels)
	}
	if len(pcm) != len(data) {
		t.Fatalf("unexpected pcm length %d", len(pcm))
	}
}

func writeTestWAV(buf *bytes.Buffer, pcm []byte, sampleRateHz, channels int) {
	bitsPerSample := uint16(16)
	byteRate := uint32(sampleRateHz * channels * 2)
	blockAlign := uint16(channels * 2)
	dataSize := uint32(len(pcm))
	riffSize := uint32(36) + dataSize

	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, riffSize)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRateHz))
	_ = binary.Write(buf, binary.LittleEndian, byteRate)
	_ = binary.Write(buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(buf, binary.LittleEndian, bitsPerSample)
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, dataSize)
	buf.Write(pcm)
}
