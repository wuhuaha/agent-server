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
	if !snapshot.Arbitration.CandidateReady || snapshot.Arbitration.DraftReady || !snapshot.Arbitration.AcceptReady {
		t.Fatalf("expected command preview to stay candidate/accept ready but wait on draft, got %+v", snapshot.Arbitration)
	}
	if !snapshot.Arbitration.PrewarmAllowed || snapshot.Arbitration.DraftAllowed {
		t.Fatalf("expected structured command preview to allow only prewarm before slot guard, got %+v", snapshot.Arbitration)
	}
	if snapshot.Arbitration.TaskFamily != SemanticTaskFamilyStructuredCommand || !snapshot.Arbitration.SlotConstraintRequired {
		t.Fatalf("expected structured command task family with slot guard, got %+v", snapshot.Arbitration)
	}
}

func TestSilenceTurnDetectorStillAllowsEarlyDraftForKnowledgeQuery(t *testing.T) {
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
		Text: "明天周几",
	})

	snapshot := detector.Snapshot(startedAt.Add(220 * time.Millisecond))
	if !snapshot.Arbitration.CandidateReady || !snapshot.Arbitration.DraftReady || !snapshot.Arbitration.AcceptReady {
		t.Fatalf("expected knowledge query to stay candidate/draft/accept ready, got %+v", snapshot.Arbitration)
	}
	if !snapshot.Arbitration.PrewarmAllowed || !snapshot.Arbitration.DraftAllowed {
		t.Fatalf("expected knowledge query to allow early draft, got %+v", snapshot.Arbitration)
	}
	if snapshot.Arbitration.TaskFamily != SemanticTaskFamilyKnowledgeQuery || snapshot.Arbitration.SlotConstraintRequired {
		t.Fatalf("expected knowledge_query task family without slot guard, got %+v", snapshot.Arbitration)
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

func TestSilenceTurnDetectorAllowsEarlyPrewarmOnMatureStablePrefix(t *testing.T) {
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
	detector.ObserveTranscriptionDelta(startedAt.Add(80*time.Millisecond), TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "打开客厅灯然后",
	})

	early := detector.Snapshot(startedAt.Add(160 * time.Millisecond))
	if early.Arbitration.PrewarmAllowed {
		t.Fatalf("expected stable prefix dwell to stay below prewarm threshold, got %+v", early.Arbitration)
	}

	mature := detector.Snapshot(startedAt.Add(260 * time.Millisecond))
	if mature.UtteranceComplete {
		t.Fatal("expected trailing incomplete live partial to keep utterance_complete false")
	}
	if mature.StablePrefix != "打开客厅灯" {
		t.Fatalf("expected stable prefix 打开客厅灯, got %q", mature.StablePrefix)
	}
	if mature.Arbitration.Stage != TurnArbitrationStagePrewarmAllowed {
		t.Fatalf("expected prewarm_allowed stage, got %q", mature.Arbitration.Stage)
	}
	if !mature.Arbitration.CandidateReady || mature.Arbitration.DraftReady {
		t.Fatalf("expected candidate_ready true and draft_ready false, got %+v", mature.Arbitration)
	}
	if !mature.Arbitration.PrewarmAllowed || mature.Arbitration.DraftAllowed || mature.Arbitration.AcceptCandidate {
		t.Fatalf("expected only low-risk prewarm promotion, got %+v", mature.Arbitration)
	}
	if mature.Arbitration.StableForMs < defaultPrewarmStableForMs {
		t.Fatalf("expected stable dwell >= %dms, got %+v", defaultPrewarmStableForMs, mature.Arbitration)
	}
}

