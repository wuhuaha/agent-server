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
	defaultSemanticSlotParserTimeout      = 280 * time.Millisecond
	defaultSemanticSlotParserMinRunes     = 4
	defaultSemanticSlotParserMinStableFor = 160 * time.Millisecond
)

const (
	SemanticSlotDomainUnknown          = "unknown"
	SemanticSlotDomainSmartHome        = "smart_home"
	SemanticSlotDomainDesktopAssistant = "desktop_assistant"
	SemanticSlotDomainGeneralChat      = "general_chat"
)

const (
	SemanticSlotStatusUnknown       = "unknown"
	SemanticSlotStatusPartial       = "partial"
	SemanticSlotStatusComplete      = "complete"
	SemanticSlotStatusAmbiguous     = "ambiguous"
	SemanticSlotStatusNotApplicable = "not_applicable"
)

const (
	SemanticSlotActionabilityObserveOnly   = "observe_only"
	SemanticSlotActionabilityWaitMore      = "wait_more"
	SemanticSlotActionabilityDraftOK       = "draft_ok"
	SemanticSlotActionabilityClarifyNeeded = "clarify_needed"
	SemanticSlotActionabilityActCandidate  = "act_candidate"
)

type SemanticSlotParser interface {
	ParsePreview(context.Context, SemanticSlotParseRequest) (SemanticSlotParseResult, error)
}

type SemanticSlotParseRequest struct {
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
	SemanticIntent string
}

type SemanticSlotParseResult struct {
	CandidateKey        string
	PartialText         string
	StablePrefix        string
	Domain              string
	TaskFamily          string
	Intent              string
	SlotStatus          string
	Actionability       string
	ClarifyNeeded       bool
	Grounded            bool
	CanonicalTarget     string
	CanonicalLocation   string
	NormalizedValue     string
	NormalizedValueUnit string
	RiskLevel           string
	RiskReason          string
	RiskConfirmRequired bool
	MissingSlots        []string
	AmbiguousSlots      []string
	Confidence          float64
	Reason              string
	Source              string
}

type LLMSemanticSlotParser struct {
	Model agent.ChatModel
}

func NewLLMSemanticSlotParser(model agent.ChatModel) LLMSemanticSlotParser {
	return LLMSemanticSlotParser{Model: model}
}

