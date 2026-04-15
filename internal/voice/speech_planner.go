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
)

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
	cfg      SpeechPlannerConfig
	observed strings.Builder
	pending  strings.Builder
}

func NewSpeechPlanner(cfg SpeechPlannerConfig) *SpeechPlanner {
	cfg = NormalizeSpeechPlannerConfig(cfg)
	return &SpeechPlanner{cfg: cfg}
}

func (p *SpeechPlanner) ObserveTextDelta(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	p.observed.WriteString(text)
	p.pending.WriteString(text)
	return p.drain(false)
}

func (p *SpeechPlanner) FinalizeText(text string) []string {
	tail := plannerRemainingTail(p.observed.String(), text)
	if strings.TrimSpace(tail) != "" {
		p.observed.WriteString(tail)
		p.pending.WriteString(tail)
	}
	segments := p.drain(true)
	remaining := strings.TrimSpace(p.pending.String())
	if remaining != "" {
		segments = append(segments, remaining)
	}
	p.pending.Reset()
	return segments
}

func (p *SpeechPlanner) drain(flush bool) []string {
	pending := strings.TrimSpace(p.pending.String())
	if pending == "" {
		p.pending.Reset()
		return nil
	}

	segments := make([]string, 0, 2)
	for {
		segment, rest := nextPlannedSpeechSegment(pending, p.cfg, flush)
		if segment == "" {
			break
		}
		segments = append(segments, segment)
		pending = rest
		if pending == "" {
			break
		}
	}

	p.pending.Reset()
	if pending != "" {
		p.pending.WriteString(pending)
	}
	return segments
}

type plannedSpeechSynthesis struct {
	queue       chan string
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
		queue:       make(chan string),
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
	for _, segment := range p.planner.ObserveTextDelta(delta.Text) {
		p.enqueue(segment)
	}
}

func (p *plannedSpeechSynthesis) Finalize(responseText string) AudioStream {
	if p == nil {
		return nil
	}
	for _, segment := range p.planner.FinalizeText(responseText) {
		p.enqueue(segment)
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

func (p *plannedSpeechSynthesis) enqueue(segment string) {
	if p == nil {
		return
	}
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" {
		return
	}
	p.enqueued = true
	p.stream.addPlannedText(trimmed)
	p.resolveStart(trimmed)
	select {
	case <-p.workerCtx.Done():
	case p.queue <- trimmed:
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
		case segment, ok := <-p.queue:
			if !ok {
				return
			}
			stream := synthesizedAudioStream(p.workerCtx, p.synthesizer, SynthesisRequest{
				SessionID: p.baseReq.SessionID,
				TurnID:    p.baseReq.TurnID,
				TraceID:   p.baseReq.TraceID,
				DeviceID:  p.baseReq.DeviceID,
				UserText:  p.baseReq.UserText,
				Text:      segment,
			})
			if stream == nil {
				continue
			}
			select {
			case <-p.workerCtx.Done():
				_ = stream.Close()
				return
			case p.stream.results <- queuedSpeechAudioResult{stream: stream}:
			}
		}
	}
}

type queuedSpeechAudioResult struct {
	stream AudioStream
	err    error
}

type queuedSpeechAudioStream struct {
	cancel    context.CancelFunc
	results   chan queuedSpeechAudioResult
	current   AudioStream
	closeOnce sync.Once
	mu        sync.Mutex
	planned   time.Duration
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
			continue
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result, ok := <-s.results:
			if !ok {
				return nil, io.EOF
			}
			if result.err != nil {
				return nil, result.err
			}
			if result.stream == nil {
				continue
			}
			s.current = result.stream
		}
	}
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

func (s *queuedSpeechAudioStream) addPlannedText(text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	s.mu.Lock()
	s.planned += estimatePlannedSpeechDuration(trimmed)
	s.mu.Unlock()
}

func (s *queuedSpeechAudioStream) PlaybackDuration(_ time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.planned
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
