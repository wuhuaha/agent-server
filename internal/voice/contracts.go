package voice

import (
	"context"
	"time"
)

type ResponseDeltaKind string
type TranscriptionDeltaKind string

const (
	ResponseDeltaKindText       ResponseDeltaKind = "text"
	ResponseDeltaKindToolCall   ResponseDeltaKind = "tool_call"
	ResponseDeltaKindToolResult ResponseDeltaKind = "tool_result"

	TranscriptionDeltaKindSpeechStart TranscriptionDeltaKind = "speech_start"
	TranscriptionDeltaKindPartial     TranscriptionDeltaKind = "partial"
	TranscriptionDeltaKindSpeechEnd   TranscriptionDeltaKind = "speech_end"
	TranscriptionDeltaKindFinal       TranscriptionDeltaKind = "final"
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
	TurnID          string
	TraceID         string
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
	// AudioStreamTransferred marks that audio ownership moved onto the
	// orchestrated early-audio path and should not be started again from the
	// final response envelope.
	AudioStreamTransferred bool
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

type ResponseAudioStartSource string

const (
	ResponseAudioStartSourceSpeechPlanner ResponseAudioStartSource = "speech_planner"
	ResponseAudioStartSourceFinalResponse ResponseAudioStartSource = "final_response"
)

type ResponseAudioStart struct {
	// Stream ownership transfers to the caller when returned from WaitAudioStart.
	Stream      AudioStream
	Text        string
	Incremental bool
	Source      ResponseAudioStartSource
}

// TurnResponseFuture separates early audio startup from the final response
// envelope so the gateway can begin speaking before turn execution fully
// settles.
type TurnResponseFuture interface {
	Wait(context.Context) (TurnResponse, error)
	WaitAudioStart(context.Context) (ResponseAudioStart, bool, error)
}

type OrchestratingResponder interface {
	RespondOrchestrated(context.Context, TurnRequest, ResponseDeltaSink) (TurnResponseFuture, error)
}

type TranscriptionRequest struct {
	SessionID    string
	TurnID       string
	TraceID      string
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
	ElapsedMs      int
	Model          string
	Device         string
	Language       string
	Emotion        string
	SpeakerID      string
	AudioEvents    []string
	EndpointReason string
	Partials       []string
	Mode           string
}

type Transcriber interface {
	Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error)
}

type TranscriptionDelta struct {
	Kind           TranscriptionDeltaKind
	Text           string
	EndpointReason string
	AudioBytes     int
}

type TranscriptionDeltaSink interface {
	EmitTranscriptionDelta(context.Context, TranscriptionDelta) error
}

type TranscriptionDeltaSinkFunc func(context.Context, TranscriptionDelta) error

func (f TranscriptionDeltaSinkFunc) EmitTranscriptionDelta(ctx context.Context, delta TranscriptionDelta) error {
	return f(ctx, delta)
}

type StreamingTranscriptionSession interface {
	PushAudio(context.Context, []byte) error
	Finish(context.Context) (TranscriptionResult, error)
	Close() error
}

type StreamingTranscriber interface {
	Transcriber
	StartStream(context.Context, TranscriptionRequest, TranscriptionDeltaSink) (StreamingTranscriptionSession, error)
}

type InputPreviewRequest struct {
	SessionID    string
	DeviceID     string
	Codec        string
	SampleRateHz int
	Channels     int
	Language     string
}

type InputPreview struct {
	PartialText     string
	EndpointReason  string
	AudioBytes      int
	CommitSuggested bool
	SpeechStarted   bool
}

type InputPreviewSession interface {
	PushAudio(context.Context, []byte) (InputPreview, error)
	Poll(time.Time) InputPreview
	Close() error
}

type InputPreviewer interface {
	StartInputPreview(context.Context, InputPreviewRequest) (InputPreviewSession, error)
}

type SynthesisRequest struct {
	SessionID string
	TurnID    string
	TraceID   string
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
