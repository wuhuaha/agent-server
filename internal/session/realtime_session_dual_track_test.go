package session

import (
	"bytes"
	"testing"
)

func startDualTrackTestSession(t *testing.T) *RealtimeSession {
	t.Helper()

	rt := NewRealtimeSession()
	if _, err := rt.Start(StartRequest{
		DeviceID:        "rtos-dual-track",
		ClientType:      "rtos",
		Mode:            "voice",
		InputCodec:      "pcm16le",
		InputSampleRate: 16000,
		InputChannels:   1,
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	return rt
}

func seedSpeakingOutputLane(t *testing.T, rt *RealtimeSession) {
	t.Helper()

	initialFrame := bytes.Repeat([]byte{0x01}, 320)
	if _, err := rt.IngestAudioFrame(initialFrame); err != nil {
		t.Fatalf("initial ingest failed: %v", err)
	}
	if _, err := rt.CommitTurn(); err != nil {
		t.Fatalf("initial commit failed: %v", err)
	}
	rt.ClearTurn()
	if _, err := rt.SetOutputState(OutputStateSpeaking); err != nil {
		t.Fatalf("set output speaking failed: %v", err)
	}
}

func TestRealtimeSessionAllowsPreviewAndCommitWhileSpeaking(t *testing.T) {
	rt := startDualTrackTestSession(t)
	seedSpeakingOutputLane(t, rt)

	previewing, err := rt.SetInputState(InputStatePreviewing)
	if err != nil {
		t.Fatalf("set previewing failed: %v", err)
	}
	if previewing.State != StateSpeaking {
		t.Fatalf("expected compat speaking state while previewing during playback, got %s", previewing.State)
	}
	if previewing.InputState != InputStatePreviewing || previewing.OutputState != OutputStateSpeaking {
		t.Fatalf("expected previewing/speaking lanes, got input=%s output=%s", previewing.InputState, previewing.OutputState)
	}

	bargeInFrame := bytes.Repeat([]byte{0x02}, 320)
	active, err := rt.IngestAudioFrame(bargeInFrame)
	if err != nil {
		t.Fatalf("overlap ingest failed: %v", err)
	}
	if active.State != StateSpeaking {
		t.Fatalf("expected compat speaking state after overlap ingest, got %s", active.State)
	}
	if active.InputState != InputStateActive || active.OutputState != OutputStateSpeaking {
		t.Fatalf("expected active/speaking lanes after overlap ingest, got input=%s output=%s", active.InputState, active.OutputState)
	}

	previewing, err = rt.SetInputState(InputStatePreviewing)
	if err != nil {
		t.Fatalf("set previewing after overlap ingest failed: %v", err)
	}
	if previewing.State != StateSpeaking || previewing.InputState != InputStatePreviewing || previewing.OutputState != OutputStateSpeaking {
		t.Fatalf("expected previewing/speaking lanes before overlap commit, got state=%s input=%s output=%s", previewing.State, previewing.InputState, previewing.OutputState)
	}

	accepted, err := rt.CommitTurn()
	if err != nil {
		t.Fatalf("overlap commit failed: %v", err)
	}
	if accepted.Snapshot.State != StateSpeaking {
		t.Fatalf("expected compat speaking state after overlap commit, got %s", accepted.Snapshot.State)
	}
	if accepted.Snapshot.InputState != InputStateCommitted || accepted.Snapshot.OutputState != OutputStateSpeaking {
		t.Fatalf("expected committed/speaking lanes after overlap commit, got input=%s output=%s", accepted.Snapshot.InputState, accepted.Snapshot.OutputState)
	}
	if accepted.Snapshot.Turns != 2 {
		t.Fatalf("expected second committed turn while playback remains active, got %d", accepted.Snapshot.Turns)
	}
	if !bytes.Equal(accepted.AudioPCM, bargeInFrame) {
		t.Fatalf("expected overlap audio length %d, got %d", len(bargeInFrame), len(accepted.AudioPCM))
	}
}

func TestRealtimeSessionAcceptTextKeepsSpeakingOutputLane(t *testing.T) {
	rt := startDualTrackTestSession(t)
	seedSpeakingOutputLane(t, rt)

	previewing, err := rt.SetInputState(InputStatePreviewing)
	if err != nil {
		t.Fatalf("set previewing failed: %v", err)
	}
	if previewing.State != StateSpeaking || previewing.InputState != InputStatePreviewing || previewing.OutputState != OutputStateSpeaking {
		t.Fatalf("expected previewing/speaking lanes before text accept, got state=%s input=%s output=%s", previewing.State, previewing.InputState, previewing.OutputState)
	}

	active, err := rt.AcceptText()
	if err != nil {
		t.Fatalf("accept text failed: %v", err)
	}
	if active.State != StateSpeaking {
		t.Fatalf("expected compat speaking state after accepting text while speaking, got %s", active.State)
	}
	if active.InputState != InputStateActive || active.OutputState != OutputStateSpeaking {
		t.Fatalf("expected active/speaking lanes after accepting text while speaking, got input=%s output=%s", active.InputState, active.OutputState)
	}

	accepted, err := rt.CommitTurn()
	if err != nil {
		t.Fatalf("text commit failed: %v", err)
	}
	if accepted.Snapshot.State != StateSpeaking {
		t.Fatalf("expected compat speaking state after text commit, got %s", accepted.Snapshot.State)
	}
	if accepted.Snapshot.InputState != InputStateCommitted || accepted.Snapshot.OutputState != OutputStateSpeaking {
		t.Fatalf("expected committed/speaking lanes after text commit, got input=%s output=%s", accepted.Snapshot.InputState, accepted.Snapshot.OutputState)
	}
	if accepted.Snapshot.Turns != 2 {
		t.Fatalf("expected second turn after text commit during playback, got %d", accepted.Snapshot.Turns)
	}
	if len(accepted.AudioPCM) != 0 {
		t.Fatalf("expected no audio payload for text-only overlap commit, got %d bytes", len(accepted.AudioPCM))
	}
}

func TestRealtimeSessionDirectLaneSettersPreserveDualTrackSemantics(t *testing.T) {
	rt := startDualTrackTestSession(t)

	previewing, err := rt.SetInputState(InputStatePreviewing)
	if err != nil {
		t.Fatalf("set previewing failed: %v", err)
	}
	if previewing.State != StateActive {
		t.Fatalf("expected compat active state while only previewing, got %s", previewing.State)
	}
	if previewing.InputState != InputStatePreviewing || previewing.OutputState != OutputStateIdle {
		t.Fatalf("expected previewing/idle lanes, got input=%s output=%s", previewing.InputState, previewing.OutputState)
	}

	speaking, err := rt.SetOutputState(OutputStateSpeaking)
	if err != nil {
		t.Fatalf("set output speaking failed: %v", err)
	}
	if speaking.State != StateSpeaking {
		t.Fatalf("expected compat speaking state after enabling output lane, got %s", speaking.State)
	}
	if speaking.InputState != InputStatePreviewing || speaking.OutputState != OutputStateSpeaking {
		t.Fatalf("expected previewing/speaking lanes, got input=%s output=%s", speaking.InputState, speaking.OutputState)
	}

	committed, err := rt.SetInputState(InputStateCommitted)
	if err != nil {
		t.Fatalf("set committed failed: %v", err)
	}
	if committed.State != StateSpeaking {
		t.Fatalf("expected compat speaking state while committed input overlaps playback, got %s", committed.State)
	}
	if committed.InputState != InputStateCommitted || committed.OutputState != OutputStateSpeaking {
		t.Fatalf("expected committed/speaking lanes, got input=%s output=%s", committed.InputState, committed.OutputState)
	}

	idleOutput, err := rt.SetOutputState(OutputStateIdle)
	if err != nil {
		t.Fatalf("set output idle failed: %v", err)
	}
	if idleOutput.State != StateActive {
		t.Fatalf("expected compat active state once output lane stops, got %s", idleOutput.State)
	}
	if idleOutput.InputState != InputStateCommitted || idleOutput.OutputState != OutputStateIdle {
		t.Fatalf("expected committed/idle lanes after output stop, got input=%s output=%s", idleOutput.InputState, idleOutput.OutputState)
	}
}
