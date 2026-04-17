package voice

import (
	"reflect"
	"testing"
	"time"
)

type fusedWaitMetrics struct {
	BaseWaitMs          int
	RuleAdjustMs        int
	SemanticWaitDeltaMs int
	PunctuationAdjustMs int
	SlotGuardAdjustMs   int
	EffectiveWaitMs     int
}

func TestFusedEndpointCompleteQuestionProgressesThroughReadinessStyles(t *testing.T) {
	detector, startedAt := newFusedEndpointTestDetector(t, "明天周几？")

	early := detector.Snapshot(startedAt.Add(60 * time.Millisecond))
	if !candidateReadyStyle(early) {
		t.Fatalf("expected complete question to reach candidate-ready style early, got %+v", early.Arbitration)
	}
	if !draftReadyStyle(early) {
		t.Fatalf("expected complete question to reach draft-ready style early, got %+v", early.Arbitration)
	}
	if !acceptReadyStyle(early) {
		t.Fatalf("complete question should be structurally accept-ready once audio/text are mature, got %+v", early.Arbitration)
	}
	if early.Arbitration.AcceptCandidate || early.Arbitration.AcceptNow {
		t.Fatalf("complete question should not enter accept window yet, got %+v", early.Arbitration)
	}

	later := detector.Snapshot(startedAt.Add(220 * time.Millisecond))
	if !candidateReadyStyle(later) {
		t.Fatalf("expected candidate-ready style to remain true, got %+v", later.Arbitration)
	}
	if !draftReadyStyle(later) {
		t.Fatalf("expected draft-ready style to remain true, got %+v", later.Arbitration)
	}
	if !acceptReadyStyle(later) {
		t.Fatalf("expected accept-ready structural gate to remain true, got %+v", later.Arbitration)
	}
	if !later.Arbitration.AcceptCandidate || !later.Arbitration.AcceptNow {
		t.Fatalf("expected accept window to open once silence reaches the effective wait budget, got %+v", later.Arbitration)
	}
}

