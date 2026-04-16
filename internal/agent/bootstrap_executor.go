package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

type BootstrapTurnExecutor struct {
	MemoryStore  MemoryStore
	ToolRegistry ToolRegistry
	ToolInvoker  ToolInvoker
}

func NewBootstrapTurnExecutor() BootstrapTurnExecutor {
	return BootstrapTurnExecutor{
		MemoryStore:  NewNoopMemoryStore(),
		ToolRegistry: NewNoopToolRegistry(),
		ToolInvoker:  NewNoopToolInvoker(),
	}
}

func (e BootstrapTurnExecutor) WithMemoryStore(store MemoryStore) BootstrapTurnExecutor {
	if store == nil {
		store = NewNoopMemoryStore()
	}
	e.MemoryStore = store
	return e
}

func (e BootstrapTurnExecutor) WithToolRegistry(registry ToolRegistry) BootstrapTurnExecutor {
	if registry == nil {
		registry = NewNoopToolRegistry()
	}
	e.ToolRegistry = registry
	return e
}

func (e BootstrapTurnExecutor) WithToolInvoker(invoker ToolInvoker) BootstrapTurnExecutor {
	if invoker == nil {
		invoker = NewNoopToolInvoker()
	}
	e.ToolInvoker = invoker
	return e
}

func (e BootstrapTurnExecutor) ExecuteTurn(ctx context.Context, input TurnInput) (TurnOutput, error) {
	collector := &turnDeltaCollector{}
	output, err := e.StreamTurn(ctx, input, collector)
	if err != nil {
		return TurnOutput{}, err
	}
	output.Deltas = collector.deltas
	return output, nil
}

func (e BootstrapTurnExecutor) StreamTurn(ctx context.Context, input TurnInput, sink TurnDeltaSink) (TurnOutput, error) {
	trimmedText := strings.TrimSpace(input.UserText)
	metadata := cloneMetadata(input.Metadata)
	memoryContext, memoryErr := e.memoryStore().LoadTurnContext(ctx, buildMemoryQuery(input, trimmedText, metadata))

	output, err := e.streamBootstrapOutput(ctx, input, trimmedText, memoryContext, memoryErr, sink)
	if err != nil {
		return TurnOutput{}, err
	}

	if shouldPersistTurn(trimmedText) {
		_ = e.memoryStore().SaveTurn(ctx, buildMemoryRecord(input, trimmedText, output.Text, metadata))
	}
	return output, nil
}

func (e BootstrapTurnExecutor) streamBootstrapOutput(ctx context.Context, input TurnInput, trimmedText string, memoryContext MemoryContext, memoryErr error, sink TurnDeltaSink) (TurnOutput, error) {
	if _, ok := parseBootstrapMemoryCommand(trimmedText); ok {
		return executeMemoryCommand(ctx, sink, memoryContext, memoryErr)
	}
	if toolCommand, ok := parseBootstrapToolCommand(trimmedText); ok {
		return e.executeToolCommand(ctx, input, toolCommand, sink)
	}
	if followUpText, ok := bootstrapPlaybackFollowUpText(trimmedText, input.Metadata); ok {
		return emitBootstrapTextOutput(ctx, sink, trimmedText, followUpText)
	}

	output := TurnOutput{Text: "agent-server realtime bootstrap reply"}
	switch {
	case trimmedText != "":
		output.Text = fmt.Sprintf("agent-server received text input: %s", trimmedText)
	case input.Audio.Present:
		output.Text = fmt.Sprintf("agent-server received %d audio frames (%d bytes)", input.Audio.Frames, input.Audio.Bytes)
	}

	if output.Text != "" {
		if err := emitTurnDelta(ctx, sink, TurnDelta{
			Kind: TurnDeltaKindText,
			Text: output.Text,
		}); err != nil {
			return TurnOutput{}, err
		}
	}

	if shouldEndSession(trimmedText) {
		output.EndSession = true
		output.EndReason = "completed"
		output.EndMessage = "dialog finished"
	}

	return output, nil
}