func (p LLMSemanticSlotParser) ParsePreview(ctx context.Context, req SemanticSlotParseRequest) (SemanticSlotParseResult, error) {
	if p.Model == nil {
		return SemanticSlotParseResult{}, fmt.Errorf("semantic slot parser model is not configured")
	}
	prompt := semanticSlotParserPrompt()
	user := semanticSlotParserUserMessage(req)
	response, err := p.Model.Complete(ctx, agent.ChatModelRequest{
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
		return SemanticSlotParseResult{}, err
	}
	result, err := decodeSemanticSlotParseResult(response.Text)
	if err != nil {
		return SemanticSlotParseResult{}, err
	}
	result.CandidateKey = semanticCandidateKey(req.StablePrefix, req.PartialText)
	result.PartialText = strings.TrimSpace(req.PartialText)
	result.StablePrefix = strings.TrimSpace(req.StablePrefix)
	if result.Source == "" {
		result.Source = "llm_semantic_slot_parser"
	}
	return result, nil
}

func semanticSlotParserPrompt() string {
	return strings.TrimSpace(`
你是一个实时语音系统里的结构化语义解析器，不是对话助手。

你的任务不是回复用户，而是对当前预览文本做保守的 domain / intent / slot completeness 判断。

请只输出一段 JSON，对象字段必须只有：
- domain: "unknown" | "smart_home" | "desktop_assistant" | "general_chat"
- task_family: "unknown" | "dialogue" | "knowledge_query" | "structured_command" | "structured_query" | "correction" | "backchannel"
- intent: 一个简短 snake_case 标签；不确定时输出 "unknown"
- slot_status: "unknown" | "partial" | "complete" | "ambiguous" | "not_applicable"
- actionability: "observe_only" | "wait_more" | "draft_ok" | "clarify_needed" | "act_candidate"
- clarify_needed: true | false
- missing_slots: 字符串数组，元素用 snake_case
- ambiguous_slots: 字符串数组，元素用 snake_case
- confidence: 0 到 1 之间的小数
- reason: 一个简短 snake_case 标签

保守规则：
1. smart_home 关注 action / target / location / attribute / value / mode / duration 这类槽位。
2. desktop_assistant 关注 action / target_app / query / window_name / system_setting / value 这类槽位。
3. 如果用户像是在闲聊、开放问答、情感表达，而不是要执行命令，优先输出 domain="general_chat" 且 slot_status="not_applicable"。
4. task_family 是比 domain 更通用的早处理抽象：
   - 开放问答/闲聊 -> "dialogue" 或 "knowledge_query"
   - 对设备/应用/结构化对象的操作 -> "structured_command"
   - 对设备/应用/结构化对象的状态查询 -> "structured_query"
   - 附和/短确认 -> "backchannel"
   - 改口/纠正/重说 -> "correction"
5. clarify_needed 表示“这句话本身大致说完了，但如果现在接收这一轮，更合理的是马上澄清，而不是继续傻等更多语音”。
6. wait_more 表示尾部槽位仍明显没说完，或补充仍在进行中，不应过早提升。
7. act_candidate 表示主要必填槽位已经足够完整，可作为后续执行或工具规划候选，但这仍不是最终 accept。
8. 不确定时，优先 domain="unknown"、task_family="unknown"、slot_status="unknown"、actionability="observe_only"。
9. 不要输出解释文本，不要使用 markdown 代码块。`)
}

func semanticSlotParserUserMessage(req SemanticSlotParseRequest) string {
	return strings.TrimSpace(fmt.Sprintf(`
请根据当前实时预览文本做保守的结构化解析：
{
  "partial_text": %q,
  "stable_prefix": %q,
  "audio_ms": %d,
  "stability": %.3f,
  "stable_for_ms": %d,
  "turn_stage": %q,
  "endpoint_hinted": %t,
  "semantic_intent": %q
}`, strings.TrimSpace(req.PartialText), strings.TrimSpace(req.StablePrefix), req.AudioMs, clampUnit(req.Stability), req.StableForMs, req.TurnStage, req.EndpointHinted, normalizeSemanticIntent(req.SemanticIntent)))
}

type semanticSlotParserJSON struct {
	Domain         string   `json:"domain"`
	TaskFamily     string   `json:"task_family"`
	Intent         string   `json:"intent"`
	SlotStatus     string   `json:"slot_status"`
	Actionability  string   `json:"actionability"`
	ClarifyNeeded  bool     `json:"clarify_needed"`
	MissingSlots   []string `json:"missing_slots"`
	AmbiguousSlots []string `json:"ambiguous_slots"`
	Confidence     float64  `json:"confidence"`
	Reason         string   `json:"reason"`
}

func decodeSemanticSlotParseResult(raw string) (SemanticSlotParseResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SemanticSlotParseResult{}, fmt.Errorf("semantic slot parser returned empty content")
	}
	start := strings.IndexByte(raw, '{')
	end := strings.LastIndexByte(raw, '}')
	if start < 0 || end <= start {
		return SemanticSlotParseResult{}, fmt.Errorf("semantic slot parser did not return json: %q", raw)
	}
	var decoded semanticSlotParserJSON
	if err := json.Unmarshal([]byte(raw[start:end+1]), &decoded); err != nil {
		return SemanticSlotParseResult{}, err
	}
	return SemanticSlotParseResult{
		Domain:         normalizeSemanticSlotDomain(decoded.Domain),
		TaskFamily:     normalizeSemanticTaskFamily(decoded.TaskFamily),
		Intent:         normalizeSemanticSlotLabel(decoded.Intent, "unknown"),
		SlotStatus:     normalizeSemanticSlotStatus(decoded.SlotStatus),
		Actionability:  normalizeSemanticSlotActionability(decoded.Actionability),
		ClarifyNeeded:  decoded.ClarifyNeeded,
		MissingSlots:   normalizeSemanticSlotList(decoded.MissingSlots),
		AmbiguousSlots: normalizeSemanticSlotList(decoded.AmbiguousSlots),
		Confidence:     clampUnit(decoded.Confidence),
		Reason:         normalizeSemanticReason(decoded.Reason),
	}, nil
}

