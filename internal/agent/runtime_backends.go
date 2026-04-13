package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type InMemoryMemoryStore struct {
	mu             sync.RWMutex
	maxTurns       int
	recentTurnSpan int
	recordsByScope map[string][]MemoryRecord
}

func NewInMemoryMemoryStore(maxTurns int) *InMemoryMemoryStore {
	if maxTurns <= 0 {
		maxTurns = 8
	}
	recentTurnSpan := 4
	if maxTurns < recentTurnSpan {
		recentTurnSpan = maxTurns
	}
	return &InMemoryMemoryStore{
		maxTurns:       maxTurns,
		recentTurnSpan: recentTurnSpan,
		recordsByScope: make(map[string][]MemoryRecord),
	}
}

func (s *InMemoryMemoryStore) LoadTurnContext(_ context.Context, query MemoryQuery) (MemoryContext, error) {
	s.mu.RLock()
	scopeRecords := make([]memoryScopedRecords, 0, 5)
	for _, scope := range memoryQueryScopes(query) {
		scopeRecords = append(scopeRecords, memoryScopedRecords{
			scope:   scope,
			records: append([]MemoryRecord(nil), s.recordsByScope[scope.Key]...),
		})
	}
	s.mu.RUnlock()

	var primary memoryScopedRecords
	var available []memoryScopedRecords
	for _, scoped := range scopeRecords {
		if len(scoped.records) == 0 {
			continue
		}
		available = append(available, scoped)
		if primary.scope.Key == "" {
			primary = scoped
		}
	}
	if primary.scope.Key == "" {
		return MemoryContext{}, nil
	}

	latest := primary.records[len(primary.records)-1]
	latestAssistant := memoryAssistantText(latest)
	summary := fmt.Sprintf("remembered %d turn(s) for %s. most recent user text: %q. most recent heard response: %q.", len(primary.records), primary.scope.Label, latest.UserText, latestAssistant)
	facts := []MemoryFact{
		{Key: "recent_turn_count", Value: fmt.Sprintf("%d", len(primary.records)), Source: "in_memory"},
		{Key: "memory_scope", Value: primary.scope.Label, Source: "in_memory"},
		{Key: "memory_scope_kind", Value: primary.scope.Kind, Source: "in_memory"},
	}
	if strings.TrimSpace(latest.UserText) != "" {
		facts = append(facts, MemoryFact{Key: "last_user_text", Value: latest.UserText, Source: "in_memory"})
	}
	if strings.TrimSpace(latest.ResponseText) != "" {
		facts = append(facts, MemoryFact{Key: "last_response_text", Value: latest.ResponseText, Source: "in_memory"})
	}
	if latestAssistant != "" {
		facts = append(facts, MemoryFact{Key: "last_heard_text", Value: latestAssistant, Source: "in_memory"})
	}

	for _, scoped := range available {
		facts = append(facts, MemoryFact{
			Key:    fmt.Sprintf("%s_turn_count", scoped.scope.Kind),
			Value:  fmt.Sprintf("%d", len(scoped.records)),
			Source: "in_memory",
		})
	}

	for idx := len(primary.records) - 1; idx >= 0 && len(primary.records)-idx <= 3; idx-- {
		record := primary.records[idx]
		ordinal := len(primary.records) - idx
		if strings.TrimSpace(record.UserText) != "" {
			facts = append(facts, MemoryFact{Key: fmt.Sprintf("recent_user_%d", ordinal), Value: record.UserText, Source: "in_memory"})
		}
		if assistantText := memoryAssistantText(record); assistantText != "" {
			facts = append(facts, MemoryFact{Key: fmt.Sprintf("recent_response_%d", ordinal), Value: assistantText, Source: "in_memory"})
		}
	}

	return MemoryContext{
		Scope:          primary.scope.Label,
		Summary:        summary,
		Facts:          facts,
		RecentMessages: s.recentMessages(primary.records),
	}, nil
}

