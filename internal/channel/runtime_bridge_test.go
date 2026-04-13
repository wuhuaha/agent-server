package channel

import (
	"context"
	"errors"
	"testing"

	"agent-server/internal/agent"
)

type stubAdapter struct {
	name         string
	normalized   NormalizedInput
	normalizeErr error
	deliverErr   error
	delivered    []OutboundMessage
}

func (a *stubAdapter) Name() string {
	return a.name
}

func (a *stubAdapter) Normalize(context.Context, InboundMessage) (NormalizedInput, error) {
	return a.normalized, a.normalizeErr
}

func (a *stubAdapter) Deliver(_ context.Context, outbound OutboundMessage) error {
	a.delivered = append(a.delivered, outbound)
	return a.deliverErr
}

type stubExecutor struct {
	input  agent.TurnInput
	output agent.TurnOutput
	err    error
}

func (e *stubExecutor) ExecuteTurn(_ context.Context, input agent.TurnInput) (agent.TurnOutput, error) {
	e.input = input
	return e.output, e.err
}

type captureReporter struct {
	statuses []DeliveryStatus
}

func (r *captureReporter) ReportDelivery(_ context.Context, status DeliveryStatus) error {
	r.statuses = append(r.statuses, status)
	return nil
}

func TestRuntimeBridgeNormalizesHandsOffAndDelivers(t *testing.T) {
	adapter := &stubAdapter{
		name: "feishu",
		normalized: NormalizedInput{
			SessionKey: "session-42",
			ThreadKey:  "thread-mapped",
			MessageKey: "msg-normalized",
			UserText:   "帮我打开客厅主灯",
			Attachments: []Attachment{
				{Name: "photo.png", URL: "https://example.com/photo.png", MIME: "image/png"},
				{Name: "note.txt", URL: "https://example.com/note.txt", MIME: "text/plain"},
			},
			Metadata: map[string]string{
				"memory.household_id": "home-1",
			},
		},
	}
	executor := &stubExecutor{
		output: agent.TurnOutput{
			Text: "已经为你打开客厅主灯。",
		},
	}
	reporter := &captureReporter{}
	bridge := RuntimeBridge{
		Adapter:  adapter,
		Executor: executor,
		Reporter: reporter,
	}

	inbound := InboundMessage{
		Channel:        "feishu",
		MessageID:      "msg-raw",
		UserID:         "user-7",
		ThreadID:       "thread-raw",
		Text:           "原始文本",
		RetryAttempt:   2,
		IdempotencyKey: "idem-7",
		Metadata: map[string]string{
			"room_id": "living_room",
		},
	}

	handoff, err := bridge.HandleInbound(context.Background(), inbound)
	if err != nil {
		t.Fatalf("HandleInbound returned error: %v", err)
	}

	if handoff.AdapterName != "feishu" {
		t.Fatalf("expected adapter name feishu, got %q", handoff.AdapterName)
	}
	if executor.input.SessionID != "session-42" {
		t.Fatalf("expected session-42, got %q", executor.input.SessionID)
	}
	if executor.input.TurnID != "msg-normalized" {
		t.Fatalf("expected normalized turn id, got %q", executor.input.TurnID)
	}
	if executor.input.ClientType != "channel:feishu" {
		t.Fatalf("expected channel client type, got %q", executor.input.ClientType)
	}
	if executor.input.UserText != "帮我打开客厅主灯" {
		t.Fatalf("expected normalized user text, got %q", executor.input.UserText)
	}
	if len(executor.input.Images) != 1 || executor.input.Images[0].Name != "photo.png" {
		t.Fatalf("expected one image attachment, got %+v", executor.input.Images)
	}
	if executor.input.Metadata["memory.user_id"] != "user-7" {
		t.Fatalf("expected memory.user_id to be injected, got %q", executor.input.Metadata["memory.user_id"])
	}
	if executor.input.Metadata["channel.thread_id"] != "thread-mapped" {
		t.Fatalf("expected mapped thread id, got %q", executor.input.Metadata["channel.thread_id"])
	}
	if len(adapter.delivered) != 1 {
		t.Fatalf("expected one delivered outbound message, got %d", len(adapter.delivered))
	}
	if adapter.delivered[0].ThreadID != "thread-mapped" {
		t.Fatalf("expected outbound thread-mapped, got %q", adapter.delivered[0].ThreadID)
	}
	if adapter.delivered[0].InReplyTo != "msg-raw" {
		t.Fatalf("expected outbound reply target msg-raw, got %q", adapter.delivered[0].InReplyTo)
	}
	if adapter.delivered[0].Text != "已经为你打开客厅主灯。" {
		t.Fatalf("expected outbound text, got %q", adapter.delivered[0].Text)
	}
	if len(reporter.statuses) != 1 {
		t.Fatalf("expected one delivery status, got %d", len(reporter.statuses))
	}
	if reporter.statuses[0].State != DeliveryStateDelivered {
		t.Fatalf("expected delivered state, got %q", reporter.statuses[0].State)
	}
	if reporter.statuses[0].TurnID != "msg-normalized" {
		t.Fatalf("expected turn id msg-normalized, got %q", reporter.statuses[0].TurnID)
	}
}

