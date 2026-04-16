package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingChatModel struct {
	requests []ChatModelRequest
	response ChatModelResponse
	err      error
}

func (m *recordingChatModel) Complete(_ context.Context, request ChatModelRequest) (ChatModelResponse, error) {
	m.requests = append(m.requests, request)
	if m.err != nil {
		return ChatModelResponse{}, m.err
	}
	return m.response, nil
}

type scriptedChatModel struct {
	requests  []ChatModelRequest
	responses []ChatModelResponse
	err       error
}

func (m *scriptedChatModel) Complete(_ context.Context, request ChatModelRequest) (ChatModelResponse, error) {
	m.requests = append(m.requests, request)
	if m.err != nil {
		return ChatModelResponse{}, m.err
	}
	if len(m.responses) == 0 {
		return ChatModelResponse{}, io.EOF
	}
	response := m.responses[0]
	m.responses = append([]ChatModelResponse(nil), m.responses[1:]...)
	return response, nil
}

type scriptedStreamingChatStep struct {
	chunks   []string
	response ChatModelResponse
	err      error
}

type scriptedStreamingChatModel struct {
	requests []ChatModelRequest
	steps    []scriptedStreamingChatStep
}

func (m *scriptedStreamingChatModel) Complete(_ context.Context, request ChatModelRequest) (ChatModelResponse, error) {
	m.requests = append(m.requests, request)
	if len(m.steps) == 0 {
		return ChatModelResponse{}, io.EOF
	}
	step := m.steps[0]
	m.steps = append([]scriptedStreamingChatStep(nil), m.steps[1:]...)
	if step.err != nil {
		return ChatModelResponse{}, step.err
	}
	return step.response, nil
}

func (m *scriptedStreamingChatModel) Stream(ctx context.Context, request ChatModelRequest, sink ChatModelDeltaSink) (ChatModelResponse, error) {
	m.requests = append(m.requests, request)
	if len(m.steps) == 0 {
		return ChatModelResponse{}, io.EOF
	}
	step := m.steps[0]
	m.steps = append([]scriptedStreamingChatStep(nil), m.steps[1:]...)
	for _, chunk := range step.chunks {
		if err := sink.EmitChatModelDelta(ctx, ChatModelDelta{Text: chunk}); err != nil {
			return ChatModelResponse{}, err
		}
	}
	if step.err != nil {
		return ChatModelResponse{}, step.err
	}
	return step.response, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type staticPromptSectionProvider struct {
	sections []PromptSection
}

func (p staticPromptSectionProvider) ListPromptSections(context.Context, PromptSectionRequest) ([]PromptSection, error) {
	return append([]PromptSection(nil), p.sections...), nil
}

type countingPromptSectionProvider struct {
	sections []PromptSection
	calls    int
}

func (p *countingPromptSectionProvider) ListPromptSections(context.Context, PromptSectionRequest) ([]PromptSection, error) {
	p.calls++
	return append([]PromptSection(nil), p.sections...), nil
}

type countingToolRegistry struct {
	tools []ToolDefinition
	calls int
}

func (r *countingToolRegistry) ListTools(context.Context, ToolCatalogRequest) ([]ToolDefinition, error) {
	r.calls++
	return append([]ToolDefinition(nil), r.tools...), nil
}

func TestLLMTurnExecutorUsesModelAndMemoryContext(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "你好，我是 DeepSeek。"},
	}
	executor := NewLLMTurnExecutor(model).
		WithMemoryStore(memoryStore).
		WithAssistantName("星幕").
		WithExecutionMode("live_control").
		WithSystemPrompt("请用简洁中文回答。")

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-1",
		DeviceID:   "rtos-1",
		ClientType: "xiaozhi-compat",
		UserText:   "早上好",
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if output.Text != "你好，我是 DeepSeek。" {
		t.Fatalf("unexpected output text %q", output.Text)
	}
	if len(output.Deltas) != 1 || output.Deltas[0].Kind != TurnDeltaKindText {
		t.Fatalf("expected one text delta, got %+v", output.Deltas)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	if got := model.requests[0].SystemPrompt; !strings.Contains(got, "请用简洁中文回答。") {
		t.Fatalf("expected custom prompt prefix, got %q", got)
	}
	if got := model.requests[0].SystemPrompt; !strings.Contains(got, "当前本地时间：") {
		t.Fatalf("expected prompt to include local time context, got %q", got)
	}
	if got := model.requests[0].SystemPrompt; !strings.Contains(got, "当前执行模式：live_control") {
		t.Fatalf("expected live_control policy, got %q", got)
	}
	if got := model.requests[0].MemoryContext.Summary; got != "bootstrap" {
		t.Fatalf("unexpected memory summary %q", got)
	}
	if len(memoryStore.saveRecords) != 1 {
		t.Fatalf("expected one memory save, got %d", len(memoryStore.saveRecords))
	}
	if got := memoryStore.saveRecords[0].ResponseText; got != output.Text {
		t.Fatalf("expected saved response text %q, got %q", output.Text, got)
	}
}

