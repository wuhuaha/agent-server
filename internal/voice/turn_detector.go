package voice

import (
	"strings"
	"time"
	"unicode"
)

const (
	defaultTurnDetectorSilenceMs        = 480
	defaultTurnDetectorMinAudioMs       = 320
	defaultTurnDetectorIncompleteHoldMs = 720
	defaultTurnDetectorLexicalMode      = "conservative"
	turnDetectorLexicalModeOff          = "off"
	turnDetectorLexicalModeConservative = "conservative"
	defaultServerEndpointReason         = "server_silence_timeout"
	lexicalHoldServerEndpointReason     = "server_lexical_hold_timeout"
)

type SilenceTurnDetectorConfig struct {
	MinAudioMs          int
	SilenceMs           int
	LexicalEndpointMode string
	IncompleteHoldMs    int
}

type SilenceTurnDetector struct {
	config          SilenceTurnDetectorConfig
	sampleRateHz    int
	channels        int
	audioBytes      int
	lastAudioAt     time.Time
	speechStarted   bool
	latestPartial   string
	commitSuggested bool
	commitReason    string
}

func NewSilenceTurnDetector(cfg SilenceTurnDetectorConfig, sampleRateHz, channels int) SilenceTurnDetector {
	if cfg.MinAudioMs <= 0 {
		cfg.MinAudioMs = defaultTurnDetectorMinAudioMs
	}
	if cfg.SilenceMs <= 0 {
		cfg.SilenceMs = defaultTurnDetectorSilenceMs
	}
	if cfg.IncompleteHoldMs <= 0 {
		cfg.IncompleteHoldMs = defaultTurnDetectorIncompleteHoldMs
	}
	cfg.LexicalEndpointMode = normalizeTurnDetectorLexicalMode(cfg.LexicalEndpointMode)
	if sampleRateHz <= 0 {
		sampleRateHz = 16000
	}
	if channels <= 0 {
		channels = 1
	}
	return SilenceTurnDetector{
		config:       cfg,
		sampleRateHz: sampleRateHz,
		channels:     channels,
	}
}

func (d *SilenceTurnDetector) ObserveAudio(now time.Time, audioBytes int) {
	if audioBytes <= 0 {
		return
	}
	d.audioBytes += audioBytes
	d.lastAudioAt = now
	d.speechStarted = true
}

func (d *SilenceTurnDetector) ObserveTranscriptionDelta(_ time.Time, delta TranscriptionDelta) {
	switch delta.Kind {
	case TranscriptionDeltaKindSpeechStart:
		d.speechStarted = true
	case TranscriptionDeltaKindPartial, TranscriptionDeltaKindFinal:
		if text := delta.Text; text != "" {
			d.latestPartial = text
		}
	}
}

func (d *SilenceTurnDetector) Snapshot(now time.Time) InputPreview {
	preview := InputPreview{
		PartialText:    d.latestPartial,
		AudioBytes:     d.audioBytes,
		SpeechStarted:  d.speechStarted,
		EndpointReason: "",
	}
	if d.commitSuggested {
		preview.CommitSuggested = true
		preview.EndpointReason = d.commitReason
		return preview
	}
	if d.latestPartial == "" || d.audioDurationMs() < d.config.MinAudioMs || d.lastAudioAt.IsZero() {
		return preview
	}
	requiredSilence, endpointReason := d.requiredSilenceForPartial()
	if now.Sub(d.lastAudioAt) < requiredSilence {
		return preview
	}
	d.commitSuggested = true
	d.commitReason = endpointReason
	preview.CommitSuggested = true
	preview.EndpointReason = endpointReason
	return preview
}

func (d *SilenceTurnDetector) audioDurationMs() int {
	if d.sampleRateHz <= 0 || d.channels <= 0 {
		return 0
	}
	return int(float64(d.audioBytes) / float64(d.channels) / 2.0 / float64(d.sampleRateHz) * 1000.0)
}

func (d *SilenceTurnDetector) requiredSilenceForPartial() (time.Duration, string) {
	required := time.Duration(d.config.SilenceMs) * time.Millisecond
	if d.config.LexicalEndpointMode == turnDetectorLexicalModeOff || looksLexicallyComplete(d.latestPartial) {
		return required, defaultServerEndpointReason
	}
	return required + time.Duration(d.config.IncompleteHoldMs)*time.Millisecond, lexicalHoldServerEndpointReason
}

func normalizeTurnDetectorLexicalMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", turnDetectorLexicalModeConservative:
		return turnDetectorLexicalModeConservative
	case turnDetectorLexicalModeOff:
		return turnDetectorLexicalModeOff
	default:
		return turnDetectorLexicalModeConservative
	}
}

func looksLexicallyComplete(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	lastRune := runeAtEnd(trimmed)
	switch lastRune {
	case '.', '!', '?', '。', '！', '？':
		return true
	case ',', '，', '、', ';', '；', ':', '：', '(', '（', '[', '{', '<', '"', '\'', '“', '‘':
		return false
	}
	if matchesAnySuffix(trimmed, chineseIncompleteSuffixes) {
		return false
	}
	if token := trailingEnglishToken(trimmed); token != "" && englishIncompleteTokens[token] {
		return false
	}
	return true
}

func runeAtEnd(text string) rune {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}
	return runes[len(runes)-1]
}

func trailingEnglishToken(text string) string {
	var builder strings.Builder
	for i := len(text) - 1; i >= 0; i-- {
		ch := rune(text[i])
		if ch > unicode.MaxASCII {
			break
		}
		if unicode.IsLetter(ch) {
			builder.WriteRune(unicode.ToLower(ch))
			continue
		}
		if builder.Len() > 0 {
			break
		}
		if unicode.IsSpace(ch) || strings.ContainsRune(".,!?;:()[]{}<>\"'`", ch) {
			continue
		}
		break
	}
	if builder.Len() == 0 {
		return ""
	}
	runes := []rune(builder.String())
	for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
		runes[left], runes[right] = runes[right], runes[left]
	}
	return string(runes)
}

func matchesAnySuffix(text string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(text, suffix) {
			return true
		}
	}
	return false
}

var chineseIncompleteSuffixes = []string{
	"然后",
	"还有",
	"而且",
	"或者",
	"但是",
	"不过",
	"因为",
	"所以",
	"如果",
	"帮我",
	"请帮我",
	"麻烦帮我",
	"把",
	"给",
	"到",
	"调到",
	"切到",
	"设到",
	"就是",
	"那个",
	"这个",
	"再",
	"先",
}

var englishIncompleteTokens = map[string]bool{
	"and":     true,
	"or":      true,
	"but":     true,
	"if":      true,
	"when":    true,
	"because": true,
	"to":      true,
	"for":     true,
	"with":    true,
	"then":    true,
	"please":  true,
}
