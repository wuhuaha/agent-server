package voice

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultIflytekRTASRScheme        = "ws"
	defaultIflytekRTASRHost          = "office-api-ast-dx.iflyaisol.com"
	defaultIflytekRTASRPath          = "/ast/communicate/v1"
	defaultIflytekRTASRAudioEncode   = "pcm_s16le"
	defaultIflytekRTASRLanguage      = "autodialect"
	defaultIflytekRTASRSampleRateHz  = 16000
	defaultIflytekRTASRFrameBytes    = 1280
	defaultIflytekRTASRFrameInterval = 40 * time.Millisecond
	defaultIflytekRTASRTimeout       = 30 * time.Second
)

type IflytekRTASRConfig struct {
	AppID           string
	AccessKeyID     string
	AccessKeySecret string
	Scheme          string
	Host            string
	Port            int
	Path            string
	AudioEncode     string
	Language        string
	SampleRateHz    int
	Timeout         time.Duration
	FrameBytes      int
	FrameInterval   time.Duration
	Dialer          *websocket.Dialer
}

type IflytekRTASRTranscriber struct {
	AppID           string
	AccessKeyID     string
	AccessKeySecret string
	Scheme          string
	Host            string
	Port            int
	Path            string
	AudioEncode     string
	Language        string
	SampleRateHz    int
	Timeout         time.Duration
	FrameBytes      int
	FrameInterval   time.Duration
	Dialer          *websocket.Dialer
}

func NewIflytekRTASRTranscriber(cfg IflytekRTASRConfig) IflytekRTASRTranscriber {
	scheme := strings.ToLower(strings.TrimSpace(cfg.Scheme))
	if scheme == "" {
		scheme = defaultIflytekRTASRScheme
	}
	sampleRateHz := cfg.SampleRateHz
	if sampleRateHz <= 0 {
		sampleRateHz = defaultIflytekRTASRSampleRateHz
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultIflytekRTASRTimeout
	}
	frameBytes := cfg.FrameBytes
	if frameBytes <= 0 {
		frameBytes = defaultIflytekRTASRFrameBytes
	}
	frameInterval := cfg.FrameInterval
	if frameInterval < 0 {
		frameInterval = 0
	}
	if frameInterval == 0 {
		frameInterval = defaultIflytekRTASRFrameInterval
	}
	if cfg.Port <= 0 {
		if scheme == "wss" {
			cfg.Port = 443
		} else {
			cfg.Port = 80
		}
	}

	return IflytekRTASRTranscriber{
		AppID:           strings.TrimSpace(cfg.AppID),
		AccessKeyID:     strings.TrimSpace(cfg.AccessKeyID),
		AccessKeySecret: strings.TrimSpace(cfg.AccessKeySecret),
		Scheme:          scheme,
		Host:            strings.TrimSpace(cfg.Host),
		Port:            cfg.Port,
		Path:            strings.TrimSpace(cfg.Path),
		AudioEncode:     firstNonEmpty(strings.TrimSpace(cfg.AudioEncode), defaultIflytekRTASRAudioEncode),
		Language:        firstNonEmpty(strings.TrimSpace(cfg.Language), defaultIflytekRTASRLanguage),
		SampleRateHz:    sampleRateHz,
		Timeout:         timeout,
		FrameBytes:      frameBytes,
		FrameInterval:   frameInterval,
		Dialer:          cfg.Dialer,
	}
}

