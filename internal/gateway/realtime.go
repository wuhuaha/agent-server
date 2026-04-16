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

type serverEndpointProfile struct {
	Available                bool   `json:"available"`
	MainPathCandidate        bool   `json:"main_path_candidate"`
	Enabled                  bool   `json:"enabled"`
	Mode                     string `json:"mode"`
	ClientCommitCompatible   bool   `json:"client_commit_compatible"`
	RequiresStreamingPreview bool   `json:"requires_streaming_preview"`
	MinAudioMs               int    `json:"min_audio_ms,omitempty"`
	SilenceMs                int    `json:"silence_ms,omitempty"`
	HintSilenceMs            int    `json:"hint_silence_ms,omitempty"`
	IncompleteHoldMs         int    `json:"incomplete_hold_ms,omitempty"`
	LexicalMode              string `json:"lexical_mode,omitempty"`
}

type RealtimeProfile struct {
	Logger                         *slog.Logger
	WSPath                         string
	ProtocolVersion                string
	Subprotocol                    string
	LLMProvider                    string
	VoiceProvider                  string
	TTSProvider                    string
	ServerEndpoint                 serverEndpointProfile
	ServerEndpointAvailable        bool
	ServerEndpointEnabled          bool
	ServerEndpointMinAudioMs       int
	ServerEndpointSilenceMs        int
	ServerEndpointLexicalMode      string
	ServerEndpointIncompleteHoldMs int
	ServerEndpointHintSilenceMs    int
	BargeInMinAudioMs              int
	BargeInHoldAudioMs             int
	AuthMode                       string
	TurnMode                       string
	IdleTimeoutMs                  int
	MaxSessionMs                   int
	MaxFrameBytes                  int
	InputCodec                     string
	InputSampleRate                int
	InputChannels                  int
	OutputCodec                    string
	OutputSampleRate               int
	OutputChannels                 int
	AllowOpus                      bool
	AllowTextInput                 bool
	AllowImageInput                bool
}

type realtimeInfo struct {
	Status             string                    `json:"status"`
	Transport          string                    `json:"transport"`
	ProtocolVersion    string                    `json:"protocol_version"`
	WSPath             string                    `json:"ws_path"`
	Subprotocol        string                    `json:"subprotocol"`
	LLMProvider        string                    `json:"llm_provider,omitempty"`
	VoiceProvider      string                    `json:"voice_provider,omitempty"`
	TTSProvider        string                    `json:"tts_provider,omitempty"`
	AuthMode           string                    `json:"auth_mode"`
	TurnMode           string                    `json:"turn_mode"`
	ServerEndpoint     serverEndpointProfile     `json:"server_endpoint"`
	VoiceCollaboration voiceCollaborationProfile `json:"voice_collaboration"`
	InputAudio         audioProfile              `json:"input_audio"`
	OutputAudio        audioProfile              `json:"output_audio"`
	Capabilities       capabilities              `json:"capabilities"`
	IdleTimeoutMs      int                       `json:"idle_timeout_ms"`
	MaxSessionMs       int                       `json:"max_session_ms"`
	MaxFrameBytes      int                       `json:"max_frame_bytes"`
	EventSchema        string                    `json:"event_schema"`
	StartSchema        string                    `json:"start_schema"`
	ProtocolDoc        string                    `json:"protocol_doc"`
	DeviceProfileDoc   string                    `json:"device_profile_doc"`
	Notes              []string                  `json:"notes"`
}

func NewRealtimeHandler(profile RealtimeProfile) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
				"error": "only GET is implemented in the bootstrap phase",
			})
			return
		}

		profile.ServerEndpoint = profile.serverEndpointProfile()

		writeJSON(w, http.StatusOK, realtimeInfo{
			Status:             "bootstrap",
			Transport:          "websocket + binary audio + json control",
			ProtocolVersion:    profile.ProtocolVersion,
			WSPath:             profile.WSPath,
			Subprotocol:        profile.Subprotocol,
			LLMProvider:        profile.LLMProvider,
			VoiceProvider:      profile.VoiceProvider,
			TTSProvider:        profile.TTSProvider,
			AuthMode:           profile.AuthMode,
			TurnMode:           profile.TurnMode,
			ServerEndpoint:     profile.ServerEndpoint,
			VoiceCollaboration: profile.voiceCollaborationProfile(),
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
			Notes:            discoveryNotes(profile),
		})
	})
}

func (p RealtimeProfile) serverEndpointProfile() serverEndpointProfile {
	profile := serverEndpointProfile{
		Available:              p.ServerEndpointAvailable,
		MainPathCandidate:      p.ServerEndpointAvailable,
		Enabled:                p.ServerEndpointEnabled,
		ClientCommitCompatible: true,
	}
	switch {
	case !p.ServerEndpointAvailable:
		profile.Mode = "unsupported"
	default:
		profile.RequiresStreamingPreview = true
		profile.MinAudioMs = p.ServerEndpointMinAudioMs
		profile.SilenceMs = p.ServerEndpointSilenceMs
		profile.HintSilenceMs = p.ServerEndpointHintSilenceMs
		profile.IncompleteHoldMs = p.ServerEndpointIncompleteHoldMs
		profile.LexicalMode = p.ServerEndpointLexicalMode
		if p.ServerEndpointEnabled {
			profile.Mode = "server_vad_assisted"
		} else {
			profile.Mode = "client_commit_only"
		}
	}
	return profile
}

func discoveryNotes(profile RealtimeProfile) []string {
	notes := []string{
		"session.start must be sent before the first inbound binary audio frame",
		"client and server may both terminate the dialog with session.end",
		"bootstrap websocket handler is available at the advertised ws_path",
		"idle timeout is enforced only while the session is active",
		"new inbound audio or session.update interrupt=true can barge into speaking state",
		"inbound audio barge-in now uses a shared adaptive threshold instead of interrupting on the very first frame",
		"clients may negotiate preview_events and playback_ack through session.start capabilities when voice_collaboration advertises them",
	}
	switch {
	case profile.ServerEndpointEnabled:
		notes = append(notes, "audio turns may auto-commit through the shared server-endpoint candidate after preview silence; explicit audio.in.commit stays supported")
	case profile.ServerEndpointAvailable:
		notes = append(notes, "shared server-endpointing is now a main-path candidate behind AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED, while explicit audio.in.commit remains the default")
	default:
		notes = append(notes, "user turns currently complete only after explicit audio.in.commit or text.in")
	}
	return notes
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(payload)
}
