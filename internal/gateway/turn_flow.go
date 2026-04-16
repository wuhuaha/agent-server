package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"agent-server/internal/voice"
)

type collectedTurnText struct {
	parts []string
}

func (c *collectedTurnText) Add(delta voice.ResponseDelta) {
	if delta.Kind != voice.ResponseDeltaKindText || strings.TrimSpace(delta.Text) == "" {
		return
	}
	c.parts = append(c.parts, delta.Text)
}

func (c *collectedTurnText) AddAll(deltas []voice.ResponseDelta) {
	for _, delta := range deltas {
		c.Add(delta)
	}
}

func (c *collectedTurnText) Joined() string {
	return strings.TrimSpace(strings.Join(c.parts, ""))
}

type turnExecutionOptions struct {
	Runtime              *connectionRuntime
	Responder            voice.Responder
	Logger               *slog.Logger
	SessionID            string
	ResponseID           string
	EmitResponseStart    func(trace turnTrace, responseID string, modalities []string, response voice.TurnResponse) error
	EmitResponseDelta    func(responseID string, delta voice.ResponseDelta) error
	OnTextDeltaCollected func(trace turnTrace, aggregatedText string)
	StartResponseAudio   func(trace turnTrace, responseID string, audioStart voice.ResponseAudioStart, aggregatedText string, completion *turnOutputOutcomeFuture) error
}

type turnExecutionResult struct {
	Trace          turnTrace
	Response       voice.TurnResponse
	ResponseID     string
	AggregatedText string
}

type responseAudioHintProvider interface {
	MayStreamAudioResponse() bool
}

func executeTurnResponse(ctx context.Context, request voice.TurnRequest, trace turnTrace, opts turnExecutionOptions) (turnExecutionResult, error) {
	responseID := strings.TrimSpace(opts.ResponseID)
	if responseID == "" {
		responseID = fmt.Sprintf("resp_%d", time.Now().UTC().UnixNano())
	}
	if orchestratingResponder, ok := opts.Responder.(voice.OrchestratingResponder); ok {
		return executeOrchestratedTurnResponse(ctx, request, trace, responseID, opts, orchestratingResponder)
	}
	if streamingResponder, ok := opts.Responder.(voice.StreamingResponder); ok {
		return executeStreamingTurnResponse(ctx, request, trace, responseID, opts, streamingResponder)
	}

	response, err := opts.Responder.Respond(ctx, request)
	if err != nil {
		return turnExecutionResult{}, err
	}

	deltas := responseDeltasForEmission(response, true)
	modalities := responseModalities(deltas, response)
	trace = markTurnResponseStart(opts.Runtime, opts.Logger, opts.SessionID, trace, responseID, modalities, response)
	if opts.EmitResponseStart != nil {
		if err := opts.EmitResponseStart(trace, responseID, modalities, response); err != nil {
			return turnExecutionResult{}, err
		}
	}
	for _, delta := range deltas {
		if delta.Kind == voice.ResponseDeltaKindText {
			markTurnFirstTextDelta(opts.Runtime, opts.Logger, opts.SessionID, delta.Text)
		}
		if err := emitTurnResponseDelta(responseID, delta, opts.EmitResponseDelta); err != nil {
			return turnExecutionResult{}, err
		}
	}

	var collector collectedTurnText
	collector.AddAll(deltas)
	if opts.OnTextDeltaCollected != nil && strings.TrimSpace(collector.Joined()) != "" {
		opts.OnTextDeltaCollected(trace, collector.Joined())
	}
	return turnExecutionResult{
		Trace:          trace,
		Response:       response,
		ResponseID:     responseID,
		AggregatedText: collector.Joined(),
	}, nil
}

