package voice

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type LoggingSemanticTurnJudge struct {
	Inner  SemanticTurnJudge
	Logger *slog.Logger
}

func (j LoggingSemanticTurnJudge) JudgePreview(ctx context.Context, req SemanticTurnRequest) (SemanticTurnJudgement, error) {
	startedAt := time.Now()
	if j.Logger != nil {
		j.Logger.Info("voice semantic judge started",
			"session_id", req.SessionID,
			"device_id", req.DeviceID,
			"partial_text", strings.TrimSpace(req.PartialText),
			"stable_prefix", strings.TrimSpace(req.StablePrefix),
			"audio_ms", req.AudioMs,
			"stability", clampUnit(req.Stability),
			"stable_for_ms", req.StableForMs,
			"turn_stage", req.TurnStage,
			"endpoint_hinted", req.EndpointHinted,
			"task_family_hint", normalizeSemanticTaskFamily(req.TaskFamilyHint),
			"slot_constraint_required", req.SlotConstraintRequired,
		)
	}
	if j.Inner == nil {
		err := fmt.Errorf("logging semantic judge inner is nil")
		if j.Logger != nil {
			j.Logger.Error("voice semantic judge failed",
				"session_id", req.SessionID,
				"device_id", req.DeviceID,
				"elapsed_ms", time.Since(startedAt).Milliseconds(),
				"error", err,
			)
		}
		return SemanticTurnJudgement{}, err
	}

	result, err := j.Inner.JudgePreview(ctx, req)
	if err != nil {
		if j.Logger != nil {
			j.Logger.Error("voice semantic judge failed",
				"session_id", req.SessionID,
				"device_id", req.DeviceID,
				"partial_text", strings.TrimSpace(req.PartialText),
				"stable_prefix", strings.TrimSpace(req.StablePrefix),
				"turn_stage", req.TurnStage,
				"elapsed_ms", time.Since(startedAt).Milliseconds(),
				"error", err,
			)
		}
		return SemanticTurnJudgement{}, err
	}

	if j.Logger != nil {
		j.Logger.Info("voice semantic judge completed",
			"session_id", req.SessionID,
			"device_id", req.DeviceID,
			"partial_text", strings.TrimSpace(req.PartialText),
			"stable_prefix", strings.TrimSpace(req.StablePrefix),
			"turn_stage", req.TurnStage,
			"utterance_status", result.UtteranceStatus,
			"interruption_intent", result.InterruptionIntent,
			"task_family", result.TaskFamily,
			"slot_readiness_hint", result.SlotReadinessHint,
			"wait_delta_ms", result.WaitDeltaMs,
			"confidence", clampUnit(result.Confidence),
			"reason", strings.TrimSpace(result.Reason),
			"source", strings.TrimSpace(result.Source),
			"elapsed_ms", time.Since(startedAt).Milliseconds(),
		)
	}
	return result, nil
}