func TestLLMTurnExecutorInjectsRecentMessagesBeforeCurrentUser(t *testing.T) {
	memoryStore := &recordingMemoryStore{
		loadContext: MemoryContext{
			Scope:   "user alice",
			Summary: "remembered recent user context",
			RecentMessages: []ChatMessage{
				{Role: "user", Content: "把客厅灯调暗一点"},
				{Role: "assistant", Content: "好的，已经把客厅灯调暗了一些。"},
			},
		},
	}
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "现在已经切到观影氛围。"},
	}
	executor := NewLLMTurnExecutor(model).WithMemoryStore(memoryStore)

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-2",
		DeviceID:   "panel-1",
		ClientType: "xiaozhi-compat",
		UserText:   "再柔和一点",
		Metadata: map[string]string{
			"user_id": "alice",
		},
	}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	messages := model.requests[0].Messages
	if len(messages) != 5 {
		t.Fatalf("expected system, memory, 2 recent messages, and current user; got %+v", messages)
	}
	if messages[2].Role != "user" || messages[2].Content != "把客厅灯调暗一点" {
		t.Fatalf("unexpected first recent message %+v", messages[2])
	}
	if messages[3].Role != "assistant" || messages[3].Content != "好的，已经把客厅灯调暗了一些。" {
		t.Fatalf("unexpected second recent message %+v", messages[3])
	}
	if messages[4].Role != "user" || messages[4].Content != "再柔和一点" {
		t.Fatalf("expected current user message last, got %+v", messages[4])
	}
	if got := memoryStore.loadQueries[0].UserID; got != "alice" {
		t.Fatalf("expected user-scoped memory query, got %q", got)
	}
}

func TestLLMTurnExecutorInjectsPreviousPlaybackContextIntoSystemPrompt(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "继续说明。"},
	}
	executor := NewLLMTurnExecutor(model).WithMemoryStore(&recordingMemoryStore{})

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-prev-playback",
		DeviceID:   "panel-1",
		ClientType: "rtos",
		UserText:   "继续，但简短一点",
		Metadata: map[string]string{
			"voice.previous.available":            "true",
			"voice.previous.heard_text":           "好的，已经为你打开客厅灯，",
			"voice.previous.missed_text":          "现在把亮度调到了最舒适的模式。",
			"voice.previous.resume_anchor":        "好的，已经为你打开客厅灯，",
			"voice.previous.response_interrupted": "true",
			"voice.previous.response_truncated":   "true",
			"voice.previous.heard_confidence":     "medium",
			"voice.previous.interruption_policy":  "hard_interrupt",
			"voice.previous.interruption_reason":  "client_barge_in_after_mark",
			"voice.previous.heard_precision_tier": "tier1_segment_mark",
			"voice.previous.heard_boundary":       "prefix",
			"voice.previous.heard_ratio_pct":      "46",
			"voice.previous.playback_completed":   "false",
		},
	}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}

	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	prompt := model.requests[0].SystemPrompt
	for _, want := range []string{
		"上一轮语音播报上下文：",
		"上一轮回复没有被用户完整听到",
		"用户实际已经听到的大致边界：好的，已经为你打开客厅灯，",
		"用户大概率还没听到的剩余部分：现在把亮度调到了最舒适的模式。",
		"若用户说“继续”“后面呢”“刚刚最后一句”",
		"若用户只是要你继续，优先续接未播出的剩余部分",
		"heard_text / resume_anchor 可能来自播放 ACK 与分段边界事实",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected system prompt to contain %q, got %q", want, prompt)
		}
	}
}

