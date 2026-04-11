package gateway

import "log/slog"

type XiaozhiCompatProfile struct {
	Logger                *slog.Logger
	Enabled               bool
	WSPath                string
	OTAPath               string
	WelcomeVersion        int
	WelcomeTransport      string
	InputCodec            string
	InputSampleRate       int
	InputChannels         int
	InputFrameDurationMs  int
	MaxFrameBytes         int
	IdleTimeoutMs         int
	MaxSessionMs          int
	ServerEndpointEnabled bool
	SourceOutputCodec     string
	SourceOutputRate      int
	SourceOutputChannels  int
	OutputCodec           string
	OutputSampleRate      int
	OutputChannels        int
	OutputFrameDurationMs int
}
