package voice

import (
	"testing"
	"time"
)

func TestSilenceTurnDetectorSuggestsCommitForLexicallyCompletePartial(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:          100,
			SilenceMs:           300,
			LexicalEndpointMode: turnDetectorLexicalModeConservative,
			IncompleteHoldMs:    600,
		},
		16000,
		1,
	)
	startedAt := time.Now()
	detector.ObserveAudio(startedAt, 6400)
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "打开客厅灯",
	})

	snapshot := detector.Snapshot(startedAt.Add(400 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected commit suggestion after base silence window")
	}
	if snapshot.EndpointReason != defaultServerEndpointReason {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}

func TestSilenceTurnDetectorExtendsHoldForIncompleteLexicalPartial(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:          100,
			SilenceMs:           300,
			LexicalEndpointMode: turnDetectorLexicalModeConservative,
			IncompleteHoldMs:    600,
		},
		16000,
		1,
	)
	startedAt := time.Now()
	detector.ObserveAudio(startedAt, 6400)
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "帮我把",
	})

	if snapshot := detector.Snapshot(startedAt.Add(500 * time.Millisecond)); snapshot.CommitSuggested {
		t.Fatal("preview should still hold incomplete lexical partial")
	}
	snapshot := detector.Snapshot(startedAt.Add(950 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected commit suggestion after lexical hold window")
	}
	if snapshot.EndpointReason != lexicalHoldServerEndpointReason {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}

func TestSilenceTurnDetectorCanDisableLexicalGuard(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:          100,
			SilenceMs:           300,
			LexicalEndpointMode: turnDetectorLexicalModeOff,
			IncompleteHoldMs:    600,
		},
		16000,
		1,
	)
	startedAt := time.Now()
	detector.ObserveAudio(startedAt, 6400)
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "帮我把",
	})

	snapshot := detector.Snapshot(startedAt.Add(400 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected commit suggestion when lexical guard is disabled")
	}
	if snapshot.EndpointReason != defaultServerEndpointReason {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}

func TestLooksLexicallyComplete(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{text: "打开客厅灯", expected: true},
		{text: "打开客厅灯。", expected: true},
		{text: "帮我把", expected: false},
		{text: "还有", expected: false},
		{text: "and", expected: false},
		{text: "turn on the kitchen light", expected: true},
	}
	for _, tc := range tests {
		if got := looksLexicallyComplete(tc.text); got != tc.expected {
			t.Fatalf("looksLexicallyComplete(%q) = %v, want %v", tc.text, got, tc.expected)
		}
	}
}
