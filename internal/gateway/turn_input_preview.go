package gateway

import (
	"context"
	"time"

	"agent-server/internal/session"
	"agent-server/internal/voice"
)

type inputPreviewObservation struct {
	Preview                   voice.InputPreview
	Active                    bool
	PartialChanged            bool
	SpeechStartedObserved     bool
	EndpointCandidateObserved bool
	CommitSuggested           bool
	Trace                     inputPreviewTrace
}

func (r *connectionRuntime) ensureInputPreview(ctx context.Context, responder voice.Responder, snapshot session.Snapshot, language string) error {
	if r.voiceSession == nil {
		return nil
	}
	return r.voiceSession.EnsureInputPreview(ctx, responder, voice.InputPreviewRequest{
		SessionID:    snapshot.SessionID,
		DeviceID:     snapshot.DeviceID,
		ClientType:   snapshot.ClientType,
		Codec:        snapshot.InputCodec,
		SampleRateHz: snapshot.InputSampleRate,
		Channels:     snapshot.InputChannels,
		Language:     language,
	})
}

func (r *connectionRuntime) pushInputPreviewAudio(ctx context.Context, payload []byte) ([]inputPreviewObservation, error) {
	if r.voiceSession == nil {
		return nil, nil
	}
	sessionID := r.session.Snapshot().SessionID
	// 一次 websocket 音频帧可能比 preview 驱动粒度更大，这里只记录一次入口音频，
	// 具体如何切成更小的 preview 子块交给 voice runtime 决定。
	r.previewTrace.ObserveAudio(sessionID, len(payload), time.Now().UTC())

	var observations []inputPreviewObservation
	appendObservation := func(observation voice.InputPreviewObservation) {
		now := time.Now().UTC()
		if observation.Active {
			_, _ = r.session.SetInputState(session.InputStatePreviewing)
		}
		trace, _, speechStarted, endpointCandidate, _ := r.previewTrace.ObservePreview(sessionID, observation.Preview, now)
		observations = append(observations, inputPreviewObservation{
			Preview:                   observation.Preview,
			Active:                    observation.Active,
			PartialChanged:            observation.PartialChanged,
			SpeechStartedObserved:     speechStarted,
			EndpointCandidateObserved: endpointCandidate,
			CommitSuggested:           observation.CommitSuggested,
			Trace:                     trace,
		})
	}

	observation, err := r.voiceSession.PushInputPreviewAudioProgressively(ctx, payload, appendObservation)
	if err != nil {
		return nil, err
	}
	if len(observations) == 0 && observation.Active {
		appendObservation(observation)
	}
	return observations, nil
}

func (r *connectionRuntime) pollInputPreview(now time.Time) inputPreviewObservation {
	if r.voiceSession == nil {
		return inputPreviewObservation{}
	}
	observation := r.voiceSession.PollInputPreview(now)
	if observation.Active {
		_, _ = r.session.SetInputState(session.InputStatePreviewing)
	}
	trace, _, speechStarted, endpointCandidate, _ := r.previewTrace.ObservePreview(r.session.Snapshot().SessionID, observation.Preview, now)
	return inputPreviewObservation{
		Preview:                   observation.Preview,
		Active:                    observation.Active,
		PartialChanged:            observation.PartialChanged,
		SpeechStartedObserved:     speechStarted,
		EndpointCandidateObserved: endpointCandidate,
		CommitSuggested:           observation.CommitSuggested,
		Trace:                     trace,
	}
}

func (r *connectionRuntime) currentInputPreviewTrace() inputPreviewTrace {
	return r.previewTrace.Current()
}

func (r *connectionRuntime) consumeInputPreview(ctx context.Context) (inputPreviewTrace, voice.TranscriptionResult, bool, error) {
	trace := r.previewTrace.Clear()
	var (
		result voice.TranscriptionResult
		ok     bool
		err    error
	)
	if r.voiceSession != nil {
		result, ok, err = r.voiceSession.FinalizeInputPreview(ctx)
	}
	snapshot := r.session.Snapshot()
	if snapshot.SessionID != "" && snapshot.InputState == session.InputStatePreviewing {
		_, _ = r.session.SetInputState(session.InputStateActive)
	}
	return trace, result, ok, err
}

func (r *connectionRuntime) clearInputPreview() inputPreviewTrace {
	trace := r.previewTrace.Clear()
	if r.voiceSession != nil {
		r.voiceSession.ClearInputPreview()
	}
	snapshot := r.session.Snapshot()
	if snapshot.SessionID != "" && snapshot.InputState == session.InputStatePreviewing {
		_, _ = r.session.SetInputState(session.InputStateActive)
	}
	return trace
}
