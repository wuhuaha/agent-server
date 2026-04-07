package voice

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultVolcengineTTSBaseURL        = "https://openspeech.bytedance.com"
	defaultVolcengineTTSPath           = "/api/v3/tts/unidirectional/sse"
	defaultVolcengineTTSResourceID     = "seed-tts-2.0"
	defaultVolcengineTTSVoiceType      = "zh_female_vv_uranus_bigtts"
	defaultVolcengineTTSTargetCodec    = "pcm16le"
	defaultVolcengineTTSTargetRateHz   = 16000
	defaultVolcengineTTSTargetChannels = 1
	defaultVolcengineTTSTimeout        = 30 * time.Second
	defaultVolcengineEmotionScale      = 4
)

type VolcengineTTSConfig struct {
	BaseURL        string
	AccessToken    string
	AppID          string
	ResourceID     string
	VoiceType      string
	SpeechRate     int
	LoudnessRate   int
	Emotion        string
	EmotionScale   int
	Model          string
	TargetCodec    string
	TargetRateHz   int
	TargetChannels int
	Timeout        time.Duration
	Client         *http.Client
}

type VolcengineTTSSynthesizer struct {
	BaseURL        string
	AccessToken    string
	AppID          string
	ResourceID     string
	VoiceType      string
	SpeechRate     int
	LoudnessRate   int
	Emotion        string
	EmotionScale   int
	Model          string
	TargetCodec    string
	TargetRateHz   int
	TargetChannels int
	Timeout        time.Duration
	Client         *http.Client
}

func NewVolcengineTTSSynthesizer(cfg VolcengineTTSConfig) VolcengineTTSSynthesizer {
	targetRateHz := cfg.TargetRateHz
	if targetRateHz <= 0 {
		targetRateHz = defaultVolcengineTTSTargetRateHz
	}
	targetChannels := cfg.TargetChannels
	if targetChannels <= 0 {
		targetChannels = defaultVolcengineTTSTargetChannels
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultVolcengineTTSTimeout
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	emotionScale := cfg.EmotionScale
	if emotionScale <= 0 {
		emotionScale = defaultVolcengineEmotionScale
	}

	return VolcengineTTSSynthesizer{
		BaseURL:        firstNonEmpty(strings.TrimSpace(cfg.BaseURL), defaultVolcengineTTSBaseURL),
		AccessToken:    strings.TrimSpace(cfg.AccessToken),
		AppID:          strings.TrimSpace(cfg.AppID),
		ResourceID:     firstNonEmpty(strings.TrimSpace(cfg.ResourceID), defaultVolcengineTTSResourceID),
		VoiceType:      firstNonEmpty(strings.TrimSpace(cfg.VoiceType), defaultVolcengineTTSVoiceType),
		SpeechRate:     cfg.SpeechRate,
		LoudnessRate:   cfg.LoudnessRate,
		Emotion:        strings.TrimSpace(cfg.Emotion),
		EmotionScale:   emotionScale,
		Model:          strings.TrimSpace(cfg.Model),
		TargetCodec:    firstNonEmpty(strings.TrimSpace(cfg.TargetCodec), defaultVolcengineTTSTargetCodec),
		TargetRateHz:   targetRateHz,
		TargetChannels: targetChannels,
		Timeout:        timeout,
		Client:         client,
	}
}

func (s VolcengineTTSSynthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
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
		Voice:        s.VoiceType,
		Model:        firstNonEmpty(s.Model, s.ResourceID),
	}, nil
}

