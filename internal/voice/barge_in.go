package voice

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultBargeInMinAudioMs       = 120
	defaultBargeInIncompleteHoldMs = 240
)

type InterruptionPolicy string

const (
	InterruptionPolicyIgnore        InterruptionPolicy = "ignore"
	InterruptionPolicyBackchannel   InterruptionPolicy = "backchannel"
	InterruptionPolicyDuckOnly      InterruptionPolicy = "duck_only"
	InterruptionPolicyHardInterrupt InterruptionPolicy = "hard_interrupt"
)

type PlaybackAction string

const (
	PlaybackActionNormal    PlaybackAction = "normal"
	PlaybackActionDuckLight PlaybackAction = "duck_light"
	PlaybackActionDuckHold  PlaybackAction = "duck_hold"
	PlaybackActionInterrupt PlaybackAction = "interrupt"
)

type PlaybackDirective struct {
	Action           PlaybackAction
	Policy           InterruptionPolicy
	Reason           string
	Gain             float64
	Attack           time.Duration
	Hold             time.Duration
	Release          time.Duration
	KeepPreview      bool
	KeepPendingAudio bool
}

type BargeInConfig struct {
	MinAudioMs       int
	IncompleteHoldMs int
}

type BargeInDecision struct {
	Accepted           bool
	Policy             InterruptionPolicy
	Reason             string
	AudioMs            int
	LexicallyComplete  bool
	MinAudioMs         int
	HoldAudioMs        int
	AcousticReady      bool
	SemanticReady      bool
	SemanticComplete   bool
	SemanticIntent     string
	SemanticConfidence float64
	SemanticTakeover   bool
	AcceptCandidate    bool
	AcceptNow          bool
	EndpointHinted     bool
	BackchannelLikely  bool
	TakeoverLexicon    bool
	TurnStage          TurnArbitrationStage
	Stability          float64
	StablePrefixRunes  int
	IntrusionScore     float64
	TakeoverScore      float64
}

func NormalizeBargeInConfig(cfg BargeInConfig) BargeInConfig {
	if cfg.MinAudioMs <= 0 {
		cfg.MinAudioMs = defaultBargeInMinAudioMs
	}
	if cfg.IncompleteHoldMs <= 0 {
		cfg.IncompleteHoldMs = defaultBargeInIncompleteHoldMs
	}
	return cfg
}