func emitBootstrapTextOutput(ctx context.Context, sink TurnDeltaSink, inputText, text string) (TurnOutput, error) {
	output := TurnOutput{Text: text}
	if err := emitTurnDelta(ctx, sink, TurnDelta{
		Kind: TurnDeltaKindText,
		Text: text,
	}); err != nil {
		return TurnOutput{}, err
	}
	if shouldEndSession(inputText) {
		output.EndSession = true
		output.EndReason = "completed"
		output.EndMessage = "dialog finished"
	}
	return output, nil
}

func (e BootstrapTurnExecutor) executeToolCommand(ctx context.Context, input TurnInput, command bootstrapToolCommand, sink TurnDeltaSink) (TurnOutput, error) {
	call := ToolCall{
		CallID:     bootstrapToolCallID(command.ToolName),
		SessionID:  input.SessionID,
		DeviceID:   input.DeviceID,
		ClientType: input.ClientType,
		ToolName:   command.ToolName,
		ToolInput:  command.ToolInput,
	}

	tools, err := e.toolRegistry().ListTools(ctx, ToolCatalogRequest{
		SessionID:  input.SessionID,
		DeviceID:   input.DeviceID,
		ClientType: input.ClientType,
	})
	if err != nil {
		return toolFailureOutput(ctx, sink, call, "failed", fmt.Sprintf("tool registry failed: %v", err))
	}
	if len(tools) > 0 && !toolExists(tools, call.ToolName) {
		return toolFailureOutput(ctx, sink, call, "unavailable", fmt.Sprintf("tool %s is not registered", call.ToolName))
	}

	if err := emitTurnDelta(ctx, sink, TurnDelta{
		Kind:       TurnDeltaKindToolCall,
		ToolCallID: call.CallID,
		ToolName:   call.ToolName,
		ToolStatus: "started",
		ToolInput:  call.ToolInput,
	}); err != nil {
		return TurnOutput{}, err
	}

	result, err := e.toolInvoker().InvokeTool(ctx, call)
	if err != nil {
		return toolFailureOutput(ctx, sink, call, "failed", err.Error())
	}
	if strings.TrimSpace(result.CallID) == "" {
		result.CallID = call.CallID
	}
	if strings.TrimSpace(result.ToolName) == "" {
		result.ToolName = call.ToolName
	}
	if strings.TrimSpace(result.ToolStatus) == "" {
		result.ToolStatus = "completed"
	}

	text := toolSummaryText(result)
	if err := emitTurnDelta(ctx, sink, TurnDelta{
		Kind:       TurnDeltaKindToolResult,
		ToolCallID: result.CallID,
		ToolName:   result.ToolName,
		ToolStatus: result.ToolStatus,
		ToolOutput: result.ToolOutput,
	}); err != nil {
		return TurnOutput{}, err
	}
	if err := emitTurnDelta(ctx, sink, TurnDelta{
		Kind: TurnDeltaKindText,
		Text: text,
	}); err != nil {
		return TurnOutput{}, err
	}
	return TurnOutput{Text: text}, nil
}

func (e BootstrapTurnExecutor) memoryStore() MemoryStore {
	if e.MemoryStore == nil {
		return NewNoopMemoryStore()
	}
	return e.MemoryStore
}

func (e BootstrapTurnExecutor) toolRegistry() ToolRegistry {
	if e.ToolRegistry == nil {
		return NewNoopToolRegistry()
	}
	return e.ToolRegistry
}

func (e BootstrapTurnExecutor) toolInvoker() ToolInvoker {
	if e.ToolInvoker == nil {
		return NewNoopToolInvoker()
	}
	return e.ToolInvoker
}

type bootstrapToolCommand struct {
	ToolName  string
	ToolInput string
}

type turnDeltaCollector struct {
	deltas []TurnDelta
}

func (c *turnDeltaCollector) EmitTurnDelta(_ context.Context, delta TurnDelta) error {
	c.deltas = append(c.deltas, delta)
	return nil
}

func parseBootstrapMemoryCommand(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	if lower == "/memory" {
		return "", true
	}
	if strings.HasPrefix(lower, "/memory ") {
		return strings.TrimSpace(trimmed[len("/memory"):]), true
	}
	return "", false
}

