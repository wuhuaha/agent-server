package session

import (
	"bytes"
	"testing"
)

func TestRealtimeSessionLifecycle(t *testing.T) {
	rt := NewRealtimeSession()

	started, err := rt.Start(StartRequest{
		DeviceID:        "rtos-001",
		ClientType:      "rtos",
		FirmwareVersion: "0.1.0",
		Mode:            "voice",
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
		ClientCanEnd:    true,
		ServerCanEnd:    true,
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if started.State != StateActive {
		t.Fatalf("expected active state, got %s", started.State)
	}
	if started.SessionID == "" {
		t.Fatal("expected generated session id")
	}

	firstFrame := bytes.Repeat([]byte{0x01}, 320)
	secondFrame := bytes.Repeat([]byte{0x02}, 320)

	audio, err := rt.IngestAudioFrame(firstFrame)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if audio.InputFrames != 1 || audio.AudioBytes != len(firstFrame) {
		t.Fatalf("unexpected audio counters: %+v", audio)
	}

	audio, err = rt.IngestAudioFrame(secondFrame)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if audio.InputFrames != 2 || audio.AudioBytes != len(firstFrame)+len(secondFrame) {
		t.Fatalf("unexpected audio counters after second frame: %+v", audio)
	}

	thinking, err := rt.CommitTurn()
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if thinking.Snapshot.State != StateThinking {
		t.Fatalf("expected thinking state, got %s", thinking.Snapshot.State)
	}
	expectedAudio := append(append([]byte(nil), firstFrame...), secondFrame...)
	if !bytes.Equal(thinking.AudioPCM, expectedAudio) {
		t.Fatalf("expected committed audio length %d, got %d", len(expectedAudio), len(thinking.AudioPCM))
	}

	speaking, err := rt.SetState(StateSpeaking)
	if err != nil {
		t.Fatalf("set state failed: %v", err)
	}
	if speaking.State != StateSpeaking {
		t.Fatalf("expected speaking state, got %s", speaking.State)
	}

	closing, err := rt.End(CloseReasonCompleted)
	if err != nil {
		t.Fatalf("end failed: %v", err)
	}
	if closing.CloseReason != CloseReasonCompleted {
		t.Fatalf("expected close reason %s, got %s", CloseReasonCompleted, closing.CloseReason)
	}

	rt.ClearTurn()
	cleared := rt.Snapshot()
	if cleared.InputFrames != 0 || cleared.AudioBytes != 0 {
		t.Fatalf("expected cleared turn buffers, got %+v", cleared)
	}

	rt.Reset()
	if rt.Snapshot().State != StateIdle {
		t.Fatalf("expected idle state after reset, got %s", rt.Snapshot().State)
	}
}

func TestRealtimeSessionRejectsSecondStart(t *testing.T) {
	rt := NewRealtimeSession()
	_, err := rt.Start(StartRequest{
		DeviceID:        "rtos-001",
		ClientType:      "rtos",
		Mode:            "voice",
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	})
	if err != nil {
		t.Fatalf("unexpected error on first start: %v", err)
	}

	if _, err := rt.Start(StartRequest{
		DeviceID:        "rtos-002",
		ClientType:      "rtos",
		Mode:            "voice",
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	}); err != ErrSessionAlreadyActive {
		t.Fatalf("expected ErrSessionAlreadyActive, got %v", err)
	}
}

func TestRealtimeSessionCommitReturnsIndependentAudioCopy(t *testing.T) {
	rt := NewRealtimeSession()
	if _, err := rt.Start(StartRequest{
		DeviceID:        "rtos-001",
		ClientType:      "rtos",
		Mode:            "voice",
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if _, err := rt.IngestAudioFrame([]byte{0x01, 0x02, 0x03, 0x04}); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	turn, err := rt.CommitTurn()
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if len(turn.AudioPCM) != 4 {
		t.Fatalf("expected 4 bytes of committed audio, got %d", len(turn.AudioPCM))
	}

	turn.AudioPCM[0] = 0x09
	rt.ClearTurn()

	if _, err := rt.IngestAudioFrame([]byte{0x01, 0x02, 0x03, 0x04}); err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	nextTurn, err := rt.CommitTurn()
	if err != nil {
		t.Fatalf("second commit failed: %v", err)
	}
	if nextTurn.AudioPCM[0] != 0x01 {
		t.Fatalf("expected internal turn buffer to stay independent, got %x", nextTurn.AudioPCM[0])
	}
}

func BenchmarkRealtimeSessionIngestAudioFrame(b *testing.B) {
	rt := NewRealtimeSession()
	if _, err := rt.Start(StartRequest{
		DeviceID:        "bench-rtos-001",
		ClientType:      "rtos",
		Mode:            "voice",
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	}); err != nil {
		b.Fatalf("start failed: %v", err)
	}

	payload := bytes.Repeat([]byte{0x01}, 640)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := rt.IngestAudioFrame(payload); err != nil {
			b.Fatalf("ingest failed: %v", err)
		}
		if (i+1)%50 == 0 {
			if _, err := rt.CommitTurn(); err != nil {
				b.Fatalf("commit failed: %v", err)
			}
			rt.ClearTurn()
		}
	}
}
