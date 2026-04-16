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
	defaultPrewarmStableForMs           = 120
	defaultDraftStableForMs             = 200
	turnDetectorLexicalModeOff          = "off"
	turnDetectorLexicalModeConservative = "conservative"
	defaultServerEndpointReason         = "server_silence_timeout"
	lexicalHoldServerEndpointReason     = "server_lexical_hold_timeout"
	correctionHoldServerEndpointReason  = "server_correction_hold_timeout"
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
	stablePrefixAt     time.Time
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
	stableFor         time.Duration
	endpointReason    string
	utteranceComplete bool
}

type utteranceCompletenessResult struct {
	Complete   bool
	HoldReason string
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

func (d *SilenceTurnDetector) ObserveTranscriptionDelta(now time.Time, delta TranscriptionDelta) {
	switch delta.Kind {
	case TranscriptionDeltaKindSpeechStart:
		d.speechStarted = true
	case TranscriptionDeltaKindPartial, TranscriptionDeltaKindFinal:
		if text := delta.Text; text != "" {
			previousStable := strings.TrimSpace(d.stablePrefix)
			nextStable := updateStablePrefix(d.latestPartial, d.stablePrefix, text, delta.Kind == TranscriptionDeltaKindFinal)
			trimmedStable := strings.TrimSpace(nextStable)
			if trimmedStable == "" {
				d.stablePrefixAt = time.Time{}
			} else if trimmedStable != previousStable || d.stablePrefixAt.IsZero() {
				d.stablePrefixAt = now
			}
			d.stablePrefix = nextStable
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
	completeness := previewUtteranceCompleteness(stablePrefix, partialText)
	utteranceComplete := completeness.Complete
	stableFor := d.stablePrefixDuration(now)
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
		stableFor:         stableFor,
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
		StableForMs:       durationMs(input.stableFor),
		AudioMs:           input.audioMs,
		SilenceMs:         durationMs(input.silence),
		RequiredSilenceMs: durationMs(input.requiredSilence),
		EndpointHinted:    strings.TrimSpace(input.endpointReason) != "",
	}
	if strings.TrimSpace(input.partialText) == "" {
		return arbitration
	}

	// 这里把早处理门槛拆成三层：
	// 1) 成熟 stable prefix 只允许低风险 prewarm
	// 2) live partial 自身已完整时，才允许 draft 级前推
	// 3) 真正 accept 仍然要再叠加静音/时长等收尾条件
	stablePrefix := strings.TrimSpace(input.stablePrefix)
	stablePrefixComplete := analyzeUtteranceCompleteness(stablePrefix).Complete
	if stablePrefix != "" &&
		stablePrefixComplete &&
		stability >= defaultPrewarmStableRatio &&
		input.stableFor >= time.Duration(defaultPrewarmStableForMs)*time.Millisecond {
		arbitration.Stage = TurnArbitrationStagePrewarmAllowed
		arbitration.PrewarmAllowed = true
	}
	if input.utteranceComplete &&
		stability >= defaultDraftStableRatio &&
		input.stableFor >= time.Duration(defaultDraftStableForMs)*time.Millisecond {
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

	// 一旦 live partial 自身已经像一句完整话，就允许更激进的可撤销前推；
	// 更细的稳定前缀驻留时间只影响“未完整 utterance 时能否先做低风险 prewarm”。
	arbitration.PrewarmAllowed = true
	arbitration.DraftAllowed = true
	if arbitration.Stage == TurnArbitrationStagePreviewOnly {
		arbitration.Stage = TurnArbitrationStageDraftAllowed
	}
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
		// 对连续重复的 incomplete partial，不要把“然后 / 不对”这类尾巴直接并入
		// stable prefix；保留前面那段已完整的安全前缀，更适合做低风险 prewarm。
		if stable != "" &&
			strings.HasPrefix(next, stable) &&
			analyzeUtteranceCompleteness(stable).Complete &&
			!analyzeUtteranceCompleteness(next).Complete {
			return stable
		}
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

func (d *SilenceTurnDetector) stablePrefixDuration(now time.Time) time.Duration {
	if strings.TrimSpace(d.stablePrefix) == "" || d.stablePrefixAt.IsZero() {
		return 0
	}
	if now.Before(d.stablePrefixAt) {
		return 0
	}
	return now.Sub(d.stablePrefixAt)
}

func (d *SilenceTurnDetector) requiredSilenceForPartial() (time.Duration, string) {
	required := time.Duration(d.config.SilenceMs) * time.Millisecond
	if d.config.LexicalEndpointMode == turnDetectorLexicalModeOff {
		return required, defaultServerEndpointReason
	}
	completeness := analyzeUtteranceCompleteness(d.latestPartial)
	if hint := strings.TrimSpace(d.latestEndpointHint); hint != "" && completeness.Complete {
		hintSilence := time.Duration(d.config.EndpointHintSilenceMs) * time.Millisecond
		if hintSilence > 0 && hintSilence < required {
			return hintSilence, hint
		}
		return required, hint
	}
	if completeness.Complete {
		return required, defaultServerEndpointReason
	}
	reason := firstNonEmpty(strings.TrimSpace(completeness.HoldReason), lexicalHoldServerEndpointReason)
	return required + time.Duration(d.config.IncompleteHoldMs)*time.Millisecond, reason
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

func previewUtteranceCompleteness(stablePrefix, partialText string) utteranceCompletenessResult {
	stableCandidate := strings.TrimSpace(firstNonEmpty(stablePrefix, partialText))
	if stableCandidate == "" {
		return utteranceCompletenessResult{}
	}

	stableCompleteness := analyzeUtteranceCompleteness(stableCandidate)
	if trimmedPartial := strings.TrimSpace(partialText); trimmedPartial != "" {
		liveCompleteness := analyzeUtteranceCompleteness(trimmedPartial)
		if !liveCompleteness.Complete {
			return liveCompleteness
		}
	}
	return stableCompleteness
}

func analyzeUtteranceCompleteness(text string) utteranceCompletenessResult {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return utteranceCompletenessResult{}
	}
	if looksCorrectionPending(trimmed) {
		return utteranceCompletenessResult{
			Complete:   false,
			HoldReason: correctionHoldServerEndpointReason,
		}
	}
	if !looksLexicallyComplete(trimmed) {
		return utteranceCompletenessResult{
			Complete:   false,
			HoldReason: lexicalHoldServerEndpointReason,
		}
	}
	return utteranceCompletenessResult{Complete: true}
}

func looksCorrectionPending(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	for _, suffix := range chineseCorrectionSuffixes {
		if !strings.HasSuffix(trimmed, suffix) {
			continue
		}
		prefix := strings.TrimSpace(strings.TrimSuffix(trimmed, suffix))
		prefix = strings.TrimRight(prefix, " \t\r\n,，。.!?！？;；:：、")
		if prefix != "" {
			return true
		}
	}
	if token := trailingEnglishToken(trimmed); englishCorrectionTokens[token] {
		prefix := strings.TrimSpace(trimmed[:len(trimmed)-len(token)])
		prefix = strings.TrimRight(prefix, " \t\r\n,，。.!?！？;；:：、")
		return prefix != ""
	}
	return false
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

var chineseCorrectionSuffixes = []string{
	"不对",
	"不是",
	"我是说",
	"我改一下",
	"改成",
	"换成",
	"等一下",
	"等等",
	"等会",
	"先别",
}

var englishCorrectionTokens = map[string]bool{
	"wait":     true,
	"sorry":    true,
	"actually": true,
	"instead":  true,
}
