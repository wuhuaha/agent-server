package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fakeWebsocketWriteConn struct {
	deadlines      []time.Time
	jsonWrites     int
	binaryWrites   int
	closeCalls     int
	writeJSONErr   error
	writeBinaryErr error
}

func (c *fakeWebsocketWriteConn) SetWriteDeadline(deadline time.Time) error {
	c.deadlines = append(c.deadlines, deadline)
	return nil
}

func (c *fakeWebsocketWriteConn) WriteJSON(any) error {
	c.jsonWrites++
	return c.writeJSONErr
}

func (c *fakeWebsocketWriteConn) WriteMessage(messageType int, _ []byte) error {
	if messageType == websocket.BinaryMessage {
		c.binaryWrites++
	}
	return c.writeBinaryErr
}

func (c *fakeWebsocketWriteConn) Close() error {
	c.closeCalls++
	return nil
}

func TestWriteWebsocketJSONAppliesDeadline(t *testing.T) {
	conn := &fakeWebsocketWriteConn{}

	if err := writeWebsocketJSON(conn, map[string]any{"ok": true}); err != nil {
		t.Fatalf("writeWebsocketJSON failed: %v", err)
	}
	if len(conn.deadlines) != 1 {
		t.Fatalf("expected one write deadline, got %d", len(conn.deadlines))
	}
	if conn.jsonWrites != 1 {
		t.Fatalf("expected one json write, got %d", conn.jsonWrites)
	}
	if conn.closeCalls != 0 {
		t.Fatalf("expected no close on successful write, got %d", conn.closeCalls)
	}
	if time.Until(conn.deadlines[0]) <= 0 {
		t.Fatalf("expected future write deadline, got %v", conn.deadlines[0])
	}
}

func TestWriteWebsocketBinaryClosesOnWriteError(t *testing.T) {
	conn := &fakeWebsocketWriteConn{writeBinaryErr: errors.New("slow peer")}

	err := writeWebsocketBinary(conn, []byte{0x01, 0x02})
	if err == nil || err.Error() != "slow peer" {
		t.Fatalf("expected slow peer write error, got %v", err)
	}
	if len(conn.deadlines) != 1 {
		t.Fatalf("expected one write deadline, got %d", len(conn.deadlines))
	}
	if conn.binaryWrites != 1 {
		t.Fatalf("expected one binary write, got %d", conn.binaryWrites)
	}
	if conn.closeCalls != 1 {
		t.Fatalf("expected connection close on write error, got %d", conn.closeCalls)
	}
}