func (s *InMemoryMemoryStore) SaveTurn(_ context.Context, record MemoryRecord) error {
	scopes := memoryRecordScopes(record)
	if len(scopes) == 0 {
		return nil
	}

	cloned := cloneMemoryRecord(record)
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, scope := range scopes {
		existing := append([]MemoryRecord(nil), s.recordsByScope[scope.Key]...)
		updated := false
		if cloned.TurnID != "" {
			for index := range existing {
				if existing[index].TurnID == cloned.TurnID {
					existing[index] = cloned
					updated = true
					break
				}
			}
		}
		if !updated {
			existing = append(existing, cloned)
		}
		if len(existing) > s.maxTurns {
			existing = append([]MemoryRecord(nil), existing[len(existing)-s.maxTurns:]...)
		}
		s.recordsByScope[scope.Key] = existing
	}
	return nil
}

func (s *InMemoryMemoryStore) recentMessages(records []MemoryRecord) []ChatMessage {
	if len(records) == 0 {
		return nil
	}
	start := len(records) - s.recentTurnSpan
	if start < 0 {
		start = 0
	}
	messages := make([]ChatMessage, 0, (len(records)-start)*2)
	for _, record := range records[start:] {
		if strings.TrimSpace(record.UserText) != "" {
			messages = append(messages, ChatMessage{
				Role:    "user",
				Content: record.UserText,
			})
		}
		if assistantText := memoryAssistantText(record); assistantText != "" {
			messages = append(messages, ChatMessage{
				Role:    "assistant",
				Content: assistantText,
			})
		}
	}
	return messages
}

type memoryScopedRecords struct {
	scope   memoryScope
	records []MemoryRecord
}

type BuiltinToolBackend struct {
	MemoryStore MemoryStore
	Now         func() time.Time
}

func NewBuiltinToolBackend(memoryStore MemoryStore) *BuiltinToolBackend {
	if memoryStore == nil {
		memoryStore = NewNoopMemoryStore()
	}
	return &BuiltinToolBackend{
		MemoryStore: memoryStore,
		Now:         time.Now,
	}
}

func (b *BuiltinToolBackend) ListTools(context.Context, ToolCatalogRequest) ([]ToolDefinition, error) {
	return []ToolDefinition{
		{
			Name:        "time.now",
			Description: "Return the server's current local and UTC time.",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			Name:        "session.describe",
			Description: "Return the current session, device, and client identifiers.",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			Name:        "memory.recall",
			Description: "Return the in-memory turn summary remembered for the current device or session.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Optional recall hint or topic to bias the memory lookup.",
					},
				},
				"additionalProperties": false,
			},
		},
	}, nil
}

func (b *BuiltinToolBackend) InvokeTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	switch strings.ToLower(strings.TrimSpace(call.ToolName)) {
	case "time.now":
		return b.invokeTimeNow(call)
	case "session.describe":
		return b.invokeSessionDescribe(call)
	case "memory.recall":
		return b.invokeMemoryRecall(ctx, call)
	default:
		return ToolResult{
			CallID:     call.CallID,
			ToolName:   call.ToolName,
			ToolStatus: "unavailable",
			ToolOutput: encodeToolOutput(map[string]any{"error": fmt.Sprintf("tool %s is not available", call.ToolName)}),
		}, nil
	}
}

func (b *BuiltinToolBackend) invokeTimeNow(call ToolCall) (ToolResult, error) {
	nowFn := b.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	return ToolResult{
		CallID:     call.CallID,
		ToolName:   call.ToolName,
		ToolStatus: "completed",
		ToolOutput: encodeToolOutput(map[string]any{
			"utc_rfc3339":   now.UTC().Format(time.RFC3339),
			"local_rfc3339": now.Format(time.RFC3339),
			"timezone":      now.Location().String(),
			"unix":          now.Unix(),
		}),
	}, nil
}

func (b *BuiltinToolBackend) invokeSessionDescribe(call ToolCall) (ToolResult, error) {
	return ToolResult{
		CallID:     call.CallID,
		ToolName:   call.ToolName,
		ToolStatus: "completed",
		ToolOutput: encodeToolOutput(map[string]any{
			"session_id":  call.SessionID,
			"device_id":   call.DeviceID,
			"client_type": call.ClientType,
		}),
	}, nil
}

