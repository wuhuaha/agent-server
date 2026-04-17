package voice

import (
	"context"
	"testing"
	"time"

	"agent-server/internal/agent"
)

type staticSemanticChatModel struct {
	text string
}

func (m staticSemanticChatModel) Complete(context.Context, agent.ChatModelRequest) (agent.ChatModelResponse, error) {
	return agent.ChatModelResponse{Text: m.text}, nil
}

func TestLLMSemanticTurnJudgeDecodesJSONResponse(t *testing.T) {
	judge := NewLLMSemanticTurnJudge(staticSemanticChatModel{
		text: "```json\n{\"utterance_status\":\"complete\",\"interruption_intent\":\"takeover\",\"task_family\":\"structured_command\",\"slot_readiness_hint\":\"ready\",\"confidence\":0.91,\"reason\":\"new_request_takeover\"}\n```",
	})
	result, err := judge.JudgePreview(context.Background(), SemanticTurnRequest{
		SessionID:   "sess_semantic",
		PartialText: "不要这个",
	})
	if err != nil {
		t.Fatalf("JudgePreview failed: %v", err)
	}
	if result.UtteranceStatus != SemanticUtteranceComplete {
		t.Fatalf("expected complete status, got %+v", result)
	}
	if result.InterruptionIntent != SemanticIntentTakeover {
		t.Fatalf("expected takeover intent, got %+v", result)
	}
	if result.TaskFamily != SemanticTaskFamilyStructuredCommand {
		t.Fatalf("expected structured command task family, got %+v", result)
	}
	if result.SlotReadinessHint != SemanticSlotReadinessReady {
		t.Fatalf("expected slot readiness hint ready, got %+v", result)
	}
	if result.Confidence < 0.9 {
		t.Fatalf("expected high confidence, got %+v", result)
	}
}

