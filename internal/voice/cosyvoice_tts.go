package voice

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultCosyVoiceTTSBaseURL        = "http://127.0.0.1:50000"
	defaultCosyVoiceTTSMode           = "sft"
	defaultCosyVoiceTTSSpeakerID      = "中文女"
	defaultCosyVoiceTTSSourceRateHz   = 22050
	defaultCosyVoiceTTSTargetCodec    = "pcm16le"
	defaultCosyVoiceTTSTargetRateHz   = 16000
	defaultCosyVoiceTTSTargetChannels = 1
	defaultCosyVoiceTTSTimeout        = 30 * time.Second
)

type CosyVoiceHTTPConfig struct {
	BaseURL            string
	Mode               string
	SpeakerID          string
	InstructText       string
	SourceSampleRateHz int
	TargetCodec        string
	TargetRateHz       int
	TargetChannels     int
	Timeout            time.Duration
	Client             *http.Client
}

type CosyVoiceHTTPSynthesizer struct {
	BaseURL            string
	Mode               string
	SpeakerID          string
	InstructText       string
	SourceSampleRateHz int
	TargetCodec        string
	TargetRateHz       int
	TargetChannels     int
	Timeout            time.Duration
	Client             *http.Client
}

func NewCosyVoiceHTTPSynthesizer(cfg CosyVoiceHTTPConfig) CosyVoiceHTTPSynthesizer {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultCosyVoiceTTSTimeout
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	targetRateHz := cfg.TargetRateHz
	if targetRateHz <= 0 {
		targetRateHz = defaultCosyVoiceTTSTargetRateHz
	}
	targetChannels := cfg.TargetChannels
	if targetChannels <= 0 {
		targetChannels = defaultCosyVoiceTTSTargetChannels
	}
	sourceRateHz := cfg.SourceSampleRateHz
	if sourceRateHz <= 0 {
		sourceRateHz = defaultCosyVoiceTTSSourceRateHz
	}

	return CosyVoiceHTTPSynthesizer{
		BaseURL:            firstNonEmpty(strings.TrimSpace(cfg.BaseURL), defaultCosyVoiceTTSBaseURL),
		Mode:               firstNonEmpty(strings.ToLower(strings.TrimSpace(cfg.Mode)), defaultCosyVoiceTTSMode),
		SpeakerID:          firstNonEmpty(strings.TrimSpace(cfg.SpeakerID), defaultCosyVoiceTTSSpeakerID),
		InstructText:       strings.TrimSpace(cfg.InstructText),
		SourceSampleRateHz: sourceRateHz,
		TargetCodec:        firstNonEmpty(strings.TrimSpace(cfg.TargetCodec), defaultCosyVoiceTTSTargetCodec),
		TargetRateHz:       targetRateHz,
		TargetChannels:     targetChannels,
		Timeout:            timeout,
		Client:             client,
	}
}

func (s CosyVoiceHTTPSynthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
	stream, err := s.StreamSynthesize(ctx, req)
	if err != nil {
		return SynthesisResult{}, err
	}
	defer stream.Close()

	audioPCM, err := collectAudioStream(ctx, stream)
	if err != nil {
		return SynthesisResult{}, err
	}
	return SynthesisResult{
		AudioPCM:     audioPCM,
		SampleRateHz: s.TargetRateHz,
		Channels:     s.TargetChannels,
		Codec:        s.TargetCodec,
		Voice:        s.SpeakerID,
		Model:        "cosyvoice_http",
	}, nil
}

