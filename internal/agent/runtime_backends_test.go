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

	if err := store.SaveTurn(ctx, MemoryRecord{SessionID: "sess_1", TurnID: "turn_1", DeviceID: "rtos-1", UserText: "hello", ResponseText: "hi", DeliveredText: "hi", HeardText: "hi", PlaybackCompleted: true}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}
	if err := store.SaveTurn(ctx, MemoryRecord{SessionID: "sess_2", TurnID: "turn_2", DeviceID: "rtos-1", UserText: "how are you", ResponseText: "fine", DeliveredText: "fine", HeardText: "fine", PlaybackCompleted: true}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}
	if err := store.SaveTurn(ctx, MemoryRecord{SessionID: "sess_3", TurnID: "turn_3", DeviceID: "rtos-1", UserText: "bye", ResponseText: "goodbye", DeliveredText: "goodbye", HeardText: "goodbye", PlaybackCompleted: true}); err != nil {
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
		SessionID:         "sess_1",
		TurnID:            "turn_1",
		DeviceID:          "shared-panel",
		UserID:            "alice",
		HouseholdID:       "home-1",
		UserText:          "打开客厅灯",
		ResponseText:      "好的，已经打开客厅灯。",
		DeliveredText:     "好的，已经打开客厅灯。",
		HeardText:         "好的，已经打开客厅灯。",
		PlaybackCompleted: true,
	}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}
	if err := store.SaveTurn(ctx, MemoryRecord{
		SessionID:         "sess_2",
		TurnID:            "turn_2",
		DeviceID:          "shared-panel",
		UserID:            "alice",
		HouseholdID:       "home-1",
		UserText:          "再暗一点",
		ResponseText:      "好的，已经调暗一些。",
		DeliveredText:     "好的，已经调暗一些。",
		HeardText:         "好的，已经调暗一些。",
		PlaybackCompleted: true,
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
	if err := store.SaveTurn(ctx, MemoryRecord{SessionID: "sess_1", TurnID: "turn_1", DeviceID: "rtos-1", UserText: "hello", ResponseText: "hi", DeliveredText: "hi", HeardText: "hi", PlaybackCompleted: true}); err != nil {
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

func TestBuiltinToolBackendCanExposeHouseholdControlSkill(t *testing.T) {
	backend := NewRuntimeToolBackend(NewNoopMemoryStore(), []string{"household_control"})

	tools, err := backend.ListTools(context.Background(), ToolCatalogRequest{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	foundTool := false
	for _, tool := range tools {
		if tool.Name == householdControlSimulationToolName {
			foundTool = true
			break
		}
	}
	if !foundTool {
		t.Fatalf("expected household control tool in %+v", tools)
	}

	fragments, err := backend.ListPromptFragments(context.Background(), SkillPromptRequest{})
	if err != nil {
		t.Fatalf("ListPromptFragments failed: %v", err)
	}
	if len(fragments) != 1 || !strings.Contains(fragments[0], householdControlSimulationToolName) {
		t.Fatalf("expected household skill prompt fragment, got %+v", fragments)
	}

	result, err := backend.InvokeTool(context.Background(), ToolCall{
		CallID:    "tool_home",
		ToolName:  householdControlSimulationToolName,
		ToolInput: `{"room_name":"客厅","device_type":"light","action":"on","utterance_summary":"打开客厅灯"}`,
	})
	if err != nil {
		t.Fatalf("InvokeTool household control failed: %v", err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(result.ToolOutput), &payload); err != nil {
		t.Fatalf("household tool output should be valid json: %v", err)
	}
	if payload["device_type"] != "light" {
		t.Fatalf("expected normalized device_type light, got %+v", payload)
	}
	if payload["goal"] != "打开客厅灯光" {
		t.Fatalf("expected normalized goal, got %+v", payload)
	}
}

func TestInMemoryMemoryStoreUsesHeardTextForRecentMessages(t *testing.T) {
	store := NewInMemoryMemoryStore(4)
	ctx := context.Background()

	if err := store.SaveTurn(ctx, MemoryRecord{
		SessionID:           "sess_1",
		TurnID:              "turn_1",
		DeviceID:            "rtos-1",
		UserText:            "讲个笑话",
		ResponseText:        "这里有一个很长很长的笑话。",
		DeliveredText:       "这里有一个很长很长的笑话。",
		HeardText:           "这里有一个很长",
		ResponseInterrupted: true,
		ResponseTruncated:   true,
		PlaybackCompleted:   false,
	}); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}

	memoryContext, err := store.LoadTurnContext(ctx, MemoryQuery{DeviceID: "rtos-1"})
	if err != nil {
		t.Fatalf("LoadTurnContext failed: %v", err)
	}
	if len(memoryContext.RecentMessages) != 2 {
		t.Fatalf("expected user and partial assistant messages, got %+v", memoryContext.RecentMessages)
	}
	if got := memoryContext.RecentMessages[1].Content; got != "这里有一个很长" {
		t.Fatalf("expected heard assistant text, got %q", got)
	}
}

func TestInMemoryMemoryStoreUpsertsTurnByTurnID(t *testing.T) {
	store := NewInMemoryMemoryStore(4)
	ctx := context.Background()

	record := MemoryRecord{
		SessionID:     "sess_1",
		TurnID:        "turn_1",
		DeviceID:      "rtos-1",
		UserText:      "打开客厅灯",
		ResponseText:  "好的，已经打开客厅灯。",
		DeliveredText: "好的，已经打开客厅灯。",
	}
	if err := store.SaveTurn(ctx, record); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}
	record.HeardText = "好的，已经打开"
	record.ResponseInterrupted = true
	record.ResponseTruncated = true
	if err := store.SaveTurn(ctx, record); err != nil {
		t.Fatalf("SaveTurn update failed: %v", err)
	}

	memoryContext, err := store.LoadTurnContext(ctx, MemoryQuery{DeviceID: "rtos-1"})
	if err != nil {
		t.Fatalf("LoadTurnContext failed: %v", err)
	}
	if len(memoryContext.RecentMessages) != 2 {
		t.Fatalf("expected one updated turn in recent messages, got %+v", memoryContext.RecentMessages)
	}
	if got := memoryContext.RecentMessages[1].Content; got != "好的，已经打开" {
		t.Fatalf("expected updated heard text, got %q", got)
	}
}
