package gateway

import (
	"encoding/binary"
	"testing"
	"time"

	"agent-server/internal/voice"
)

func TestOutputEffectStateInterpolatesDuckEnvelope(t *testing.T) {
	var effects outputEffectState
	appliedAt := time.Unix(100, 0).UTC()
	ok := effects.ApplyDirective(voice.PlaybackDirective{
		Action:  voice.PlaybackActionDuckHold,
		Policy:  voice.InterruptionPolicyDuckOnly,
		Gain:    0.5,
		Attack:  100 * time.Millisecond,
		Hold:    100 * time.Millisecond,
		Release: 100 * time.Millisecond,
	}, appliedAt)
	if !ok {
		t.Fatal("expected duck directive to be applied")
	}

	if got := effects.CurrentGain(appliedAt.Add(50 * time.Millisecond)); got >= 1.0 || got <= 0.5 {
		t.Fatalf("expected attack gain between 0.5 and 1.0, got %.3f", got)
	}
	if got := effects.CurrentGain(appliedAt.Add(150 * time.Millisecond)); got != 0.5 {
		t.Fatalf("expected hold gain 0.5, got %.3f", got)
	}
	if got := effects.CurrentGain(appliedAt.Add(250 * time.Millisecond)); got >= 1.0 || got <= 0.5 {
		t.Fatalf("expected release gain between 0.5 and 1.0, got %.3f", got)
	}
	if got := effects.CurrentGain(appliedAt.Add(400 * time.Millisecond)); got != 1.0 {
		t.Fatalf("expected effect to release back to 1.0, got %.3f", got)
	}
}

func TestOutputEffectStateScalesPCM16Samples(t *testing.T) {
	var effects outputEffectState
	effects.ApplyDirective(voice.PlaybackDirective{
		Action: voice.PlaybackActionDuckLight,
		Policy: voice.InterruptionPolicyBackchannel,
		Gain:   0.25,
		Hold:   time.Second,
	}, time.Unix(200, 0).UTC())

	chunk := make([]byte, 4)
	binary.LittleEndian.PutUint16(chunk[0:], uint16(int16(1200)))
	negative := int16(-1200)
	binary.LittleEndian.PutUint16(chunk[2:], uint16(negative))

	scaled := effects.ApplyPCM16(chunk, time.Unix(200, 0).UTC().Add(10*time.Millisecond))
	if got := int16(binary.LittleEndian.Uint16(scaled[0:])); got != 300 {
		t.Fatalf("expected first sample 300, got %d", got)
	}
	if got := int16(binary.LittleEndian.Uint16(scaled[2:])); got != -300 {
		t.Fatalf("expected second sample -300, got %d", got)
	}
}
