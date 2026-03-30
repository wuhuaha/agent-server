package voice

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type MimoTTSSynthesizer struct {
	BaseURL        string
	APIKey         string
	Model          string
	Voice          string
	Style          string
	Client         *http.Client
	TargetCodec    string
	TargetRateHz   int
	TargetChannels int
}

type mimoChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []mimoChatMessage       `json:"messages"`
	Audio    mimoChatCompletionAudio `json:"audio"`
	Stream   bool                    `json:"stream,omitempty"`
}

type mimoChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mimoChatCompletionAudio struct {
	Format string `json:"format"`
	Voice  string `json:"voice"`
}

type mimoChatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Audio   struct {
				Data       string `json:"data"`
				Transcript string `json:"transcript"`
			} `json:"audio"`
		} `json:"message"`
	} `json:"choices"`
}

func NewMimoTTSSynthesizer(
	apiKey, baseURL, model, voice, style string,
	timeout time.Duration,
	targetCodec string,
	targetRateHz, targetChannels int,
) MimoTTSSynthesizer {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.xiaomimimo.com/v1"
	}
	if model == "" {
		model = "mimo-v2-tts"
	}
	if voice == "" {
		voice = "mimo_default"
	}
	if targetCodec == "" {
		targetCodec = "pcm16le"
	}
	if targetRateHz <= 0 {
		targetRateHz = 16000
	}
	if targetChannels <= 0 {
		targetChannels = 1
	}
	return MimoTTSSynthesizer{
		BaseURL:        baseURL,
		APIKey:         strings.TrimSpace(apiKey),
		Model:          model,
		Voice:          voice,
		Style:          strings.TrimSpace(style),
		Client:         &http.Client{Timeout: timeout},
		TargetCodec:    targetCodec,
		TargetRateHz:   targetRateHz,
		TargetChannels: targetChannels,
	}
}

