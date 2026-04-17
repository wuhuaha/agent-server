package voice

import "testing"

func TestEntityCatalogGrounderPromotesUniqueSmartHomeTarget(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	result := grounder.GroundPreview(SemanticSlotParseRequest{
		PartialText:  "打开客厅灯",
		StablePrefix: "打开客厅灯",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "device_control",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityClarifyNeeded,
		ClarifyNeeded: true,
		MissingSlots:  []string{"target"},
		Confidence:    0.86,
		Reason:        "missing_target_need_clarify",
		Source:        "test",
	})

	if !result.Grounded {
		t.Fatalf("expected result to be grounded, got %+v", result)
	}
	if result.CanonicalTarget != "客厅灯" {
		t.Fatalf("expected canonical target 客厅灯, got %+v", result)
	}
	if result.CanonicalLocation != "客厅" {
		t.Fatalf("expected canonical location 客厅, got %+v", result)
	}
	if result.SlotStatus != SemanticSlotStatusComplete {
		t.Fatalf("expected grounded result to become complete, got %+v", result)
	}
	if result.Actionability != SemanticSlotActionabilityActCandidate || result.ClarifyNeeded {
		t.Fatalf("expected grounded result to become act_candidate without clarify, got %+v", result)
	}
	if len(result.MissingSlots) != 0 {
		t.Fatalf("expected missing target to be cleared, got %+v", result.MissingSlots)
	}
}

func TestEntityCatalogGrounderMarksGenericTargetAmbiguous(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	result := grounder.GroundPreview(SemanticSlotParseRequest{
		PartialText:  "把灯调亮一点",
		StablePrefix: "把灯调亮一点",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "set_attribute",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityDraftOK,
		Confidence:    0.82,
		Reason:        "value_present_target_unclear",
		Source:        "test",
	})

	if result.CanonicalTarget != "" {
		t.Fatalf("expected no canonical target on ambiguous generic request, got %+v", result)
	}
	if result.SlotStatus != SemanticSlotStatusAmbiguous {
		t.Fatalf("expected ambiguous status, got %+v", result)
	}
	if result.Actionability != SemanticSlotActionabilityClarifyNeeded || !result.ClarifyNeeded {
		t.Fatalf("expected clarify_needed actionability, got %+v", result)
	}
	if len(result.AmbiguousSlots) != 1 || result.AmbiguousSlots[0] != "target" {
		t.Fatalf("expected ambiguous target slot, got %+v", result.AmbiguousSlots)
	}
}

func TestEntityCatalogGrounderSupportsDesktopAssistantAlias(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	result := grounder.GroundPreview(SemanticSlotParseRequest{
		PartialText:  "打开 VS Code",
		StablePrefix: "打开 VS Code",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainDesktopAssistant,
		Intent:        "open_app",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityClarifyNeeded,
		ClarifyNeeded: true,
		MissingSlots:  []string{"target_app"},
		Confidence:    0.83,
		Reason:        "missing_target_app",
		Source:        "test",
	})

	if !result.Grounded {
		t.Fatalf("expected desktop result to be grounded, got %+v", result)
	}
	if result.CanonicalTarget != "Visual Studio Code" {
		t.Fatalf("expected VS Code alias to ground, got %+v", result)
	}
	if result.SlotStatus != SemanticSlotStatusComplete || result.Actionability != SemanticSlotActionabilityActCandidate {
		t.Fatalf("expected desktop alias grounding to promote completion, got %+v", result)
	}
	if len(result.MissingSlots) != 0 {
		t.Fatalf("expected target_app to be cleared, got %+v", result.MissingSlots)
	}
}

