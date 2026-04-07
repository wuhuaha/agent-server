package voice

import (
	"context"
	"fmt"
	"strings"

	"agent-server/internal/agent"
)

type ASRResponder struct {
	Transcriber            Transcriber
	Executor               agent.TurnExecutor
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
	return collectTurnResponse(ctx, req, r.RespondStream)
}

func (r ASRResponder) RespondStream(ctx context.Context, req TurnRequest, sink ResponseDeltaSink) (TurnResponse, error) {
	switch {
	case strings.TrimSpace(req.Text) != "":
		userText := strings.TrimSpace(req.Text)
		turn, err := executeTurnStream(ctx, r.Executor, req, userText, sink)
		if err != nil {
			return TurnResponse{}, err
		}
		audioChunks, audioStream := r.audioOutput(ctx, req, userText, turn.Text)
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
	case len(req.AudioPCM) == 0:
		return r.singleTextResponse(ctx, req, req.Text, "未收到有效音频输入。", sink)
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

	userText := strings.TrimSpace(result.Text)
	if userText == "" {
		return r.singleTextResponse(ctx, req, req.Text, "未识别到有效语音。", sink)
	}

	req.Metadata = turnMetadataWithTranscription(req.Metadata, result)
	turn, err := executeTurnStream(ctx, r.Executor, req, userText, sink)
	if err != nil {
		return TurnResponse{}, err
	}

	audioChunks, audioStream := r.audioOutput(ctx, req, userText, turn.Text)
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

func (r ASRResponder) singleTextResponse(ctx context.Context, req TurnRequest, userText, text string, sink ResponseDeltaSink) (TurnResponse, error) {
	delta := ResponseDelta{Kind: ResponseDeltaKindText, Text: text}
	if err := emitResponseDelta(ctx, sink, delta); err != nil {
		return TurnResponse{}, err
	}
	audioChunks, audioStream := r.audioOutput(ctx, req, userText, text)
	response := TurnResponse{
		InputText:   strings.TrimSpace(userText),
		Text:        text,
		AudioChunks: audioChunks,
		AudioStream: audioStream,
	}
	if sink == nil {
		response.Deltas = []ResponseDelta{delta}
	}
	return response, nil
}

func (r ASRResponder) WithTurnExecutor(executor agent.TurnExecutor) ASRResponder {
	r.Executor = executor
	return r
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
