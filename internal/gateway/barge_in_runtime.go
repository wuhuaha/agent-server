package gateway

import (
	"context"

	"agent-server/internal/session"
	"agent-server/internal/voice"
)

type pendingBargeInAudio struct {
	chunks     [][]byte
	audioBytes int
}

func (r *connectionRuntime) stagePendingBargeInAudio(payload []byte) {
	if len(payload) == 0 {
		return
	}
	cloned := append([]byte(nil), payload...)
	r.pendingBargeInMu.Lock()
	defer r.pendingBargeInMu.Unlock()
	r.pendingBargeIn.chunks = append(r.pendingBargeIn.chunks, cloned)
	r.pendingBargeIn.audioBytes += len(cloned)
}

func (r *connectionRuntime) hasPendingBargeInAudio() bool {
	r.pendingBargeInMu.Lock()
	defer r.pendingBargeInMu.Unlock()
	return len(r.pendingBargeIn.chunks) > 0
}

func (r *connectionRuntime) resetPendingBargeInAudio() {
	r.pendingBargeInMu.Lock()
	r.pendingBargeIn = pendingBargeInAudio{}
	r.pendingBargeInMu.Unlock()
}

func (r *connectionRuntime) pendingBargeInPreview(base voice.InputPreview) voice.InputPreview {
	r.pendingBargeInMu.Lock()
	defer r.pendingBargeInMu.Unlock()
	if r.pendingBargeIn.audioBytes > base.AudioBytes {
		base.AudioBytes = r.pendingBargeIn.audioBytes
	}
	if r.pendingBargeIn.audioBytes > 0 {
		base.SpeechStarted = true
	}
	return base
}

func (r *connectionRuntime) flushPendingBargeInAudio() (session.Snapshot, error) {
	r.pendingBargeInMu.Lock()
	pending := r.pendingBargeIn
	r.pendingBargeIn = pendingBargeInAudio{}
	r.pendingBargeInMu.Unlock()

	var snapshot session.Snapshot
	var err error
	for _, chunk := range pending.chunks {
		snapshot, err = r.session.IngestOwnedAudioFrame(chunk)
		if err != nil {
			return session.Snapshot{}, err
		}
	}
	return snapshot, nil
}

func (r *connectionRuntime) previewForBargeIn(ctx context.Context, responder voice.Responder, snapshot session.Snapshot, payload []byte) voice.InputPreview {
	preview := voice.InputPreview{}
	if responder != nil {
		if err := r.ensureInputPreview(ctx, responder, snapshot, ""); err == nil {
			if pushed, _, pushErr := r.pushInputPreviewAudio(ctx, payload); pushErr == nil {
				preview = pushed
			}
		}
	}
	return r.pendingBargeInPreview(preview)
}
