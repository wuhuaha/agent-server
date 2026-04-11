package app

import (
	"io"
	"log/slog"
	"testing"

	"agent-server/internal/agent"
	"agent-server/internal/voice"
)

func TestBuildResponderSupportsIflytekRTASR(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := withRealtimeDefaults(Config{
		Voice: VoiceConfig{
			Provider:                 "iflytek_rtasr",
			ServerEndpointMinAudioMs: 640,
			ServerEndpointSilenceMs:  960,
			IflytekRTASR: IflytekRTASRProviderConfig{
				AppID:           "app-id",
				AccessKeyID:     "key-id",
				AccessKeySecret: "key-secret",
			},
		},
	})

	responder := buildResponder(cfg, logger, agent.NewBootstrapTurnExecutor(), nil)
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
}

func TestBuildResponderSupportsFunASRHTTPPreviewThresholds(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := withRealtimeDefaults(Config{
		Voice: VoiceConfig{
			Provider:                 "funasr_http",
			ASRURL:                   "http://127.0.0.1:8091/v1/asr/transcribe",
			ASRLanguage:              "zh",
			ServerEndpointMinAudioMs: 400,
			ServerEndpointSilenceMs:  700,
			EmitPlaceholderAudio:     true,
		},
	})

	responder := buildResponder(cfg, logger, agent.NewBootstrapTurnExecutor(), nil)
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
}

func TestBuildSynthesizerSupportsStreamingCloudProviders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		name         string
		config       Config
		expectedType any
	}{
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
