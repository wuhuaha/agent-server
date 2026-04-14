package gateway

import (
	"time"

	"github.com/gorilla/websocket"
)

var websocketWriteTimeout = 5 * time.Second

type websocketWriteConn interface {
	SetWriteDeadline(time.Time) error
	WriteJSON(any) error
	WriteMessage(int, []byte) error
	Close() error
}

func writeWebsocketJSON(conn websocketWriteConn, payload any) error {
	return withWebsocketWriteDeadline(conn, func() error {
		return conn.WriteJSON(payload)
	})
}

func writeWebsocketBinary(conn websocketWriteConn, payload []byte) error {
	return withWebsocketWriteDeadline(conn, func() error {
		return conn.WriteMessage(websocket.BinaryMessage, payload)
	})
}

func withWebsocketWriteDeadline(conn websocketWriteConn, write func() error) error {
	if err := conn.SetWriteDeadline(time.Now().Add(websocketWriteTimeout)); err != nil {
		_ = conn.Close()
		return err
	}
	if err := write(); err != nil {
		_ = conn.Close()
		return err
	}
	return nil
}