func executeOrchestratedTurnResponse(
	ctx context.Context,
	request voice.TurnRequest,
	trace turnTrace,
	responseID string,
	opts turnExecutionOptions,
	orchestratingResponder voice.OrchestratingResponder,
) (turnExecutionResult, error) {
	streamCtx, cancel := context.WithCancel(ctx)

	deltaCh := make(chan voice.ResponseDelta, 8)
	future, err := orchestratingResponder.RespondOrchestrated(streamCtx, request, voice.ResponseDeltaSinkFunc(func(ctx context.Context, delta voice.ResponseDelta) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case deltaCh <- delta:
			return nil
		}
	}))
	if err != nil {
		cancel()
		return turnExecutionResult{}, err
	}

	type streamedResponseResult struct {
		response voice.TurnResponse
		err      error
	}
	type audioStartResult struct {
		start voice.ResponseAudioStart
		ok    bool
		err   error
	}

	responseCh := make(chan streamedResponseResult, 1)
	go func() {
		response, err := future.Wait(streamCtx)
		responseCh <- streamedResponseResult{response: response, err: err}
		close(deltaCh)
	}()

	var audioCh <-chan audioStartResult
	if opts.StartResponseAudio != nil {
		startCh := make(chan audioStartResult, 1)
		audioCh = startCh
		go func() {
			start, ok, err := future.WaitAudioStart(streamCtx)
			startCh <- audioStartResult{start: start, ok: ok, err: err}
		}()
	}

	var (
		collector         collectedTurnText
		response          voice.TurnResponse
		responseReady     bool
		sentResponseStart bool
		seenAnyDelta      bool
		completion        *turnOutputOutcomeFuture
	)
	responseChRef := (<-chan streamedResponseResult)(responseCh)
	audioChRef := audioCh

	resolveCompletion := func() {
		if completion == nil || !responseReady {
			return
		}
		completion.Resolve(turnOutputOutcome{
			EndSession: response.EndSession,
			EndReason:  response.EndReason,
			EndMessage: response.EndMessage,
		})
	}

	for responseChRef != nil || deltaCh != nil || audioChRef != nil {
		select {
		case delta, ok := <-deltaCh:
			if !ok {
				deltaCh = nil
				continue
			}
			seenAnyDelta = true
			collector.Add(delta)
			if !sentResponseStart {
				modalities := []string{"text"}
				if opts.StartResponseAudio != nil {
					if hinted, ok := opts.Responder.(responseAudioHintProvider); ok && hinted.MayStreamAudioResponse() {
						modalities = append(modalities, "audio")
					}
				}
				trace = markTurnResponseStart(opts.Runtime, opts.Logger, opts.SessionID, trace, responseID, modalities, response)
				if opts.EmitResponseStart != nil {
					if err := opts.EmitResponseStart(trace, responseID, modalities, response); err != nil {
						cancel()
						return turnExecutionResult{}, err
					}
				}
				sentResponseStart = true
			}
			if delta.Kind == voice.ResponseDeltaKindText {
				markTurnFirstTextDelta(opts.Runtime, opts.Logger, opts.SessionID, delta.Text)
			}
			if err := emitTurnResponseDelta(responseID, delta, opts.EmitResponseDelta); err != nil {
				cancel()
				return turnExecutionResult{}, err
			}
			if delta.Kind == voice.ResponseDeltaKindText && opts.OnTextDeltaCollected != nil {
				opts.OnTextDeltaCollected(trace, collector.Joined())
			}
		case startResult := <-audioChRef:
			audioChRef = nil
			if startResult.err != nil {
				cancel()
				return turnExecutionResult{}, startResult.err
			}
			if !startResult.ok {
				resolveCompletion()
				continue
			}
			if !sentResponseStart {
				modalities := modalitiesForAudioStart(startResult.start, seenAnyDelta)
				trace = markTurnResponseStart(opts.Runtime, opts.Logger, opts.SessionID, trace, responseID, modalities, response)
				if opts.EmitResponseStart != nil {
					if err := opts.EmitResponseStart(trace, responseID, modalities, response); err != nil {
						cancel()
						return turnExecutionResult{}, err
					}
				}
				sentResponseStart = true
			}
			start := wrapResponseAudioStart(startResult.start, cancel)
			completion = newTurnOutputOutcomeFuture()
			if err := opts.StartResponseAudio(trace, responseID, start, aggregatedTextForAudioStart(collector, startResult.start), completion); err != nil {
				cancel()
				return turnExecutionResult{}, err
			}
			response = markResponseAudioTransferred(response)
			resolveCompletion()
		case result := <-responseChRef:
			responseChRef = nil
			if result.err != nil {
				cancel()
				return turnExecutionResult{}, result.err
			}
			response = result.response
			responseReady = true
			if completion != nil {
				response = markResponseAudioTransferred(response)
			}
			resolveCompletion()
			if !sentResponseStart {
				deltas := responseDeltasForEmission(response, true)
				modalities := responseModalities(deltas, response)
				trace = markTurnResponseStart(opts.Runtime, opts.Logger, opts.SessionID, trace, responseID, modalities, response)
				if opts.EmitResponseStart != nil {
					if err := opts.EmitResponseStart(trace, responseID, modalities, response); err != nil {
						cancel()
						return turnExecutionResult{}, err
					}
				}
				for _, delta := range deltas {
					if delta.Kind == voice.ResponseDeltaKindText {
						markTurnFirstTextDelta(opts.Runtime, opts.Logger, opts.SessionID, delta.Text)
					}
					if err := emitTurnResponseDelta(responseID, delta, opts.EmitResponseDelta); err != nil {
						cancel()
						return turnExecutionResult{}, err
					}
				}
				collector.AddAll(deltas)
				if opts.OnTextDeltaCollected != nil && strings.TrimSpace(collector.Joined()) != "" {
					opts.OnTextDeltaCollected(trace, collector.Joined())
				}
				sentResponseStart = true
			}
		}
	}

	if response.AudioStream != nil && !response.AudioStreamTransferred {
		response.AudioStream = &cancelOnCloseAudioStream{
			inner:  response.AudioStream,
			cancel: cancel,
		}
	} else if !response.AudioStreamTransferred {
		cancel()
	}

	return turnExecutionResult{
		Trace:          trace,
		Response:       response,
		ResponseID:     responseID,
		AggregatedText: collector.Joined(),
	}, nil
}

