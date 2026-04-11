package voice

import (
	"encoding/json"
	"strconv"
	"strings"
)

func turnMetadataWithTranscription(base map[string]string, result TranscriptionResult) map[string]string {
	metadata := cloneStringMap(base)
	transcriptionMetadata := buildTranscriptionMetadata(result)
	if len(transcriptionMetadata) == 0 {
		return metadata
	}
	if metadata == nil {
		metadata = make(map[string]string, len(transcriptionMetadata))
	}
	for key, value := range transcriptionMetadata {
		metadata[key] = value
	}
	return metadata
}

func buildTranscriptionMetadata(result TranscriptionResult) map[string]string {
	metadata := make(map[string]string, 12)
	put := func(key, value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			metadata[key] = trimmed
		}
	}

	put("speech.source", "asr")
	put("speech.language", result.Language)
	put("speech.emotion", result.Emotion)
	put("speech.speaker_id", result.SpeakerID)
	put("speech.endpoint_reason", result.EndpointReason)
	put("speech.model", result.Model)
	put("speech.device", result.Device)
	put("speech.transcriber_mode", result.Mode)
	if result.DurationMs > 0 {
		metadata["speech.duration_ms"] = strconv.Itoa(result.DurationMs)
	}
	if result.ElapsedMs > 0 {
		metadata["speech.elapsed_ms"] = strconv.Itoa(result.ElapsedMs)
	}
	if encoded := encodeSpeechStringList(result.AudioEvents); encoded != "" {
		metadata["speech.audio_events"] = encoded
	}
	if encoded := encodeSpeechStringList(result.Partials); encoded != "" {
		metadata["speech.partials"] = encoded
		metadata["speech.partial_count"] = strconv.Itoa(len(result.Partials))
	}
	if encoded := encodeSpeechStringList(result.Segments); encoded != "" {
		metadata["speech.segments"] = encoded
	}
	if len(metadata) == 1 && metadata["speech.source"] == "asr" {
		return nil
	}
	return metadata
}

func encodeSpeechStringList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func cloneStringMap(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
