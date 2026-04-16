package app

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"agent-server/internal/agent"
	"agent-server/internal/voice"
)

func TestBuildResponderSupportsIflytekRTASR(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := withRealtimeDefaults(Config{
		Voice: VoiceConfig{
			Provider:                       "iflytek_rtasr",
			ServerEndpointMinAudioMs:       640,
			ServerEndpointSilenceMs:        960,
			ServerEndpointLexicalMode:      "off",
			ServerEndpointIncompleteHoldMs: 1500,
			ServerEndpointHintSilenceMs:    220,
			SpeechPlannerEnabled:           true,
			SpeechPlannerMinChunkRunes:     5,
			SpeechPlannerTargetChunkRunes:  18,
			IflytekRTASR: IflytekRTASRProviderConfig{
				AppID:           "app-id",
				AccessKeyID:     "key-id",
				AccessKeySecret: "key-secret",
			},
		},
	})

	responder := buildResponder(cfg, logger, agent.NewBootstrapTurnExecutor(), agent.NewInMemoryMemoryStore(4), nil)
	asrResponder, ok := responder.(voice.ASRResponder)
	if !ok {
		t.Fatalf("expected ASRResponder, got %T", responder)
	}
	loggedTranscriber, ok := asrResponder.Transcriber.(voice.LoggingTranscriber)
	if !ok {
		t.Fatalf("expected LoggingTranscriber, got %T", asrResponder.Transcriber)
	}
	buffered, ok := loggedTranscriber.Inner.(voice.BufferedStreamingTranscriber)
	if !ok {
		t.Fatalf("expected BufferedStreamingTranscriber, got %T", loggedTranscriber.Inner)
	}
	if _, ok := buffered.Inner.(voice.IflytekRTASRTranscriber); !ok {
		t.Fatalf("expected IflytekRTASRTranscriber, got %T", buffered.Inner)
	}
	if asrResponder.TurnDetectionMinAudioMs != 640 {
		t.Fatalf("expected min audio threshold 640ms, got %d", asrResponder.TurnDetectionMinAudioMs)
	}
	if asrResponder.TurnDetectionSilenceMs != 960 {
		t.Fatalf("expected silence threshold 960ms, got %d", asrResponder.TurnDetectionSilenceMs)
	}
	if asrResponder.TurnDetectionLexicalMode != "off" {
		t.Fatalf("expected lexical mode off, got %q", asrResponder.TurnDetectionLexicalMode)
	}
	if asrResponder.TurnDetectionIncompleteHoldMs != 1500 {
		t.Fatalf("expected incomplete hold 1500ms, got %d", asrResponder.TurnDetectionIncompleteHoldMs)
	}
	if asrResponder.TurnDetectionHintSilenceMs != 220 {
		t.Fatalf("expected hint silence 220ms, got %d", asrResponder.TurnDetectionHintSilenceMs)
	}
	if !asrResponder.SpeechPlannerEnabled {
		t.Fatal("expected speech planner to stay enabled")
	}
	if asrResponder.SpeechPlannerMinChunkRunes != 5 {
		t.Fatalf("expected speech planner min chunk runes 5, got %d", asrResponder.SpeechPlannerMinChunkRunes)
	}
	if asrResponder.SpeechPlannerTargetChunkRunes != 18 {
		t.Fatalf("expected speech planner target chunk runes 18, got %d", asrResponder.SpeechPlannerTargetChunkRunes)
	}
}

