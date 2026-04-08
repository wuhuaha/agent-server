package control

import "net/http"

type RealtimeProfile struct {
	ProtocolVersion string `json:"protocol_version"`
	WSPath          string `json:"ws_path"`
	Subprotocol     string `json:"subprotocol"`
	LLMProvider     string `json:"llm_provider,omitempty"`
	AuthMode        string `json:"auth_mode"`
	TurnMode        string `json:"turn_mode"`
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
