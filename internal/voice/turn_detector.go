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
	defaultTurnDetectorHintSilenceMs    = 160
	defaultTurnDetectorAcceptLeadMs     = 120
	defaultTurnDetectorLexicalMode      = "conservative"
	turnDetectorLexicalModeOff          = "off"
	turnDetectorLexicalModeConservative = "conservative"
	defaultServerEndpointReason         = "server_silence_timeout"
	lexicalHoldServerEndpointReason     = "server_lexical_hold_timeout"
	defaultPrewarmStableRatio           = 0.6
	defaultDraftStableRatio             = 0.82
)

type SilenceTurnDetectorConfig struct {
	MinAudioMs            int
	SilenceMs             int
	LexicalEndpointMode   string
	IncompleteHoldMs      int
	EndpointHintSilenceMs int
}

type SilenceTurnDetector struct {
	config             SilenceTurnDetectorConfig
	sampleRateHz       int
	channels           int
	audioBytes         int
	lastAudioAt        time.Time
	speechStarted      bool
	latestPartial      string
	stablePrefix       string
	latestEndpointHint string
	commitSuggested    bool
	commitReason       string
}

type MultiSignalTurnArbitrator struct {
	cfg SilenceTurnDetectorConfig
}

type turnArbitrationInput struct {
	partialText       string
	stablePrefix      string
	audioMs           int
	silence           time.Duration
	requiredSilence   time.Duration
	endpointReason    string
	utteranceComplete bool
}

func NewSilenceTurnDetector(cfg SilenceTurnDetectorConfig, sampleRateHz, channels int) SilenceTurnDetector {
	cfg = NormalizeSilenceTurnDetectorConfig(cfg)
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
			d.stablePrefix = updateStablePrefix(d.latestPartial, d.stablePrefix, text, delta.Kind == TranscriptionDeltaKindFinal)
			d.latestPartial = text
		}
		d.latestEndpointHint = strings.TrimSpace(delta.EndpointReason)
	case TranscriptionDeltaKindSpeechEnd:
		d.latestEndpointHint = strings.TrimSpace(delta.EndpointReason)
	}
}

func (d *SilenceTurnDetector) Snapshot(now time.Time) InputPreview {
	partialText := strings.TrimSpace(d.latestPartial)
	stablePrefix := strings.TrimSpace(d.stablePrefix)
	audioMs := d.audioDurationMs()
	utteranceComplete := looksLexicallyComplete(firstNonEmpty(stablePrefix, partialText))
	preview := InputPreview{
		PartialText:       d.latestPartial,
		StablePrefix:      d.stablePrefix,
		AudioBytes:        d.audioBytes,
		SpeechStarted:     d.speechStarted,
		EndpointReason:    "",
		UtteranceComplete: utteranceComplete,
	}
	if partialText == "" || d.lastAudioAt.IsZero() {
		preview.Arbitration = TurnArbitration{
			Stage:             TurnArbitrationStagePreviewOnly,
			AudioMs:           audioMs,
			Stability:         previewTextStabilityRatio(partialText, stablePrefix),
			RequiredSilenceMs: d.config.SilenceMs,
		}
		return preview
	}
	requiredSilence, endpointReason := d.requiredSilenceForPartial()
	arbitration := NewMultiSignalTurnArbitrator(d.config).Arbitrate(turnArbitrationInput{
		partialText:       partialText,
		stablePrefix:      stablePrefix,
		audioMs:           audioMs,
		silence:           now.Sub(d.lastAudioAt),
		requiredSilence:   requiredSilence,
		endpointReason:    strings.TrimSpace(endpointReason),
		utteranceComplete: utteranceComplete,
	})
	if d.commitSuggested {
		arbitration.Stage = TurnArbitrationStageAcceptNow
		arbitration.AcceptCandidate = true
		arbitration.AcceptNow = true
		arbitration.Reason = d.commitReason
	}
	if arbitration.AcceptNow {
		d.commitSuggested = true
		d.commitReason = firstNonEmpty(strings.TrimSpace(arbitration.Reason), endpointReason)
		preview.CommitSuggested = true
	}
	if arbitration.AcceptCandidate {
		preview.EndpointReason = firstNonEmpty(strings.TrimSpace(arbitration.Reason), endpointReason)
	}
	preview.Arbitration = arbitration
	return preview
}