func parseBootstrapToolCommand(text string) (bootstrapToolCommand, bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	if lower != "/tool" && !strings.HasPrefix(lower, "/tool ") {
		return bootstrapToolCommand{}, false
	}
	rest := strings.TrimSpace(trimmed[len("/tool"):])
	if rest == "" {
		return bootstrapToolCommand{}, false
	}
	toolName, toolInput, found := strings.Cut(rest, " ")
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return bootstrapToolCommand{}, false
	}
	if !found || strings.TrimSpace(toolInput) == "" {
		toolInput = "{}"
	}
	return bootstrapToolCommand{
		ToolName:  toolName,
		ToolInput: strings.TrimSpace(toolInput),
	}, true
}

func executeMemoryCommand(ctx context.Context, sink TurnDeltaSink, memoryContext MemoryContext, memoryErr error) (TurnOutput, error) {
	text := "memory: nothing remembered yet."
	switch {
	case memoryErr != nil:
		text = fmt.Sprintf("memory unavailable: %v", memoryErr)
	case strings.TrimSpace(memoryContext.Summary) != "":
		text = "memory: " + strings.TrimSpace(memoryContext.Summary)
	}

	if err := emitTurnDelta(ctx, sink, TurnDelta{
		Kind: TurnDeltaKindText,
		Text: text,
	}); err != nil {
		return TurnOutput{}, err
	}
	return TurnOutput{Text: text}, nil
}

func shouldPersistTurn(text string) bool {
	if _, ok := parseBootstrapMemoryCommand(text); ok {
		return false
	}
	if _, ok := parseBootstrapToolCommand(text); ok {
		return false
	}
	return true
}

func toolFailureOutput(ctx context.Context, sink TurnDeltaSink, call ToolCall, status, output string) (TurnOutput, error) {
	result := ToolResult{
		CallID:     call.CallID,
		ToolName:   call.ToolName,
		ToolStatus: status,
		ToolOutput: strings.TrimSpace(output),
	}
	text := toolSummaryText(result)
	if err := emitTurnDelta(ctx, sink, TurnDelta{
		Kind:       TurnDeltaKindToolResult,
		ToolCallID: result.CallID,
		ToolName:   result.ToolName,
		ToolStatus: result.ToolStatus,
		ToolOutput: result.ToolOutput,
	}); err != nil {
		return TurnOutput{}, err
	}
	if err := emitTurnDelta(ctx, sink, TurnDelta{
		Kind: TurnDeltaKindText,
		Text: text,
	}); err != nil {
		return TurnOutput{}, err
	}
	return TurnOutput{Text: text}, nil
}

func emitTurnDelta(ctx context.Context, sink TurnDeltaSink, delta TurnDelta) error {
	if sink == nil {
		return nil
	}
	return sink.EmitTurnDelta(ctx, delta)
}

func toolSummaryText(result ToolResult) string {
	toolName := strings.TrimSpace(result.ToolName)
	if toolName == "" {
		toolName = "tool"
	}
	status := strings.TrimSpace(result.ToolStatus)
	output := strings.TrimSpace(result.ToolOutput)

	switch status {
	case "", "completed", "success", "succeeded", "ok":
		if output != "" {
			return fmt.Sprintf("tool %s completed: %s", toolName, output)
		}
		return fmt.Sprintf("tool %s completed.", toolName)
	case "unavailable":
		if output != "" {
			return fmt.Sprintf("tool %s is unavailable: %s", toolName, output)
		}
		return fmt.Sprintf("tool %s is unavailable.", toolName)
	default:
		if output != "" {
			return fmt.Sprintf("tool %s %s: %s", toolName, status, output)
		}
		return fmt.Sprintf("tool %s %s.", toolName, status)
	}
}

func toolExists(tools []ToolDefinition, name string) bool {
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool.Name), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func bootstrapToolCallID(name string) string {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if trimmed == "" {
		return "tool_call"
	}
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	value := strings.Trim(builder.String(), "_")
	if value == "" {
		value = "call"
	}
	return "tool_" + value
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func shouldEndSession(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	switch normalized {
	case "/end", "bye", "goodbye", "结束", "结束对话":
		return true
	default:
		return false
	}
}
