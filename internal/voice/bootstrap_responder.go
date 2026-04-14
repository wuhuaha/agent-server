package voice

import (
	"context"
	"strings"

	"agent-server/internal/agent"
)

type BootstrapResponder struct {
	OutputCodec                   string
	OutputSampleRate              int
	OutputChannels                int
	Executor                      agent.TurnExecutor
	Synthesizer                   Synthesizer
	MemoryStore                   agent.MemoryStore
	SpeechPlannerEnabled          bool
	SpeechPlannerMinChunkRunes    int
	SpeechPlannerTargetChunkRunes int
}

func NewBootstrapResponder(outputCodec string, outputSampleRate, outputChannels int) BootstrapResponder {
	return BootstrapResponder{
		OutputCodec:                   outputCodec,
		OutputSampleRate:              outputSampleRate,
		OutputChannels:                outputChannels,
		SpeechPlannerEnabled:          true,
		SpeechPlannerMinChunkRunes:    defaultSpeechPlannerMinChunkRunes,
		SpeechPlannerTargetChunkRunes: defaultSpeechPlannerTargetChunkRunes,
	}
}

func (r BootstrapResponder) Respond(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	return collectTurnResponse(ctx, req, r.RespondStream)
}

func (r BootstrapResponder) RespondStream(ctx context.Context, req TurnRequest, sink ResponseDeltaSink) (TurnResponse, error) {
	userText := strings.TrimSpace(req.Text)
	planner := newPlannedSpeechSynthesis(ctx, r.Synthesizer, SynthesisRequest{
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		TraceID:   req.TraceID,
		DeviceID:  req.DeviceID,
		UserText:  userText,
	}, r.speechPlannerConfig())

	emitSink := sink
	if planner != nil {
		emitSink = ResponseDeltaSinkFunc(func(ctx context.Context, delta ResponseDelta) error {
			if err := emitResponseDelta(ctx, sink, delta); err != nil {
				return err
			}
			planner.ObserveDelta(delta)
			return nil
		})
	}

	turn, err := executeTurnStream(ctx, r.Executor, req, req.Text, emitSink)
	if err != nil {
		if planner != nil {
			planner.Close()
		}
		return TurnResponse{}, err
	}

	audioChunks, audioStream := r.audioOutput(ctx, req, userText, turn.Text)
	if planner != nil {
		if plannedStream := planner.Finalize(turn.Text); plannedStream != nil {
			audioChunks = nil
			audioStream = plannedStream
		}
	}
	response := TurnResponse{
		InputText:   userText,
		Text:        turn.Text,
		AudioChunks: audioChunks,
		AudioStream: audioStream,
		EndSession:  turn.EndSession,
		EndReason:   turn.EndReason,
		EndMessage:  turn.EndMessage,
	}
	if sink == nil {
		response.Deltas = responseDeltasFromTurn(turn)
	}
	return response, nil
}

func (r BootstrapResponder) WithTurnExecutor(executor agent.TurnExecutor) BootstrapResponder {
	r.Executor = executor
	return r
}

func (r BootstrapResponder) WithSynthesizer(s Synthesizer) BootstrapResponder {
	r.Synthesizer = s
	return r
}

func (r BootstrapResponder) WithMemoryStore(store agent.MemoryStore) BootstrapResponder {
	r.MemoryStore = store
	return r
}

func (r BootstrapResponder) WithSpeechPlannerConfig(cfg SpeechPlannerConfig) BootstrapResponder {
	cfg = NormalizeSpeechPlannerConfig(cfg)
	r.SpeechPlannerEnabled = cfg.Enabled
	r.SpeechPlannerMinChunkRunes = cfg.MinChunkRunes
	r.SpeechPlannerTargetChunkRunes = cfg.TargetChunkRunes
	return r
}

func (r BootstrapResponder) NewSessionOrchestrator() *SessionOrchestrator {
	return NewSessionOrchestrator(r.MemoryStore)
}

func (r BootstrapResponder) speechPlannerConfig() SpeechPlannerConfig {
	return NormalizeSpeechPlannerConfig(SpeechPlannerConfig{
		Enabled:          r.SpeechPlannerEnabled,
		MinChunkRunes:    r.SpeechPlannerMinChunkRunes,
		TargetChunkRunes: r.SpeechPlannerTargetChunkRunes,
	})
}

func bootstrapAudio(codec string, sampleRate, channels int) [][]byte {
	if codec != "pcm16le" || sampleRate <= 0 || channels <= 0 {
		return nil
	}

	samplesPer20ms := sampleRate / 50
	if samplesPer20ms <= 0 {
		samplesPer20ms = 320
	}

	frameSize := samplesPer20ms * channels * 2
	if frameSize <= 0 {
		return nil
	}

	return [][]byte{
		make([]byte, frameSize),
		make([]byte, frameSize),
	}
}

func (r BootstrapResponder) audioOutput(ctx context.Context, req TurnRequest, userText, responseText string) ([][]byte, AudioStream) {
	if audioChunks, audioStream := synthesizedAudio(ctx, r.Synthesizer, req, userText, responseText); audioChunks != nil || audioStream != nil {
		return audioChunks, audioStream
	}
	return bootstrapAudio(r.OutputCodec, r.OutputSampleRate, r.OutputChannels), nil
}
