package gateway

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"time"

	"agent-server/internal/voice"
)

type outputEffectState struct {
	mu sync.Mutex

	appliedAt    time.Time
	attackUntil  time.Time
	holdUntil    time.Time
	releaseUntil time.Time
	fromGain     float64
	targetGain   float64
}

func (s *outputEffectState) ApplyDirective(directive voice.PlaybackDirective, now time.Time) bool {
	if !directive.ShouldDuckOutput() {
		return false
	}

	targetGain := clampGain(directive.Gain)
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.currentGainLocked(now)
	s.appliedAt = now
	s.fromGain = current
	s.targetGain = targetGain
	s.attackUntil = now.Add(maxDuration(directive.Attack, 0))
	s.holdUntil = s.attackUntil.Add(maxDuration(directive.Hold, 0))
	s.releaseUntil = s.holdUntil.Add(maxDuration(directive.Release, 0))
	return true
}

func (s *outputEffectState) CurrentGain(now time.Time) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentGainLocked(now)
}

func (s *outputEffectState) currentGainLocked(now time.Time) float64 {
	if s.appliedAt.IsZero() {
		return 1.0
	}

	if s.attackUntil.After(s.appliedAt) && now.Before(s.attackUntil) {
		return interpolateGain(s.fromGain, s.targetGain, now.Sub(s.appliedAt), s.attackUntil.Sub(s.appliedAt))
	}
	if now.Before(s.holdUntil) || now.Equal(s.holdUntil) {
		return s.targetGain
	}

	releaseStart := s.holdUntil
	if releaseStart.IsZero() {
		releaseStart = s.attackUntil
	}
	if s.releaseUntil.After(releaseStart) && now.Before(s.releaseUntil) {
		return interpolateGain(s.targetGain, 1.0, now.Sub(releaseStart), s.releaseUntil.Sub(releaseStart))
	}

	s.appliedAt = time.Time{}
	s.attackUntil = time.Time{}
	s.holdUntil = time.Time{}
	s.releaseUntil = time.Time{}
	s.fromGain = 1.0
	s.targetGain = 1.0
	return 1.0
}

func (s *outputEffectState) ApplyPCM16(chunk []byte, now time.Time) []byte {
	gain := s.CurrentGain(now)
	if gain >= 0.999 || len(chunk) < 2 {
		return chunk
	}

	scaled := append([]byte(nil), chunk...)
	for i := 0; i+1 < len(scaled); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(scaled[i:]))
		value := int(math.Round(float64(sample) * gain))
		if value > math.MaxInt16 {
			value = math.MaxInt16
		} else if value < math.MinInt16 {
			value = math.MinInt16
		}
		binary.LittleEndian.PutUint16(scaled[i:], uint16(int16(value)))
	}
	return scaled
}

type pcm16EffectAudioStream struct {
	inner   voice.AudioStream
	effects *outputEffectState
}

func newPCM16EffectAudioStream(inner voice.AudioStream, effects *outputEffectState) voice.AudioStream {
	if inner == nil || effects == nil {
		return inner
	}
	return &pcm16EffectAudioStream{
		inner:   inner,
		effects: effects,
	}
}

func (s *pcm16EffectAudioStream) Next(ctx context.Context) ([]byte, error) {
	chunk, err := s.inner.Next(ctx)
	if err != nil || len(chunk) == 0 {
		return chunk, err
	}
	return s.effects.ApplyPCM16(chunk, time.Now().UTC()), nil
}

func (s *pcm16EffectAudioStream) Close() error {
	return s.inner.Close()
}

func (s *pcm16EffectAudioStream) PlaybackDuration(frameDuration time.Duration) time.Duration {
	if aware, ok := s.inner.(playbackDurationAware); ok {
		return aware.PlaybackDuration(frameDuration)
	}
	return 0
}

func (s *pcm16EffectAudioStream) NextSegment(ctx context.Context) (voice.PlaybackSegment, bool, error) {
	segmented, ok := s.inner.(voice.SegmentedAudioStream)
	if !ok {
		return voice.PlaybackSegment{}, false, nil
	}
	return segmented.NextSegment(ctx)
}

func clampGain(gain float64) float64 {
	switch {
	case gain <= 0:
		return 0
	case gain >= 1:
		return 1
	default:
		return gain
	}
}

func interpolateGain(from, to float64, elapsed, total time.Duration) float64 {
	if total <= 0 {
		return to
	}
	ratio := float64(elapsed) / float64(total)
	if ratio <= 0 {
		return from
	}
	if ratio >= 1 {
		return to
	}
	return from + (to-from)*ratio
}

func maxDuration(value, fallback time.Duration) time.Duration {
	if value > fallback {
		return value
	}
	return fallback
}
