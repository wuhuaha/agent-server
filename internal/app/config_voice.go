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

func loadVoiceConfig() VoiceConfig {
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
		SpeechPlannerEnabled:           getenvBool("AGENT_SERVER_VOICE_SPEECH_PLANNER_ENABLED", true),
		SpeechPlannerMinChunkRunes:     getenvInt("AGENT_SERVER_VOICE_SPEECH_PLANNER_MIN_CHUNK_RUNES", 6),
		SpeechPlannerTargetChunkRunes:  getenvInt("AGENT_SERVER_VOICE_SPEECH_PLANNER_TARGET_CHUNK_RUNES", 24),
		EmitPlaceholderAudio:           getenvBool("AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO", true),
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
