package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"agent-server/internal/session"
	"agent-server/pkg/events"
)

const (
	previewEventModePreviewV1        = "preview_v1"
	playbackAckModeSegmentMarkV1     = "segment_mark_v1"
	playbackEventSourceServerPreview = "server_preview"
)

type previewEventsProfile struct {
	Enabled           bool   `json:"enabled"`
	SpeechStart       bool   `json:"speech_start"`
	Partial           bool   `json:"partial"`
	EndpointCandidate bool   `json:"endpoint_candidate"`
	Mode              string `json:"mode"`
}

type playbackAckProfile struct {
	Enabled   bool   `json:"enabled"`
	Started   bool   `json:"started"`
	Mark      bool   `json:"mark"`
	Cleared   bool   `json:"cleared"`
	Completed bool   `json:"completed"`
	Mode      string `json:"mode"`
}

type voiceCollaborationProfile struct {
	PreviewEvents previewEventsProfile `json:"preview_events"`
	PlaybackAck   playbackAckProfile   `json:"playback_ack"`
}

type sessionStartPlaybackAckCapability struct {
	Mode string `json:"mode"`
}

type collaborationNegotiation struct {
	PreviewEvents bool
	PlaybackAck   sessionStartPlaybackAckCapability
}

type inputSpeechStartPayload struct {
	PreviewID     string `json:"preview_id"`
	AudioOffsetMs int    `json:"audio_offset_ms"`
	Source        string `json:"source"`
}

type inputPreviewPayload struct {
	PreviewID     string   `json:"preview_id"`
	Text          string   `json:"text"`
	StablePrefix  string   `json:"stable_prefix,omitempty"`
	IsFinal       bool     `json:"is_final"`
	Stability     *float64 `json:"stability,omitempty"`
	AudioOffsetMs int      `json:"audio_offset_ms"`
}

type inputEndpointPayload struct {
	PreviewID     string `json:"preview_id"`
	Candidate     bool   `json:"candidate"`
	Reason        string `json:"reason"`
	AudioOffsetMs int    `json:"audio_offset_ms"`
}

type audioOutMetaPayload struct {
	ResponseID         string `json:"response_id"`
	PlaybackID         string `json:"playback_id"`
	SegmentID          string `json:"segment_id"`
	Text               string `json:"text,omitempty"`
	ExpectedDurationMs int    `json:"expected_duration_ms"`
	IsLastSegment      bool   `json:"is_last_segment"`
}

type audioOutStartedPayload struct {
	ResponseID string `json:"response_id"`
	PlaybackID string `json:"playback_id"`
	SegmentID  string `json:"segment_id"`
}

type audioOutMarkPayload struct {
	ResponseID       string `json:"response_id"`
	PlaybackID       string `json:"playback_id"`
	SegmentID        string `json:"segment_id"`
	PlayedDurationMs int    `json:"played_duration_ms,omitempty"`
}

type audioOutClearedPayload struct {
	ResponseID            string `json:"response_id"`
	PlaybackID            string `json:"playback_id"`
	ClearedAfterSegmentID string `json:"cleared_after_segment_id"`
	Reason                string `json:"reason"`
}

type audioOutCompletedPayload struct {
	ResponseID string `json:"response_id"`
	PlaybackID string `json:"playback_id"`
}

type audioPlaybackMeta struct {
	ResponseID       string
	PlaybackID       string
	SegmentID        string
	Text             string
	ExpectedDuration time.Duration
	IsLastSegment    bool
	SegmentIndex     int
	TextBefore       string
	TextAfter        string
	DurationBefore   time.Duration
	DurationAfter    time.Duration
}

type playbackAckState struct {
	mu            sync.Mutex
	meta          audioPlaybackMeta
	segments      map[string]audioPlaybackMeta
	startedAt     time.Time
	lastMarkMs    int
	clearedAt     time.Time
	clearedReason string
	completedAt   time.Time
	completed     bool
	terminal      playbackAckTerminal
	waitCh        chan struct{}
}

type playbackAckTerminal string

const (
	playbackAckTerminalNone      playbackAckTerminal = ""
	playbackAckTerminalCleared   playbackAckTerminal = "cleared"
	playbackAckTerminalCompleted playbackAckTerminal = "completed"
)

