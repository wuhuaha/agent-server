package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultToolLoopMaxSteps = 6

type LLMTurnExecutor struct {
	MemoryStore    MemoryStore
	ToolRegistry   ToolRegistry
	ToolInvoker    ToolInvoker
	PromptSections PromptSectionProvider
	Model          ChatModel
	SystemPrompt   string
	AssistantName  string
	Persona        string
	ExecutionMode  string
	prewarm        *llmPrewarmState
}

type llmPrewarmState struct {
	mu        sync.Mutex
	bySession map[string]llmPrewarmCandidate
}

type llmPrewarmCandidate struct {
	SessionID     string
	DeviceID      string
	ClientType    string
	UserText      string
	MetadataSig   string
	PreparedAt    time.Time
	SystemPrompt  string
	MemoryContext MemoryContext
	Tools         []ToolDefinition
	Aliases       toolAliasSet
}

const llmPrewarmTTL = 15 * time.Second

func NewLLMTurnExecutor(model ChatModel) LLMTurnExecutor {
	return LLMTurnExecutor{
		MemoryStore:    NewNoopMemoryStore(),
		ToolRegistry:   NewNoopToolRegistry(),
		ToolInvoker:    NewNoopToolInvoker(),
		PromptSections: NewBuiltinPromptSectionProvider(),
		Model:          model,
		prewarm:        &llmPrewarmState{bySession: make(map[string]llmPrewarmCandidate)},
	}
}

func (e LLMTurnExecutor) WithMemoryStore(store MemoryStore) LLMTurnExecutor {
	if store == nil {
		store = NewNoopMemoryStore()
	}
	e.MemoryStore = store
	return e
}

func (e LLMTurnExecutor) WithToolRegistry(registry ToolRegistry) LLMTurnExecutor {
	if registry == nil {
		registry = NewNoopToolRegistry()
	}
	e.ToolRegistry = registry
	return e
}

func (e LLMTurnExecutor) WithToolInvoker(invoker ToolInvoker) LLMTurnExecutor {
	if invoker == nil {
		invoker = NewNoopToolInvoker()
	}
	e.ToolInvoker = invoker
	return e
}

func (e LLMTurnExecutor) WithPromptSectionProvider(provider PromptSectionProvider) LLMTurnExecutor {
	if provider == nil {
		provider = NewBuiltinPromptSectionProvider()
	}
	e.PromptSections = provider
	return e
}

func (e LLMTurnExecutor) WithSystemPrompt(prompt string) LLMTurnExecutor {
	e.SystemPrompt = strings.TrimSpace(prompt)
	return e
}

func (e LLMTurnExecutor) WithAssistantName(name string) LLMTurnExecutor {
	e.AssistantName = strings.TrimSpace(name)
	return e
}

func (e LLMTurnExecutor) WithPersona(persona string) LLMTurnExecutor {
	e.Persona = strings.TrimSpace(persona)
	return e
}

func (e LLMTurnExecutor) WithExecutionMode(mode string) LLMTurnExecutor {
	e.ExecutionMode = strings.TrimSpace(mode)
	return e
}

func (e LLMTurnExecutor) ExecuteTurn(ctx context.Context, input TurnInput) (TurnOutput, error) {
	collector := &turnDeltaCollector{}
	output, err := e.StreamTurn(ctx, input, collector)
	if err != nil {
		return TurnOutput{}, err
	}
	output.Deltas = collector.deltas
	return output, nil
}

