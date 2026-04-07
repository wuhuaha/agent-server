package agent

import "context"

type NoopMemoryStore struct{}

func NewNoopMemoryStore() NoopMemoryStore {
	return NoopMemoryStore{}
}

func (NoopMemoryStore) LoadTurnContext(context.Context, MemoryQuery) (MemoryContext, error) {
	return MemoryContext{}, nil
}

func (NoopMemoryStore) SaveTurn(context.Context, MemoryRecord) error {
	return nil
}

type NoopToolRegistry struct{}

func NewNoopToolRegistry() NoopToolRegistry {
	return NoopToolRegistry{}
}

func (NoopToolRegistry) ListTools(context.Context, ToolCatalogRequest) ([]ToolDefinition, error) {
	return nil, nil
}

type NoopToolInvoker struct{}

func NewNoopToolInvoker() NoopToolInvoker {
	return NoopToolInvoker{}
}

func (NoopToolInvoker) InvokeTool(_ context.Context, call ToolCall) (ToolResult, error) {
	return ToolResult{
		CallID:     call.CallID,
		ToolName:   call.ToolName,
		ToolStatus: "unavailable",
		ToolOutput: "tool runtime not configured",
	}, nil
}
