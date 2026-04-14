package voice

import "time"

const (
	defaultBargeInMinAudioMs       = 120
	defaultBargeInIncompleteHoldMs = 240
)

type BargeInConfig struct {
	MinAudioMs       int
	IncompleteHoldMs int
}

func NormalizeBargeInConfig(cfg BargeInConfig) BargeInConfig {
	if cfg.MinAudioMs <= 0 {
		cfg.MinAudioMs = defaultBargeInMinAudioMs
	}
	if cfg.IncompleteHoldMs <= 0 {
		cfg.IncompleteHoldMs = defaultBargeInIncompleteHoldMs
	}
	return cfg
}

func ShouldAcceptBargeIn(preview InputPreview, sampleRateHz, channels int, cfg BargeInConfig) bool {
	cfg = NormalizeBargeInConfig(cfg)
	if !preview.SpeechStarted || preview.AudioBytes <= 0 || sampleRateHz <= 0 || channels <= 0 {
		return false
	}
	audioMs := pcm16AudioDurationMs(preview.AudioBytes, sampleRateHz, channels)
	if audioMs < cfg.MinAudioMs {
		return false
	}
	if looksLexicallyComplete(preview.PartialText) {
		return true
	}
	return audioMs >= cfg.MinAudioMs+cfg.IncompleteHoldMs
}

func pcm16AudioDurationMs(audioBytes, sampleRateHz, channels int) int {
	if audioBytes <= 0 || sampleRateHz <= 0 || channels <= 0 {
		return 0
	}
	return int((time.Duration(audioBytes/channels/2) * time.Second) / time.Duration(sampleRateHz) / time.Millisecond)
}
