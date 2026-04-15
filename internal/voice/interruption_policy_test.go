package voice

import (
	"testing"
	"time"
)

func TestEvaluateBargeInPolicyBoundaries(t *testing.T) {
	cfg := BargeInConfig{MinAudioMs: 120, IncompleteHoldMs: 240}

	tests := []struct {
		name            string
		preview         InputPreview
		wantPolicy      InterruptionPolicy
		wantAccepted    bool
		wantInterrupt   bool
		wantDuckOutput  bool
		wantReason      string
	}{
		{
			name:         "ignore_without_speech_start",
			preview:      InputPreview{},
			wantPolicy:   InterruptionPolicyIgnore,
			wantAccepted: false,
			wantReason:   "no_speech_started",
		},
		{
			name: "backchannel_short_ack",
			preview: InputPreview{
				PartialText:   "好的",
				AudioBytes:    pcmFrameBytes(16000, 1, 180),
				SpeechStarted: true,
			},
			wantPolicy:     InterruptionPolicyBackchannel,
			wantAccepted:   false,
			wantInterrupt:  false,
			wantDuckOutput: true,
			wantReason:     "backchannel_short_ack",
		},
		{
			name: "duck_only_incomplete_preview",
			preview: InputPreview{
				PartialText:   "帮我",
				AudioBytes:    pcmFrameBytes(16000, 1, 160),
				SpeechStarted: true,
			},
			wantPolicy:     InterruptionPolicyDuckOnly,
			wantAccepted:   false,
			wantInterrupt:  false,
			wantDuckOutput: true,
			wantReason:     "duck_pending_incomplete_preview",
		},
		{
			name: "hard_interrupt_complete_preview",
			preview: InputPreview{
				PartialText:   "打开客厅灯",
				AudioBytes:    pcmFrameBytes(16000, 1, 140),
				SpeechStarted: true,
			},
			wantPolicy:     InterruptionPolicyHardInterrupt,
			wantAccepted:   true,
			wantInterrupt:  true,
			wantDuckOutput: false,
			wantReason:     "accepted_complete_preview",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := EvaluateBargeIn(tc.preview, 16000, 1, cfg)
			if decision.Policy != tc.wantPolicy {
				t.Fatalf("expected policy %s, got %+v", tc.wantPolicy, decision)
			}
			if decision.Accepted != tc.wantAccepted {
				t.Fatalf("expected accepted=%v, got %+v", tc.wantAccepted, decision)
			}
			if decision.ShouldInterrupt() != tc.wantInterrupt {
				t.Fatalf("expected interrupt=%v, got %+v", tc.wantInterrupt, decision)
			}
			if decision.ShouldDuckOutput() != tc.wantDuckOutput {
				t.Fatalf("expected duck_output=%v, got %+v", tc.wantDuckOutput, decision)
			}
			if decision.Reason != tc.wantReason {
				t.Fatalf("expected reason %q, got %+v", tc.wantReason, decision)
			}
		})
	}
}