func EvaluateBargeIn(preview InputPreview, sampleRateHz, channels int, cfg BargeInConfig) BargeInDecision {
	cfg = NormalizeBargeInConfig(cfg)
	decision := BargeInDecision{
		Policy:      InterruptionPolicyIgnore,
		MinAudioMs:  cfg.MinAudioMs,
		HoldAudioMs: cfg.IncompleteHoldMs,
	}
	if !preview.SpeechStarted || preview.AudioBytes <= 0 {
		decision.Reason = "no_speech_started"
		return decision
	}
	if sampleRateHz <= 0 || channels <= 0 {
		decision.Reason = "invalid_audio_format"
		return decision
	}
	audioMs := pcm16AudioDurationMs(preview.AudioBytes, sampleRateHz, channels)
	decision.AudioMs = audioMs
	partialText := strings.TrimSpace(preview.PartialText)
	stablePrefix := strings.TrimSpace(preview.StablePrefix)
	bestText := firstNonEmpty(stablePrefix, partialText)

	decision.TurnStage = normalizedBargeInTurnStage(preview)
	decision.Stability = clampUnit(preview.Arbitration.Stability)
	decision.StablePrefixRunes = utf8.RuneCountInString(stablePrefix)
	decision.EndpointHinted = preview.Arbitration.EndpointHinted || strings.TrimSpace(preview.EndpointReason) != ""
	decision.AcceptCandidate = preview.Arbitration.AcceptCandidate || decision.TurnStage == TurnArbitrationStageAcceptCandidate || decision.TurnStage == TurnArbitrationStageAcceptNow || decision.EndpointHinted
	decision.AcceptNow = preview.Arbitration.AcceptNow || preview.CommitSuggested || decision.TurnStage == TurnArbitrationStageAcceptNow
	decision.SemanticReady = preview.Arbitration.SemanticReady
	decision.SemanticComplete = preview.Arbitration.SemanticComplete
	decision.SemanticIntent = normalizeSemanticIntent(preview.Arbitration.SemanticIntent)
	decision.SemanticConfidence = clampUnit(preview.Arbitration.SemanticConfidence)
	decision.SemanticTakeover = semanticIntentIsTakeover(decision.SemanticIntent)
	decision.BackchannelLikely = looksLikeBackchannel(bestText)
	if decision.SemanticIntent == SemanticIntentBackchannel && decision.SemanticConfidence >= semanticJudgeMediumConfidence {
		decision.BackchannelLikely = true
	}
	decision.TakeoverLexicon = looksLikeTakeoverLexicon(bestText)
	decision.LexicallyComplete = looksLexicallyComplete(bestText)
	decision.AcousticReady = audioMs >= bargeInAcousticGateMinAudioMs(cfg.MinAudioMs)
	decision.SemanticReady = bargeInSemanticReady(preview, bestText, stablePrefix, decision)
	decision.IntrusionScore = bargeInIntrusionScore(audioMs, cfg, decision, partialText, stablePrefix)
	decision.TakeoverScore = bargeInTakeoverScore(decision)

	if decision.SemanticIntent == SemanticIntentBackchannel && decision.SemanticConfidence >= semanticJudgeHighConfidence &&
		!decision.AcceptCandidate && !decision.AcceptNow && !decision.TakeoverLexicon {
		decision.Policy = InterruptionPolicyBackchannel
		decision.Reason = "semantic_backchannel"
		return decision
	}
	if decision.AcousticReady && decision.SemanticTakeover && decision.SemanticConfidence >= semanticJudgeHighConfidence {
		decision.Policy = InterruptionPolicyHardInterrupt
		decision.Accepted = true
		decision.Reason = "accepted_semantic_takeover"
		return decision
	}
	if decision.BackchannelLikely && !decision.AcceptCandidate && !decision.AcceptNow && !decision.TakeoverLexicon &&
		audioMs >= bargeInBackchannelMinAudioMs(cfg.MinAudioMs) {
		decision.Policy = InterruptionPolicyBackchannel
		decision.Reason = "backchannel_short_ack"
		return decision
	}
	if audioMs >= cfg.MinAudioMs && !decision.BackchannelLikely && (decision.LexicallyComplete || decision.TakeoverLexicon || decision.SemanticTakeover) {
		decision.Policy = InterruptionPolicyHardInterrupt
		decision.Accepted = true
		if decision.TakeoverLexicon || decision.SemanticTakeover {
			decision.Reason = "accepted_takeover_lexicon"
		} else {
			decision.Reason = "accepted_complete_preview"
		}
		return decision
	}

	if decision.AcceptNow && (decision.LexicallyComplete || decision.SemanticReady || decision.TakeoverLexicon || decision.SemanticTakeover) {
		decision.Policy = InterruptionPolicyHardInterrupt
		decision.Accepted = true
		decision.Reason = bargeInAcceptReason(decision)
		return decision
	}
	if decision.AcceptCandidate && decision.SemanticReady && !decision.BackchannelLikely &&
		(decision.LexicallyComplete || decision.TakeoverLexicon || decision.SemanticTakeover || decision.TakeoverScore >= 0.68) {
		decision.Policy = InterruptionPolicyHardInterrupt
		decision.Accepted = true
		decision.Reason = "accepted_accept_candidate"
		return decision
	}
	if audioMs >= cfg.MinAudioMs+cfg.IncompleteHoldMs && (bestText != "" || decision.IntrusionScore >= 0.72) && !decision.BackchannelLikely {
		decision.Policy = InterruptionPolicyHardInterrupt
		decision.Accepted = true
		decision.Reason = "accepted_incomplete_after_hold"
		return decision
	}

	if audioMs < cfg.MinAudioMs {
		if partialText != "" || stablePrefix != "" {
			decision.Policy = InterruptionPolicyDuckOnly
			decision.Reason = "duck_pending_min_audio"
			return decision
		}
		if decision.AcousticReady {
			decision.Policy = InterruptionPolicyDuckOnly
			decision.Reason = "duck_pending_audio_only"
			return decision
		}
		decision.Reason = "below_min_audio"
		return decision
	}

	if decision.BackchannelLikely && decision.TakeoverScore < 0.35 && !decision.AcceptCandidate && !decision.AcceptNow {
		decision.Policy = InterruptionPolicyBackchannel
		decision.Reason = "backchannel_short_ack"
		return decision
	}

	decision.Policy = InterruptionPolicyDuckOnly
	if partialText == "" && stablePrefix == "" {
		decision.Reason = "duck_pending_audio_only"
		return decision
	}
	if decision.SemanticReady {
		decision.Reason = "duck_pending_semantic_confirmation"
		return decision
	}
	decision.Reason = "duck_pending_incomplete_preview"
	return decision
}

func ShouldAcceptBargeIn(preview InputPreview, sampleRateHz, channels int, cfg BargeInConfig) bool {
	return EvaluateBargeIn(preview, sampleRateHz, channels, cfg).Accepted
}

func (d BargeInDecision) ShouldInterrupt() bool {
	return d.PlaybackDirective().ShouldInterruptOutput()
}

func (d BargeInDecision) ShouldDuckOutput() bool {
	return d.PlaybackDirective().ShouldDuckOutput()
}

