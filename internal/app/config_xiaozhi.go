package app

import (
	"errors"
	"strings"
)

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

func loadXiaozhiConfig() XiaozhiCompatConfig {
	return XiaozhiCompatConfig{
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
	}
}

func applyXiaozhiDefaults(cfg *Config) {
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
	if cfg.Xiaozhi.OutputFrameDurationMs <= 0 {
		cfg.Xiaozhi.OutputFrameDurationMs = 60
	}
}

func validateXiaozhiConfig(cfg Config) error {
	if !cfg.Xiaozhi.Enabled {
		return nil
	}
	var problems []string
	if strings.TrimSpace(cfg.Xiaozhi.WSPath) == "" || strings.TrimSpace(cfg.Xiaozhi.OTAPath) == "" {
		problems = append(problems, "xiaozhi ws_path and ota_path must not be empty when xiaozhi is enabled")
	}
	if cfg.Xiaozhi.InputChannels != 1 || cfg.Xiaozhi.OutputChannels != 1 {
		problems = append(problems, "xiaozhi currently requires mono input and output")
	}
	if strings.ToLower(strings.TrimSpace(cfg.Realtime.OutputCodec)) != "pcm16le" {
		problems = append(problems, "xiaozhi requires realtime.output_codec=pcm16le so compat encoding has a pcm16le source")
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}
