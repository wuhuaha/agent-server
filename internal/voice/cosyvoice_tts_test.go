//go:build integration

package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCosyVoiceHTTPSynthesizerStreamSynthesize(t *testing.T) {
	sourcePCM := repeatedPCM16Sample(0x0400, 882)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference_sft" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form failed: %v", err)
		}
		if got := r.Form.Get("tts_text"); got != "本地 TTS 测试" {
			t.Fatalf("unexpected text %q", got)
		}
		if got := r.Form.Get("spk_id"); got != "中文女" {
			t.Fatalf("unexpected spk_id %q", got)
		}
		if got := r.Form.Get("stream"); got != "true" {
			t.Fatalf("unexpected stream flag %q", got)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(sourcePCM[:len(sourcePCM)/2])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write(sourcePCM[len(sourcePCM)/2:])
	}))
	defer server.Close()

	synthesizer := NewCosyVoiceHTTPSynthesizer(CosyVoiceHTTPConfig{
		BaseURL:            server.URL,
		Mode:               "sft",
		SpeakerID:          "中文女",
		SourceSampleRateHz: 22050,
		TargetCodec:        "pcm16le",
		TargetRateHz:       16000,
		TargetChannels:     1,
		Timeout:            5 * time.Second,
	})

	stream, err := synthesizer.StreamSynthesize(context.Background(), SynthesisRequest{Text: "本地 TTS 测试"})
	if err != nil {
		t.Fatalf("StreamSynthesize failed: %v", err)
	}
	defer stream.Close()

	collected, err := collectAudioStream(context.Background(), stream)
	if err != nil {
		t.Fatalf("collectAudioStream failed: %v", err)
	}
	if len(collected) != 1280 {
		t.Fatalf("expected 1280 bytes after resample, got %d", len(collected))
	}
	if !bytes.Equal(collected, repeatedPCM16Sample(0x0400, 640)) {
		t.Fatalf("expected constant-amplitude resampled audio")
	}
}

func TestCosyVoiceHTTPSynthesizerSupportsInstructMode(t *testing.T) {
	sourcePCM := repeatedPCM16Sample(0x0200, 441)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference_instruct" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form failed: %v", err)
		}
		if got := r.Form.Get("instruct_text"); got != "保持温和、自然" {
			t.Fatalf("unexpected instruct text %q", got)
		}
		_, _ = w.Write(sourcePCM)
	}))
	defer server.Close()

	synthesizer := NewCosyVoiceHTTPSynthesizer(CosyVoiceHTTPConfig{
		BaseURL:            server.URL,
		Mode:               "instruct",
		SpeakerID:          "中文女",
		InstructText:       "保持温和、自然",
		SourceSampleRateHz: 22050,
		TargetCodec:        "pcm16le",
		TargetRateHz:       16000,
		TargetChannels:     1,
		Timeout:            5 * time.Second,
	})

	result, err := synthesizer.Synthesize(context.Background(), SynthesisRequest{Text: "指令式合成"})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if result.Model != "cosyvoice_http" {
		t.Fatalf("unexpected model %q", result.Model)
	}
	if result.Voice != "中文女" {
		t.Fatalf("unexpected voice %q", result.Voice)
	}
	if len(result.AudioPCM) != 640 {
		t.Fatalf("expected 640 bytes after resample, got %d", len(result.AudioPCM))
	}
}

func repeatedPCM16Sample(sample int16, count int) []byte {
	payload := make([]byte, count*2)
	for i := 0; i < count; i++ {
		binary.LittleEndian.PutUint16(payload[i*2:i*2+2], uint16(sample))
	}
	return payload
}