func TestEntityCatalogGrounderUsesSessionRecentContextToBreakGenericTargetTie(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	sessionID := "sess_recent_context"

	first := grounder.GroundPreview(SemanticSlotParseRequest{
		SessionID:   sessionID,
		PartialText: "打开客厅灯",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "device_control",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityClarifyNeeded,
		MissingSlots:  []string{"target"},
		Confidence:    0.86,
		Reason:        "missing_target_need_clarify",
		Source:        "test",
	})
	if first.CanonicalTarget != "客厅灯" {
		t.Fatalf("expected first command to ground 客厅灯, got %+v", first)
	}

	second := grounder.GroundPreview(SemanticSlotParseRequest{
		SessionID:   sessionID,
		PartialText: "把灯调亮一点",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "set_attribute",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityDraftOK,
		MissingSlots:  []string{"target"},
		Confidence:    0.9,
		Reason:        "missing_target_need_clarify",
		Source:        "test",
	})

	if second.CanonicalTarget != "客厅灯" {
		t.Fatalf("expected recent session context to prefer 客厅灯, got %+v", second)
	}
	if second.SlotStatus != SemanticSlotStatusComplete || second.Actionability != SemanticSlotActionabilityActCandidate {
		t.Fatalf("expected recent-context disambiguation to promote act_candidate, got %+v", second)
	}
}

func TestEntityCatalogGrounderBuildsASRHintsFromRecentContext(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	sessionID := "sess_asr_hints"
	grounder.GroundPreview(SemanticSlotParseRequest{
		SessionID:   sessionID,
		PartialText: "打开客厅灯",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "device_control",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityClarifyNeeded,
		MissingSlots:  []string{"target"},
		Confidence:    0.86,
		Reason:        "missing_target_need_clarify",
		Source:        "test",
	})

	hints := grounder.TranscriptionHintsForSession(sessionID)
	if len(hints.Hotwords) == 0 {
		t.Fatalf("expected non-empty recent-context hotwords, got %+v", hints)
	}
	if hints.Hotwords[0] != "客厅灯" {
		t.Fatalf("expected latest entity to lead hotwords, got %+v", hints)
	}
}

func TestEntityCatalogGrounderNormalizesValueAndClearsMissingValue(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	result := grounder.GroundPreview(SemanticSlotParseRequest{
		SessionID:   "sess_value_normalize",
		PartialText: "把客厅空调调到26度",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "set_temperature",
		SlotStatus:    SemanticSlotStatusPartial,
		Actionability: SemanticSlotActionabilityDraftOK,
		MissingSlots:  []string{"target", "value"},
		Confidence:    0.88,
		Reason:        "missing_target_and_value",
		Source:        "test",
	})

	if result.CanonicalTarget != "客厅空调" {
		t.Fatalf("expected target to ground to 客厅空调, got %+v", result)
	}
	if result.NormalizedValue != "26" || result.NormalizedValueUnit != "temperature_celsius" {
		t.Fatalf("expected normalized temperature value, got %+v", result)
	}
	if len(result.MissingSlots) != 0 {
		t.Fatalf("expected normalized value to clear missing slots, got %+v", result.MissingSlots)
	}
	if result.SlotStatus != SemanticSlotStatusComplete || result.Actionability != SemanticSlotActionabilityActCandidate {
		t.Fatalf("expected normalized value to promote completion, got %+v", result)
	}
}

func TestEntityCatalogGrounderAddsHighRiskConfirmationForDoorLock(t *testing.T) {
	grounder := NewDefaultEntityCatalogGrounder()
	result := grounder.GroundPreview(SemanticSlotParseRequest{
		SessionID:   "sess_risk_lock",
		PartialText: "把门锁打开",
	}, SemanticSlotParseResult{
		Domain:        SemanticSlotDomainSmartHome,
		Intent:        "device_control",
		SlotStatus:    SemanticSlotStatusComplete,
		Actionability: SemanticSlotActionabilityActCandidate,
		Confidence:    0.9,
		Reason:        "required_slots_grounded",
		Source:        "test",
	})

	if result.CanonicalTarget != "入户门锁" {
		t.Fatalf("expected lock target to ground, got %+v", result)
	}
	if result.RiskLevel != SemanticRiskLevelHigh || !result.RiskConfirmRequired {
		t.Fatalf("expected high risk confirmation to be required, got %+v", result)
	}
	if result.Actionability != SemanticSlotActionabilityClarifyNeeded || !result.ClarifyNeeded {
		t.Fatalf("expected high risk action to downgrade to clarify_needed, got %+v", result)
	}
}
