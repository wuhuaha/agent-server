package voice

import "testing"

func TestShouldAcceptBargeInRequiresEnoughAudioForIncompletePreview(t *testing.T) {
	cfg := BargeInConfig{MinAudioMs: 120, IncompleteHoldMs: 240}
	preview := InputPreview{
		PartialText:   "嗯",
		AudioBytes:    pcmFrameBytes(16000, 1, 200),
		SpeechStarted: true,
	}
	if ShouldAcceptBargeIn(preview, 16000, 1, cfg) {
		t.Fatal("expected standalone hesitation to stay below barge-in threshold")
	}
	preview.AudioBytes = pcmFrameBytes(16000, 1, 380)
	if !ShouldAcceptBargeIn(preview, 16000, 1, cfg) {
		t.Fatal("expected longer hesitation audio to eventually pass the hold threshold")
	}
}

func TestShouldAcceptBargeInAllowsLexicallyCompletePreviewSooner(t *testing.T) {
	cfg := BargeInConfig{MinAudioMs: 120, IncompleteHoldMs: 240}
	preview := InputPreview{
		PartialText:   "打开客厅灯",
		AudioBytes:    pcmFrameBytes(16000, 1, 140),
		SpeechStarted: true,
	}
	if !ShouldAcceptBargeIn(preview, 16000, 1, cfg) {
		t.Fatal("expected lexically complete preview to pass the base barge-in threshold")
	}
}

func TestShouldAcceptBargeInRejectsEmptyOrUnsoundedPreview(t *testing.T) {
	cfg := BargeInConfig{}
	if ShouldAcceptBargeIn(InputPreview{}, 16000, 1, cfg) {
		t.Fatal("expected empty preview to be rejected")
	}
	if ShouldAcceptBargeIn(InputPreview{AudioBytes: pcmFrameBytes(16000, 1, 160)}, 16000, 1, cfg) {
		t.Fatal("expected preview without speech_started to be rejected")
	}
}
