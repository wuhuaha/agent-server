package gateway

import (
	"errors"
	"io"
	"log/slog"
	"net"

	"agent-server/internal/session"

	"github.com/gorilla/websocket"
)

func appendWebsocketErrorLogAttrs(attrs []any, err error) []any {
	if err == nil {
		return attrs
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		attrs = append(attrs,
			"ws_close_code", closeErr.Code,
			"ws_close_text", closeErr.Text,
		)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		attrs = append(attrs, "net_timeout", netErr.Timeout())
	}
	if errors.Is(err, io.EOF) {
		attrs = append(attrs, "io_eof", true)
	}
	return attrs
}

func isExpectedWebsocketClosure(err error) bool {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		switch closeErr.Code {
		case websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived:
			return true
		}
	}
	return errors.Is(err, io.EOF)
}

func logWebsocketInboundTermination(logger *slog.Logger, runtime *connectionRuntime, trace turnTrace, err error) {
	if logger == nil || err == nil {
		return
	}
	snapshot := session.Snapshot{}
	if runtime != nil && runtime.session != nil {
		snapshot = runtime.session.Snapshot()
	}
	attrs := []any{
		"session_id", snapshot.SessionID,
		"session_state", snapshot.State,
	}
	if runtime != nil && runtime.remoteAddr != "" {
		attrs = append(attrs, "remote_addr", runtime.remoteAddr)
	}
	attrs = appendWebsocketErrorLogAttrs(attrs, err)

	if trace.TurnID != "" {
		if isExpectedWebsocketClosure(err) {
			logger.Info("gateway websocket inbound closed", turnTraceLogAttrs(snapshot.SessionID, trace, attrs...)...)
			return
		}
		logger.Warn("gateway websocket inbound read failed", turnTraceLogAttrs(snapshot.SessionID, trace, append(attrs, "error", err)...)...)
		return
	}

	if isExpectedWebsocketClosure(err) {
		logger.Info("gateway websocket inbound closed", attrs...)
		return
	}
	attrs = append(attrs, "error", err)
	logger.Warn("gateway websocket inbound read failed", attrs...)
}