func TestSilenceTurnDetectorComputesEffectiveWaitFromBaseAndRuleAdjust(t *testing.T) {
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

	snapshot := detector.Snapshot(startedAt.Add(500 * time.Millisecond))
	if snapshot.Arbitration.BaseWaitMs != 300 {
		t.Fatalf("expected base wait 300ms, got %+v", snapshot.Arbitration)
	}
	if snapshot.Arbitration.RuleAdjustMs != 600 {
		t.Fatalf("expected lexical/rule hold 600ms, got %+v", snapshot.Arbitration)
	}
	if snapshot.Arbitration.EffectiveWaitMs != 900 {
		t.Fatalf("expected effective wait 900ms, got %+v", snapshot.Arbitration)
	}
	if !snapshot.Arbitration.CandidateReady || !snapshot.Arbitration.AcceptReady {
		t.Fatalf("expected candidate/accept readiness to stay true while waiting, got %+v", snapshot.Arbitration)
	}
	if snapshot.Arbitration.AcceptCandidate || snapshot.Arbitration.AcceptNow {
		t.Fatalf("expected 500ms silence to remain below effective wait, got %+v", snapshot.Arbitration)
	}
}

func TestSilenceTurnDetectorKeepsSafeStablePrefixAcrossRepeatedIncompleteTail(t *testing.T) {
	detector := NewSilenceTurnDetector(NormalizeSilenceTurnDetectorConfig(SilenceTurnDetectorConfig{}), 16000, 1)
	startedAt := time.Now()
	detector.ObserveAudio(startedAt, 6400)
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "打开客厅灯",
	})
	detector.ObserveTranscriptionDelta(startedAt.Add(80*time.Millisecond), TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "打开客厅灯然后",
	})
	detector.ObserveTranscriptionDelta(startedAt.Add(160*time.Millisecond), TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "打开客厅灯然后",
	})

	snapshot := detector.Snapshot(startedAt.Add(260 * time.Millisecond))
	if snapshot.StablePrefix != "打开客厅灯" {
		t.Fatalf("expected stable prefix to keep the completed clause, got %q", snapshot.StablePrefix)
	}
}

func TestSilenceTurnDetectorSuppressesStablePrefixPrewarmDuringCorrectionPendingPartial(t *testing.T) {
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
	detector.ObserveTranscriptionDelta(startedAt.Add(120*time.Millisecond), TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: "打开客厅灯，不对",
	})

	snapshot := detector.Snapshot(startedAt.Add(220 * time.Millisecond))
	if snapshot.StablePrefix != "打开客厅灯" {
		t.Fatalf("expected stable prefix to keep the earlier clause, got %q", snapshot.StablePrefix)
	}
	if snapshot.UtteranceComplete {
		t.Fatal("expected correction-pending live partial to suppress utterance_complete despite stable prefix")
	}
	if snapshot.Arbitration.PrewarmAllowed || snapshot.Arbitration.DraftAllowed || snapshot.Arbitration.AcceptCandidate {
		t.Fatalf("expected correction-pending preview to stay out of downstream promotion, got %+v", snapshot.Arbitration)
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

func TestSilenceTurnDetectorExtendsHoldForCorrectionPendingPartial(t *testing.T) {
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
		Text: "打开客厅灯，不对",
	})

	if snapshot := detector.Snapshot(startedAt.Add(450 * time.Millisecond)); snapshot.CommitSuggested {
		t.Fatal("correction-pending partial should stay on hold before the extended window")
	}
	snapshot := detector.Snapshot(startedAt.Add(950 * time.Millisecond))
	if !snapshot.CommitSuggested {
		t.Fatal("expected correction-pending partial to commit after the extended hold window")
	}
	if snapshot.EndpointReason != correctionHoldServerEndpointReason {
		t.Fatalf("unexpected endpoint reason %q", snapshot.EndpointReason)
	}
}

func TestLooksCorrectionPendingRequiresPriorClause(t *testing.T) {
	if !looksCorrectionPending("打开客厅灯，不对") {
		t.Fatal("expected prior clause plus correction cue to stay pending")
	}
	if looksCorrectionPending("不对") {
		t.Fatal("standalone correction cue should remain actionable instead of forcing a hold")
	}
	if !looksCorrectionPending("turn on the light, wait") {
		t.Fatal("expected trailing English correction cue after a clause to stay pending")
	}
}
