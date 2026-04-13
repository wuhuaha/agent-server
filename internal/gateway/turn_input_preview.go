package gateway

import (
	"context"
	"time"

	"agent-server/internal/session"
	"agent-server/internal/voice"
)

func (r *connectionRuntime) ensureInputPreview(ctx context.Context, responder voice.Responder, snapshot session.Snapshot, language string) error {
	if r.voiceSession == nil {
		return nil
	}
	return r.voiceSession.EnsureInputPreview(ctx, responder, voice.InputPreviewRequest{
		SessionID:    snapshot.SessionID,
		DeviceID:     snapshot.DeviceID,
		Codec:        snapshot.InputCodec,
		SampleRateHz: snapshot.InputSampleRate,
		Channels:     snapshot.InputChannels,
		Language:     language,
	})
}

func (r *connectionRuntime) pushInputPreviewAudio(ctx context.Context, payload []byte) (voice.InputPreview, bool, error) {
	if r.voiceSession == nil {
		return voice.InputPreview{}, false, nil
	}
	observation, err := r.voiceSession.PushInputPreviewAudio(ctx, payload)
	return observation.Preview, observation.PartialChanged, err
}

func (r *connectionRuntime) pollInputPreview(now time.Time) (voice.InputPreview, bool, bool, bool) {
	if r.voiceSession == nil {
		return voice.InputPreview{}, false, false, false
	}
	observation := r.voiceSession.PollInputPreview(now)
	return observation.Preview, observation.Active, observation.PartialChanged, observation.CommitSuggested
}

func (r *connectionRuntime) clearInputPreview() {
	if r.voiceSession != nil {
		r.voiceSession.ClearInputPreview()
	}
}
