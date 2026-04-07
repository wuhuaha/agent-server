package voice

import (
	"context"
	"strings"

	"agent-server/internal/agent"
)

func executeTurn(ctx context.Context, executor agent.TurnExecutor, req TurnRequest, userText string) (agent.TurnOutput, error) {
	if executor == nil {
		executor = agent.NewBootstrapTurnExecutor()
	}
	return executor.ExecuteTurn(ctx, turnInputFromRequest(req, userText))
}

func executeTurnStream(ctx context.Context, executor agent.TurnExecutor, req TurnRequest, userText string, sink ResponseDeltaSink) (agent.TurnOutput, error) {
	if executor == nil {
		executor = agent.NewBootstrapTurnExecutor()
	}
	input := turnInputFromRequest(req, userText)
	if streamingExecutor, ok := executor.(agent.StreamingTurnExecutor); ok {
		return streamingExecutor.StreamTurn(ctx, input, agent.TurnDeltaSinkFunc(func(ctx context.Context, delta agent.TurnDelta) error {
			return emitResponseDelta(ctx, sink, responseDeltaFromTurnDelta(delta))
		}))
	}

	output, err := executor.ExecuteTurn(ctx, input)
	if err != nil {
		return agent.TurnOutput{}, err
	}
	if err := emitResponseDeltas(ctx, sink, responseDeltasFromTurn(output)); err != nil {
		return agent.TurnOutput{}, err
	}
	return output, nil
}

func turnInputFromRequest(req TurnRequest, userText string) agent.TurnInput {
	return agent.TurnInput{
		SessionID:  req.SessionID,
		DeviceID:   req.DeviceID,
		ClientType: req.ClientType,
		UserText:   userText,
		Audio: agent.AudioInput{
			Present:      hasTurnAudio(req),
			Frames:       req.InputFrames,
			Bytes:        req.AudioBytes,
			Codec:        req.InputCodec,
			SampleRateHz: req.InputSampleRate,
			Channels:     req.InputChannels,
		},
		Metadata: cloneStringMap(req.Metadata),
	}
}

func hasTurnAudio(req TurnRequest) bool {
	return len(req.AudioPCM) > 0 || req.AudioBytes > 0 || req.InputFrames > 0
}

func responseDeltasFromTurn(output agent.TurnOutput) []ResponseDelta {
	if len(output.Deltas) == 0 {
		if strings.TrimSpace(output.Text) == "" {
			return nil
		}
		return []ResponseDelta{{
			Kind: ResponseDeltaKindText,
			Text: output.Text,
		}}
	}

	deltas := make([]ResponseDelta, 0, len(output.Deltas))
	for _, delta := range output.Deltas {
		deltas = append(deltas, responseDeltaFromTurnDelta(delta))
	}
	return deltas
}

func responseDeltaFromTurnDelta(delta agent.TurnDelta) ResponseDelta {
	return ResponseDelta{
		Kind:       ResponseDeltaKind(delta.Kind),
		Text:       delta.Text,
		ToolCallID: delta.ToolCallID,
		ToolName:   delta.ToolName,
		ToolStatus: delta.ToolStatus,
		ToolInput:  delta.ToolInput,
		ToolOutput: delta.ToolOutput,
	}
}

func collectTurnResponse(ctx context.Context, req TurnRequest, streamer func(context.Context, TurnRequest, ResponseDeltaSink) (TurnResponse, error)) (TurnResponse, error) {
	collector := &responseDeltaCollector{}
	response, err := streamer(ctx, req, collector)
	if err != nil {
		return TurnResponse{}, err
	}
	if len(response.Deltas) == 0 {
		response.Deltas = collector.deltas
	}
	if len(response.Deltas) == 0 && strings.TrimSpace(response.Text) != "" {
		response.Deltas = []ResponseDelta{{
			Kind: ResponseDeltaKindText,
			Text: response.Text,
		}}
	}
	return response, nil
}

func emitResponseDelta(ctx context.Context, sink ResponseDeltaSink, delta ResponseDelta) error {
	if sink == nil {
		return nil
	}
	return sink.EmitResponseDelta(ctx, delta)
}

func emitResponseDeltas(ctx context.Context, sink ResponseDeltaSink, deltas []ResponseDelta) error {
	for _, delta := range deltas {
		if err := emitResponseDelta(ctx, sink, delta); err != nil {
			return err
		}
	}
	return nil
}

type responseDeltaCollector struct {
	deltas []ResponseDelta
}

func (c *responseDeltaCollector) EmitResponseDelta(_ context.Context, delta ResponseDelta) error {
	c.deltas = append(c.deltas, delta)
	return nil
}
