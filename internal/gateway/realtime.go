package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type audioProfile struct {
	Codec        string `json:"codec"`
	SampleRateHz int    `json:"sample_rate_hz"`
	Channels     int    `json:"channels"`
}

type capabilities struct {
	AllowOpus       bool `json:"allow_opus"`
	AllowTextInput  bool `json:"allow_text_input"`
	AllowImageInput bool `json:"allow_image_input"`
}

type RealtimeProfile struct {
	Logger                *slog.Logger
	WSPath                string
	ProtocolVersion       string
	Subprotocol           string
	LLMProvider           string
	VoiceProvider         string
	TTSProvider           string
	ServerEndpointEnabled bool
	BargeInMinAudioMs     int
	BargeInHoldAudioMs    int
	AuthMode              string
	TurnMode              string
	IdleTimeoutMs         int
	MaxSessionMs          int
	MaxFrameBytes         int
	InputCodec            string
	InputSampleRate       int
	InputChannels         int
	OutputCodec           string
	OutputSampleRate      int
	OutputChannels        int
	AllowOpus             bool
	AllowTextInput        bool
	AllowImageInput       bool
}

type realtimeInfo struct {
	Status           string       `json:"status"`
	Transport        string       `json:"transport"`
	ProtocolVersion  string       `json:"protocol_version"`
	WSPath           string       `json:"ws_path"`
	Subprotocol      string       `json:"subprotocol"`
	LLMProvider      string       `json:"llm_provider,omitempty"`
	VoiceProvider    string       `json:"voice_provider,omitempty"`
	TTSProvider      string       `json:"tts_provider,omitempty"`
	AuthMode         string       `json:"auth_mode"`
	TurnMode         string       `json:"turn_mode"`
	InputAudio       audioProfile `json:"input_audio"`
	OutputAudio      audioProfile `json:"output_audio"`
	Capabilities     capabilities `json:"capabilities"`
	IdleTimeoutMs    int          `json:"idle_timeout_ms"`
	MaxSessionMs     int          `json:"max_session_ms"`
	MaxFrameBytes    int          `json:"max_frame_bytes"`
	EventSchema      string       `json:"event_schema"`
	StartSchema      string       `json:"start_schema"`
	ProtocolDoc      string       `json:"protocol_doc"`
	DeviceProfileDoc string       `json:"device_profile_doc"`
	Notes            []string     `json:"notes"`
}

func NewRealtimeHandler(profile RealtimeProfile) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
				"error": "only GET is implemented in the bootstrap phase",
			})
			return
		}

		writeJSON(w, http.StatusOK, realtimeInfo{
			Status:          "bootstrap",
			Transport:       "websocket + binary audio + json control",
			ProtocolVersion: profile.ProtocolVersion,
			WSPath:          profile.WSPath,
			Subprotocol:     profile.Subprotocol,
			LLMProvider:     profile.LLMProvider,
			VoiceProvider:   profile.VoiceProvider,
			TTSProvider:     profile.TTSProvider,
			AuthMode:        profile.AuthMode,
			TurnMode:        profile.TurnMode,
			InputAudio: audioProfile{
				Codec:        profile.InputCodec,
				SampleRateHz: profile.InputSampleRate,
				Channels:     profile.InputChannels,
			},
			OutputAudio: audioProfile{
				Codec:        profile.OutputCodec,
				SampleRateHz: profile.OutputSampleRate,
				Channels:     profile.OutputChannels,
			},
			Capabilities: capabilities{
				AllowOpus:       profile.AllowOpus,
				AllowTextInput:  profile.AllowTextInput,
				AllowImageInput: profile.AllowImageInput,
			},
			IdleTimeoutMs:    profile.IdleTimeoutMs,
			MaxSessionMs:     profile.MaxSessionMs,
			MaxFrameBytes:    profile.MaxFrameBytes,
			EventSchema:      "schemas/realtime/session-envelope.schema.json",
			StartSchema:      "schemas/realtime/device-session-start.schema.json",
			ProtocolDoc:      "docs/protocols/realtime-session-v0.md",
			DeviceProfileDoc: "docs/protocols/rtos-device-ws-v0.md",
			Notes: []string{
				"session.start must be sent before the first inbound binary audio frame",
				"user turns currently complete only after explicit audio.in.commit or text.in",
				"client and server may both terminate the dialog with session.end",
				"bootstrap websocket handler is available at the advertised ws_path",
				"idle timeout is enforced only while the session is active",
				"new inbound audio or session.update interrupt=true can barge into speaking state",
				"inbound audio barge-in now uses a shared adaptive threshold instead of interrupting on the very first frame",
			},
		})
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(payload)
}
