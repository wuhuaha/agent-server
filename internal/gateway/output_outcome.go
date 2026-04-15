package gateway

import (
	"context"
	"sync"
)

type turnOutputOutcome struct {
	EndSession bool
	EndReason  string
	EndMessage string
}

type turnOutputOutcomeFuture struct {
	done chan struct{}

	mu      sync.Mutex
	ready   bool
	outcome turnOutputOutcome
}

func newTurnOutputOutcomeFuture() *turnOutputOutcomeFuture {
	return &turnOutputOutcomeFuture{done: make(chan struct{})}
}

func resolvedTurnOutputOutcome(outcome turnOutputOutcome) *turnOutputOutcomeFuture {
	future := newTurnOutputOutcomeFuture()
	future.Resolve(outcome)
	return future
}

func (f *turnOutputOutcomeFuture) Resolve(outcome turnOutputOutcome) {
	if f == nil {
		return
	}

	f.mu.Lock()
	if f.ready {
		f.mu.Unlock()
		return
	}
	f.ready = true
	f.outcome = outcome
	close(f.done)
	f.mu.Unlock()
}

func (f *turnOutputOutcomeFuture) Wait(ctx context.Context) (turnOutputOutcome, error) {
	if f == nil {
		return turnOutputOutcome{}, nil
	}

	select {
	case <-ctx.Done():
		return turnOutputOutcome{}, ctx.Err()
	case <-f.done:
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	return f.outcome, nil
}