func (s VolcengineTTSSynthesizer) StreamSynthesize(ctx context.Context, req SynthesisRequest) (AudioStream, error) {
	if err := s.validate(req); err != nil {
		return nil, err
	}

	payload := map[string]any{
		"user": map[string]any{
			"uid": s.AppID,
		},
		"req_params": map[string]any{
			"text":    req.Text,
			"speaker": s.VoiceType,
			"audio_params": map[string]any{
				"format":        "pcm",
				"sample_rate":   s.TargetRateHz,
				"speech_rate":   s.SpeechRate,
				"loudness_rate": s.LoudnessRate,
			},
		},
	}
	if s.Emotion != "" {
		audioParams := asMap(asMap(payload["req_params"])["audio_params"])
		audioParams["emotion"] = s.Emotion
		audioParams["emotion_scale"] = s.EmotionScale
	}
	if s.Model != "" {
		asMap(payload["req_params"])["model"] = s.Model
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.BaseURL, "/")+defaultVolcengineTTSPath, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-App-Id", s.AppID)
	httpReq.Header.Set("X-Api-Access-Key", s.AccessToken)
	httpReq.Header.Set("X-Api-Resource-Id", s.ResourceID)
	httpReq.Header.Set("X-Api-Request-Id", fmt.Sprintf("agent-server-%d", time.Now().UnixNano()))

	resp, err := s.Client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		payload, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("volcengine tts returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	return &volcengineTTSAudioStream{
		body:       resp.Body,
		reader:     bufio.NewReader(resp.Body),
		frameBytes: pcm16FrameBytes(s.TargetRateHz, s.TargetChannels),
	}, nil
}

func (s VolcengineTTSSynthesizer) validate(req SynthesisRequest) error {
	if strings.TrimSpace(s.AccessToken) == "" || strings.TrimSpace(s.AppID) == "" {
		return fmt.Errorf("volcengine tts credentials are incomplete")
	}
	if strings.TrimSpace(req.Text) == "" {
		return fmt.Errorf("tts text is empty")
	}
	if s.TargetCodec != "pcm16le" {
		return fmt.Errorf("volcengine tts only supports pcm16le target codec, got %s", s.TargetCodec)
	}
	if s.TargetChannels != 1 {
		return fmt.Errorf("volcengine tts only supports mono output, got %d channels", s.TargetChannels)
	}
	if s.TargetRateHz <= 0 {
		return fmt.Errorf("volcengine tts target sample rate must be positive")
	}
	return nil
}

type volcengineTTSAudioStream struct {
	body       io.ReadCloser
	reader     *bufio.Reader
	frameBytes int
	pending    []byte
	done       bool
}

func (s *volcengineTTSAudioStream) Next(ctx context.Context) ([]byte, error) {
	for len(s.pending) < s.frameBytes && !s.done {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		eventData, err := readSSEEvent(s.reader)
		if err != nil {
			if err == io.EOF {
				s.done = true
				break
			}
			return nil, err
		}
		if strings.TrimSpace(eventData) == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(eventData), &payload); err != nil {
			return nil, err
		}
		if audioBase64, _ := payload["data"].(string); strings.TrimSpace(audioBase64) != "" {
			decoded, err := base64.StdEncoding.DecodeString(audioBase64)
			if err != nil {
				return nil, err
			}
			s.pending = append(s.pending, decoded...)
			continue
		}
		if code, ok := asInt(payload["code"]); ok {
			if code == 20000000 {
				s.done = true
				break
			}
			if code != 0 {
				return nil, fmt.Errorf("volcengine tts returned code=%d", code)
			}
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

func (s *volcengineTTSAudioStream) Close() error {
	s.done = true
	return s.body.Close()
}

func readSSEEvent(reader *bufio.Reader) (string, error) {
	var dataLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				dataLines = append(dataLines, strings.TrimLeft(strings.TrimPrefix(trimmed, "data:"), " "))
			}
			if err == io.EOF && len(dataLines) > 0 {
				return strings.Join(dataLines, "\n"), nil
			}
			return "", err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if len(dataLines) == 0 {
				continue
			}
			return strings.Join(dataLines, "\n"), nil
		}
		if strings.HasPrefix(trimmed, ":") {
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			dataLines = append(dataLines, strings.TrimLeft(strings.TrimPrefix(trimmed, "data:"), " "))
		}
	}
}
