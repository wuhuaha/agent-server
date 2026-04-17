package voice

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	defaultSpeechPlannerMinChunkRunes    = 6
	defaultSpeechPlannerTargetChunkRunes = 24
	defaultSpeechPlannerMaxChunkRunes    = 42
	defaultSpeechPlannerQueueSize        = 4
)

type SpeechClauseBoundaryKind string

const (
	SpeechClauseBoundaryStrongStop   SpeechClauseBoundaryKind = "strong_stop"
	SpeechClauseBoundarySoftContinue SpeechClauseBoundaryKind = "soft_continue"
	SpeechClauseBoundaryForcedBreath SpeechClauseBoundaryKind = "forced_breath"
	SpeechClauseBoundaryFinalFlush   SpeechClauseBoundaryKind = "final_flush"
)

type SpeechClauseProsodyHint string

const (
	SpeechClauseProsodyNeutral          SpeechClauseProsodyHint = "neutral"
	SpeechClauseProsodyContinuationHold SpeechClauseProsodyHint = "continuation_hold"
	SpeechClauseProsodyFinalFall        SpeechClauseProsodyHint = "final_fall"
	SpeechClauseProsodyAckLead          SpeechClauseProsodyHint = "ack_lead"
)

type PlannedSpeechClause struct {
	Index                   int
	Text                    string
	BoundaryKind            SpeechClauseBoundaryKind
	ProsodyHint             SpeechClauseProsodyHint
	CanStartBeforeFinalized bool
	EstimatedDuration       time.Duration
}

type SpeechPlannerConfig struct {
	Enabled          bool
	MinChunkRunes    int
	TargetChunkRunes int
}

func NormalizeSpeechPlannerConfig(cfg SpeechPlannerConfig) SpeechPlannerConfig {
	if cfg.MinChunkRunes <= 0 {
		cfg.MinChunkRunes = defaultSpeechPlannerMinChunkRunes
	}
	if cfg.TargetChunkRunes <= 0 {
		cfg.TargetChunkRunes = defaultSpeechPlannerTargetChunkRunes
	}
	if cfg.TargetChunkRunes < cfg.MinChunkRunes {
		cfg.TargetChunkRunes = cfg.MinChunkRunes
	}
	return cfg
}

type SpeechPlanner struct {
	cfg             SpeechPlannerConfig
	observed        strings.Builder
	pending         strings.Builder
	nextClauseIndex int
}

func NewSpeechPlanner(cfg SpeechPlannerConfig) *SpeechPlanner {
	cfg = NormalizeSpeechPlannerConfig(cfg)
	return &SpeechPlanner{cfg: cfg}
}

func (p *SpeechPlanner) ObserveTextDelta(text string) []string {
	return plannedSpeechClauseTexts(p.ObserveTextDeltaClauses(text))
}

func (p *SpeechPlanner) ObserveTextDeltaClauses(text string) []PlannedSpeechClause {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	p.observed.WriteString(text)
	p.pending.WriteString(text)
	return p.drainClauses(false)
}

func (p *SpeechPlanner) FinalizeText(text string) []string {
	return plannedSpeechClauseTexts(p.FinalizeTextClauses(text))
}

func (p *SpeechPlanner) FinalizeTextClauses(text string) []PlannedSpeechClause {
	tail := plannerRemainingTail(p.observed.String(), text)
	if strings.TrimSpace(tail) != "" {
		p.observed.WriteString(tail)
		p.pending.WriteString(tail)
	}
	clauses := p.drainClauses(true)
	remaining := strings.TrimSpace(p.pending.String())
	if remaining != "" {
		clauses = append(clauses, p.nextClause(remaining, SpeechClauseBoundaryFinalFlush, false))
	}
	p.pending.Reset()
	return clauses
}

func (p *SpeechPlanner) drainClauses(flush bool) []PlannedSpeechClause {
	pending := strings.TrimSpace(p.pending.String())
	if pending == "" {
		p.pending.Reset()
		return nil
	}

	clauses := make([]PlannedSpeechClause, 0, 2)
	for {
		clause, rest := nextPlannedSpeechClause(pending, p.cfg, flush, !flush, p.nextClauseIndex)
		if strings.TrimSpace(clause.Text) == "" {
			break
		}
		clauses = append(clauses, clause)
		p.nextClauseIndex++
		pending = rest
		if pending == "" {
			break
		}
	}

	p.pending.Reset()
	if pending != "" {
		p.pending.WriteString(pending)
	}
	return clauses
}