func TestFusedEndpointCompleteQuestionWaitsLessThanContinueOrCorrection(t *testing.T) {
	question := mergedSemanticPreview(t, "明天周几？", 220*time.Millisecond, SemanticTurnJudgement{
		UtteranceStatus:    SemanticUtteranceComplete,
		InterruptionIntent: SemanticIntentQuestion,
		Confidence:         0.92,
		Reason:             "standalone_question",
		Source:             "test",
	}, func(result *SemanticTurnJudgement) {
		setOptionalStringField(result, "DynamicWaitPolicy", "shorten")
		setOptionalIntField(result, "WaitDeltaMs", -140)
	})
	followOn := mergedSemanticPreview(t, "明天周几然后", 220*time.Millisecond, SemanticTurnJudgement{
		UtteranceStatus:    SemanticUtteranceIncomplete,
		InterruptionIntent: SemanticIntentContinue,
		Confidence:         0.84,
		Reason:             "tail_continue",
		Source:             "test",
	}, func(result *SemanticTurnJudgement) {
		setOptionalStringField(result, "DynamicWaitPolicy", "extend")
		setOptionalIntField(result, "WaitDeltaMs", 180)
	})
	correction := mergedSemanticPreview(t, "明天周几，不对", 220*time.Millisecond, SemanticTurnJudgement{
		UtteranceStatus:    SemanticUtteranceCorrection,
		InterruptionIntent: SemanticIntentCorrection,
		Confidence:         0.89,
		Reason:             "repair_in_progress",
		Source:             "test",
	}, func(result *SemanticTurnJudgement) {
		setOptionalStringField(result, "DynamicWaitPolicy", "extend")
		setOptionalIntField(result, "WaitDeltaMs", 320)
	})

	if !acceptReadyStyle(question) {
		t.Fatalf("expected complete question to stay structurally accept-ready, got %+v", question.Arbitration)
	}
	if !question.Arbitration.AcceptCandidate || !question.Arbitration.AcceptNow {
		t.Fatalf("expected complete question to reach accept window first, got %+v", question.Arbitration)
	}
	if !acceptReadyStyle(followOn) {
		t.Fatalf("expected follow-on partial to stay structurally ready while waiting for more silence, got %+v", followOn.Arbitration)
	}
	if followOn.Arbitration.AcceptCandidate || followOn.Arbitration.AcceptNow {
		t.Fatalf("expected follow-on partial to stay out of accept window, got %+v", followOn.Arbitration)
	}
	if !acceptReadyStyle(correction) {
		t.Fatalf("expected correction partial to stay structurally ready while semantic hold extends wait, got %+v", correction.Arbitration)
	}
	if correction.Arbitration.AcceptCandidate || correction.Arbitration.AcceptNow {
		t.Fatalf("expected correction partial to stay out of accept window, got %+v", correction.Arbitration)
	}

	questionWait := effectiveWaitMs(question.Arbitration)
	followOnWait := effectiveWaitMs(followOn.Arbitration)
	correctionWait := effectiveWaitMs(correction.Arbitration)
	if questionWait >= followOnWait {
		t.Fatalf("expected complete question wait %dms to be shorter than continue wait %dms", questionWait, followOnWait)
	}
	if questionWait >= correctionWait {
		t.Fatalf("expected complete question wait %dms to be shorter than correction wait %dms", questionWait, correctionWait)
	}

	t.Run("wait_accounting_when_exposed", func(t *testing.T) {
		if !hasFusedWaitMetrics(question.Arbitration) {
			t.Skip("TurnArbitration does not expose BaseWaitMs/SemanticWaitDeltaMs/PunctuationAdjustMs/SlotGuardAdjustMs/EffectiveWaitMs yet")
		}
		assertWaitAccountingFormula(t, question.Arbitration)
		assertWaitAccountingFormula(t, followOn.Arbitration)
		assertWaitAccountingFormula(t, correction.Arbitration)

		metrics := mustFusedWaitMetrics(t, question.Arbitration)
		if metrics.SemanticWaitDeltaMs >= 0 {
			t.Fatalf("expected complete question to shorten semantic wait, got %+v", metrics)
		}
	})
}

func TestFusedEndpointSlotIncompleteCommandStaysConservativeLonger(t *testing.T) {
	base := previewSnapshotForText(t, "把灯调亮一点", 60*time.Millisecond)
	if !candidateReadyStyle(base) {
		t.Fatalf("expected lexically complete command preview to reach candidate-ready style, got %+v", base.Arbitration)
	}
	if !draftReadyStyle(base) {
		t.Fatalf("expected lexically complete command preview to reach draft-ready style, got %+v", base.Arbitration)
	}
	if !acceptReadyStyle(base) {
		t.Fatalf("command preview should already be structurally accept-ready before silence closes the turn, got %+v", base.Arbitration)
	}
	if base.Arbitration.AcceptCandidate || base.Arbitration.AcceptNow {
		t.Fatalf("command preview should not enter accept window yet, got %+v", base.Arbitration)
	}

	guarded := mergeSemanticSlotParse(base, SemanticSlotParseResult{
		CandidateKey:  semanticCandidateKey(base.StablePrefix, base.PartialText),
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "set_attribute",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityWaitMore,
		MissingSlots:  []string{"target"},
		Confidence:    0.9,
		Reason:        "tail_slot_incomplete",
		Source:        "test",
	})

	if !candidateReadyStyle(guarded) {
		t.Fatalf("slot-incomplete command should still be mature enough for low-risk candidate handling, got %+v", guarded.Arbitration)
	}
	if draftReadyStyle(guarded) {
		t.Fatalf("slot-incomplete command should lose draft-ready style, got %+v", guarded.Arbitration)
	}
	if !acceptReadyStyle(guarded) {
		t.Fatalf("slot-incomplete command should still be structurally accept-ready while the slot guard only extends wait, got %+v", guarded.Arbitration)
	}
	if guarded.Arbitration.AcceptCandidate || guarded.Arbitration.AcceptNow {
		t.Fatalf("slot-incomplete command should stay out of accept window, got %+v", guarded.Arbitration)
	}
	if !guarded.Arbitration.SlotReady || len(guarded.Arbitration.SlotMissing) != 1 || guarded.Arbitration.SlotMissing[0] != "target" {
		t.Fatalf("expected slot parser guard details to propagate, got %+v", guarded.Arbitration)
	}

	t.Run("slot_guard_wait_extension_when_exposed", func(t *testing.T) {
		if !hasFusedWaitMetrics(guarded.Arbitration) {
			t.Skip("TurnArbitration does not expose fused wait accounting fields yet")
		}
		assertWaitAccountingFormula(t, guarded.Arbitration)
		metrics := mustFusedWaitMetrics(t, guarded.Arbitration)
		if metrics.SlotGuardAdjustMs <= 0 {
			t.Fatalf("expected slot-incomplete command to extend wait via SlotGuardAdjustMs, got %+v", metrics)
		}
		if metrics.EffectiveWaitMs <= metrics.BaseWaitMs {
			t.Fatalf("expected slot guard to keep the command conservative longer, got %+v", metrics)
		}
	})
}

