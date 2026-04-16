package voice

import (
	"context"
	"testing"

	"agent-server/internal/agent"
)

type staticSemanticChatModel struct {
	text string
}

func (m staticSemanticChatModel) Complete(context.Context, agent.ChatModelRequest) (agent.ChatModelResponse, error) {
	return agent.ChatModelResponse{Text: m.text}, nil
}

func TestLLMSemanticTurnJudgeDecodesJSONResponse(t *testing.T) {
	judge := NewLLMSemanticTurnJudge(staticSemanticChatModel{
		text: "```json\n{\"utterance_status\":\"complete\",\"interruption_intent\":\"takeover\",\"confidence\":0.91,\"reason\":\"new_request_takeover\"}\n```",
	})
	result, err := judge.JudgePreview(context.Background(), SemanticTurnRequest{
		SessionID:   "sess_semantic",
		PartialText: "不要这个",
	})
	if err != nil {
		t.Fatalf("JudgePreview failed: %v", err)
	}
	if result.UtteranceStatus != SemanticUtteranceComplete {
		t.Fatalf("expected complete status, got %+v", result)
	}
	if result.InterruptionIntent != SemanticIntentTakeover {
		t.Fatalf("expected takeover intent, got %+v", result)
	}
	if result.Confidence < 0.9 {
		t.Fatalf("expected high confidence, got %+v", result)
	}
}

func TestMergeSemanticJudgementPromotesDraftAllowed(t *testing.T) {
	snapshot := InputPreview{
		PartialText:       "明天周几",
		StablePrefix:      "明天周几",
		UtteranceComplete: false,
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageWaitForMore,
			PrewarmAllowed: false,
			DraftAllowed:   false,
		},
	}
	merged := mergeSemanticJudgement(snapshot, SemanticTurnJudgement{
		CandidateKey:       semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		UtteranceStatus:    SemanticUtteranceComplete,
		InterruptionIntent: SemanticIntentQuestion,
		Confidence:         0.86,
		Reason:             "standalone_question",
		Source:             "test",
	})
	if !merged.UtteranceComplete {
		t.Fatalf("expected semantic merge to promote utterance complete, got %+v", merged)
	}
	if merged.Arbitration.Stage != TurnArbitrationStageDraftAllowed {
		t.Fatalf("expected draft_allowed after semantic promotion, got %+v", merged.Arbitration)
	}
	if !merged.Arbitration.DraftAllowed || !merged.Arbitration.PrewarmAllowed {
		t.Fatalf("expected semantic merge to allow prewarm and draft, got %+v", merged.Arbitration)
	}
}

func TestMergeSemanticJudgementKeepsCorrectionPendingConservative(t *testing.T) {
	snapshot := InputPreview{
		PartialText:       "打开客厅灯不对",
		StablePrefix:      "打开客厅灯",
		UtteranceComplete: true,
		Arbitration: TurnArbitration{
			Stage:          TurnArbitrationStageDraftAllowed,
			PrewarmAllowed: true,
			DraftAllowed:   true,
		},
	}
	merged := mergeSemanticJudgement(snapshot, SemanticTurnJudgement{
		CandidateKey:       semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText),
		UtteranceStatus:    SemanticUtteranceCorrection,
		InterruptionIntent: SemanticIntentCorrection,
		Confidence:         0.82,
		Reason:             "repair_in_progress",
		Source:             "test",
	})
	if merged.UtteranceComplete {
		t.Fatalf("expected correction judgement to keep utterance incomplete, got %+v", merged)
	}
	if merged.Arbitration.DraftAllowed {
		t.Fatalf("expected correction judgement to suppress draft, got %+v", merged.Arbitration)
	}
}