func (t IflytekRTASRTranscriber) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	if err := t.validate(req); err != nil {
		return TranscriptionResult{}, err
	}

	dialer := t.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	dialer = cloneWebsocketDialer(dialer)
	dialer.HandshakeTimeout = t.Timeout

	query := buildIflytekRTASRQuery(
		t.AppID,
		t.AccessKeyID,
		t.AccessKeySecret,
		t.AudioEncode,
		t.Language,
		strconv.Itoa(t.SampleRateHz),
	)
	conn, _, err := dialer.DialContext(ctx, buildIflytekRTASRURL(t.Scheme, t.Host, t.Port, t.Path, query), nil)
	if err != nil {
		return TranscriptionResult{}, err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(t.Timeout))
	_ = conn.SetWriteDeadline(time.Now().Add(t.Timeout))

	started, err := readJSONWebsocketMessage(conn)
	if err != nil {
		return TranscriptionResult{}, err
	}
	sessionID, _ := asMap(started["data"])["sessionId"].(string)

	if err := writeIflytekRTASRAudio(ctx, conn, req.AudioPCM, t.FrameBytes, t.FrameInterval); err != nil {
		return TranscriptionResult{}, err
	}
	endPayload := map[string]any{"end": true}
	if sessionID != "" {
		endPayload["sessionId"] = sessionID
	}
	if err := conn.WriteJSON(endPayload); err != nil {
		return TranscriptionResult{}, err
	}

	segmentsByID := map[int]string{}
	trailingSegments := make([]string, 0, 1)
	partials := make([]string, 0, 4)
	for {
		message, err := readJSONWebsocketMessage(conn)
		if err != nil {
			return TranscriptionResult{}, err
		}
		if action, _ := message["action"].(string); strings.EqualFold(action, "error") {
			return TranscriptionResult{}, fmt.Errorf("iflytek rtasr returned action=error")
		}
		if code, ok := asInt(message["code"]); ok && code != 0 {
			return TranscriptionResult{}, fmt.Errorf("iflytek rtasr returned code=%d", code)
		}
		text := strings.TrimSpace(extractIflytekRTASRText(message))
		if text != "" {
			partials = append(partials, text)
			if segID, ok := extractIflytekRTASRSegmentID(message); ok {
				segmentsByID[segID] = text
			} else {
				trailingSegments = append(trailingSegments, text)
			}
		}
		if extractIflytekRTASRLastSegment(message) {
			break
		}
	}

	segmentIDs := make([]int, 0, len(segmentsByID))
	for segmentID := range segmentsByID {
		segmentIDs = append(segmentIDs, segmentID)
	}
	slices.Sort(segmentIDs)

	segments := make([]string, 0, len(segmentIDs)+len(trailingSegments))
	for _, segmentID := range segmentIDs {
		if segment := strings.TrimSpace(segmentsByID[segmentID]); segment != "" {
			segments = append(segments, segment)
		}
	}
	segments = append(segments, trailingSegments...)
	text := strings.Join(segments, "")

	return TranscriptionResult{
		Text:           text,
		Segments:       append([]string(nil), segments...),
		DurationMs:     pcm16DurationMs(req.AudioPCM, t.SampleRateHz, 1),
		Model:          "iflytek_rtasr",
		Device:         "cloud",
		Language:       firstNonEmpty(strings.TrimSpace(req.Language), strings.TrimSpace(t.Language)),
		EndpointReason: "last_segment",
		Partials:       append([]string(nil), partials...),
	}, nil
}

func (t IflytekRTASRTranscriber) validate(req TranscriptionRequest) error {
	if strings.TrimSpace(t.AppID) == "" || strings.TrimSpace(t.AccessKeyID) == "" || strings.TrimSpace(t.AccessKeySecret) == "" {
		return fmt.Errorf("iflytek rtasr credentials are incomplete")
	}
	if strings.TrimSpace(t.Host) == "" {
		t.Host = defaultIflytekRTASRHost
	}
	if strings.TrimSpace(t.Path) == "" {
		t.Path = defaultIflytekRTASRPath
	}
	if strings.TrimSpace(req.Codec) != "" && !strings.EqualFold(strings.TrimSpace(req.Codec), "pcm16le") {
		return fmt.Errorf("iflytek rtasr expects pcm16le input, got %s", req.Codec)
	}
	if req.Channels > 0 && req.Channels != 1 {
		return fmt.Errorf("iflytek rtasr expects mono input, got %d channels", req.Channels)
	}
	if req.SampleRateHz > 0 && req.SampleRateHz != t.SampleRateHz {
		return fmt.Errorf("iflytek rtasr expects %d Hz input, got %d", t.SampleRateHz, req.SampleRateHz)
	}
	if len(req.AudioPCM) == 0 {
		return fmt.Errorf("iflytek rtasr audio payload is empty")
	}
	return nil
}

