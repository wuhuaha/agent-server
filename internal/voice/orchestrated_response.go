package voice

import (
	"context"
	"sync"
)

type asyncTurnResponseFuture struct {
	responseReady chan struct{}
	audioReady    chan struct{}

	mu               sync.Mutex
	resolved         bool
	response         TurnResponse
	err              error
	audioResolved    bool
	audioAvailable   bool
	audioStart       ResponseAudioStart
	audioClaimed     bool
	responseDelivered bool
}

func newAsyncTurnResponseFuture() *asyncTurnResponseFuture {
	return &asyncTurnResponseFuture{
		responseReady: make(chan struct{}),
		audioReady:    make(chan struct{}),
	}
}

func (f *asyncTurnResponseFuture) PublishAudioStart(start ResponseAudioStart) {
	if f == nil || start.Stream == nil {
		return
	}

	f.mu.Lock()
	if f.audioResolved || f.responseDelivered {
		f.mu.Unlock()
		return
	}
	f.audioResolved = true
	f.audioAvailable = true
	f.audioStart = start
	close(f.audioReady)
	f.mu.Unlock()
}

func (f *asyncTurnResponseFuture) Resolve(response TurnResponse, err error) {
	if f == nil {
		return
	}

	f.mu.Lock()
	if f.resolved {
		f.mu.Unlock()
		return
	}
	f.resolved = true
	f.response = response
	f.err = err
	if !f.audioResolved {
		start, ok := responseAudioStartFromTurnResponse(response)
		f.audioResolved = true
		f.audioAvailable = ok
		if ok {
			f.audioStart = start
		}
		close(f.audioReady)
	}
	close(f.responseReady)
	f.mu.Unlock()
}

func (f *asyncTurnResponseFuture) Wait(ctx context.Context) (TurnResponse, error) {
	if f == nil {
		return TurnResponse{}, nil
	}

	select {
	case <-ctx.Done():
		return TurnResponse{}, ctx.Err()
	case <-f.responseReady:
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return TurnResponse{}, f.err
	}

	response := f.response
	switch {
	case f.audioClaimed:
		return detachTurnResponseAudio(response), nil
	case !f.responseDelivered:
		f.responseDelivered = true
		return response, nil
	case turnResponseHasAudio(response):
		return detachTurnResponseAudio(response), nil
	default:
		return response, nil
	}
}

func (f *asyncTurnResponseFuture) WaitAudioStart(ctx context.Context) (ResponseAudioStart, bool, error) {
	if f == nil {
		return ResponseAudioStart{}, false, nil
	}

	select {
	case <-ctx.Done():
		return ResponseAudioStart{}, false, ctx.Err()
	case <-f.audioReady:
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return ResponseAudioStart{}, false, f.err
	}
	if !f.audioAvailable || f.audioClaimed || f.responseDelivered {
		return ResponseAudioStart{}, false, nil
	}
	f.audioClaimed = true
	return f.audioStart, true, nil
}

func responseAudioStartFromTurnResponse(response TurnResponse) (ResponseAudioStart, bool) {
	switch {
	case response.AudioStream != nil:
		return ResponseAudioStart{
			Stream:      response.AudioStream,
			Text:        response.Text,
			Incremental: false,
			Source:      ResponseAudioStartSourceFinalResponse,
		}, true
	case len(response.AudioChunks) > 0:
		return ResponseAudioStart{
			Stream:      NewStaticAudioStream(response.AudioChunks),
			Text:        response.Text,
			Incremental: false,
			Source:      ResponseAudioStartSourceFinalResponse,
		}, true
	default:
		return ResponseAudioStart{}, false
	}
}

func turnResponseHasAudio(response TurnResponse) bool {
	return response.AudioStream != nil || len(response.AudioChunks) > 0 || response.AudioStreamTransferred
}

func detachTurnResponseAudio(response TurnResponse) TurnResponse {
	if response.AudioStream == nil && len(response.AudioChunks) == 0 {
		return response
	}
	response.AudioStream = nil
	response.AudioChunks = nil
	response.AudioStreamTransferred = true
	return response
}