func TestLLMTurnExecutorBypassesModelForDeterministicContinueFollowUp(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "不应该调用模型"},
	}
	executor := NewLLMTurnExecutor(model).WithMemoryStore(memoryStore)

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-follow-up-direct",
		DeviceID:   "panel-1",
		ClientType: "rtos",
		UserText:   "接着说",
		Metadata: map[string]string{
			"voice.previous.available":            "true",
			"voice.previous.heard_text":           "好的，已经为你打开客厅灯，",
			"voice.previous.missed_text":          "现在把亮度调到了最舒适的模式。",
			"voice.previous.resume_anchor":        "好的，已经为你打开客厅灯，",
			"voice.previous.response_interrupted": "true",
			"voice.previous.response_truncated":   "true",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}

	if got := output.Text; got != "现在把亮度调到了最舒适的模式。" {
		t.Fatalf("expected deterministic missed-tail reply, got %q", got)
	}
	if len(output.Deltas) != 1 || output.Deltas[0].Text != "现在把亮度调到了最舒适的模式。" {
		t.Fatalf("expected one deterministic text delta, got %+v", output.Deltas)
	}
	if len(model.requests) != 0 {
		t.Fatalf("expected exact continue follow-up to bypass model, got %d request(s)", len(model.requests))
	}
	if len(memoryStore.saveRecords) != 1 {
		t.Fatalf("expected one memory save, got %d", len(memoryStore.saveRecords))
	}
	if got := memoryStore.saveRecords[0].ResponseText; got != "现在把亮度调到了最舒适的模式。" {
		t.Fatalf("expected saved deterministic reply, got %q", got)
	}
}

func TestLLMTurnExecutorAddsPlaybackFollowUpRuntimeHintForLooseContinue(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "继续说明。"},
	}
	executor := NewLLMTurnExecutor(model).WithMemoryStore(&recordingMemoryStore{})

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-follow-up",
		DeviceID:   "panel-1",
		ClientType: "rtos",
		UserText:   "继续，但简短一点",
		Metadata: map[string]string{
			"voice.previous.available":            "true",
			"voice.previous.heard_text":           "好的，已经为你打开客厅灯，",
			"voice.previous.missed_text":          "现在把亮度调到了最舒适的模式。",
			"voice.previous.resume_anchor":        "好的，已经为你打开客厅灯，",
			"voice.previous.response_interrupted": "true",
			"voice.previous.response_truncated":   "true",
		},
	}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}

	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	messages := model.requests[0].Messages
	if len(messages) < 4 {
		t.Fatalf("expected system + memory + runtime hint + user messages, got %+v", messages)
	}
	found := false
	for _, message := range messages {
		if message.Role != "system" {
			continue
		}
		if strings.Contains(message.Content, "Runtime voice follow-up hint:") &&
			strings.Contains(message.Content, "canonical continuation") &&
			strings.Contains(message.Content, "Canonical continuation text: 现在把亮度调到了最舒适的模式。") &&
			strings.Contains(message.Content, "Avoid repeating the heard boundary") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected runtime follow-up system hint, got %+v", messages)
	}
}

func TestLLMTurnExecutorSkipsPlaybackFollowUpHintForUnrelatedQuestion(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "明天是星期五。"},
	}
	executor := NewLLMTurnExecutor(model).WithMemoryStore(&recordingMemoryStore{})

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-unrelated",
		DeviceID:   "panel-1",
		ClientType: "rtos",
		UserText:   "明天周几",
		Metadata: map[string]string{
			"voice.previous.available":            "true",
			"voice.previous.heard_text":           "好的，已经为你打开客厅灯，",
			"voice.previous.missed_text":          "现在把亮度调到了最舒适的模式。",
			"voice.previous.resume_anchor":        "好的，已经为你打开客厅灯，",
			"voice.previous.response_interrupted": "true",
			"voice.previous.response_truncated":   "true",
		},
	}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}

	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	for _, message := range model.requests[0].Messages {
		if message.Role == "system" && strings.Contains(message.Content, "Runtime voice follow-up hint:") {
			t.Fatalf("expected unrelated question to skip runtime follow-up hint, got %+v", model.requests[0].Messages)
		}
	}
}