func TestBuildResponderSupportsFunASRHTTPPreviewThresholds(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := withRealtimeDefaults(Config{
		Voice: VoiceConfig{
			Provider:                       "funasr_http",
			ASRURL:                         "http://127.0.0.1:8091/v1/asr/transcribe",
			ASRLanguage:                    "zh",
			ServerEndpointMinAudioMs:       400,
			ServerEndpointSilenceMs:        700,
			ServerEndpointLexicalMode:      "conservative",
			ServerEndpointIncompleteHoldMs: 900,
			ServerEndpointHintSilenceMs:    180,
			SpeechPlannerEnabled:           true,
			SpeechPlannerMinChunkRunes:     7,
			SpeechPlannerTargetChunkRunes:  20,
			LLMSemanticJudgeEnabled:        true,
			LLMSemanticJudgeLLM: VoiceLLMProviderConfig{
				Provider: "openai_compat",
				BaseURL:  "http://127.0.0.1:8012/v1",
				Model:    "Qwen/Qwen3-1.7B",
			},
			LLMSemanticJudgeTimeoutMs:      180,
			LLMSemanticJudgeMinRunes:       3,
			LLMSemanticJudgeMinStableForMs: 140,
			LLMSlotParserEnabled:           true,
			LLMSlotParserLLM: VoiceLLMProviderConfig{
				Provider: "openai_compat",
				BaseURL:  "http://127.0.0.1:8012/v1",
				Model:    "Qwen/Qwen3-4B-Instruct-2507",
			},
			LLMSlotParserTimeoutMs:      260,
			LLMSlotParserMinRunes:       5,
			LLMSlotParserMinStableForMs: 180,
			EmitPlaceholderAudio:        true,
		},
	})

	responder := buildResponder(cfg, logger, agent.NewBootstrapTurnExecutor(), agent.NewInMemoryMemoryStore(4), nil)
	asrResponder, ok := responder.(voice.ASRResponder)
	if !ok {
		t.Fatalf("expected ASRResponder, got %T", responder)
	}
	if asrResponder.TurnDetectionMinAudioMs != 400 {
		t.Fatalf("expected min audio threshold 400ms, got %d", asrResponder.TurnDetectionMinAudioMs)
	}
	if asrResponder.TurnDetectionSilenceMs != 700 {
		t.Fatalf("expected silence threshold 700ms, got %d", asrResponder.TurnDetectionSilenceMs)
	}
	if asrResponder.TurnDetectionLexicalMode != "conservative" {
		t.Fatalf("expected lexical mode conservative, got %q", asrResponder.TurnDetectionLexicalMode)
	}
	if asrResponder.TurnDetectionIncompleteHoldMs != 900 {
		t.Fatalf("expected incomplete hold 900ms, got %d", asrResponder.TurnDetectionIncompleteHoldMs)
	}
	if asrResponder.TurnDetectionHintSilenceMs != 180 {
		t.Fatalf("expected hint silence 180ms, got %d", asrResponder.TurnDetectionHintSilenceMs)
	}
	if !asrResponder.SpeechPlannerEnabled {
		t.Fatal("expected speech planner to stay enabled")
	}
	if asrResponder.SpeechPlannerMinChunkRunes != 7 {
		t.Fatalf("expected speech planner min chunk runes 7, got %d", asrResponder.SpeechPlannerMinChunkRunes)
	}
	if asrResponder.SpeechPlannerTargetChunkRunes != 20 {
		t.Fatalf("expected speech planner target chunk runes 20, got %d", asrResponder.SpeechPlannerTargetChunkRunes)
	}
	if asrResponder.SemanticJudge == nil {
		t.Fatal("expected llm semantic judge to be configured")
	}
	if asrResponder.SemanticJudgeTimeout != 180*time.Millisecond {
		t.Fatalf("expected semantic judge timeout 180ms, got %s", asrResponder.SemanticJudgeTimeout)
	}
	if asrResponder.SemanticJudgeMinRunes != 3 {
		t.Fatalf("expected semantic judge min runes 3, got %d", asrResponder.SemanticJudgeMinRunes)
	}
	if asrResponder.SemanticJudgeMinStableFor != 140*time.Millisecond {
		t.Fatalf("expected semantic judge min stable_for 140ms, got %s", asrResponder.SemanticJudgeMinStableFor)
	}
	if asrResponder.SlotParser == nil {
		t.Fatal("expected llm slot parser to be configured")
	}
	if asrResponder.SlotParserTimeout != 260*time.Millisecond {
		t.Fatalf("expected slot parser timeout 260ms, got %s", asrResponder.SlotParserTimeout)
	}
	if asrResponder.SlotParserMinRunes != 5 {
		t.Fatalf("expected slot parser min runes 5, got %d", asrResponder.SlotParserMinRunes)
	}
	if asrResponder.SlotParserMinStableFor != 180*time.Millisecond {
		t.Fatalf("expected slot parser min stable_for 180ms, got %s", asrResponder.SlotParserMinStableFor)
	}
}

