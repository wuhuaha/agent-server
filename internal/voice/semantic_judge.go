package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"agent-server/internal/agent"
)

const (
	defaultSemanticJudgeTimeout      = 220 * time.Millisecond
	defaultSemanticJudgeMinRunes     = 2
	defaultSemanticJudgeMinStableFor = 120 * time.Millisecond
	semanticJudgeHighConfidence      = 0.78
	semanticJudgeMediumConfidence    = 0.68
)

const (
	SemanticWaitPolicyKeep    = "keep"
	SemanticWaitPolicyShorten = "shorten"
	SemanticWaitPolicyExtend  = "extend"
)

const (
	SemanticIntentUnknown     = "unknown"
	SemanticIntentBackchannel = "backchannel"
	SemanticIntentTakeover    = "takeover"
	SemanticIntentCorrection  = "correction"
	SemanticIntentRequest     = "request"
	SemanticIntentQuestion    = "question"
	SemanticIntentContinue    = "continue"
	SemanticIntentOther       = "other"
)

const (
	SemanticUtteranceIncomplete = "incomplete"
	SemanticUtteranceComplete   = "complete"
	SemanticUtteranceCorrection = "correction"
)

type SemanticTurnJudge interface {
	JudgePreview(context.Context, SemanticTurnRequest) (SemanticTurnJudgement, error)
}

type SemanticTurnRequest struct {
	SessionID      string
	DeviceID       string
	ClientType     string
	PartialText    string
	StablePrefix   string
	AudioMs        int
	Stability      float64
	StableForMs    int
	TurnStage      TurnArbitrationStage
	EndpointHinted bool
}

type SemanticTurnJudgement struct {
	CandidateKey       string
	PartialText        string
	StablePrefix       string
	UtteranceStatus    string
	InterruptionIntent string
	DynamicWaitPolicy  string
	WaitDeltaMs        int
	Confidence         float64
	Reason             string
	Source             string
}

type LLMSemanticTurnJudge struct {
	Model agent.ChatModel
}

func NewLLMSemanticTurnJudge(model agent.ChatModel) LLMSemanticTurnJudge {
	return LLMSemanticTurnJudge{Model: model}
}

func (j LLMSemanticTurnJudge) JudgePreview(ctx context.Context, req SemanticTurnRequest) (SemanticTurnJudgement, error) {
	if j.Model == nil {
		return SemanticTurnJudgement{}, fmt.Errorf("semantic turn judge model is not configured")
	}
	prompt := semanticJudgePrompt()
	user := semanticJudgeUserMessage(req)
	response, err := j.Model.Complete(ctx, agent.ChatModelRequest{
		SessionID:    req.SessionID,
		DeviceID:     req.DeviceID,
		ClientType:   req.ClientType,
		SystemPrompt: prompt,
		Messages: []agent.ChatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return SemanticTurnJudgement{}, err
	}
	judgement, err := decodeSemanticTurnJudgement(response.Text)
	if err != nil {
		return SemanticTurnJudgement{}, err
	}
	judgement.CandidateKey = semanticCandidateKey(req.StablePrefix, req.PartialText)
	judgement.PartialText = strings.TrimSpace(req.PartialText)
	judgement.StablePrefix = strings.TrimSpace(req.StablePrefix)
	if judgement.Source == "" {
		judgement.Source = "llm_semantic_judge"
	}
	return judgement, nil
}

func semanticJudgePrompt() string {
	return strings.TrimSpace(`
你是一个实时语音系统里的语义裁判器，不是对话助手。

你的任务不是回复用户，而是为 turn-taking / interruption 做保守判定。

请只输出一段 JSON，对象字段必须只有：
- utterance_status: "incomplete" | "complete" | "correction"
- interruption_intent: "unknown" | "backchannel" | "takeover" | "correction" | "request" | "question" | "continue" | "other"
- dynamic_wait_policy: "shorten" | "keep" | "extend"
- wait_delta_ms: 一个整数，单位毫秒；shorten 为负数，extend 为正数，不确定时输出 0
- confidence: 0 到 1 之间的小数
- reason: 一个简短 snake_case 标签

判定原则：
1. complete 表示“语义上已经足够完整，可开始可撤销的 draft / planning”，不是最终 accept。
2. correction 表示用户明显在改口、修正、补充、重说，或前半句尚未真正收束。
3. backchannel 只用于很短的附和、应答、确认，不应默认视为接管会话。
4. takeover 用于明显要打断、纠正、否定上一轮、切换请求、要求停止或重新开始。
5. 不确定时，优先输出 utterance_status="incomplete" 且 interruption_intent="unknown"。
6. 不要输出解释文本，不要使用 markdown 代码块。`)
}

func semanticJudgeUserMessage(req SemanticTurnRequest) string {
	return strings.TrimSpace(fmt.Sprintf(`
请根据当前实时预览文本做保守分类：
{
  "partial_text": %q,
  "stable_prefix": %q,
  "audio_ms": %d,
  "stability": %.3f,
  "stable_for_ms": %d,
  "turn_stage": %q,
  "endpoint_hinted": %t
}`, strings.TrimSpace(req.PartialText), strings.TrimSpace(req.StablePrefix), req.AudioMs, clampUnit(req.Stability), req.StableForMs, req.TurnStage, req.EndpointHinted))
}

type semanticJudgeJSON struct {
	UtteranceStatus    string  `json:"utterance_status"`
	InterruptionIntent string  `json:"interruption_intent"`
	DynamicWaitPolicy  string  `json:"dynamic_wait_policy"`
	WaitDeltaMs        int     `json:"wait_delta_ms"`
	Confidence         float64 `json:"confidence"`
	Reason             string  `json:"reason"`
}

func decodeSemanticTurnJudgement(raw string) (SemanticTurnJudgement, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SemanticTurnJudgement{}, fmt.Errorf("semantic turn judge returned empty content")
	}
	start := strings.IndexByte(raw, '{')
	end := strings.LastIndexByte(raw, '}')
	if start < 0 || end <= start {
		return SemanticTurnJudgement{}, fmt.Errorf("semantic turn judge did not return json: %q", raw)
	}
	var decoded semanticJudgeJSON
	if err := json.Unmarshal([]byte(raw[start:end+1]), &decoded); err != nil {
		return SemanticTurnJudgement{}, err
	}
	return SemanticTurnJudgement{
		UtteranceStatus:    normalizeSemanticUtteranceStatus(decoded.UtteranceStatus),
		InterruptionIntent: normalizeSemanticIntent(decoded.InterruptionIntent),
		DynamicWaitPolicy:  normalizeSemanticWaitPolicy(decoded.DynamicWaitPolicy),
		WaitDeltaMs:        clampSemanticWaitDeltaMs(decoded.WaitDeltaMs),
		Confidence:         clampUnit(decoded.Confidence),
		Reason:             normalizeSemanticReason(decoded.Reason),
	}, nil
}

func normalizeSemanticUtteranceStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SemanticUtteranceComplete:
		return SemanticUtteranceComplete
	case SemanticUtteranceCorrection:
		return SemanticUtteranceCorrection
	default:
		return SemanticUtteranceIncomplete
	}
}

func normalizeSemanticIntent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SemanticIntentBackchannel:
		return SemanticIntentBackchannel
	case SemanticIntentTakeover:
		return SemanticIntentTakeover
	case SemanticIntentCorrection:
		return SemanticIntentCorrection
	case SemanticIntentRequest:
		return SemanticIntentRequest
	case SemanticIntentQuestion:
		return SemanticIntentQuestion
	case SemanticIntentContinue:
		return SemanticIntentContinue
	case SemanticIntentOther:
		return SemanticIntentOther
	default:
		return SemanticIntentUnknown
	}
}

func normalizeSemanticReason(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "semantic_unknown"
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_")
	value = replacer.Replace(value)
	return value
}

func normalizeSemanticWaitPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SemanticWaitPolicyShorten:
		return SemanticWaitPolicyShorten
	case SemanticWaitPolicyExtend:
		return SemanticWaitPolicyExtend
	default:
		return SemanticWaitPolicyKeep
	}
}

func clampSemanticWaitDeltaMs(value int) int {
	const limit = 600
	switch {
	case value > limit:
		return limit
	case value < -limit:
		return -limit
	default:
		return value
	}
}

func defaultSemanticWaitDelta(judgement SemanticTurnJudgement) int {
	switch judgement.UtteranceStatus {
	case SemanticUtteranceCorrection:
		return 320
	case SemanticUtteranceComplete:
		switch normalizeSemanticIntent(judgement.InterruptionIntent) {
		case SemanticIntentQuestion:
			return -140
		case SemanticIntentRequest, SemanticIntentTakeover:
			return -120
		case SemanticIntentBackchannel:
			return 180
		case SemanticIntentContinue:
			return 180
		default:
			return -80
		}
	default:
		switch normalizeSemanticIntent(judgement.InterruptionIntent) {
		case SemanticIntentContinue, SemanticIntentCorrection:
			return 220
		case SemanticIntentBackchannel:
			return 120
		default:
			return 0
		}
	}
}

func semanticCandidateKey(stablePrefix, partialText string) string {
	return strings.TrimSpace(stablePrefix) + "\n---\n" + strings.TrimSpace(partialText)
}

