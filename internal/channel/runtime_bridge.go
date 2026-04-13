package channel

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"agent-server/internal/agent"
)

type DeliveryState string

const (
	DeliveryStateDelivered DeliveryState = "delivered"
	DeliveryStateSkipped   DeliveryState = "skipped"
	DeliveryStateFailed    DeliveryState = "failed"
)

type DeliveryStatus struct {
	AdapterName    string
	Channel        string
	SessionID      string
	ThreadID       string
	MessageID      string
	TurnID         string
	TraceID        string
	Attempt        int
	IdempotencyKey string
	State          DeliveryState
	FailureStage   string
	Error          string
}

type DeliveryReporter interface {
	ReportDelivery(context.Context, DeliveryStatus) error
}

type DeliveryReporterFunc func(context.Context, DeliveryStatus) error

func (f DeliveryReporterFunc) ReportDelivery(ctx context.Context, status DeliveryStatus) error {
	return f(ctx, status)
}

type NoopDeliveryReporter struct{}

func (NoopDeliveryReporter) ReportDelivery(context.Context, DeliveryStatus) error {
	return nil
}

type RuntimeHandoff struct {
	AdapterName string
	Inbound     InboundMessage
	Normalized  NormalizedInput
	TurnInput   agent.TurnInput
	TurnOutput  agent.TurnOutput
	Outbound    OutboundMessage
}

type RuntimeBridge struct {
	Adapter  Adapter
	Executor agent.TurnExecutor
	Reporter DeliveryReporter
}

func (b RuntimeBridge) HandleInbound(ctx context.Context, inbound InboundMessage) (RuntimeHandoff, error) {
	adapterName, err := b.validate()
	if err != nil {
		return RuntimeHandoff{}, err
	}

	handoff := RuntimeHandoff{
		AdapterName: adapterName,
		Inbound:     cloneInboundMessage(inbound),
	}
	reporter := b.deliveryReporter()

	normalized, err := b.Adapter.Normalize(ctx, inbound)
	if err != nil {
		status := statusFromInbound(adapterName, inbound)
		status.State = DeliveryStateFailed
		status.FailureStage = "normalize"
		status.Error = err.Error()
		reporter.ReportDelivery(ctx, status)
		return handoff, fmt.Errorf("normalize inbound channel message: %w", err)
	}

	handoff.Normalized = cloneNormalizedInput(normalized)
	handoff.TurnInput = buildTurnInput(adapterName, inbound, normalized)

	output, err := b.Executor.ExecuteTurn(ctx, handoff.TurnInput)
	if err != nil {
		status := statusFromTurnInput(adapterName, inbound, handoff.TurnInput)
		status.State = DeliveryStateFailed
		status.FailureStage = "execute"
		status.Error = err.Error()
		reporter.ReportDelivery(ctx, status)
		return handoff, fmt.Errorf("execute channel turn: %w", err)
	}

	handoff.TurnOutput = cloneTurnOutput(output)
	handoff.Outbound = buildOutboundMessage(adapterName, inbound, normalized, handoff.TurnInput, output)

	if strings.TrimSpace(handoff.Outbound.Text) == "" {
		status := statusFromTurnInput(adapterName, inbound, handoff.TurnInput)
		status.State = DeliveryStateSkipped
		reporter.ReportDelivery(ctx, status)
		return handoff, nil
	}

	if err := b.Adapter.Deliver(ctx, handoff.Outbound); err != nil {
		status := statusFromTurnInput(adapterName, inbound, handoff.TurnInput)
		status.State = DeliveryStateFailed
		status.FailureStage = "deliver"
		status.Error = err.Error()
		reporter.ReportDelivery(ctx, status)
		return handoff, fmt.Errorf("deliver outbound channel message: %w", err)
	}

	status := statusFromTurnInput(adapterName, inbound, handoff.TurnInput)
	status.State = DeliveryStateDelivered
	reporter.ReportDelivery(ctx, status)
	return handoff, nil
}

func (b RuntimeBridge) validate() (string, error) {
	if b.Adapter == nil {
		return "", errors.New("channel runtime bridge requires an adapter")
	}
	if b.Executor == nil {
		return "", errors.New("channel runtime bridge requires a turn executor")
	}
	name := strings.TrimSpace(b.Adapter.Name())
	if name == "" {
		return "", errors.New("channel runtime bridge requires adapter.Name()")
	}
	return name, nil
}

func (b RuntimeBridge) deliveryReporter() DeliveryReporter {
	if b.Reporter == nil {
		return NoopDeliveryReporter{}
	}
	return b.Reporter
}

func buildTurnInput(adapterName string, inbound InboundMessage, normalized NormalizedInput) agent.TurnInput {
	sessionID := firstNonEmpty(
		normalized.SessionKey,
		inbound.SessionID,
		normalized.ThreadKey,
		inbound.ThreadID,
		inbound.UserID,
	)
	messageKey := firstNonEmpty(normalized.MessageKey, inbound.MessageID, inbound.IdempotencyKey)
	threadKey := firstNonEmpty(normalized.ThreadKey, inbound.ThreadID, sessionID)
	metadata := buildTurnMetadata(adapterName, inbound, normalized, sessionID, threadKey, messageKey)
	attachments := normalized.Attachments
	if len(attachments) == 0 {
		attachments = inbound.Files
	}

	return agent.TurnInput{
		SessionID:  sessionID,
		TurnID:     messageKey,
		TraceID:    buildTraceID(adapterName, sessionID, messageKey),
		ClientType: "channel:" + adapterName,
		UserText:   strings.TrimSpace(firstNonEmpty(normalized.UserText, inbound.Text)),
		Images:     attachmentsToImages(attachments),
		Metadata:   metadata,
	}
}