func TestBuildResponderSupportsBootstrapSpeechPlannerConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := withRealtimeDefaults(Config{
		Voice: VoiceConfig{
			Provider:                      "bootstrap",
			SpeechPlannerEnabled:          false,
			SpeechPlannerMinChunkRunes:    9,
			SpeechPlannerTargetChunkRunes: 27,
		},
	})

	responder := buildResponder(cfg, logger, agent.NewBootstrapTurnExecutor(), agent.NewInMemoryMemoryStore(4), nil)
	bootstrap, ok := responder.(voice.BootstrapResponder)
	if !ok {
		t.Fatalf("expected BootstrapResponder, got %T", responder)
	}
	if bootstrap.SpeechPlannerEnabled {
		t.Fatal("expected bootstrap speech planner to be disabled")
	}
	if bootstrap.SpeechPlannerMinChunkRunes != 9 {
		t.Fatalf("expected bootstrap speech planner min chunk runes 9, got %d", bootstrap.SpeechPlannerMinChunkRunes)
	}
	if bootstrap.SpeechPlannerTargetChunkRunes != 27 {
		t.Fatalf("expected bootstrap speech planner target chunk runes 27, got %d", bootstrap.SpeechPlannerTargetChunkRunes)
	}
}

func TestBuildSynthesizerSupportsStreamingCloudProviders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		name         string
		config       Config
		expectedType any
	}{
		{
			name: "cosyvoice_http",
			config: withRealtimeDefaults(Config{
				TTS: TTSConfig{
					Provider: "cosyvoice_http",
					CosyVoice: CosyVoiceTTSProviderConfig{
						BaseURL:            "http://127.0.0.1:50000",
						Mode:               "sft",
						SpeakerID:          "中文女",
						SourceSampleRateHz: 22050,
					},
				},
			}),
			expectedType: voice.CosyVoiceHTTPSynthesizer{},
		},
		{
			name: "iflytek_tts_ws",
			config: withRealtimeDefaults(Config{
				TTS: TTSConfig{
					Provider: "iflytek_tts_ws",
					Iflytek: IflytekTTSProviderConfig{
						AppID:     "app-id",
						APIKey:    "api-key",
						APISecret: "api-secret",
					},
				},
			}),
			expectedType: voice.IflytekTTSSynthesizer{},
		},
		{
			name: "volcengine_tts",
			config: withRealtimeDefaults(Config{
				TTS: TTSConfig{
					Provider: "volcengine_tts",
					Volcengine: VolcengineTTSProviderConfig{
						AccessToken: "access-token",
						AppID:       "app-id",
					},
				},
			}),
			expectedType: voice.VolcengineTTSSynthesizer{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synthesizer := buildSynthesizer(tc.config, logger)
			logged, ok := synthesizer.(voice.LoggingSynthesizer)
			if !ok {
				t.Fatalf("expected LoggingSynthesizer, got %T", synthesizer)
			}
			switch tc.expectedType.(type) {
			case voice.CosyVoiceHTTPSynthesizer:
				if _, ok := logged.Inner.(voice.CosyVoiceHTTPSynthesizer); !ok {
					t.Fatalf("expected CosyVoiceHTTPSynthesizer, got %T", logged.Inner)
				}
			case voice.IflytekTTSSynthesizer:
				if _, ok := logged.Inner.(voice.IflytekTTSSynthesizer); !ok {
					t.Fatalf("expected IflytekTTSSynthesizer, got %T", logged.Inner)
				}
			case voice.VolcengineTTSSynthesizer:
				if _, ok := logged.Inner.(voice.VolcengineTTSSynthesizer); !ok {
					t.Fatalf("expected VolcengineTTSSynthesizer, got %T", logged.Inner)
				}
			default:
				t.Fatalf("unsupported expected type %T", tc.expectedType)
			}
		})
	}
}
