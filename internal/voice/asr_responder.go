package voice

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"agent-server/internal/agent"
)

const (
	defaultASRStreamingChunkMs    = 40
	defaultPreviewPrewarmMinRunes = 4
	defaultPreviewPrewarmTimeout  = 1500 * time.Millisecond
)

type ASRResponder struct {
	Transcriber                   Transcriber
	Executor                      agent.TurnExecutor
	SemanticJudge                 SemanticTurnJudge
	SlotParser                    SemanticSlotParser
	Synthesizer                   Synthesizer
	MemoryStore                   agent.MemoryStore
	Language                      string
	TurnDetectionSilenceMs        int
	TurnDetectionMinAudioMs       int
	TurnDetectionLexicalMode      string
	TurnDetectionIncompleteHoldMs int
	TurnDetectionHintSilenceMs    int
	SemanticJudgeTimeout          time.Duration
	SemanticJudgeMinRunes         int
	SemanticJudgeMinStableFor     time.Duration
	SemanticJudgeRollout          SemanticJudgeRolloutConfig
	SlotParserTimeout             time.Duration
	SlotParserMinRunes            int
	SlotParserMinStableFor        time.Duration
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
	hints := r.transcriptionHints(req.SessionID)
	stream, err := streaming.StartStream(ctx, TranscriptionRequest{
		SessionID:    req.SessionID,
		DeviceID:     req.DeviceID,
		Codec:        req.Codec,
		SampleRateHz: req.SampleRateHz,
		Channels:     req.Channels,
		Language:     firstNonEmpty(req.Language, r.Language),
		Hotwords:     hints.Hotwords,
		HintPhrases:  hints.HintPhrases,
	}, TranscriptionDeltaSinkFunc(func(_ context.Context, delta TranscriptionDelta) error {
		detector.ObserveTranscriptionDelta(time.Now(), delta)
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return &asrInputPreviewSession{
		stream:               stream,
		detector:             &detector,
		prewarmer:            previewTurnPrewarmer(r.Executor),
		semanticJudge:        r.SemanticJudge,
		semanticDecision:     decideSemanticJudgeRollout(req, r.SemanticJudgeRollout, r.SemanticJudge),
		semanticTimeout:      firstPositiveDuration(r.SemanticJudgeTimeout, defaultSemanticJudgeTimeout),
		semanticMinRunes:     maxInt(r.SemanticJudgeMinRunes, defaultSemanticJudgeMinRunes),
		semanticMinStableFor: firstPositiveDuration(r.SemanticJudgeMinStableFor, defaultSemanticJudgeMinStableFor),
		slotParser:           r.SlotParser,
		slotParserTimeout:    firstPositiveDuration(r.SlotParserTimeout, defaultSemanticSlotParserTimeout),
		slotParserMinRunes:   maxInt(r.SlotParserMinRunes, defaultSemanticSlotParserMinRunes),
		slotParserStableFor:  firstPositiveDuration(r.SlotParserMinStableFor, defaultSemanticSlotParserMinStableFor),
		previewRequest:       req,
		minPrewarmRunes:      defaultPreviewPrewarmMinRunes,
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
		SemanticJudgeTimeout:          defaultSemanticJudgeTimeout,
		SemanticJudgeMinRunes:         defaultSemanticJudgeMinRunes,
		SemanticJudgeMinStableFor:     defaultSemanticJudgeMinStableFor,
		SemanticJudgeRollout:          NormalizeSemanticJudgeRolloutConfig(SemanticJudgeRolloutConfig{}),
		SlotParserTimeout:             defaultSemanticSlotParserTimeout,
		SlotParserMinRunes:            defaultSemanticSlotParserMinRunes,
		SlotParserMinStableFor:        defaultSemanticSlotParserMinStableFor,
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

	result, err := r.transcriptionForTurn(ctx, req)
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

func (r ASRResponder) WithSemanticJudge(judge SemanticTurnJudge, timeout time.Duration, minRunes int, minStableFor time.Duration) ASRResponder {
	r.SemanticJudge = judge
	if timeout > 0 {
		r.SemanticJudgeTimeout = timeout
	}
	if minRunes > 0 {
		r.SemanticJudgeMinRunes = minRunes
	}
	if minStableFor > 0 {
		r.SemanticJudgeMinStableFor = minStableFor
	}
	return r
}

func (r ASRResponder) WithSemanticJudgeRollout(cfg SemanticJudgeRolloutConfig) ASRResponder {
	r.SemanticJudgeRollout = NormalizeSemanticJudgeRolloutConfig(cfg)
	return r
}

func (r ASRResponder) WithSlotParser(parser SemanticSlotParser, timeout time.Duration, minRunes int, minStableFor time.Duration) ASRResponder {
	r.SlotParser = parser
	if timeout > 0 {
		r.SlotParserTimeout = timeout
	}
	if minRunes > 0 {
		r.SlotParserMinRunes = minRunes
	}
	if minStableFor > 0 {
		r.SlotParserMinStableFor = minStableFor
	}
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
	hints := r.transcriptionHints(req.SessionID)
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
		Hotwords:     hints.Hotwords,
		HintPhrases:  hints.HintPhrases,
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

func (r ASRResponder) transcriptionHints(sessionID string) TranscriptionHints {
	provider, ok := r.SlotParser.(TranscriptionHintProvider)
	if !ok || strings.TrimSpace(sessionID) == "" {
		return TranscriptionHints{}
	}
	return provider.TranscriptionHintsForSession(sessionID)
}

func (r ASRResponder) transcriptionForTurn(ctx context.Context, req TurnRequest) (TranscriptionResult, error) {
	if req.PreviewTranscription != nil && strings.TrimSpace(req.PreviewTranscription.Text) != "" {
		return *req.PreviewTranscription, nil
	}
	return r.transcribeAudio(ctx, req)
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
	stream               StreamingTranscriptionSession
	detector             *SilenceTurnDetector
	prewarmer            agent.TurnPrewarmer
	semanticJudge        SemanticTurnJudge
	semanticDecision     semanticJudgeRolloutDecision
	semanticState        previewSemanticJudgeState
	semanticTimeout      time.Duration
	semanticMinRunes     int
	semanticMinStableFor time.Duration
	slotParser           SemanticSlotParser
	slotState            previewSemanticSlotParserState
	slotParserTimeout    time.Duration
	slotParserMinRunes   int
	slotParserStableFor  time.Duration
	previewRequest       InputPreviewRequest
	minPrewarmRunes      int
	lastPrewarmText      string
	finishOnce           sync.Once
	finishResult         TranscriptionResult
	finishErr            error
}

func (s *asrInputPreviewSession) PushAudio(ctx context.Context, chunk []byte) (InputPreview, error) {
	return s.PushAudioProgressively(ctx, chunk, nil)
}

func (s *asrInputPreviewSession) PushAudioProgressively(ctx context.Context, chunk []byte, emit func(InputPreview)) (InputPreview, error) {
	if s == nil {
		return InputPreview{}, nil
	}
	chunks := splitPCMForStreaming(chunk, s.previewRequest.SampleRateHz, s.previewRequest.Channels, defaultASRStreamingChunkMs)
	if len(chunks) == 0 {
		return InputPreview{}, nil
	}

	var last InputPreview
	for _, piece := range chunks {
		snapshot, err := s.pushPreviewChunk(ctx, piece)
		if err != nil {
			return InputPreview{}, err
		}
		last = snapshot
		if emit != nil {
			emit(snapshot)
		}
	}
	return last, nil
}

func (s *asrInputPreviewSession) Poll(now time.Time) InputPreview {
	if s.detector == nil {
		return InputPreview{}
	}
	snapshot := s.detector.Snapshot(now)
	s.maybeLaunchSemanticJudge(snapshot)
	snapshot = s.mergedSemanticSnapshot(snapshot)
	snapshot = s.annotateSemanticJudgeTracing(snapshot)
	s.maybeLaunchSlotParser(snapshot)
	snapshot = s.mergedSlotSnapshot(snapshot)
	s.maybePrewarm(snapshot)
	return snapshot
}

func (s *asrInputPreviewSession) Finish(ctx context.Context) (TranscriptionResult, error) {
	if s.stream == nil {
		return TranscriptionResult{}, nil
	}
	s.finishOnce.Do(func() {
		s.finishResult, s.finishErr = s.stream.Finish(ctx)
	})
	return s.finishResult, s.finishErr
}

func (s *asrInputPreviewSession) Close() error {
	if s.stream == nil {
		return nil
	}
	return s.stream.Close()
}

// pushPreviewChunk 保持“入口大帧 -> preview 小步推进”这件事仍由 voice runtime
// 负责，这样网关既能更早拿到 partial，又不会反过来接管 ASR 的分块策略。
func (s *asrInputPreviewSession) pushPreviewChunk(ctx context.Context, chunk []byte) (InputPreview, error) {
	if s.detector != nil {
		s.detector.ObserveAudio(time.Now(), len(chunk))
	}
	if err := s.stream.PushAudio(ctx, chunk); err != nil {
		return InputPreview{}, err
	}
	if s.detector == nil {
		return InputPreview{}, nil
	}
	snapshot := s.detector.Snapshot(time.Now())
	s.maybeLaunchSemanticJudge(snapshot)
	snapshot = s.mergedSemanticSnapshot(snapshot)
	snapshot = s.annotateSemanticJudgeTracing(snapshot)
	s.maybeLaunchSlotParser(snapshot)
	snapshot = s.mergedSlotSnapshot(snapshot)
	s.maybePrewarm(snapshot)
	return snapshot, nil
}

func (s *asrInputPreviewSession) maybePrewarm(snapshot InputPreview) {
	if s == nil || s.prewarmer == nil {
		return
	}
	if !snapshot.Arbitration.PrewarmAllowed {
		return
	}
	candidate := strings.TrimSpace(snapshot.StablePrefix)
	if candidate == "" && snapshot.CommitSuggested && snapshot.UtteranceComplete {
		candidate = strings.TrimSpace(snapshot.PartialText)
	}
	if candidate == "" || candidate == s.lastPrewarmText {
		return
	}
	if utf8.RuneCountInString(candidate) < s.minPrewarmRunes {
		return
	}
	s.lastPrewarmText = candidate
	// 这里允许在“stable prefix 已成熟、但整句还没最终 complete”时先做轻量预热；
	// 真正复用仍然要求 accepted text 精确匹配，因此这条前推链路是可撤销的。
	input := agent.TurnInput{
		SessionID:  s.previewRequest.SessionID,
		DeviceID:   s.previewRequest.DeviceID,
		ClientType: s.previewRequest.ClientType,
		UserText:   candidate,
		Metadata: map[string]string{
			"voice.preview.prewarm":                    "true",
			"voice.preview.partial_text":               strings.TrimSpace(snapshot.PartialText),
			"voice.preview.stable_prefix":              candidate,
			"voice.preview.utterance_complete":         strconv.FormatBool(snapshot.UtteranceComplete),
			"voice.preview.turn_stage":                 string(snapshot.Arbitration.Stage),
			"voice.preview.candidate_ready":            strconv.FormatBool(snapshot.Arbitration.CandidateReady),
			"voice.preview.draft_ready":                strconv.FormatBool(snapshot.Arbitration.DraftReady),
			"voice.preview.accept_ready":               strconv.FormatBool(snapshot.Arbitration.AcceptReady),
			"voice.preview.stable_for_ms":              strconv.Itoa(snapshot.Arbitration.StableForMs),
			"voice.preview.stability_percent":          strconv.Itoa(int(snapshot.Arbitration.Stability * 100)),
			"voice.preview.base_wait_ms":               strconv.Itoa(snapshot.Arbitration.BaseWaitMs),
			"voice.preview.rule_adjust_ms":             strconv.Itoa(snapshot.Arbitration.RuleAdjustMs),
			"voice.preview.punctuation_adjust_ms":      strconv.Itoa(snapshot.Arbitration.PunctuationAdjustMs),
			"voice.preview.semantic_wait_delta_ms":     strconv.Itoa(snapshot.Arbitration.SemanticWaitDeltaMs),
			"voice.preview.semantic_variant":           strings.TrimSpace(snapshot.Arbitration.SemanticJudgeVariant),
			"voice.preview.semantic_enabled":           strconv.FormatBool(snapshot.Arbitration.SemanticJudgeEnabled),
			"voice.preview.slot_guard_adjust_ms":       strconv.Itoa(snapshot.Arbitration.SlotGuardAdjustMs),
			"voice.preview.effective_wait_ms":          strconv.Itoa(snapshot.Arbitration.EffectiveWaitMs),
			"voice.preview.commit_suggested":           strconv.FormatBool(snapshot.CommitSuggested),
			"voice.preview.endpoint_candidate":         strconv.FormatBool(snapshot.Arbitration.AcceptCandidate),
			"voice.preview.endpoint_reason":            strings.TrimSpace(snapshot.EndpointReason),
			"voice.preview.semantic_ready":             strconv.FormatBool(snapshot.Arbitration.SemanticReady),
			"voice.preview.semantic_complete":          strconv.FormatBool(snapshot.Arbitration.SemanticComplete),
			"voice.preview.semantic_intent":            strings.TrimSpace(snapshot.Arbitration.SemanticIntent),
			"voice.preview.semantic_slot_readiness":    strings.TrimSpace(snapshot.Arbitration.SemanticSlotReadiness),
			"voice.preview.task_family":                strings.TrimSpace(snapshot.Arbitration.TaskFamily),
			"voice.preview.slot_constraint_required":   strconv.FormatBool(snapshot.Arbitration.SlotConstraintRequired),
			"voice.preview.semantic_confidence":        strconv.FormatFloat(snapshot.Arbitration.SemanticConfidence, 'f', 3, 64),
			"voice.preview.slot_ready":                 strconv.FormatBool(snapshot.Arbitration.SlotReady),
			"voice.preview.slot_complete":              strconv.FormatBool(snapshot.Arbitration.SlotComplete),
			"voice.preview.slot_grounded":              strconv.FormatBool(snapshot.Arbitration.SlotGrounded),
			"voice.preview.slot_domain":                strings.TrimSpace(snapshot.Arbitration.SlotDomain),
			"voice.preview.slot_intent":                strings.TrimSpace(snapshot.Arbitration.SlotIntent),
			"voice.preview.slot_status":                strings.TrimSpace(snapshot.Arbitration.SlotStatus),
			"voice.preview.slot_actionability":         strings.TrimSpace(snapshot.Arbitration.SlotActionability),
			"voice.preview.slot_clarify_needed":        strconv.FormatBool(snapshot.Arbitration.SlotClarifyNeeded),
			"voice.preview.slot_canonical_target":      strings.TrimSpace(snapshot.Arbitration.SlotCanonicalTarget),
			"voice.preview.slot_canonical_location":    strings.TrimSpace(snapshot.Arbitration.SlotCanonicalLocation),
			"voice.preview.slot_normalized_value":      strings.TrimSpace(snapshot.Arbitration.SlotNormalizedValue),
			"voice.preview.slot_normalized_unit":       strings.TrimSpace(snapshot.Arbitration.SlotNormalizedValueUnit),
			"voice.preview.slot_risk_level":            strings.TrimSpace(snapshot.Arbitration.SlotRiskLevel),
			"voice.preview.slot_risk_reason":           strings.TrimSpace(snapshot.Arbitration.SlotRiskReason),
			"voice.preview.slot_risk_confirm_required": strconv.FormatBool(snapshot.Arbitration.SlotRiskConfirmRequired),
			"voice.preview.slot_missing":               encodeSpeechStringList(snapshot.Arbitration.SlotMissing),
			"voice.preview.slot_ambiguous":             encodeSpeechStringList(snapshot.Arbitration.SlotAmbiguous),
		},
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultPreviewPrewarmTimeout)
		defer cancel()
		s.prewarmer.PrewarmTurn(ctx, input)
	}()
}

func previewTurnPrewarmer(executor agent.TurnExecutor) agent.TurnPrewarmer {
	prewarmer, _ := executor.(agent.TurnPrewarmer)
	return prewarmer
}

func (s *asrInputPreviewSession) maybeLaunchSemanticJudge(snapshot InputPreview) {
	if s == nil || s.semanticJudge == nil || !s.semanticDecision.Enabled {
		return
	}
	if !shouldJudgeSemantic(snapshot, s.semanticMinRunes, s.semanticMinStableFor) {
		return
	}
	request := s.semanticTurnRequest(snapshot)
	if request == nil {
		return
	}
	key := semanticCandidateKey(request.StablePrefix, request.PartialText)
	if !s.semanticState.shouldLaunch(key) {
		return
	}
	go func(request SemanticTurnRequest, key string) {
		ctx, cancel := context.WithTimeout(context.Background(), firstPositiveDuration(s.semanticTimeout, defaultSemanticJudgeTimeout))
		defer cancel()
		result, err := s.semanticJudge.JudgePreview(ctx, request)
		if err != nil {
			s.semanticState.clearRequest()
			return
		}
		result.CandidateKey = key
		s.semanticState.storeResult(result)
	}(*request, key)
}

func (s *asrInputPreviewSession) semanticTurnRequest(snapshot InputPreview) *SemanticTurnRequest {
	if s == nil {
		return nil
	}
	partialText := strings.TrimSpace(snapshot.PartialText)
	stablePrefix := strings.TrimSpace(snapshot.StablePrefix)
	if partialText == "" && stablePrefix == "" {
		return nil
	}
	return &SemanticTurnRequest{
		SessionID:              s.previewRequest.SessionID,
		DeviceID:               s.previewRequest.DeviceID,
		ClientType:             s.previewRequest.ClientType,
		PartialText:            partialText,
		StablePrefix:           stablePrefix,
		AudioMs:                snapshot.Arbitration.AudioMs,
		Stability:              snapshot.Arbitration.Stability,
		StableForMs:            snapshot.Arbitration.StableForMs,
		TurnStage:              snapshot.Arbitration.Stage,
		EndpointHinted:         snapshot.Arbitration.EndpointHinted,
		TaskFamilyHint:         snapshot.Arbitration.TaskFamily,
		SlotConstraintRequired: snapshot.Arbitration.SlotConstraintRequired,
	}
}

func (s *asrInputPreviewSession) mergedSemanticSnapshot(snapshot InputPreview) InputPreview {
	if s == nil {
		return snapshot
	}
	result, ok := s.semanticState.resultFor(semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText))
	if !ok {
		return snapshot
	}
	return mergeSemanticJudgement(snapshot, result)
}

func (s *asrInputPreviewSession) annotateSemanticJudgeTracing(snapshot InputPreview) InputPreview {
	if s == nil {
		return snapshot
	}
	snapshot.Arbitration.SemanticJudgeVariant = strings.TrimSpace(s.semanticDecision.Variant)
	snapshot.Arbitration.SemanticJudgeEnabled = s.semanticDecision.Enabled
	return snapshot
}

func (s *asrInputPreviewSession) maybeLaunchSlotParser(snapshot InputPreview) {
	if s == nil || s.slotParser == nil {
		return
	}
	if !shouldParseSemanticSlots(snapshot, s.slotParserMinRunes, s.slotParserStableFor) {
		return
	}
	request := s.semanticSlotParseRequest(snapshot)
	if request == nil {
		return
	}
	key := semanticCandidateKey(request.StablePrefix, request.PartialText)
	if !s.slotState.shouldLaunch(key) {
		return
	}
	go func(request SemanticSlotParseRequest, key string) {
		ctx, cancel := context.WithTimeout(context.Background(), firstPositiveDuration(s.slotParserTimeout, defaultSemanticSlotParserTimeout))
		defer cancel()
		result, err := s.slotParser.ParsePreview(ctx, request)
		if err != nil {
			s.slotState.clearRequest()
			return
		}
		result.CandidateKey = key
		s.slotState.storeResult(result)
	}(*request, key)
}

func (s *asrInputPreviewSession) semanticSlotParseRequest(snapshot InputPreview) *SemanticSlotParseRequest {
	if s == nil {
		return nil
	}
	partialText := strings.TrimSpace(snapshot.PartialText)
	stablePrefix := strings.TrimSpace(snapshot.StablePrefix)
	if partialText == "" && stablePrefix == "" {
		return nil
	}
	return &SemanticSlotParseRequest{
		SessionID:      s.previewRequest.SessionID,
		DeviceID:       s.previewRequest.DeviceID,
		ClientType:     s.previewRequest.ClientType,
		PartialText:    partialText,
		StablePrefix:   stablePrefix,
		AudioMs:        snapshot.Arbitration.AudioMs,
		Stability:      snapshot.Arbitration.Stability,
		StableForMs:    snapshot.Arbitration.StableForMs,
		TurnStage:      snapshot.Arbitration.Stage,
		EndpointHinted: snapshot.Arbitration.EndpointHinted,
		SemanticIntent: snapshot.Arbitration.SemanticIntent,
	}
}

func (s *asrInputPreviewSession) mergedSlotSnapshot(snapshot InputPreview) InputPreview {
	if s == nil {
		return snapshot
	}
	result, ok := s.slotState.resultFor(semanticCandidateKey(snapshot.StablePrefix, snapshot.PartialText))
	if !ok {
		return snapshot
	}
	return mergeSemanticSlotParse(snapshot, result)
}
