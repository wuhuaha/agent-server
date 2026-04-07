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

func TestDeepSeekChatModelIncludesToolDefinitionsAndExplicitMessages(t *testing.T) {
	var payload struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role       string `json:"role"`
			Content    any    `json:"content"`
			ToolCallID string `json:"tool_call_id,omitempty"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"messages"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters,omitempty"`
			} `json:"function"`
		} `json:"tools,omitempty"`
	}

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request failed: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"已完成"}}]}`)),
			}, nil
		}),
	}

	model := NewDeepSeekChatModel(DeepSeekChatModelConfig{
		APIKey:     "test-key",
		HTTPClient: httpClient,
		Timeout:    2 * time.Second,
	})

	_, err := model.Complete(context.Background(), ChatModelRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "帮我看一下会话"},
			{
				Role: "assistant",
				ToolCalls: []ChatToolCall{
					{ID: "call_1", Name: "session_describe", Arguments: `{}`},
				},
			},
			{Role: "tool", ToolCallID: "call_1", Content: `{"session_id":"sess-1"}`},
		},
		Tools: []ToolDefinition{
			{
				Name:        "session_describe",
				Description: "Return session metadata.",
				Parameters: map[string]any{
					"type":                 "object",
					"properties":           map[string]any{},
					"additionalProperties": false,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if payload.Stream {
		t.Fatal("expected non-streaming request")
	}
	if len(payload.Messages) != 4 {
		t.Fatalf("expected 4 explicit messages, got %+v", payload.Messages)
	}
	if payload.Messages[2].Role != "assistant" || len(payload.Messages[2].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool-call message, got %+v", payload.Messages[2])
	}
	if got := payload.Messages[2].ToolCalls[0].Function.Name; got != "session_describe" {
		t.Fatalf("unexpected tool call name %q", got)
	}
	if payload.Messages[3].Role != "tool" || payload.Messages[3].ToolCallID != "call_1" {
		t.Fatalf("expected tool message, got %+v", payload.Messages[3])
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("expected one tool definition, got %+v", payload.Tools)
	}
	if payload.Tools[0].Type != "function" {
		t.Fatalf("unexpected tool type %q", payload.Tools[0].Type)
	}
	if payload.Tools[0].Function.Name != "session_describe" {
		t.Fatalf("unexpected tool definition name %q", payload.Tools[0].Function.Name)
	}
}

func TestDeepSeekChatModelStreamParsesTextDeltas(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
				},
				Body: io.NopCloser(strings.NewReader(strings.Join([]string{
					`data: {"choices":[{"delta":{"role":"assistant","content":"你好，"},"finish_reason":null}]}`,
					``,
					`data: {"choices":[{"delta":{"content":"这里是 DeepSeek。"},"finish_reason":"stop"}]}`,
					``,
					`data: [DONE]`,
					``,
				}, "\n"))),
			}, nil
		}),
	}

	model := NewDeepSeekChatModel(DeepSeekChatModelConfig{
		APIKey:     "test-key",
		HTTPClient: httpClient,
		Timeout:    2 * time.Second,
	})

	var chunks []string
	response, err := model.Stream(context.Background(), ChatModelRequest{
		SystemPrompt: "请简洁回答。",
		UserText:     "你好",
	}, ChatModelDeltaSinkFunc(func(_ context.Context, delta ChatModelDelta) error {
		chunks = append(chunks, delta.Text)
		return nil
	}))
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	if strings.Join(chunks, "") != "你好，这里是 DeepSeek。" {
		t.Fatalf("unexpected streamed chunks %+v", chunks)
	}
	if response.Text != "你好，这里是 DeepSeek。" {
		t.Fatalf("unexpected response text %q", response.Text)
	}
	if response.FinishReason != "stop" {
		t.Fatalf("unexpected finish reason %q", response.FinishReason)
	}
	if response.Message.Role != "assistant" {
		t.Fatalf("unexpected response role %q", response.Message.Role)
	}
}

func TestDeepSeekChatModelStreamParsesToolCalls(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
				},
				Body: io.NopCloser(strings.NewReader(strings.Join([]string{
					`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"session_describe","arguments":"{"}}]},"finish_reason":null}]}`,
					``,
					`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}]}`,
					``,
					`data: [DONE]`,
					``,
				}, "\n"))),
			}, nil
		}),
	}

	model := NewDeepSeekChatModel(DeepSeekChatModelConfig{
		APIKey:     "test-key",
		HTTPClient: httpClient,
		Timeout:    2 * time.Second,
	})

	response, err := model.Stream(context.Background(), ChatModelRequest{
		SystemPrompt: "请简洁回答。",
		UserText:     "看一下会话",
	}, nil)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	if response.FinishReason != "tool_calls" {
		t.Fatalf("unexpected finish reason %q", response.FinishReason)
	}
	if len(response.Message.ToolCalls) != 1 {
		t.Fatalf("expected one parsed tool call, got %+v", response.Message.ToolCalls)
	}
	if got := response.Message.ToolCalls[0].Name; got != "session_describe" {
		t.Fatalf("unexpected tool call name %q", got)
	}
	if got := response.Message.ToolCalls[0].Arguments; got != "{}" {
		t.Fatalf("unexpected tool call arguments %q", got)
	}
}
