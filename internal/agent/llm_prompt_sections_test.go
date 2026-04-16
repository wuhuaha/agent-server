package agent

import (
	"strings"
	"testing"
)

func TestRenderSpeechInputContextPromptIncludesSpeechHints(t *testing.T) {
	prompt := renderSpeechInputContextPrompt(map[string]string{
		"speech.emotion":                   "anxious",
		"speech.audio_events":              "[\"speech\",\"bgm\"]",
		"speech.text_terminal_punctuation": "question_mark",
		"speech.text_clause_count":         "2",
		"speech.endpoint_reason":           "silence_timeout",
	})
	for _, want := range []string{
		"用户当前语气/情绪弱信号",
		"当前音频事件/环境线索",
		"问句收尾",
		"意群数估计：2",
		"silence_timeout",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected speech input context prompt to contain %q, got %q", want, prompt)
		}
	}
}

func TestRenderSpeechInputContextPromptSkipsNeutralEmptyHints(t *testing.T) {
	prompt := renderSpeechInputContextPrompt(map[string]string{
		"speech.emotion": "calm",
	})
	if prompt != "" {
		t.Fatalf("expected empty prompt for neutral-only hints, got %q", prompt)
	}
}
