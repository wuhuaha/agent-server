package app

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"agent-server/internal/agent"
	"agent-server/internal/control"
	"agent-server/internal/gateway"
	"agent-server/internal/voice"
)

func NewServer(cfg Config, logger *slog.Logger) *http.Server {
	cfg = withRealtimeDefaults(cfg)
	turnExecutor := buildTurnExecutor(cfg, logger)
	synthesizer := buildSynthesizer(cfg, logger)
	responder := buildResponder(cfg, logger, turnExecutor, synthesizer)

	mux := http.NewServeMux()
	mux.Handle("/healthz", control.NewHealthHandler(cfg.ServiceName))
	mux.Handle("/v1/info", control.NewInfoHandler(cfg.ServiceName, cfg.Environment, cfg.Version, control.RealtimeProfile{
		ProtocolVersion: cfg.Realtime.ProtocolVersion,
		WSPath:          cfg.Realtime.WSPath,
		Subprotocol:     cfg.Realtime.Subprotocol,
		AuthMode:        cfg.Realtime.AuthMode,
		TurnMode:        cfg.Realtime.TurnMode,
	}))
	mux.Handle("/v1/realtime", gateway.NewRealtimeHandler(gateway.RealtimeProfile{
		WSPath:           cfg.Realtime.WSPath,
		ProtocolVersion:  cfg.Realtime.ProtocolVersion,
		Subprotocol:      cfg.Realtime.Subprotocol,
		VoiceProvider:    cfg.Voice.Provider,
		TTSProvider:      cfg.TTS.Provider,
		AuthMode:         cfg.Realtime.AuthMode,
		TurnMode:         cfg.Realtime.TurnMode,
		IdleTimeoutMs:    cfg.Realtime.IdleTimeoutMs,
		MaxSessionMs:     cfg.Realtime.MaxSessionMs,
		MaxFrameBytes:    cfg.Realtime.MaxFrameBytes,
		InputCodec:       cfg.Realtime.InputCodec,
		InputSampleRate:  cfg.Realtime.InputSampleRate,
		InputChannels:    cfg.Realtime.InputChannels,
		OutputCodec:      cfg.Realtime.OutputCodec,
		OutputSampleRate: cfg.Realtime.OutputSampleRate,
		OutputChannels:   cfg.Realtime.OutputChannels,
		AllowOpus:        cfg.Realtime.AllowOpus,
		AllowTextInput:   cfg.Realtime.AllowTextInput,
		AllowImageInput:  cfg.Realtime.AllowImageInput,
	}))
	mux.Handle(cfg.Realtime.WSPath, gateway.NewRealtimeWSHandler(gateway.RealtimeProfile{
		WSPath:           cfg.Realtime.WSPath,
		ProtocolVersion:  cfg.Realtime.ProtocolVersion,
		Subprotocol:      cfg.Realtime.Subprotocol,
		VoiceProvider:    cfg.Voice.Provider,
		TTSProvider:      cfg.TTS.Provider,
		AuthMode:         cfg.Realtime.AuthMode,
		TurnMode:         cfg.Realtime.TurnMode,
		IdleTimeoutMs:    cfg.Realtime.IdleTimeoutMs,
		MaxSessionMs:     cfg.Realtime.MaxSessionMs,
		MaxFrameBytes:    cfg.Realtime.MaxFrameBytes,
		InputCodec:       cfg.Realtime.InputCodec,
		InputSampleRate:  cfg.Realtime.InputSampleRate,
		InputChannels:    cfg.Realtime.InputChannels,
		OutputCodec:      cfg.Realtime.OutputCodec,
		OutputSampleRate: cfg.Realtime.OutputSampleRate,
		OutputChannels:   cfg.Realtime.OutputChannels,
		AllowOpus:        cfg.Realtime.AllowOpus,
		AllowTextInput:   cfg.Realtime.AllowTextInput,
		AllowImageInput:  cfg.Realtime.AllowImageInput,
	}, responder))
	mux.Handle("/debug/realtime-h5", http.RedirectHandler("/debug/realtime-h5/", http.StatusTemporaryRedirect))
	mux.Handle("/debug/realtime-h5/", http.StripPrefix("/debug/realtime-h5/", control.NewWebH5Handler()))

	if cfg.Xiaozhi.Enabled {
		xiaozhiProfile := buildXiaozhiProfile(cfg)
		mux.Handle(xiaozhiProfile.OTAPath, gateway.NewXiaozhiOTAHandler(xiaozhiProfile))
		mux.Handle(xiaozhiProfile.WSPath, gateway.NewXiaozhiWSHandler(xiaozhiProfile, responder))
	}

	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func buildTurnExecutor(cfg Config, logger *slog.Logger) agent.TurnExecutor {
	memoryStore := buildMemoryStore(cfg.Agent, logger)
	toolRegistry, toolInvoker := buildToolBackend(cfg.Agent, memoryStore, logger)
	bootstrap := agent.NewBootstrapTurnExecutor().
		WithMemoryStore(memoryStore).
		WithToolRegistry(toolRegistry).
		WithToolInvoker(toolInvoker)

	switch strings.ToLower(strings.TrimSpace(cfg.Agent.LLMProvider)) {
	case "", "bootstrap":
		logger.Info("agent turn executor configured", "provider", "bootstrap")
		return bootstrap
	case "deepseek", "deepseek_chat":
		if strings.TrimSpace(cfg.Agent.DeepSeek.APIKey) == "" {
			logger.Warn("deepseek chat provider requested but api key is empty; falling back to bootstrap executor")
			return bootstrap
		}
		logger.Info(
			"agent turn executor configured",
			"provider", "deepseek_chat",
			"model", cfg.Agent.DeepSeek.Model,
			"base_url", cfg.Agent.DeepSeek.BaseURL,
			"persona", cfg.Agent.Persona,
			"execution_mode", cfg.Agent.ExecutionMode,
		)
		return agent.NewLLMTurnExecutor(agent.NewDeepSeekChatModel(agent.DeepSeekChatModelConfig{
			BaseURL:     cfg.Agent.DeepSeek.BaseURL,
			APIKey:      cfg.Agent.DeepSeek.APIKey,
			Model:       cfg.Agent.DeepSeek.Model,
			Temperature: cfg.Agent.DeepSeek.Temperature,
			MaxTokens:   cfg.Agent.DeepSeek.MaxTokens,
			Timeout:     time.Duration(cfg.Agent.LLMTimeoutMs) * time.Millisecond,
		})).
			WithMemoryStore(memoryStore).
			WithToolRegistry(toolRegistry).
			WithToolInvoker(toolInvoker).
			WithPersona(cfg.Agent.Persona).
			WithExecutionMode(cfg.Agent.ExecutionMode).
			WithAssistantName(cfg.Agent.AssistantName).
			WithSystemPrompt(cfg.Agent.LLMSystemPrompt)
	default:
		logger.Warn("unknown agent llm provider, falling back to bootstrap executor", "provider", cfg.Agent.LLMProvider)
		return bootstrap
	}
}

func buildMemoryStore(cfg AgentConfig, logger *slog.Logger) agent.MemoryStore {
	switch strings.ToLower(strings.TrimSpace(cfg.MemoryProvider)) {
	case "", "in_memory":
		maxTurns := cfg.MemoryMaxTurns
		if maxTurns <= 0 {
			maxTurns = 8
		}
		logger.Info("agent memory store configured", "provider", "in_memory", "max_turns", maxTurns)
		return agent.NewInMemoryMemoryStore(maxTurns)
	case "noop":
		logger.Info("agent memory store configured", "provider", "noop")
		return agent.NewNoopMemoryStore()
	default:
		maxTurns := cfg.MemoryMaxTurns
		if maxTurns <= 0 {
			maxTurns = 8
		}
		logger.Warn("unknown agent memory provider, falling back to in_memory", "provider", cfg.MemoryProvider, "max_turns", maxTurns)
		return agent.NewInMemoryMemoryStore(maxTurns)
	}
}

func buildToolBackend(cfg AgentConfig, memoryStore agent.MemoryStore, logger *slog.Logger) (agent.ToolRegistry, agent.ToolInvoker) {
	switch strings.ToLower(strings.TrimSpace(cfg.ToolProvider)) {
	case "", "builtin":
		logger.Info("agent tool backend configured", "provider", "builtin")
		backend := agent.NewBuiltinToolBackend(memoryStore)
		return backend, backend
	case "noop":
		logger.Info("agent tool backend configured", "provider", "noop")
		return agent.NewNoopToolRegistry(), agent.NewNoopToolInvoker()
	default:
		logger.Warn("unknown agent tool provider, falling back to builtin", "provider", cfg.ToolProvider)
		backend := agent.NewBuiltinToolBackend(memoryStore)
		return backend, backend
	}
}

func buildXiaozhiProfile(cfg Config) gateway.XiaozhiCompatProfile {
	return gateway.XiaozhiCompatProfile{
		Enabled:               cfg.Xiaozhi.Enabled,
		WSPath:                cfg.Xiaozhi.WSPath,
		OTAPath:               cfg.Xiaozhi.OTAPath,
		WelcomeVersion:        cfg.Xiaozhi.WelcomeVersion,
		WelcomeTransport:      cfg.Xiaozhi.WelcomeTransport,
		InputCodec:            cfg.Xiaozhi.InputCodec,
		InputSampleRate:       cfg.Xiaozhi.InputSampleRate,
		InputChannels:         cfg.Xiaozhi.InputChannels,
		InputFrameDurationMs:  cfg.Xiaozhi.InputFrameDurationMs,
		MaxFrameBytes:         cfg.Xiaozhi.MaxFrameBytes,
		IdleTimeoutMs:         cfg.Xiaozhi.IdleTimeoutMs,
		MaxSessionMs:          cfg.Xiaozhi.MaxSessionMs,
		SourceOutputCodec:     cfg.Realtime.OutputCodec,
		SourceOutputRate:      cfg.Realtime.OutputSampleRate,
		SourceOutputChannels:  cfg.Realtime.OutputChannels,
		OutputCodec:           cfg.Xiaozhi.OutputCodec,
		OutputSampleRate:      cfg.Xiaozhi.OutputSampleRate,
		OutputChannels:        cfg.Xiaozhi.OutputChannels,
		OutputFrameDurationMs: cfg.Xiaozhi.OutputFrameDurationMs,
	}
}

func buildResponder(cfg Config, logger *slog.Logger, turnExecutor agent.TurnExecutor, synthesizer voice.Synthesizer) voice.Responder {
	bootstrap := voice.NewBootstrapResponder(
		cfg.Realtime.OutputCodec,
		cfg.Realtime.OutputSampleRate,
		cfg.Realtime.OutputChannels,
	).WithTurnExecutor(turnExecutor).WithSynthesizer(synthesizer)

	switch strings.ToLower(strings.TrimSpace(cfg.Voice.Provider)) {
	case "", "bootstrap":
		return bootstrap
	case "funasr_http":
		transcriber := voice.NewHTTPTranscriber(
			cfg.Voice.ASRURL,
			time.Duration(cfg.Voice.ASRTimeoutMs)*time.Millisecond,
			cfg.Voice.ASRLanguage,
		)
		logger.Info("voice responder configured", "provider", "funasr_http", "asr_url", cfg.Voice.ASRURL)
		return voice.NewASRResponder(
			transcriber,
			cfg.Voice.ASRLanguage,
			cfg.Realtime.OutputCodec,
			cfg.Realtime.OutputSampleRate,
			cfg.Realtime.OutputChannels,
			cfg.Voice.EmitPlaceholderAudio,
		).WithTurnExecutor(turnExecutor).WithSynthesizer(synthesizer)
	case "iflytek_rtasr":
		if strings.TrimSpace(cfg.Voice.IflytekRTASR.AppID) == "" ||
			strings.TrimSpace(cfg.Voice.IflytekRTASR.AccessKeyID) == "" ||
			strings.TrimSpace(cfg.Voice.IflytekRTASR.AccessKeySecret) == "" {
			logger.Warn("iflytek rtasr provider requested but credentials are incomplete; falling back to bootstrap responder")
			return bootstrap
		}
		transcriber := voice.NewIflytekRTASRTranscriber(voice.IflytekRTASRConfig{
			AppID:           cfg.Voice.IflytekRTASR.AppID,
			AccessKeyID:     cfg.Voice.IflytekRTASR.AccessKeyID,
			AccessKeySecret: cfg.Voice.IflytekRTASR.AccessKeySecret,
			Scheme:          cfg.Voice.IflytekRTASR.Scheme,
			Host:            cfg.Voice.IflytekRTASR.Host,
			Port:            cfg.Voice.IflytekRTASR.Port,
			Path:            cfg.Voice.IflytekRTASR.Path,
			AudioEncode:     cfg.Voice.IflytekRTASR.AudioEncode,
			Language:        cfg.Voice.IflytekRTASR.Language,
			SampleRateHz:    cfg.Voice.IflytekRTASR.SampleRateHz,
			Timeout:         time.Duration(cfg.Voice.ASRTimeoutMs) * time.Millisecond,
			FrameBytes:      cfg.Voice.IflytekRTASR.FrameBytes,
			FrameInterval:   time.Duration(cfg.Voice.IflytekRTASR.FrameIntervalMs) * time.Millisecond,
		})
		logger.Info("voice responder configured", "provider", "iflytek_rtasr", "host", cfg.Voice.IflytekRTASR.Host, "scheme", cfg.Voice.IflytekRTASR.Scheme)
		return voice.NewASRResponder(
			transcriber,
			cfg.Voice.IflytekRTASR.Language,
			cfg.Realtime.OutputCodec,
			cfg.Realtime.OutputSampleRate,
			cfg.Realtime.OutputChannels,
			cfg.Voice.EmitPlaceholderAudio,
		).WithTurnExecutor(turnExecutor).WithSynthesizer(synthesizer)
	default:
		logger.Warn("unknown voice provider, falling back to bootstrap responder", "provider", cfg.Voice.Provider)
		return bootstrap
	}
}

func buildSynthesizer(cfg Config, logger *slog.Logger) voice.Synthesizer {
	switch strings.ToLower(strings.TrimSpace(cfg.TTS.Provider)) {
	case "", "none":
		return nil
	case "mimo_v2_tts":
		if strings.TrimSpace(cfg.TTS.MimoAPIKey) == "" {
			logger.Warn("mimo tts provider requested but MIMO_API_KEY is empty; falling back to no tts")
			return nil
		}
		logger.Info("tts synthesizer configured", "provider", "mimo_v2_tts", "voice", cfg.TTS.MimoVoice, "base_url", cfg.TTS.MimoBaseURL)
		synthesizer := voice.NewMimoTTSSynthesizer(
			cfg.TTS.MimoAPIKey,
			cfg.TTS.MimoBaseURL,
			cfg.TTS.MimoModel,
			cfg.TTS.MimoVoice,
			cfg.TTS.MimoStyle,
			time.Duration(cfg.TTS.TimeoutMs)*time.Millisecond,
			cfg.Realtime.OutputCodec,
			cfg.Realtime.OutputSampleRate,
			cfg.Realtime.OutputChannels,
		)
		return voice.LoggingSynthesizer{
			Inner:  synthesizer,
			Logger: logger,
		}
	case "iflytek_tts_ws":
		if strings.TrimSpace(cfg.TTS.Iflytek.AppID) == "" ||
			strings.TrimSpace(cfg.TTS.Iflytek.APIKey) == "" ||
			strings.TrimSpace(cfg.TTS.Iflytek.APISecret) == "" {
			logger.Warn("iflytek tts provider requested but credentials are incomplete; falling back to no tts")
			return nil
		}
		logger.Info("tts synthesizer configured", "provider", "iflytek_tts_ws", "voice", cfg.TTS.Iflytek.Voice, "host", cfg.TTS.Iflytek.Host)
		return voice.LoggingSynthesizer{
			Inner: voice.NewIflytekTTSSynthesizer(voice.IflytekTTSConfig{
				AppID:          cfg.TTS.Iflytek.AppID,
				APIKey:         cfg.TTS.Iflytek.APIKey,
				APISecret:      cfg.TTS.Iflytek.APISecret,
				Scheme:         cfg.TTS.Iflytek.Scheme,
				Host:           cfg.TTS.Iflytek.Host,
				Port:           cfg.TTS.Iflytek.Port,
				Path:           cfg.TTS.Iflytek.Path,
				Voice:          cfg.TTS.Iflytek.Voice,
				AUE:            cfg.TTS.Iflytek.AUE,
				AUF:            cfg.TTS.Iflytek.AUF,
				TextEncoding:   cfg.TTS.Iflytek.TextEncoding,
				Speed:          cfg.TTS.Iflytek.Speed,
				Volume:         cfg.TTS.Iflytek.Volume,
				Pitch:          cfg.TTS.Iflytek.Pitch,
				TargetCodec:    cfg.Realtime.OutputCodec,
				TargetRateHz:   cfg.Realtime.OutputSampleRate,
				TargetChannels: cfg.Realtime.OutputChannels,
				Timeout:        time.Duration(cfg.TTS.TimeoutMs) * time.Millisecond,
			}),
			Logger: logger,
		}
	case "volcengine_tts":
		if strings.TrimSpace(cfg.TTS.Volcengine.AccessToken) == "" || strings.TrimSpace(cfg.TTS.Volcengine.AppID) == "" {
			logger.Warn("volcengine tts provider requested but credentials are incomplete; falling back to no tts")
			return nil
		}
		logger.Info("tts synthesizer configured", "provider", "volcengine_tts", "voice", cfg.TTS.Volcengine.VoiceType, "base_url", cfg.TTS.Volcengine.BaseURL)
		return voice.LoggingSynthesizer{
			Inner: voice.NewVolcengineTTSSynthesizer(voice.VolcengineTTSConfig{
				BaseURL:        cfg.TTS.Volcengine.BaseURL,
				AccessToken:    cfg.TTS.Volcengine.AccessToken,
				AppID:          cfg.TTS.Volcengine.AppID,
				ResourceID:     cfg.TTS.Volcengine.ResourceID,
				VoiceType:      cfg.TTS.Volcengine.VoiceType,
				SpeechRate:     cfg.TTS.Volcengine.SpeechRate,
				LoudnessRate:   cfg.TTS.Volcengine.LoudnessRate,
				Emotion:        cfg.TTS.Volcengine.Emotion,
				EmotionScale:   cfg.TTS.Volcengine.EmotionScale,
				Model:          cfg.TTS.Volcengine.Model,
				TargetCodec:    cfg.Realtime.OutputCodec,
				TargetRateHz:   cfg.Realtime.OutputSampleRate,
				TargetChannels: cfg.Realtime.OutputChannels,
				Timeout:        time.Duration(cfg.TTS.TimeoutMs) * time.Millisecond,
			}),
			Logger: logger,
		}
	default:
		logger.Warn("unknown tts provider; disabling tts", "provider", cfg.TTS.Provider)
		return nil
	}
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr, "elapsed", time.Since(startedAt).String())
	})
}
