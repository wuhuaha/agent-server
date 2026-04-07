package voice

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultIflytekTTSScheme        = "ws"
	defaultIflytekTTSHost          = "tts-api.xfyun.cn"
	defaultIflytekTTSPath          = "/v2/tts"
	defaultIflytekTTSAUE           = "raw"
	defaultIflytekTTSTTE           = "UTF8"
	defaultIflytekTTSVoice         = "xiaoyan"
	defaultIflytekTTSSpeed         = 50
	defaultIflytekTTSVolume        = 50
	defaultIflytekTTSPitch         = 50
	defaultIflytekTTSTargetCodec   = "pcm16le"
	defaultIflytekTTSTargetRateHz  = 16000
	defaultIflytekTTSTargetChannel = 1
	defaultIflytekTTSTimeout       = 30 * time.Second
)

type IflytekTTSConfig struct {
	AppID          string
	APIKey         string
	APISecret      string
	Scheme         string
	Host           string
	Port           int
	Path           string
	Voice          string
	AUE            string
	AUF            string
	TextEncoding   string
	Speed          int
	Volume         int
	Pitch          int
	TargetCodec    string
	TargetRateHz   int
	TargetChannels int
	Timeout        time.Duration
	Dialer         *websocket.Dialer
}

type IflytekTTSSynthesizer struct {
	AppID          string
	APIKey         string
	APISecret      string
	Scheme         string
	Host           string
	Port           int
	Path           string
	Voice          string
	AUE            string
	AUF            string
	TextEncoding   string
	Speed          int
	Volume         int
	Pitch          int
	TargetCodec    string
	TargetRateHz   int
	TargetChannels int
	Timeout        time.Duration
	Dialer         *websocket.Dialer
}

func NewIflytekTTSSynthesizer(cfg IflytekTTSConfig) IflytekTTSSynthesizer {
	scheme := strings.ToLower(strings.TrimSpace(cfg.Scheme))
	if scheme == "" {
		scheme = defaultIflytekTTSScheme
	}
	if cfg.Port <= 0 {
		if scheme == "wss" {
			cfg.Port = 443
		} else {
			cfg.Port = 80
		}
	}
	targetRateHz := cfg.TargetRateHz
	if targetRateHz <= 0 {
		targetRateHz = defaultIflytekTTSTargetRateHz
	}
	targetChannels := cfg.TargetChannels
	if targetChannels <= 0 {
		targetChannels = defaultIflytekTTSTargetChannel
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultIflytekTTSTimeout
	}

	return IflytekTTSSynthesizer{
		AppID:          strings.TrimSpace(cfg.AppID),
		APIKey:         strings.TrimSpace(cfg.APIKey),
		APISecret:      strings.TrimSpace(cfg.APISecret),
		Scheme:         scheme,
		Host:           firstNonEmpty(strings.TrimSpace(cfg.Host), defaultIflytekTTSHost),
		Port:           cfg.Port,
		Path:           firstNonEmpty(strings.TrimSpace(cfg.Path), defaultIflytekTTSPath),
		Voice:          firstNonEmpty(strings.TrimSpace(cfg.Voice), defaultIflytekTTSVoice),
		AUE:            firstNonEmpty(strings.TrimSpace(cfg.AUE), defaultIflytekTTSAUE),
		AUF:            firstNonEmpty(strings.TrimSpace(cfg.AUF), fmt.Sprintf("audio/L16;rate=%d", targetRateHz)),
		TextEncoding:   firstNonEmpty(strings.TrimSpace(cfg.TextEncoding), defaultIflytekTTSTTE),
		Speed:          iflytekTTSIntDefault(cfg.Speed, defaultIflytekTTSSpeed),
		Volume:         iflytekTTSIntDefault(cfg.Volume, defaultIflytekTTSVolume),
		Pitch:          iflytekTTSIntDefault(cfg.Pitch, defaultIflytekTTSPitch),
		TargetCodec:    firstNonEmpty(strings.TrimSpace(cfg.TargetCodec), defaultIflytekTTSTargetCodec),
		TargetRateHz:   targetRateHz,
		TargetChannels: targetChannels,
		Timeout:        timeout,
		Dialer:         cfg.Dialer,
	}
}

func (s IflytekTTSSynthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
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
		Voice:        s.Voice,
		Model:        "iflytek_tts_ws",
	}, nil
}

