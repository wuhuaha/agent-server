package gateway

import (
	"context"
	"time"

	"agent-server/internal/session"
	"agent-server/internal/voice"
)

const inputPreviewPollInterval = 80 * time.Millisecond

type inputTurnPreview struct {
	session          voice.InputPreviewSession
	last             voice.InputPreview
	lastPartialText  string
	lastCommitLogged bool
}

func (r *connectionRuntime) ensureInputPreview(ctx context.Context, responder voice.Responder, snapshot session.Snapshot, language string) error {
	r.previewMu.Lock()
	if r.preview != nil {
		r.previewMu.Unlock()
		return nil
	}
	r.previewMu.Unlock()

	previewer, ok := responder.(voice.InputPreviewer)
	if !ok {
		return nil
	}
	session, err := previewer.StartInputPreview(ctx, voice.InputPreviewRequest{
		SessionID:    snapshot.SessionID,
		DeviceID:     snapshot.DeviceID,
		Codec:        snapshot.InputCodec,
		SampleRateHz: snapshot.InputSampleRate,
		Channels:     snapshot.InputChannels,
		Language:     language,
	})
	if err != nil {
		return err
	}
	r.previewMu.Lock()
	defer r.previewMu.Unlock()
	if r.preview != nil {
		_ = session.Close()
		return nil
	}
	r.preview = &inputTurnPreview{session: session}
	return nil
}

func (r *connectionRuntime) pushInputPreviewAudio(ctx context.Context, payload []byte) (voice.InputPreview, bool, error) {
	r.previewMu.Lock()
	preview := r.preview
	r.previewMu.Unlock()
	if preview == nil || preview.session == nil {
		return voice.InputPreview{}, false, nil
	}
	snapshot, err := preview.session.PushAudio(ctx, payload)
	if err != nil {
		return voice.InputPreview{}, false, err
	}
	r.previewMu.Lock()
	defer r.previewMu.Unlock()
	if r.preview == nil {
		return snapshot, false, nil
	}
	changed := snapshot.PartialText != "" && snapshot.PartialText != r.preview.lastPartialText
	r.preview.last = snapshot
	if changed {
		r.preview.lastPartialText = snapshot.PartialText
	}
	return snapshot, changed, nil
}

func (r *connectionRuntime) pollInputPreview(now time.Time) (voice.InputPreview, bool, bool, bool) {
	r.previewMu.Lock()
	preview := r.preview
	r.previewMu.Unlock()
	if preview == nil || preview.session == nil {
		return voice.InputPreview{}, false, false, false
	}
	snapshot := preview.session.Poll(now)
	r.previewMu.Lock()
	defer r.previewMu.Unlock()
	if r.preview == nil {
		return snapshot, false, false, false
	}
	changed := snapshot.PartialText != "" && snapshot.PartialText != r.preview.lastPartialText
	if changed {
		r.preview.lastPartialText = snapshot.PartialText
	}
	commitNew := snapshot.CommitSuggested && !r.preview.lastCommitLogged
	if commitNew {
		r.preview.lastCommitLogged = true
	}
	r.preview.last = snapshot
	return snapshot, true, changed, commitNew
}

func (r *connectionRuntime) clearInputPreview() {
	r.previewMu.Lock()
	preview := r.preview
	r.preview = nil
	r.previewMu.Unlock()
	if preview != nil && preview.session != nil {
		_ = preview.session.Close()
	}
}

func (r *connectionRuntime) previewReadDeadline(now time.Time) time.Time {
	r.previewMu.Lock()
	defer r.previewMu.Unlock()
	if r.preview == nil || r.preview.session == nil {
		return time.Time{}
	}
	return now.Add(inputPreviewPollInterval)
}
