package agent

import (
	"context"
	"strings"
)

type RuntimeSkill interface {
	Name() string
	ListTools(context.Context, ToolCatalogRequest) ([]ToolDefinition, error)
	ListPromptFragments(context.Context, SkillPromptRequest) ([]string, error)
	InvokeTool(context.Context, ToolCall) (ToolResult, bool, error)
}

type RuntimeSkillRegistry struct {
	skillsByName map[string]RuntimeSkill
}

func NewRuntimeSkillRegistry(skillNames []string) *RuntimeSkillRegistry {
	registry := &RuntimeSkillRegistry{
		skillsByName: make(map[string]RuntimeSkill),
	}
	for _, skillName := range skillNames {
		skill := newBuiltinRuntimeSkill(skillName)
		if skill == nil {
			continue
		}
		registry.skillsByName[skill.Name()] = skill
	}
	return registry
}

func (r *RuntimeSkillRegistry) ListTools(ctx context.Context, request ToolCatalogRequest) ([]ToolDefinition, error) {
	tools := make([]ToolDefinition, 0, len(r.skillsByName))
	for _, skill := range r.skillsByName {
		definitions, err := skill.ListTools(ctx, request)
		if err != nil {
			return nil, err
		}
		tools = append(tools, definitions...)
	}
	return tools, nil
}

func (r *RuntimeSkillRegistry) ListPromptFragments(ctx context.Context, request SkillPromptRequest) ([]string, error) {
	fragments := make([]string, 0, len(r.skillsByName))
	for _, skill := range r.skillsByName {
		items, err := skill.ListPromptFragments(ctx, request)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, items...)
	}
	return fragments, nil
}

func (r *RuntimeSkillRegistry) InvokeTool(ctx context.Context, call ToolCall) (ToolResult, bool, error) {
	for _, skill := range r.skillsByName {
		result, handled, err := skill.InvokeTool(ctx, call)
		if handled || err != nil {
			return result, handled, err
		}
	}
	return ToolResult{}, false, nil
}

type RuntimeToolBackend struct {
	base   *BuiltinToolBackend
	skills *RuntimeSkillRegistry
}

func NewRuntimeToolBackend(memoryStore MemoryStore, skillNames []string) *RuntimeToolBackend {
	return &RuntimeToolBackend{
		base:   NewBuiltinToolBackend(memoryStore),
		skills: NewRuntimeSkillRegistry(skillNames),
	}
}

func (b *RuntimeToolBackend) ListTools(ctx context.Context, request ToolCatalogRequest) ([]ToolDefinition, error) {
	baseTools, err := b.base.ListTools(ctx, request)
	if err != nil {
		return nil, err
	}
	skillTools, err := b.skills.ListTools(ctx, request)
	if err != nil {
		return nil, err
	}
	return append(baseTools, skillTools...), nil
}

func (b *RuntimeToolBackend) ListPromptFragments(ctx context.Context, request SkillPromptRequest) ([]string, error) {
	return b.skills.ListPromptFragments(ctx, request)
}

func (b *RuntimeToolBackend) InvokeTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	if b.base.hasTool(call.ToolName) {
		return b.base.InvokeTool(ctx, call)
	}
	if result, handled, err := b.skills.InvokeTool(ctx, call); handled || err != nil {
		return result, err
	}
	return ToolResult{
		CallID:     call.CallID,
		ToolName:   strings.TrimSpace(call.ToolName),
		ToolStatus: "unavailable",
		ToolOutput: encodeToolOutput(map[string]any{"error": "tool " + strings.TrimSpace(call.ToolName) + " is not available"}),
	}, nil
}

func newBuiltinRuntimeSkill(name string) RuntimeSkill {
	switch canonicalBuiltinSkillName(name) {
	case builtinSkillHouseholdControl:
		return HouseholdControlSkill{}
	default:
		return nil
	}
}