func (p *SpeechPlanner) nextClause(text string, boundary SpeechClauseBoundaryKind, canStartBeforeFinalized bool) PlannedSpeechClause {
	clause := classifyPlannedSpeechClause(text, boundary, canStartBeforeFinalized)
	clause.Index = p.nextClauseIndex
	p.nextClauseIndex++
	return clause
}

type plannedSpeechSynthesis struct {
	queue       chan PlannedSpeechClause
	stream      *queuedSpeechAudioStream
	workerCtx   context.Context
	queueClose  sync.Once
	planner     *SpeechPlanner
	baseReq     SynthesisRequest
	synthesizer Synthesizer
	enqueued    bool
	startOnce   sync.Once
	startReady  chan struct{}
	startMu     sync.Mutex
	startText   string
	started     bool
}

func newPlannedSpeechSynthesis(ctx context.Context, synthesizer Synthesizer, req SynthesisRequest, cfg SpeechPlannerConfig) *plannedSpeechSynthesis {
	cfg = NormalizeSpeechPlannerConfig(cfg)
	if synthesizer == nil || !cfg.Enabled {
		return nil
	}

	workerCtx, cancel := context.WithCancel(ctx)
	stream := newQueuedSpeechAudioStream(cancel)
	planner := &plannedSpeechSynthesis{
		queue:       make(chan PlannedSpeechClause, defaultSpeechPlannerQueueSize),
		stream:      stream,
		workerCtx:   workerCtx,
		planner:     NewSpeechPlanner(cfg),
		baseReq:     req,
		synthesizer: synthesizer,
		startReady:  make(chan struct{}),
	}
	go planner.runWorker()
	return planner
}

func (p *plannedSpeechSynthesis) ObserveDelta(delta ResponseDelta) {
	if p == nil || delta.Kind != ResponseDeltaKindText {
		return
	}
	for _, clause := range p.planner.ObserveTextDeltaClauses(delta.Text) {
		p.enqueueClause(clause)
	}
}

func (p *plannedSpeechSynthesis) Finalize(responseText string) AudioStream {
	if p == nil {
		return nil
	}
	for _, clause := range p.planner.FinalizeTextClauses(responseText) {
		p.enqueueClause(clause)
	}
	p.closeQueue()
	if !p.enqueued {
		p.resolveStart("")
		p.stream.Close()
		return nil
	}
	return p.stream
}

func (p *plannedSpeechSynthesis) Close() {
	if p == nil {
		return
	}
	p.closeQueue()
	p.resolveStart("")
	p.stream.Close()
}

func (p *plannedSpeechSynthesis) closeQueue() {
	if p == nil {
		return
	}
	p.queueClose.Do(func() {
		close(p.queue)
	})
}

func (p *plannedSpeechSynthesis) enqueueClause(clause PlannedSpeechClause) {
	if p == nil {
		return
	}
	trimmed := strings.TrimSpace(clause.Text)
	if trimmed == "" {
		return
	}
	clause.Text = trimmed
	if clause.EstimatedDuration <= 0 {
		clause.EstimatedDuration = estimateClausePlaybackDuration(clause)
	}
	p.enqueued = true
	p.stream.addPlannedClause(clause)
	p.resolveStart(trimmed)
	select {
	case <-p.workerCtx.Done():
	case p.queue <- clause:
	}
}

func (p *plannedSpeechSynthesis) WaitAudioStart(ctx context.Context) (ResponseAudioStart, bool, error) {
	if p == nil {
		return ResponseAudioStart{}, false, nil
	}

	select {
	case <-ctx.Done():
		return ResponseAudioStart{}, false, ctx.Err()
	case <-p.startReady:
	}

	p.startMu.Lock()
	defer p.startMu.Unlock()
	if !p.started {
		return ResponseAudioStart{}, false, nil
	}
	return ResponseAudioStart{
		Stream:      p.stream,
		Text:        p.startText,
		Incremental: true,
		Source:      ResponseAudioStartSourceSpeechPlanner,
	}, true, nil
}

