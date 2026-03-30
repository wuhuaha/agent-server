package voice

import (
	"context"
	"fmt"
	"strings"
)

type ASRResponder struct {
	Transcriber            Transcriber
	Synthesizer            Synthesizer
	Language               string
	OutputCodec            string
	OutputSampleRate       int
	OutputChannels         int
	EmitPlaceholderAudio   bool
	TextInputResponseStyle string
}

func NewASRResponder(
	transcriber Transcriber,
	language string,
	outputCodec string,
	outputSampleRate, outputChannels int,
	emitPlaceholderAudio bool,
) ASRResponder {
	return ASRResponder{
		Transcriber:          transcriber,
		Language:             strings.TrimSpace(language),
		OutputCodec:          outputCodec,
		OutputSampleRate:     outputSampleRate,
		OutputChannels:       outputChannels,
		EmitPlaceholderAudio: emitPlaceholderAudio,
	}
}

func (r ASRResponder) Respond(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	switch {
	case strings.TrimSpace(req.Text) != "":
		text := fmt.Sprintf("已收到文本输入: %s", strings.TrimSpace(req.Text))
		audioChunks, audioStream := r.audioOutput(ctx, req, req.Text, text)
		return TurnResponse{
			Text:        text,
			AudioChunks: audioChunks,
			AudioStream: audioStream,
		}, nil
	case len(req.AudioPCM) == 0:
		text := "未收到有效音频输入。"
		audioChunks, audioStream := r.audioOutput(ctx, req, req.Text, text)
		return TurnResponse{
			Text:        text,
			AudioChunks: audioChunks,
			AudioStream: audioStream,
		}, nil
	case r.Transcriber == nil:
		return TurnResponse{}, fmt.Errorf("asr transcriber is not configured")
	}

	result, err := r.Transcriber.Transcribe(ctx, TranscriptionRequest{
		SessionID:    req.SessionID,
		DeviceID:     req.DeviceID,
		AudioPCM:     req.AudioPCM,
		Codec:        req.InputCodec,
		SampleRateHz: req.InputSampleRate,
		Channels:     req.InputChannels,
		Language:     r.Language,
	})
	if err != nil {
		return TurnResponse{}, err
	}

	text := strings.TrimSpace(result.Text)
	if text == "" {
		text = "未识别到有效语音。"
	} else {
		text = fmt.Sprintf("识别结果: %s", text)
	}

	audioChunks, audioStream := r.audioOutput(ctx, req, result.Text, text)
	return TurnResponse{
		Text:        text,
		AudioChunks: audioChunks,
		AudioStream: audioStream,
	}, nil
}

func (r ASRResponder) WithSynthesizer(s Synthesizer) ASRResponder {
	r.Synthesizer = s
	return r
}

func (r ASRResponder) audioOutput(ctx context.Context, req TurnRequest, userText, responseText string) ([][]byte, AudioStream) {
	if audioChunks, audioStream := synthesizedAudio(ctx, r.Synthesizer, req, userText, responseText); audioChunks != nil || audioStream != nil {
		return audioChunks, audioStream
	}
	if !r.EmitPlaceholderAudio {
		return nil, nil
	}
	return bootstrapAudio(r.OutputCodec, r.OutputSampleRate, r.OutputChannels), nil
}
