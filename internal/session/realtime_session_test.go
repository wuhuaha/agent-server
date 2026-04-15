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
	if started.InputState != InputStateActive || started.OutputState != OutputStateIdle {
		t.Fatalf("expected active/idle lanes on start, got input=%s output=%s", started.InputState, started.OutputState)
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
	if audio.State != StateActive || audio.InputState != InputStateActive || audio.OutputState != OutputStateIdle {
		t.Fatalf("expected active compat state while ingesting, got state=%s input=%s output=%s", audio.State, audio.InputState, audio.OutputState)
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
	if thinking.Snapshot.InputState != InputStateCommitted || thinking.Snapshot.OutputState != OutputStateThinking {
		t.Fatalf("expected committed/thinking lanes after commit, got input=%s output=%s", thinking.Snapshot.InputState, thinking.Snapshot.OutputState)
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
	if speaking.InputState != InputStateCommitted || speaking.OutputState != OutputStateSpeaking {
		t.Fatalf("expected committed/speaking lanes, got input=%s output=%s", speaking.InputState, speaking.OutputState)
	}

	active, err := rt.SetState(StateActive)
	if err != nil {
		t.Fatalf("return active failed: %v", err)
	}
	if active.State != StateActive {
		t.Fatalf("expected active state on return, got %s", active.State)
	}
	if active.InputState != InputStateActive || active.OutputState != OutputStateIdle {
		t.Fatalf("expected active/idle lanes after active return, got input=%s output=%s", active.InputState, active.OutputState)
	}

	closing, err := rt.End(CloseReasonCompleted)
	if err != nil {
		t.Fatalf("end failed: %v", err)
	}
	if closing.CloseReason != CloseReasonCompleted {
		t.Fatalf("expected close reason %s, got %s", CloseReasonCompleted, closing.CloseReason)
	}
	if closing.State != StateClosing || closing.InputState != InputStateClosing || closing.OutputState != OutputStateClosing {
		t.Fatalf("expected closing/closing lanes, got state=%s input=%s output=%s", closing.State, closing.InputState, closing.OutputState)
	}

	rt.ClearTurn()
	cleared := rt.Snapshot()
	if cleared.InputFrames != 0 || cleared.AudioBytes != 0 {
		t.Fatalf("expected cleared turn buffers, got %+v", cleared)
	}

	rt.Reset()
	reset := rt.Snapshot()
	if reset.State != StateIdle {
		t.Fatalf("expected idle state after reset, got %s", reset.State)
	}
	if reset.InputState != InputStateIdle || reset.OutputState != OutputStateIdle {
		t.Fatalf("expected idle lanes after reset, got input=%s output=%s", reset.InputState, reset.OutputState)
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

func TestRealtimeSessionSupportsConcurrentInputAndOutputLanes(t *testing.T) {
	rt := NewRealtimeSession()
	if _, err := rt.Start(StartRequest{
		DeviceID:        "rtos-002",
		ClientType:      "rtos",
		Mode:            "voice",
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	initialFrame := bytes.Repeat([]byte{0x03}, 320)
	if _, err := rt.IngestAudioFrame(initialFrame); err != nil {
		t.Fatalf("initial ingest failed: %v", err)
	}
	if _, err := rt.CommitTurn(); err != nil {
		t.Fatalf("initial commit failed: %v", err)
	}
	rt.ClearTurn()

	speaking, err := rt.SetState(StateSpeaking)
	if err != nil {
		t.Fatalf("set speaking failed: %v", err)
	}
	if speaking.InputState != InputStateCommitted || speaking.OutputState != OutputStateSpeaking {
		t.Fatalf("expected committed/speaking lanes before overlap, got input=%s output=%s", speaking.InputState, speaking.OutputState)
	}

	bargeInFrame := bytes.Repeat([]byte{0x04}, 320)
	overlap, err := rt.IngestAudioFrame(bargeInFrame)
	if err != nil {
		t.Fatalf("overlap ingest failed: %v", err)
	}
	if overlap.State != StateSpeaking {
		t.Fatalf("expected compat speaking state during overlap, got %s", overlap.State)
	}
	if overlap.InputState != InputStateActive || overlap.OutputState != OutputStateSpeaking {
		t.Fatalf("expected active/speaking lanes during overlap, got input=%s output=%s", overlap.InputState, overlap.OutputState)
	}

	accepted, err := rt.CommitTurn()
	if err != nil {
		t.Fatalf("overlap commit failed: %v", err)
	}
	if accepted.Snapshot.State != StateSpeaking {
		t.Fatalf("expected speaking compat state when output keeps running, got %s", accepted.Snapshot.State)
	}
	if accepted.Snapshot.InputState != InputStateCommitted || accepted.Snapshot.OutputState != OutputStateSpeaking {
		t.Fatalf("expected committed/speaking lanes after overlap commit, got input=%s output=%s", accepted.Snapshot.InputState, accepted.Snapshot.OutputState)
	}
	if !bytes.Equal(accepted.AudioPCM, bargeInFrame) {
		t.Fatalf("expected overlap commit audio length %d, got %d", len(bargeInFrame), len(accepted.AudioPCM))
	}

	active, err := rt.SetState(StateActive)
	if err != nil {
		t.Fatalf("set active failed: %v", err)
	}
	if active.State != StateActive || active.InputState != InputStateActive || active.OutputState != OutputStateIdle {
		t.Fatalf("expected active/idle lanes after output return, got state=%s input=%s output=%s", active.State, active.InputState, active.OutputState)
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

func BenchmarkRealtimeSessionIngestOwnedAudioFrame(b *testing.B) {
	rt := NewRealtimeSession()
	if _, err := rt.Start(StartRequest{
		DeviceID:        "bench-rtos-owned-001",
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
		if _, err := rt.IngestOwnedAudioFrame(payload); err != nil {
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
