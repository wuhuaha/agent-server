package voice

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type LoggingSemanticSlotParser struct {
	Inner  SemanticSlotParser
	Logger *slog.Logger
}

func (p LoggingSemanticSlotParser) ParsePreview(ctx context.Context, req SemanticSlotParseRequest) (SemanticSlotParseResult, error) {
	startedAt := time.Now()
	if p.Logger != nil {
		p.Logger.Info("voice semantic slot parser started",
			"session_id", req.SessionID,
			"device_id", req.DeviceID,
			"partial_text", strings.TrimSpace(req.PartialText),
			"stable_prefix", strings.TrimSpace(req.StablePrefix),
			"audio_ms", req.AudioMs,
			"stability", clampUnit(req.Stability),
			"stable_for_ms", req.StableForMs,
			"turn_stage", req.TurnStage,
			"endpoint_hinted", req.EndpointHinted,
			"semantic_intent", normalizeSemanticIntent(req.SemanticIntent),
			"prompt_profile", strings.TrimSpace(req.PromptProfile),
			"prompt_hints_count", len(req.PromptHints),
		)
	}
	if p.Inner == nil {
		err := fmt.Errorf("logging semantic slot parser inner is nil")
		if p.Logger != nil {
			p.Logger.Error("voice semantic slot parser failed",
				"session_id", req.SessionID,
				"device_id", req.DeviceID,
				"elapsed_ms", time.Since(startedAt).Milliseconds(),
				"error", err,
			)
		}
		return SemanticSlotParseResult{}, err
	}

	result, err := p.Inner.ParsePreview(ctx, req)
	if err != nil {
		if p.Logger != nil {
			p.Logger.Error("voice semantic slot parser failed",
				"session_id", req.SessionID,
				"device_id", req.DeviceID,
				"partial_text", strings.TrimSpace(req.PartialText),
				"stable_prefix", strings.TrimSpace(req.StablePrefix),
				"turn_stage", req.TurnStage,
				"semantic_intent", normalizeSemanticIntent(req.SemanticIntent),
				"prompt_profile", strings.TrimSpace(req.PromptProfile),
				"elapsed_ms", time.Since(startedAt).Milliseconds(),
				"error", err,
			)
		}
		return SemanticSlotParseResult{}, err
	}

	if p.Logger != nil {
		p.Logger.Info("voice semantic slot parser completed",
			"session_id", req.SessionID,
			"device_id", req.DeviceID,
			"partial_text", strings.TrimSpace(req.PartialText),
			"stable_prefix", strings.TrimSpace(req.StablePrefix),
			"turn_stage", req.TurnStage,
			"domain", result.Domain,
			"task_family", result.TaskFamily,
			"intent", result.Intent,
			"slot_status", result.SlotStatus,
			"actionability", result.Actionability,
			"clarify_needed", result.ClarifyNeeded,
			"missing_slots", append([]string(nil), result.MissingSlots...),
			"ambiguous_slots", append([]string(nil), result.AmbiguousSlots...),
			"confidence", clampUnit(result.Confidence),
			"reason", strings.TrimSpace(result.Reason),
			"source", strings.TrimSpace(result.Source),
			"elapsed_ms", time.Since(startedAt).Milliseconds(),
		)
	}
	return result, nil
}

func (p LoggingSemanticSlotParser) TranscriptionHintsForSession(sessionID string) TranscriptionHints {
	provider, ok := p.Inner.(TranscriptionHintProvider)
	if !ok {
		return TranscriptionHints{}
	}
	return provider.TranscriptionHintsForSession(sessionID)
}
