package app

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"agent-server/internal/control"
	"agent-server/internal/gateway"
	"agent-server/internal/voice"
)

func NewServer(cfg Config, logger *slog.Logger) *http.Server {
	cfg = withRealtimeDefaults(cfg)
	synthesizer := buildSynthesizer(cfg, logger)
	responder := buildResponder(cfg, logger, synthesizer)

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

	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func buildResponder(cfg Config, logger *slog.Logger, synthesizer voice.Synthesizer) voice.Responder {
	bootstrap := voice.NewBootstrapResponder(
		cfg.Realtime.OutputCodec,
		cfg.Realtime.OutputSampleRate,
		cfg.Realtime.OutputChannels,
	).WithSynthesizer(synthesizer)

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
		).WithSynthesizer(synthesizer)
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
