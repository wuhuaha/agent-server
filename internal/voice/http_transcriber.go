package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPTranscriber struct {
	Endpoint string
	Client   *http.Client
	Language string
}

type httpTranscriptionRequest struct {
	SessionID    string   `json:"session_id"`
	DeviceID     string   `json:"device_id"`
	Codec        string   `json:"codec"`
	SampleRateHz int      `json:"sample_rate_hz"`
	Channels     int      `json:"channels"`
	Language     string   `json:"language"`
	AudioBase64  string   `json:"audio_base64"`
	Hotwords     []string `json:"hotwords,omitempty"`
	HintPhrases  []string `json:"hint_phrases,omitempty"`
}

type httpTranscriptionResponse struct {
	StreamID              string   `json:"stream_id"`
	Text                  string   `json:"text"`
	PreviewText           string   `json:"preview_text"`
	Segments              []string `json:"segments"`
	DurationMs            int      `json:"duration_ms"`
	ElapsedMs             int      `json:"elapsed_ms"`
	Model                 string   `json:"model"`
	Device                string   `json:"device"`
	Language              string   `json:"language"`
	Emotion               string   `json:"emotion"`
	SpeakerID             string   `json:"speaker_id"`
	AudioEvents           []string `json:"audio_events"`
	EndpointReason        string   `json:"endpoint_reason"`
	Partials              []string `json:"partials"`
	LatestPartial         string   `json:"latest_partial"`
	PreviewChanged        bool     `json:"preview_changed"`
	PreviewEndpointReason string   `json:"preview_endpoint_reason"`
	Mode                  string   `json:"mode"`
	Error                 string   `json:"error"`
}

type httpStreamingStartRequest struct {
	SessionID    string   `json:"session_id"`
	TurnID       string   `json:"turn_id"`
	TraceID      string   `json:"trace_id"`
	DeviceID     string   `json:"device_id"`
	Codec        string   `json:"codec"`
	SampleRateHz int      `json:"sample_rate_hz"`
	Channels     int      `json:"channels"`
	Language     string   `json:"language"`
	Hotwords     []string `json:"hotwords,omitempty"`
	HintPhrases  []string `json:"hint_phrases,omitempty"`
}

type httpStreamingPushRequest struct {
	StreamID    string `json:"stream_id"`
	AudioBase64 string `json:"audio_base64"`
}

type httpStreamingFinishRequest struct {
	StreamID string `json:"stream_id"`
}

type httpStreamingSession struct {
	transcriber        HTTPTranscriber
	streamID           string
	sink               TranscriptionDeltaSink
	started            bool
	closed             bool
	totalAudioBytes    int
	lastPartial        string
	lastEndpointReason string
}

func NewHTTPTranscriber(endpoint string, timeout time.Duration, language string) HTTPTranscriber {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return HTTPTranscriber{
		Endpoint: strings.TrimSpace(endpoint),
		Client: &http.Client{
			Timeout: timeout,
		},
		Language: strings.TrimSpace(language),
	}
}

func (t HTTPTranscriber) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	if strings.TrimSpace(t.Endpoint) == "" {
		return TranscriptionResult{}, fmt.Errorf("http transcriber endpoint is empty")
	}
	payload := httpTranscriptionRequest{
		SessionID:    req.SessionID,
		DeviceID:     req.DeviceID,
		Codec:        req.Codec,
		SampleRateHz: req.SampleRateHz,
		Channels:     req.Channels,
		Language:     firstNonEmpty(req.Language, t.Language, "auto"),
		AudioBase64:  base64.StdEncoding.EncodeToString(req.AudioPCM),
		Hotwords:     normalizedHintValues(req.Hotwords),
		HintPhrases:  normalizedHintValues(req.HintPhrases),
	}
	var decoded httpTranscriptionResponse
	if err := t.postJSON(ctx, t.Endpoint, payload, &decoded); err != nil {
		return TranscriptionResult{}, err
	}
	return decoded.toResult("batch"), nil
}