func (p *plannedSpeechSynthesis) resolveStart(text string) {
	if p == nil {
		return
	}
	trimmed := strings.TrimSpace(text)
	p.startOnce.Do(func() {
		p.startMu.Lock()
		p.startText = trimmed
		p.started = trimmed != ""
		p.startMu.Unlock()
		close(p.startReady)
	})
}

func (p *plannedSpeechSynthesis) runWorker() {
	defer close(p.stream.results)
	for {
		select {
		case <-p.workerCtx.Done():
			return
		case clause, ok := <-p.queue:
			if !ok {
				return
			}
			stream := synthesizedPlannedClauseStream(p.workerCtx, p.synthesizer, p.baseReq, clause)
			if stream == nil {
				continue
			}
			select {
			case <-p.workerCtx.Done():
				_ = stream.Close()
				return
			case p.stream.results <- queuedSpeechAudioResult{stream: stream, segment: playbackSegmentForClause(clause)}:
			}
		}
	}
}

type queuedSpeechAudioResult struct {
	stream  AudioStream
	segment PlaybackSegment
	err     error
}

type queuedSpeechAudioStream struct {
	cancel    context.CancelFunc
	results   chan queuedSpeechAudioResult
	current   AudioStream
	closeOnce sync.Once
	mu        sync.Mutex
	planned   time.Duration
	segmented bool
}

func newQueuedSpeechAudioStream(cancel context.CancelFunc) *queuedSpeechAudioStream {
	return &queuedSpeechAudioStream{
		cancel:  cancel,
		results: make(chan queuedSpeechAudioResult, 1),
	}
}

func (s *queuedSpeechAudioStream) Next(ctx context.Context) ([]byte, error) {
	for {
		if s.current != nil {
			chunk, err := s.current.Next(ctx)
			if err == nil {
				return chunk, nil
			}
			if err != io.EOF {
				return nil, err
			}
			_ = s.current.Close()
			s.current = nil
			if s.segmented {
				return nil, io.EOF
			}
			continue
		}

		result, ok, err := s.loadNextResult(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, io.EOF
		}
		if result.stream == nil {
			continue
		}
	}
}

func (s *queuedSpeechAudioStream) NextSegment(ctx context.Context) (PlaybackSegment, bool, error) {
	s.segmented = true
	if s.current != nil {
		return PlaybackSegment{}, false, nil
	}
	result, ok, err := s.loadNextResult(ctx)
	if err != nil {
		return PlaybackSegment{}, false, err
	}
	if !ok {
		return PlaybackSegment{}, false, io.EOF
	}
	if result.stream == nil {
		return PlaybackSegment{}, false, nil
	}
	return result.segment, true, nil
}

func (s *queuedSpeechAudioStream) Close() error {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.current != nil {
			_ = s.current.Close()
			s.current = nil
		}
		for {
			select {
			case result, ok := <-s.results:
				if !ok {
					return
				}
				if result.stream != nil {
					_ = result.stream.Close()
				}
			default:
				return
			}
		}
	})
	return nil
}

func (s *queuedSpeechAudioStream) addPlannedClause(clause PlannedSpeechClause) {
	if strings.TrimSpace(clause.Text) == "" {
		return
	}
	s.mu.Lock()
	duration := clause.EstimatedDuration
	if duration <= 0 {
		duration = estimateClausePlaybackDuration(clause)
	}
	s.planned += duration
	s.mu.Unlock()
}

func (s *queuedSpeechAudioStream) PlaybackDuration(_ time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.planned
}

func (s *queuedSpeechAudioStream) loadNextResult(ctx context.Context) (queuedSpeechAudioResult, bool, error) {
	select {
	case <-ctx.Done():
		return queuedSpeechAudioResult{}, false, ctx.Err()
	case result, ok := <-s.results:
		if !ok {
			return queuedSpeechAudioResult{}, false, nil
		}
		if result.err != nil {
			return queuedSpeechAudioResult{}, false, result.err
		}
		s.current = result.stream
		return result, true, nil
	}
}

