package app

import (
	"errors"
	"strings"
)

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

type DeepSeekChatConfig struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int
}

func loadAgentConfig() AgentConfig {
	return AgentConfig{
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
	}
}

func applyAgentDefaults(cfg *Config) {
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
	if cfg.Agent.LLMTimeoutMs <= 0 {
		cfg.Agent.LLMTimeoutMs = 30000
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
	if strings.TrimSpace(cfg.Agent.DeepSeek.BaseURL) == "" {
		cfg.Agent.DeepSeek.BaseURL = "https://api.deepseek.com"
	}
	if strings.TrimSpace(cfg.Agent.DeepSeek.Model) == "" {
		cfg.Agent.DeepSeek.Model = "deepseek-chat"
	}
}

func validateAgentConfig(cfg Config) error {
	var problems []string
	switch strings.ToLower(strings.TrimSpace(cfg.Agent.MemoryProvider)) {
	case "in_memory", "noop":
	default:
		problems = append(problems, "agent.memory_provider must be in_memory or noop")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Agent.ToolProvider)) {
	case "builtin", "noop":
	default:
		problems = append(problems, "agent.tool_provider must be builtin or noop")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Agent.ExecutionMode)) {
	case "simulation", "dry_run", "live_control":
	default:
		problems = append(problems, "agent.execution_mode must be simulation, dry_run, or live_control")
	}
	switch provider := strings.ToLower(strings.TrimSpace(cfg.Agent.LLMProvider)); provider {
	case "", "auto", "bootstrap":
	case "deepseek", "deepseek_chat":
		if strings.TrimSpace(cfg.Agent.DeepSeek.APIKey) == "" {
			problems = append(problems, "agent.deepseek api key is required when deepseek_chat is selected")
		}
	default:
		problems = append(problems, "agent.llm_provider must be auto, bootstrap, or deepseek_chat")
	}
	if cfg.Agent.MemoryMaxTurns <= 0 {
		problems = append(problems, "agent.memory_max_turns must be positive")
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
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
