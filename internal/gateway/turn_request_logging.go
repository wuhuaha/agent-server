package gateway

import (
	"log/slog"
	"strings"

	"agent-server/internal/voice"
)

func logTurnRequestPrepared(logger *slog.Logger, sessionID string, trace turnTrace, request voice.TurnRequest) {
	if logger == nil {
		return
	}
	provisionalInputText := strings.TrimSpace(request.Text)
	if provisionalInputText == "" && request.PreviewTranscription != nil {
		provisionalInputText = strings.TrimSpace(request.PreviewTranscription.Text)
	}

	logTurnTraceInfo(logger, "gateway turn request prepared", sessionID, trace,
		"input_text_len", len(strings.TrimSpace(request.Text)),
		"provisional_input_text_len", len(provisionalInputText),
		"preview_transcription_available", request.PreviewTranscription != nil,
		"preview_text_len", previewTextLen(request.PreviewTranscription),
		"preview_mode", previewMode(request.PreviewTranscription),
		"preview_endpoint_reason", previewEndpointReason(request.PreviewTranscription),
		"input_audio_bytes", len(request.AudioPCM),
		"audio_bytes", request.AudioBytes,
		"input_frames", request.InputFrames,
		"input_codec", request.InputCodec,
		"input_sample_rate_hz", request.InputSampleRate,
		"input_channels", request.InputChannels,
		"voice_previous_available", request.Metadata["voice.previous.available"] == "true",
		"metadata_keys_count", len(request.Metadata),
	)
}

func previewTextLen(result *voice.TranscriptionResult) int {
	if result == nil {
		return 0
	}
	return len(strings.TrimSpace(result.Text))
}

func previewMode(result *voice.TranscriptionResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.Mode)
}

func previewEndpointReason(result *voice.TranscriptionResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.EndpointReason)
}
