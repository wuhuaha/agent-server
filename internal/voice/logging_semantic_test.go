package voice

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type stubSemanticJudge struct {
	result SemanticTurnJudgement
	err    error
}

func (s stubSemanticJudge) JudgePreview(context.Context, SemanticTurnRequest) (SemanticTurnJudgement, error) {
	if s.err != nil {
		return SemanticTurnJudgement{}, s.err
	}
	return s.result, nil
}

type stubSemanticSlotParser struct {
	result SemanticSlotParseResult
	err    error
	hints  TranscriptionHints
}

func (s stubSemanticSlotParser) ParsePreview(context.Context, SemanticSlotParseRequest) (SemanticSlotParseResult, error) {
	if s.err != nil {
		return SemanticSlotParseResult{}, s.err
	}
	return s.result, nil
}

func (s stubSemanticSlotParser) TranscriptionHintsForSession(string) TranscriptionHints {
	return s.hints
}

func TestLoggingSemanticTurnJudgeLogsStartAndCompletion(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	judge := LoggingSemanticTurnJudge{
		Inner: stubSemanticJudge{
			result: SemanticTurnJudgement{
				UtteranceStatus:    SemanticUtteranceComplete,
				InterruptionIntent: SemanticIntentQuestion,
				TaskFamily:         SemanticTaskFamilyKnowledgeQuery,
				SlotReadinessHint:  SemanticSlotReadinessNotApplicable,
				Confidence:         0.91,
				Reason:             "standalone_question",
				Source:             "test",
			},
		},
		Logger: logger,
	}

	_, err := judge.JudgePreview(context.Background(), SemanticTurnRequest{
		SessionID:    "sess_semantic",
		DeviceID:     "dev_semantic",
		PartialText:  "明天周几",
		StablePrefix: "明天周几",
		AudioMs:      240,
		Stability:    0.88,
		StableForMs:  180,
		TurnStage:    TurnArbitrationStageDraftAllowed,
	})
	if err != nil {
		t.Fatalf("JudgePreview failed: %v", err)
	}

	logs := buf.String()
	for _, want := range []string{
		`"msg":"voice semantic judge started"`,
		`"msg":"voice semantic judge completed"`,
		`"session_id":"sess_semantic"`,
		`"utterance_status":"complete"`,
		`"task_family":"knowledge_query"`,
		`"reason":"standalone_question"`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected logs to contain %s, got:\n%s", want, logs)
		}
	}
}

func TestLoggingSemanticSlotParserLogsAndForwardsHints(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	parser := LoggingSemanticSlotParser{
		Inner: stubSemanticSlotParser{
			result: SemanticSlotParseResult{
				Domain:         SemanticSlotDomainUnknown,
				TaskFamily:     SemanticTaskFamilyStructuredCommand,
				Intent:         "set_timer",
				SlotStatus:     SemanticSlotStatusPartial,
				Actionability:  SemanticSlotActionabilityWaitMore,
				MissingSlots:   []string{"duration"},
				AmbiguousSlots: []string{"target"},
				Confidence:     0.83,
				Reason:         "tail_slot_missing",
				Source:         "test",
			},
			hints: TranscriptionHints{
				Hotwords:    []string{"计时器"},
				HintPhrases: []string{"开始计时"},
			},
		},
		Logger: logger,
	}

	_, err := parser.ParsePreview(context.Background(), SemanticSlotParseRequest{
		SessionID:      "sess_slot",
		DeviceID:       "dev_slot",
		PartialText:    "帮我开始计时",
		StablePrefix:   "帮我开始计时",
		AudioMs:        320,
		Stability:      0.9,
		StableForMs:    200,
		TurnStage:      TurnArbitrationStagePrewarmAllowed,
		SemanticIntent: SemanticIntentRequest,
		PromptProfile:  "seed_companion",
		PromptHints:    []string{"timer"},
	})
	if err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}
	hints := parser.TranscriptionHintsForSession("sess_slot")
	if len(hints.Hotwords) != 1 || hints.Hotwords[0] != "计时器" {
		t.Fatalf("expected hotwords to be forwarded, got %+v", hints)
	}
	if len(hints.HintPhrases) != 1 || hints.HintPhrases[0] != "开始计时" {
		t.Fatalf("expected hint phrases to be forwarded, got %+v", hints)
	}

	logs := buf.String()
	for _, want := range []string{
		`"msg":"voice semantic slot parser started"`,
		`"msg":"voice semantic slot parser completed"`,
		`"session_id":"sess_slot"`,
		`"slot_status":"partial"`,
		`"actionability":"wait_more"`,
		`"missing_slots":["duration"]`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected logs to contain %s, got:\n%s", want, logs)
		}
	}
}

func TestLoggingSemanticSlotParserLogsFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	parser := LoggingSemanticSlotParser{
		Inner:  stubSemanticSlotParser{err: errors.New("slot parse timeout")},
		Logger: logger,
	}

	if _, err := parser.ParsePreview(context.Background(), SemanticSlotParseRequest{
		SessionID:   "sess_slot_err",
		PartialText: "打开灯",
	}); err == nil {
		t.Fatal("expected ParsePreview to fail")
	}

	logs := buf.String()
	if !strings.Contains(logs, `"msg":"voice semantic slot parser failed"`) {
		t.Fatalf("expected failure log, got:\n%s", logs)
	}
}