func (p RealtimeProfile) voiceCollaborationProfile() voiceCollaborationProfile {
	previewEnabled := p.ServerEndpointEnabled
	return voiceCollaborationProfile{
		PreviewEvents: previewEventsProfile{
			Enabled:           previewEnabled,
			SpeechStart:       previewEnabled,
			Partial:           previewEnabled,
			EndpointCandidate: previewEnabled,
			Mode:              previewEventModePreviewV1,
		},
		PlaybackAck: playbackAckProfile{
			Enabled:   true,
			Started:   true,
			Mark:      true,
			Cleared:   true,
			Completed: true,
			Mode:      playbackAckModeSegmentMarkV1,
		},
	}
}

func negotiateVoiceCollaboration(profile RealtimeProfile, caps sessionStartCapabilities) collaborationNegotiation {
	collaboration := profile.voiceCollaborationProfile()
	negotiated := collaborationNegotiation{
		PreviewEvents: collaboration.PreviewEvents.Enabled && caps.PreviewEvents,
	}
	if collaboration.PlaybackAck.Enabled && caps.PlaybackAck != nil && strings.TrimSpace(caps.PlaybackAck.Mode) == playbackAckModeSegmentMarkV1 {
		negotiated.PlaybackAck = *caps.PlaybackAck
	}
	return negotiated
}

func (n collaborationNegotiation) PlaybackAckEnabled() bool {
	return strings.TrimSpace(n.PlaybackAck.Mode) != ""
}

func (c sessionStartCapabilities) playbackAckMode() string {
	if c.PlaybackAck == nil {
		return ""
	}
	return strings.TrimSpace(c.PlaybackAck.Mode)
}

func newAudioPlaybackMeta(responseID, text string, expectedDuration time.Duration) audioPlaybackMeta {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		responseID = fmt.Sprintf("resp_%d", time.Now().UTC().UnixNano())
	}
	trimmedText := strings.TrimSpace(text)
	return audioPlaybackMeta{
		ResponseID:       responseID,
		PlaybackID:       fmt.Sprintf("playback_%s", responseID),
		SegmentID:        audioPlaybackSegmentID(responseID, 1),
		Text:             trimmedText,
		ExpectedDuration: expectedDuration,
		IsLastSegment:    true,
		SegmentIndex:     1,
		TextBefore:       "",
		TextAfter:        trimmedText,
		DurationBefore:   0,
		DurationAfter:    expectedDuration,
	}
}

func audioPlaybackSegmentID(responseID string, segmentIndex int) string {
	if segmentIndex <= 0 {
		segmentIndex = 1
	}
	return fmt.Sprintf("segment_%s_%04d", responseID, segmentIndex)
}

func nextSegmentAudioPlaybackMeta(base audioPlaybackMeta, segmentText string, expectedDuration time.Duration, isLast bool) audioPlaybackMeta {
	segmentText = strings.TrimSpace(segmentText)
	index := base.SegmentIndex + 1
	if index <= 0 {
		index = 1
	}
	textBefore := base.TextAfter
	textAfter := strings.TrimSpace(textBefore + segmentText)
	durationBefore := base.DurationAfter
	durationAfter := durationBefore + expectedDuration
	return audioPlaybackMeta{
		ResponseID:       base.ResponseID,
		PlaybackID:       base.PlaybackID,
		SegmentID:        audioPlaybackSegmentID(base.ResponseID, index),
		Text:             segmentText,
		ExpectedDuration: expectedDuration,
		IsLastSegment:    isLast,
		SegmentIndex:     index,
		TextBefore:       textBefore,
		TextAfter:        textAfter,
		DurationBefore:   durationBefore,
		DurationAfter:    durationAfter,
	}
}

func previewAudioOffsetMs(audioBytes, sampleRate, channels int) int {
	if audioBytes <= 0 || sampleRate <= 0 || channels <= 0 {
		return 0
	}
	bytesPerMillisecond := float64(sampleRate*channels*2) / 1000.0
	if bytesPerMillisecond <= 0 {
		return 0
	}
	return int(float64(audioBytes) / bytesPerMillisecond)
}

func audioDurationMs(duration time.Duration) int {
	if duration <= 0 {
		return 0
	}
	return int(duration / time.Millisecond)
}