func (s IflytekTTSSynthesizer) StreamSynthesize(ctx context.Context, req SynthesisRequest) (AudioStream, error) {
	if err := s.validate(req); err != nil {
		return nil, err
	}

	dialer := s.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	dialer = cloneWebsocketDialer(dialer)
	dialer.HandshakeTimeout = s.Timeout

	query := buildIflytekTTSQuery(s.APIKey, s.APISecret, s.Host, s.Path)
	conn, _, err := dialer.DialContext(ctx, buildIflytekTTSURL(s.Scheme, s.Host, s.Port, s.Path, query), nil)
	if err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(s.Timeout))
	_ = conn.SetWriteDeadline(time.Now().Add(s.Timeout))

	payload := map[string]any{
		"common": map[string]any{
			"app_id": s.AppID,
		},
		"business": map[string]any{
			"aue":    s.AUE,
			"auf":    s.AUF,
			"vcn":    s.Voice,
			"tte":    s.TextEncoding,
			"speed":  s.Speed,
			"volume": s.Volume,
			"pitch":  s.Pitch,
		},
		"data": map[string]any{
			"status": 2,
			"text":   base64.StdEncoding.EncodeToString([]byte(req.Text)),
		},
	}
	if err := conn.WriteJSON(payload); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &iflytekTTSAudioStream{
		conn:       conn,
		frameBytes: pcm16FrameBytes(s.TargetRateHz, s.TargetChannels),
	}, nil
}

func (s IflytekTTSSynthesizer) validate(req SynthesisRequest) error {
	if strings.TrimSpace(s.AppID) == "" || strings.TrimSpace(s.APIKey) == "" || strings.TrimSpace(s.APISecret) == "" {
		return fmt.Errorf("iflytek tts credentials are incomplete")
	}
	if strings.TrimSpace(req.Text) == "" {
		return fmt.Errorf("tts text is empty")
	}
	if s.TargetCodec != "pcm16le" {
		return fmt.Errorf("iflytek tts only supports pcm16le target codec, got %s", s.TargetCodec)
	}
	if s.TargetChannels != 1 {
		return fmt.Errorf("iflytek tts only supports mono output, got %d channels", s.TargetChannels)
	}
	if s.TargetRateHz <= 0 {
		return fmt.Errorf("iflytek tts target sample rate must be positive")
	}
	return nil
}

type iflytekTTSAudioStream struct {
	conn       *websocket.Conn
	frameBytes int
	pending    []byte
	done       bool
}

func (s *iflytekTTSAudioStream) Next(ctx context.Context) ([]byte, error) {
	for len(s.pending) < s.frameBytes && !s.done {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		message, err := readJSONWebsocketMessage(s.conn)
		if err != nil {
			return nil, err
		}
		if code, ok := asInt(message["code"]); ok && code != 0 {
			return nil, fmt.Errorf("iflytek tts returned code=%d", code)
		}
		data := asMap(message["data"])
		audioBase64, _ := data["audio"].(string)
		if strings.TrimSpace(audioBase64) != "" {
			decoded, err := base64.StdEncoding.DecodeString(audioBase64)
			if err != nil {
				return nil, err
			}
			s.pending = append(s.pending, decoded...)
		}
		if status, ok := asInt(data["status"]); ok && status == 2 {
			s.done = true
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

func (s *iflytekTTSAudioStream) Close() error {
	s.done = true
	return s.conn.Close()
}

func buildIflytekTTSQuery(apiKey, apiSecret, host, path string) string {
	date := time.Now().UTC().Format(http.TimeFormat)
	signatureOrigin := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1", host, date, path)
	mac := hmac.New(sha256.New, []byte(apiSecret))
	_, _ = mac.Write([]byte(signatureOrigin))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	authorizationOrigin := fmt.Sprintf(
		`api_key="%s",algorithm="hmac-sha256",headers="host date request-line",signature="%s"`,
		apiKey,
		signature,
	)
	values := url.Values{}
	values.Set("host", host)
	values.Set("date", date)
	values.Set("authorization", base64.StdEncoding.EncodeToString([]byte(authorizationOrigin)))
	return values.Encode()
}

func buildIflytekTTSURL(scheme, host string, port int, path, rawQuery string) string {
	urlValue := url.URL{
		Scheme:   firstNonEmpty(strings.TrimSpace(scheme), defaultIflytekTTSScheme),
		Host:     host,
		Path:     path,
		RawQuery: rawQuery,
	}
	if (urlValue.Scheme == "ws" && port != 80) || (urlValue.Scheme == "wss" && port != 443) {
		urlValue.Host = fmt.Sprintf("%s:%d", host, port)
	}
	return urlValue.String()
}

func iflytekTTSIntDefault(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