func (d BargeInDecision) PlaybackDirective() PlaybackDirective {
	return playbackDirectiveForDecision(d)
}

func (d PlaybackDirective) ShouldInterruptOutput() bool {
	return d.Action == PlaybackActionInterrupt
}

func (d PlaybackDirective) ShouldDuckOutput() bool {
	return d.Action == PlaybackActionDuckLight || d.Action == PlaybackActionDuckHold
}

func pcm16AudioDurationMs(audioBytes, sampleRateHz, channels int) int {
	if audioBytes <= 0 || sampleRateHz <= 0 || channels <= 0 {
		return 0
	}
	return int((time.Duration(audioBytes/channels/2) * time.Second) / time.Duration(sampleRateHz) / time.Millisecond)
}

func bargeInBackchannelMinAudioMs(minAudioMs int) int {
	if minAudioMs <= 0 {
		return 60
	}
	threshold := minAudioMs / 2
	if threshold < 60 {
		return 60
	}
	return threshold
}

func bargeInAcousticGateMinAudioMs(minAudioMs int) int {
	if minAudioMs <= 0 {
		return 60
	}
	threshold := minAudioMs / 2
	if threshold > minAudioMs {
		threshold = minAudioMs
	}
	if threshold < 60 {
		threshold = 60
		if threshold > minAudioMs {
			threshold = minAudioMs
		}
	}
	return threshold
}