func estimatePlannedSpeechDuration(text string) time.Duration {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return 0
	}

	// Keep this intentionally simple: we want a stable heard-text estimate for
	// incremental audio streams, not phoneme-accurate duration prediction.
	duration := time.Duration(len(runes)) * 110 * time.Millisecond
	for _, r := range runes {
		switch r {
		case '，', ',', '、':
			duration += 120 * time.Millisecond
		case '。', '.', '！', '!', '？', '?', ';', '；', ':', '：':
			duration += 220 * time.Millisecond
		}
	}
	return duration
}

func estimateClausePlaybackDuration(clause PlannedSpeechClause) time.Duration {
	if clause.EstimatedDuration > 0 {
		return clause.EstimatedDuration
	}
	return estimatePlannedSpeechDuration(clause.Text)
}

func playbackSegmentForClause(clause PlannedSpeechClause) PlaybackSegment {
	return PlaybackSegment{
		Index:            clause.Index,
		Text:             strings.TrimSpace(clause.Text),
		ExpectedDuration: estimateClausePlaybackDuration(clause),
		IsLastSegment:    clause.BoundaryKind == SpeechClauseBoundaryFinalFlush,
	}
}

func nextPlannedSpeechClause(text string, cfg SpeechPlannerConfig, flush bool, canStartBeforeFinalized bool, clauseIndex int) (PlannedSpeechClause, string) {
	segment, rest := nextPlannedSpeechSegment(text, cfg, flush)
	if strings.TrimSpace(segment) == "" {
		return PlannedSpeechClause{}, rest
	}
	boundary := classifySpeechClauseBoundary(segment, rest, flush)
	clause := classifyPlannedSpeechClause(segment, boundary, canStartBeforeFinalized)
	clause.Index = clauseIndex
	return clause, rest
}

func nextPlannedSpeechSegment(text string, cfg SpeechPlannerConfig, flush bool) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}
	cfg = NormalizeSpeechPlannerConfig(cfg)
	runes := []rune(trimmed)
	if len(runes) == 0 {
		return "", ""
	}

	cut := plannedSpeechBoundary(runes, cfg, flush)
	if cut <= 0 {
		return "", trimmed
	}
	if cut > len(runes) {
		cut = len(runes)
	}
	segment := strings.TrimSpace(string(runes[:cut]))
	rest := strings.TrimLeftFunc(string(runes[cut:]), unicode.IsSpace)
	return segment, rest
}

func plannedSpeechClauseTexts(clauses []PlannedSpeechClause) []string {
	if len(clauses) == 0 {
		return nil
	}
	texts := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		if trimmed := strings.TrimSpace(clause.Text); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}
	return texts
}

func classifySpeechClauseBoundary(segment, rest string, flush bool) SpeechClauseBoundaryKind {
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" {
		return SpeechClauseBoundaryFinalFlush
	}
	if flush && strings.TrimSpace(rest) == "" {
		return SpeechClauseBoundaryFinalFlush
	}
	last := runeAtEnd(trimmed)
	switch {
	case isStrongSpeechBoundary(last):
		return SpeechClauseBoundaryStrongStop
	case isSoftSpeechBoundary(last):
		return SpeechClauseBoundarySoftContinue
	case strings.TrimSpace(rest) != "":
		return SpeechClauseBoundaryForcedBreath
	default:
		return SpeechClauseBoundaryFinalFlush
	}
}

func classifyPlannedSpeechClause(text string, boundary SpeechClauseBoundaryKind, canStartBeforeFinalized bool) PlannedSpeechClause {
	trimmed := strings.TrimSpace(text)
	clause := PlannedSpeechClause{
		Text:                    trimmed,
		BoundaryKind:            boundary,
		CanStartBeforeFinalized: canStartBeforeFinalized,
	}
	switch {
	case looksLikeAckLeadClause(trimmed, boundary):
		clause.ProsodyHint = SpeechClauseProsodyAckLead
	case boundary == SpeechClauseBoundarySoftContinue || boundary == SpeechClauseBoundaryForcedBreath:
		clause.ProsodyHint = SpeechClauseProsodyContinuationHold
	case boundary == SpeechClauseBoundaryStrongStop || boundary == SpeechClauseBoundaryFinalFlush:
		clause.ProsodyHint = SpeechClauseProsodyFinalFall
	default:
		clause.ProsodyHint = SpeechClauseProsodyNeutral
	}
	clause.EstimatedDuration = estimatePlannedSpeechDuration(trimmed)
	return clause
}