func TestMergeSemanticJudgementPromotesDraftAllowed(t *testing.T) {
	snapshot := InputPreview{
		PartialText:       "明天周几",
		StablePrefix:      "明天周几",
		UtteranceComplete: false,
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageWaitForMore,
			PrewarmAllowed: false,
			DraftAllowed:   false,
		},
	}
	merged := mergeSemanticJudgement(snapshot, SemanticTurnJudgement{
		CandidateKey:       semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		UtteranceStatus:    SemanticUtteranceComplete,
		InterruptionIntent: SemanticIntentQuestion,
		TaskFamily:         SemanticTaskFamilyKnowledgeQuery,
		SlotReadinessHint:  SemanticSlotReadinessNotApplicable,
		Confidence:         0.86,
		Reason:             "standalone_question",
		Source:             "test",
	})
	if !merged.UtteranceComplete {
		t.Fatalf("expected semantic merge to promote utterance complete, got %+v", merged)
	}
	if merged.Arbitration.Stage != TurnArbitrationStageDraftAllowed {
		t.Fatalf("expected draft_allowed after semantic promotion, got %+v", merged.Arbitration)
	}
	if !merged.Arbitration.DraftAllowed || !merged.Arbitration.PrewarmAllowed {
		t.Fatalf("expected semantic merge to allow prewarm and draft, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.TaskFamily != SemanticTaskFamilyKnowledgeQuery {
		t.Fatalf("expected knowledge_query task family, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.SemanticSlotReadiness != SemanticSlotReadinessNotApplicable {
		t.Fatalf("expected semantic slot readiness hint to propagate, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticJudgementKeepsStructuredCommandAtPrewarmUntilSlotsArrive(t *testing.T) {
	snapshot := InputPreview{
		PartialText:       "打开客厅灯",
		StablePrefix:      "打开客厅灯",
		UtteranceComplete: false,
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageWaitForMore,
			PrewarmAllowed: false,
			DraftAllowed:   false,
		},
	}
	merged := mergeSemanticJudgement(snapshot, SemanticTurnJudgement{
		CandidateKey:       semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		UtteranceStatus:    SemanticUtteranceComplete,
		InterruptionIntent: SemanticIntentRequest,
		TaskFamily:         SemanticTaskFamilyStructuredCommand,
		SlotReadinessHint:  SemanticSlotReadinessWaitSlot,
		Confidence:         0.88,
		Reason:             "device_control_complete",
		Source:             "test",
	})
	if !merged.Arbitration.PrewarmAllowed {
		t.Fatalf("expected structured command to allow prewarm, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.DraftAllowed {
		t.Fatalf("expected structured command to wait for slot guard before draft, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.TaskFamily != SemanticTaskFamilyStructuredCommand || !merged.Arbitration.SlotConstraintRequired {
		t.Fatalf("expected structured command task family with slot guard, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.SemanticSlotReadiness != SemanticSlotReadinessWaitSlot {
		t.Fatalf("expected semantic slot readiness wait_slot, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticJudgementShortensEffectiveWaitForCompleteQuestion(t *testing.T) {
	snapshot := InputPreview{
		PartialText:       "明天周几",
		StablePrefix:      "明天周几",
		UtteranceComplete: false,
		Arbitration: TurnArbitration{
			Stage:           TurnArbitrationStageWaitForMore,
			AudioMs:         320,
			SilenceMs:       260,
			MinAudioMs:      100,
			BaseWaitMs:      360,
			EffectiveWaitMs: 360,
		},
	}
	merged := mergeSemanticJudgement(snapshot, SemanticTurnJudgement{
		CandidateKey:       semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		UtteranceStatus:    SemanticUtteranceComplete,
		InterruptionIntent: SemanticIntentQuestion,
		TaskFamily:         SemanticTaskFamilyKnowledgeQuery,
		SlotReadinessHint:  SemanticSlotReadinessNotApplicable,
		DynamicWaitPolicy:  SemanticWaitPolicyShorten,
		WaitDeltaMs:        -140,
		Confidence:         0.9,
		Reason:             "standalone_question",
		Source:             "test",
	})
	if merged.Arbitration.SemanticWaitDeltaMs != -140 {
		t.Fatalf("expected semantic wait delta to propagate, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.EffectiveWaitMs != 220 {
		t.Fatalf("expected effective wait 220ms after semantic shortening, got %+v", merged.Arbitration)
	}
	if !merged.Arbitration.AcceptCandidate || !merged.Arbitration.AcceptNow {
		t.Fatalf("expected shortened wait to allow accept immediately, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticJudgementKeepsCorrectionPendingConservative(t *testing.T) {
	snapshot := InputPreview{
		PartialText:       "打开客厅灯不对",
		StablePrefix:      "打开客厅灯",
		UtteranceComplete: true,
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageDraftAllowed,
			PrewarmAllowed: true,
			DraftAllowed:   true,
		},
	}
	merged := mergeSemanticJudgement(snapshot, SemanticTurnJudgement{
		CandidateKey:       semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		UtteranceStatus:    SemanticUtteranceCorrection,
		InterruptionIntent: SemanticIntentCorrection,
		TaskFamily:         SemanticTaskFamilyCorrection,
		SlotReadinessHint:  SemanticSlotReadinessUnknown,
		Confidence:         0.82,
		Reason:             "repair_in_progress",
		Source:             "test",
	})
	if merged.UtteranceComplete {
		t.Fatalf("expected correction judgement to keep utterance incomplete, got %+v", merged)
	}
	if merged.Arbitration.DraftAllowed {
		t.Fatalf("expected correction judgement to suppress draft, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.SemanticWaitDeltaMs <= 0 {
		t.Fatalf("expected correction judgement to extend wait, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticJudgementSemanticFamilyCanOverrideLexicalSlotGuard(t *testing.T) {
	snapshot := InputPreview{
		PartialText:       "帮我看看明天上海天气",
		StablePrefix:      "帮我看看明天上海天气",
		UtteranceComplete: false,
		Arbitration: TurnArbitration{
			Stage:                  TurnArbitrationStageWaitForMore,
			TaskFamily:             SemanticTaskFamilyStructuredCommand,
			SlotConstraintRequired: true,
		},
	}
	merged := mergeSemanticJudgement(snapshot, SemanticTurnJudgement{
		CandidateKey:       semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		UtteranceStatus:    SemanticUtteranceComplete,
		InterruptionIntent: SemanticIntentQuestion,
		TaskFamily:         SemanticTaskFamilyKnowledgeQuery,
		SlotReadinessHint:  SemanticSlotReadinessNotApplicable,
		Confidence:         0.9,
		Reason:             "weather_query_complete",
		Source:             "test",
	})
	if merged.Arbitration.TaskFamily != SemanticTaskFamilyKnowledgeQuery {
		t.Fatalf("expected semantic family to override lexical floor, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.SlotConstraintRequired {
		t.Fatalf("expected semantic family override to remove slot constraint, got %+v", merged.Arbitration)
	}
	if !merged.Arbitration.DraftAllowed {
		t.Fatalf("expected knowledge query semantic judgement to allow draft, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticJudgementClarifyHintCanPromoteDraftBeforeSlotParser(t *testing.T) {
	snapshot := InputPreview{
		PartialText:       "把灯调到舒服一点",
		StablePrefix:      "把灯调到舒服一点",
		UtteranceComplete: false,
		Arbitration: TurnArbitration{
			Stage:                  TurnArbitrationStageWaitForMore,
			TaskFamily:             SemanticTaskFamilyStructuredCommand,
			SlotConstraintRequired: true,
			PrewarmAllowed:         false,
			DraftAllowed:           false,
		},
	}
	merged := mergeSemanticJudgement(snapshot, SemanticTurnJudgement{
		CandidateKey:       semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		UtteranceStatus:    SemanticUtteranceComplete,
		InterruptionIntent: SemanticIntentRequest,
		TaskFamily:         SemanticTaskFamilyStructuredCommand,
		SlotReadinessHint:  SemanticSlotReadinessClarify,
		Confidence:         0.87,
		Reason:             "ambiguous_brightness_need_clarify",
		Source:             "test",
	})
	if !merged.Arbitration.DraftAllowed || !merged.Arbitration.SlotClarifyNeeded {
		t.Fatalf("expected clarify hint to promote draft and clarify-needed, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.SemanticSlotReadiness != SemanticSlotReadinessClarify {
		t.Fatalf("expected clarify semantic slot readiness hint, got %+v", merged.Arbitration)
	}
}

func TestLLMSemanticSlotParserDecodesJSONResponse(t *testing.T) {
	parser := NewLLMSemanticSlotParser(staticSemanticChatModel{
		text: "```json\n{\"domain\":\"smart_home\",\"task_family\":\"structured_command\",\"intent\":\"device_control\",\"slot_status\":\"complete\",\"actionability\":\"act_candidate\",\"clarify_needed\":false,\"missing_slots\":[],\"ambiguous_slots\":[],\"confidence\":0.88,\"reason\":\"required_slots_grounded\"}\n```",
	})
	result, err := parser.ParsePreview(context.Background(), SemanticSlotParseRequest{
		SessionID:   "sess_slot",
		PartialText: "打开客厅灯",
	})
	if err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}
	if result.Domain != SemanticSlotDomainSmartHome {
		t.Fatalf("expected smart_home domain, got %+v", result)
	}
	if result.TaskFamily != SemanticTaskFamilyStructuredCommand {
		t.Fatalf("expected structured_command task family, got %+v", result)
	}
	if result.Intent != "device_control" {
		t.Fatalf("expected device_control intent, got %+v", result)
	}
	if result.SlotStatus != SemanticSlotStatusComplete || result.Actionability != SemanticSlotActionabilityActCandidate {
		t.Fatalf("expected complete act_candidate, got %+v", result)
	}
}

func TestMergeSemanticSlotParsePromotesClarifyDraft(t *testing.T) {
	snapshot := InputPreview{
		PartialText:  "把灯调亮一点",
		StablePrefix: "把灯调亮一点",
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageWaitForMore,
			PrewarmAllowed: false,
			DraftAllowed:   false,
		},
	}
	merged := mergeSemanticSlotParse(snapshot, SemanticSlotParseResult{
		CandidateKey:  semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "set_attribute",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityClarifyNeeded,
		ClarifyNeeded: true,
		MissingSlots:  []string{"target"},
		Confidence:    0.84,
		Reason:        "missing_target_need_clarify",
		Source:        "test",
	})
	if merged.Arbitration.Stage != TurnArbitrationStageDraftAllowed {
		t.Fatalf("expected clarify-needed slot parse to promote draft_allowed, got %+v", merged.Arbitration)
	}
	if !merged.Arbitration.DraftAllowed || !merged.Arbitration.SlotClarifyNeeded {
		t.Fatalf("expected draft + clarify flags, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.TaskFamily != SemanticTaskFamilyStructuredCommand || !merged.Arbitration.SlotConstraintRequired {
		t.Fatalf("expected structured command task family to propagate, got %+v", merged.Arbitration)
	}
	if got := merged.Arbitration.SlotMissing; len(got) != 1 || got[0] != "target" {
		t.Fatalf("expected missing target slot, got %+v", got)
	}
}

func TestMergeSemanticSlotParsePropagatesGroundedCanonicalSummary(t *testing.T) {
	snapshot := InputPreview{
		PartialText:  "打开客厅灯",
		StablePrefix: "打开客厅灯",
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageWaitForMore,
			PrewarmAllowed: false,
			DraftAllowed:   false,
		},
	}
	merged := mergeSemanticSlotParse(snapshot, SemanticSlotParseResult{
		CandidateKey:      semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		Domain:            SemanticSlotDomainSmartHome,
		Intent:            "device_control",
		SlotStatus:        SemanticSlotStatusComplete,
		Actionability:     SemanticSlotActionabilityActCandidate,
		Grounded:          true,
		CanonicalTarget:   "客厅灯",
		CanonicalLocation: "客厅",
		Confidence:        0.88,
		Reason:            "catalog_target_grounded",
		Source:            "test",
	})
	if !merged.Arbitration.SlotGrounded {
		t.Fatalf("expected grounded summary to propagate, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.SlotCanonicalTarget != "客厅灯" || merged.Arbitration.SlotCanonicalLocation != "客厅" {
		t.Fatalf("expected canonical summaries to propagate, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticSlotParsePropagatesNormalizedValueAndRiskSummary(t *testing.T) {
	snapshot := InputPreview{
		PartialText:  "把门锁打开",
		StablePrefix: "把门锁打开",
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageWaitForMore,
			PrewarmAllowed: false,
			DraftAllowed:   false,
		},
	}
	merged := mergeSemanticSlotParse(snapshot, SemanticSlotParseResult{
		CandidateKey:        semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		Domain:              SemanticSlotDomainSmartHome,
		Intent:              "device_control",
		SlotStatus:          SemanticSlotStatusComplete,
		Actionability:       SemanticSlotActionabilityClarifyNeeded,
		Grounded:            true,
		CanonicalTarget:     "入户门锁",
		RiskLevel:           SemanticRiskLevelHigh,
		RiskReason:          "catalog_high_risk_target",
		RiskConfirmRequired: true,
		Confidence:          0.92,
		Reason:              "catalog_high_risk_target",
		Source:              "test",
	})
	if merged.Arbitration.SlotRiskLevel != SemanticRiskLevelHigh || !merged.Arbitration.SlotRiskConfirmRequired {
		t.Fatalf("expected risk summary to propagate, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.SlotRiskReason != "catalog_high_risk_target" {
		t.Fatalf("expected risk reason to propagate, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticSlotParseCanPullDraftBackToWaitMore(t *testing.T) {
	snapshot := InputPreview{
		PartialText:  "把客厅灯",
		StablePrefix: "把客厅灯",
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageDraftAllowed,
			PrewarmAllowed: true,
			DraftAllowed:   true,
		},
	}
	merged := mergeSemanticSlotParse(snapshot, SemanticSlotParseResult{
		CandidateKey:  semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "device_control",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityWaitMore,
		Confidence:    0.82,
		Reason:        "tail_slot_incomplete",
		Source:        "test",
	})
	if merged.Arbitration.DraftAllowed {
		t.Fatalf("expected slot wait_more to suppress draft, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.Stage != TurnArbitrationStagePrewarmAllowed {
		t.Fatalf("expected slot wait_more to keep only prewarm_allowed, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticSlotParseAddsSlotGuardDelay(t *testing.T) {
	snapshot := InputPreview{
		PartialText:  "把客厅灯",
		StablePrefix: "把客厅灯",
		Arbitration: TurnArbitration{
			Stage:           TurnArbitrationStageWaitForMore,
			AudioMs:         320,
			SilenceMs:       360,
			MinAudioMs:      100,
			BaseWaitMs:      300,
			EffectiveWaitMs: 300,
			PrewarmAllowed:  true,
		},
	}
	merged := mergeSemanticSlotParse(snapshot, SemanticSlotParseResult{
		CandidateKey:  semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "device_control",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityWaitMore,
		Confidence:    0.84,
		Reason:        "tail_slot_incomplete",
		Source:        "test",
	})
	if merged.Arbitration.SlotGuardAdjustMs <= 0 {
		t.Fatalf("expected slot wait_more to extend wait, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.EffectiveWaitMs <= merged.Arbitration.BaseWaitMs {
		t.Fatalf("expected slot guard to raise effective wait, got %+v", merged.Arbitration)
	}
	if merged.Arbitration.AcceptCandidate || merged.Arbitration.AcceptNow {
		t.Fatalf("expected slot guard to keep preview conservative, got %+v", merged.Arbitration)
	}
}

func TestShouldParseSemanticSlotsLaunchesEarlyForStructuredCommands(t *testing.T) {
	snapshot := InputPreview{
		PartialText:  "打开客厅灯",
		StablePrefix: "打开客厅灯",
		Arbitration: TurnArbitration{
			AudioMs:                320,
			StableForMs:            40,
			CandidateReady:         true,
			TaskFamily:             SemanticTaskFamilyStructuredCommand,
			SlotConstraintRequired: true,
		},
	}
	if !shouldParseSemanticSlots(snapshot, 2, 160*time.Millisecond) {
		t.Fatalf("expected structured command candidate to launch slot parser early, got %+v", snapshot.Arbitration)
	}
}

func TestShouldJudgeSemanticLaunchesForCandidateReadyKnowledgeQuery(t *testing.T) {
	snapshot := InputPreview{
		PartialText:  "帮我看看明天上海天气",
		StablePrefix: "帮我看看明天上海天气",
		Arbitration: TurnArbitration{
			AudioMs:        360,
			StableForMs:    40,
			CandidateReady: true,
			TaskFamily:     SemanticTaskFamilyKnowledgeQuery,
		},
	}
	if !shouldJudgeSemantic(snapshot, 4, 160*time.Millisecond) {
		t.Fatalf("expected candidate-ready knowledge query to launch semantic judge early, got %+v", snapshot.Arbitration)
	}
}
