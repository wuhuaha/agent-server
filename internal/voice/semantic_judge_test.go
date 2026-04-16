package voice

import (
	"context"
	"testing"

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
		text: "```json\n{\"utterance_status\":\"complete\",\"interruption_intent\":\"takeover\",\"confidence\":0.91,\"reason\":\"new_request_takeover\"}\n```",
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
}

func TestLLMSemanticSlotParserDecodesJSONResponse(t *testing.T) {
	parser := NewLLMSemanticSlotParser(staticSemanticChatModel{
		text: "```json\n{\"domain\":\"smart_home\",\"intent\":\"device_control\",\"slot_status\":\"complete\",\"actionability\":\"act_candidate\",\"clarify_needed\":false,\"missing_slots\":[],\"ambiguous_slots\":[],\"confidence\":0.88,\"reason\":\"required_slots_grounded\"}\n```",
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
	if got := merged.Arbitration.SlotMissing; len(got) != 1 || got[0] != "target" {
		t.Fatalf("expected missing target slot, got %+v", got)
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
