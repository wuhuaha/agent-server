package voice

import "time"

const (
	defaultTurnDetectorSilenceMs  = 480
	defaultTurnDetectorMinAudioMs = 320
	defaultServerEndpointReason   = "server_silence_timeout"
)

type SilenceTurnDetectorConfig struct {
	MinAudioMs int
	SilenceMs  int
}

type SilenceTurnDetector struct {
	config          SilenceTurnDetectorConfig
	sampleRateHz    int
	channels        int
	audioBytes      int
	lastAudioAt     time.Time
	speechStarted   bool
	latestPartial   string
	commitSuggested bool
}

func NewSilenceTurnDetector(cfg SilenceTurnDetectorConfig, sampleRateHz, channels int) SilenceTurnDetector {
	if cfg.MinAudioMs <= 0 {
		cfg.MinAudioMs = defaultTurnDetectorMinAudioMs
	}
	if cfg.SilenceMs <= 0 {
		cfg.SilenceMs = defaultTurnDetectorSilenceMs
	}
	if sampleRateHz <= 0 {
		sampleRateHz = 16000
	}
	if channels <= 0 {
		channels = 1
	}
	return SilenceTurnDetector{
		config:       cfg,
		sampleRateHz: sampleRateHz,
		channels:     channels,
	}
}

func (d *SilenceTurnDetector) ObserveAudio(now time.Time, audioBytes int) {
	if audioBytes <= 0 {
		return
	}
	d.audioBytes += audioBytes
	d.lastAudioAt = now
	d.speechStarted = true
}

func (d *SilenceTurnDetector) ObserveTranscriptionDelta(_ time.Time, delta TranscriptionDelta) {
	switch delta.Kind {
	case TranscriptionDeltaKindSpeechStart:
		d.speechStarted = true
	case TranscriptionDeltaKindPartial, TranscriptionDeltaKindFinal:
		if text := delta.Text; text != "" {
			d.latestPartial = text
		}
	}
}

func (d *SilenceTurnDetector) Snapshot(now time.Time) InputPreview {
	preview := InputPreview{
		PartialText:    d.latestPartial,
		AudioBytes:     d.audioBytes,
		SpeechStarted:  d.speechStarted,
		EndpointReason: "",
	}
	if d.commitSuggested {
		preview.CommitSuggested = true
		preview.EndpointReason = defaultServerEndpointReason
		return preview
	}
	if d.latestPartial == "" || d.audioDurationMs() < d.config.MinAudioMs || d.lastAudioAt.IsZero() {
		return preview
	}
	if now.Sub(d.lastAudioAt) < time.Duration(d.config.SilenceMs)*time.Millisecond {
		return preview
	}
	d.commitSuggested = true
	preview.CommitSuggested = true
	preview.EndpointReason = defaultServerEndpointReason
	return preview
}

func (d *SilenceTurnDetector) audioDurationMs() int {
	if d.sampleRateHz <= 0 || d.channels <= 0 {
		return 0
	}
	return int(float64(d.audioBytes) / float64(d.channels) / 2.0 / float64(d.sampleRateHz) * 1000.0)
}
