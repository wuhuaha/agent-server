package voice

import "testing"

func TestPostProcessSemanticSlotResultDoesNotInferRiskFromLexicalBusinessTerms(t *testing.T) {
	result := postProcessSemanticSlotResult(
		SemanticSlotParseRequest{
			PartialText: "删除这个文件",
		},
		SemanticSlotParseResult{
			Domain:          SemanticSlotDomainDesktopAssistant,
			Intent:          "delete_file",
			SlotStatus:      SemanticSlotStatusComplete,
			Actionability:   SemanticSlotActionabilityActCandidate,
			ClarifyNeeded:   false,
			Confidence:      0.92,
			Reason:          "required_slots_grounded",
			Source:          "test",
			CanonicalTarget: "report.txt",
		},
	)

	if result.RiskLevel != "" && result.RiskLevel != SemanticRiskLevelUnknown {
		t.Fatalf("expected no lexical-only risk inference, got %+v", result)
	}
	if result.RiskConfirmRequired {
		t.Fatalf("expected lexical-only term not to force confirm, got %+v", result)
	}
	if result.Actionability != SemanticSlotActionabilityActCandidate {
		t.Fatalf("expected actionability to stay unchanged without abstract risk tag, got %+v", result)
	}
}

func TestPostProcessSemanticSlotResultUsesAnnotatedRiskLevelForGenericGating(t *testing.T) {
	result := postProcessSemanticSlotResult(
		SemanticSlotParseRequest{
			PartialText: "执行目标操作",
		},
		SemanticSlotParseResult{
			Domain:          SemanticSlotDomainUnknown,
			Intent:          "execute_target",
			SlotStatus:      SemanticSlotStatusComplete,
			Actionability:   SemanticSlotActionabilityActCandidate,
			Confidence:      0.93,
			Reason:          "required_slots_grounded",
			Source:          "test",
			RiskLevel:       SemanticRiskLevelHigh,
			CanonicalTarget: "target",
		},
	)

	if result.RiskLevel != SemanticRiskLevelHigh {
		t.Fatalf("expected annotated high risk to survive, got %+v", result)
	}
	if !result.RiskConfirmRequired || !result.ClarifyNeeded {
		t.Fatalf("expected annotated high risk to require confirmation, got %+v", result)
	}
	if result.Actionability != SemanticSlotActionabilityClarifyNeeded {
		t.Fatalf("expected annotated high risk to downgrade actionability, got %+v", result)
	}
}
