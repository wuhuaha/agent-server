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
	Runtime           *connectionRuntime
	Responder         voice.Responder
	Logger            *slog.Logger
	SessionID         string
	ResponseID        string
	EmitResponseStart func(trace turnTrace, responseID string, modalities []string, response voice.TurnResponse) error
	EmitResponseDelta func(responseID string, delta voice.ResponseDelta) error
}

type turnExecutionResult struct {
	Trace          turnTrace
	Response       voice.TurnResponse
	ResponseID     string
	AggregatedText string
}

func executeTurnResponse(ctx context.Context, request voice.TurnRequest, trace turnTrace, opts turnExecutionOptions) (turnExecutionResult, error) {
	responseID := strings.TrimSpace(opts.ResponseID)
	if responseID == "" {
		responseID = fmt.Sprintf("resp_%d", time.Now().UTC().UnixNano())
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
	if err := emitTurnResponseDeltas(responseID, deltas, opts.EmitResponseDelta); err != nil {
		return turnExecutionResult{}, err
	}

	var collector collectedTurnText
	collector.AddAll(deltas)
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
			if err := emitTurnResponseDelta(responseID, delta, opts.EmitResponseDelta); err != nil {
				cancel()
				return turnExecutionResult{}, err
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
				if err := emitTurnResponseDeltas(responseID, deltas, opts.EmitResponseDelta); err != nil {
					cancel()
					return turnExecutionResult{}, err
				}
				collector.AddAll(deltas)
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