func (s MimoTTSSynthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
	if err := s.validate(); err != nil {
		return SynthesisResult{}, err
	}
	if strings.TrimSpace(req.Text) == "" {
		return SynthesisResult{}, fmt.Errorf("tts text is empty")
	}

	body, err := json.Marshal(mimoChatCompletionRequest{
		Model:    s.Model,
		Messages: s.buildMessages(req),
		Audio: mimoChatCompletionAudio{
			Format: "wav",
			Voice:  s.Voice,
		},
	})
	if err != nil {
		return SynthesisResult{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return SynthesisResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", s.APIKey)

	resp, err := s.Client.Do(httpReq)
	if err != nil {
		return SynthesisResult{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return SynthesisResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return SynthesisResult{}, fmt.Errorf("mimo tts returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded mimoChatCompletionResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return SynthesisResult{}, err
	}
	if len(decoded.Choices) == 0 {
		return SynthesisResult{}, fmt.Errorf("mimo tts returned no choices")
	}

	audioData := strings.TrimSpace(decoded.Choices[0].Message.Audio.Data)
	if audioData == "" {
		return SynthesisResult{}, fmt.Errorf("mimo tts returned empty audio data")
	}
	wavBytes, err := base64.StdEncoding.DecodeString(audioData)
	if err != nil {
		return SynthesisResult{}, err
	}

	audioPCM, sampleRateHz, channels, err := decodeWAVPCM16(wavBytes)
	if err != nil {
		return SynthesisResult{}, err
	}
	adaptedPCM, err := adaptPCM16(audioPCM, sampleRateHz, channels, s.TargetRateHz, s.TargetChannels)
	if err != nil {
		return SynthesisResult{}, err
	}

	return SynthesisResult{
		AudioPCM:     adaptedPCM,
		SampleRateHz: s.TargetRateHz,
		Channels:     s.TargetChannels,
		Codec:        s.TargetCodec,
		Voice:        s.Voice,
		Model:        decoded.Model,
	}, nil
}

func (s MimoTTSSynthesizer) StreamSynthesize(ctx context.Context, req SynthesisRequest) (AudioStream, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, fmt.Errorf("tts text is empty")
	}

	body, err := json.Marshal(mimoChatCompletionRequest{
		Model:    s.Model,
		Messages: s.buildMessages(req),
		Audio: mimoChatCompletionAudio{
			Format: "pcm16",
			Voice:  s.Voice,
		},
		Stream: true,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("api-key", s.APIKey)

	resp, err := s.Client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("mimo tts returned %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("mimo tts returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	frameBytes := s.TargetRateHz / 50 * s.TargetChannels * 2
	if frameBytes <= 0 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("target realtime profile produced an invalid frame size")
	}

	return &mimoPCM16AudioStream{
		body:       resp.Body,
		reader:     bufio.NewReader(resp.Body),
		frameBytes: frameBytes,
	}, nil
}

func (s MimoTTSSynthesizer) validate() error {
	if s.APIKey == "" {
		return fmt.Errorf("MIMO_API_KEY is not configured")
	}
	if s.TargetCodec != "pcm16le" {
		return fmt.Errorf("unsupported target codec %s", s.TargetCodec)
	}
	return nil
}

func (s MimoTTSSynthesizer) buildMessages(req SynthesisRequest) []mimoChatMessage {
	assistantText := strings.TrimSpace(req.Text)
	if style := strings.TrimSpace(s.Style); style != "" {
		assistantText = fmt.Sprintf("<style>%s</style>%s", style, assistantText)
	}

	messages := make([]mimoChatMessage, 0, 2)
	if userText := strings.TrimSpace(req.UserText); userText != "" {
		messages = append(messages, mimoChatMessage{Role: "user", Content: userText})
	}
	messages = append(messages, mimoChatMessage{Role: "assistant", Content: assistantText})
	return messages
}

type mimoPCM16AudioStream struct {
	body       io.ReadCloser
	reader     *bufio.Reader
	frameBytes int
	pending    []byte
	done       bool
}

func (s *mimoPCM16AudioStream) Next(ctx context.Context) ([]byte, error) {
	for len(s.pending) < s.frameBytes && !s.done {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		eventData, err := s.readNextEvent()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.done = true
				break
			}
			return nil, err
		}

		trimmed := strings.TrimSpace(eventData)
		if trimmed == "" {
			continue
		}
		if trimmed == "[DONE]" {
			s.done = true
			break
		}

		audioChunks, err := extractMimoAudioData(trimmed)
		if err != nil {
			return nil, err
		}
		for _, audioChunk := range audioChunks {
			decoded, err := base64.StdEncoding.DecodeString(audioChunk)
			if err != nil {
				return nil, err
			}
			s.pending = append(s.pending, decoded...)
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

	if len(s.pending) <= s.frameBytes {
		chunk := append([]byte(nil), s.pending...)
		s.pending = nil
		return chunk, nil
	}

	chunk := append([]byte(nil), s.pending[:s.frameBytes]...)
	s.pending = append([]byte(nil), s.pending[s.frameBytes:]...)
	return chunk, nil
}

func (s *mimoPCM16AudioStream) Close() error {
	s.done = true
	return s.body.Close()
}

func (s *mimoPCM16AudioStream) readNextEvent() (string, error) {
	var dataLines []string

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				dataLines = append(dataLines, strings.TrimLeft(strings.TrimPrefix(trimmed, "data:"), " "))
			}
			if errors.Is(err, io.EOF) && len(dataLines) > 0 {
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

func extractMimoAudioData(eventData string) ([]string, error) {
	var payload any
	if err := json.Unmarshal([]byte(eventData), &payload); err != nil {
		return nil, err
	}
	return collectAudioData(payload, false), nil
}

func collectAudioData(value any, inAudio bool) []string {
	switch typed := value.(type) {
	case map[string]any:
		var chunks []string
		if audio, ok := typed["audio"]; ok {
			chunks = append(chunks, collectAudioData(audio, true)...)
		}
		if message, ok := typed["message"]; ok {
			chunks = append(chunks, collectAudioData(message, inAudio)...)
		}
		if choices, ok := typed["choices"].([]any); ok {
			for _, choice := range choices {
				chunks = append(chunks, collectAudioData(choice, inAudio)...)
			}
		}
		if delta, ok := typed["delta"]; ok {
			switch deltaValue := delta.(type) {
			case string:
				eventType, _ := typed["type"].(string)
				if strings.Contains(strings.ToLower(eventType), "audio") && strings.TrimSpace(deltaValue) != "" {
					chunks = append(chunks, strings.TrimSpace(deltaValue))
				}
			default:
				chunks = append(chunks, collectAudioData(deltaValue, inAudio)...)
			}
		}
		if data, ok := typed["data"].(string); ok && inAudio {
			trimmed := strings.TrimSpace(data)
			if trimmed != "" {
				chunks = append(chunks, trimmed)
			}
		}
		return chunks
	case []any:
		var chunks []string
		for _, item := range typed {
			chunks = append(chunks, collectAudioData(item, inAudio)...)
		}
		return chunks
	default:
		return nil
	}
}
