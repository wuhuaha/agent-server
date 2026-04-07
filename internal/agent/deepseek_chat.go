package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	defaultDeepSeekBaseURL = "https://api.deepseek.com"
	defaultDeepSeekModel   = "deepseek-chat"
)

type DeepSeekChatModelConfig struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int
	Timeout     time.Duration
	HTTPClient  *http.Client
}

type DeepSeekChatModel struct {
	baseURL     string
	apiKey      string
	model       string
	temperature float64
	maxTokens   int
	httpClient  *http.Client
}

func NewDeepSeekChatModel(cfg DeepSeekChatModelConfig) DeepSeekChatModel {
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultDeepSeekModel
	}
	return DeepSeekChatModel{
		baseURL:     normalizeDeepSeekBaseURL(cfg.BaseURL),
		apiKey:      strings.TrimSpace(cfg.APIKey),
		model:       model,
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
		httpClient:  client,
	}
}

func (m DeepSeekChatModel) Complete(ctx context.Context, request ChatModelRequest) (ChatModelResponse, error) {
	httpRequest, err := m.newRequest(ctx, request, false)
	if err != nil {
		return ChatModelResponse{}, err
	}

	response, err := m.httpClient.Do(httpRequest)
	if err != nil {
		return ChatModelResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ChatModelResponse{}, fmt.Errorf("deepseek chat completion failed: %s", deepseekErrorMessage(response))
	}

	var decoded deepseekChatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return ChatModelResponse{}, err
	}
	return decodeDeepSeekResponse(decoded)
}

func (m DeepSeekChatModel) Stream(ctx context.Context, request ChatModelRequest, sink ChatModelDeltaSink) (ChatModelResponse, error) {
	httpRequest, err := m.newRequest(ctx, request, true)
	if err != nil {
		return ChatModelResponse{}, err
	}
	httpRequest.Header.Set("Accept", "text/event-stream")

	response, err := m.httpClient.Do(httpRequest)
	if err != nil {
		return ChatModelResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ChatModelResponse{}, fmt.Errorf("deepseek chat completion failed: %s", deepseekErrorMessage(response))
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var content strings.Builder
	var finishReason string
	role := "assistant"
	toolCalls := make(map[int]ChatToolCall)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		var chunk deepseekChatCompletionChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return ChatModelResponse{}, err
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if strings.TrimSpace(choice.Delta.Role) != "" {
			role = strings.TrimSpace(choice.Delta.Role)
		}
		if choice.Delta.Content != "" {
			content.WriteString(choice.Delta.Content)
			if err := emitChatModelDelta(ctx, sink, ChatModelDelta{Text: choice.Delta.Content}); err != nil {
				return ChatModelResponse{}, err
			}
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			merged := toolCalls[toolCall.Index]
			if strings.TrimSpace(toolCall.ID) != "" {
				merged.ID = strings.TrimSpace(toolCall.ID)
			}
			if strings.TrimSpace(toolCall.Function.Name) != "" {
				merged.Name = strings.TrimSpace(toolCall.Function.Name)
			}
			if toolCall.Function.Arguments != "" {
				merged.Arguments += toolCall.Function.Arguments
			}
			toolCalls[toolCall.Index] = merged
		}
		if strings.TrimSpace(choice.FinishReason) != "" {
			finishReason = strings.TrimSpace(choice.FinishReason)
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatModelResponse{}, err
	}

	message := ChatMessage{
		Role:      role,
		Content:   content.String(),
		ToolCalls: flattenIndexedToolCalls(toolCalls),
	}
	text := strings.TrimSpace(message.Content)
	if text == "" && len(message.ToolCalls) == 0 {
		return ChatModelResponse{}, fmt.Errorf("deepseek chat completion returned empty content")
	}
	return ChatModelResponse{
		Text:         text,
		FinishReason: finishReason,
		Message:      message,
	}, nil
}

func (m DeepSeekChatModel) newRequest(ctx context.Context, request ChatModelRequest, stream bool) (*http.Request, error) {
	body, err := json.Marshal(m.buildRequest(request, stream))
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+m.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	return httpRequest, nil
}

func (m DeepSeekChatModel) buildRequest(request ChatModelRequest, stream bool) deepseekChatCompletionRequest {
	messages := request.Messages
	if len(messages) == 0 {
		messages = initialChatMessages(request.SystemPrompt, request.MemoryContext, request.UserText)
	}

	payload := deepseekChatCompletionRequest{
		Model:    m.model,
		Messages: make([]deepseekChatMessage, 0, len(messages)),
		Stream:   stream,
	}
	payload.Temperature = &m.temperature
	if m.maxTokens > 0 {
		payload.MaxTokens = m.maxTokens
	}

	for _, message := range messages {
		payload.Messages = append(payload.Messages, deepseekChatMessage{
			Role:       message.Role,
			Content:    deepseekMessageContent(message),
			ToolCallID: message.ToolCallID,
			ToolCalls:  deepseekToolCallsFromChat(message.ToolCalls),
		})
	}

	if len(request.Tools) > 0 {
		payload.Tools = make([]deepseekToolDefinition, 0, len(request.Tools))
		for _, tool := range request.Tools {
			if strings.TrimSpace(tool.Name) == "" {
				continue
			}
			payload.Tools = append(payload.Tools, deepseekToolDefinition{
				Type: "function",
				Function: deepseekToolFunction{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  cloneToolParameters(tool.Parameters),
					Strict:      tool.Strict,
				},
			})
		}
		if len(payload.Tools) > 0 {
			payload.ToolChoice = "auto"
		}
	}

	return payload
}

type deepseekChatCompletionRequest struct {
	Model       string                   `json:"model"`
	Messages    []deepseekChatMessage    `json:"messages"`
	Stream      bool                     `json:"stream"`
	Temperature *float64                 `json:"temperature,omitempty"`
	MaxTokens   int                      `json:"max_tokens,omitempty"`
	Tools       []deepseekToolDefinition `json:"tools,omitempty"`
	ToolChoice  any                      `json:"tool_choice,omitempty"`
}

type deepseekChatMessage struct {
	Role       string             `json:"role"`
	Content    any                `json:"content"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	ToolCalls  []deepseekToolCall `json:"tool_calls,omitempty"`
}

type deepseekToolDefinition struct {
	Type     string               `json:"type"`
	Function deepseekToolFunction `json:"function"`
}

type deepseekToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

type deepseekToolCall struct {
	ID       string                `json:"id,omitempty"`
	Type     string                `json:"type,omitempty"`
	Function deepseekToolCallEntry `json:"function"`
}

type deepseekToolCallEntry struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type deepseekChatCompletionResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role      string             `json:"role"`
			Content   any                `json:"content"`
			ToolCalls []deepseekToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

type deepseekChatCompletionChunk struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Role      string                  `json:"role"`
			Content   string                  `json:"content"`
			ToolCalls []deepseekToolCallChunk `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

type deepseekToolCallChunk struct {
	Index    int                   `json:"index"`
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function deepseekToolCallEntry `json:"function"`
}

type deepseekAPIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func decodeDeepSeekResponse(decoded deepseekChatCompletionResponse) (ChatModelResponse, error) {
	if len(decoded.Choices) == 0 {
		return ChatModelResponse{}, fmt.Errorf("deepseek chat completion returned no choices")
	}

	choice := decoded.Choices[0]
	text := renderDeepSeekContent(choice.Message.Content)
	message := ChatMessage{
		Role:      strings.TrimSpace(choice.Message.Role),
		Content:   text,
		ToolCalls: chatToolCallsFromDeepSeek(choice.Message.ToolCalls),
	}
	if message.Role == "" {
		message.Role = "assistant"
	}

	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" && len(message.ToolCalls) == 0 {
		return ChatModelResponse{}, fmt.Errorf("deepseek chat completion returned empty content")
	}
	return ChatModelResponse{
		Text:         trimmedText,
		FinishReason: strings.TrimSpace(choice.FinishReason),
		Message:      message,
	}, nil
}

func deepseekMessageContent(message ChatMessage) any {
	if message.Role == "assistant" && message.Content == "" && len(message.ToolCalls) > 0 {
		return nil
	}
	return message.Content
}

func deepseekToolCallsFromChat(toolCalls []ChatToolCall) []deepseekToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	converted := make([]deepseekToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		converted = append(converted, deepseekToolCall{
			ID:   toolCall.ID,
			Type: "function",
			Function: deepseekToolCallEntry{
				Name:      toolCall.Name,
				Arguments: normalizedToolArguments(toolCall.Arguments),
			},
		})
	}
	return converted
}

