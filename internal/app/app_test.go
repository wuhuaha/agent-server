package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-server/internal/agent"
)

func mustNewServer(t *testing.T, cfg Config) *http.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server, err := NewServer(cfg, logger)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	return server
}

func TestHealthz(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
	}

	server := mustNewServer(t, cfg)

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

	server := mustNewServer(t, cfg)

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

func TestInfoIncludesServerEndpointCandidateProfile(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
		Voice: VoiceConfig{
			Provider:                       "funasr_http",
			ServerEndpointEnabled:          true,
			ServerEndpointMinAudioMs:       320,
			ServerEndpointSilenceMs:        480,
			ServerEndpointLexicalMode:      "conservative",
			ServerEndpointIncompleteHoldMs: 720,
			ServerEndpointHintSilenceMs:    160,
		},
	}

	server := mustNewServer(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	res := httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	realtimeProfile, ok := body["realtime_profile"].(map[string]any)
	if !ok {
		t.Fatalf("expected realtime_profile object, got %#v", body["realtime_profile"])
	}
	serverEndpoint, ok := realtimeProfile["server_endpoint"].(map[string]any)
	if !ok {
		t.Fatalf("expected realtime_profile.server_endpoint object, got %#v", realtimeProfile["server_endpoint"])
	}
	if got := serverEndpoint["main_path_candidate"]; got != true {
		t.Fatalf("expected info to expose server endpoint candidate, got %v", got)
	}
	if got := serverEndpoint["mode"]; got != "server_vad_assisted" {
		t.Fatalf("expected server endpoint mode server_vad_assisted, got %v", got)
	}
}