func (e LLMTurnExecutor) PrewarmTurn(ctx context.Context, input TurnInput) {
	trimmedText := strings.TrimSpace(input.UserText)
	if trimmedText == "" || e.model() == nil {
		return
	}
	metadata := cloneMetadata(input.Metadata)
	memoryContext, err := e.memoryStore().LoadTurnContext(ctx, buildMemoryQuery(input, trimmedText, metadata))
	if err != nil {
		return
	}
	candidate := llmPrewarmCandidate{
		SessionID:     input.SessionID,
		DeviceID:      input.DeviceID,
		ClientType:    input.ClientType,
		UserText:      trimmedText,
		MetadataSig:   llmPrewarmMetadataSignature(metadata),
		PreparedAt:    time.Now().UTC(),
		SystemPrompt:  e.systemPrompt(ctx, input),
		MemoryContext: cloneMemoryContext(memoryContext),
	}
	candidate.Tools, candidate.Aliases = e.listModelTools(ctx, input)
	e.prewarmState().store(candidate)
}

func (e LLMTurnExecutor) StreamTurn(ctx context.Context, input TurnInput, sink TurnDeltaSink) (TurnOutput, error) {
	trimmedText := strings.TrimSpace(input.UserText)
	metadata := cloneMetadata(input.Metadata)
	prewarmed, hasPrewarm := e.prewarmState().consumeMatch(input, trimmedText, metadata)
	memoryContext := MemoryContext{}
	var memoryErr error
	if hasPrewarm {
		memoryContext = cloneMemoryContext(prewarmed.MemoryContext)
	} else {
		memoryContext, memoryErr = e.memoryStore().LoadTurnContext(ctx, buildMemoryQuery(input, trimmedText, metadata))
	}

	output, err := e.streamLLMOutput(ctx, input, trimmedText, metadata, memoryContext, memoryErr, prewarmed, hasPrewarm, sink)
	if err != nil {
		return TurnOutput{}, err
	}

	if shouldPersistTurn(trimmedText) {
		_ = e.memoryStore().SaveTurn(ctx, buildMemoryRecord(input, trimmedText, output.Text, metadata))
	}
	return output, nil
}

func (e LLMTurnExecutor) streamLLMOutput(
	ctx context.Context,
	input TurnInput,
	trimmedText string,
	metadata map[string]string,
	memoryContext MemoryContext,
	memoryErr error,
	prewarmed llmPrewarmCandidate,
	hasPrewarm bool,
	sink TurnDeltaSink,
) (TurnOutput, error) {
	bootstrap := e.bootstrapDelegate()
	if trimmedText == "" || e.model() == nil {
		return bootstrap.streamBootstrapOutput(ctx, input, trimmedText, memoryContext, memoryErr, sink)
	}
	if _, ok := parseBootstrapMemoryCommand(trimmedText); ok {
		return bootstrap.streamBootstrapOutput(ctx, input, trimmedText, memoryContext, memoryErr, sink)
	}
	if _, ok := parseBootstrapToolCommand(trimmedText); ok {
		return bootstrap.streamBootstrapOutput(ctx, input, trimmedText, memoryContext, memoryErr, sink)
	}

	var (
		systemPrompt string
		tools        []ToolDefinition
		aliases      toolAliasSet
	)
	if hasPrewarm {
		systemPrompt = prewarmed.SystemPrompt
		memoryContext = cloneMemoryContext(prewarmed.MemoryContext)
		memoryErr = nil
		tools = cloneToolDefinitions(prewarmed.Tools)
		aliases = prewarmed.Aliases.clone()
	} else {
		systemPrompt = e.systemPrompt(ctx, input)
		tools, aliases = e.listModelTools(ctx, input)
	}
	messages := initialChatMessages(systemPrompt, memoryContext, trimmedText, metadata)
	var outputText strings.Builder

	for step := 0; step < defaultToolLoopMaxSteps; step++ {
		response, err := e.runModelStep(ctx, ChatModelRequest{
			SessionID:     input.SessionID,
			DeviceID:      input.DeviceID,
			ClientType:    input.ClientType,
			UserText:      trimmedText,
			SystemPrompt:  systemPrompt,
			MemoryContext: memoryContext,
			Metadata:      metadata,
			Images:        append([]ImageInput(nil), input.Images...),
			Messages:      cloneChatMessages(messages),
			Tools:         cloneToolDefinitions(tools),
		}, sink, &outputText)
		if err != nil {
			return TurnOutput{}, err
		}

		assistantMessage := normalizedAssistantMessage(response)
		messages = append(messages, assistantMessage)

		if len(assistantMessage.ToolCalls) == 0 {
			text := strings.TrimSpace(outputText.String())
			if text == "" {
				return TurnOutput{}, fmt.Errorf("chat model returned empty text")
			}
			return turnOutputFromText(trimmedText, text), nil
		}

		toolMessages, err := e.executeToolCalls(ctx, input, aliases, assistantMessage.ToolCalls, sink)
		if err != nil {
			return TurnOutput{}, err
		}
		messages = append(messages, toolMessages...)
	}

	return TurnOutput{}, fmt.Errorf("chat model exceeded tool step budget")
}