func TestLLMTurnExecutorReusesExactPreviewPrewarm(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	promptProvider := &countingPromptSectionProvider{sections: []PromptSection{{Name: "persona", Content: "你是测试助手。"}}}
	toolRegistry := &countingToolRegistry{tools: []ToolDefinition{{Name: "time.now"}}}
	model := &recordingChatModel{response: ChatModelResponse{Text: "明天是星期五。"}}
	executor := NewLLMTurnExecutor(model).
		WithMemoryStore(memoryStore).
		WithPromptSectionProvider(promptProvider).
		WithToolRegistry(toolRegistry)

	executor.PrewarmTurn(context.Background(), TurnInput{
		SessionID:  "sess-prewarm",
		DeviceID:   "panel-1",
		ClientType: "rtos",
		UserText:   "明天周几",
		Metadata: map[string]string{
			"voice.preview.prewarm":            "true",
			"voice.preview.stable_prefix":      "明天周几",
			"voice.preview.utterance_complete": "true",
		},
	})

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-prewarm",
		DeviceID:   "panel-1",
		ClientType: "rtos",
		UserText:   "明天周几",
	}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}

	if got := len(memoryStore.loadQueries); got != 1 {
		t.Fatalf("expected one memory load total with prewarm reuse, got %d", got)
	}
	if got := promptProvider.calls; got != 1 {
		t.Fatalf("expected one prompt-section call total with prewarm reuse, got %d", got)
	}
	if got := toolRegistry.calls; got != 1 {
		t.Fatalf("expected one tool-registry call total with prewarm reuse, got %d", got)
	}
}

func TestLLMTurnExecutorSkipsPreviewPrewarmWhenFinalTextChanges(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	promptProvider := &countingPromptSectionProvider{sections: []PromptSection{{Name: "persona", Content: "你是测试助手。"}}}
	toolRegistry := &countingToolRegistry{tools: []ToolDefinition{{Name: "time.now"}}}
	model := &recordingChatModel{response: ChatModelResponse{Text: "明天是星期五。"}}
	executor := NewLLMTurnExecutor(model).
		WithMemoryStore(memoryStore).
		WithPromptSectionProvider(promptProvider).
		WithToolRegistry(toolRegistry)

	executor.PrewarmTurn(context.Background(), TurnInput{
		SessionID:  "sess-prewarm-miss",
		DeviceID:   "panel-1",
		ClientType: "rtos",
		UserText:   "明天周",
		Metadata: map[string]string{
			"voice.preview.prewarm":            "true",
			"voice.preview.stable_prefix":      "明天周",
			"voice.preview.utterance_complete": "true",
		},
	})

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-prewarm-miss",
		DeviceID:   "panel-1",
		ClientType: "rtos",
		UserText:   "明天周几",
	}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}

	if got := len(memoryStore.loadQueries); got != 2 {
		t.Fatalf("expected prewarm miss to load memory twice, got %d", got)
	}
	if got := promptProvider.calls; got != 2 {
		t.Fatalf("expected prewarm miss to call prompt sections twice, got %d", got)
	}
	if got := toolRegistry.calls; got != 2 {
		t.Fatalf("expected prewarm miss to call tool registry twice, got %d", got)
	}
}

