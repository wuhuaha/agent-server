package gateway

import (
	"context"
	"strings"
	"sync"
	"time"

	"agent-server/internal/session"
	"agent-server/internal/voice"

	"github.com/gorilla/websocket"
)

const outputFrameInterval = 20 * time.Millisecond

type outputStream struct {
	cancel     context.CancelFunc
	done       chan struct{}
	completion *turnOutputOutcomeFuture
	effects    outputEffectState
}

type connectionRuntime struct {
	conn             *websocket.Conn
	peer             *wsPeer
	session          *session.RealtimeSession
	inputNormalizer  voice.InputNormalizer
	voiceSession     *voice.SessionOrchestrator
	turnTrace        turnTraceState
	previewTrace     inputPreviewTraceState
	collaboration    collaborationNegotiation
	playbackAckState playbackAckState
	remoteAddr       string

	outputMu         sync.Mutex
	output           *outputStream
	pendingBargeInMu sync.Mutex
	pendingBargeIn   pendingBargeInAudio
}

func newConnectionRuntime(conn *websocket.Conn, peer *wsPeer, rtSession *session.RealtimeSession, responder voice.Responder) *connectionRuntime {
	remoteAddr := ""
	if conn != nil && conn.RemoteAddr() != nil {
		remoteAddr = conn.RemoteAddr().String()
	}
	return &connectionRuntime{
		conn:         conn,
		peer:         peer,
		session:      rtSession,
		voiceSession: voice.NewSessionOrchestratorFromResponder(responder),
		remoteAddr:   remoteAddr,
	}
}

func newOutputStream(cancel context.CancelFunc, completion *turnOutputOutcomeFuture) *outputStream {
	return &outputStream{
		cancel:     cancel,
		done:       make(chan struct{}),
		completion: completion,
	}
}

func (r *connectionRuntime) attachOutput(stream *outputStream) {
	if stream == nil {
		return
	}

	r.outputMu.Lock()
	r.output = stream
	r.outputMu.Unlock()
}

func (r *connectionRuntime) installOutput(cancel context.CancelFunc, completion *turnOutputOutcomeFuture) *outputStream {
	stream := newOutputStream(cancel, completion)
	r.attachOutput(stream)
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
	return r.interruptOutputWithPolicy(voice.InterruptionPolicyHardInterrupt, "hard_interrupt", wait)
}

func (r *connectionRuntime) interruptOutputWithDecision(decision voice.BargeInDecision, wait time.Duration) bool {
	return r.interruptOutputWithPolicy(decision.Policy, decision.Reason, wait)
}

func (r *connectionRuntime) interruptOutputWithPolicy(policy voice.InterruptionPolicy, reason string, wait time.Duration) bool {
	r.outputMu.Lock()
	stream := r.output
	r.outputMu.Unlock()

	if stream == nil {
		return false
	}
	directive := (voice.BargeInDecision{Policy: policy, Reason: reason}).PlaybackDirective()
	if !directive.ShouldInterruptOutput() {
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
		if normalized := strings.TrimSpace(string(policy)); normalized != "" {
			r.voiceSession.InterruptPlaybackWithPolicy(policy, reason)
		} else {
			r.voiceSession.InterruptPlayback()
		}
	}
	return true
}

func (r *connectionRuntime) applyOutputDirective(directive voice.PlaybackDirective) bool {
	r.outputMu.Lock()
	stream := r.output
	r.outputMu.Unlock()

	if stream == nil {
		return false
	}
	if !stream.effects.ApplyDirective(directive, time.Now().UTC()) {
		return false
	}
	if r.voiceSession != nil {
		r.voiceSession.RecordInterruptionPolicy(directive.Policy, directive.Reason)
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
