package gateway

import (
	"context"
	"sync"
	"time"

	"agent-server/internal/session"
	"agent-server/internal/voice"

	"github.com/gorilla/websocket"
)

const outputFrameInterval = 20 * time.Millisecond

type outputStream struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type connectionRuntime struct {
	conn            *websocket.Conn
	peer            *wsPeer
	session         *session.RealtimeSession
	inputNormalizer voice.InputNormalizer
	voiceSession    *voice.SessionOrchestrator
	turnTrace       turnTraceState

	outputMu         sync.Mutex
	output           *outputStream
	pendingBargeInMu sync.Mutex
	pendingBargeIn   pendingBargeInAudio
}

func newConnectionRuntime(conn *websocket.Conn, peer *wsPeer, rtSession *session.RealtimeSession, responder voice.Responder) *connectionRuntime {
	return &connectionRuntime{
		conn:         conn,
		peer:         peer,
		session:      rtSession,
		voiceSession: voice.NewSessionOrchestratorFromResponder(responder),
	}
}

func (r *connectionRuntime) installOutput(cancel context.CancelFunc) *outputStream {
	stream := &outputStream{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	r.outputMu.Lock()
	r.output = stream
	r.outputMu.Unlock()

	return stream
}

func (r *connectionRuntime) clearOutput(stream *outputStream) {
	r.outputMu.Lock()
	if r.output == stream {
		r.output = nil
	}
	r.outputMu.Unlock()
}

func (r *connectionRuntime) interruptOutput(wait time.Duration) bool {
	r.outputMu.Lock()
	stream := r.output
	r.outputMu.Unlock()

	if stream == nil {
		return false
	}

	stream.cancel()
	if wait > 0 {
		select {
		case <-stream.done:
		case <-time.After(wait):
		}
	}
	if r.voiceSession != nil {
		r.voiceSession.InterruptPlayback()
	}
	return true
}

func applyReadDeadline(runtime *connectionRuntime, snapshot session.Snapshot, profile RealtimeProfile) error {
	deadline := readDeadlineForSnapshot(snapshot, profile)
	return runtime.conn.SetReadDeadline(deadline)
}

func readDeadlineForSnapshot(snapshot session.Snapshot, profile RealtimeProfile) time.Time {
	if snapshot.SessionID == "" {
		return time.Time{}
	}

	var deadline time.Time
	if profile.MaxSessionMs > 0 && !snapshot.StartedAt.IsZero() {
		deadline = snapshot.StartedAt.Add(time.Duration(profile.MaxSessionMs) * time.Millisecond)
	}

	if snapshot.State == session.StateActive && profile.IdleTimeoutMs > 0 && !snapshot.LastActivityAt.IsZero() {
		idleDeadline := snapshot.LastActivityAt.Add(time.Duration(profile.IdleTimeoutMs) * time.Millisecond)
		if deadline.IsZero() || idleDeadline.Before(deadline) {
			deadline = idleDeadline
		}
	}

	return deadline
}

func deadlineCloseReason(snapshot session.Snapshot, profile RealtimeProfile, now time.Time) (session.CloseReason, bool) {
	if snapshot.SessionID == "" {
		return "", false
	}

	if profile.MaxSessionMs > 0 && !snapshot.StartedAt.IsZero() {
		maxDeadline := snapshot.StartedAt.Add(time.Duration(profile.MaxSessionMs) * time.Millisecond)
		if !now.Before(maxDeadline) {
			return session.CloseReasonMaxDuration, true
		}
	}

	if snapshot.State == session.StateActive && profile.IdleTimeoutMs > 0 && !snapshot.LastActivityAt.IsZero() {
		idleDeadline := snapshot.LastActivityAt.Add(time.Duration(profile.IdleTimeoutMs) * time.Millisecond)
		if !now.Before(idleDeadline) {
			return session.CloseReasonIdle, true
		}
	}

	return "", false
}

func closeMessageForReason(reason session.CloseReason) string {
	switch reason {
	case session.CloseReasonIdle:
		return "session closed after idle timeout"
	case session.CloseReasonMaxDuration:
		return "session reached max duration"
	case session.CloseReasonCompleted:
		return "server ended the dialog"
	case session.CloseReasonError:
		return "session closed because of a server error"
	default:
		return string(reason)
	}
}