func TestLLMTurnExecutorStreamsTextDeltasFromStreamingModel(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	model := &scriptedStreamingChatModel{
		steps: []scriptedStreamingChatStep{{
			chunks: []string{"你好，", "我是小欧管家。"},
			response: ChatModelResponse{
				Text:         "你好，我是小欧管家。",
				FinishReason: "stop",
				Message: ChatMessage{
					Role:    "assistant",
					Content: "你好，我是小欧管家。",
				},
			},
		}},
	}
	executor := NewLLMTurnExecutor(model).WithMemoryStore(memoryStore)
	sink := &recordingTurnDeltaSink{}

	output, err := executor.StreamTurn(context.Background(), TurnInput{
		SessionID:  "sess-stream",
		DeviceID:   "rtos-1",
		ClientType: "xiaozhi-compat",
		UserText:   "你好",
	}, sink)
	if err != nil {
		t.Fatalf("StreamTurn failed: %v", err)
	}
	if output.Text != "你好，我是小欧管家。" {
		t.Fatalf("unexpected output text %q", output.Text)
	}
	if len(output.Deltas) != 0 {
		t.Fatalf("expected streaming output to omit materialized deltas, got %+v", output.Deltas)
	}
	if len(sink.deltas) != 2 {
		t.Fatalf("expected two streamed text deltas, got %+v", sink.deltas)
	}
	if sink.deltas[0].Kind != TurnDeltaKindText || sink.deltas[0].Text != "你好，" {
		t.Fatalf("unexpected first streamed delta %+v", sink.deltas[0])
	}
	if sink.deltas[1].Kind != TurnDeltaKindText || sink.deltas[1].Text != "我是小欧管家。" {
		t.Fatalf("unexpected second streamed delta %+v", sink.deltas[1])
	}
	if len(memoryStore.saveRecords) != 1 {
		t.Fatalf("expected one memory save, got %d", len(memoryStore.saveRecords))
	}
	if got := memoryStore.saveRecords[0].ResponseText; got != "你好，我是小欧管家。" {
		t.Fatalf("unexpected saved response text %q", got)
	}
}

func TestLLMTurnExecutorExecutesModelToolLoopAndReinjectsToolResults(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	toolInvoker := &recordingToolInvoker{result: ToolResult{
		ToolStatus: "completed",
		ToolOutput: `{"session_id":"sess-tool"}`,
	}}
	model := &scriptedChatModel{
		responses: []ChatModelResponse{
			{
				FinishReason: "tool_calls",
				Message: ChatMessage{
					Role: "assistant",
					ToolCalls: []ChatToolCall{
						{ID: "call_1", Name: "session_describe", Arguments: `{}`},
					},
				},
			},
			{
				Text:         "这是当前会话信息。",
				FinishReason: "stop",
				Message: ChatMessage{
					Role:    "assistant",
					Content: "这是当前会话信息。",
				},
			},
		},
	}
	executor := NewLLMTurnExecutor(model).
		WithMemoryStore(memoryStore).
		WithToolRegistry(staticToolRegistry{tools: []ToolDefinition{
			{Name: "session.describe", Description: "Return session metadata."},
		}}).
		WithToolInvoker(toolInvoker)

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-tool",
		DeviceID:   "rtos-1",
		ClientType: "xiaozhi-compat",
		UserText:   "看一下当前会话",
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if output.Text != "这是当前会话信息。" {
		t.Fatalf("unexpected output text %q", output.Text)
	}
	if len(model.requests) != 2 {
		t.Fatalf("expected two model requests, got %d", len(model.requests))
	}
	if len(model.requests[0].Tools) != 1 {
		t.Fatalf("expected one model-facing tool, got %+v", model.requests[0].Tools)
	}
	if got := model.requests[0].Tools[0].Name; got != "session_describe" {
		t.Fatalf("expected sanitized model-facing tool name, got %q", got)
	}
	if len(toolInvoker.calls) != 1 {
		t.Fatalf("expected one tool invocation, got %d", len(toolInvoker.calls))
	}
	if got := toolInvoker.calls[0].ToolName; got != "session.describe" {
		t.Fatalf("expected runtime tool name session.describe, got %q", got)
	}
	if got := toolInvoker.calls[0].ToolInput; got != `{}` {
		t.Fatalf("unexpected tool input %q", got)
	}
	if len(model.requests[1].Messages) < 2 {
		t.Fatalf("expected second request to include assistant/tool messages, got %+v", model.requests[1].Messages)
	}
	foundAssistantToolCall := false
	foundToolResult := false
	for _, message := range model.requests[1].Messages {
		if message.Role == "assistant" && len(message.ToolCalls) == 1 && message.ToolCalls[0].Name == "session_describe" {
			foundAssistantToolCall = true
		}
		if message.Role == "tool" && message.ToolCallID == "call_1" && message.Content == `{"session_id":"sess-tool"}` {
			foundToolResult = true
		}
	}
	if !foundAssistantToolCall {
		t.Fatalf("expected second request to reinject assistant tool call, got %+v", model.requests[1].Messages)
	}
	if !foundToolResult {
		t.Fatalf("expected second request to reinject tool result, got %+v", model.requests[1].Messages)
	}
	if len(output.Deltas) != 3 {
		t.Fatalf("expected tool call, tool result, and final text deltas, got %+v", output.Deltas)
	}
	if output.Deltas[0].Kind != TurnDeltaKindToolCall {
		t.Fatalf("expected first delta tool_call, got %+v", output.Deltas[0])
	}
	if output.Deltas[1].Kind != TurnDeltaKindToolResult {
		t.Fatalf("expected second delta tool_result, got %+v", output.Deltas[1])
	}
	if output.Deltas[2].Kind != TurnDeltaKindText || output.Deltas[2].Text != "这是当前会话信息。" {
		t.Fatalf("unexpected final text delta %+v", output.Deltas[2])
	}
	if len(memoryStore.saveRecords) != 1 {
		t.Fatalf("expected one memory save, got %d", len(memoryStore.saveRecords))
	}
	if got := memoryStore.saveRecords[0].ResponseText; got != "这是当前会话信息。" {
		t.Fatalf("unexpected saved response text %q", got)
	}
}

