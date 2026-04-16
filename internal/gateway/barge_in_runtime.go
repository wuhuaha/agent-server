package gateway

import (
	"context"
	"time"

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

func (r *connectionRuntime) resolvePostPlaybackActiveSnapshot() (session.Snapshot, bool, error) {
	snapshot := r.session.Snapshot()
	if snapshot.SessionID == "" {
		return session.Snapshot{}, false, nil
	}

	keepPreview := snapshot.InputState == session.InputStatePreviewing
	keepPending := r.hasPendingBargeInAudio()
	if !keepPreview && !keepPending {
		r.resetPendingBargeInAudio()
		r.clearInputPreview()
		return session.Snapshot{}, false, nil
	}

	active, err := r.session.SetOutputState(session.OutputStateIdle)
	if err != nil {
		return session.Snapshot{}, true, err
	}
	current := active
	if keepPending {
		flushed, err := r.flushPendingBargeInAudio()
		if err != nil {
			return session.Snapshot{}, true, err
		}
		if flushed.SessionID != "" {
			current = flushed
		}
	}
	if keepPreview {
		previewing, err := r.session.SetInputState(session.InputStatePreviewing)
		if err != nil {
			return session.Snapshot{}, true, err
		}
		current = previewing
	}
	return current, true, nil
}

func (r *connectionRuntime) previewForBargeIn(ctx context.Context, responder voice.Responder, snapshot session.Snapshot, payload []byte) inputPreviewObservation {
	observation := inputPreviewObservation{}
	if responder != nil {
		if err := r.ensureInputPreview(ctx, responder, snapshot, ""); err == nil {
			if pushed, pushErr := r.pushInputPreviewAudio(ctx, payload); pushErr == nil {
				if len(pushed) > 0 {
					// barge-in 只需要当前这一刻最新的 preview 视图，用最后一个 observation
					// 代表“这一帧音频推进完成后的最终状态”即可。
					observation = pushed[len(pushed)-1]
				}
			}
		}
	}
	observation.Preview = r.pendingBargeInPreview(observation.Preview)
	trace, _, _, _, _ := r.previewTrace.ObservePreview(snapshot.SessionID, observation.Preview, time.Now().UTC())
	observation.Trace = trace
	return observation
}
