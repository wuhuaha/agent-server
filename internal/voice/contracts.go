package voice

import "context"

type ResponseDeltaKind string

const (
	ResponseDeltaKindText       ResponseDeltaKind = "text"
	ResponseDeltaKindToolCall   ResponseDeltaKind = "tool_call"
	ResponseDeltaKindToolResult ResponseDeltaKind = "tool_result"
)

type SessionInput struct {
	SessionID  string
	Codec      string
	SampleRate int
}

type AudioChunk struct {
	Sequence int64
	Bytes    []byte
	Final    bool
}

type Runtime interface {
	StartSession(context.Context, SessionInput) error
	IngestAudio(context.Context, string, AudioChunk) error
	EndSession(context.Context, string, string) error
}

type TurnRequest struct {
	SessionID       string
	DeviceID        string
	ClientType      string
	Text            string
	Metadata        map[string]string
	AudioPCM        []byte
	AudioBytes      int
	InputFrames     int
	InputCodec      string
	InputSampleRate int
	InputChannels   int
}

type ResponseDelta struct {
	Kind       ResponseDeltaKind
	Text       string
	ToolCallID string
	ToolName   string
	ToolStatus string
	ToolInput  string
	ToolOutput string
}

type TurnResponse struct {
	InputText   string
	Text        string
	Deltas      []ResponseDelta
	AudioChunks [][]byte
	AudioStream AudioStream
	EndSession  bool
	EndReason   string
	EndMessage  string
}

type Responder interface {
	Respond(context.Context, TurnRequest) (TurnResponse, error)
}

type ResponseDeltaSink interface {
	EmitResponseDelta(context.Context, ResponseDelta) error
}

type ResponseDeltaSinkFunc func(context.Context, ResponseDelta) error

func (f ResponseDeltaSinkFunc) EmitResponseDelta(ctx context.Context, delta ResponseDelta) error {
	return f(ctx, delta)
}

type StreamingResponder interface {
	RespondStream(context.Context, TurnRequest, ResponseDeltaSink) (TurnResponse, error)
}

type TranscriptionRequest struct {
	SessionID    string
	DeviceID     string
	AudioPCM     []byte
	Codec        string
	SampleRateHz int
	Channels     int
	Language     string
}

type TranscriptionResult struct {
	Text           string
	Segments       []string
	DurationMs     int
	Model          string
	Device         string
	Language       string
	Emotion        string
	SpeakerID      string
	AudioEvents    []string
	EndpointReason string
	Partials       []string
}

type Transcriber interface {
	Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error)
}

type SynthesisRequest struct {
	SessionID string
	DeviceID  string
	UserText  string
	Text      string
}

type SynthesisResult struct {
	AudioPCM     []byte
	SampleRateHz int
	Channels     int
	Codec        string
	Voice        string
	Model        string
}

type Synthesizer interface {
	Synthesize(context.Context, SynthesisRequest) (SynthesisResult, error)
}

type AudioStream interface {
	Next(context.Context) ([]byte, error)
	Close() error
}

type StreamingSynthesizer interface {
	Synthesizer
	StreamSynthesize(context.Context, SynthesisRequest) (AudioStream, error)
}
