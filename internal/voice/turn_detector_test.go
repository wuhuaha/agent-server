package voice

import (
	"testing"
	"time"
)

func TestSilenceTurnDetectorSuggestsCommitForLexicallyCompletePartial(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeConservative,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
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
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeConservative,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
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
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeOff,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
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
		{text: "然后呢", expected: false},
		{text: "嗯", expected: false},
		{text: "那个", expected: false},
		{text: "and", expected: false},
		{text: "uh", expected: false},
		{text: "um", expected: false},
		{text: "turn on the light...", expected: false},
		{text: "turn on the kitchen light", expected: true},
	}
	for _, tc := range tests {
		if got := looksLexicallyComplete(tc.text); got != tc.expected {
			t.Fatalf("looksLexicallyComplete(%q) = %v, want %v", tc.text, got, tc.expected)
		}
	}
}

func TestSilenceTurnDetectorUsesProviderEndpointHintToShortenSilence(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeConservative,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
		},
		16000,
		1,
	)
	startedAt := time.Now()
	detector.ObserveAudio(startedAt, 6400)
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind:           TranscriptionDeltaKindPartial,
		Text:           "打开客厅灯",
		EndpointReason: "preview_tail_silence",
	})

	if snapshot := detector.Snapshot(startedAt.Add(100 * time.Millisecond)); snapshot.CommitSuggested {
		t.Fatal("preview should not commit before hint silence window elapses")
	}
	snapshot := detector.Snapshot(startedAt.Add(170 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected commit suggestion after shortened hint silence window")
	}
	if snapshot.EndpointReason != "preview_tail_silence" {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}

func TestSilenceTurnDetectorPromotesStableCompletePreviewBeforeAccept(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeConservative,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
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

	snapshot := detector.Snapshot(startedAt.Add(220 * time.Millisecond))
	if snapshot.CommitSuggested {
		t.Fatal("expected preview to remain uncommitted before required silence elapses")
	}
	if snapshot.EndpointReason != defaultServerEndpointReason {
		t.Fatalf("expected endpoint candidate reason %q, got %q", defaultServerEndpointReason, snapshot.EndpointReason)
	}
	if snapshot.Arbitration.Stage != TurnArbitrationStageAcceptCandidate {
		t.Fatalf("expected accept candidate stage, got %q", snapshot.Arbitration.Stage)
	}
	if !snapshot.Arbitration.PrewarmAllowed || !snapshot.Arbitration.DraftAllowed {
		t.Fatalf("expected stable complete preview to allow prewarm and draft, got %+v", snapshot.Arbitration)
	}
}

func TestSilenceTurnDetectorMarksIncompletePreviewAsWaitForMore(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeConservative,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
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

	snapshot := detector.Snapshot(startedAt.Add(180 * time.Millisecond))
	if snapshot.CommitSuggested {
		t.Fatal("expected incomplete preview to avoid commit")
	}
	if snapshot.Arbitration.Stage != TurnArbitrationStageWaitForMore {
		t.Fatalf("expected wait_for_more stage, got %q", snapshot.Arbitration.Stage)
	}
	if snapshot.Arbitration.PrewarmAllowed || snapshot.Arbitration.DraftAllowed || snapshot.Arbitration.AcceptCandidate {
		t.Fatalf("expected incomplete preview to avoid downstream promotion, got %+v", snapshot.Arbitration)
	}
}

func TestSilenceTurnDetectorUsesSileroEndpointHintToShortenSilence(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeConservative,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
		},
		16000,
		1,
	)
	startedAt := time.Now()
	detector.ObserveAudio(startedAt, 6400)
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind:           TranscriptionDeltaKindPartial,
		Text:           "打开客厅灯",
		EndpointReason: "preview_silero_vad_silence",
	})

	if snapshot := detector.Snapshot(startedAt.Add(100 * time.Millisecond)); snapshot.CommitSuggested {
		t.Fatal("preview should not commit before hint silence window elapses")
	}
	snapshot := detector.Snapshot(startedAt.Add(170 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected commit suggestion after shortened hint silence window")
	}
	if snapshot.EndpointReason != "preview_silero_vad_silence" {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}

func TestSilenceTurnDetectorDoesNotLetHintBypassIncompleteLexicalHold(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeConservative,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
		},
		16000,
		1,
	)
	startedAt := time.Now()
	detector.ObserveAudio(startedAt, 6400)
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind:           TranscriptionDeltaKindPartial,
		Text:           "帮我把",
		EndpointReason: "preview_tail_silence",
	})

	if snapshot := detector.Snapshot(startedAt.Add(400 * time.Millisecond)); snapshot.CommitSuggested {
		t.Fatal("incomplete lexical partial should still be held even with provider hint")
	}
	snapshot := detector.Snapshot(startedAt.Add(950 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected commit suggestion after lexical hold window")
	}
	if snapshot.EndpointReason != lexicalHoldServerEndpointReason {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}

func TestSilenceTurnDetectorHoldsStandaloneHesitationUtterance(t *testing.T) {
	detector := NewSilenceTurnDetector(
		SilenceTurnDetectorConfig{
			MinAudioMs:            100,
			SilenceMs:             300,
			LexicalEndpointMode:   turnDetectorLexicalModeConservative,
			IncompleteHoldMs:      600,
			EndpointHintSilenceMs: 120,
		},
		16000,
		1,
	)
	startedAt := time.Now()
	detector.ObserveAudio(startedAt, 6400)
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "嗯",
	})

	if snapshot := detector.Snapshot(startedAt.Add(500 * time.Millisecond)); snapshot.CommitSuggested {
		t.Fatal("standalone hesitation should stay on the lexical hold path")
	}
	snapshot := detector.Snapshot(startedAt.Add(950 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected commit suggestion after lexical hold window")
	}
	if snapshot.EndpointReason != lexicalHoldServerEndpointReason {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}