func (e LLMTurnExecutor) runModelStep(
	ctx context.Context,
	request ChatModelRequest,
	sink TurnDeltaSink,
	outputText *strings.Builder,
) (ChatModelResponse, error) {
	if streamingModel, ok := e.model().(StreamingChatModel); ok {
		var stepText strings.Builder
		response, err := streamingModel.Stream(ctx, request, ChatModelDeltaSinkFunc(func(ctx context.Context, delta ChatModelDelta) error {
			if delta.Text == "" {
				return nil
			}
			stepText.WriteString(delta.Text)
			if outputText != nil {
				outputText.WriteString(delta.Text)
			}
			return emitTurnDelta(ctx, sink, TurnDelta{
				Kind: TurnDeltaKindText,
				Text: delta.Text,
			})
		}))
		if err != nil {
			return ChatModelResponse{}, err
		}
		if response.Message.Role == "" {
			response.Message.Role = "assistant"
		}
		if response.Message.Content == "" {
			response.Message.Content = stepText.String()
		}
		if response.Text == "" {
			response.Text = strings.TrimSpace(response.Message.Content)
		}
		return response, nil
	}

	response, err := e.model().Complete(ctx, request)
	if err != nil {
		return ChatModelResponse{}, err
	}

	text := responseText(response)
	if text != "" {
		if outputText != nil {
			outputText.WriteString(text)
		}
		if err := emitTurnDelta(ctx, sink, TurnDelta{
			Kind: TurnDeltaKindText,
			Text: text,
		}); err != nil {
			return ChatModelResponse{}, err
		}
	}
	if response.Message.Role == "" {
		response.Message.Role = "assistant"
	}
	if response.Message.Content == "" {
		response.Message.Content = text
	}
	if response.Text == "" {
		response.Text = strings.TrimSpace(text)
	}
	return response, nil
}

func (e LLMTurnExecutor) executeToolCalls(
	ctx context.Context,
	input TurnInput,
	aliases toolAliasSet,
	toolCalls []ChatToolCall,
	sink TurnDeltaSink,
) ([]ChatMessage, error) {
	toolMessages := make([]ChatMessage, 0, len(toolCalls))
	for idx, toolCall := range toolCalls {
		callID := strings.TrimSpace(toolCall.ID)
		if callID == "" {
			callID = fmt.Sprintf("tool_call_%d", idx+1)
		}

		actualName := aliases.actualName(toolCall.Name)
		if actualName == "" {
			actualName = strings.TrimSpace(toolCall.Name)
		}
		toolInput := normalizedToolArguments(toolCall.Arguments)

		call := ToolCall{
			CallID:     callID,
			SessionID:  input.SessionID,
			DeviceID:   input.DeviceID,
			ClientType: input.ClientType,
			ToolName:   actualName,
			ToolInput:  toolInput,
		}
		if err := emitTurnDelta(ctx, sink, TurnDelta{
			Kind:       TurnDeltaKindToolCall,
			ToolCallID: call.CallID,
			ToolName:   call.ToolName,
			ToolStatus: "started",
			ToolInput:  call.ToolInput,
		}); err != nil {
			return nil, err
		}

		result, err := e.invokeTool(ctx, call)
		if err != nil {
			return nil, err
		}
		if err := emitTurnDelta(ctx, sink, TurnDelta{
			Kind:       TurnDeltaKindToolResult,
			ToolCallID: result.CallID,
			ToolName:   result.ToolName,
			ToolStatus: result.ToolStatus,
			ToolOutput: result.ToolOutput,
		}); err != nil {
			return nil, err
		}
		toolMessages = append(toolMessages, ChatMessage{
			Role:       "tool",
			ToolCallID: result.CallID,
			Content:    result.ToolOutput,
		})
	}
	return toolMessages, nil
}

