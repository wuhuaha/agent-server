package voice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agent-server/internal/agent"
)

const defaultASRStreamingChunkMs = 40

type ASRResponder struct {
	Transcriber                   Transcriber
	Executor                      agent.TurnExecutor
	Synthesizer                   Synthesizer
	MemoryStore                   agent.MemoryStore
	Language                      string
	TurnDetectionSilenceMs        int
	TurnDetectionMinAudioMs       int
	TurnDetectionLexicalMode      string
	TurnDetectionIncompleteHoldMs int
	TurnDetectionHintSilenceMs    int
	OutputCodec                   string
	OutputSampleRate              int
	OutputChannels                int
	EmitPlaceholderAudio          bool
	TextInputResponseStyle        string
	SpeechPlannerEnabled          bool
	SpeechPlannerMinChunkRunes    int
	SpeechPlannerTargetChunkRunes int
}

func (r ASRResponder) StartInputPreview(ctx context.Context, req InputPreviewRequest) (InputPreviewSession, error) {
	if r.Transcriber == nil {
		return nil, fmt.Errorf("asr transcriber is not configured")
	}
	streaming, ok := r.Transcriber.(StreamingTranscriber)
	if !ok {
		return nil, fmt.Errorf("asr transcriber does not support streaming preview")
	}
	detector := NewSilenceTurnDetector(
		r.turnDetectionConfig(),
		req.SampleRateHz,
		req.Channels,
	)
	stream, err := streaming.StartStream(ctx, TranscriptionRequest{
		SessionID:    req.SessionID,
		DeviceID:     req.DeviceID,
		Codec:        req.Codec,
		SampleRateHz: req.SampleRateHz,
		Channels:     req.Channels,
		Language:     firstNonEmpty(req.Language, r.Language),
	}, TranscriptionDeltaSinkFunc(func(_ context.Context, delta TranscriptionDelta) error {
		detector.ObserveTranscriptionDelta(time.Now(), delta)
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return &asrInputPreviewSession{
		stream:   stream,
		detector: &detector,
	}, nil
}

func NewASRResponder(
	transcriber Transcriber,
	language string,
	outputCodec string,
	outputSampleRate, outputChannels int,
	emitPlaceholderAudio bool,
) ASRResponder {
	return ASRResponder{
		Transcriber:                   transcriber,
		Language:                      strings.TrimSpace(language),
		OutputCodec:                   outputCodec,
		OutputSampleRate:              outputSampleRate,
		OutputChannels:                outputChannels,
		EmitPlaceholderAudio:          emitPlaceholderAudio,
		SpeechPlannerEnabled:          true,
		SpeechPlannerMinChunkRunes:    defaultSpeechPlannerMinChunkRunes,
		SpeechPlannerTargetChunkRunes: defaultSpeechPlannerTargetChunkRunes,
	}
}

func (r ASRResponder) Respond(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	return collectTurnResponse(ctx, req, r.RespondStream)
}

func (r ASRResponder) RespondOrchestrated(ctx context.Context, req TurnRequest, sink ResponseDeltaSink) (TurnResponseFuture, error) {
	future := newAsyncTurnResponseFuture()
	go func() {
		response, err := r.respondWithOptionalPlanning(ctx, req, sink, future)
		future.Resolve(response, err)
	}()
	return future, nil
}

func (r ASRResponder) RespondStream(ctx context.Context, req TurnRequest, sink ResponseDeltaSink) (TurnResponse, error) {
	return r.respondWithOptionalPlanning(ctx, req, sink, nil)
}

func (r ASRResponder) respondWithOptionalPlanning(ctx context.Context, req TurnRequest, sink ResponseDeltaSink, future *asyncTurnResponseFuture) (TurnResponse, error) {
	switch {
	case strings.TrimSpace(req.Text) != "":
		userText := strings.TrimSpace(req.Text)
		turn, audioChunks, audioStream, err := r.executeTurnWithPlannedSpeech(ctx, req, userText, sink, future)
		if err != nil {
			return TurnResponse{}, err
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
	case len(req.AudioPCM) == 0:
		return r.singleTextResponse(ctx, req, req.Text, "未收到有效音频输入。", sink)
	case r.Transcriber == nil:
		return TurnResponse{}, fmt.Errorf("asr transcriber is not configured")
	}

	result, err := r.transcribeAudio(ctx, req)
	if err != nil {
		return TurnResponse{}, err
	}

	userText := strings.TrimSpace(result.Text)
	if userText == "" {
		return r.singleTextResponse(ctx, req, req.Text, "未识别到有效语音。", sink)
	}

	req.Metadata = turnMetadataWithTranscription(req.Metadata, result)
	turn, audioChunks, audioStream, err := r.executeTurnWithPlannedSpeech(ctx, req, userText, sink, future)
	if err != nil {
		return TurnResponse{}, err
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

func (r ASRResponder) WithSpeechPlannerConfig(cfg SpeechPlannerConfig) ASRResponder {
	cfg = NormalizeSpeechPlannerConfig(cfg)
	r.SpeechPlannerEnabled = cfg.Enabled
	r.SpeechPlannerMinChunkRunes = cfg.MinChunkRunes
	r.SpeechPlannerTargetChunkRunes = cfg.TargetChunkRunes
	return r
}

func (r ASRResponder) WithMemoryStore(store agent.MemoryStore) ASRResponder {
	r.MemoryStore = store
	return r
}

func (r ASRResponder) WithTurnDetectionConfig(cfg SilenceTurnDetectorConfig) ASRResponder {
	r.TurnDetectionMinAudioMs = cfg.MinAudioMs
	r.TurnDetectionSilenceMs = cfg.SilenceMs
	r.TurnDetectionLexicalMode = cfg.LexicalEndpointMode
	r.TurnDetectionIncompleteHoldMs = cfg.IncompleteHoldMs
	r.TurnDetectionHintSilenceMs = cfg.EndpointHintSilenceMs
	return r
}

func (r ASRResponder) WithTurnDetection(minAudioMs, silenceMs int) ASRResponder {
	cfg := r.turnDetectionConfig()
	cfg.MinAudioMs = minAudioMs
	cfg.SilenceMs = silenceMs
	return r.WithTurnDetectionConfig(cfg)
}

func (r ASRResponder) WithTurnDetectionLexicalGuard(mode string, incompleteHoldMs int) ASRResponder {
	cfg := r.turnDetectionConfig()
	cfg.LexicalEndpointMode = mode
	cfg.IncompleteHoldMs = incompleteHoldMs
	return r.WithTurnDetectionConfig(cfg)
}

func (r ASRResponder) NewSessionOrchestrator() *SessionOrchestrator {
	return NewSessionOrchestrator(r.MemoryStore)
}

func (r ASRResponder) MayStreamAudioResponse() bool {
	return r.Synthesizer != nil || r.EmitPlaceholderAudio
}

func (r ASRResponder) speechPlannerConfig() SpeechPlannerConfig {
	return NormalizeSpeechPlannerConfig(SpeechPlannerConfig{
		Enabled:          r.SpeechPlannerEnabled,
		MinChunkRunes:    r.SpeechPlannerMinChunkRunes,
		TargetChunkRunes: r.SpeechPlannerTargetChunkRunes,
	})
}

func (r ASRResponder) executeTurnWithPlannedSpeech(ctx context.Context, req TurnRequest, userText string, sink ResponseDeltaSink, future *asyncTurnResponseFuture) (agent.TurnOutput, [][]byte, AudioStream, error) {
	planner := newPlannedSpeechSynthesis(ctx, r.Synthesizer, SynthesisRequest{
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		TraceID:   req.TraceID,
		DeviceID:  req.DeviceID,
		UserText:  strings.TrimSpace(userText),
	}, r.speechPlannerConfig())

	emitSink := sink
	if planner != nil {
		if future != nil {
			go func() {
				start, ok, err := planner.WaitAudioStart(ctx)
				if err == nil && ok {
					future.PublishAudioStart(start)
				}
			}()
		}
		emitSink = ResponseDeltaSinkFunc(func(ctx context.Context, delta ResponseDelta) error {
			if err := emitResponseDelta(ctx, sink, delta); err != nil {
				return err
			}
			planner.ObserveDelta(delta)
			return nil
		})
	}

	turn, err := executeTurnStream(ctx, r.Executor, req, userText, emitSink)
	if err != nil {
		if planner != nil {
			planner.Close()
		}
		return agent.TurnOutput{}, nil, nil, err
	}

	if planner != nil {
		if plannedStream := planner.Finalize(turn.Text); plannedStream != nil {
			return turn, nil, plannedStream, nil
		}
	}

	audioChunks, audioStream := r.audioOutput(ctx, req, userText, turn.Text)
	return turn, audioChunks, audioStream, nil
}

func (r ASRResponder) transcribeAudio(ctx context.Context, req TurnRequest) (TranscriptionResult, error) {
	transcriptionReq := TranscriptionRequest{
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		TraceID:      req.TraceID,
		DeviceID:     req.DeviceID,
		AudioPCM:     req.AudioPCM,
		Codec:        req.InputCodec,
		SampleRateHz: req.InputSampleRate,
		Channels:     req.InputChannels,
		Language:     r.Language,
	}
	if streaming, ok := r.Transcriber.(StreamingTranscriber); ok {
		stream, err := streaming.StartStream(ctx, transcriptionReq, nil)
		if err == nil && stream != nil {
			defer func() { _ = stream.Close() }()
			for _, chunk := range splitPCMForStreaming(req.AudioPCM, req.InputSampleRate, req.InputChannels, defaultASRStreamingChunkMs) {
				if err := stream.PushAudio(ctx, chunk); err != nil {
					return TranscriptionResult{}, err
				}
			}
			return stream.Finish(ctx)
		}
	}
	return r.Transcriber.Transcribe(ctx, transcriptionReq)
}

func splitPCMForStreaming(audioPCM []byte, sampleRateHz, channels, frameMs int) [][]byte {
	if len(audioPCM) == 0 {
		return nil
	}
	frameBytes := pcmFrameBytes(sampleRateHz, channels, frameMs)
	if frameBytes <= 0 || len(audioPCM) <= frameBytes {
		return [][]byte{audioPCM}
	}
	chunks := make([][]byte, 0, (len(audioPCM)+frameBytes-1)/frameBytes)
	for start := 0; start < len(audioPCM); start += frameBytes {
		end := start + frameBytes
		if end > len(audioPCM) {
			end = len(audioPCM)
		}
		chunks = append(chunks, audioPCM[start:end])
	}
	return chunks
}

func pcmFrameBytes(sampleRateHz, channels, frameMs int) int {
	if sampleRateHz <= 0 || channels <= 0 || frameMs <= 0 {
		return defaultBufferedStreamingChunkBytes
	}
	samplesPerChannel := (sampleRateHz * frameMs) / 1000
	if samplesPerChannel <= 0 {
		return defaultBufferedStreamingChunkBytes
	}
	return samplesPerChannel * channels * 2
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

func (r ASRResponder) turnDetectionConfig() SilenceTurnDetectorConfig {
	return SilenceTurnDetectorConfig{
		MinAudioMs:            r.TurnDetectionMinAudioMs,
		SilenceMs:             r.TurnDetectionSilenceMs,
		LexicalEndpointMode:   r.TurnDetectionLexicalMode,
		IncompleteHoldMs:      r.TurnDetectionIncompleteHoldMs,
		EndpointHintSilenceMs: r.TurnDetectionHintSilenceMs,
	}
}

type asrInputPreviewSession struct {
	stream   StreamingTranscriptionSession
	detector *SilenceTurnDetector
}

func (s *asrInputPreviewSession) PushAudio(ctx context.Context, chunk []byte) (InputPreview, error) {
	if s.detector != nil {
		s.detector.ObserveAudio(time.Now(), len(chunk))
	}
	if err := s.stream.PushAudio(ctx, chunk); err != nil {
		return InputPreview{}, err
	}
	if s.detector == nil {
		return InputPreview{}, nil
	}
	return s.detector.Snapshot(time.Now()), nil
}

func (s *asrInputPreviewSession) Poll(now time.Time) InputPreview {
	if s.detector == nil {
		return InputPreview{}
	}
	return s.detector.Snapshot(now)
}

func (s *asrInputPreviewSession) Close() error {
	if s.stream == nil {
		return nil
	}
	return s.stream.Close()
}
