package channel

import "context"

type Attachment struct {
	Name string
	URL  string
	MIME string
}

type InboundMessage struct {
	Channel        string
	MessageID      string
	UserID         string
	ThreadID       string
	Text           string
	SessionID      string
	RetryAttempt   int
	IdempotencyKey string
	Files          []Attachment
	Metadata       map[string]string
}

type NormalizedInput struct {
	SessionKey  string
	ThreadKey   string
	MessageKey  string
	UserText    string
	Attachments []Attachment
	Metadata    map[string]string
}

type OutboundMessage struct {
	Channel        string
	ThreadID       string
	SessionID      string
	InReplyTo      string
	IdempotencyKey string
	Text           string
	Metadata       map[string]string
}

type Adapter interface {
	Name() string
	Normalize(context.Context, InboundMessage) (NormalizedInput, error)
	Deliver(context.Context, OutboundMessage) error
}
