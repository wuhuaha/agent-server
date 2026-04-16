package app

import (
	"errors"
	"strings"
)

type VoiceConfig struct {
	Provider                       string
	ASRURL                         string
	ASRTimeoutMs                   int
	ASRLanguage                    string
	ServerEndpointEnabled          bool
	ServerEndpointMinAudioMs       int
	ServerEndpointSilenceMs        int
	ServerEndpointLexicalMode      string
	ServerEndpointIncompleteHoldMs int
	ServerEndpointHintSilenceMs    int
	BargeInMinAudioMs              int
	BargeInHoldAudioMs             int
	LLMSemanticJudgeEnabled        bool
	LLMSemanticJudgeLLM            VoiceLLMProviderConfig
	LLMSemanticJudgeTimeoutMs      int
	LLMSemanticJudgeMinRunes       int
	LLMSemanticJudgeMinStableForMs int
	LLMSlotParserEnabled           bool
	LLMSlotParserLLM               VoiceLLMProviderConfig
	LLMSlotParserTimeoutMs         int
	LLMSlotParserMinRunes          int
	LLMSlotParserMinStableForMs    int
	SpeechPlannerEnabled           bool
	SpeechPlannerMinChunkRunes     int
	SpeechPlannerTargetChunkRunes  int
	EmitPlaceholderAudio           bool
	IflytekRTASR                   IflytekRTASRProviderConfig
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

type VoiceLLMProviderConfig struct {
	Provider    string
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int
}

func loadVoiceConfig() VoiceConfig {
	semanticJudgeProvider := getenv("AGENT_SERVER_VOICE_SEMANTIC_JUDGE_PROVIDER", "")
	semanticJudgeBaseURL := getenv("AGENT_SERVER_VOICE_SEMANTIC_JUDGE_BASE_URL", getenv("AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL", "https://api.deepseek.com"))
	semanticJudgeAPIKey := getenv("AGENT_SERVER_VOICE_SEMANTIC_JUDGE_API_KEY", getenv("AGENT_SERVER_AGENT_DEEPSEEK_API_KEY", getenv("DEEPSEEK_API_KEY", "")))
	semanticJudgeModel := getenv("AGENT_SERVER_VOICE_SEMANTIC_JUDGE_MODEL", getenv("AGENT_SERVER_AGENT_DEEPSEEK_MODEL", "deepseek-chat"))
	if strings.TrimSpace(semanticJudgeProvider) == "" {
		switch strings.ToLower(strings.TrimSpace(getenv("AGENT_SERVER_AGENT_LLM_PROVIDER", "auto"))) {
		case "", "auto", "deepseek", "deepseek_chat":
			if strings.TrimSpace(semanticJudgeAPIKey) != "" || strings.TrimSpace(getenv("AGENT_SERVER_VOICE_SEMANTIC_JUDGE_BASE_URL", "")) != "" {
				semanticJudgeProvider = "deepseek_chat"
			}
		}
	}
	slotParserProvider := getenv("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_PROVIDER", semanticJudgeProvider)
	slotParserBaseURL := getenv("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_BASE_URL", semanticJudgeBaseURL)
	slotParserAPIKey := getenv("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_API_KEY", semanticJudgeAPIKey)
	slotParserModel := getenv("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_MODEL", semanticJudgeModel)
	return VoiceConfig{
		Provider:                       getenv("AGENT_SERVER_VOICE_PROVIDER", "bootstrap"),
		ASRURL:                         getenv("AGENT_SERVER_VOICE_ASR_URL", "http://127.0.0.1:8091/v1/asr/transcribe"),
		ASRTimeoutMs:                   getenvInt("AGENT_SERVER_VOICE_ASR_TIMEOUT_MS", 30000),
		ASRLanguage:                    getenv("AGENT_SERVER_VOICE_ASR_LANGUAGE", "auto"),
		ServerEndpointEnabled:          getenvBool("AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED", false),
		ServerEndpointMinAudioMs:       getenvInt("AGENT_SERVER_VOICE_SERVER_ENDPOINT_MIN_AUDIO_MS", 320),
		ServerEndpointSilenceMs:        getenvInt("AGENT_SERVER_VOICE_SERVER_ENDPOINT_SILENCE_MS", 480),
		ServerEndpointLexicalMode:      getenv("AGENT_SERVER_VOICE_SERVER_ENDPOINT_LEXICAL_MODE", "conservative"),
		ServerEndpointIncompleteHoldMs: getenvInt("AGENT_SERVER_VOICE_SERVER_ENDPOINT_INCOMPLETE_HOLD_MS", 720),
		ServerEndpointHintSilenceMs:    getenvInt("AGENT_SERVER_VOICE_SERVER_ENDPOINT_HINT_SILENCE_MS", 160),
		BargeInMinAudioMs:              getenvInt("AGENT_SERVER_VOICE_BARGE_IN_MIN_AUDIO_MS", 120),
		BargeInHoldAudioMs:             getenvInt("AGENT_SERVER_VOICE_BARGE_IN_HOLD_AUDIO_MS", 240),
		LLMSemanticJudgeEnabled:        getenvBool("AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_ENABLED", true),
		LLMSemanticJudgeLLM: VoiceLLMProviderConfig{
			Provider:    semanticJudgeProvider,
			BaseURL:     semanticJudgeBaseURL,
			APIKey:      semanticJudgeAPIKey,
			Model:       semanticJudgeModel,
			Temperature: getenvFloat64("AGENT_SERVER_VOICE_SEMANTIC_JUDGE_TEMPERATURE", 0.1),
			MaxTokens:   getenvInt("AGENT_SERVER_VOICE_SEMANTIC_JUDGE_MAX_TOKENS", 128),
		},
		LLMSemanticJudgeTimeoutMs:      getenvInt("AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_TIMEOUT_MS", 220),
		LLMSemanticJudgeMinRunes:       getenvInt("AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_MIN_RUNES", 2),
		LLMSemanticJudgeMinStableForMs: getenvInt("AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_MIN_STABLE_FOR_MS", 120),
		LLMSlotParserEnabled:           getenvBool("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_ENABLED", true),
		LLMSlotParserLLM: VoiceLLMProviderConfig{
			Provider:    slotParserProvider,
			BaseURL:     slotParserBaseURL,
			APIKey:      slotParserAPIKey,
			Model:       slotParserModel,
			Temperature: getenvFloat64("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_TEMPERATURE", 0.1),
			MaxTokens:   getenvInt("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_MAX_TOKENS", 256),
		},
		LLMSlotParserTimeoutMs:        getenvInt("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_TIMEOUT_MS", 280),
		LLMSlotParserMinRunes:         getenvInt("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_MIN_RUNES", 4),
		LLMSlotParserMinStableForMs:   getenvInt("AGENT_SERVER_VOICE_LLM_SLOT_PARSER_MIN_STABLE_FOR_MS", 160),
		SpeechPlannerEnabled:          getenvBool("AGENT_SERVER_VOICE_SPEECH_PLANNER_ENABLED", true),
		SpeechPlannerMinChunkRunes:    getenvInt("AGENT_SERVER_VOICE_SPEECH_PLANNER_MIN_CHUNK_RUNES", 6),
		SpeechPlannerTargetChunkRunes: getenvInt("AGENT_SERVER_VOICE_SPEECH_PLANNER_TARGET_CHUNK_RUNES", 24),
		EmitPlaceholderAudio:          getenvBool("AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO", true),
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
	}
}

func applyVoiceDefaults(cfg *Config) {
	if cfg.Voice.Provider == "" {
		cfg.Voice.Provider = "bootstrap"
	}
	if cfg.Voice.ASRURL == "" {
		cfg.Voice.ASRURL = "http://127.0.0.1:8091/v1/asr/transcribe"
	}
	if cfg.Voice.ASRTimeoutMs <= 0 {
		cfg.Voice.ASRTimeoutMs = 30000
	}
	if cfg.Voice.ASRLanguage == "" {
		cfg.Voice.ASRLanguage = "auto"
	}
	if cfg.Voice.ServerEndpointMinAudioMs <= 0 {
		cfg.Voice.ServerEndpointMinAudioMs = 320
	}
	if cfg.Voice.ServerEndpointSilenceMs <= 0 {
		cfg.Voice.ServerEndpointSilenceMs = 480
	}
	if strings.TrimSpace(cfg.Voice.ServerEndpointLexicalMode) == "" {
		cfg.Voice.ServerEndpointLexicalMode = "conservative"
	}
	if cfg.Voice.ServerEndpointIncompleteHoldMs <= 0 {
		cfg.Voice.ServerEndpointIncompleteHoldMs = 720
	}
	if cfg.Voice.ServerEndpointHintSilenceMs <= 0 {
		cfg.Voice.ServerEndpointHintSilenceMs = 160
	}
	if cfg.Voice.BargeInMinAudioMs <= 0 {
		cfg.Voice.BargeInMinAudioMs = 120
	}
	if cfg.Voice.BargeInHoldAudioMs <= 0 {
		cfg.Voice.BargeInHoldAudioMs = 240
	}
	cfg.Voice.LLMSemanticJudgeLLM.Provider = normalizeVoiceLLMProvider(cfg.Voice.LLMSemanticJudgeLLM.Provider)
	if strings.TrimSpace(cfg.Voice.LLMSemanticJudgeLLM.BaseURL) == "" && cfg.Voice.LLMSemanticJudgeLLM.Provider == "deepseek_chat" {
		cfg.Voice.LLMSemanticJudgeLLM.BaseURL = "https://api.deepseek.com"
	}
	if strings.TrimSpace(cfg.Voice.LLMSemanticJudgeLLM.Model) == "" && cfg.Voice.LLMSemanticJudgeLLM.Provider == "deepseek_chat" {
		cfg.Voice.LLMSemanticJudgeLLM.Model = "deepseek-chat"
	}
	if cfg.Voice.LLMSemanticJudgeTimeoutMs <= 0 {
		cfg.Voice.LLMSemanticJudgeTimeoutMs = 220
	}
	if cfg.Voice.LLMSemanticJudgeMinRunes <= 0 {
		cfg.Voice.LLMSemanticJudgeMinRunes = 2
	}
	if cfg.Voice.LLMSemanticJudgeMinStableForMs <= 0 {
		cfg.Voice.LLMSemanticJudgeMinStableForMs = 120
	}
	cfg.Voice.LLMSlotParserLLM.Provider = normalizeVoiceLLMProvider(cfg.Voice.LLMSlotParserLLM.Provider)
	if strings.TrimSpace(cfg.Voice.LLMSlotParserLLM.BaseURL) == "" && cfg.Voice.LLMSlotParserLLM.Provider == "deepseek_chat" {
		cfg.Voice.LLMSlotParserLLM.BaseURL = "https://api.deepseek.com"
	}
	if strings.TrimSpace(cfg.Voice.LLMSlotParserLLM.Model) == "" && cfg.Voice.LLMSlotParserLLM.Provider == "deepseek_chat" {
		cfg.Voice.LLMSlotParserLLM.Model = "deepseek-chat"
	}
	if cfg.Voice.LLMSlotParserTimeoutMs <= 0 {
		cfg.Voice.LLMSlotParserTimeoutMs = 280
	}
	if cfg.Voice.LLMSlotParserMinRunes <= 0 {
		cfg.Voice.LLMSlotParserMinRunes = 4
	}
	if cfg.Voice.LLMSlotParserMinStableForMs <= 0 {
		cfg.Voice.LLMSlotParserMinStableForMs = 160
	}
	if cfg.Voice.SpeechPlannerMinChunkRunes <= 0 {
		cfg.Voice.SpeechPlannerMinChunkRunes = 6
	}
	if cfg.Voice.SpeechPlannerTargetChunkRunes <= 0 {
		cfg.Voice.SpeechPlannerTargetChunkRunes = 24
	}
	if cfg.Voice.SpeechPlannerTargetChunkRunes < cfg.Voice.SpeechPlannerMinChunkRunes {
		cfg.Voice.SpeechPlannerTargetChunkRunes = cfg.Voice.SpeechPlannerMinChunkRunes
	}
}

func validateVoiceConfig(cfg Config) error {
	var problems []string
	switch strings.ToLower(strings.TrimSpace(cfg.Voice.Provider)) {
	case "bootstrap":
	case "funasr_http":
		if strings.TrimSpace(cfg.Voice.ASRURL) == "" {
			problems = append(problems, "voice.asr_url is required when voice provider is funasr_http")
		}
	case "iflytek_rtasr":
		if strings.TrimSpace(cfg.Voice.IflytekRTASR.AppID) == "" ||
			strings.TrimSpace(cfg.Voice.IflytekRTASR.AccessKeyID) == "" ||
			strings.TrimSpace(cfg.Voice.IflytekRTASR.AccessKeySecret) == "" {
			problems = append(problems, "voice.iflytek_rtasr credentials are required when voice provider is iflytek_rtasr")
		}
	default:
		problems = append(problems, "voice.provider must be bootstrap, funasr_http, or iflytek_rtasr")
	}
	if cfg.Voice.ServerEndpointEnabled && !voiceProviderSupportsStreamingPreview(cfg.Voice.Provider) {
		problems = append(problems, "voice.server_endpoint_enabled requires a streaming preview-capable transcriber")
	}
	if cfg.Voice.LLMSemanticJudgeEnabled {
		problems = append(problems, validateVoiceLLMProvider("voice.llm_semantic_judge", cfg.Voice.LLMSemanticJudgeLLM)...)
	}
	if cfg.Voice.LLMSlotParserEnabled {
		problems = append(problems, validateVoiceLLMProvider("voice.llm_slot_parser", cfg.Voice.LLMSlotParserLLM)...)
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

func voiceProviderSupportsStreamingPreview(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "funasr_http", "iflytek_rtasr":
		return true
	default:
		return false
	}
}

func normalizeVoiceLLMProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "none", "disabled", "off":
		return ""
	case "deepseek", "deepseek_chat":
		return "deepseek_chat"
	case "openai", "openai_compat", "local_openai":
		return "openai_compat"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func validateVoiceLLMProvider(name string, cfg VoiceLLMProviderConfig) []string {
	switch normalizeVoiceLLMProvider(cfg.Provider) {
	case "", "disabled":
		return nil
	case "deepseek_chat":
		if strings.TrimSpace(cfg.APIKey) == "" {
			return []string{name + " api key is required when provider is deepseek_chat"}
		}
		return nil
	case "openai_compat":
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return []string{name + " base_url is required when provider is openai_compat"}
		}
		return nil
	default:
		return []string{name + " provider must be disabled, deepseek_chat, or openai_compat"}
	}
}
