package gateway

import (
	"log/slog"
	"strings"
	"time"

	"agent-server/internal/session"
	"agent-server/internal/voice"
)

type turnCompletionHooks struct {
	Runtime             *connectionRuntime
	Profile             RealtimeProfile
	Logger              *slog.Logger
	SessionID           string
	ResolveReturnActive func() (session.Snapshot, bool, error)
	OnReturnActive      func(trace turnTrace, active session.Snapshot) error
	OnEndSession        func(trace turnTrace, reason session.CloseReason, message string) error
}

type interruptSpeakingOptions struct {
	ClearPreview bool
	ClearPending bool
	Decision     *voice.BargeInDecision
	Policy       voice.InterruptionPolicy
	Reason       string
}

type turnFinalizeHooks struct {
	Completion       turnCompletionHooks
	BeforeNoAudio    func(trace turnTrace, response voice.TurnResponse, aggregatedText string) error
	BeforeSpeaking   func(trace turnTrace) error
	StartAudioStream func(trace turnTrace, audioStream voice.AudioStream, response voice.TurnResponse, aggregatedText string) error
}

type playbackDurationAware interface {
	PlaybackDuration(frameDuration time.Duration) time.Duration
}

func finalizeTurnLifecycle(trace turnTrace, response voice.TurnResponse, aggregatedText string, hooks turnFinalizeHooks) error {
	hooks.Completion.Runtime.session.ClearTurn()

	audioStream := audioStreamForTurnResponse(response)
	if audioStream != nil {
		speaking, err := hooks.Completion.Runtime.session.SetOutputState(session.OutputStateSpeaking)
		if err != nil {
			return err
		}
		trace = hooks.Completion.Runtime.turnTrace.MarkSpeaking()
		logTurnTraceInfo(hooks.Completion.Logger, "gateway turn speaking", hooks.Completion.SessionID, trace,
			"speaking_latency_ms", trace.SpeakingLatencyMs(),
		)
		if err := applyReadDeadline(hooks.Completion.Runtime, speaking, hooks.Completion.Profile); err != nil {
			return err
		}
		if hooks.BeforeSpeaking != nil {
			if err := hooks.BeforeSpeaking(trace); err != nil {
				return err
			}
		}
		return hooks.StartAudioStream(trace, audioStream, response, aggregatedText)
	}

	if hooks.BeforeNoAudio != nil {
		if err := hooks.BeforeNoAudio(trace, response, aggregatedText); err != nil {
			return err
		}
	}

	return completeTurnReturnOrClose(trace, response.EndSession, response.EndReason, response.EndMessage, hooks.Completion)
}

func completeTurnReturnOrClose(trace turnTrace, endSession bool, endReason, endMessage string, hooks turnCompletionHooks) error {
	if endSession {
		reason := session.CloseReason(defaultCloseReason(endReason, string(session.CloseReasonCompleted)))
		message := strings.TrimSpace(endMessage)
		if message == "" {
			message = closeMessageForReason(reason)
		}
		trace = hooks.Runtime.turnTrace.MarkCompleted()
		logTurnTraceInfo(hooks.Logger, "gateway turn completed", hooks.SessionID, trace,
			"completed_latency_ms", trace.CompletedLatencyMs(),
			"end_session", true,
			"end_reason", string(reason),
		)
		hooks.Runtime.turnTrace.Clear()
		if hooks.OnEndSession == nil {
			return nil
		}
		return hooks.OnEndSession(trace, reason, message)
	}

	var (
		active session.Snapshot
		err    error
	)
	if hooks.ResolveReturnActive != nil {
		var resolved bool
		active, resolved, err = hooks.ResolveReturnActive()
		if err != nil {
			return err
		}
		if !resolved {
			active, err = hooks.Runtime.session.SetState(session.StateActive)
			if err != nil {
				return err
			}
		}
	} else {
		active, err = hooks.Runtime.session.SetState(session.StateActive)
		if err != nil {
			return err
		}
	}
	if err := applyReadDeadline(hooks.Runtime, active, hooks.Profile); err != nil {
		return err
	}
	trace = hooks.Runtime.turnTrace.MarkActive()
	trace = hooks.Runtime.turnTrace.MarkCompleted()
	logTurnTraceInfo(hooks.Logger, "gateway turn completed", hooks.SessionID, trace,
		"active_return_latency_ms", trace.ActiveReturnLatencyMs(),
		"completed_latency_ms", trace.CompletedLatencyMs(),
		"end_session", false,
	)
	hooks.Runtime.turnTrace.Clear()
	if hooks.OnReturnActive == nil {
		return nil
	}
	if err := hooks.OnReturnActive(trace, active); err != nil {
		logTurnTraceError(hooks.Logger, "gateway active update write failed", hooks.SessionID, trace, err,
			"remote_addr", hooks.Runtime.remoteAddr,
			"ws_stage", "session_update_active",
		)
		return err
	}
	logTurnTraceInfo(hooks.Logger, "gateway active update sent", hooks.SessionID, trace,
		"remote_addr", hooks.Runtime.remoteAddr,
	)
	return nil
}