func newFusedEndpointTestDetector(t *testing.T, text string) (SilenceTurnDetector, time.Time) {
	t.Helper()
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
	startedAt := time.Unix(1_700_000_000, 0)
	detector.ObserveAudio(startedAt, pcmFrameBytes(16000, 1, 200))
	detector.ObserveTranscriptionDelta(startedAt, TranscriptionDelta{
		Kind: TranscriptionDeltaKindPartial,
		Text: text,
	})
	return detector, startedAt
}

func previewSnapshotForText(t *testing.T, text string, silence time.Duration) InputPreview {
	t.Helper()
	detector, startedAt := newFusedEndpointTestDetector(t, text)
	return detector.Snapshot(startedAt.Add(silence))
}

func mergedSemanticPreview(t *testing.T, text string, silence time.Duration, result SemanticTurnJudgement, configure func(*SemanticTurnJudgement)) InputPreview {
	t.Helper()
	snapshot := previewSnapshotForText(t, text, silence)
	result.CandidateKey = semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText)
	if configure != nil {
		configure(&result)
	}
	return mergeSemanticJudgement(snapshot, result)
}

func candidateReadyStyle(snapshot InputPreview) bool {
	if ready, ok := optionalBoolField(snapshot.Arbitration, "CandidateReady"); ok {
		return ready
	}
	switch snapshot.Arbitration.Stage {
	case TurnArbitrationStagePrewarmAllowed,
		TurnArbitrationStageDraftAllowed,
		TurnArbitrationStageAcceptCandidate,
		TurnArbitrationStageAcceptNow:
		return true
	default:
		return snapshot.Arbitration.AcceptCandidate || snapshot.Arbitration.AcceptNow
	}
}

func draftReadyStyle(snapshot InputPreview) bool {
	if ready, ok := optionalBoolField(snapshot.Arbitration, "DraftReady"); ok {
		return ready
	}
	switch snapshot.Arbitration.Stage {
	case TurnArbitrationStageDraftAllowed,
		TurnArbitrationStageAcceptCandidate,
		TurnArbitrationStageAcceptNow:
		return true
	default:
		return snapshot.Arbitration.DraftAllowed
	}
}

func acceptReadyStyle(snapshot InputPreview) bool {
	if ready, ok := optionalBoolField(snapshot.Arbitration, "AcceptReady"); ok {
		return ready
	}
	switch snapshot.Arbitration.Stage {
	case TurnArbitrationStageAcceptCandidate, TurnArbitrationStageAcceptNow:
		return true
	default:
		return snapshot.Arbitration.AcceptCandidate || snapshot.Arbitration.AcceptNow
	}
}

func effectiveWaitMs(arbitration TurnArbitration) int {
	if wait, ok := optionalIntField(arbitration, "EffectiveWaitMs"); ok {
		return wait
	}
	return arbitration.RequiredSilenceMs
}

func hasFusedWaitMetrics(arbitration TurnArbitration) bool {
	_, ok := fusedWaitMetricsFromArbitration(arbitration)
	return ok
}

