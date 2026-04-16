package gateway

import (
	"fmt"
	"log/slog"
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
}

type playbackAckState struct {
	mu            sync.Mutex
	meta          audioPlaybackMeta
	startedAt     time.Time
	lastMarkMs    int
	clearedAt     time.Time
	clearedReason string
	completedAt   time.Time
	completed     bool
}

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
	return audioPlaybackMeta{
		ResponseID:       responseID,
		PlaybackID:       fmt.Sprintf("playback_%s", responseID),
		SegmentID:        fmt.Sprintf("segment_%s_0001", responseID),
		Text:             strings.TrimSpace(text),
		ExpectedDuration: expectedDuration,
		IsLastSegment:    true,
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
		if err := runtime.peer.WriteEvent(events.TypeInputPreview, snapshot.SessionID, inputPreviewPayload{
			PreviewID:     previewID,
			Text:          text,
			StablePrefix:  text,
			IsFinal:       false,
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

func (r *connectionRuntime) installPlaybackAckMeta(meta audioPlaybackMeta) {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	r.playbackAckState.meta = meta
	r.playbackAckState.startedAt = time.Time{}
	r.playbackAckState.lastMarkMs = 0
	r.playbackAckState.clearedAt = time.Time{}
	r.playbackAckState.clearedReason = ""
	r.playbackAckState.completedAt = time.Time{}
	r.playbackAckState.completed = false
}

func (r *connectionRuntime) clearPlaybackAckState() {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	r.playbackAckState.meta = audioPlaybackMeta{}
	r.playbackAckState.startedAt = time.Time{}
	r.playbackAckState.lastMarkMs = 0
	r.playbackAckState.clearedAt = time.Time{}
	r.playbackAckState.clearedReason = ""
	r.playbackAckState.completedAt = time.Time{}
	r.playbackAckState.completed = false
}

func (r *connectionRuntime) playbackAckMeta() audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	return r.playbackAckState.meta
}

func (r *connectionRuntime) recordPlaybackStarted(now time.Time) audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	if r.playbackAckState.startedAt.IsZero() {
		r.playbackAckState.startedAt = now
	}
	return r.playbackAckState.meta
}

func (r *connectionRuntime) recordPlaybackMark(now time.Time, playedDurationMs int) audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	if playedDurationMs > r.playbackAckState.lastMarkMs {
		r.playbackAckState.lastMarkMs = playedDurationMs
	}
	if r.playbackAckState.startedAt.IsZero() {
		r.playbackAckState.startedAt = now
	}
	return r.playbackAckState.meta
}

func (r *connectionRuntime) recordPlaybackCleared(now time.Time, reason string) audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	r.playbackAckState.clearedAt = now
	r.playbackAckState.clearedReason = strings.TrimSpace(reason)
	return r.playbackAckState.meta
}

func (r *connectionRuntime) recordPlaybackCompleted(now time.Time) audioPlaybackMeta {
	r.playbackAckState.mu.Lock()
	defer r.playbackAckState.mu.Unlock()
	r.playbackAckState.completedAt = now
	r.playbackAckState.completed = true
	return r.playbackAckState.meta
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
		return true
	}
	return meta.ResponseID == responseID && meta.PlaybackID == playbackID
}

func logPlaybackAckInfo(logger *slog.Logger, msg string, runtime *connectionRuntime, payload any, attrs ...any) {
	base := []any{
		"session_id", runtime.session.Snapshot().SessionID,
		"remote_addr", runtime.remoteAddr,
		"payload", payload,
	}
	base = append(base, attrs...)
	logger.Info(msg, base...)
}