func (e LLMTurnExecutor) invokeTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	result, err := e.toolInvoker().InvokeTool(ctx, call)
	if err != nil {
		result = ToolResult{
			CallID:     call.CallID,
			ToolName:   call.ToolName,
			ToolStatus: "failed",
			ToolOutput: err.Error(),
		}
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
	return result, nil
}

func (e LLMTurnExecutor) listModelTools(ctx context.Context, input TurnInput) ([]ToolDefinition, toolAliasSet) {
	tools, err := e.toolRegistry().ListTools(ctx, ToolCatalogRequest{
		SessionID:  input.SessionID,
		DeviceID:   input.DeviceID,
		ClientType: input.ClientType,
	})
	if err != nil || len(tools) == 0 {
		return nil, toolAliasSet{}
	}
	modelTools, aliases := buildModelToolDefinitions(tools)
	return modelTools, aliases
}

func (e LLMTurnExecutor) bootstrapDelegate() BootstrapTurnExecutor {
	return NewBootstrapTurnExecutor().
		WithMemoryStore(e.memoryStore()).
		WithToolRegistry(e.toolRegistry()).
		WithToolInvoker(e.toolInvoker())
}

func (e LLMTurnExecutor) systemPrompt(ctx context.Context, input TurnInput) string {
	baseSections, err := e.promptSections().ListPromptSections(ctx, PromptSectionRequest{
		SessionID:     input.SessionID,
		DeviceID:      input.DeviceID,
		ClientType:    input.ClientType,
		UserText:      input.UserText,
		Metadata:      cloneMetadata(input.Metadata),
		Template:      e.SystemPrompt,
		AssistantName: e.AssistantName,
		Persona:       e.Persona,
		ExecutionMode: e.ExecutionMode,
	})
	if err != nil || len(baseSections) == 0 {
		baseSections, _ = NewBuiltinPromptSectionProvider().ListPromptSections(ctx, PromptSectionRequest{
			SessionID:     input.SessionID,
			DeviceID:      input.DeviceID,
			ClientType:    input.ClientType,
			UserText:      input.UserText,
			Metadata:      cloneMetadata(input.Metadata),
			Template:      e.SystemPrompt,
			AssistantName: e.AssistantName,
			Persona:       e.Persona,
			ExecutionMode: e.ExecutionMode,
		})
	}
	base := composePromptSections(baseSections)
	provider, ok := e.toolRegistry().(SkillPromptProvider)
	if !ok {
		return base
	}
	fragments, err := provider.ListPromptFragments(ctx, SkillPromptRequest{
		SessionID:  input.SessionID,
		DeviceID:   input.DeviceID,
		ClientType: input.ClientType,
		UserText:   input.UserText,
		Metadata:   cloneMetadata(input.Metadata),
	})
	if err != nil || len(fragments) == 0 {
		return base
	}
	rendered := []string{strings.TrimSpace(base)}
	for _, fragment := range fragments {
		if trimmed := strings.TrimSpace(fragment); trimmed != "" {
			rendered = append(rendered, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(rendered, "\n\n"))
}

func (e LLMTurnExecutor) memoryStore() MemoryStore {
	if e.MemoryStore == nil {
		return NewNoopMemoryStore()
	}
	return e.MemoryStore
}

func (e LLMTurnExecutor) toolRegistry() ToolRegistry {
	if e.ToolRegistry == nil {
		return NewNoopToolRegistry()
	}
	return e.ToolRegistry
}

func (e LLMTurnExecutor) toolInvoker() ToolInvoker {
	if e.ToolInvoker == nil {
		return NewNoopToolInvoker()
	}
	return e.ToolInvoker
}

func (e LLMTurnExecutor) model() ChatModel {
	return e.Model
}

func (e LLMTurnExecutor) promptSections() PromptSectionProvider {
	if e.PromptSections == nil {
		return NewBuiltinPromptSectionProvider()
	}
	return e.PromptSections
}

func (e LLMTurnExecutor) prewarmState() *llmPrewarmState {
	if e.prewarm != nil {
		return e.prewarm
	}
	return &llmPrewarmState{bySession: make(map[string]llmPrewarmCandidate)}
}

func initialChatMessages(systemPrompt string, memoryContext MemoryContext, userText string, metadata map[string]string) []ChatMessage {
	messages := make([]ChatMessage, 0, 3)
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		systemPrompt = composePromptSections([]PromptSection{
			{
				Name:    "persona",
				Content: renderAgentPersonaPrompt("", "", defaultAgentPersona),
			},
			{
				Name:    "runtime_output_contract",
				Content: defaultAgentRuntimeOutputContract(),
			},
			{
				Name:    "execution_mode_policy",
				Content: defaultAgentExecutionModePolicy(defaultAgentExecutionMode),
			},
		})
	}
	messages = append(messages, ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})
	if memory := strings.TrimSpace(memoryContext.Summary); memory != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: "Conversation memory:\n" + memory,
		})
	}
	messages = append(messages, cloneChatMessages(memoryContext.RecentMessages)...)
	messages = append(messages, playbackFollowUpMessages(userText, metadata)...)
	messages = append(messages, ChatMessage{
		Role:    "user",
		Content: strings.TrimSpace(userText),
	})
	return messages
}