func executeStreamingTurnResponse(
	ctx context.Context,
	request voice.TurnRequest,
	trace turnTrace,
	responseID string,
	opts turnExecutionOptions,
	streamingResponder voice.StreamingResponder,
) (turnExecutionResult, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	// Keep the responder-scoped context alive after RespondStream returns so any
	// returned audio stream can continue synthesizing until playback closes it.
	// We still cancel on early gateway errors below.

	type streamedResponseResult struct {
		response voice.TurnResponse
		err      error
	}

	deltaCh := make(chan voice.ResponseDelta, 8)
	responseCh := make(chan streamedResponseResult, 1)

	go func() {
		defer close(deltaCh)
		response, err := streamingResponder.RespondStream(streamCtx, request, voice.ResponseDeltaSinkFunc(func(ctx context.Context, delta voice.ResponseDelta) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case deltaCh <- delta:
				return nil
			}
		}))
		responseCh <- streamedResponseResult{response: response, err: err}
	}()

	var collector collectedTurnText
	var response voice.TurnResponse
	responseReady := false
	sentResponseStart := false
	var responseChRef <-chan streamedResponseResult = responseCh

	for !responseReady || deltaCh != nil {
		select {
		case delta, ok := <-deltaCh:
			if !ok {
				deltaCh = nil
				continue
			}
			collector.Add(delta)
			if !sentResponseStart {
				modalities := []string{"text"}
				trace = markTurnResponseStart(opts.Runtime, opts.Logger, opts.SessionID, trace, responseID, modalities, response)
				if opts.EmitResponseStart != nil {
					if err := opts.EmitResponseStart(trace, responseID, modalities, response); err != nil {
						cancel()
						return turnExecutionResult{}, err
					}
				}
				sentResponseStart = true
			}
			if delta.Kind == voice.ResponseDeltaKindText {
				markTurnFirstTextDelta(opts.Runtime, opts.Logger, opts.SessionID, delta.Text)
			}
			if err := emitTurnResponseDelta(responseID, delta, opts.EmitResponseDelta); err != nil {
				cancel()
				return turnExecutionResult{}, err
			}
			if delta.Kind == voice.ResponseDeltaKindText && opts.OnTextDeltaCollected != nil {
				opts.OnTextDeltaCollected(trace, collector.Joined())
			}
		case result := <-responseChRef:
			responseChRef = nil
			if result.err != nil {
				cancel()
				return turnExecutionResult{}, result.err
			}
			response = result.response
			responseReady = true
			if !sentResponseStart {
				deltas := responseDeltasForEmission(response, true)
				modalities := responseModalities(deltas, response)
				trace = markTurnResponseStart(opts.Runtime, opts.Logger, opts.SessionID, trace, responseID, modalities, response)
				if opts.EmitResponseStart != nil {
					if err := opts.EmitResponseStart(trace, responseID, modalities, response); err != nil {
						cancel()
						return turnExecutionResult{}, err
					}
				}
				for _, delta := range deltas {
					if delta.Kind == voice.ResponseDeltaKindText {
						markTurnFirstTextDelta(opts.Runtime, opts.Logger, opts.SessionID, delta.Text)
					}
					if err := emitTurnResponseDelta(responseID, delta, opts.EmitResponseDelta); err != nil {
						cancel()
						return turnExecutionResult{}, err
					}
				}
				collector.AddAll(deltas)
				if opts.OnTextDeltaCollected != nil && strings.TrimSpace(collector.Joined()) != "" {
					opts.OnTextDeltaCollected(trace, collector.Joined())
				}
				sentResponseStart = true
			}
		}
	}

	if response.AudioStream != nil {
		response.AudioStream = &cancelOnCloseAudioStream{
			inner:  response.AudioStream,
			cancel: cancel,
		}
	} else {
		cancel()
	}

	return turnExecutionResult{
		Trace:          trace,
		Response:       response,
		ResponseID:     responseID,
		AggregatedText: collector.Joined(),
	}, nil
}