func (t HTTPTranscriber) StartStream(ctx context.Context, req TranscriptionRequest, sink TranscriptionDeltaSink) (StreamingTranscriptionSession, error) {
	if strings.TrimSpace(t.Endpoint) == "" {
		return nil, fmt.Errorf("http transcriber endpoint is empty")
	}
	startURL, err := t.streamEndpoint("/v1/asr/stream/start")
	if err != nil {
		return nil, err
	}
	var decoded httpTranscriptionResponse
	if err := t.postJSON(ctx, startURL, httpStreamingStartRequest{
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		TraceID:      req.TraceID,
		DeviceID:     req.DeviceID,
		Codec:        req.Codec,
		SampleRateHz: req.SampleRateHz,
		Channels:     req.Channels,
		Language:     firstNonEmpty(req.Language, t.Language, "auto"),
		Hotwords:     normalizedHintValues(req.Hotwords),
		HintPhrases:  normalizedHintValues(req.HintPhrases),
	}, &decoded); err != nil {
		return nil, err
	}
	streamID := strings.TrimSpace(decoded.StreamID)
	if streamID == "" {
		return nil, fmt.Errorf("http streaming transcriber returned empty stream_id")
	}
	return &httpStreamingSession{
		transcriber: t,
		streamID:    streamID,
		sink:        sink,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizedHintValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func (t HTTPTranscriber) streamEndpoint(route string) (string, error) {
	endpoint := strings.TrimSpace(t.Endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("http transcriber endpoint is empty")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse transcriber endpoint: %w", err)
	}
	path := strings.TrimSpace(parsed.Path)
	switch {
	case strings.HasSuffix(path, "/v1/asr/transcribe"):
		parsed.Path = strings.TrimSuffix(path, "/v1/asr/transcribe") + route
	case path == "":
		parsed.Path = route
	default:
		parsed.Path = strings.TrimRight(path, "/") + route
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (t HTTPTranscriber) postJSON(ctx context.Context, endpoint string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("transcriber returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if err := json.Unmarshal(respBody, target); err != nil {
		return err
	}
	if withError, ok := target.(*httpTranscriptionResponse); ok && strings.TrimSpace(withError.Error) != "" {
		return errors.New(withError.Error)
	}
	return nil
}

func (r httpTranscriptionResponse) toResult(defaultMode string) TranscriptionResult {
	mode := strings.TrimSpace(r.Mode)
	if mode == "" {
		mode = defaultMode
	}
	return TranscriptionResult{
		Text:           strings.TrimSpace(r.Text),
		Segments:       append([]string(nil), r.Segments...),
		DurationMs:     r.DurationMs,
		ElapsedMs:      r.ElapsedMs,
		Model:          r.Model,
		Device:         r.Device,
		Language:       strings.TrimSpace(r.Language),
		Emotion:        strings.TrimSpace(r.Emotion),
		SpeakerID:      strings.TrimSpace(r.SpeakerID),
		AudioEvents:    append([]string(nil), r.AudioEvents...),
		EndpointReason: strings.TrimSpace(r.EndpointReason),
		Partials:       append([]string(nil), r.Partials...),
		Mode:           mode,
	}
}

func (s *httpStreamingSession) PushAudio(ctx context.Context, chunk []byte) error {
	if s.closed {
		return fmt.Errorf("streaming transcription session is closed")
	}
	if len(chunk) == 0 {
		return nil
	}
	if !s.started {
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{
			Kind:       TranscriptionDeltaKindSpeechStart,
			AudioBytes: len(chunk),
		}); err != nil {
			return err
		}
		s.started = true
	}
	pushURL, err := s.transcriber.streamEndpoint("/v1/asr/stream/push")
	if err != nil {
		return err
	}
	var decoded httpTranscriptionResponse
	if err := s.transcriber.postJSON(ctx, pushURL, httpStreamingPushRequest{
		StreamID:    s.streamID,
		AudioBase64: base64.StdEncoding.EncodeToString(chunk),
	}, &decoded); err != nil {
		return err
	}
	s.totalAudioBytes += len(chunk)
	previewText := strings.TrimSpace(decoded.PreviewText)
	if previewText == "" {
		previewText = strings.TrimSpace(decoded.Text)
	}
	if previewText == "" && decoded.PreviewChanged {
		previewText = strings.TrimSpace(decoded.LatestPartial)
	}
	endpointReason := strings.TrimSpace(decoded.PreviewEndpointReason)
	if previewText != "" && (previewText != s.lastPartial || endpointReason != s.lastEndpointReason) {
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{
			Kind:           TranscriptionDeltaKindPartial,
			Text:           previewText,
			EndpointReason: endpointReason,
			AudioBytes:     s.totalAudioBytes,
		}); err != nil {
			return err
		}
		s.lastPartial = previewText
		s.lastEndpointReason = endpointReason
	}
	return nil
}

func (s *httpStreamingSession) Finish(ctx context.Context) (TranscriptionResult, error) {
	if s.closed {
		return TranscriptionResult{}, fmt.Errorf("streaming transcription session is closed")
	}
	finishURL, err := s.transcriber.streamEndpoint("/v1/asr/stream/finish")
	if err != nil {
		return TranscriptionResult{}, err
	}
	var decoded httpTranscriptionResponse
	if err := s.transcriber.postJSON(ctx, finishURL, httpStreamingFinishRequest{StreamID: s.streamID}, &decoded); err != nil {
		s.closed = true
		return TranscriptionResult{}, err
	}
	if s.started {
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{
			Kind:           TranscriptionDeltaKindSpeechEnd,
			EndpointReason: strings.TrimSpace(decoded.EndpointReason),
			AudioBytes:     s.totalAudioBytes,
		}); err != nil {
			s.closed = true
			return TranscriptionResult{}, err
		}
	}
	result := decoded.toResult("stream")
	if strings.TrimSpace(result.Text) != "" {
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{
			Kind:           TranscriptionDeltaKindFinal,
			Text:           result.Text,
			EndpointReason: result.EndpointReason,
			AudioBytes:     s.totalAudioBytes,
		}); err != nil {
			s.closed = true
			return TranscriptionResult{}, err
		}
	}
	s.closed = true
	return result, nil
}

func (s *httpStreamingSession) Close() error {
	if s.closed {
		return nil
	}
	closeURL, err := s.transcriber.streamEndpoint("/v1/asr/stream/close")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = s.transcriber.postJSON(ctx, closeURL, httpStreamingFinishRequest{StreamID: s.streamID}, &httpTranscriptionResponse{})
	s.closed = true
	return err
}
