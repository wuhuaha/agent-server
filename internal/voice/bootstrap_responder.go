package voice

import (
	"context"
	"fmt"
)

type BootstrapResponder struct {
	OutputCodec      string
	OutputSampleRate int
	OutputChannels   int
	Synthesizer      Synthesizer
}

func NewBootstrapResponder(outputCodec string, outputSampleRate, outputChannels int) BootstrapResponder {
	return BootstrapResponder{
		OutputCodec:      outputCodec,
		OutputSampleRate: outputSampleRate,
		OutputChannels:   outputChannels,
	}
}

func (r BootstrapResponder) Respond(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	text := "agent-server realtime bootstrap reply"
	switch {
	case req.Text != "":
		text = fmt.Sprintf("agent-server received text input: %s", req.Text)
	case req.InputFrames > 0:
		text = fmt.Sprintf("agent-server received %d audio frames (%d bytes)", req.InputFrames, req.AudioBytes)
	}

	audioChunks, audioStream := r.audioOutput(ctx, req, req.Text, text)
	return TurnResponse{
		Text:        text,
		AudioChunks: audioChunks,
		AudioStream: audioStream,
	}, nil
}

func (r BootstrapResponder) WithSynthesizer(s Synthesizer) BootstrapResponder {
	r.Synthesizer = s
	return r
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
