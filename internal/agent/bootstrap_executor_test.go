package agent

import (
	"context"
	"strings"
	"testing"
)

type recordingMemoryStore struct {
	loadQueries []MemoryQuery
	saveRecords []MemoryRecord
	loadContext MemoryContext
}

func (m *recordingMemoryStore) LoadTurnContext(_ context.Context, query MemoryQuery) (MemoryContext, error) {
	m.loadQueries = append(m.loadQueries, query)
	if m.loadContext.Summary != "" || m.loadContext.Scope != "" || len(m.loadContext.Facts) > 0 || len(m.loadContext.RecentMessages) > 0 {
		return m.loadContext, nil
	}
	return MemoryContext{Summary: "bootstrap"}, nil
}

func (m *recordingMemoryStore) SaveTurn(_ context.Context, record MemoryRecord) error {
	m.saveRecords = append(m.saveRecords, record)
	return nil
}

type staticToolRegistry struct {
	tools []ToolDefinition
}

func (r staticToolRegistry) ListTools(context.Context, ToolCatalogRequest) ([]ToolDefinition, error) {
	return append([]ToolDefinition(nil), r.tools...), nil
}

type recordingToolInvoker struct {
	calls  []ToolCall
	result ToolResult
}

func (i *recordingToolInvoker) InvokeTool(_ context.Context, call ToolCall) (ToolResult, error) {
	i.calls = append(i.calls, call)
	return i.result, nil
}

type recordingTurnDeltaSink struct {
	deltas []TurnDelta
}

func (s *recordingTurnDeltaSink) EmitTurnDelta(_ context.Context, delta TurnDelta) error {
	s.deltas = append(s.deltas, delta)
	return nil
}

func TestBootstrapTurnExecutorForText(t *testing.T) {
	executor := NewBootstrapTurnExecutor()

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{UserText: "hello"})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if !strings.Contains(output.Text, "hello") {
		t.Fatalf("expected response text to mention input, got %q", output.Text)
	}
	if output.EndSession {
		t.Fatal("did not expect session end directive for ordinary text")
	}
	if len(output.Deltas) != 1 || output.Deltas[0].Kind != TurnDeltaKindText {
		t.Fatalf("expected one text delta, got %+v", output.Deltas)
	}
}