func normalizeSemanticSlotDomain(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SemanticSlotDomainSmartHome:
		return SemanticSlotDomainSmartHome
	case SemanticSlotDomainDesktopAssistant:
		return SemanticSlotDomainDesktopAssistant
	case SemanticSlotDomainGeneralChat:
		return SemanticSlotDomainGeneralChat
	default:
		return SemanticSlotDomainUnknown
	}
}

func normalizeSemanticSlotStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SemanticSlotStatusPartial:
		return SemanticSlotStatusPartial
	case SemanticSlotStatusComplete:
		return SemanticSlotStatusComplete
	case SemanticSlotStatusAmbiguous:
		return SemanticSlotStatusAmbiguous
	case SemanticSlotStatusNotApplicable:
		return SemanticSlotStatusNotApplicable
	default:
		return SemanticSlotStatusUnknown
	}
}

func normalizeSemanticSlotActionability(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SemanticSlotActionabilityWaitMore:
		return SemanticSlotActionabilityWaitMore
	case SemanticSlotActionabilityDraftOK:
		return SemanticSlotActionabilityDraftOK
	case SemanticSlotActionabilityClarifyNeeded:
		return SemanticSlotActionabilityClarifyNeeded
	case SemanticSlotActionabilityActCandidate:
		return SemanticSlotActionabilityActCandidate
	default:
		return SemanticSlotActionabilityObserveOnly
	}
}

func normalizeSemanticSlotLabel(value, fallback string) string {
	value = normalizeSemanticReason(value)
	if value == "semantic_unknown" {
		return fallback
	}
	return value
}

func normalizeSemanticSlotList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		label := normalizeSemanticSlotLabel(value, "")
		if label == "" || label == "semantic_unknown" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		normalized = append(normalized, label)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

type previewSemanticSlotParserState struct {
	mu               sync.Mutex
	evaluating       bool
	lastRequestedKey string
	lastResult       SemanticSlotParseResult
}

func (s *previewSemanticSlotParserState) shouldLaunch(key string) bool {
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

func (s *previewSemanticSlotParserState) storeResult(result SemanticSlotParseResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluating = false
	s.lastResult = result
}

func (s *previewSemanticSlotParserState) clearRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluating = false
}

func (s *previewSemanticSlotParserState) resultFor(key string) (SemanticSlotParseResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" || s.lastResult.CandidateKey != key {
		return SemanticSlotParseResult{}, false
	}
	return s.lastResult, true
}

func shouldParseSemanticSlots(snapshot InputPreview, minRunes int, minStableFor time.Duration) bool {
	bestText := strings.TrimSpace(firstNonEmpty(snapshot.StablePrefix, snapshot.PartialText))
	if bestText == "" {
		return false
	}
	if minRunes <= 0 {
		minRunes = defaultSemanticSlotParserMinRunes
	}
	if utf8.RuneCountInString(bestText) < minRunes {
		return false
	}
	if snapshot.Arbitration.SemanticIntent == SemanticIntentBackchannel && snapshot.Arbitration.SemanticConfidence >= semanticJudgeMediumConfidence {
		return false
	}
	if snapshot.Arbitration.SlotConstraintRequired && snapshot.Arbitration.CandidateReady {
		return true
	}
	stableFor := time.Duration(snapshot.Arbitration.StableForMs) * time.Millisecond
	if stableFor >= minStableFor {
		return true
	}
	if inferLexicalTaskFamily(bestText) == SemanticTaskFamilyStructuredCommand && snapshot.Arbitration.CandidateReady {
		return true
	}
	if snapshot.Arbitration.DraftAllowed || snapshot.Arbitration.AcceptCandidate || snapshot.Arbitration.AcceptNow {
		return true
	}
	if snapshot.Arbitration.SemanticComplete {
		return true
	}
	return snapshot.UtteranceComplete
}

