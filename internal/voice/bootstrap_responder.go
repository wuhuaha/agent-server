package voice

import (
	"context"
	"strings"

	"agent-server/internal/agent"
)

type BootstrapResponder struct {
	OutputCodec      string
	OutputSampleRate int
	OutputChannels   int
	Executor         agent.TurnExecutor
	Synthesizer      Synthesizer
	MemoryStore      agent.MemoryStore
}

func NewBootstrapResponder(outputCodec string, outputSampleRate, outputChannels int) BootstrapResponder {
	return BootstrapResponder{
		OutputCodec:      outputCodec,
		OutputSampleRate: outputSampleRate,
		OutputChannels:   outputChannels,
	}
}

func (r BootstrapResponder) Respond(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	return collectTurnResponse(ctx, req, r.RespondStream)
}

func (r BootstrapResponder) RespondStream(ctx context.Context, req TurnRequest, sink ResponseDeltaSink) (TurnResponse, error) {
	turn, err := executeTurnStream(ctx, r.Executor, req, req.Text, sink)
	if err != nil {
		return TurnResponse{}, err
	}

	audioChunks, audioStream := r.audioOutput(ctx, req, req.Text, turn.Text)
	response := TurnResponse{
		InputText:   strings.TrimSpace(req.Text),
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

func (r BootstrapResponder) NewSessionOrchestrator() *SessionOrchestrator {
	return NewSessionOrchestrator(r.MemoryStore)
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
