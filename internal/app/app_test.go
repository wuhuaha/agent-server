package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
