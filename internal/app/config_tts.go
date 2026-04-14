package app

import (
	"errors"
	"strings"
)

type TTSConfig struct {
	Provider    string
	MimoBaseURL string
	MimoAPIKey  string
	MimoModel   string
	MimoVoice   string
	MimoStyle   string
	TimeoutMs   int
	CosyVoice   CosyVoiceTTSProviderConfig
	Iflytek     IflytekTTSProviderConfig
	Volcengine  VolcengineTTSProviderConfig
}

type CosyVoiceTTSProviderConfig struct {
	BaseURL            string
	Mode               string
	SpeakerID          string
	InstructText       string
	SourceSampleRateHz int
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

func loadTTSConfig() TTSConfig {
	return TTSConfig{
		Provider:    getenv("AGENT_SERVER_TTS_PROVIDER", "none"),
		MimoBaseURL: getenv("AGENT_SERVER_TTS_MIMO_BASE_URL", "https://api.xiaomimimo.com/v1"),
		MimoAPIKey:  getenv("MIMO_API_KEY", ""),
		MimoModel:   getenv("AGENT_SERVER_TTS_MIMO_MODEL", "mimo-v2-tts"),
		MimoVoice:   getenv("AGENT_SERVER_TTS_MIMO_VOICE", "mimo_default"),
		MimoStyle:   getenv("AGENT_SERVER_TTS_MIMO_STYLE", ""),
		TimeoutMs:   getenvInt("AGENT_SERVER_TTS_TIMEOUT_MS", 30000),
		CosyVoice: CosyVoiceTTSProviderConfig{
			BaseURL:            getenv("AGENT_SERVER_TTS_COSYVOICE_BASE_URL", "http://127.0.0.1:50000"),
			Mode:               getenv("AGENT_SERVER_TTS_COSYVOICE_MODE", "sft"),
			SpeakerID:          getenv("AGENT_SERVER_TTS_COSYVOICE_SPK_ID", "中文女"),
			InstructText:       getenv("AGENT_SERVER_TTS_COSYVOICE_INSTRUCT_TEXT", ""),
			SourceSampleRateHz: getenvInt("AGENT_SERVER_TTS_COSYVOICE_SOURCE_SAMPLE_RATE", 22050),
		},
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
	}
}

func applyTTSDefaults(cfg *Config) {
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
	if cfg.TTS.TimeoutMs <= 0 {
		cfg.TTS.TimeoutMs = 30000
	}
	if cfg.TTS.CosyVoice.BaseURL == "" {
		cfg.TTS.CosyVoice.BaseURL = "http://127.0.0.1:50000"
	}
	if cfg.TTS.CosyVoice.Mode == "" {
		cfg.TTS.CosyVoice.Mode = "sft"
	}
	if cfg.TTS.CosyVoice.SpeakerID == "" {
		cfg.TTS.CosyVoice.SpeakerID = "中文女"
	}
	if cfg.TTS.CosyVoice.SourceSampleRateHz <= 0 {
		cfg.TTS.CosyVoice.SourceSampleRateHz = 22050
	}
}

func validateTTSConfig(cfg Config) error {
	var problems []string
	switch strings.ToLower(strings.TrimSpace(cfg.TTS.Provider)) {
	case "", "none":
	case "mimo_v2_tts":
		if strings.TrimSpace(cfg.TTS.MimoAPIKey) == "" {
			problems = append(problems, "tts.mimo api key is required when tts provider is mimo_v2_tts")
		}
	case "cosyvoice_http":
		if strings.TrimSpace(cfg.TTS.CosyVoice.BaseURL) == "" {
			problems = append(problems, "tts.cosyvoice base url is required when tts provider is cosyvoice_http")
		}
		switch strings.ToLower(strings.TrimSpace(cfg.TTS.CosyVoice.Mode)) {
		case "", "sft":
			if strings.TrimSpace(cfg.TTS.CosyVoice.SpeakerID) == "" {
				problems = append(problems, "tts.cosyvoice spk_id is required when cosyvoice mode is sft")
			}
		case "instruct":
			if strings.TrimSpace(cfg.TTS.CosyVoice.SpeakerID) == "" {
				problems = append(problems, "tts.cosyvoice spk_id is required when cosyvoice mode is instruct")
			}
			if strings.TrimSpace(cfg.TTS.CosyVoice.InstructText) == "" {
				problems = append(problems, "tts.cosyvoice instruct_text is required when cosyvoice mode is instruct")
			}
		default:
			problems = append(problems, "tts.cosyvoice mode must be sft or instruct")
		}
		if cfg.TTS.CosyVoice.SourceSampleRateHz <= 0 {
			problems = append(problems, "tts.cosyvoice source sample rate must be positive")
		}
	case "iflytek_tts_ws":
		if strings.TrimSpace(cfg.TTS.Iflytek.AppID) == "" ||
			strings.TrimSpace(cfg.TTS.Iflytek.APIKey) == "" ||
			strings.TrimSpace(cfg.TTS.Iflytek.APISecret) == "" {
			problems = append(problems, "tts.iflytek credentials are required when tts provider is iflytek_tts_ws")
		}
	case "volcengine_tts":
		if strings.TrimSpace(cfg.TTS.Volcengine.AccessToken) == "" || strings.TrimSpace(cfg.TTS.Volcengine.AppID) == "" {
			problems = append(problems, "tts.volcengine access token and app id are required when tts provider is volcengine_tts")
		}
	default:
		problems = append(problems, "tts.provider must be none, mimo_v2_tts, cosyvoice_http, iflytek_tts_ws, or volcengine_tts")
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}
