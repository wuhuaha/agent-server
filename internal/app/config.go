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

type AgentConfig struct {
	MemoryProvider  string
	MemoryMaxTurns  int
	ToolProvider    string
	Skills          string
	LLMProvider     string
	LLMTimeoutMs    int
	Persona         string
	ExecutionMode   string
	AssistantName   string
	LLMSystemPrompt string
	DeepSeek        DeepSeekChatConfig
}

type VoiceConfig struct {
	Provider             string
	ASRURL               string
	ASRTimeoutMs         int
	ASRLanguage          string
	EmitPlaceholderAudio bool
	IflytekRTASR         IflytekRTASRProviderConfig
}

type TTSConfig struct {
	Provider    string
	MimoBaseURL string
	MimoAPIKey  string
	MimoModel   string
	MimoVoice   string
	MimoStyle   string
	TimeoutMs   int
	Iflytek     IflytekTTSProviderConfig
	Volcengine  VolcengineTTSProviderConfig
}

type IflytekRTASRProviderConfig struct {
	AppID           string
	AccessKeyID     string
	AccessKeySecret string
	Scheme          string
	Host            string
	Port            int
	Path            string
	AudioEncode     string
	Language        string
	SampleRateHz    int
	FrameBytes      int
	FrameIntervalMs int
}

type IflytekTTSProviderConfig struct {
	AppID        string
	APIKey       string
	APISecret    string
	Scheme       string
	Host         string
	Port         int
	Path         string
	Voice        string
	AUE          string
	AUF          string
	TextEncoding string
	Speed        int
	Volume       int
	Pitch        int
}

type VolcengineTTSProviderConfig struct {
	BaseURL      string
	AccessToken  string
	AppID        string
	ResourceID   string
	VoiceType    string
	SpeechRate   int
	LoudnessRate int
	Emotion      string
	EmotionScale int
	Model        string
}

type DeepSeekChatConfig struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int
}

type XiaozhiCompatConfig struct {
	Enabled               bool
	WSPath                string
	OTAPath               string
	WelcomeVersion        int
	WelcomeTransport      string
	InputCodec            string
	InputSampleRate       int
	InputChannels         int
	InputFrameDurationMs  int
	MaxFrameBytes         int
	IdleTimeoutMs         int
	MaxSessionMs          int
	OutputCodec           string
	OutputSampleRate      int
	OutputChannels        int
	OutputFrameDurationMs int
}

