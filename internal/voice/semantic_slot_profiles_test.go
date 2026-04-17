package voice

import (
	"context"
	"strings"
	"testing"

	"agent-server/internal/agent"
)

type captureSemanticSlotPromptModel struct {
	responseText string
	requests     []agent.ChatModelRequest
}

func (m *captureSemanticSlotPromptModel) Complete(_ context.Context, req agent.ChatModelRequest) (agent.ChatModelResponse, error) {
	m.requests = append(m.requests, req)
	return agent.ChatModelResponse{Text: m.responseText}, nil
}

func TestSemanticSlotParserPromptDefaultsStayGeneric(t *testing.T) {
	prompt := semanticSlotParserPrompt(SemanticSlotParseRequest{})
	if !strings.Contains(prompt, "shared runtime 的默认政策中心是 task_family") {
		t.Fatalf("expected generic prompt to emphasize task_family ownership, got %q", prompt)
	}
	if strings.Contains(prompt, "smart_home 关注") {
		t.Fatalf("did not expect shared default prompt to hardcode smart_home slot policy, got %q", prompt)
	}
	if strings.Contains(prompt, "desktop_assistant 关注") {
		t.Fatalf("did not expect shared default prompt to hardcode desktop_assistant slot policy, got %q", prompt)
	}
	if strings.Contains(prompt, "当前请求显式附带了以下补充 hints") {
		t.Fatalf("did not expect generic prompt to include explicit hint block, got %q", prompt)
	}
}

func TestSemanticSlotParserPromptAddsExplicitSeedProfileHints(t *testing.T) {
	prompt := semanticSlotParserPrompt(SemanticSlotParseRequest{
		PromptProfile: BuiltInEntityCatalogProfileSeedCompanion,
	})
	if !strings.Contains(prompt, "profile=seed_companion") {
		t.Fatalf("expected prompt to include explicit seed profile marker, got %q", prompt)
	}
	if !strings.Contains(prompt, "domain 映射到 smart_home") {
		t.Fatalf("expected prompt to include smart_home hint only under explicit profile, got %q", prompt)
	}
	if !strings.Contains(prompt, "domain 映射到 desktop_assistant") {
		t.Fatalf("expected prompt to include desktop_assistant hint only under explicit profile, got %q", prompt)
	}
}

func TestSemanticSlotParserPromptMergesExplicitProfileAndCustomHints(t *testing.T) {
	prompt := semanticSlotParserPrompt(SemanticSlotParseRequest{
		PromptProfile: BuiltInEntityCatalogProfileSeedCompanion,
		PromptHints: []string{
			"把 projector / 投影 / 幕布 相关目标优先视为 structured_command 的 target 提示。",
			"把 projector / 投影 / 幕布 相关目标优先视为 structured_command 的 target 提示。",
		},
	})
	if strings.Count(prompt, "projector / 投影 / 幕布") != 1 {
		t.Fatalf("expected explicit custom hints to dedupe, got %q", prompt)
	}
	if !strings.Contains(prompt, "只用于补强当前轮次") {
		t.Fatalf("expected explicit hint block disclaimer, got %q", prompt)
	}
}

func TestLLMSemanticSlotParserUsesGenericPromptByDefault(t *testing.T) {
	model := &captureSemanticSlotPromptModel{
		responseText: "{\"domain\":\"unknown\",\"task_family\":\"knowledge_query\",\"intent\":\"calendar_query\",\"slot_status\":\"not_applicable\",\"actionability\":\"draft_ok\",\"clarify_needed\":false,\"missing_slots\":[],\"ambiguous_slots\":[],\"confidence\":0.78,\"reason\":\"generic_question\"}",
	}
	parser := NewLLMSemanticSlotParser(model)
	if _, err := parser.ParsePreview(context.Background(), SemanticSlotParseRequest{
		SessionID:   "sess_generic_prompt",
		PartialText: "明天周几",
	}); err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected one captured request, got %d", len(model.requests))
	}
	prompt := model.requests[0].SystemPrompt
	if !strings.Contains(prompt, "shared runtime 的默认政策中心是 task_family") {
		t.Fatalf("expected generic system prompt, got %q", prompt)
	}
	if strings.Contains(prompt, "profile=seed_companion") {
		t.Fatalf("did not expect generic parse path to include seed profile hints, got %q", prompt)
	}
}

func TestProfileAwareSemanticSlotParserInjectsExplicitHints(t *testing.T) {
	model := &captureSemanticSlotPromptModel{
		responseText: "{\"domain\":\"smart_home\",\"task_family\":\"structured_command\",\"intent\":\"device_control\",\"slot_status\":\"complete\",\"actionability\":\"act_candidate\",\"clarify_needed\":false,\"missing_slots\":[],\"ambiguous_slots\":[],\"confidence\":0.88,\"reason\":\"required_slots_grounded\"}",
	}
	parser := NewProfileAwareSemanticSlotParser(
		NewLLMSemanticSlotParser(model),
		BuiltInEntityCatalogProfileSeedCompanion,
		"把主灯 / 客厅灯 / 吊灯 相关别名优先视为 target 提示。",
	)
	if _, err := parser.ParsePreview(context.Background(), SemanticSlotParseRequest{
		SessionID:   "sess_seed_profile",
		PartialText: "打开客厅主灯",
	}); err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected one captured request, got %d", len(model.requests))
	}
	prompt := model.requests[0].SystemPrompt
	if !strings.Contains(prompt, "profile=seed_companion") {
		t.Fatalf("expected explicit seed profile hints in wrapped parser prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "主灯 / 客厅灯 / 吊灯") {
		t.Fatalf("expected custom prompt hint to propagate, got %q", prompt)
	}
	if !strings.Contains(prompt, "不代表 shared runtime 默认场景") {
		t.Fatalf("expected wrapped parser prompt to preserve generic-default disclaimer, got %q", prompt)
	}
}
