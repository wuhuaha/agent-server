package voice

import "testing"

func TestEvaluateBargeInClassifiesShortBackchannelWithoutInterrupt(t *testing.T) {
	cfg := BargeInConfig{MinAudioMs: 120, IncompleteHoldMs: 240}
	decision := EvaluateBargeIn(InputPreview{
		PartialText:   "嗯嗯",
		AudioBytes:    pcmFrameBytes(16000, 1, 180),
		SpeechStarted: true,
	}, 16000, 1, cfg)
	if decision.Policy != InterruptionPolicyBackchannel {
		t.Fatalf("expected backchannel policy, got %+v", decision)
	}
	if decision.Accepted || decision.ShouldInterrupt() {
		t.Fatalf("expected short backchannel to avoid hard interrupt, got %+v", decision)
	}
	if !decision.ShouldDuckOutput() {
		t.Fatalf("expected backchannel to remain actionable for future ducking, got %+v", decision)
	}
}

func TestEvaluateBargeInRequiresEnoughAudioForIncompletePreview(t *testing.T) {
	cfg := BargeInConfig{MinAudioMs: 120, IncompleteHoldMs: 240}
	preview := InputPreview{
		PartialText:   "嗯",
		AudioBytes:    pcmFrameBytes(16000, 1, 200),
		SpeechStarted: true,
	}
	decision := EvaluateBargeIn(preview, 16000, 1, cfg)
	if decision.Policy != InterruptionPolicyBackchannel {
		t.Fatalf("expected standalone hesitation to classify as backchannel first, got %+v", decision)
	}
	preview.AudioBytes = pcmFrameBytes(16000, 1, 380)
	decision = EvaluateBargeIn(preview, 16000, 1, cfg)
	if decision.Policy != InterruptionPolicyBackchannel {
		t.Fatalf("expected repeated hesitation to remain a backchannel, got %+v", decision)
	}
}

func TestEvaluateBargeInAllowsLexicallyCompletePreviewSooner(t *testing.T) {
	cfg := BargeInConfig{MinAudioMs: 120, IncompleteHoldMs: 240}
	preview := InputPreview{
		PartialText:   "打开客厅灯",
		AudioBytes:    pcmFrameBytes(16000, 1, 140),
		SpeechStarted: true,
	}
	decision := EvaluateBargeIn(preview, 16000, 1, cfg)
	if decision.Policy != InterruptionPolicyHardInterrupt || !decision.Accepted {
		t.Fatalf("expected complete preview to hard interrupt, got %+v", decision)
	}
}

func TestEvaluateBargeInEscalatesIncompletePreviewAfterHold(t *testing.T) {
	cfg := BargeInConfig{MinAudioMs: 120, IncompleteHoldMs: 240}
	preview := InputPreview{
		PartialText:   "帮我",
		AudioBytes:    pcmFrameBytes(16000, 1, 240),
		SpeechStarted: true,
	}
	decision := EvaluateBargeIn(preview, 16000, 1, cfg)
	if decision.Policy != InterruptionPolicyDuckOnly || decision.Accepted {
		t.Fatalf("expected incomplete preview before hold to stay duck_only, got %+v", decision)
	}

	preview.AudioBytes = pcmFrameBytes(16000, 1, 420)
	decision = EvaluateBargeIn(preview, 16000, 1, cfg)
	if decision.Policy != InterruptionPolicyHardInterrupt || !decision.Accepted {
		t.Fatalf("expected incomplete preview after hold to hard interrupt, got %+v", decision)
	}
}

func TestEvaluateBargeInRejectsEmptyOrUnsoundedPreview(t *testing.T) {
	cfg := BargeInConfig{}
	decision := EvaluateBargeIn(InputPreview{}, 16000, 1, cfg)
	if decision.Policy != InterruptionPolicyIgnore || decision.Accepted {
		t.Fatalf("expected empty preview to be ignored, got %+v", decision)
	}
	decision = EvaluateBargeIn(InputPreview{AudioBytes: pcmFrameBytes(16000, 1, 160)}, 16000, 1, cfg)
	if decision.Policy != InterruptionPolicyIgnore || decision.Accepted {
		t.Fatalf("expected preview without speech_started to be ignored, got %+v", decision)
	}
}
