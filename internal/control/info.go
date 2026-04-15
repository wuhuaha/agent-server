package control

import "net/http"

type RealtimeProfile struct {
	ProtocolVersion                string                `json:"protocol_version"`
	WSPath                         string                `json:"ws_path"`
	Subprotocol                    string                `json:"subprotocol"`
	LLMProvider                    string                `json:"llm_provider,omitempty"`
	AuthMode                       string                `json:"auth_mode"`
	TurnMode                       string                `json:"turn_mode"`
	ServerEndpoint                 ServerEndpointProfile `json:"server_endpoint"`
	ServerEndpointAvailable        bool                  `json:"-"`
	ServerEndpointEnabled          bool                  `json:"-"`
	ServerEndpointMinAudioMs       int                   `json:"-"`
	ServerEndpointSilenceMs        int                   `json:"-"`
	ServerEndpointLexicalMode      string                `json:"-"`
	ServerEndpointIncompleteHoldMs int                   `json:"-"`
	ServerEndpointHintSilenceMs    int                   `json:"-"`
}

type ServerEndpointProfile struct {
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

type infoResponse struct {
	Service         string          `json:"service"`
	Environment     string          `json:"environment"`
	Version         string          `json:"version"`
	Priorities      []string        `json:"priorities"`
	Protocol        string          `json:"protocol"`
	RealtimeProfile RealtimeProfile `json:"realtime_profile"`
}

func NewInfoHandler(serviceName, environment, version string, profile RealtimeProfile) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		profile.ServerEndpoint = profile.ServerEndpointProfile()
		writeJSON(w, http.StatusOK, infoResponse{
			Service:     serviceName,
			Environment: environment,
			Version:     version,
			Priorities: []string{
				"architecture",
				"rtos-voice-fast-path",
				"agent-runtime-core",
				"channel-skill-extensibility",
				"security-backfill",
			},
			Protocol:        "realtime-session-v0",
			RealtimeProfile: profile,
		})
	})
}

func (p RealtimeProfile) ServerEndpointProfile() ServerEndpointProfile {
	profile := ServerEndpointProfile{
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