func TestEvaluateBargeInThresholdEdges(t *testing.T) {
	cfg := BargeInConfig{MinAudioMs: 120, IncompleteHoldMs: 240}

	tests := []struct {
		name         string
		preview      InputPreview
		sampleRateHz int
		channels     int
		wantPolicy   InterruptionPolicy
		wantAccepted bool
		wantReason   string
	}{
		{
			name: "partial_below_min_audio_stays_duck_only",
			preview: InputPreview{
				PartialText:   "打开",
				AudioBytes:    pcmFrameBytes(16000, 1, 119),
				SpeechStarted: true,
			},
			sampleRateHz: 16000,
			channels:     1,
			wantPolicy:   InterruptionPolicyDuckOnly,
			wantAccepted: false,
			wantReason:   "duck_pending_min_audio",
		},
		{
			name: "audio_only_at_min_audio_stays_duck_only",
			preview: InputPreview{
				AudioBytes:    pcmFrameBytes(16000, 1, 120),
				SpeechStarted: true,
			},
			sampleRateHz: 16000,
			channels:     1,
			wantPolicy:   InterruptionPolicyDuckOnly,
			wantAccepted: false,
			wantReason:   "duck_pending_audio_only",
		},
		{
			name: "incomplete_preview_at_hold_boundary_becomes_hard_interrupt",
			preview: InputPreview{
				PartialText:   "帮我",
				AudioBytes:    pcmFrameBytes(16000, 1, 360),
				SpeechStarted: true,
			},
			sampleRateHz: 16000,
			channels:     1,
			wantPolicy:   InterruptionPolicyHardInterrupt,
			wantAccepted: true,
			wantReason:   "accepted_incomplete_after_hold",
		},
		{
			name: "invalid_audio_format_is_ignored",
			preview: InputPreview{
				PartialText:   "打开客厅灯",
				AudioBytes:    pcmFrameBytes(16000, 1, 180),
				SpeechStarted: true,
			},
			sampleRateHz: 0,
			channels:     1,
			wantPolicy:   InterruptionPolicyIgnore,
			wantAccepted: false,
			wantReason:   "invalid_audio_format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := EvaluateBargeIn(tc.preview, tc.sampleRateHz, tc.channels, cfg)
			if decision.Policy != tc.wantPolicy {
				t.Fatalf("expected policy %s, got %+v", tc.wantPolicy, decision)
			}
			if decision.Accepted != tc.wantAccepted {
				t.Fatalf("expected accepted=%v, got %+v", tc.wantAccepted, decision)
			}
			if decision.Reason != tc.wantReason {
				t.Fatalf("expected reason %q, got %+v", tc.wantReason, decision)
			}
		})
	}
}

func TestBargeInDecisionPlaybackDirectiveProfiles(t *testing.T) {
	tests := []struct {
		name              string
		decision          BargeInDecision
		wantAction        PlaybackAction
		wantPolicy        InterruptionPolicy
		wantGain          float64
		wantAttack        time.Duration
		wantHold          time.Duration
		wantRelease       time.Duration
		wantKeepPreview   bool
		wantKeepPending   bool
		wantDuckOutput    bool
		wantInterrupt     bool
	}{
		{
			name: "normal_ignore",
			decision: BargeInDecision{
				Policy: InterruptionPolicyIgnore,
				Reason: "no_speech_started",
			},
			wantAction:      PlaybackActionNormal,
			wantPolicy:      InterruptionPolicyIgnore,
			wantGain:        1.0,
			wantAttack:      0,
			wantHold:        0,
			wantRelease:     0,
			wantKeepPreview: false,
			wantKeepPending: false,
		},
		{
			name: "duck_light_backchannel",
			decision: BargeInDecision{
				Policy:   InterruptionPolicyBackchannel,
				Reason:   "backchannel_short_ack",
				AudioMs:  180,
				MinAudioMs: 120,
			},
			wantAction:      PlaybackActionDuckLight,
			wantPolicy:      InterruptionPolicyBackchannel,
			wantGain:        0.72,
			wantAttack:      45 * time.Millisecond,
			wantHold:        260 * time.Millisecond,
			wantRelease:     180 * time.Millisecond,
			wantKeepPreview: true,
			wantKeepPending: true,
			wantDuckOutput:  true,
		},
		{
			name: "duck_hold_incomplete_preview",
			decision: BargeInDecision{
				Policy:      InterruptionPolicyDuckOnly,
				Reason:      "duck_pending_incomplete_preview",
				AudioMs:     160,
				MinAudioMs:  120,
				HoldAudioMs: 240,
			},
			wantAction:      PlaybackActionDuckHold,
			wantPolicy:      InterruptionPolicyDuckOnly,
			wantGain:        0.36,
			wantAttack:      30 * time.Millisecond,
			wantHold:        280 * time.Millisecond,
			wantRelease:     220 * time.Millisecond,
			wantKeepPreview: true,
			wantKeepPending: true,
			wantDuckOutput:  true,
		},
		{
			name: "interrupt_hard_interrupt",
			decision: BargeInDecision{
				Policy: InterruptionPolicyHardInterrupt,
				Reason: "accepted_complete_preview",
			},
			wantAction:      PlaybackActionInterrupt,
			wantPolicy:      InterruptionPolicyHardInterrupt,
			wantGain:        0,
			wantAttack:      0,
			wantHold:        0,
			wantRelease:     0,
			wantKeepPreview: true,
			wantKeepPending: true,
			wantInterrupt:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			directive := tc.decision.PlaybackDirective()
			if directive.Action != tc.wantAction {
				t.Fatalf("expected action %s, got %+v", tc.wantAction, directive)
			}
			if directive.Policy != tc.wantPolicy {
				t.Fatalf("expected policy %s, got %+v", tc.wantPolicy, directive)
			}
			if directive.Gain != tc.wantGain {
				t.Fatalf("expected gain %.2f, got %+v", tc.wantGain, directive)
			}
			if directive.Attack != tc.wantAttack || directive.Hold != tc.wantHold || directive.Release != tc.wantRelease {
				t.Fatalf("unexpected timings, got %+v", directive)
			}
			if directive.KeepPreview != tc.wantKeepPreview || directive.KeepPendingAudio != tc.wantKeepPending {
				t.Fatalf("unexpected keep flags, got %+v", directive)
			}
			if directive.ShouldDuckOutput() != tc.wantDuckOutput {
				t.Fatalf("expected duck_output=%v, got %+v", tc.wantDuckOutput, directive)
			}
			if directive.ShouldInterruptOutput() != tc.wantInterrupt {
				t.Fatalf("expected interrupt=%v, got %+v", tc.wantInterrupt, directive)
			}
		})
	}
}

