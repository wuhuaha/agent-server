package gateway

import "log/slog"

func logInputPreviewObservationLifecycle(logger *slog.Logger, sessionID string, prefix string, observation inputPreviewObservation) {
	if logger == nil || observation.Trace.PreviewID == "" {
		return
	}
	if observation.PartialChanged {
		logInputPreviewTraceInfo(logger, prefix+" updated", sessionID, observation.Trace,
			"partial_text", observation.Preview.PartialText,
			"audio_bytes", observation.Preview.AudioBytes,
		)
	}
	if observation.SpeechStartedObserved {
		logInputPreviewTraceInfo(logger, prefix+" speech started", sessionID, observation.Trace,
			"audio_bytes", observation.Preview.AudioBytes,
		)
	}
	if observation.CandidateReadyObserved {
		logInputPreviewTraceInfo(logger, prefix+" candidate ready", sessionID, observation.Trace)
	}
	if observation.DraftReadyObserved {
		logInputPreviewTraceInfo(logger, prefix+" draft ready", sessionID, observation.Trace)
	}
	if observation.AcceptReadyObserved {
		logInputPreviewTraceInfo(logger, prefix+" accept ready", sessionID, observation.Trace)
	}
	if observation.SemanticReadyObserved {
		logInputPreviewTraceInfo(logger, prefix+" semantic ready", sessionID, observation.Trace,
			"partial_text", observation.Preview.PartialText,
			"audio_bytes", observation.Preview.AudioBytes,
		)
	}
	if observation.SlotReadyObserved {
		logInputPreviewTraceInfo(logger, prefix+" slot ready", sessionID, observation.Trace,
			"partial_text", observation.Preview.PartialText,
			"audio_bytes", observation.Preview.AudioBytes,
		)
	}
	if observation.EndpointCandidateObserved {
		logInputPreviewTraceInfo(logger, prefix+" endpoint candidate", sessionID, observation.Trace,
			"partial_text", observation.Preview.PartialText,
			"audio_bytes", observation.Preview.AudioBytes,
			"endpoint_reason", observation.Preview.EndpointReason,
		)
	}
	if observation.CommitSuggested {
		logInputPreviewTraceInfo(logger, prefix+" commit suggested", sessionID, observation.Trace,
			"partial_text", observation.Preview.PartialText,
			"audio_bytes", observation.Preview.AudioBytes,
			"endpoint_reason", observation.Preview.EndpointReason,
		)
	}
}
