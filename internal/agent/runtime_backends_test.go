package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestInMemoryMemoryStoreKeepsRecentTurns(t *testing.T) {
	store := NewInMemoryMemoryStore(2)
	ctx := context.Background()

	if err := store.SaveTurn(ctx, MemoryRecord{SessionID: "sess_1", DeviceID: "rtos-1", UserText: "hello", ResponseText: "hi"}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}
	if err := store.SaveTurn(ctx, MemoryRecord{SessionID: "sess_2", DeviceID: "rtos-1", UserText: "how are you", ResponseText: "fine"}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}
	if err := store.SaveTurn(ctx, MemoryRecord{SessionID: "sess_3", DeviceID: "rtos-1", UserText: "bye", ResponseText: "goodbye"}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}

	memoryContext, err := store.LoadTurnContext(ctx, MemoryQuery{DeviceID: "rtos-1"})
	if err != nil {
		t.Fatalf("LoadTurnContext failed: %v", err)
	}
	if !strings.Contains(memoryContext.Summary, "remembered 2 turn(s)") {
		t.Fatalf("unexpected memory summary %q", memoryContext.Summary)
	}
	if !strings.Contains(memoryContext.Summary, "bye") {
		t.Fatalf("expected latest user text in summary, got %q", memoryContext.Summary)
	}
	if memoryContext.Scope != "device rtos-1" {
		t.Fatalf("unexpected memory scope %q", memoryContext.Scope)
	}
	if len(memoryContext.RecentMessages) != 4 {
		t.Fatalf("expected 4 recent messages for the last 2 turns, got %+v", memoryContext.RecentMessages)
	}
	if memoryContext.RecentMessages[0].Role != "user" || memoryContext.RecentMessages[0].Content != "how are you" {
		t.Fatalf("unexpected oldest retained recent message %+v", memoryContext.RecentMessages[0])
	}
	if memoryContext.RecentMessages[3].Role != "assistant" || memoryContext.RecentMessages[3].Content != "goodbye" {
		t.Fatalf("unexpected latest retained recent message %+v", memoryContext.RecentMessages[3])
	}
}

func TestInMemoryMemoryStoreCanRecallUserScopedHistory(t *testing.T) {
	store := NewInMemoryMemoryStore(4)
	ctx := context.Background()

	if err := store.SaveTurn(ctx, MemoryRecord{
		SessionID:    "sess_1",
		DeviceID:     "shared-panel",
		UserID:       "alice",
		HouseholdID:  "home-1",
		UserText:     "打开客厅灯",
		ResponseText: "好的，已经打开客厅灯。",
	}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}
	if err := store.SaveTurn(ctx, MemoryRecord{
		SessionID:    "sess_2",
		DeviceID:     "shared-panel",
		UserID:       "alice",
		HouseholdID:  "home-1",
		UserText:     "再暗一点",
		ResponseText: "好的，已经调暗一些。",
	}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}

	memoryContext, err := store.LoadTurnContext(ctx, MemoryQuery{
		UserID:      "alice",
		HouseholdID: "home-1",
	})
	if err != nil {
		t.Fatalf("LoadTurnContext failed: %v", err)
	}
	if memoryContext.Scope != "user alice" {
		t.Fatalf("expected user-scoped recall, got %q", memoryContext.Scope)
	}
	if len(memoryContext.RecentMessages) != 4 {
		t.Fatalf("expected 4 recent messages, got %+v", memoryContext.RecentMessages)
	}
	foundHouseholdFact := false
	for _, fact := range memoryContext.Facts {
		if fact.Key == "household_turn_count" && fact.Value == "2" {
			foundHouseholdFact = true
		}
	}
	if !foundHouseholdFact {
		t.Fatalf("expected household scope fact in %+v", memoryContext.Facts)
	}
}

func TestBuiltinToolBackendListsAndInvokesTools(t *testing.T) {
	store := NewInMemoryMemoryStore(4)
	ctx := context.Background()
	if err := store.SaveTurn(ctx, MemoryRecord{SessionID: "sess_1", DeviceID: "rtos-1", UserText: "hello", ResponseText: "hi"}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}

	backend := NewBuiltinToolBackend(store)
	backend.Now = func() time.Time {
		return time.Date(2026, 3, 31, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))
	}

	tools, err := backend.ListTools(ctx, ToolCatalogRequest{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 builtin tools, got %+v", tools)
	}

	timeResult, err := backend.InvokeTool(ctx, ToolCall{CallID: "tool_time", ToolName: "time.now"})
	if err != nil {
		t.Fatalf("InvokeTool time.now failed: %v", err)
	}
	if timeResult.ToolStatus != "completed" {
		t.Fatalf("expected completed time tool result, got %+v", timeResult)
	}
	if !strings.Contains(timeResult.ToolOutput, "utc_rfc3339") {
		t.Fatalf("expected encoded time output, got %q", timeResult.ToolOutput)
	}

	memoryResult, err := backend.InvokeTool(ctx, ToolCall{CallID: "tool_mem", SessionID: "sess_1", DeviceID: "rtos-1", ToolName: "memory.recall", ToolInput: `{"query":"recent"}`})
	if err != nil {
		t.Fatalf("InvokeTool memory.recall failed: %v", err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(memoryResult.ToolOutput), &payload); err != nil {
		t.Fatalf("memory tool output should be valid json: %v", err)
	}
	if !strings.Contains(payload["summary"].(string), "remembered") {
		t.Fatalf("unexpected memory summary payload %+v", payload)
	}
	if _, ok := payload["facts"].(map[string]any); !ok {
		t.Fatalf("expected facts map in memory output, got %+v", payload)
	}
	if _, ok := payload["recent_messages"].([]any); !ok {
		t.Fatalf("expected recent_messages array in memory output, got %+v", payload)
	}

	sessionResult, err := backend.InvokeTool(ctx, ToolCall{CallID: "tool_session", SessionID: "sess_1", DeviceID: "rtos-1", ClientType: "rtos", ToolName: "session.describe"})
	if err != nil {
		t.Fatalf("InvokeTool session.describe failed: %v", err)
	}
	if !strings.Contains(sessionResult.ToolOutput, "sess_1") {
		t.Fatalf("expected session output to mention session id, got %q", sessionResult.ToolOutput)
	}
}