func (b *BuiltinToolBackend) invokeMemoryRecall(ctx context.Context, call ToolCall) (ToolResult, error) {
	queryText, err := extractMemoryQuery(call.ToolInput)
	if err != nil {
		return ToolResult{
			CallID:     call.CallID,
			ToolName:   call.ToolName,
			ToolStatus: "failed",
			ToolOutput: encodeToolOutput(map[string]any{"error": err.Error()}),
		}, nil
	}
	memoryContext, err := b.MemoryStore.LoadTurnContext(ctx, MemoryQuery{
		SessionID:  call.SessionID,
		DeviceID:   call.DeviceID,
		ClientType: call.ClientType,
		UserText:   queryText,
	})
	if err != nil {
		return ToolResult{}, err
	}

	facts := make(map[string]string, len(memoryContext.Facts))
	for _, fact := range memoryContext.Facts {
		facts[fact.Key] = fact.Value
	}
	return ToolResult{
		CallID:     call.CallID,
		ToolName:   call.ToolName,
		ToolStatus: "completed",
		ToolOutput: encodeToolOutput(map[string]any{
			"scope":           memoryContext.Scope,
			"summary":         memoryContext.Summary,
			"facts":           facts,
			"recent_messages": memoryContext.RecentMessages,
		}),
	}, nil
}

func (b *BuiltinToolBackend) hasTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "time.now", "session.describe", "memory.recall":
		return true
	default:
		return false
	}
}

func extractMemoryQuery(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return "", nil
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", fmt.Errorf("invalid tool input json: %w", err)
	}
	if query, ok := payload["query"].(string); ok {
		return strings.TrimSpace(query), nil
	}
	return "", nil
}

func encodeToolOutput(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(encoded)
}

func cloneMemoryRecord(record MemoryRecord) MemoryRecord {
	cloned := record
	if len(record.Metadata) > 0 {
		cloned.Metadata = make(map[string]string, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func memoryAssistantText(record MemoryRecord) string {
	if heard := strings.TrimSpace(record.HeardText); heard != "" {
		return heard
	}
	if record.PlaybackCompleted {
		if delivered := strings.TrimSpace(record.DeliveredText); delivered != "" {
			return delivered
		}
		if response := strings.TrimSpace(record.ResponseText); response != "" {
			return response
		}
	}
	return ""
}

type memoryScope struct {
	Kind  string
	ID    string
	Key   string
	Label string
}

func memoryQueryScopes(query MemoryQuery) []memoryScope {
	return memoryScopes(query.SessionID, query.DeviceID, query.UserID, query.RoomID, query.HouseholdID)
}

func memoryRecordScopes(record MemoryRecord) []memoryScope {
	return memoryScopes(record.SessionID, record.DeviceID, record.UserID, record.RoomID, record.HouseholdID)
}

func memoryScopes(sessionID, deviceID, userID, roomID, householdID string) []memoryScope {
	scopes := make([]memoryScope, 0, 5)
	appendScope := func(kind, id string) {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return
		}
		scope := memoryScope{
			Kind:  kind,
			ID:    trimmed,
			Key:   kind + ":" + trimmed,
			Label: kind + " " + trimmed,
		}
		scopes = append(scopes, scope)
	}

	appendScope("session", sessionID)
	appendScope("user", userID)
	appendScope("device", deviceID)
	appendScope("room", roomID)
	appendScope("household", householdID)
	return dedupeMemoryScopes(scopes)
}

func dedupeMemoryScopes(scopes []memoryScope) []memoryScope {
	if len(scopes) <= 1 {
		return scopes
	}
	seen := make(map[string]struct{}, len(scopes))
	deduped := make([]memoryScope, 0, len(scopes))
	for _, scope := range scopes {
		if _, ok := seen[scope.Key]; ok {
			continue
		}
		seen[scope.Key] = struct{}{}
		deduped = append(deduped, scope)
	}
	return deduped
}