func assertWaitAccountingFormula(t *testing.T, arbitration TurnArbitration) {
	t.Helper()
	metrics := mustFusedWaitMetrics(t, arbitration)
	want := clampInt(
		metrics.BaseWaitMs+metrics.RuleAdjustMs+metrics.SemanticWaitDeltaMs+metrics.PunctuationAdjustMs+metrics.SlotGuardAdjustMs,
		defaultTurnDetectorMinWaitMs,
		defaultTurnDetectorMaxWaitMs,
	)
	if got := metrics.EffectiveWaitMs; got != want {
		t.Fatalf("expected EffectiveWaitMs to equal the clamped fused wait sum, got %+v", metrics)
	}
}

func mustFusedWaitMetrics(t *testing.T, arbitration TurnArbitration) fusedWaitMetrics {
	t.Helper()
	metrics, ok := fusedWaitMetricsFromArbitration(arbitration)
	if !ok {
		t.Fatalf("expected fused wait accounting fields on TurnArbitration")
	}
	return metrics
}

func fusedWaitMetricsFromArbitration(arbitration TurnArbitration) (fusedWaitMetrics, bool) {
	baseWaitMs, ok := optionalIntField(arbitration, "BaseWaitMs")
	if !ok {
		return fusedWaitMetrics{}, false
	}
	ruleAdjustMs, ok := optionalIntField(arbitration, "RuleAdjustMs")
	if !ok {
		return fusedWaitMetrics{}, false
	}
	semanticWaitDeltaMs, ok := optionalIntField(arbitration, "SemanticWaitDeltaMs")
	if !ok {
		return fusedWaitMetrics{}, false
	}
	punctuationAdjustMs, ok := optionalIntField(arbitration, "PunctuationAdjustMs")
	if !ok {
		return fusedWaitMetrics{}, false
	}
	slotGuardAdjustMs, ok := optionalIntField(arbitration, "SlotGuardAdjustMs")
	if !ok {
		return fusedWaitMetrics{}, false
	}
	effectiveWaitMs, ok := optionalIntField(arbitration, "EffectiveWaitMs")
	if !ok {
		return fusedWaitMetrics{}, false
	}
	return fusedWaitMetrics{
		BaseWaitMs:          baseWaitMs,
		RuleAdjustMs:        ruleAdjustMs,
		SemanticWaitDeltaMs: semanticWaitDeltaMs,
		PunctuationAdjustMs: punctuationAdjustMs,
		SlotGuardAdjustMs:   slotGuardAdjustMs,
		EffectiveWaitMs:     effectiveWaitMs,
	}, true
}

func optionalBoolField(target any, name string) (bool, bool) {
	field, ok := optionalFieldValue(target, name)
	if !ok || field.Kind() != reflect.Bool {
		return false, false
	}
	return field.Bool(), true
}

func optionalIntField(target any, name string) (int, bool) {
	field, ok := optionalFieldValue(target, name)
	if !ok {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(field.Int()), true
	default:
		return 0, false
	}
}

func optionalFieldValue(target any, name string) (reflect.Value, bool) {
	value := reflect.ValueOf(target)
	if !value.IsValid() {
		return reflect.Value{}, false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	field := value.FieldByName(name)
	if !field.IsValid() {
		return reflect.Value{}, false
	}
	return field, true
}

func setOptionalIntField(target any, name string, value int) bool {
	field, ok := settableFieldValue(target, name)
	if !ok {
		return false
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		field.SetInt(int64(value))
		return true
	default:
		return false
	}
}

func setOptionalStringField(target any, name, value string) bool {
	field, ok := settableFieldValue(target, name)
	if !ok || field.Kind() != reflect.String {
		return false
	}
	field.SetString(value)
	return true
}

func settableFieldValue(target any, name string) (reflect.Value, bool) {
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return reflect.Value{}, false
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	field := value.FieldByName(name)
	if !field.IsValid() || !field.CanSet() {
		return reflect.Value{}, false
	}
	return field, true
}
