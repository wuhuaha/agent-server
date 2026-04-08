package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-server/internal/agent"
)

func TestHealthz(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}
}

func TestInfo(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	res := httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}

	if !strings.Contains(res.Body.String(), cfg.ServiceName) {
		t.Fatalf("expected info response to mention %q", cfg.ServiceName)
	}
}

func TestWebH5DebugRouteServesIndex(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/debug/realtime-h5/", nil)
	res := httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}
	if !strings.Contains(res.Body.String(), "/v1/realtime/ws") {
		t.Fatalf("expected web h5 page to mention realtime websocket path, got %q", res.Body.String())
	}
}

func TestXiaozhiCompatRoutesAreMountedWhenEnabled(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
		Xiaozhi: XiaozhiCompatConfig{
			Enabled: true,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(cfg, logger)

	req := httptest.NewRequest(http.MethodPost, "/xiaozhi/ota/", strings.NewReader(`{"application":{"version":"test-fw"}}`))
	req.Host = "compat.example.com"
	res := httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}
	if !strings.Contains(res.Body.String(), "/xiaozhi/v1/") {
		t.Fatalf("expected OTA response to mention xiaozhi websocket path, got %q", res.Body.String())
	}
}

func TestBuildTurnExecutorUsesRealRuntimeBackendsByDefault(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	executor := buildTurnExecutor(withRealtimeDefaults(Config{}), logger)

	bootstrap, ok := executor.(agent.BootstrapTurnExecutor)
	if !ok {
		t.Fatalf("expected bootstrap executor, got %T", executor)
	}
	if _, ok := bootstrap.MemoryStore.(*agent.InMemoryMemoryStore); !ok {
		t.Fatalf("expected in-memory memory store, got %T", bootstrap.MemoryStore)
	}
	if _, ok := bootstrap.ToolRegistry.(*agent.BuiltinToolBackend); !ok {
		t.Fatalf("expected builtin tool registry, got %T", bootstrap.ToolRegistry)
	}
	if _, ok := bootstrap.ToolInvoker.(*agent.BuiltinToolBackend); !ok {
		t.Fatalf("expected builtin tool invoker, got %T", bootstrap.ToolInvoker)
	}
}

func TestBuildTurnExecutorSupportsDeepSeekChat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	executor := buildTurnExecutor(withRealtimeDefaults(Config{
		Agent: AgentConfig{
			LLMProvider: "deepseek_chat",
			DeepSeek: DeepSeekChatConfig{
				APIKey: "test-key",
			},
		},
	}), logger)

	llm, ok := executor.(agent.LLMTurnExecutor)
	if !ok {
		t.Fatalf("expected LLMTurnExecutor, got %T", executor)
	}
	if _, ok := llm.Model.(agent.DeepSeekChatModel); !ok {
		t.Fatalf("expected DeepSeekChatModel, got %T", llm.Model)
	}
	if llm.AssistantName != "小欧管家" {
		t.Fatalf("expected default assistant name xiaoou-guanjia, got %q", llm.AssistantName)
	}
	if llm.Persona != "household_control_screen" {
		t.Fatalf("expected default persona household_control_screen, got %q", llm.Persona)
	}
	if llm.ExecutionMode != "simulation" {
		t.Fatalf("expected default execution mode simulation, got %q", llm.ExecutionMode)
	}
	if _, ok := llm.MemoryStore.(*agent.InMemoryMemoryStore); !ok {
		t.Fatalf("expected in-memory memory store, got %T", llm.MemoryStore)
	}
}

func TestBuildTurnExecutorAutoSelectsDeepSeekChatWhenAPIKeyPresent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	executor := buildTurnExecutor(withRealtimeDefaults(Config{
		Agent: AgentConfig{
			DeepSeek: DeepSeekChatConfig{
				APIKey: "test-key",
			},
		},
	}), logger)

	if _, ok := executor.(agent.LLMTurnExecutor); !ok {
		t.Fatalf("expected LLMTurnExecutor, got %T", executor)
	}
}

func TestRealtimeDiscoveryReportsEffectiveLLMProvider(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
		Agent: AgentConfig{
			LLMProvider: "deepseek_chat",
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	res := httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}
	body := res.Body.String()
	if !strings.Contains(body, `"llm_provider":"bootstrap"`) {
		t.Fatalf("expected discovery to expose effective bootstrap fallback, got %q", body)
	}
}

func TestRealtimeDefaultsUseClientCommitTurnMode(t *testing.T) {
	cfg := withRealtimeDefaults(Config{})
	if cfg.Realtime.TurnMode != "client_wakeup_client_commit" {
		t.Fatalf("expected client_wakeup_client_commit, got %q", cfg.Realtime.TurnMode)
	}
	if cfg.Agent.Persona != "household_control_screen" {
		t.Fatalf("expected household_control_screen persona default, got %q", cfg.Agent.Persona)
	}
	if cfg.Agent.ExecutionMode != "simulation" {
		t.Fatalf("expected simulation execution mode default, got %q", cfg.Agent.ExecutionMode)
	}
	if cfg.Agent.Skills != "household_control" {
		t.Fatalf("expected household_control runtime skill default, got %q", cfg.Agent.Skills)
	}
}