func TestSessionOrchestratorPersistsDuckOnlyMetadataOnCompletedPlayback(t *testing.T) {
	store := &countingMemoryStore{}
	orchestrator := NewSessionOrchestrator(store)
	request := TurnRequest{
		SessionID:  "sess-duck",
		TurnID:     "turn-duck",
		DeviceID:   "dev-duck",
		ClientType: "rtos",
	}

	orchestrator.PrepareTurn(request, "帮我调暗灯光", "好的，正在帮你把灯光调暗一些。")
	orchestrator.StartPlayback("好的，正在帮你把灯光调暗一些。", 200*time.Millisecond, 2*time.Second)
	orchestrator.ObservePlaybackChunk()
	orchestrator.ObservePlaybackChunk()
	orchestrator.RecordInterruptionDecision(BargeInDecision{
		Policy: InterruptionPolicyDuckOnly,
		Reason: "duck_pending_incomplete_preview",
	})
	orchestrator.CompletePlayback()

	if got := len(store.saves); got != 2 {
		t.Fatalf("expected prepare + complete saves, got %d", got)
	}
	record := store.saves[1]
	if record.ResponseInterrupted || record.ResponseTruncated {
		t.Fatalf("expected duck_only to avoid interruption flags, got %+v", record)
	}
	if got := record.Metadata[interruptionPolicyMetadataKey]; got != string(InterruptionPolicyDuckOnly) {
		t.Fatalf("expected duck_only policy metadata, got %q", got)
	}
	if got := record.Metadata[interruptionReasonMetadataKey]; got != "duck_pending_incomplete_preview" {
		t.Fatalf("expected duck_only reason metadata, got %q", got)
	}
	if got := record.Metadata[heardTextBoundaryMetadataKey]; got != string(HeardTextBoundaryFull) {
		t.Fatalf("expected full heard-text boundary after completed playback, got %q", got)
	}
	if got := record.Metadata[playedDurationMetadataKey]; got != "400" {
		t.Fatalf("expected played duration metadata 400ms, got %q", got)
	}
	if got := record.Metadata[plannedDurationMetadataKey]; got != "2000" {
		t.Fatalf("expected planned duration metadata 2000ms, got %q", got)
	}
}