func TestLLMTurnExecutorDefaultPromptUsesAssistantNameAndHomeControlConstraints(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "好的，已经为你处理好了。"},
	}
	executor := NewLLMTurnExecutor(model).WithAssistantName("云璟")

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{UserText: "介绍一下你自己"}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	prompt := model.requests[0].SystemPrompt
	if !strings.Contains(prompt, "云璟") {
		t.Fatalf("expected prompt to include assistant name, got %q", prompt)
	}
	if !strings.Contains(prompt, "不输出 JSON") {
		t.Fatalf("expected prompt to include natural-language-only rule, got %q", prompt)
	}
	if !strings.Contains(prompt, "不要主动提及调试阶段") {
		t.Fatalf("expected prompt to include debug-stage concealment rule, got %q", prompt)
	}
	if !strings.Contains(prompt, "仿真执行成功式反馈") {
		t.Fatalf("expected prompt to include simulated success rule, got %q", prompt)
	}
	if !strings.Contains(prompt, "当前执行模式：simulation") {
		t.Fatalf("expected prompt to include simulation mode policy, got %q", prompt)
	}
}

func TestLLMTurnExecutorLiveControlModeDoesNotInheritSimulationPolicy(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "好的。"},
	}
	executor := NewLLMTurnExecutor(model).
		WithAssistantName("璟").
		WithExecutionMode("live_control")

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{UserText: "你能做什么"}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	prompt := model.requests[0].SystemPrompt
	if !strings.Contains(prompt, "当前执行模式：live_control") {
		t.Fatalf("expected live_control policy, got %q", prompt)
	}
	if strings.Contains(prompt, "仿真执行成功式反馈") {
		t.Fatalf("did not expect simulation policy in live_control prompt, got %q", prompt)
	}
	if strings.Contains(prompt, "不要主动提及调试阶段") {
		t.Fatalf("did not expect debug-stage concealment rule in live_control prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "只有在系统实际完成执行") {
		t.Fatalf("expected live_control execution rule, got %q", prompt)
	}
}

func TestLLMTurnExecutorCustomPromptReplacesAssistantNamePlaceholderAndAppendsModePolicy(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "好的。"},
	}
	executor := NewLLMTurnExecutor(model).
		WithAssistantName("璟").
		WithExecutionMode("dry_run").
		WithSystemPrompt("你是{{assistant_name}}，请直接确认控制结果。")

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{UserText: "你是谁"}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	got := model.requests[0].SystemPrompt
	if !strings.Contains(got, "你是璟，请直接确认控制结果。") {
		t.Fatalf("expected assistant_name placeholder replacement, got %q", got)
	}
	if !strings.Contains(got, "当前执行模式：dry_run") {
		t.Fatalf("expected dry_run policy, got %q", got)
	}
	if !strings.Contains(got, "不要声称已经真实执行完成") {
		t.Fatalf("expected dry_run non-execution rule, got %q", got)
	}
}