func buildIflytekRTASRQuery(appID, accessKeyID, accessKeySecret, audioEncode, language, sampleRate string) string {
	params := map[string]string{
		"accessKeyId":  accessKeyID,
		"appId":        appID,
		"audio_encode": audioEncode,
		"lang":         language,
		"samplerate":   sampleRate,
		"utc":          beijingTimeNow().Format("2006-01-02T15:04:05-0700"),
		"uuid":         strings.ReplaceAll(strconv.FormatInt(time.Now().UnixNano(), 16), "-", ""),
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	encodedPairs := make([]string, 0, len(keys))
	for _, key := range keys {
		encodedPairs = append(encodedPairs, url.QueryEscape(key)+"="+url.QueryEscape(params[key]))
	}
	signature := base64.StdEncoding.EncodeToString(signHMACSHA1(accessKeySecret, strings.Join(encodedPairs, "&")))
	queryValues := url.Values{}
	for _, key := range keys {
		queryValues.Set(key, params[key])
	}
	queryValues.Set("signature", signature)
	return queryValues.Encode()
}

func buildIflytekRTASRURL(scheme, host string, port int, path, rawQuery string) string {
	host = firstNonEmpty(strings.TrimSpace(host), defaultIflytekRTASRHost)
	path = firstNonEmpty(strings.TrimSpace(path), defaultIflytekRTASRPath)
	if port <= 0 {
		if scheme == "wss" {
			port = 443
		} else {
			port = 80
		}
	}
	urlValue := url.URL{
		Scheme:   firstNonEmpty(strings.TrimSpace(scheme), defaultIflytekRTASRScheme),
		Host:     host,
		Path:     path,
		RawQuery: rawQuery,
	}
	if (urlValue.Scheme == "ws" && port != 80) || (urlValue.Scheme == "wss" && port != 443) {
		urlValue.Host = fmt.Sprintf("%s:%d", host, port)
	}
	return urlValue.String()
}

func writeIflytekRTASRAudio(ctx context.Context, conn *websocket.Conn, audioPCM []byte, frameBytes int, frameInterval time.Duration) error {
	if frameBytes <= 0 {
		frameBytes = defaultIflytekRTASRFrameBytes
	}
	if frameInterval < 0 {
		frameInterval = 0
	}
	startedAt := time.Now()
	frameIndex := 0
	for offset := 0; offset < len(audioPCM); offset += frameBytes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if frameInterval > 0 {
			expectedAt := startedAt.Add(time.Duration(frameIndex) * frameInterval)
			if wait := time.Until(expectedAt); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		}
		end := offset + frameBytes
		if end > len(audioPCM) {
			end = len(audioPCM)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, audioPCM[offset:end]); err != nil {
			return err
		}
		frameIndex++
	}
	return nil
}

func extractIflytekRTASRText(message map[string]any) string {
	data := asMap(message["data"])
	if directText, _ := data["text"].(string); strings.TrimSpace(directText) != "" {
		return strings.TrimSpace(directText)
	}
	cn := asMap(data["cn"])
	st := asMap(cn["st"])
	rtItems, _ := st["rt"].([]any)
	var parts []string
	for _, rtItem := range rtItems {
		wsItems, _ := asMap(rtItem)["ws"].([]any)
		for _, wsItem := range wsItems {
			cwItems, _ := asMap(wsItem)["cw"].([]any)
			for _, cwItem := range cwItems {
				token, _ := asMap(cwItem)["w"].(string)
				if strings.TrimSpace(token) != "" {
					parts = append(parts, token)
					break
				}
			}
		}
	}
	return strings.Join(parts, "")
}

func extractIflytekRTASRSegmentID(message map[string]any) (int, bool) {
	data := asMap(message["data"])
	return asInt(data["seg_id"])
}

func extractIflytekRTASRLastSegment(message map[string]any) bool {
	data := asMap(message["data"])
	value, _ := data["ls"].(bool)
	return value
}

func beijingTimeNow() time.Time {
	return time.Now().In(time.FixedZone("UTC+8", 8*60*60))
}

func signHMACSHA1(secret, payload string) []byte {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func readJSONWebsocketMessage(conn *websocket.Conn) (map[string]any, error) {
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return nil, fmt.Errorf("unsupported websocket message type %d", messageType)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func asMap(value any) map[string]any {
	typed, _ := value.(map[string]any)
	return typed
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func pcm16DurationMs(audioPCM []byte, sampleRateHz, channels int) int {
	if sampleRateHz <= 0 || channels <= 0 || len(audioPCM) == 0 {
		return 0
	}
	samples := len(audioPCM) / 2 / channels
	if samples <= 0 {
		return 0
	}
	return samples * 1000 / sampleRateHz
}

func cloneWebsocketDialer(dialer *websocket.Dialer) *websocket.Dialer {
	cloned := *dialer
	return &cloned
}