func clampPlaybackMarkDuration(value, maxValue time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func playbackAckMetaForSegmentLocked(state *playbackAckState, segmentID string) audioPlaybackMeta {
	if state == nil {
		return audioPlaybackMeta{}
	}
	if trimmed := strings.TrimSpace(segmentID); trimmed != "" {
		if meta, ok := state.segments[trimmed]; ok {
			return meta
		}
	}
	return state.meta
}

func heardTextForPlaybackSegment(meta audioPlaybackMeta, localPlayed time.Duration) string {
	local := strings.TrimSpace(meta.Text)
	if local == "" {
		return strings.TrimSpace(meta.TextBefore)
	}
	heardLocal := voiceHeardTextForPlayback(local, localPlayed, meta.ExpectedDuration)
	return strings.TrimSpace(strings.TrimSpace(meta.TextBefore) + heardLocal)
}

func voiceHeardTextForPlayback(text string, playedDuration, plannedDuration time.Duration) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || playedDuration <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) == 0 {
		return ""
	}
	if plannedDuration > 0 {
		ratio := float64(playedDuration) / float64(plannedDuration)
		if ratio >= 1 {
			return trimmed
		}
		count := int(math.Ceil(float64(len(runes)) * ratio))
		if count <= 0 {
			count = 1
		}
		if count > len(runes) {
			count = len(runes)
		}
		return string(runes[:count])
	}
	count := int(playedDuration / (110 * time.Millisecond))
	if count <= 0 {
		count = 1
	}
	if count > len(runes) {
		count = len(runes)
	}
	return string(runes[:count])
}

func (h *realtimeWSHandler) emitPreviewObservationEvents(runtime *connectionRuntime, snapshot session.Snapshot, observation inputPreviewObservation) error {
	if !runtime.collaboration.PreviewEvents {
		return nil
	}
	previewID := strings.TrimSpace(observation.Trace.PreviewID)
	if previewID == "" {
		return nil
	}
	audioOffsetMs := previewAudioOffsetMs(observation.Preview.AudioBytes, snapshot.InputSampleRate, snapshot.InputChannels)
	if observation.SpeechStartedObserved {
		if err := runtime.peer.WriteEvent(events.TypeInputSpeechStart, snapshot.SessionID, inputSpeechStartPayload{
			PreviewID:     previewID,
			AudioOffsetMs: audioOffsetMs,
			Source:        playbackEventSourceServerPreview,
		}); err != nil {
			return err
		}
	}
	if observation.PartialChanged {
		text := strings.TrimSpace(observation.Preview.PartialText)
		stablePrefix := strings.TrimSpace(observation.Preview.StablePrefix)
		if stablePrefix == "" {
			stablePrefix = text
		}
		stability := previewTextStability(text, stablePrefix)
		if err := runtime.peer.WriteEvent(events.TypeInputPreview, snapshot.SessionID, inputPreviewPayload{
			PreviewID:     previewID,
			Text:          text,
			StablePrefix:  stablePrefix,
			IsFinal:       false,
			Stability:     stability,
			AudioOffsetMs: audioOffsetMs,
		}); err != nil {
			return err
		}
	}
	if observation.EndpointCandidateObserved {
		if err := runtime.peer.WriteEvent(events.TypeInputEndpoint, snapshot.SessionID, inputEndpointPayload{
			PreviewID:     previewID,
			Candidate:     true,
			Reason:        firstNonEmpty(strings.TrimSpace(observation.Preview.EndpointReason), "server_endpoint_candidate"),
			AudioOffsetMs: audioOffsetMs,
		}); err != nil {
			return err
		}
	}
	return nil
}

func previewTextStability(text, stablePrefix string) *float64 {
	textRunes := []rune(strings.TrimSpace(text))
	if len(textRunes) == 0 {
		return nil
	}
	stableRunes := []rune(strings.TrimSpace(stablePrefix))
	value := float64(len(stableRunes)) / float64(len(textRunes))
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	return &value
}

func (r *connectionRuntime) installPlaybackAckMeta(meta audioPlaybackMeta) {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	r.playbackAckState.meta = audioPlaybackMeta{
		ResponseID: meta.ResponseID,
		PlaybackID: meta.PlaybackID,
	}
	r.playbackAckState.segments = make(map[string]audioPlaybackMeta, 4)
	r.playbackAckState.startedAt = time.Time{}
	r.playbackAckState.lastMarkMs = 0
	r.playbackAckState.clearedAt = time.Time{}
	r.playbackAckState.clearedReason = ""
	r.playbackAckState.completedAt = time.Time{}
	r.playbackAckState.completed = false
	r.playbackAckState.terminal = playbackAckTerminalNone
	r.playbackAckState.waitCh = make(chan struct{})
}

