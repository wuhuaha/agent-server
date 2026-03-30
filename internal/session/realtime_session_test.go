package session

import "testing"

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

	audio, err := rt.IngestAudioFrame(make([]byte, 640))
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if audio.InputFrames != 1 || audio.AudioBytes != 640 {
		t.Fatalf("unexpected audio counters: %+v", audio)
	}
	if len(audio.TurnAudio) != 640 {
		t.Fatalf("expected 640 bytes of buffered turn audio, got %d", len(audio.TurnAudio))
	}

	thinking, err := rt.CommitTurn()
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if thinking.State != StateThinking {
		t.Fatalf("expected thinking state, got %s", thinking.State)
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
	if cleared.InputFrames != 0 || cleared.AudioBytes != 0 || len(cleared.TurnAudio) != 0 {
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
