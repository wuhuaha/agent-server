package events

import "time"

type Type string

const (
	TypeSessionStart      Type = "session.start"
	TypeSessionUpdate     Type = "session.update"
	TypeAudioInAppend     Type = "audio.in.append"
	TypeAudioInCommit     Type = "audio.in.commit"
	TypeTextIn            Type = "text.in"
	TypeImageIn           Type = "image.in"
	TypeInputSpeechStart  Type = "input.speech.start"
	TypeInputPreview      Type = "input.preview"
	TypeInputEndpoint     Type = "input.endpoint"
	TypeResponseStart     Type = "response.start"
	TypeResponseChunk     Type = "response.chunk"
	TypeAudioOutMeta      Type = "audio.out.meta"
	TypeAudioOutChunk     Type = "audio.out.chunk"
	TypeAudioOutStarted   Type = "audio.out.started"
	TypeAudioOutMark      Type = "audio.out.mark"
	TypeAudioOutCleared   Type = "audio.out.cleared"
	TypeAudioOutCompleted Type = "audio.out.completed"
	TypeSessionEnd        Type = "session.end"
	TypeError             Type = "error"
)

type Envelope struct {
	Type      Type           `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	Seq       int64          `json:"seq,omitempty"`
	Timestamp time.Time      `json:"ts"`
	Payload   any            `json:"payload,omitempty"`
	Auth      map[string]any `json:"auth,omitempty"`
	Device    map[string]any `json:"device,omitempty"`
	Trace     map[string]any `json:"trace,omitempty"`
}

func New(eventType Type, sessionID string, seq int64, payload any) Envelope {
	return Envelope{
		Type:      eventType,
		SessionID: sessionID,
		Seq:       seq,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}