func (r *connectionRuntime) clearPlaybackAckState() {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	r.playbackAckState.meta = audioPlaybackMeta{}
	r.playbackAckState.segments = nil
	r.playbackAckState.startedAt = time.Time{}
	r.playbackAckState.lastMarkMs = 0
	r.playbackAckState.clearedAt = time.Time{}
	r.playbackAckState.clearedReason = ""
	r.playbackAckState.completedAt = time.Time{}
	r.playbackAckState.completed = false
	r.playbackAckState.terminal = playbackAckTerminalNone
	r.playbackAckState.waitCh = nil
}

func (r *connectionRuntime) playbackAckMeta() audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	return r.playbackAckState.meta
}

func (r *connectionRuntime) syncAnnouncedPlaybackContext() {
	if r == nil || r.voiceSession == nil {
		return
	}
	meta := r.playbackAckMeta()
	deliveredText := strings.TrimSpace(meta.TextAfter)
	if deliveredText == "" {
		deliveredText = strings.TrimSpace(meta.Text)
	}
	plannedDuration := meta.DurationAfter
	if plannedDuration <= 0 {
		plannedDuration = meta.ExpectedDuration
	}
	if deliveredText == "" && plannedDuration <= 0 {
		return
	}
	r.voiceSession.UpdatePlayback(deliveredText, plannedDuration)
}

func (r *connectionRuntime) activatePlaybackAckSegment(meta audioPlaybackMeta) audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	if r.playbackAckState.segments == nil {
		r.playbackAckState.segments = make(map[string]audioPlaybackMeta, 4)
	}
	if strings.TrimSpace(meta.SegmentID) != "" {
		r.playbackAckState.segments[meta.SegmentID] = meta
	}
	r.playbackAckState.meta = meta
	return meta
}

func (r *connectionRuntime) recordPlaybackStarted(now time.Time) audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	if r.playbackAckState.startedAt.IsZero() {
		r.playbackAckState.startedAt = now
	}
	return r.playbackAckState.meta
}

func (r *connectionRuntime) recordPlaybackMark(now time.Time, segmentID string, playedDurationMs int) (audioPlaybackMeta, time.Duration, string) {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	meta := playbackAckMetaForSegmentLocked(&r.playbackAckState, segmentID)
	localPlayed := clampPlaybackMarkDuration(time.Duration(maxInt(playedDurationMs, 0))*time.Millisecond, meta.ExpectedDuration)
	totalPlayed := meta.DurationBefore + localPlayed
	totalPlayedMs := audioDurationMs(totalPlayed)
	if totalPlayedMs > r.playbackAckState.lastMarkMs {
		r.playbackAckState.lastMarkMs = totalPlayedMs
	}
	if r.playbackAckState.startedAt.IsZero() {
		r.playbackAckState.startedAt = now
	}
	return meta, totalPlayed, heardTextForPlaybackSegment(meta, localPlayed)
}

func (r *connectionRuntime) recordPlaybackCleared(now time.Time, segmentID, reason string) (audioPlaybackMeta, time.Duration, string) {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	meta := playbackAckMetaForSegmentLocked(&r.playbackAckState, segmentID)
	totalPlayedMs := audioDurationMs(meta.DurationAfter)
	if totalPlayedMs > r.playbackAckState.lastMarkMs {
		r.playbackAckState.lastMarkMs = totalPlayedMs
	}
	r.playbackAckState.clearedAt = now
	r.playbackAckState.clearedReason = strings.TrimSpace(reason)
	r.playbackAckState.finishLocked(playbackAckTerminalCleared)
	return meta, meta.DurationAfter, strings.TrimSpace(meta.TextAfter)
}

func (r *connectionRuntime) recordPlaybackCompleted(now time.Time) audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	r.playbackAckState.completedAt = now
	r.playbackAckState.completed = true
	r.playbackAckState.finishLocked(playbackAckTerminalCompleted)
	return r.playbackAckState.meta
}

func (s *playbackAckState) finishLocked(terminal playbackAckTerminal) {
	if s.terminal != playbackAckTerminalNone {
		return
	}
	s.terminal = terminal
	if s.waitCh != nil {
		close(s.waitCh)
	}
}