func firstPositiveDuration(primary, fallback time.Duration) time.Duration {
	if primary > 0 {
		return primary
	}
	return fallback
}

type previewSemanticJudgeState struct {
	mu               sync.Mutex
	evaluating       bool
	lastRequestedKey string
	lastResult       SemanticTurnJudgement
}

func (s *previewSemanticJudgeState) shouldLaunch(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" {
		return false
	}
	if s.evaluating && s.lastRequestedKey == key {
		return false
	}
	if s.lastResult.CandidateKey == key {
		return false
	}
	s.evaluating = true
	s.lastRequestedKey = key
	return true
}

func (s *previewSemanticJudgeState) storeResult(result SemanticTurnJudgement) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluating = false
	s.lastResult = result
}

func (s *previewSemanticJudgeState) clearRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluating = false
}

func (s *previewSemanticJudgeState) resultFor(key string) (SemanticTurnJudgement, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" || s.lastResult.CandidateKey != key {
		return SemanticTurnJudgement{}, false
	}
	return s.lastResult, true
}

func shouldJudgeSemantic(snapshot InputPreview, minRunes int, minStableFor time.Duration) bool {
	bestText := strings.TrimSpace(firstNonEmpty(snapshot.StablePrefix, snapshot.PartialText))
	if bestText == "" {
		return false
	}
	if minRunes <= 0 {
		minRunes = defaultSemanticJudgeMinRunes
	}
	if utf8.RuneCountInString(bestText) < minRunes && !looksLikeTakeoverLexicon(bestText) && !looksLikeBackchannel(bestText) {
		return false
	}
	stableFor := time.Duration(snapshot.Arbitration.StableForMs) * time.Millisecond
	if stableFor >= minStableFor {
		return true
	}
	if snapshot.Arbitration.AcceptCandidate || snapshot.Arbitration.AcceptNow || snapshot.Arbitration.DraftAllowed {
		return true
	}
	if looksLikeTakeoverLexicon(bestText) || looksLikeBackchannel(bestText) {
		return true
	}
	return false
}

// mergeSemanticJudgement 让 LLM 语义判定只做“可撤销的语义加权”，
// 不直接替代声学/静音安全底座，从而避免实时链路完全依赖一次模型返回。
func mergeSemanticJudgement(snapshot InputPreview, judgement SemanticTurnJudgement) InputPreview {
	if judgement.CandidateKey == "" {
		return snapshot
	}
	arbitration := snapshot.Arbitration
	arbitration.SemanticReady = true
	arbitration.SemanticComplete = judgement.UtteranceStatus == SemanticUtteranceComplete
	arbitration.SemanticIntent = normalizeSemanticIntent(judgement.InterruptionIntent)
	arbitration.SemanticReason = normalizeSemanticReason(judgement.Reason)
	arbitration.SemanticSource = strings.TrimSpace(judgement.Source)
	arbitration.SemanticConfidence = clampUnit(judgement.Confidence)
	arbitration.SemanticWaitPolicy = normalizeSemanticWaitPolicy(judgement.DynamicWaitPolicy)
	arbitration.SemanticWaitDeltaMs = clampSemanticWaitDeltaMs(judgement.WaitDeltaMs)
	if arbitration.SemanticWaitDeltaMs == 0 {
		arbitration.SemanticWaitDeltaMs = defaultSemanticWaitDelta(judgement)
	}

	if arbitration.SemanticConfidence >= semanticJudgeMediumConfidence {
		switch judgement.UtteranceStatus {
		case SemanticUtteranceCorrection:
			snapshot.UtteranceComplete = false
			if arbitration.Stage == TurnArbitrationStageDraftAllowed {
				arbitration.Stage = TurnArbitrationStageWaitForMore
			}
			arbitration.DraftAllowed = false
		case SemanticUtteranceComplete:
			if arbitration.SemanticIntent != SemanticIntentBackchannel {
				snapshot.UtteranceComplete = true
				arbitration.PrewarmAllowed = true
				arbitration.DraftAllowed = true
				if arbitration.Stage == TurnArbitrationStagePreviewOnly ||
					arbitration.Stage == TurnArbitrationStageWaitForMore ||
					arbitration.Stage == TurnArbitrationStagePrewarmAllowed {
					arbitration.Stage = TurnArbitrationStageDraftAllowed
				}
				if strings.TrimSpace(arbitration.Reason) == "" {
					arbitration.Reason = "semantic_complete"
				}
			}
		}
	}
	snapshot.Arbitration = recomputeTurnArbitration(snapshot, arbitration)
	return snapshot
}
