package voice

import (
	"context"
	"strings"
	"testing"
)

func TestBootstrapResponderForText(t *testing.T) {
	responder := NewBootstrapResponder("pcm16le", 16000, 1)

	response, err := responder.Respond(context.Background(), TurnRequest{
		SessionID: "sess_test",
		DeviceID:  "rtos-001",
		Text:      "hello",
	})
	if err != nil {
		t.Fatalf("respond failed: %v", err)
	}
	if !strings.Contains(response.Text, "hello") {
		t.Fatalf("expected response text to mention input, got %q", response.Text)
	}
	if len(response.AudioChunks) != 2 {
		t.Fatalf("expected 2 bootstrap audio chunks, got %d", len(response.AudioChunks))
	}
}