func TestRuntimeBridgeReportsDeliverFailure(t *testing.T) {
	adapter := &stubAdapter{
		name: "slack",
		normalized: NormalizedInput{
			SessionKey: "session-9",
			UserText:   "hello",
		},
		deliverErr: errors.New("delivery offline"),
	}
	executor := &stubExecutor{
		output: agent.TurnOutput{Text: "world"},
	}
	reporter := &captureReporter{}
	bridge := RuntimeBridge{
		Adapter:  adapter,
		Executor: executor,
		Reporter: reporter,
	}

	_, err := bridge.HandleInbound(context.Background(), InboundMessage{
		Channel:   "slack",
		MessageID: "msg-9",
		ThreadID:  "thread-9",
	})
	if err == nil || err.Error() != "deliver outbound channel message: delivery offline" {
		t.Fatalf("expected delivery failure, got %v", err)
	}
	if len(reporter.statuses) != 1 {
		t.Fatalf("expected one delivery status, got %d", len(reporter.statuses))
	}
	if reporter.statuses[0].State != DeliveryStateFailed {
		t.Fatalf("expected failed state, got %q", reporter.statuses[0].State)
	}
	if reporter.statuses[0].FailureStage != "deliver" {
		t.Fatalf("expected deliver failure stage, got %q", reporter.statuses[0].FailureStage)
	}
}

func TestRuntimeBridgeSkipsEmptyOutboundText(t *testing.T) {
	adapter := &stubAdapter{
		name: "telegram",
		normalized: NormalizedInput{
			SessionKey: "session-11",
			UserText:   "noop",
		},
	}
	executor := &stubExecutor{
		output: agent.TurnOutput{},
	}
	reporter := &captureReporter{}
	bridge := RuntimeBridge{
		Adapter:  adapter,
		Executor: executor,
		Reporter: reporter,
	}

	handoff, err := bridge.HandleInbound(context.Background(), InboundMessage{
		Channel:   "telegram",
		MessageID: "msg-11",
	})
	if err != nil {
		t.Fatalf("HandleInbound returned error: %v", err)
	}
	if handoff.Outbound.Text != "" {
		t.Fatalf("expected empty outbound text, got %q", handoff.Outbound.Text)
	}
	if len(adapter.delivered) != 0 {
		t.Fatalf("expected no delivery calls, got %d", len(adapter.delivered))
	}
	if len(reporter.statuses) != 1 || reporter.statuses[0].State != DeliveryStateSkipped {
		t.Fatalf("expected skipped delivery state, got %+v", reporter.statuses)
	}
}