func markTurnResponseStart(runtime *connectionRuntime, logger *slog.Logger, sessionID string, trace turnTrace, responseID string, modalities []string, response voice.TurnResponse) turnTrace {
	trace = runtime.turnTrace.MarkResponseStart()
	logTurnTraceInfo(logger, "gateway turn response started", sessionID, trace,
		"response_id", responseID,
		"response_start_latency_ms", trace.ResponseStartLatencyMs(),
		"modalities", strings.Join(modalities, ","),
		"has_audio", response.AudioStream != nil || len(response.AudioChunks) > 0,
	)
	return trace
}

func emitTurnResponseDeltas(responseID string, deltas []voice.ResponseDelta, emit func(responseID string, delta voice.ResponseDelta) error) error {
	for _, delta := range deltas {
		if err := emitTurnResponseDelta(responseID, delta, emit); err != nil {
			return err
		}
	}
	return nil
}

func emitTurnResponseDelta(responseID string, delta voice.ResponseDelta, emit func(responseID string, delta voice.ResponseDelta) error) error {
	if emit == nil {
		return nil
	}
	return emit(responseID, delta)
}

func wrapResponseAudioStart(start voice.ResponseAudioStart, cancel context.CancelFunc) voice.ResponseAudioStart {
	if start.Stream != nil {
		start.Stream = &cancelOnCloseAudioStream{
			inner:  start.Stream,
			cancel: cancel,
		}
	}
	return start
}

func markResponseAudioTransferred(response voice.TurnResponse) voice.TurnResponse {
	if response.AudioStream == nil && len(response.AudioChunks) == 0 {
		return response
	}
	response.AudioStream = nil
	response.AudioChunks = nil
	response.AudioStreamTransferred = true
	return response
}

type cancelOnCloseAudioStream struct {
	inner     voice.AudioStream
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func (s *cancelOnCloseAudioStream) Next(ctx context.Context) ([]byte, error) {
	chunk, err := s.inner.Next(ctx)
	if err != nil {
		s.release()
	}
	return chunk, err
}

func (s *cancelOnCloseAudioStream) Close() error {
	err := s.inner.Close()
	s.release()
	return err
}

func (s *cancelOnCloseAudioStream) release() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
}

func (s *cancelOnCloseAudioStream) NextSegment(ctx context.Context) (voice.PlaybackSegment, bool, error) {
	segmented, ok := s.inner.(voice.SegmentedAudioStream)
	if !ok {
		return voice.PlaybackSegment{}, false, nil
	}
	return segmented.NextSegment(ctx)
}

func modalitiesForAudioStart(start voice.ResponseAudioStart, seenAnyDelta bool) []string {
	modalities := []string{"audio"}
	if seenAnyDelta || strings.TrimSpace(start.Text) != "" {
		modalities = append([]string{"text"}, modalities...)
	}
	return modalities
}

func aggregatedTextForAudioStart(collector collectedTurnText, start voice.ResponseAudioStart) string {
	if joined := collector.Joined(); strings.TrimSpace(joined) != "" {
		return joined
	}
	return strings.TrimSpace(start.Text)
}