func (s CosyVoiceHTTPSynthesizer) StreamSynthesize(ctx context.Context, req SynthesisRequest) (AudioStream, error) {
	if err := s.validate(req); err != nil {
		return nil, err
	}

	endpoint, payload := s.endpointAndPayload(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.Client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cosyvoice tts returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return &cosyVoiceAudioStream{
		body:       resp.Body,
		frameBytes: pcm16FrameBytes(s.TargetRateHz, s.TargetChannels),
		resampler:  newPCM16LinearResampler(s.SourceSampleRateHz, s.TargetRateHz),
	}, nil
}

func (s CosyVoiceHTTPSynthesizer) validate(req SynthesisRequest) error {
	if strings.TrimSpace(s.BaseURL) == "" {
		return fmt.Errorf("cosyvoice base url is empty")
	}
	if strings.TrimSpace(req.Text) == "" {
		return fmt.Errorf("tts text is empty")
	}
	if s.TargetCodec != "pcm16le" {
		return fmt.Errorf("cosyvoice tts only supports pcm16le target codec, got %s", s.TargetCodec)
	}
	if s.TargetChannels != 1 {
		return fmt.Errorf("cosyvoice tts only supports mono output, got %d channels", s.TargetChannels)
	}
	if s.TargetRateHz <= 0 {
		return fmt.Errorf("cosyvoice tts target sample rate must be positive")
	}
	if s.SourceSampleRateHz <= 0 {
		return fmt.Errorf("cosyvoice tts source sample rate must be positive")
	}
	switch s.Mode {
	case "sft":
		if strings.TrimSpace(s.SpeakerID) == "" {
			return fmt.Errorf("cosyvoice tts speaker id is required in sft mode")
		}
	case "instruct":
		if strings.TrimSpace(s.SpeakerID) == "" {
			return fmt.Errorf("cosyvoice tts speaker id is required in instruct mode")
		}
		if strings.TrimSpace(s.InstructText) == "" {
			return fmt.Errorf("cosyvoice tts instruct text is required in instruct mode")
		}
	default:
		return fmt.Errorf("unsupported cosyvoice mode %s", s.Mode)
	}
	return nil
}

func (s CosyVoiceHTTPSynthesizer) endpointAndPayload(req SynthesisRequest) (string, url.Values) {
	payload := url.Values{}
	payload.Set("tts_text", req.Text)

	switch s.Mode {
	case "instruct":
		payload.Set("spk_id", s.SpeakerID)
		payload.Set("instruct_text", s.InstructText)
		payload.Set("stream", "true")
		return strings.TrimRight(s.BaseURL, "/") + "/inference_instruct", payload
	default:
		payload.Set("spk_id", s.SpeakerID)
		payload.Set("stream", "true")
		return strings.TrimRight(s.BaseURL, "/") + "/inference_sft", payload
	}
}

type cosyVoiceAudioStream struct {
	body       io.ReadCloser
	frameBytes int
	resampler  *pcm16LinearResampler
	pending    []byte
	done       bool
}

func (s *cosyVoiceAudioStream) Next(ctx context.Context) ([]byte, error) {
	for len(s.pending) < s.frameBytes && !s.done {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		buffer := make([]byte, 4096)
		n, err := s.body.Read(buffer)
		if n > 0 {
			resampled, resampleErr := s.resampler.Feed(buffer[:n])
			if resampleErr != nil {
				return nil, resampleErr
			}
			s.pending = append(s.pending, resampled...)
		}
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			s.done = true
			rest, flushErr := s.resampler.Flush()
			if flushErr != nil {
				return nil, flushErr
			}
			s.pending = append(s.pending, rest...)
			break
		}
	}

	if len(s.pending) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.done {
			return nil, io.EOF
		}
	}

	return nextPCMChunk(&s.pending, s.frameBytes), nil
}

func (s *cosyVoiceAudioStream) Close() error {
	s.done = true
	return s.body.Close()
}

type pcm16LinearResampler struct {
	sourceRate int
	targetRate int
	step       float64

	rawRemainder []byte
	samples      []int16
	nextPos      float64
	eof          bool
}

func newPCM16LinearResampler(sourceRate, targetRate int) *pcm16LinearResampler {
	return &pcm16LinearResampler{
		sourceRate: sourceRate,
		targetRate: targetRate,
		step:       float64(sourceRate) / float64(targetRate),
	}
}

func (r *pcm16LinearResampler) Feed(chunk []byte) ([]byte, error) {
	if len(chunk) == 0 {
		return nil, nil
	}
	r.rawRemainder = append(r.rawRemainder, chunk...)
	usableBytes := len(r.rawRemainder) / 2 * 2
	if usableBytes == 0 {
		return nil, nil
	}

	for offset := 0; offset < usableBytes; offset += 2 {
		r.samples = append(r.samples, int16(binary.LittleEndian.Uint16(r.rawRemainder[offset:offset+2])))
	}
	r.rawRemainder = append([]byte(nil), r.rawRemainder[usableBytes:]...)
	return r.emitAvailable(), nil
}

func (r *pcm16LinearResampler) Flush() ([]byte, error) {
	if len(r.rawRemainder) != 0 {
		return nil, fmt.Errorf("cosyvoice pcm stream ended with an odd byte count")
	}
	r.eof = true
	return r.emitAvailable(), nil
}

func (r *pcm16LinearResampler) emitAvailable() []byte {
	if r.targetRate <= 0 || r.sourceRate <= 0 || len(r.samples) == 0 {
		return nil
	}

	var output []byte
	for {
		switch {
		case len(r.samples) >= 2 && r.nextPos <= float64(len(r.samples)-2):
			output = appendPCM16Sample(output, interpolatePCM16(r.samples, r.nextPos))
			r.nextPos += r.step
		case r.eof && r.nextPos <= float64(len(r.samples)-1):
			output = appendPCM16Sample(output, r.samples[len(r.samples)-1])
			r.nextPos += r.step
		default:
			r.compact()
			return output
		}
	}
}

func (r *pcm16LinearResampler) compact() {
	drop := int(math.Floor(r.nextPos))
	if drop <= 0 {
		return
	}
	if drop >= len(r.samples) {
		if r.eof {
			r.samples = nil
			r.nextPos = 0
			return
		}
		drop = len(r.samples) - 1
	}
	r.samples = append([]int16(nil), r.samples[drop:]...)
	r.nextPos -= float64(drop)
}

func interpolatePCM16(samples []int16, pos float64) int16 {
	base := int(math.Floor(pos))
	if base >= len(samples)-1 {
		return samples[len(samples)-1]
	}
	fraction := pos - float64(base)
	left := float64(samples[base])
	right := float64(samples[base+1])
	return int16(math.Round(left + (right-left)*fraction))
}

func appendPCM16Sample(dst []byte, sample int16) []byte {
	var encoded [2]byte
	binary.LittleEndian.PutUint16(encoded[:], uint16(sample))
	return append(dst, encoded[:]...)
}