func NewMultiSignalTurnArbitrator(cfg SilenceTurnDetectorConfig) MultiSignalTurnArbitrator {
	return MultiSignalTurnArbitrator{cfg: NormalizeSilenceTurnDetectorConfig(cfg)}
}

func NormalizeSilenceTurnDetectorConfig(cfg SilenceTurnDetectorConfig) SilenceTurnDetectorConfig {
	if cfg.MinAudioMs <= 0 {
		cfg.MinAudioMs = defaultTurnDetectorMinAudioMs
	}
	if cfg.SilenceMs <= 0 {
		cfg.SilenceMs = defaultTurnDetectorSilenceMs
	}
	if cfg.IncompleteHoldMs <= 0 {
		cfg.IncompleteHoldMs = defaultTurnDetectorIncompleteHoldMs
	}
	if cfg.EndpointHintSilenceMs <= 0 {
		cfg.EndpointHintSilenceMs = defaultTurnDetectorHintSilenceMs
	}
	cfg.LexicalEndpointMode = normalizeTurnDetectorLexicalMode(cfg.LexicalEndpointMode)
	return cfg
}

func (a MultiSignalTurnArbitrator) Arbitrate(input turnArbitrationInput) TurnArbitration {
	stability := previewTextStabilityRatio(input.partialText, input.stablePrefix)
	arbitration := TurnArbitration{
		Stage:             TurnArbitrationStagePreviewOnly,
		Reason:            strings.TrimSpace(input.endpointReason),
		Stability:         stability,
		AudioMs:           input.audioMs,
		SilenceMs:         durationMs(input.silence),
		RequiredSilenceMs: durationMs(input.requiredSilence),
		EndpointHinted:    strings.TrimSpace(input.endpointReason) != "",
	}
	if strings.TrimSpace(input.partialText) == "" {
		return arbitration
	}

	stablePrefix := strings.TrimSpace(input.stablePrefix)
	if stablePrefix != "" && stability >= defaultPrewarmStableRatio {
		arbitration.Stage = TurnArbitrationStagePrewarmAllowed
		arbitration.PrewarmAllowed = input.utteranceComplete
	}
	if input.utteranceComplete && stability >= defaultDraftStableRatio {
		arbitration.Stage = TurnArbitrationStageDraftAllowed
		arbitration.PrewarmAllowed = true
		arbitration.DraftAllowed = true
	}
	candidateLead := time.Duration(defaultTurnDetectorAcceptLeadMs) * time.Millisecond
	if !input.utteranceComplete {
		if input.audioMs >= a.cfg.MinAudioMs && input.requiredSilence > 0 && input.silence+candidateLead >= input.requiredSilence {
			arbitration.Stage = TurnArbitrationStageAcceptCandidate
			arbitration.AcceptCandidate = true
		}
		if input.audioMs >= a.cfg.MinAudioMs && input.silence >= input.requiredSilence {
			arbitration.Stage = TurnArbitrationStageAcceptNow
			arbitration.AcceptCandidate = true
			arbitration.AcceptNow = true
		}
		if arbitration.AcceptNow || arbitration.AcceptCandidate {
			return arbitration
		}
		if arbitration.Stage == TurnArbitrationStagePreviewOnly {
			arbitration.Stage = TurnArbitrationStageWaitForMore
		}
		return arbitration
	}

	arbitration.PrewarmAllowed = true
	arbitration.DraftAllowed = true
	if input.audioMs >= a.cfg.MinAudioMs && input.requiredSilence > 0 && input.silence+candidateLead >= input.requiredSilence {
		arbitration.Stage = TurnArbitrationStageAcceptCandidate
		arbitration.AcceptCandidate = true
	}
	if input.audioMs >= a.cfg.MinAudioMs && input.silence >= input.requiredSilence {
		arbitration.Stage = TurnArbitrationStageAcceptNow
		arbitration.AcceptCandidate = true
		arbitration.AcceptNow = true
	}
	return arbitration
}

