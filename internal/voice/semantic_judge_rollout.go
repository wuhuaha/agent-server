package voice

import (
	"hash/fnv"
	"strings"
)

const (
	SemanticJudgeRolloutModeControl       = "control"
	SemanticJudgeRolloutModeSemantic      = "semantic"
	SemanticJudgeRolloutModeStickyPercent = "sticky_percent"

	SemanticJudgeVariantControl  = "control"
	SemanticJudgeVariantSemantic = "semantic"
)

const semanticJudgeRolloutSalt = "voice.semantic_judge.rollout.v1"

type SemanticJudgeRolloutConfig struct {
	Mode       string
	Percentage int
}

type semanticJudgeRolloutDecision struct {
	Variant string
	Enabled bool
	Bucket  int
}

func NormalizeSemanticJudgeRolloutConfig(cfg SemanticJudgeRolloutConfig) SemanticJudgeRolloutConfig {
	cfg.Mode = NormalizeSemanticJudgeRolloutMode(cfg.Mode)
	return cfg
}

func NormalizeSemanticJudgeRolloutMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "control", "off", "disabled", "false", "0":
		return SemanticJudgeRolloutModeControl
	case "semantic", "enabled", "on", "true", "100":
		return SemanticJudgeRolloutModeSemantic
	case "sticky_percent", "sticky", "percent", "percentage", "rollout", "ab":
		return SemanticJudgeRolloutModeStickyPercent
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func IsSupportedSemanticJudgeRolloutMode(mode string) bool {
	switch NormalizeSemanticJudgeRolloutMode(mode) {
	case SemanticJudgeRolloutModeControl, SemanticJudgeRolloutModeSemantic, SemanticJudgeRolloutModeStickyPercent:
		return true
	default:
		return false
	}
}

func clampSemanticJudgeRolloutPercent(percent int) int {
	switch {
	case percent < 0:
		return 0
	case percent > 100:
		return 100
	default:
		return percent
	}
}

func decideSemanticJudgeRollout(req InputPreviewRequest, cfg SemanticJudgeRolloutConfig, judge SemanticTurnJudge) semanticJudgeRolloutDecision {
	cfg = NormalizeSemanticJudgeRolloutConfig(cfg)
	decision := semanticJudgeRolloutDecision{
		Variant: SemanticJudgeVariantControl,
		Bucket:  -1,
	}
	switch cfg.Mode {
	case SemanticJudgeRolloutModeSemantic:
		decision.Variant = SemanticJudgeVariantSemantic
	case SemanticJudgeRolloutModeStickyPercent:
		decision.Bucket = semanticJudgeRolloutBucket(req)
		if decision.Bucket < clampSemanticJudgeRolloutPercent(cfg.Percentage) {
			decision.Variant = SemanticJudgeVariantSemantic
		}
	default:
		decision.Variant = SemanticJudgeVariantControl
	}
	decision.Enabled = decision.Variant == SemanticJudgeVariantSemantic && judge != nil
	return decision
}

func semanticJudgeRolloutBucket(req InputPreviewRequest) int {
	stickyKey := semanticJudgeStickyKey(req)
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(semanticJudgeRolloutSalt))
	_, _ = hasher.Write([]byte("|"))
	_, _ = hasher.Write([]byte(stickyKey))
	return int(hasher.Sum32() % 100)
}

func semanticJudgeStickyKey(req InputPreviewRequest) string {
	sessionID := strings.TrimSpace(req.SessionID)
	deviceID := strings.TrimSpace(req.DeviceID)
	clientType := strings.TrimSpace(req.ClientType)
	switch {
	case sessionID != "" && deviceID != "":
		return "session:" + sessionID + "|device:" + deviceID
	case sessionID != "":
		return "session:" + sessionID
	case deviceID != "":
		return "device:" + deviceID
	case clientType != "":
		return "client:" + clientType
	default:
		return "preview:anonymous"
	}
}
