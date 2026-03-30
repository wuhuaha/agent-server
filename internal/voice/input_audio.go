package voice

import (
	"fmt"
	"strings"

	pionopus "github.com/pion/opus"
)

type InputNormalizer interface {
	Decode([]byte) ([]byte, error)
	OutputCodec() string
	OutputSampleRate() int
	OutputChannels() int
}

type PCM16InputNormalizer struct {
	sampleRateHz int
	channels     int
}

func NewPCM16InputNormalizer(sampleRateHz, channels int) PCM16InputNormalizer {
	return PCM16InputNormalizer{sampleRateHz: sampleRateHz, channels: channels}
}

func (n PCM16InputNormalizer) Decode(payload []byte) ([]byte, error) {
	return append([]byte(nil), payload...), nil
}

func (n PCM16InputNormalizer) OutputCodec() string {
	return "pcm16le"
}

func (n PCM16InputNormalizer) OutputSampleRate() int {
	return n.sampleRateHz
}

func (n PCM16InputNormalizer) OutputChannels() int {
	return n.channels
}

type OpusInputNormalizer struct {
	decoder            pionopus.Decoder
	targetSampleRateHz int
	targetChannels     int
}

func NewOpusInputNormalizer(targetSampleRateHz, targetChannels int) OpusInputNormalizer {
	return OpusInputNormalizer{
		decoder:            pionopus.NewDecoder(),
		targetSampleRateHz: targetSampleRateHz,
		targetChannels:     targetChannels,
	}
}

func (n *OpusInputNormalizer) Decode(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	info, err := parseOpusPacketInfo(payload)
	if err != nil {
		return nil, err
	}
	if info.Mode != opusConfigurationModeSilkOnly {
		return nil, fmt.Errorf("unsupported opus mode %s; only SILK-only speech packets are currently supported", info.Mode)
	}

	out := make([]byte, info.SampleCount*info.Channels*2*3)
	bandwidth, isStereo, err := n.decoder.Decode(payload, out)
	if err != nil {
		return nil, err
	}

	decodedChannels := 1
	if isStereo {
		decodedChannels = 2
	}
	if decodedChannels != info.Channels {
		return nil, fmt.Errorf("decoded opus channel count %d does not match toc channel count %d", decodedChannels, info.Channels)
	}
	if decodedChannels != n.targetChannels {
		return nil, fmt.Errorf("unsupported opus channel count %d; target profile requires %d", decodedChannels, n.targetChannels)
	}

	decodedRateHz := bandwidth.SampleRate() * 3
	if decodedRateHz <= 0 {
		return nil, fmt.Errorf("decoded opus packet reported invalid sample rate")
	}

	payloadPCM := out[:info.SampleCount*decodedChannels*2*3]
	if decodedRateHz == n.targetSampleRateHz {
		return append([]byte(nil), payloadPCM...), nil
	}

	adapted, err := adaptPCM16(payloadPCM, decodedRateHz, decodedChannels, n.targetSampleRateHz, n.targetChannels)
	if err != nil {
		return nil, err
	}
	return adapted, nil
}

func (n *OpusInputNormalizer) OutputCodec() string {
	return "pcm16le"
}

func (n *OpusInputNormalizer) OutputSampleRate() int {
	return n.targetSampleRateHz
}

func (n *OpusInputNormalizer) OutputChannels() int {
	return n.targetChannels
}

func NewInputNormalizer(codec string, sampleRateHz, channels int) (InputNormalizer, error) {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "", "pcm16le":
		return NewPCM16InputNormalizer(sampleRateHz, channels), nil
	case "opus":
		normalizer := NewOpusInputNormalizer(sampleRateHz, channels)
		return &normalizer, nil
	default:
		return nil, fmt.Errorf("unsupported input codec %s", codec)
	}
}

type opusConfigurationMode string

const (
	opusConfigurationModeSilkOnly opusConfigurationMode = "silk-only"
	opusConfigurationModeHybrid   opusConfigurationMode = "hybrid"
	opusConfigurationModeCeltOnly opusConfigurationMode = "celt-only"
)

type opusPacketInfo struct {
	Mode         opusConfigurationMode
	SampleRateHz int
	Channels     int
	FrameCount   int
	SampleCount  int
}

func parseOpusPacketInfo(payload []byte) (opusPacketInfo, error) {
	if len(payload) == 0 {
		return opusPacketInfo{}, fmt.Errorf("opus packet is empty")
	}

	toc := payload[0]
	config := toc >> 3
	channels := 1
	if toc&0b00000100 != 0 {
		channels = 2
	}

	mode, sampleRateHz, frameDurationNs, err := opusConfigurationInfo(config)
	if err != nil {
		return opusPacketInfo{}, err
	}

	frameCount, err := opusFrameCount(payload, toc&0b00000011)
	if err != nil {
		return opusPacketInfo{}, err
	}
	if frameDurationNs*frameCount > 120000000 {
		return opusPacketInfo{}, fmt.Errorf("opus packet exceeds 120 ms maximum duration")
	}

	sampleCount := sampleRateHz * frameDurationNs * frameCount / 1000000000
	if sampleCount <= 0 {
		return opusPacketInfo{}, fmt.Errorf("opus packet produced invalid sample count")
	}

	return opusPacketInfo{
		Mode:         mode,
		SampleRateHz: sampleRateHz,
		Channels:     channels,
		FrameCount:   frameCount,
		SampleCount:  sampleCount,
	}, nil
}

func opusConfigurationInfo(config byte) (opusConfigurationMode, int, int, error) {
	switch {
	case config <= 3:
		return opusConfigurationModeSilkOnly, 8000, []int{10000000, 20000000, 40000000, 60000000}[config], nil
	case config <= 7:
		return opusConfigurationModeSilkOnly, 12000, []int{10000000, 20000000, 40000000, 60000000}[config-4], nil
	case config <= 11:
		return opusConfigurationModeSilkOnly, 16000, []int{10000000, 20000000, 40000000, 60000000}[config-8], nil
	case config <= 13:
		return opusConfigurationModeHybrid, 24000, []int{10000000, 20000000}[config-12], nil
	case config <= 15:
		return opusConfigurationModeHybrid, 48000, []int{10000000, 20000000}[config-14], nil
	case config <= 19:
		return opusConfigurationModeCeltOnly, 8000, []int{2500000, 5000000, 10000000, 20000000}[config-16], nil
	case config <= 23:
		return opusConfigurationModeCeltOnly, 16000, []int{2500000, 5000000, 10000000, 20000000}[config-20], nil
	case config <= 27:
		return opusConfigurationModeCeltOnly, 24000, []int{2500000, 5000000, 10000000, 20000000}[config-24], nil
	case config <= 31:
		return opusConfigurationModeCeltOnly, 48000, []int{2500000, 5000000, 10000000, 20000000}[config-28], nil
	default:
		return "", 0, 0, fmt.Errorf("unsupported opus configuration %d", config)
	}
}

func opusFrameCount(payload []byte, frameCode byte) (int, error) {
	switch frameCode {
	case 0:
		return 1, nil
	case 1, 2:
		return 2, nil
	case 3:
		if len(payload) < 2 {
			return 0, fmt.Errorf("opus packet is missing the code 3 frame count byte")
		}
		frameCount := int(payload[1] & 0b00111111)
		if frameCount == 0 {
			return 0, fmt.Errorf("opus packet frame count must not be zero")
		}
		return frameCount, nil
	default:
		return 0, fmt.Errorf("unsupported opus frame code %d", frameCode)
	}
}