func normalizedAssistantMessage(response ChatModelResponse) ChatMessage {
	message := response.Message
	if message.Role == "" {
		message.Role = "assistant"
	}
	if message.Content == "" {
		message.Content = responseText(response)
	}
	message.ToolCalls = cloneToolCalls(message.ToolCalls)
	return message
}

func responseText(response ChatModelResponse) string {
	if response.Message.Content != "" {
		return response.Message.Content
	}
	return response.Text
}

func turnOutputFromText(inputText, text string) TurnOutput {
	output := TurnOutput{Text: text}
	if shouldEndSession(inputText) {
		output.EndSession = true
		output.EndReason = "completed"
		output.EndMessage = "dialog finished"
	}
	return output
}

func normalizedToolArguments(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return "{}"
	}
	return trimmed
}

type toolAliasSet struct {
	modelToActual map[string]string
}

func (s toolAliasSet) actualName(name string) string {
	if len(s.modelToActual) == 0 {
		return strings.TrimSpace(name)
	}
	if actual := strings.TrimSpace(s.modelToActual[strings.TrimSpace(name)]); actual != "" {
		return actual
	}
	return strings.TrimSpace(name)
}

func buildModelToolDefinitions(tools []ToolDefinition) ([]ToolDefinition, toolAliasSet) {
	modelTools := make([]ToolDefinition, 0, len(tools))
	aliases := toolAliasSet{modelToActual: make(map[string]string, len(tools))}
	used := make(map[string]int, len(tools))

	for _, tool := range tools {
		actualName := strings.TrimSpace(tool.Name)
		if actualName == "" {
			continue
		}
		modelName := makeModelToolName(actualName)
		if modelName == "" {
			continue
		}
		if count := used[modelName]; count > 0 {
			modelName = fmt.Sprintf("%s_%d", modelName, count+1)
		}
		used[modelName]++

		cloned := tool
		cloned.Name = modelName
		cloned.Parameters = cloneToolParameters(tool.Parameters)
		modelTools = append(modelTools, cloned)
		aliases.modelToActual[modelName] = actualName
	}

	if len(modelTools) == 0 {
		return nil, toolAliasSet{}
	}
	return modelTools, aliases
}

