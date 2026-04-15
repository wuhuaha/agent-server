package gateway

import (
	"testing"
	"time"

	"agent-server/internal/voice"
)

func TestPlannedPlaybackDurationForResponseUsesAudioChunks(t *testing.T) {
	response := voice.TurnResponse{
		AudioChunks: [][]byte{{1}, {2}, {3}},
	}
	if got := plannedPlaybackDurationForResponse(response, 20*time.Millisecond); got != 60*time.Millisecond {
		t.Fatalf("expected 60ms playback duration, got %s", got)
	}
}

func TestPlannedPlaybackDurationForResponseUsesAwareAudioStream(t *testing.T) {
	stream := voice.NewStaticAudioStream([][]byte{{1}, {2}, {3}, {4}})
	response := voice.TurnResponse{AudioStream: stream}
	if got := plannedPlaybackDurationForResponse(response, 20*time.Millisecond); got != 80*time.Millisecond {
		t.Fatalf("expected 80ms playback duration, got %s", got)
	}
}
