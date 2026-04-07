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
	"strings"
	"time"
)

type HTTPTranscriber struct {
	Endpoint string
	Client   *http.Client
	Language string
}

type httpTranscriptionRequest struct {
	SessionID    string `json:"session_id"`
	DeviceID     string `json:"device_id"`
	Codec        string `json:"codec"`
	SampleRateHz int    `json:"sample_rate_hz"`
	Channels     int    `json:"channels"`
	Language     string `json:"language"`
	AudioBase64  string `json:"audio_base64"`
}

type httpTranscriptionResponse struct {
	Text           string   `json:"text"`
	Segments       []string `json:"segments"`
	DurationMs     int      `json:"duration_ms"`
	Model          string   `json:"model"`
	Device         string   `json:"device"`
	Language       string   `json:"language"`
	Emotion        string   `json:"emotion"`
	SpeakerID      string   `json:"speaker_id"`
	AudioEvents    []string `json:"audio_events"`
	EndpointReason string   `json:"endpoint_reason"`
	Partials       []string `json:"partials"`
	Error          string   `json:"error"`
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
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TranscriptionResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint, bytes.NewReader(body))
	if err != nil {
		return TranscriptionResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(httpReq)
	if err != nil {
		return TranscriptionResult{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranscriptionResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return TranscriptionResult{}, fmt.Errorf("transcriber returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded httpTranscriptionResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return TranscriptionResult{}, err
	}
	if strings.TrimSpace(decoded.Error) != "" {
		return TranscriptionResult{}, errors.New(decoded.Error)
	}

	return TranscriptionResult{
		Text:           strings.TrimSpace(decoded.Text),
		Segments:       decoded.Segments,
		DurationMs:     decoded.DurationMs,
		Model:          decoded.Model,
		Device:         decoded.Device,
		Language:       strings.TrimSpace(decoded.Language),
		Emotion:        strings.TrimSpace(decoded.Emotion),
		SpeakerID:      strings.TrimSpace(decoded.SpeakerID),
		AudioEvents:    append([]string(nil), decoded.AudioEvents...),
		EndpointReason: strings.TrimSpace(decoded.EndpointReason),
		Partials:       append([]string(nil), decoded.Partials...),
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