func updateStablePrefix(previousPartial, previousStable, nextPartial string, final bool) string {
	next := strings.TrimSpace(nextPartial)
	if next == "" {
		return strings.TrimSpace(previousStable)
	}
	if final {
		return next
	}
	prev := strings.TrimSpace(previousPartial)
	stable := strings.TrimSpace(previousStable)
	if prev == "" {
		return stable
	}
	if prev == next {
		return next
	}
	common := strings.TrimSpace(longestCommonPrefix(prev, next))
	if common == "" {
		return stable
	}
	if stable == "" || strings.HasPrefix(common, stable) {
		return common
	}
	return stable
}

func longestCommonPrefix(a, b string) string {
	aRunes := []rune(a)
	bRunes := []rune(b)
	limit := len(aRunes)
	if len(bRunes) < limit {
		limit = len(bRunes)
	}
	idx := 0
	for idx < limit && aRunes[idx] == bRunes[idx] {
		idx++
	}
	if idx == 0 {
		return ""
	}
	return string(aRunes[:idx])
}

func previewTextStabilityRatio(text, stablePrefix string) float64 {
	textRunes := []rune(strings.TrimSpace(text))
	if len(textRunes) == 0 {
		return 0
	}
	stableRunes := []rune(strings.TrimSpace(stablePrefix))
	value := float64(len(stableRunes)) / float64(len(textRunes))
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func durationMs(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	return int(value / time.Millisecond)
}

func (d *SilenceTurnDetector) audioDurationMs() int {
	return pcm16AudioDurationMs(d.audioBytes, d.sampleRateHz, d.channels)
}

func (d *SilenceTurnDetector) requiredSilenceForPartial() (time.Duration, string) {
	required := time.Duration(d.config.SilenceMs) * time.Millisecond
	if hint := strings.TrimSpace(d.latestEndpointHint); hint != "" && looksLexicallyComplete(d.latestPartial) {
		hintSilence := time.Duration(d.config.EndpointHintSilenceMs) * time.Millisecond
		if hintSilence > 0 && hintSilence < required {
			return hintSilence, hint
		}
		return required, hint
	}
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
	if matchesAnyValue(trimmed, chineseStandaloneHesitations) {
		return false
	}
	if tokens := asciiWordTokens(trimmed); len(tokens) == 1 && englishStandaloneHesitations[tokens[0]] {
		return false
	}
	if strings.HasSuffix(trimmed, "...") || strings.HasSuffix(trimmed, "……") {
		return false
	}
	lastRune := runeAtEnd(trimmed)
	switch lastRune {
	case '.', '!', '?', '。', '！', '？':
		return true
	case ',', '，', '、', ';', '；', ':', '：', '(', '（', '[', '{', '<', '"', '\'', '“', '‘', '-', '—', '…':
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

func matchesAnyValue(text string, values []string) bool {
	for _, value := range values {
		if text == value {
			return true
		}
	}
	return false
}

func asciiWordTokens(text string) []string {
	var tokens []string
	var builder strings.Builder
	flush := func() {
		if builder.Len() == 0 {
			return
		}
		tokens = append(tokens, builder.String())
		builder.Reset()
	}
	for _, r := range text {
		if r <= unicode.MaxASCII && unicode.IsLetter(r) {
			builder.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return tokens
}

var chineseIncompleteSuffixes = []string{
	"然后",
	"然后呢",
	"还有",
	"还有呢",
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

var chineseStandaloneHesitations = []string{
	"嗯",
	"嗯嗯",
	"呃",
	"额",
	"啊",
	"啊啊",
	"哦",
	"噢",
	"诶",
	"欸",
	"唉",
	"那个",
	"这个",
	"就是",
	"然后",
	"还有",
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

var englishStandaloneHesitations = map[string]bool{
	"uh":  true,
	"um":  true,
	"hmm": true,
	"mm":  true,
	"erm": true,
}