// Config contains the startup configuration for the main service process.
type Config struct {
	ListenAddr  string
	ServiceName string
	Environment string
	Version     string
	Realtime    RealtimeConfig
	Xiaozhi     XiaozhiCompatConfig
	Agent       AgentConfig
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
			TurnMode:         getenv("AGENT_SERVER_REALTIME_TURN_MODE", "client_wakeup_client_commit"),
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
		Xiaozhi: XiaozhiCompatConfig{
			Enabled:               getenvBool("AGENT_SERVER_XIAOZHI_ENABLED", false),
			WSPath:                getenv("AGENT_SERVER_XIAOZHI_WS_PATH", "/xiaozhi/v1/"),
			OTAPath:               getenv("AGENT_SERVER_XIAOZHI_OTA_PATH", "/xiaozhi/ota/"),
			WelcomeVersion:        getenvInt("AGENT_SERVER_XIAOZHI_WELCOME_VERSION", 1),
			WelcomeTransport:      getenv("AGENT_SERVER_XIAOZHI_WELCOME_TRANSPORT", "websocket"),
			InputCodec:            getenv("AGENT_SERVER_XIAOZHI_INPUT_CODEC", "opus"),
			InputSampleRate:       getenvInt("AGENT_SERVER_XIAOZHI_INPUT_SAMPLE_RATE", 16000),
			InputChannels:         getenvInt("AGENT_SERVER_XIAOZHI_INPUT_CHANNELS", 1),
			InputFrameDurationMs:  getenvInt("AGENT_SERVER_XIAOZHI_INPUT_FRAME_DURATION_MS", 60),
			MaxFrameBytes:         getenvInt("AGENT_SERVER_XIAOZHI_MAX_FRAME_BYTES", 0),
			IdleTimeoutMs:         getenvInt("AGENT_SERVER_XIAOZHI_IDLE_TIMEOUT_MS", 0),
			MaxSessionMs:          getenvInt("AGENT_SERVER_XIAOZHI_MAX_SESSION_MS", 0),
			OutputCodec:           getenv("AGENT_SERVER_XIAOZHI_OUTPUT_CODEC", "opus"),
			OutputSampleRate:      getenvInt("AGENT_SERVER_XIAOZHI_OUTPUT_SAMPLE_RATE", 24000),
			OutputChannels:        getenvInt("AGENT_SERVER_XIAOZHI_OUTPUT_CHANNELS", 1),
			OutputFrameDurationMs: getenvInt("AGENT_SERVER_XIAOZHI_OUTPUT_FRAME_DURATION_MS", 60),
		},
		Agent: AgentConfig{
			MemoryProvider:  getenv("AGENT_SERVER_AGENT_MEMORY_PROVIDER", "in_memory"),
			MemoryMaxTurns:  getenvInt("AGENT_SERVER_AGENT_MEMORY_MAX_TURNS", 8),
			ToolProvider:    getenv("AGENT_SERVER_AGENT_TOOL_PROVIDER", "builtin"),
			Skills:          getenv("AGENT_SERVER_AGENT_SKILLS", "household_control"),
			LLMProvider:     getenv("AGENT_SERVER_AGENT_LLM_PROVIDER", "auto"),
			LLMTimeoutMs:    getenvInt("AGENT_SERVER_AGENT_LLM_TIMEOUT_MS", 30000),
			Persona:         getenv("AGENT_SERVER_AGENT_PERSONA", "household_control_screen"),
			ExecutionMode:   getenv("AGENT_SERVER_AGENT_EXECUTION_MODE", "simulation"),
			AssistantName:   getenv("AGENT_SERVER_AGENT_ASSISTANT_NAME", "小欧管家"),
			LLMSystemPrompt: getenv("AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT", ""),
			DeepSeek: DeepSeekChatConfig{
				BaseURL:     getenv("AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
				APIKey:      getenv("AGENT_SERVER_AGENT_DEEPSEEK_API_KEY", getenv("DEEPSEEK_API_KEY", "")),
				Model:       getenv("AGENT_SERVER_AGENT_DEEPSEEK_MODEL", "deepseek-chat"),
				Temperature: getenvFloat64("AGENT_SERVER_AGENT_DEEPSEEK_TEMPERATURE", 0.2),
				MaxTokens:   getenvInt("AGENT_SERVER_AGENT_DEEPSEEK_MAX_TOKENS", 0),
			},
		},
		Voice: VoiceConfig{
			Provider:             getenv("AGENT_SERVER_VOICE_PROVIDER", "bootstrap"),
			ASRURL:               getenv("AGENT_SERVER_VOICE_ASR_URL", "http://127.0.0.1:8091/v1/asr/transcribe"),
			ASRTimeoutMs:         getenvInt("AGENT_SERVER_VOICE_ASR_TIMEOUT_MS", 30000),
			ASRLanguage:          getenv("AGENT_SERVER_VOICE_ASR_LANGUAGE", "auto"),
			EmitPlaceholderAudio: getenvBool("AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO", true),
			IflytekRTASR: IflytekRTASRProviderConfig{
				AppID:           getenv("AGENT_SERVER_VOICE_IFLYTEK_RTASR_APP_ID", ""),
				AccessKeyID:     getenv("AGENT_SERVER_VOICE_IFLYTEK_RTASR_ACCESS_KEY_ID", ""),
				AccessKeySecret: getenv("AGENT_SERVER_VOICE_IFLYTEK_RTASR_ACCESS_KEY_SECRET", ""),
				Scheme:          getenv("AGENT_SERVER_VOICE_IFLYTEK_RTASR_SCHEME", "ws"),
				Host:            getenv("AGENT_SERVER_VOICE_IFLYTEK_RTASR_HOST", "office-api-ast-dx.iflyaisol.com"),
				Port:            getenvInt("AGENT_SERVER_VOICE_IFLYTEK_RTASR_PORT", 80),
				Path:            getenv("AGENT_SERVER_VOICE_IFLYTEK_RTASR_PATH", "/ast/communicate/v1"),
				AudioEncode:     getenv("AGENT_SERVER_VOICE_IFLYTEK_RTASR_AUDIO_ENCODE", "pcm_s16le"),
				Language:        getenv("AGENT_SERVER_VOICE_IFLYTEK_RTASR_LANGUAGE", "autodialect"),
				SampleRateHz:    getenvInt("AGENT_SERVER_VOICE_IFLYTEK_RTASR_SAMPLE_RATE", 16000),
				FrameBytes:      getenvInt("AGENT_SERVER_VOICE_IFLYTEK_RTASR_FRAME_BYTES", 1280),
				FrameIntervalMs: getenvInt("AGENT_SERVER_VOICE_IFLYTEK_RTASR_FRAME_INTERVAL_MS", 40),
			},
		},
		TTS: TTSConfig{
			Provider:    getenv("AGENT_SERVER_TTS_PROVIDER", "none"),
			MimoBaseURL: getenv("AGENT_SERVER_TTS_MIMO_BASE_URL", "https://api.xiaomimimo.com/v1"),
			MimoAPIKey:  getenv("MIMO_API_KEY", ""),
			MimoModel:   getenv("AGENT_SERVER_TTS_MIMO_MODEL", "mimo-v2-tts"),
			MimoVoice:   getenv("AGENT_SERVER_TTS_MIMO_VOICE", "mimo_default"),
			MimoStyle:   getenv("AGENT_SERVER_TTS_MIMO_STYLE", ""),
			TimeoutMs:   getenvInt("AGENT_SERVER_TTS_TIMEOUT_MS", 30000),
			Iflytek: IflytekTTSProviderConfig{
				AppID:        getenv("AGENT_SERVER_TTS_IFLYTEK_APP_ID", getenv("IFLYTEK_TTS_APP_ID", "")),
				APIKey:       getenv("AGENT_SERVER_TTS_IFLYTEK_API_KEY", getenv("IFLYTEK_TTS_API_KEY", "")),
				APISecret:    getenv("AGENT_SERVER_TTS_IFLYTEK_API_SECRET", getenv("IFLYTEK_TTS_API_SECRET", "")),
				Scheme:       getenv("AGENT_SERVER_TTS_IFLYTEK_SCHEME", "ws"),
				Host:         getenv("AGENT_SERVER_TTS_IFLYTEK_HOST", "tts-api.xfyun.cn"),
				Port:         getenvInt("AGENT_SERVER_TTS_IFLYTEK_PORT", 80),
				Path:         getenv("AGENT_SERVER_TTS_IFLYTEK_PATH", "/v2/tts"),
				Voice:        getenv("AGENT_SERVER_TTS_IFLYTEK_VOICE", "xiaoyan"),
				AUE:          getenv("AGENT_SERVER_TTS_IFLYTEK_AUE", "raw"),
				AUF:          getenv("AGENT_SERVER_TTS_IFLYTEK_AUF", ""),
				TextEncoding: getenv("AGENT_SERVER_TTS_IFLYTEK_TEXT_ENCODING", "UTF8"),
				Speed:        getenvInt("AGENT_SERVER_TTS_IFLYTEK_SPEED", 50),
				Volume:       getenvInt("AGENT_SERVER_TTS_IFLYTEK_VOLUME", 50),
				Pitch:        getenvInt("AGENT_SERVER_TTS_IFLYTEK_PITCH", 50),
			},
			Volcengine: VolcengineTTSProviderConfig{
				BaseURL:      getenv("AGENT_SERVER_TTS_VOLCENGINE_BASE_URL", "https://openspeech.bytedance.com"),
				AccessToken:  getenv("AGENT_SERVER_TTS_VOLCENGINE_ACCESS_TOKEN", getenv("VOLCENGINE_TTS_ACCESS_TOKEN", "")),
				AppID:        getenv("AGENT_SERVER_TTS_VOLCENGINE_APP_ID", getenv("VOLCENGINE_TTS_APPID", "")),
				ResourceID:   getenv("AGENT_SERVER_TTS_VOLCENGINE_RESOURCE_ID", getenv("VOLCENGINE_TTS_RESOURCE_ID", "seed-tts-2.0")),
				VoiceType:    getenv("AGENT_SERVER_TTS_VOLCENGINE_VOICE_TYPE", getenv("VOLCENGINE_TTS_DEFAULT_VOICE_TYPE", "zh_female_vv_uranus_bigtts")),
				SpeechRate:   getenvInt("AGENT_SERVER_TTS_VOLCENGINE_SPEECH_RATE", 0),
				LoudnessRate: getenvInt("AGENT_SERVER_TTS_VOLCENGINE_LOUDNESS_RATE", 0),
				Emotion:      getenv("AGENT_SERVER_TTS_VOLCENGINE_EMOTION", ""),
				EmotionScale: getenvInt("AGENT_SERVER_TTS_VOLCENGINE_EMOTION_SCALE", 4),
				Model:        getenv("AGENT_SERVER_TTS_VOLCENGINE_MODEL", ""),
			},
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

func getenvFloat64(key string, fallback float64) float64 {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func effectiveLLMProvider(cfg AgentConfig) string {
	switch strings.ToLower(strings.TrimSpace(cfg.LLMProvider)) {
	case "", "auto":
		if strings.TrimSpace(cfg.DeepSeek.APIKey) != "" {
			return "deepseek_chat"
		}
		return "bootstrap"
	case "bootstrap":
		return "bootstrap"
	case "deepseek", "deepseek_chat":
		if strings.TrimSpace(cfg.DeepSeek.APIKey) == "" {
			return "bootstrap"
		}
		return "deepseek_chat"
	default:
		return "bootstrap"
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
		cfg.Realtime.TurnMode = "client_wakeup_client_commit"
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
	if cfg.Xiaozhi.WSPath == "" {
		cfg.Xiaozhi.WSPath = "/xiaozhi/v1/"
	}
	if cfg.Xiaozhi.OTAPath == "" {
		cfg.Xiaozhi.OTAPath = "/xiaozhi/ota/"
	}
	if cfg.Xiaozhi.WelcomeVersion <= 0 {
		cfg.Xiaozhi.WelcomeVersion = 1
	}
	if cfg.Xiaozhi.WelcomeTransport == "" {
		cfg.Xiaozhi.WelcomeTransport = "websocket"
	}
	if cfg.Xiaozhi.InputCodec == "" {
		cfg.Xiaozhi.InputCodec = "opus"
	}
	if cfg.Xiaozhi.InputSampleRate <= 0 {
		cfg.Xiaozhi.InputSampleRate = 16000
	}
	if cfg.Xiaozhi.InputChannels <= 0 {
		cfg.Xiaozhi.InputChannels = 1
	}
	if cfg.Xiaozhi.InputFrameDurationMs <= 0 {
		cfg.Xiaozhi.InputFrameDurationMs = 60
	}
	if cfg.Xiaozhi.MaxFrameBytes <= 0 {
		cfg.Xiaozhi.MaxFrameBytes = cfg.Realtime.MaxFrameBytes
	}
	if cfg.Xiaozhi.IdleTimeoutMs <= 0 {
		cfg.Xiaozhi.IdleTimeoutMs = cfg.Realtime.IdleTimeoutMs
	}
	if cfg.Xiaozhi.MaxSessionMs <= 0 {
		cfg.Xiaozhi.MaxSessionMs = cfg.Realtime.MaxSessionMs
	}
	if cfg.Xiaozhi.OutputCodec == "" {
		cfg.Xiaozhi.OutputCodec = "opus"
	}
	if cfg.Xiaozhi.OutputSampleRate <= 0 {
		cfg.Xiaozhi.OutputSampleRate = 24000
	}
	if cfg.Xiaozhi.OutputChannels <= 0 {
		cfg.Xiaozhi.OutputChannels = 1
	}
	if strings.TrimSpace(cfg.Agent.AssistantName) == "" {
		cfg.Agent.AssistantName = "小欧管家"
	}
	if strings.TrimSpace(cfg.Agent.Persona) == "" {
		cfg.Agent.Persona = "household_control_screen"
	}
	if strings.TrimSpace(cfg.Agent.ExecutionMode) == "" {
		cfg.Agent.ExecutionMode = "simulation"
	}
	if cfg.Xiaozhi.OutputFrameDurationMs <= 0 {
		cfg.Xiaozhi.OutputFrameDurationMs = 60
	}
	if cfg.Agent.MemoryProvider == "" {
		cfg.Agent.MemoryProvider = "in_memory"
	}
	if cfg.Agent.MemoryMaxTurns <= 0 {
		cfg.Agent.MemoryMaxTurns = 8
	}
	if cfg.Agent.ToolProvider == "" {
		cfg.Agent.ToolProvider = "builtin"
	}
	if cfg.Agent.Skills == "" {
		cfg.Agent.Skills = "household_control"
	}
	if strings.TrimSpace(cfg.Agent.LLMProvider) == "" {
		cfg.Agent.LLMProvider = "auto"
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