func TestBootstrapTurnExecutorDoesNotOwnHouseholdControlRules(t *testing.T) {
	executor := NewBootstrapTurnExecutor()

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		UserText: "把灯打开",
		Metadata: map[string]string{
			"room_name": "客厅",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if !strings.Contains(output.Text, "把灯打开") {
		t.Fatalf("expected bootstrap echo path, got %q", output.Text)
	}
	if len(output.Deltas) != 1 || output.Deltas[0].Kind != TurnDeltaKindText {
		t.Fatalf("expected one text delta, got %+v", output.Deltas)
	}
}

func TestBootstrapTurnExecutorForAudio(t *testing.T) {
	executor := NewBootstrapTurnExecutor()

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		Audio: AudioInput{
			Present: true,
			Frames:  5,
			Bytes:   3200,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if !strings.Contains(output.Text, "5 audio frames") {
		t.Fatalf("expected audio summary in response text, got %q", output.Text)
	}
	if len(output.Deltas) != 1 || output.Deltas[0].Text == "" {
		t.Fatalf("expected non-empty text delta, got %+v", output.Deltas)
	}
}

func TestBootstrapTurnExecutorCanEndSession(t *testing.T) {
	executor := NewBootstrapTurnExecutor()

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{UserText: "结束对话"})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if !output.EndSession {
		t.Fatal("expected session end directive")
	}
	if output.EndReason != "completed" {
		t.Fatalf("expected completed reason, got %q", output.EndReason)
	}
}

func TestBootstrapTurnExecutorUsesMemoryHooks(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	executor := NewBootstrapTurnExecutor().WithMemoryStore(memoryStore)

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess_123",
		DeviceID:   "rtos-001",
		ClientType: "rtos",
		UserText:   "hello",
		Metadata: map[string]string{
			"source": "test",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if output.Text == "" {
		t.Fatal("expected text output")
	}
	if len(memoryStore.loadQueries) != 1 {
		t.Fatalf("expected one memory load, got %d", len(memoryStore.loadQueries))
	}
	if len(memoryStore.saveRecords) != 1 {
		t.Fatalf("expected one memory save, got %d", len(memoryStore.saveRecords))
	}
	if got := memoryStore.loadQueries[0].SessionID; got != "sess_123" {
		t.Fatalf("unexpected memory load session id %q", got)
	}
	if got := memoryStore.saveRecords[0].ResponseText; got != output.Text {
		t.Fatalf("expected saved response text %q, got %q", output.Text, got)
	}
	if got := memoryStore.saveRecords[0].Metadata["source"]; got != "test" {
		t.Fatalf("expected saved metadata source=test, got %q", got)
	}
}

func TestBootstrapTurnExecutorMemoryCommandUsesRealMemoryStore(t *testing.T) {
	store := NewInMemoryMemoryStore(4)
	executor := NewBootstrapTurnExecutor().WithMemoryStore(store)

	if _, err := executor.ExecuteTurn(context.Background(), TurnInput{SessionID: "sess_1", DeviceID: "rtos-1", UserText: "hello"}); err != nil {
		t.Fatalf("ExecuteTurn seed turn failed: %v", err)
	}

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{SessionID: "sess_2", DeviceID: "rtos-1", UserText: "/memory"})
	if err != nil {
		t.Fatalf("ExecuteTurn /memory failed: %v", err)
	}
	if !strings.Contains(output.Text, "remembered 1 turn(s)") {
		t.Fatalf("expected remembered turn summary, got %q", output.Text)
	}
	if len(output.Deltas) != 1 || output.Deltas[0].Kind != TurnDeltaKindText {
		t.Fatalf("expected one text delta for /memory, got %+v", output.Deltas)
	}

	memoryContext, err := store.LoadTurnContext(context.Background(), MemoryQuery{DeviceID: "rtos-1"})
	if err != nil {
		t.Fatalf("LoadTurnContext failed: %v", err)
	}
	if strings.Contains(memoryContext.Summary, "/memory") {
		t.Fatalf("expected /memory command to stay out of persisted memory, got %q", memoryContext.Summary)
	}
}

func TestBootstrapTurnExecutorEmitsToolDeltasForToolCommand(t *testing.T) {
	memoryStore := &recordingMemoryStore{}
	toolInvoker := &recordingToolInvoker{result: ToolResult{
		ToolStatus: "completed",
		ToolOutput: `{"events":1}`,
	}}
	executor := NewBootstrapTurnExecutor().
		WithMemoryStore(memoryStore).
		WithToolRegistry(staticToolRegistry{tools: []ToolDefinition{{Name: "calendar.lookup"}}}).
		WithToolInvoker(toolInvoker)

	output, err := executor.ExecuteTurn(context.Background(), TurnInput{
		SessionID:  "sess_tool",
		DeviceID:   "rtos-001",
		ClientType: "rtos",
		UserText:   `/tool calendar.lookup {"date":"2026-03-31"}`,
	})
	if err != nil {
		t.Fatalf("ExecuteTurn failed: %v", err)
	}
	if len(toolInvoker.calls) != 1 {
		t.Fatalf("expected one tool invocation, got %d", len(toolInvoker.calls))
	}
	if got := toolInvoker.calls[0].ToolName; got != "calendar.lookup" {
		t.Fatalf("unexpected tool name %q", got)
	}
	if len(memoryStore.saveRecords) != 0 {
		t.Fatalf("expected bootstrap tool command to skip memory persistence, got %+v", memoryStore.saveRecords)
	}
	if len(output.Deltas) != 3 {
		t.Fatalf("expected 3 deltas, got %+v", output.Deltas)
	}
	if output.Deltas[0].Kind != TurnDeltaKindToolCall {
		t.Fatalf("expected first delta tool_call, got %+v", output.Deltas[0])
	}
	if output.Deltas[1].Kind != TurnDeltaKindToolResult {
		t.Fatalf("expected second delta tool_result, got %+v", output.Deltas[1])
	}
	if output.Deltas[2].Kind != TurnDeltaKindText {
		t.Fatalf("expected third delta text, got %+v", output.Deltas[2])
	}
	if got := output.Deltas[0].ToolInput; got != `{"date":"2026-03-31"}` {
		t.Fatalf("unexpected tool input %q", got)
	}
	if got := output.Deltas[1].ToolOutput; got != `{"events":1}` {
		t.Fatalf("unexpected tool output %q", got)
	}
	if !strings.Contains(output.Text, "calendar.lookup") {
		t.Fatalf("expected tool summary text, got %q", output.Text)
	}
}

func TestBootstrapTurnExecutorStreamTurnEmitsWithoutMaterializingOutputDeltas(t *testing.T) {
	sink := &recordingTurnDeltaSink{}
	executor := NewBootstrapTurnExecutor()

	output, err := executor.StreamTurn(context.Background(), TurnInput{UserText: "hello"}, sink)
	if err != nil {
		t.Fatalf("StreamTurn failed: %v", err)
	}
	if len(output.Deltas) != 0 {
		t.Fatalf("expected StreamTurn output to omit materialized deltas, got %+v", output.Deltas)
	}
	if len(sink.deltas) != 1 || sink.deltas[0].Kind != TurnDeltaKindText {
		t.Fatalf("expected streamed text delta, got %+v", sink.deltas)
	}
}