func TestWebH5DebugRouteServesIndex(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
	}

	server := mustNewServer(t, cfg)

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

	server := mustNewServer(t, cfg)

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

	logged, ok := executor.(agent.LoggingTurnExecutor)
	if !ok {
		t.Fatalf("expected LoggingTurnExecutor, got %T", executor)
	}
	bootstrap, ok := logged.Inner.(agent.BootstrapTurnExecutor)
	if !ok {
		t.Fatalf("expected bootstrap executor, got %T", logged.Inner)
	}
	if _, ok := bootstrap.MemoryStore.(*agent.InMemoryMemoryStore); !ok {
		t.Fatalf("expected in-memory memory store, got %T", bootstrap.MemoryStore)
	}
	if _, ok := bootstrap.ToolRegistry.(*agent.RuntimeToolBackend); !ok {
		t.Fatalf("expected builtin tool registry, got %T", bootstrap.ToolRegistry)
	}
	if _, ok := bootstrap.ToolInvoker.(*agent.RuntimeToolBackend); !ok {
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

	logged, ok := executor.(agent.LoggingTurnExecutor)
	if !ok {
		t.Fatalf("expected LoggingTurnExecutor, got %T", executor)
	}
	llm, ok := logged.Inner.(agent.LLMTurnExecutor)
	if !ok {
		t.Fatalf("expected LLMTurnExecutor, got %T", logged.Inner)
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

	logged, ok := executor.(agent.LoggingTurnExecutor)
	if !ok {
		t.Fatalf("expected LoggingTurnExecutor, got %T", executor)
	}
	if _, ok := logged.Inner.(agent.LLMTurnExecutor); !ok {
		t.Fatalf("expected LLMTurnExecutor, got %T", logged.Inner)
	}
}

func TestRealtimeDiscoveryReportsEffectiveLLMProvider(t *testing.T) {
	cfg := Config{
		ListenAddr:  ":0",
		ServiceName: "agent-server",
		Environment: "test",
		Version:     "test",
		Agent: AgentConfig{
			LLMProvider: "auto",
		},
	}

	server := mustNewServer(t, cfg)

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
	if cfg.Voice.ServerEndpointEnabled {
		t.Fatal("expected server endpoint preview to stay disabled by default")
	}
	if cfg.Voice.ServerEndpointLexicalMode != "conservative" {
		t.Fatalf("expected default lexical mode conservative, got %q", cfg.Voice.ServerEndpointLexicalMode)
	}
	if cfg.Voice.ServerEndpointIncompleteHoldMs != 720 {
		t.Fatalf("expected default incomplete hold 720ms, got %d", cfg.Voice.ServerEndpointIncompleteHoldMs)
	}
	if cfg.Voice.ServerEndpointHintSilenceMs != 160 {
		t.Fatalf("expected default hint silence 160ms, got %d", cfg.Voice.ServerEndpointHintSilenceMs)
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

func TestConfigValidateRejectsExplicitDeepSeekWithoutAPIKey(t *testing.T) {
	err := Config{
		Agent: AgentConfig{
			LLMProvider: "deepseek_chat",
		},
	}.Validate()
	if err == nil || !strings.Contains(err.Error(), "agent.deepseek api key is required") {
		t.Fatalf("expected deepseek api key validation error, got %v", err)
	}
}

func TestConfigValidateAllowsSemanticJudgeOpenAICompatWithoutAPIKey(t *testing.T) {
	err := Config{
		Voice: VoiceConfig{
			Provider:                "funasr_http",
			LLMSemanticJudgeEnabled: true,
			LLMSemanticJudgeLLM: VoiceLLMProviderConfig{
				Provider: "openai_compat",
				BaseURL:  "http://127.0.0.1:8012/v1",
				Model:    "Qwen/Qwen3-1.7B",
			},
		},
	}.Validate()
	if err != nil {
		t.Fatalf("expected openai-compatible semantic judge config to validate, got %v", err)
	}
}

func TestConfigValidateRejectsSemanticJudgeDeepSeekWithoutAPIKey(t *testing.T) {
	err := Config{
		Voice: VoiceConfig{
			Provider:                "funasr_http",
			LLMSemanticJudgeEnabled: true,
			LLMSemanticJudgeLLM: VoiceLLMProviderConfig{
				Provider: "deepseek_chat",
				BaseURL:  "https://api.deepseek.com",
				Model:    "deepseek-chat",
			},
		},
	}.Validate()
	if err == nil || !strings.Contains(err.Error(), "voice.llm_semantic_judge api key is required") {
		t.Fatalf("expected semantic judge api key validation error, got %v", err)
	}
}

func TestConfigValidateAllowsSlotParserOpenAICompatWithoutAPIKey(t *testing.T) {
	err := Config{
		Voice: VoiceConfig{
			Provider:             "funasr_http",
			LLMSlotParserEnabled: true,
			LLMSlotParserLLM: VoiceLLMProviderConfig{
				Provider: "openai_compat",
				BaseURL:  "http://127.0.0.1:8012/v1",
				Model:    "Qwen/Qwen3-4B-Instruct-2507",
			},
		},
	}.Validate()
	if err != nil {
		t.Fatalf("expected openai-compatible slot parser config to validate, got %v", err)
	}
}

func TestConfigValidateRejectsXiaozhiWithoutPCM16LERealtimeOutput(t *testing.T) {
	err := Config{
		Xiaozhi: XiaozhiCompatConfig{
			Enabled: true,
		},
		Realtime: RealtimeConfig{
			OutputCodec: "opus",
		},
	}.Validate()
	if err == nil || !strings.Contains(err.Error(), "xiaozhi requires realtime.output_codec=pcm16le") {
		t.Fatalf("expected xiaozhi realtime output validation error, got %v", err)
	}
}

func TestConfigValidateRejectsHiddenPreviewOnBootstrapVoice(t *testing.T) {
	err := Config{
		Voice: VoiceConfig{
			Provider:              "bootstrap",
			ServerEndpointEnabled: true,
		},
	}.Validate()
	if err == nil || !strings.Contains(err.Error(), "voice.server_endpoint_enabled requires a streaming preview-capable transcriber") {
		t.Fatalf("expected hidden preview validation error, got %v", err)
	}
}

func TestConfigValidateAllowsStreamingPreviewVoiceProviders(t *testing.T) {
	err := Config{
		Voice: VoiceConfig{
			Provider:              "iflytek_rtasr",
			ServerEndpointEnabled: true,
			IflytekRTASR: IflytekRTASRProviderConfig{
				AppID:           "test-app",
				AccessKeyID:     "test-key-id",
				AccessKeySecret: "test-key-secret",
			},
		},
	}.Validate()
	if err != nil {
		t.Fatalf("expected iflytek_rtasr preview config to validate, got %v", err)
	}
}

func TestConfigValidateRejectsCosyVoiceInstructModeWithoutInstruction(t *testing.T) {
	err := Config{
		TTS: TTSConfig{
			Provider: "cosyvoice_http",
			CosyVoice: CosyVoiceTTSProviderConfig{
				BaseURL:            "http://127.0.0.1:50000",
				Mode:               "instruct",
				SpeakerID:          "中文女",
				SourceSampleRateHz: 22050,
			},
		},
	}.Validate()
	if err == nil || !strings.Contains(err.Error(), "tts.cosyvoice instruct_text is required") {
		t.Fatalf("expected cosyvoice instruct validation error, got %v", err)
	}
}