func TestLLMTurnExecutorCanComposeCustomCorePromptSectionsWithSkillSections(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "好的。"},
	}
	toolBackend := NewRuntimeToolBackend(NewNoopMemoryStore(), []string{"household_control"})
	executor := NewLLMTurnExecutor(model).
		WithPromptSectionProvider(staticPromptSectionProvider{
			sections: []PromptSection{
				{Name: "persona", Content: "你是星澜。"},
				{Name: "policy", Content: "请保持冷静、简洁。"},
			},
		}).
		WithToolRegistry(toolBackend).
		WithToolInvoker(toolBackend)

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-sections",
		DeviceID:   "panel-1",
		ClientType: "web-h5",
		UserText:   "把客厅灯打开",
	}); err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(model.requests))
	}
	prompt := model.requests[0].SystemPrompt
	if !strings.Contains(prompt, "你是星澜。") {
		t.Fatalf("expected custom persona section, got %q", prompt)
	}
	if !strings.Contains(prompt, "请保持冷静、简洁。") {
		t.Fatalf("expected custom policy section, got %q", prompt)
	}
	if !strings.Contains(prompt, "已启用 runtime skill: household_control") {
		t.Fatalf("expected runtime skill prompt section, got %q", prompt)
	}
	if strings.Contains(prompt, "当前执行模式：simulation") {
		t.Fatalf("did not expect builtin execution policy when custom core sections are injected, got %q", prompt)
	}
}

func TestLLMTurnExecutorPreservesBootstrapCommands(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "should not be used"},
	}
	executor := NewLLMTurnExecutor(model).WithMemoryStore(memoryStore)

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{UserText: "/memory"})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if !strings.Contains(output.Text, "memory: bootstrap") {
		t.Fatalf("expected bootstrap memory response, got %q", output.Text)
	}
	if len(model.requests) != 0 {
		t.Fatalf("expected /memory to bypass model, got %d requests", len(model.requests))
	}
	if len(memoryStore.saveRecords) != 0 {
		t.Fatalf("expected /memory to skip persistence, got %+v", memoryStore.saveRecords)
	}
}

func TestLLMTurnExecutorDoesNotBypassModelForHouseholdControl(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "我已经理解你的需求。"},
	}
	executor := NewLLMTurnExecutor(model).
		WithAssistantName("小欧管家").
		WithExecutionMode("simulation")

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		UserText: "把灯打开",
		Metadata: map[string]string{
			"room_name": "客厅",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if output.Text != "我已经理解你的需求。" {
		t.Fatalf("unexpected model output %q", output.Text)
	}
	if len(model.requests) != 1 {
		t.Fatalf("expected household request to reach model, got %d requests", len(model.requests))
	}
	if len(output.Deltas) != 1 || output.Deltas[0].Text != "我已经理解你的需求。" {
		t.Fatalf("expected one model text delta, got %+v", output.Deltas)
	}
}