func looksLikeAckLeadClause(text string, boundary SpeechClauseBoundaryKind) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || boundary != SpeechClauseBoundarySoftContinue {
		return false
	}
	if len([]rune(trimmed)) > 8 {
		return false
	}
	for _, token := range speechPlannerAckLeadTokens {
		if strings.HasPrefix(trimmed, token) {
			return true
		}
	}
	return false
}

var speechPlannerAckLeadTokens = []string{
	"好，",
	"好的，",
	"当然可以，",
	"没问题，",
	"可以，",
	"收到，",
	"明白，",
}

func plannedSpeechBoundary(runes []rune, cfg SpeechPlannerConfig, flush bool) int {
	total := len(runes)
	if total == 0 {
		return 0
	}

	maxChunkRunes := cfg.TargetChunkRunes + maxInt(cfg.TargetChunkRunes/2, 8)
	if maxChunkRunes < defaultSpeechPlannerMaxChunkRunes {
		maxChunkRunes = defaultSpeechPlannerMaxChunkRunes
	}

	lastStrong := 0
	lastSoft := 0
	lastSpace := 0
	firstStrong := 0
	firstSoft := 0
	for i, r := range runes {
		cut := i + 1
		switch {
		case isStrongSpeechBoundary(r):
			lastStrong = cut
			if firstStrong == 0 && cut >= cfg.MinChunkRunes {
				firstStrong = cut
			}
		case isSoftSpeechBoundary(r):
			lastSoft = cut
			if firstSoft == 0 && cut >= cfg.MinChunkRunes {
				firstSoft = cut
			}
		case unicode.IsSpace(r):
			lastSpace = cut
		}
	}

	if firstStrong > 0 && firstStrong < total {
		return firstStrong
	}
	if lastStrong >= cfg.MinChunkRunes {
		return lastStrong
	}
	if lastSoft >= cfg.MinChunkRunes && lastSoft == total {
		return lastSoft
	}
	if total >= cfg.TargetChunkRunes {
		if firstSoft > 0 && firstSoft < total {
			return firstSoft
		}
		if lastSoft >= cfg.MinChunkRunes {
			return lastSoft
		}
		if lastSpace >= cfg.MinChunkRunes {
			return lastSpace
		}
		if !flush {
			// 实时语音场景里，不能因为缺少逗号/句号就一直攒到超长再起播。
			// 一旦累计到目标长度，先按 target rune 强制切出首段，让 TTS 尽早启动。
			return cfg.TargetChunkRunes
		}
	}
	if total >= maxChunkRunes {
		switch {
		case lastSoft >= cfg.MinChunkRunes:
			return lastSoft
		case lastSpace >= cfg.MinChunkRunes:
			return lastSpace
		default:
			if cfg.TargetChunkRunes < total {
				return cfg.TargetChunkRunes
			}
			return total
		}
	}
	if flush {
		switch {
		case firstSoft > 0 && firstSoft < total:
			return firstSoft
		case lastSoft >= cfg.MinChunkRunes:
			return lastSoft
		case lastSpace >= cfg.MinChunkRunes:
			return lastSpace
		default:
			return total
		}
	}
	return 0
}

func isStrongSpeechBoundary(r rune) bool {
	switch r {
	case '.', '!', '?', ';', '\n', '。', '！', '？', '；', '…':
		return true
	default:
		return false
	}
}

func isSoftSpeechBoundary(r rune) bool {
	switch r {
	case ',', ':', '，', '、', '：':
		return true
	default:
		return false
	}
}

func plannerRemainingTail(observed, final string) string {
	if strings.TrimSpace(final) == "" {
		return ""
	}
	if observed == "" {
		return final
	}
	observedRunes := []rune(observed)
	finalRunes := []rune(final)
	match := 0
	for match < len(observedRunes) && match < len(finalRunes) {
		if observedRunes[match] != finalRunes[match] {
			break
		}
		match++
	}
	if match >= len(finalRunes) {
		return ""
	}
	return string(finalRunes[match:])
}
