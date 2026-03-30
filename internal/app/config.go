package app

import (
	"os"
	"strconv"
	"strings"
)

// RealtimeConfig contains the bootstrap configuration for the first RTOS wire profile.
type RealtimeConfig struct {
	WSPath           string
	ProtocolVersion  string
	Subprotocol      string
	AuthMode         string
	TurnMode         string
	IdleTimeoutMs    int
	MaxSessionMs     int
	MaxFrameBytes    int
	InputCodec       string
	InputSampleRate  int
	InputChannels    int
	OutputCodec      string
	OutputSampleRate int
	OutputChannels   int
	AllowOpus        bool
	AllowTextInput   bool
	AllowImageInput  bool
}

type VoiceConfig struct {
	Provider             string
	ASRURL               string
	ASRTimeoutMs         int
	ASRLanguage          string
	EmitPlaceholderAudio bool
}

type TTSConfig struct {
	Provider    string
	MimoBaseURL string
	MimoAPIKey  string
	MimoModel   string
	MimoVoice   string
	MimoStyle   string
	TimeoutMs   int
}

// Config contains the startup configuration for the main service process.
type Config struct {
	ListenAddr  string
	ServiceName string
	Environment string
	Version     string
	Realtime    RealtimeConfig
	Voice       VoiceConfig
	TTS         TTSConfig
}

func LoadConfig() Config {
	return withRealtimeDefaults(Config{
		ListenAddr:  getenv("AGENT_SERVER_ADDR", ":8080"),
		ServiceName: getenv("AGENT_SERVER_NAME", "agent-server"),
		Environment: getenv("AGENT_SERVER_ENV", "dev"),
		Version:     getenv("AGENT_SERVER_VERSION", "0.1.0-dev"),
		Realtime: RealtimeConfig{
			WSPath:           getenv("AGENT_SERVER_REALTIME_WS_PATH", "/v1/realtime/ws"),
			ProtocolVersion:  getenv("AGENT_SERVER_REALTIME_PROTOCOL_VERSION", "rtos-ws-v0"),
			Subprotocol:      getenv("AGENT_SERVER_REALTIME_SUBPROTOCOL", "agent-server.realtime.v0"),
			AuthMode:         getenv("AGENT_SERVER_REALTIME_AUTH_MODE", "disabled"),
			TurnMode:         getenv("AGENT_SERVER_REALTIME_TURN_MODE", "client_wakeup_server_vad"),
			IdleTimeoutMs:    getenvInt("AGENT_SERVER_REALTIME_IDLE_TIMEOUT_MS", 15000),
			MaxSessionMs:     getenvInt("AGENT_SERVER_REALTIME_MAX_SESSION_MS", 300000),
			MaxFrameBytes:    getenvInt("AGENT_SERVER_REALTIME_MAX_FRAME_BYTES", 4096),
			InputCodec:       getenv("AGENT_SERVER_REALTIME_INPUT_CODEC", "pcm16le"),
			InputSampleRate:  getenvInt("AGENT_SERVER_REALTIME_INPUT_SAMPLE_RATE", 16000),
			InputChannels:    getenvInt("AGENT_SERVER_REALTIME_INPUT_CHANNELS", 1),
			OutputCodec:      getenv("AGENT_SERVER_REALTIME_OUTPUT_CODEC", "pcm16le"),
			OutputSampleRate: getenvInt("AGENT_SERVER_REALTIME_OUTPUT_SAMPLE_RATE", 16000),
			OutputChannels:   getenvInt("AGENT_SERVER_REALTIME_OUTPUT_CHANNELS", 1),
			AllowOpus:        getenvBool("AGENT_SERVER_REALTIME_ALLOW_OPUS", true),
			AllowTextInput:   getenvBool("AGENT_SERVER_REALTIME_ALLOW_TEXT_INPUT", true),
			AllowImageInput:  getenvBool("AGENT_SERVER_REALTIME_ALLOW_IMAGE_INPUT", false),
		},
		Voice: VoiceConfig{
			Provider:             getenv("AGENT_SERVER_VOICE_PROVIDER", "bootstrap"),
			ASRURL:               getenv("AGENT_SERVER_VOICE_ASR_URL", "http://127.0.0.1:8091/v1/asr/transcribe"),
			ASRTimeoutMs:         getenvInt("AGENT_SERVER_VOICE_ASR_TIMEOUT_MS", 30000),
			ASRLanguage:          getenv("AGENT_SERVER_VOICE_ASR_LANGUAGE", "auto"),
			EmitPlaceholderAudio: getenvBool("AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO", true),
		},
		TTS: TTSConfig{
			Provider:    getenv("AGENT_SERVER_TTS_PROVIDER", "none"),
			MimoBaseURL: getenv("AGENT_SERVER_TTS_MIMO_BASE_URL", "https://api.xiaomimimo.com/v1"),
			MimoAPIKey:  getenv("MIMO_API_KEY", ""),
			MimoModel:   getenv("AGENT_SERVER_TTS_MIMO_MODEL", "mimo-v2-tts"),
			MimoVoice:   getenv("AGENT_SERVER_TTS_MIMO_VOICE", "mimo_default"),
			MimoStyle:   getenv("AGENT_SERVER_TTS_MIMO_STYLE", ""),
			TimeoutMs:   getenvInt("AGENT_SERVER_TTS_TIMEOUT_MS", 30000),
		},
	})
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}

	return fallback
}

func getenvInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func withRealtimeDefaults(cfg Config) Config {
	if cfg.Realtime.WSPath == "" {
		cfg.Realtime.WSPath = "/v1/realtime/ws"
	}
	if cfg.Realtime.ProtocolVersion == "" {
		cfg.Realtime.ProtocolVersion = "rtos-ws-v0"
	}
	if cfg.Realtime.Subprotocol == "" {
		cfg.Realtime.Subprotocol = "agent-server.realtime.v0"
	}
	if cfg.Realtime.AuthMode == "" {
		cfg.Realtime.AuthMode = "disabled"
	}
	if cfg.Realtime.TurnMode == "" {
		cfg.Realtime.TurnMode = "client_wakeup_server_vad"
	}
	if cfg.Realtime.IdleTimeoutMs == 0 {
		cfg.Realtime.IdleTimeoutMs = 15000
	}
	if cfg.Realtime.MaxSessionMs == 0 {
		cfg.Realtime.MaxSessionMs = 300000
	}
	if cfg.Realtime.MaxFrameBytes == 0 {
		cfg.Realtime.MaxFrameBytes = 4096
	}
	if cfg.Realtime.InputCodec == "" {
		cfg.Realtime.InputCodec = "pcm16le"
	}
	if cfg.Realtime.InputSampleRate == 0 {
		cfg.Realtime.InputSampleRate = 16000
	}
	if cfg.Realtime.InputChannels == 0 {
		cfg.Realtime.InputChannels = 1
	}
	if cfg.Realtime.OutputCodec == "" {
		cfg.Realtime.OutputCodec = "pcm16le"
	}
	if cfg.Realtime.OutputSampleRate == 0 {
		cfg.Realtime.OutputSampleRate = 16000
	}
	if cfg.Realtime.OutputChannels == 0 {
		cfg.Realtime.OutputChannels = 1
	}
	if cfg.Voice.Provider == "" {
		cfg.Voice.Provider = "bootstrap"
	}
	if cfg.Voice.ASRURL == "" {
		cfg.Voice.ASRURL = "http://127.0.0.1:8091/v1/asr/transcribe"
	}
	if cfg.Voice.ASRTimeoutMs == 0 {
		cfg.Voice.ASRTimeoutMs = 30000
	}
	if cfg.Voice.ASRLanguage == "" {
		cfg.Voice.ASRLanguage = "auto"
	}
	if cfg.TTS.Provider == "" {
		cfg.TTS.Provider = "none"
	}
	if cfg.TTS.MimoBaseURL == "" {
		cfg.TTS.MimoBaseURL = "https://api.xiaomimimo.com/v1"
	}
	if cfg.TTS.MimoModel == "" {
		cfg.TTS.MimoModel = "mimo-v2-tts"
	}
	if cfg.TTS.MimoVoice == "" {
		cfg.TTS.MimoVoice = "mimo_default"
	}
	if cfg.TTS.TimeoutMs == 0 {
		cfg.TTS.TimeoutMs = 30000
	}
	return cfg
}
