package gateway

type XiaozhiCompatProfile struct {
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
	SourceOutputCodec     string
	SourceOutputRate      int
	SourceOutputChannels  int
	OutputCodec           string
	OutputSampleRate      int
	OutputChannels        int
	OutputFrameDurationMs int
}