func TestLLMTurnExecutorUsesHouseholdSkillThroughToolLoop(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	toolBackend := NewRuntimeToolBackend(memoryStore, []string{"household_control"})
	model := &scriptedChatModel{
		responses: []ChatModelResponse{
			{
				FinishReason: "tool_calls",
				Message: ChatMessage{
					Role: "assistant",
					ToolCalls: []ChatToolCall{
						{
							ID:        "call_home_1",
							Name:      "home_control_simulate",
							Arguments: `{"room_name":"客厅","device_type":"light","action":"on","utterance_summary":"打开客厅灯"}`,
						},
					},
				},
			},
			{
				Text:         "好的，已经把客厅灯打开了。",
				FinishReason: "stop",
				Message: ChatMessage{
					Role:    "assistant",
					Content: "好的，已经把客厅灯打开了。",
				},
			},
		},
	}
	executor := NewLLMTurnExecutor(model).
		WithMemoryStore(memoryStore).
		WithToolRegistry(toolBackend).
		WithToolInvoker(toolBackend).
		WithAssistantName("小欧管家").
		WithExecutionMode("simulation")

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess-home",
		DeviceID:   "panel-1",
		ClientType: "web-h5",
		UserText:   "把灯打开",
		Metadata: map[string]string{
			"room_name": "客厅",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if output.Text != "好的，已经把客厅灯打开了。" {
		t.Fatalf("unexpected final output %q", output.Text)
	}
	if len(model.requests) != 2 {
		t.Fatalf("expected two model requests, got %d", len(model.requests))
	}
	if !strings.Contains(model.requests[0].SystemPrompt, "已启用 runtime skill: household_control") {
		t.Fatalf("expected household skill prompt fragment, got %q", model.requests[0].SystemPrompt)
	}
	foundHouseholdTool := false
	for _, tool := range model.requests[0].Tools {
		if tool.Name == "home_control_simulate" {
			foundHouseholdTool = true
			break
		}
	}
	if !foundHouseholdTool {
		t.Fatalf("expected household skill tool in first request, got %+v", model.requests[0].Tools)
	}
	secondMessages := model.requests[1].Messages
	if len(secondMessages) == 0 || secondMessages[len(secondMessages)-1].Role != "tool" {
		t.Fatalf("expected tool reinjection before final answer, got %+v", secondMessages)
	}
	if !strings.Contains(secondMessages[len(secondMessages)-1].Content, `"device_type":"light"`) {
		t.Fatalf("expected household tool output in reinjected tool message, got %+v", secondMessages[len(secondMessages)-1])
	}
}

func TestLLMTurnExecutorFallsBackToBootstrapSummaryWithoutText(t *testing.T) {
	model := &recordingChatModel{
		response: ChatModelResponse{Text: "should not be used"},
	}
	executor := NewLLMTurnExecutor(model)

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		Audio: AudioInput{
			Present: true,
			Frames:  3,
			Bytes:   1920,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if !strings.Contains(output.Text, "3 audio frames") {
		t.Fatalf("expected bootstrap audio summary, got %q", output.Text)
	}
	if len(model.requests) != 0 {
		t.Fatalf("expected audio-only turn to bypass model, got %d requests", len(model.requests))
	}
}

func TestDeepSeekChatModelCallsCompatibleChatCompletionsAPI(t *testing.T) {
	var seenAuthorization string
	var seenPath string
	var payload struct {
		Model       string   `json:"model"`
		Stream      bool     `json:"stream"`
		Temperature *float64 `json:"temperature,omitempty"`
		MaxTokens   int      `json:"max_tokens,omitempty"`
		Messages    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seenAuthorization = r.Header.Get("Authorization")
			seenPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request failed: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"DeepSeek 已接入"}}]}`)),
			}, nil
		}),
	}

	model := NewDeepSeekChatModel(DeepSeekChatModelConfig{
		BaseURL:     "https://api.deepseek.com",
		APIKey:      "test-key",
		Model:       "deepseek-chat",
		Temperature: 0.2,
		MaxTokens:   512,
		Timeout:     2 * time.Second,
		HTTPClient:  httpClient,
	})

	response, err := model.Complete(context.Background(), ChatModelRequest{
		SystemPrompt: "请简洁回答。",
		MemoryContext: MemoryContext{
			Summary: "记住用户偏好中文语音。",
		},
		UserText: "现在状态如何？",
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if response.Text != "DeepSeek 已接入" {
		t.Fatalf("unexpected response text %q", response.Text)
	}
	if seenAuthorization != "Bearer test-key" {
		t.Fatalf("unexpected authorization header %q", seenAuthorization)
	}
	if seenPath != "/chat/completions" {
		t.Fatalf("unexpected request path %q", seenPath)
	}
	if payload.Model != "deepseek-chat" {
		t.Fatalf("unexpected model %q", payload.Model)
	}
	if payload.Stream {
		t.Fatal("expected non-streaming request")
	}
	if payload.Temperature == nil || *payload.Temperature != 0.2 {
		t.Fatalf("unexpected temperature %+v", payload.Temperature)
	}
	if payload.MaxTokens != 512 {
		t.Fatalf("unexpected max_tokens %d", payload.MaxTokens)
	}
	if len(payload.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %+v", payload.Messages)
	}
	if payload.Messages[0].Role != "system" || payload.Messages[0].Content != "请简洁回答。" {
		t.Fatalf("unexpected system message %+v", payload.Messages[0])
	}
	if payload.Messages[1].Role != "system" || !strings.Contains(payload.Messages[1].Content, "记住用户偏好中文语音") {
		t.Fatalf("unexpected memory message %+v", payload.Messages[1])
	}
	if payload.Messages[2].Role != "user" || payload.Messages[2].Content != "现在状态如何？" {
		t.Fatalf("unexpected user message %+v", payload.Messages[2])
	}
}
