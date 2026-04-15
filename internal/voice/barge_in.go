package voice

import (
	"strings"
	"time"
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
	Accepted          bool
	Policy            InterruptionPolicy
	Reason            string
	AudioMs           int
	LexicallyComplete bool
	MinAudioMs        int
	HoldAudioMs       int
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
	if looksLikeBackchannel(partialText) && audioMs >= bargeInBackchannelMinAudioMs(cfg.MinAudioMs) {
		decision.Policy = InterruptionPolicyBackchannel
		decision.Reason = "backchannel_short_ack"
		return decision
	}
	if audioMs < cfg.MinAudioMs {
		if partialText != "" {
			decision.Policy = InterruptionPolicyDuckOnly
			decision.Reason = "duck_pending_min_audio"
			return decision
		}
		decision.Reason = "below_min_audio"
		return decision
	}
	decision.LexicallyComplete = looksLexicallyComplete(partialText)
	if decision.LexicallyComplete {
		decision.Policy = InterruptionPolicyHardInterrupt
		decision.Accepted = true
		decision.Reason = "accepted_complete_preview"
		return decision
	}
	if audioMs >= cfg.MinAudioMs+cfg.IncompleteHoldMs {
		decision.Policy = InterruptionPolicyHardInterrupt
		decision.Accepted = true
		decision.Reason = "accepted_incomplete_after_hold"
		return decision
	}
	decision.Policy = InterruptionPolicyDuckOnly
	if partialText == "" {
		decision.Reason = "duck_pending_audio_only"
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