func chatToolCallsFromDeepSeek(toolCalls []deepseekToolCall) []ChatToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	converted := make([]ChatToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		converted = append(converted, ChatToolCall{
			ID:        strings.TrimSpace(toolCall.ID),
			Name:      strings.TrimSpace(toolCall.Function.Name),
			Arguments: normalizedToolArguments(toolCall.Function.Arguments),
		})
	}
	return converted
}

func flattenIndexedToolCalls(indexed map[int]ChatToolCall) []ChatToolCall {
	if len(indexed) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(indexed))
	for idx := range indexed {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	result := make([]ChatToolCall, 0, len(indexed))
	for _, idx := range indexes {
		toolCall := indexed[idx]
		toolCall.Arguments = normalizedToolArguments(toolCall.Arguments)
		result = append(result, toolCall)
	}
	return result
}

func emitChatModelDelta(ctx context.Context, sink ChatModelDeltaSink, delta ChatModelDelta) error {
	if sink == nil {
		return nil
	}
	return sink.EmitChatModelDelta(ctx, delta)
}

func deepseekErrorMessage(response *http.Response) string {
	body, err := io.ReadAll(io.LimitReader(response.Body, 8192))
	if err != nil {
		return response.Status
	}

	var decoded deepseekAPIErrorResponse
	if err := json.Unmarshal(body, &decoded); err == nil && strings.TrimSpace(decoded.Error.Message) != "" {
		return decoded.Error.Message
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return response.Status
	}
	return trimmed
}

func renderDeepSeekContent(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			segment, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text, _ := segment["text"].(string)
			if strings.TrimSpace(text) == "" {
				continue
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func normalizeDeepSeekBaseURL(raw string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(raw), "/")
	if baseURL == "" {
		return defaultDeepSeekBaseURL
	}
	return baseURL
}
