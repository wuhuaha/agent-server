package channel

import "context"

type Attachment struct {
	Name string
	URL  string
	MIME string
}

type InboundMessage struct {
	Channel   string
	UserID    string
	ThreadID  string
	Text      string
	SessionID string
	Files     []Attachment
}

type NormalizedInput struct {
	SessionKey  string
	UserText    string
	Attachments []Attachment
}

type OutboundMessage struct {
	Channel   string
	ThreadID  string
	SessionID string
	Text      string
}

type Adapter interface {
	Name() string
	Normalize(context.Context, InboundMessage) (NormalizedInput, error)
	Deliver(context.Context, OutboundMessage) error
}