// mergeSemanticSlotParse 让 slot completeness 作为可撤销的结构化约束进入 preview 仲裁：
// 可以在“主要槽位已经够用”时提早放行 draft，也可以在“尾部槽位明显没说完”时拉回保守状态。
func mergeSemanticSlotParse(snapshot InputPreview, result SemanticSlotParseResult) InputPreview {
	if result.CandidateKey == "" {
		return snapshot
	}
	arbitration := snapshot.Arbitration
	arbitration.SlotReady = true
	arbitration.SlotComplete = result.SlotStatus == SemanticSlotStatusComplete
	arbitration.SlotGrounded = result.Grounded
	arbitration.SlotDomain = normalizeSemanticSlotDomain(result.Domain)
	arbitration.TaskFamily = inferSlotTaskFamily(snapshot, result)
	arbitration.SlotConstraintRequired = taskFamilyRequiresSlotReadiness(arbitration.TaskFamily)
	arbitration.SlotIntent = normalizeSemanticSlotLabel(result.Intent, "unknown")
	arbitration.SlotStatus = normalizeSemanticSlotStatus(result.SlotStatus)
	arbitration.SlotActionability = normalizeSemanticSlotActionability(result.Actionability)
	arbitration.SlotReason = normalizeSemanticReason(result.Reason)
	arbitration.SlotSource = strings.TrimSpace(result.Source)
	arbitration.SlotConfidence = clampUnit(result.Confidence)
	arbitration.SlotClarifyNeeded = result.ClarifyNeeded
	arbitration.SlotCanonicalTarget = strings.TrimSpace(result.CanonicalTarget)
	arbitration.SlotCanonicalLocation = strings.TrimSpace(result.CanonicalLocation)
	arbitration.SlotNormalizedValue = strings.TrimSpace(result.NormalizedValue)
	arbitration.SlotNormalizedValueUnit = strings.TrimSpace(result.NormalizedValueUnit)
	arbitration.SlotRiskLevel = normalizeSemanticRiskLevel(result.RiskLevel)
	arbitration.SlotRiskReason = normalizeSemanticReason(result.RiskReason)
	arbitration.SlotRiskConfirmRequired = result.RiskConfirmRequired
	arbitration.SlotMissing = append([]string(nil), normalizeSemanticSlotList(result.MissingSlots)...)
	arbitration.SlotAmbiguous = append([]string(nil), normalizeSemanticSlotList(result.AmbiguousSlots)...)

	if arbitration.SlotConfidence >= semanticJudgeMediumConfidence {
		switch arbitration.SlotActionability {
		case SemanticSlotActionabilityDraftOK, SemanticSlotActionabilityClarifyNeeded, SemanticSlotActionabilityActCandidate:
			arbitration.PrewarmAllowed = true
			arbitration.DraftAllowed = true
			if arbitration.Stage == TurnArbitrationStagePreviewOnly ||
				arbitration.Stage == TurnArbitrationStageWaitForMore ||
				arbitration.Stage == TurnArbitrationStagePrewarmAllowed {
				arbitration.Stage = TurnArbitrationStageDraftAllowed
			}
			if strings.TrimSpace(arbitration.Reason) == "" {
				arbitration.Reason = "slot_" + arbitration.SlotActionability
			}
		case SemanticSlotActionabilityWaitMore:
			arbitration.DraftAllowed = false
			if arbitration.Stage == TurnArbitrationStageDraftAllowed {
				if arbitration.PrewarmAllowed {
					arbitration.Stage = TurnArbitrationStagePrewarmAllowed
				} else {
					arbitration.Stage = TurnArbitrationStageWaitForMore
				}
			}
			if strings.TrimSpace(arbitration.Reason) == "" {
				arbitration.Reason = "slot_wait_more"
			}
		}
	}
	arbitration.SlotGuardAdjustMs = slotGuardAdjustMs(arbitration)
	snapshot.Arbitration = recomputeTurnArbitration(snapshot, arbitration)
	return snapshot
}
