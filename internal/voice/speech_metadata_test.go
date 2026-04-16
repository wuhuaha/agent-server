package voice

import "testing"

func TestBuildTranscriptionMetadataAddsPunctuationAndClauseHints(t *testing.T) {
	metadata := buildTranscriptionMetadata(TranscriptionResult{
		Text:        "明天周几？",
		Segments:    []string{"明天周几？"},
		AudioEvents: []string{"speech", "bgm"},
	})
	if got := metadata["speech.text_terminal_punctuation"]; got != "question_mark" {
		t.Fatalf("expected question_mark terminal punctuation, got %q", got)
	}
	if got := metadata["speech.text_clause_count"]; got != "1" {
		t.Fatalf("expected clause count 1, got %q", got)
	}
	if got := metadata["speech.audio_events"]; got == "" {
		t.Fatal("expected encoded audio events metadata")
	}
}

func TestDetectSpeechTerminalPunctuation(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "明天周几？", want: "question_mark"},
		{text: "好的。", want: "strong_stop"},
		{text: "嗯，", want: "soft_pause"},
		{text: "这个嘛……", want: "ellipsis"},
		{text: "打开客厅灯", want: ""},
	}
	for _, tc := range tests {
		if got := detectSpeechTerminalPunctuation(tc.text); got != tc.want {
			t.Fatalf("detectSpeechTerminalPunctuation(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}