func interruptSpeakingFlow(runtime *connectionRuntime, profile RealtimeProfile, logger *slog.Logger, onInterrupted func() error, onReturnActive func(turnTrace, session.Snapshot) error) error {
	return interruptSpeakingFlowWithOptions(runtime, profile, logger, interruptSpeakingOptions{
		ClearPreview: true,
		ClearPending: true,
	}, onInterrupted, onReturnActive)
}

func interruptSpeakingFlowWithOptions(runtime *connectionRuntime, profile RealtimeProfile, logger *slog.Logger, opts interruptSpeakingOptions, onInterrupted func() error, onReturnActive func(turnTrace, session.Snapshot) error) error {
	snapshot := runtime.session.Snapshot()
	if snapshot.State != session.StateSpeaking && runtime.output == nil {
		return nil
	}
	var interrupted bool
	switch {
	case opts.Decision != nil:
		interrupted = runtime.interruptOutputWithDecision(*opts.Decision, 100*time.Millisecond)
	case opts.Policy != "":
		interrupted = runtime.interruptOutputWithPolicy(opts.Policy, opts.Reason, 100*time.Millisecond)
	default:
		interrupted = runtime.interruptOutput(100 * time.Millisecond)
	}
	if interrupted && onInterrupted != nil {
		if err := onInterrupted(); err != nil {
			return err
		}
	}
	if snapshot.SessionID == "" || snapshot.State != session.StateSpeaking {
		return nil
	}
	active, err := runtime.session.SetOutputState(session.OutputStateIdle)
	if err != nil {
		return err
	}
	trace := runtime.turnTrace.MarkInterrupted()
	trace = runtime.turnTrace.MarkActive()
	logTurnTraceInfo(logger, "gateway turn interrupted", active.SessionID, trace,
		"active_return_latency_ms", trace.ActiveReturnLatencyMs(),
	)
	if opts.ClearPreview {
		runtime.clearInputPreview()
	}
	if opts.ClearPending {
		runtime.resetPendingBargeInAudio()
	}
	if err := applyReadDeadline(runtime, active, profile); err != nil {
		return err
	}
	if onReturnActive == nil {
		return nil
	}
	if err := onReturnActive(trace, active); err != nil {
		logTurnTraceError(logger, "gateway barge-in active update write failed", active.SessionID, trace, err,
			"remote_addr", runtime.remoteAddr,
			"ws_stage", "session_update_active_barge_in",
		)
		return err
	}
	logTurnTraceInfo(logger, "gateway barge-in active update sent", active.SessionID, trace,
		"remote_addr", runtime.remoteAddr,
	)
	return nil
}

func audioStreamForTurnResponse(response voice.TurnResponse) voice.AudioStream {
	if response.AudioStream != nil {
		return response.AudioStream
	}
	if len(response.AudioChunks) > 0 {
		return voice.NewStaticAudioStream(response.AudioChunks)
	}
	return nil
}

func plannedPlaybackDurationForResponse(response voice.TurnResponse, chunkDuration time.Duration) time.Duration {
	if chunkDuration <= 0 {
		return 0
	}
	if len(response.AudioChunks) > 0 {
		return time.Duration(len(response.AudioChunks)) * chunkDuration
	}
	return plannedPlaybackDurationForAudioStream(response.AudioStream, chunkDuration)
}

func plannedPlaybackDurationForAudioStream(audioStream voice.AudioStream, chunkDuration time.Duration) time.Duration {
	if chunkDuration <= 0 || audioStream == nil {
		return 0
	}
	if aware, ok := audioStream.(playbackDurationAware); ok {
		return aware.PlaybackDuration(chunkDuration)
	}
	return 0
}
