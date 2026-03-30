package voice

import "context"

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
	Text            string
	AudioPCM        []byte
	AudioBytes      int
	InputFrames     int
	InputCodec      string
	InputSampleRate int
	InputChannels   int
}

type TurnResponse struct {
	Text        string
	AudioChunks [][]byte
	AudioStream AudioStream
}

type Responder interface {
	Respond(context.Context, TurnRequest) (TurnResponse, error)
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
	Text       string
	Segments   []string
	DurationMs int
	Model      string
	Device     string
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
