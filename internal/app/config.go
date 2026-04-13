package app

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

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
		Realtime:    loadRealtimeConfig(),
		Xiaozhi:     loadXiaozhiConfig(),
		Agent:       loadAgentConfig(),
		Voice:       loadVoiceConfig(),
		TTS:         loadTTSConfig(),
	})
}

func (cfg Config) Validate() error {
	cfg = withRealtimeDefaults(cfg)
	return errors.Join(
		validateRealtimeConfig(cfg),
		validateXiaozhiConfig(cfg),
		validateAgentConfig(cfg),
		validateVoiceConfig(cfg),
		validateTTSConfig(cfg),
	)
}

func withRealtimeDefaults(cfg Config) Config {
	applyRealtimeDefaults(&cfg)
	applyXiaozhiDefaults(&cfg)
	applyAgentDefaults(&cfg)
	applyVoiceDefaults(&cfg)
	applyTTSDefaults(&cfg)
	return cfg
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
