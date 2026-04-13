package app

import (
	"errors"
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

func loadRealtimeConfig() RealtimeConfig {
	return RealtimeConfig{
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
	}
}

func applyRealtimeDefaults(cfg *Config) {
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
	if cfg.Realtime.IdleTimeoutMs <= 0 {
		cfg.Realtime.IdleTimeoutMs = 15000
	}
	if cfg.Realtime.MaxSessionMs <= 0 {
		cfg.Realtime.MaxSessionMs = 300000
	}
	if cfg.Realtime.MaxFrameBytes <= 0 {
		cfg.Realtime.MaxFrameBytes = 4096
	}
	if cfg.Realtime.InputCodec == "" {
		cfg.Realtime.InputCodec = "pcm16le"
	}
	if cfg.Realtime.InputSampleRate <= 0 {
		cfg.Realtime.InputSampleRate = 16000
	}
	if cfg.Realtime.InputChannels <= 0 {
		cfg.Realtime.InputChannels = 1
	}
	if cfg.Realtime.OutputCodec == "" {
		cfg.Realtime.OutputCodec = "pcm16le"
	}
	if cfg.Realtime.OutputSampleRate <= 0 {
		cfg.Realtime.OutputSampleRate = 16000
	}
	if cfg.Realtime.OutputChannels <= 0 {
		cfg.Realtime.OutputChannels = 1
	}
}

func validateRealtimeConfig(cfg Config) error {
	var problems []string
	if strings.TrimSpace(cfg.Realtime.WSPath) == "" {
		problems = append(problems, "realtime.ws_path must not be empty")
	}
	if strings.TrimSpace(cfg.Realtime.ProtocolVersion) == "" {
		problems = append(problems, "realtime.protocol_version must not be empty")
	}
	if strings.TrimSpace(cfg.Realtime.Subprotocol) == "" {
		problems = append(problems, "realtime.subprotocol must not be empty")
	}
	if cfg.Realtime.MaxFrameBytes <= 0 {
		problems = append(problems, "realtime.max_frame_bytes must be positive")
	}
	if cfg.Realtime.InputSampleRate <= 0 || cfg.Realtime.InputChannels <= 0 {
		problems = append(problems, "realtime input audio shape must be positive")
	}
	if cfg.Realtime.OutputSampleRate <= 0 || cfg.Realtime.OutputChannels <= 0 {
		problems = append(problems, "realtime output audio shape must be positive")
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}