func TestSessionOrchestratorSkipsIgnoreInterruptionMetadata(t *testing.T) {
	store := &countingMemoryStore{}
	orchestrator := NewSessionOrchestrator(store)
	request := TurnRequest{
		SessionID:  "sess-ignore",
		TurnID:     "turn-ignore",
		DeviceID:   "dev-ignore",
		ClientType: "rtos",
	}

	orchestrator.PrepareTurn(request, "打开卧室灯", "好的，已经为你打开卧室灯。")
	orchestrator.StartPlayback("好的，已经为你打开卧室灯。", 200*time.Millisecond, 2*time.Second)
	orchestrator.ObservePlaybackChunk()
	orchestrator.RecordInterruptionDecision(BargeInDecision{
		Policy: InterruptionPolicyIgnore,
		Reason: "no_speech_started",
	})
	orchestrator.CompletePlayback()

	if got := len(store.saves); got != 2 {
		t.Fatalf("expected prepare + complete saves, got %d", got)
	}
	record := store.saves[1]
	if _, ok := record.Metadata[interruptionPolicyMetadataKey]; ok {
		t.Fatalf("expected ignore policy to skip interruption policy metadata, got %+v", record.Metadata)
	}
	if _, ok := record.Metadata[interruptionReasonMetadataKey]; ok {
		t.Fatalf("expected ignore policy to skip interruption reason metadata, got %+v", record.Metadata)
	}
	if got := record.Metadata[heardTextBoundaryMetadataKey]; got != string(HeardTextBoundaryFull) {
		t.Fatalf("expected heard-text boundary metadata to remain full on completed playback, got %q", got)
	}
}

func TestSessionOrchestratorPersistsHardInterruptHeardTextMetadata(t *testing.T) {
	store := &countingMemoryStore{}
	orchestrator := NewSessionOrchestrator(store)
	request := TurnRequest{
		SessionID:  "sess-hard",
		TurnID:     "turn-hard",
		DeviceID:   "dev-hard",
		ClientType: "rtos",
	}

	delivered := "好的，已经为你打开客厅灯。"
	orchestrator.PrepareTurn(request, "打开客厅灯", delivered)
	orchestrator.StartPlayback(delivered, 200*time.Millisecond, 2*time.Second)
	orchestrator.ObservePlaybackChunk()
	orchestrator.ObservePlaybackChunk()

	summary := orchestrator.InterruptPlaybackWithDecision(BargeInDecision{
		Policy: InterruptionPolicyHardInterrupt,
		Reason: "accepted_complete_preview",
	})
	if summary.Policy != InterruptionPolicyHardInterrupt {
		t.Fatalf("expected hard interrupt summary, got %+v", summary)
	}
	if summary.HeardTextBoundary != HeardTextBoundaryPrefix || !summary.Truncated {
		t.Fatalf("expected prefix heard text after interrupt, got %+v", summary)
	}
	if summary.HeardText == "" || summary.HeardText == delivered {
		t.Fatalf("expected interrupted playback to persist heard prefix, got %q", summary.HeardText)
	}
	if summary.PlayedDuration != 400*time.Millisecond || summary.PlannedDuration != 2*time.Second {
		t.Fatalf("unexpected duration summary %+v", summary)
	}

	if got := len(store.saves); got != 2 {
		t.Fatalf("expected prepare + interrupt saves, got %d", got)
	}
	record := store.saves[1]
	if !record.ResponseInterrupted || !record.ResponseTruncated {
		t.Fatalf("expected interrupted truncated record, got %+v", record)
	}
	if record.HeardText != summary.HeardText {
		t.Fatalf("expected record heard text %q, got %q", summary.HeardText, record.HeardText)
	}
	if got := record.Metadata[interruptionPolicyMetadataKey]; got != string(InterruptionPolicyHardInterrupt) {
		t.Fatalf("expected hard interrupt policy metadata, got %q", got)
	}
	if got := record.Metadata[interruptionReasonMetadataKey]; got != "accepted_complete_preview" {
		t.Fatalf("expected hard interrupt reason metadata, got %q", got)
	}
	if got := record.Metadata[heardTextBoundaryMetadataKey]; got != string(HeardTextBoundaryPrefix) {
		t.Fatalf("expected heard boundary prefix metadata, got %q", got)
	}
	if got := record.Metadata[playedDurationMetadataKey]; got != "400" {
		t.Fatalf("expected played duration metadata 400ms, got %q", got)
	}
	if got := record.Metadata[plannedDurationMetadataKey]; got != "2000" {
		t.Fatalf("expected planned duration metadata 2000ms, got %q", got)
	}
}