func makeModelToolName(name string) string {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	modelName := strings.Trim(builder.String(), "_-")
	if modelName == "" {
		modelName = "tool"
	}
	if len(modelName) > 64 {
		modelName = modelName[:64]
	}
	return modelName
}

func cloneChatMessages(messages []ChatMessage) []ChatMessage {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]ChatMessage, 0, len(messages))
	for _, message := range messages {
		cloned = append(cloned, ChatMessage{
			Role:       message.Role,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolCalls:  cloneToolCalls(message.ToolCalls),
		})
	}
	return cloned
}

func cloneToolCalls(toolCalls []ChatToolCall) []ChatToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	cloned := make([]ChatToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		cloned = append(cloned, toolCall)
	}
	return cloned
}

func cloneToolDefinitions(tools []ToolDefinition) []ToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	cloned := make([]ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		cloned = append(cloned, ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  cloneToolParameters(tool.Parameters),
			Strict:      tool.Strict,
		})
	}
	return cloned
}

func cloneToolParameters(parameters map[string]any) map[string]any {
	if len(parameters) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(parameters))
	for key, value := range parameters {
		cloned[key] = cloneJSONLike(value)
	}
	return cloned
}

func (s *llmPrewarmState) store(candidate llmPrewarmCandidate) {
	if s == nil || strings.TrimSpace(candidate.SessionID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bySession == nil {
		s.bySession = make(map[string]llmPrewarmCandidate)
	}
	s.bySession[candidate.SessionID] = candidate
}

func (s *llmPrewarmState) consumeMatch(input TurnInput, trimmedText string, metadata map[string]string) (llmPrewarmCandidate, bool) {
	if s == nil || strings.TrimSpace(input.SessionID) == "" {
		return llmPrewarmCandidate{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate, ok := s.bySession[input.SessionID]
	if !ok {
		return llmPrewarmCandidate{}, false
	}
	delete(s.bySession, input.SessionID)
	if time.Since(candidate.PreparedAt) > llmPrewarmTTL {
		return llmPrewarmCandidate{}, false
	}
	if strings.TrimSpace(candidate.DeviceID) != strings.TrimSpace(input.DeviceID) ||
		strings.TrimSpace(candidate.ClientType) != strings.TrimSpace(input.ClientType) ||
		strings.TrimSpace(candidate.UserText) != strings.TrimSpace(trimmedText) ||
		candidate.MetadataSig != llmPrewarmMetadataSignature(metadata) {
		return llmPrewarmCandidate{}, false
	}
	return candidate, true
}

func llmPrewarmMetadataSignature(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	keys := make([]string, 0, len(metadata))
	for key, value := range metadata {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" || strings.HasPrefix(trimmedKey, "voice.preview.") {
			continue
		}
		if trimmedValue := strings.TrimSpace(value); trimmedValue != "" {
			keys = append(keys, trimmedKey+"="+trimmedValue)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return strings.Join(keys, "&")
}

func cloneMemoryContext(context MemoryContext) MemoryContext {
	cloned := context
	if len(context.Facts) > 0 {
		cloned.Facts = append([]MemoryFact(nil), context.Facts...)
	}
	cloned.RecentMessages = cloneChatMessages(context.RecentMessages)
	return cloned
}

func (s toolAliasSet) clone() toolAliasSet {
	if len(s.modelToActual) == 0 {
		return toolAliasSet{}
	}
	cloned := make(map[string]string, len(s.modelToActual))
	for key, value := range s.modelToActual {
		cloned[key] = value
	}
	return toolAliasSet{modelToActual: cloned}
}

func cloneJSONLike(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, nested := range typed {
			cloned[key] = cloneJSONLike(nested)
		}
		return cloned
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, nested := range typed {
			cloned = append(cloned, cloneJSONLike(nested))
		}
		return cloned
	default:
		return typed
	}
}