func looksLikeBackchannel(text string) bool {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return false
	}
	normalized = strings.Trim(normalized, " \t\r\n,.!?;:，。！？；：~～")
	if normalized == "" {
		return false
	}
	if _, ok := backchannelTokens[normalized]; ok {
		return true
	}
	runes := []rune(normalized)
	if len(runes) > 4 {
		return false
	}
	if len(runes) == 1 {
		switch runes[0] {
		case '嗯', '啊', '哦', '喔', '唉', '欸', '哎', '好':
			return true
		}
	}
	if len(runes) <= 3 {
		same := true
		for _, r := range runes[1:] {
			if r != runes[0] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

var backchannelTokens = map[string]struct{}{
	"嗯":   {},
	"嗯嗯":  {},
	"啊":   {},
	"哦":   {},
	"喔":   {},
	"唉":   {},
	"欸":   {},
	"哎":   {},
	"好":   {},
	"好的":  {},
	"行":   {},
	"行吧":  {},
	"可以":  {},
	"收到":  {},
	"知道了": {},
	"明白了": {},
	"对":   {},
	"是的":  {},
	"没事":  {},
	"谢谢":  {},
	"哈哈":  {},
}

func normalizedBargeInTurnStage(preview InputPreview) TurnArbitrationStage {
	if stage := preview.Arbitration.Stage; stage != "" {
		return stage
	}
	switch {
	case preview.CommitSuggested:
		return TurnArbitrationStageAcceptNow
	case strings.TrimSpace(preview.EndpointReason) != "":
		return TurnArbitrationStageAcceptCandidate
	case strings.TrimSpace(preview.PartialText) != "":
		return TurnArbitrationStagePreviewOnly
	default:
		return TurnArbitrationStagePreviewOnly
	}
}

func bargeInSemanticReady(preview InputPreview, bestText, stablePrefix string, decision BargeInDecision) bool {
	if strings.TrimSpace(bestText) == "" {
		return false
	}
	if preview.Arbitration.SemanticReady && preview.Arbitration.SemanticConfidence >= semanticJudgeMediumConfidence {
		return true
	}
	if preview.Arbitration.DraftAllowed || preview.Arbitration.AcceptCandidate || preview.Arbitration.AcceptNow {
		return true
	}
	if preview.UtteranceComplete || decision.LexicallyComplete || decision.TakeoverLexicon || decision.SemanticTakeover {
		return true
	}
	if stablePrefix != "" && (decision.Stability >= defaultPrewarmStableRatio || decision.StablePrefixRunes >= 4) {
		return true
	}
	return false
}

func bargeInIntrusionScore(audioMs int, cfg BargeInConfig, decision BargeInDecision, partialText, stablePrefix string) float64 {
	score := 0.0
	if audioMs > 0 {
		score += 0.35 * clampUnit(float64(audioMs)/float64(maxInt(bargeInAcousticGateMinAudioMs(cfg.MinAudioMs), 1)))
	}
	if partialText != "" || stablePrefix != "" {
		score += 0.15
	}
	if decision.AcousticReady {
		score += 0.1
	}
	if decision.EndpointHinted {
		score += 0.1
	}
	if decision.AcceptCandidate {
		score += 0.1
	}
	if decision.AcceptNow {
		score += 0.1
	}
	if decision.StablePrefixRunes >= 4 {
		score += 0.05
	}
	if decision.SemanticConfidence >= semanticJudgeMediumConfidence {
		score += 0.1
	}
	if decision.BackchannelLikely {
		score -= 0.15
	}
	return clampUnit(score)
}

func bargeInTakeoverScore(decision BargeInDecision) float64 {
	score := 0.0
	if decision.LexicallyComplete {
		score += 0.3
	}
	if decision.SemanticReady {
		score += 0.2
	}
	if decision.AcceptCandidate {
		score += 0.15
	}
	if decision.AcceptNow {
		score += 0.2
	}
	if decision.TakeoverLexicon {
		score += 0.2
	}
	if decision.SemanticTakeover {
		score += 0.25
	}
	if decision.SemanticComplete {
		score += 0.1
	}
	if decision.EndpointHinted {
		score += 0.05
	}
	if decision.BackchannelLikely {
		score -= 0.45
	}
	if decision.Stability >= defaultDraftStableRatio {
		score += 0.1
	}
	return clampUnit(score)
}

func bargeInAcceptReason(decision BargeInDecision) string {
	switch {
	case decision.TakeoverLexicon || decision.SemanticTakeover:
		return "accepted_takeover_lexicon"
	case decision.AcceptNow:
		return "accepted_accept_now"
	case decision.LexicallyComplete:
		return "accepted_complete_preview"
	default:
		return "accepted_semantic_confirmation"
	}
}

func semanticIntentIsTakeover(intent string) bool {
	switch normalizeSemanticIntent(intent) {
	case SemanticIntentTakeover, SemanticIntentCorrection, SemanticIntentRequest, SemanticIntentQuestion, SemanticIntentContinue:
		return true
	default:
		return false
	}
}

func looksLikeTakeoverLexicon(text string) bool {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return false
	}
	for _, token := range bargeInTakeoverLexicon {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

var bargeInTakeoverLexicon = []string{
	"等一下",
	"等会",
	"等等",
	"停一下",
	"停停",
	"不是",
	"不对",
	"打断一下",
	"先别",
	"先停",
	"重新",
	"改成",
	"换成",
}

func playbackDirectiveForDecision(decision BargeInDecision) PlaybackDirective {
	directive := PlaybackDirective{
		Action: PlaybackActionNormal,
		Policy: normalizeInterruptionPolicy(decision.Policy),
		Gain:   1.0,
		Reason: strings.TrimSpace(decision.Reason),
	}
	if directive.Policy == InterruptionPolicyIgnore {
		directive.Policy = InterruptionPolicyIgnore
		if directive.Reason == "" {
			directive.Reason = "normal"
		}
		return directive
	}

	switch directive.Policy {
	case InterruptionPolicyBackchannel:
		directive.Action = PlaybackActionDuckLight
		directive.Gain = 0.72
		directive.Attack = 45 * time.Millisecond
		directive.Hold = clampDirectiveDuration(
			time.Duration(maxInt(decision.AudioMs+80, 180))*time.Millisecond,
			180*time.Millisecond,
			420*time.Millisecond,
		)
		directive.Release = 180 * time.Millisecond
		directive.KeepPreview = true
		directive.KeepPendingAudio = true
	case InterruptionPolicyDuckOnly:
		directive.Action = PlaybackActionDuckHold
		directive.Gain = 0.36
		directive.Attack = 30 * time.Millisecond
		directive.Hold = recommendedDuckHold(decision)
		directive.Release = 220 * time.Millisecond
		directive.KeepPreview = true
		directive.KeepPendingAudio = true
	case InterruptionPolicyHardInterrupt:
		directive.Action = PlaybackActionInterrupt
		directive.Gain = 0
		directive.KeepPreview = true
		directive.KeepPendingAudio = true
	default:
		directive.Policy = InterruptionPolicyIgnore
	}
	if directive.Reason == "" {
		directive.Reason = string(directive.Action)
	}
	return directive
}

func recommendedDuckHold(decision BargeInDecision) time.Duration {
	remainingMs := 0
	switch strings.TrimSpace(decision.Reason) {
	case "duck_pending_min_audio":
		remainingMs = maxInt(decision.MinAudioMs-decision.AudioMs, 0)
	case "duck_pending_audio_only":
		remainingMs = maxInt(decision.HoldAudioMs, 0)
	default:
		remainingMs = maxInt(decision.MinAudioMs+decision.HoldAudioMs-decision.AudioMs, 0)
	}
	base := time.Duration(remainingMs)*time.Millisecond + 80*time.Millisecond
	return clampDirectiveDuration(base, 180*time.Millisecond, 720*time.Millisecond)
}

func clampDirectiveDuration(value, minValue, maxValue time.Duration) time.Duration {
	if value < minValue {
		return minValue
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampUnit(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}