func (r *connectionRuntime) waitForPlaybackAckTerminal(ctx context.Context, timeout time.Duration) (playbackAckTerminal, string, bool) {
	r.playbackAckState.mu.Lock()
	waitCh := r.playbackAckState.waitCh
	terminal := r.playbackAckState.terminal
	reason := r.playbackAckState.clearedReason
	r.playbackAckState.mu.Unlock()
	if terminal != playbackAckTerminalNone {
		return terminal, reason, true
	}
	if waitCh == nil {
		return playbackAckTerminalNone, "", false
	}
	if timeout <= 0 {
		timeout = 600 * time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return playbackAckTerminalNone, "", false
	case <-waitCh:
	case <-timer.C:
		return playbackAckTerminalNone, "", false
	}
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	return r.playbackAckState.terminal, r.playbackAckState.clearedReason, r.playbackAckState.terminal != playbackAckTerminalNone
}

func playbackAckTerminalWaitTimeout(meta audioPlaybackMeta) time.Duration {
	switch {
	case meta.ExpectedDuration >= 1500*time.Millisecond:
		return 1200 * time.Millisecond
	case meta.ExpectedDuration >= 600*time.Millisecond:
		return 900 * time.Millisecond
	default:
		return 600 * time.Millisecond
	}
}

func validatePlaybackAckMode(mode string) error {
	mode = strings.TrimSpace(mode)
	switch mode {
	case "", playbackAckModeSegmentMarkV1:
		return nil
	default:
		return fmt.Errorf("unsupported playback_ack.mode %s", mode)
	}
}

func validatePlaybackAckIdentity(meta audioPlaybackMeta, responseID, playbackID string) bool {
	if strings.TrimSpace(responseID) == "" || strings.TrimSpace(playbackID) == "" {
		return false
	}
	if meta.ResponseID == "" || meta.PlaybackID == "" {
		return false
	}
	return meta.ResponseID == responseID && meta.PlaybackID == playbackID
}

func logPlaybackAckInfo(logger *slog.Logger, msg string, runtime *connectionRuntime, payload any, attrs ...any) {
	base := []any{
		"session_id", runtime.session.Snapshot().SessionID,
		"remote_addr", runtime.remoteAddr,
		"payload", payload,
	}
	base = appendPlaybackAckPayloadLogAttrs(base, payload)
	base = append(base, attrs...)
	logger.Info(msg, base...)
}

func appendPlaybackAckPayloadLogAttrs(attrs []any, payload any) []any {
	switch v := payload.(type) {
	case audioOutStartedPayload:
		return append(attrs,
			"response_id", strings.TrimSpace(v.ResponseID),
			"playback_id", strings.TrimSpace(v.PlaybackID),
			"segment_id", strings.TrimSpace(v.SegmentID),
		)
	case *audioOutStartedPayload:
		if v == nil {
			return attrs
		}
		return appendPlaybackAckPayloadLogAttrs(attrs, *v)
	case audioOutMarkPayload:
		return append(attrs,
			"response_id", strings.TrimSpace(v.ResponseID),
			"playback_id", strings.TrimSpace(v.PlaybackID),
			"segment_id", strings.TrimSpace(v.SegmentID),
			"played_duration_ms", v.PlayedDurationMs,
		)
	case *audioOutMarkPayload:
		if v == nil {
			return attrs
		}
		return appendPlaybackAckPayloadLogAttrs(attrs, *v)
	case audioOutClearedPayload:
		return append(attrs,
			"response_id", strings.TrimSpace(v.ResponseID),
			"playback_id", strings.TrimSpace(v.PlaybackID),
			"cleared_after_segment_id", strings.TrimSpace(v.ClearedAfterSegmentID),
			"cleared_reason", strings.TrimSpace(v.Reason),
		)
	case *audioOutClearedPayload:
		if v == nil {
			return attrs
		}
		return appendPlaybackAckPayloadLogAttrs(attrs, *v)
	case audioOutCompletedPayload:
		return append(attrs,
			"response_id", strings.TrimSpace(v.ResponseID),
			"playback_id", strings.TrimSpace(v.PlaybackID),
		)
	case *audioOutCompletedPayload:
		if v == nil {
			return attrs
		}
		return appendPlaybackAckPayloadLogAttrs(attrs, *v)
	default:
		return attrs
	}
}
