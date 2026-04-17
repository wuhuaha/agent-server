package voice

import (
	"context"
	"fmt"
	"strings"
)

const (
	SemanticSlotPromptProfileOff = "off"
)

type profileAwareSemanticSlotParser struct {
	inner         SemanticSlotParser
	promptProfile string
	promptHints   []string
}

// NewProfileAwareSemanticSlotParser injects only explicit, opt-in prompt hints.
// Shared runtime defaults stay generic when no profile or hints are supplied.
func NewProfileAwareSemanticSlotParser(inner SemanticSlotParser, profile string, hints ...string) SemanticSlotParser {
	if inner == nil {
		return nil
	}
	profile = normalizeSemanticSlotPromptProfile(profile)
	hints = normalizeSemanticSlotPromptHints(hints)
	if profile == "" && len(hints) == 0 {
		return inner
	}
	return profileAwareSemanticSlotParser{
		inner:         inner,
		promptProfile: profile,
		promptHints:   hints,
	}
}

func (p profileAwareSemanticSlotParser) ParsePreview(ctx context.Context, req SemanticSlotParseRequest) (SemanticSlotParseResult, error) {
	if strings.TrimSpace(req.PromptProfile) == "" {
		req.PromptProfile = p.promptProfile
	}
	req.PromptHints = mergeSemanticSlotPromptHints(req.PromptHints, p.promptHints)
	return p.inner.ParsePreview(ctx, req)
}

func normalizeSemanticSlotPromptProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "", SemanticSlotPromptProfileOff, "none", "disabled":
		return ""
	case "seed", "demo", BuiltInEntityCatalogProfileSeedCompanion:
		return BuiltInEntityCatalogProfileSeedCompanion
	default:
		return strings.ToLower(strings.TrimSpace(profile))
	}
}

func semanticSlotPromptHintsBlock(req SemanticSlotParseRequest) string {
	lines := semanticSlotPromptProfileLines(req.PromptProfile)
	lines = mergeSemanticSlotPromptHints(lines, req.PromptHints)
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("当前请求显式附带了以下补充 hints；它们只用于补强当前轮次，不会改变 shared runtime 默认政策：")
	for idx, line := range lines {
		fmt.Fprintf(&b, "\n%d. %s", idx+1, line)
	}
	return b.String()
}

func semanticSlotPromptProfileLines(profile string) []string {
	switch normalizeSemanticSlotPromptProfile(profile) {
	case BuiltInEntityCatalogProfileSeedCompanion:
		return []string{
			"profile=seed_companion：这是一个显式 opt-in 的 research/demo profile，不代表 shared runtime 默认场景。",
			"若用户明显在控制家庭设备或房间内对象，可将 domain 映射到 smart_home，并优先抽取 action / target / location / attribute / value / mode / duration 这类通用槽位。",
			"若用户明显在控制桌面应用、窗口或系统设置，可将 domain 映射到 desktop_assistant，并优先抽取 action / target_app / query / window_name / system_setting / value 这类通用槽位。",
			"即使启用了该 profile，early gate 仍以 task_family 和 slot completeness 为主，不要把 smart_home 或 desktop_assistant 当成政策中心。",
		}
	default:
		return nil
	}
}

func normalizeSemanticSlotPromptHints(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func mergeSemanticSlotPromptHints(base []string, extra []string) []string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	merged := make([]string, 0, len(base)+len(extra))
	merged = append(merged, normalizeSemanticSlotPromptHints(base)...)
	seen := make(map[string]struct{}, len(merged))
	for _, line := range merged {
		seen[line] = struct{}{}
	}
	for _, line := range normalizeSemanticSlotPromptHints(extra) {
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		merged = append(merged, line)
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