func buildOutboundMessage(adapterName string, inbound InboundMessage, normalized NormalizedInput, turnInput agent.TurnInput, output agent.TurnOutput) OutboundMessage {
	text := strings.TrimSpace(output.Text)
	if text == "" {
		text = strings.TrimSpace(output.EndMessage)
	}
	return OutboundMessage{
		Channel:        firstNonEmpty(inbound.Channel, adapterName),
		ThreadID:       firstNonEmpty(normalized.ThreadKey, inbound.ThreadID, turnInput.SessionID),
		SessionID:      turnInput.SessionID,
		InReplyTo:      firstNonEmpty(inbound.MessageID, normalized.MessageKey),
		IdempotencyKey: firstNonEmpty(normalized.MessageKey, inbound.IdempotencyKey, inbound.MessageID, turnInput.TurnID),
		Text:           text,
		Metadata: map[string]string{
			"channel.name":      firstNonEmpty(inbound.Channel, adapterName),
			"channel.thread_id": firstNonEmpty(normalized.ThreadKey, inbound.ThreadID, turnInput.SessionID),
			"channel.turn_id":   turnInput.TurnID,
			"channel.trace_id":  turnInput.TraceID,
		},
	}
}

func buildTurnMetadata(adapterName string, inbound InboundMessage, normalized NormalizedInput, sessionID, threadKey, messageKey string) map[string]string {
	metadata := cloneStringMap(inbound.Metadata)
	if metadata == nil {
		metadata = map[string]string{}
	}
	for key, value := range normalized.Metadata {
		metadata[key] = value
	}
	setIfEmpty(metadata, "user_id", inbound.UserID)
	setIfEmpty(metadata, "memory.user_id", inbound.UserID)
	setIfEmpty(metadata, "channel.name", firstNonEmpty(inbound.Channel, adapterName))
	setIfEmpty(metadata, "channel.thread_id", threadKey)
	setIfEmpty(metadata, "channel.session_id", sessionID)
	setIfEmpty(metadata, "channel.message_id", firstNonEmpty(inbound.MessageID, messageKey))
	setIfEmpty(metadata, "channel.idempotency_key", firstNonEmpty(inbound.IdempotencyKey, messageKey, inbound.MessageID))
	if inbound.RetryAttempt > 0 {
		setIfEmpty(metadata, "channel.retry_attempt", strconv.Itoa(inbound.RetryAttempt))
	}
	return metadata
}

func buildTraceID(adapterName, sessionID, messageKey string) string {
	parts := []string{"channel", strings.TrimSpace(adapterName)}
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if trimmed := strings.TrimSpace(messageKey); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "/")
}

func attachmentsToImages(attachments []Attachment) []agent.ImageInput {
	images := make([]agent.ImageInput, 0, len(attachments))
	for _, attachment := range attachments {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(attachment.MIME)), "image/") {
			continue
		}
		images = append(images, agent.ImageInput{
			Name: attachment.Name,
			URL:  attachment.URL,
			MIME: attachment.MIME,
		})
	}
	return images
}

func statusFromInbound(adapterName string, inbound InboundMessage) DeliveryStatus {
	return DeliveryStatus{
		AdapterName:    adapterName,
		Channel:        firstNonEmpty(inbound.Channel, adapterName),
		SessionID:      firstNonEmpty(inbound.SessionID, inbound.ThreadID, inbound.UserID),
		ThreadID:       inbound.ThreadID,
		MessageID:      inbound.MessageID,
		Attempt:        inbound.RetryAttempt,
		IdempotencyKey: firstNonEmpty(inbound.IdempotencyKey, inbound.MessageID),
	}
}

func statusFromTurnInput(adapterName string, inbound InboundMessage, turnInput agent.TurnInput) DeliveryStatus {
	status := statusFromInbound(adapterName, inbound)
	status.SessionID = firstNonEmpty(turnInput.SessionID, status.SessionID)
	status.TurnID = turnInput.TurnID
	status.TraceID = turnInput.TraceID
	return status
}

func cloneInboundMessage(inbound InboundMessage) InboundMessage {
	cloned := inbound
	cloned.Files = append([]Attachment(nil), inbound.Files...)
	cloned.Metadata = cloneStringMap(inbound.Metadata)
	return cloned
}

func cloneNormalizedInput(normalized NormalizedInput) NormalizedInput {
	cloned := normalized
	cloned.Attachments = append([]Attachment(nil), normalized.Attachments...)
	cloned.Metadata = cloneStringMap(normalized.Metadata)
	return cloned
}

func cloneTurnOutput(output agent.TurnOutput) agent.TurnOutput {
	cloned := output
	cloned.Deltas = append([]agent.TurnDelta(nil), output.Deltas...)
	return cloned
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

func setIfEmpty(metadata map[string]string, key, value string) {
	if metadata == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return
	}
	if strings.TrimSpace(metadata[key]) != "" {
		return
	}
	metadata[key] = value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
