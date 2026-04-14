package gateway

import (
	"context"
	"errors"
	"time"

	"github.com/gorilla/websocket"
)

var errContinueReadLoop = errors.New("continue websocket read loop")

type websocketInboundMessage struct {
	messageType int
	payload     []byte
	err         error
}

func startWebsocketReadPump(ctx context.Context, conn *websocket.Conn) <-chan websocketInboundMessage {
	ch := make(chan websocketInboundMessage, 1)
	go func() {
		defer close(ch)
		for {
			messageType, payload, err := conn.ReadMessage()
			select {
			case <-ctx.Done():
				return
			case ch <- websocketInboundMessage{
				messageType: messageType,
				payload:     payload,
				err:         err,
			}:
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

func previewTickerC(ticker *time.Ticker) <-chan time.Time {
	if ticker == nil {
		return nil
	}
	return ticker.C
}
