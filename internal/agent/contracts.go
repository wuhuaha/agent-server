package agent

import "context"

type TurnDeltaKind string

const (
	TurnDeltaKindText       TurnDeltaKind = "text"
	TurnDeltaKindToolCall   TurnDeltaKind = "tool_call"
	TurnDeltaKindToolResult TurnDeltaKind = "tool_result"
)

type AudioInput struct {
	Present      bool
	Frames       int
	Bytes        int
	Codec        string
	SampleRateHz int
	Channels     int
}

type ImageInput struct {
	Name string
	URL  string
	MIME string
}

type TurnInput struct {
	SessionID  string
	TurnID     string
	TraceID    string
	DeviceID   string
	ClientType string
	UserText   string
	Audio      AudioInput
	Images     []ImageInput
	Metadata   map[string]string
}

type TurnDelta struct {
	Kind       TurnDeltaKind
	Text       string
	ToolCallID string
	ToolName   string
	ToolStatus string
	ToolInput  string
	ToolOutput string
}

type TurnOutput struct {
	Text       string
	Deltas     []TurnDelta
	EndSession bool
	EndReason  string
	EndMessage string
}

type TurnExecutor interface {
	ExecuteTurn(context.Context, TurnInput) (TurnOutput, error)
}

type TurnDeltaSink interface {
	EmitTurnDelta(context.Context, TurnDelta) error
}

type TurnDeltaSinkFunc func(context.Context, TurnDelta) error

func (f TurnDeltaSinkFunc) EmitTurnDelta(ctx context.Context, delta TurnDelta) error {
	return f(ctx, delta)
}

type StreamingTurnExecutor interface {
	StreamTurn(context.Context, TurnInput, TurnDeltaSink) (TurnOutput, error)
}

type MemoryQuery struct {
	SessionID   string
	DeviceID    string
	ClientType  string
	UserID      string
	RoomID      string
	HouseholdID string
	UserText    string
	Metadata    map[string]string
}

type MemoryFact struct {
	Key    string
	Value  string
	Source string
}

type MemoryContext struct {
	Scope          string
	Summary        string
	Facts          []MemoryFact
	RecentMessages []ChatMessage
}

type MemoryRecord struct {
	TurnID              string
	SessionID           string
	DeviceID            string
	ClientType          string
	UserID              string
	RoomID              string
	HouseholdID         string
	UserText            string
	ResponseText        string
	DeliveredText       string
	HeardText           string
	ResponseInterrupted bool
	ResponseTruncated   bool
	PlaybackCompleted   bool
	Metadata            map[string]string
}

type MemoryStore interface {
	LoadTurnContext(context.Context, MemoryQuery) (MemoryContext, error)
	SaveTurn(context.Context, MemoryRecord) error
}

type ToolCatalogRequest struct {
	SessionID  string
	DeviceID   string
	ClientType string
}

type SkillPromptRequest struct {
	SessionID  string
	DeviceID   string
	ClientType string
	UserText   string
	Metadata   map[string]string
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Strict      bool
}

type ToolRegistry interface {
	ListTools(context.Context, ToolCatalogRequest) ([]ToolDefinition, error)
}

type SkillPromptProvider interface {
	ListPromptFragments(context.Context, SkillPromptRequest) ([]string, error)
}

type ToolCall struct {
	CallID     string
	SessionID  string
	DeviceID   string
	ClientType string
	ToolName   string
	ToolInput  string
}

type ToolResult struct {
	CallID     string
	ToolName   string
	ToolStatus string
	ToolOutput string
}

type ToolInvoker interface {
	InvokeTool(context.Context, ToolCall) (ToolResult, error)
}
