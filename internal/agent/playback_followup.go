package agent

import "strings"

const (
	previousPlaybackAvailableKey    = "voice.previous.available"
	previousPlaybackHeardTextKey    = "voice.previous.heard_text"
	previousPlaybackMissedTextKey   = "voice.previous.missed_text"
	previousPlaybackResumeAnchorKey = "voice.previous.resume_anchor"
	previousPlaybackInterruptedKey  = "voice.previous.response_interrupted"
	previousPlaybackTruncatedKey    = "voice.previous.response_truncated"
)

type previousPlaybackContext struct {
	Available           bool
	HeardText           string
	MissedText          string
	ResumeAnchor        string
	ResponseInterrupted bool
	ResponseTruncated   bool
}

type playbackFollowUpIntent string

const (
	playbackFollowUpIntentNone           playbackFollowUpIntent = ""
	playbackFollowUpIntentContinue       playbackFollowUpIntent = "continue"
	playbackFollowUpIntentRecallBoundary playbackFollowUpIntent = "recall_boundary"
	playbackFollowUpIntentRecapTail      playbackFollowUpIntent = "recap_tail"
)

func parsePreviousPlaybackContext(metadata map[string]string) previousPlaybackContext {
	ctx := previousPlaybackContext{
		Available:           metadataFlag(metadata, previousPlaybackAvailableKey),
		HeardText:           strings.TrimSpace(metadata[previousPlaybackHeardTextKey]),
		MissedText:          strings.TrimSpace(metadata[previousPlaybackMissedTextKey]),
		ResumeAnchor:        strings.TrimSpace(metadata[previousPlaybackResumeAnchorKey]),
		ResponseInterrupted: metadataFlag(metadata, previousPlaybackInterruptedKey),
		ResponseTruncated:   metadataFlag(metadata, previousPlaybackTruncatedKey),
	}
	if !ctx.Available && (ctx.HeardText != "" || ctx.MissedText != "" || ctx.ResumeAnchor != "") {
		ctx.Available = true
	}
	return ctx
}

func (c previousPlaybackContext) actionable() bool {
	return c.Available && (c.HeardText != "" || c.MissedText != "" || c.ResumeAnchor != "")
}

func detectPlaybackFollowUpIntent(userText string, ctx previousPlaybackContext) playbackFollowUpIntent {
	if intent := detectDeterministicPlaybackFollowUpIntent(userText, ctx); intent != playbackFollowUpIntentNone {
		return intent
	}
	return detectLoosePlaybackFollowUpIntent(userText, ctx)
}

func detectDeterministicPlaybackFollowUpIntent(userText string, ctx previousPlaybackContext) playbackFollowUpIntent {
	if !ctx.actionable() {
		return playbackFollowUpIntentNone
	}
	normalized := normalizePlaybackFollowUpText(userText)
	if normalized == "" {
		return playbackFollowUpIntentNone
	}

	if isPlaybackContinueIntent(normalized) && ctx.MissedText != "" {
		return playbackFollowUpIntentContinue
	}
	if isPlaybackBoundaryRecallIntent(normalized) && (ctx.HeardText != "" || ctx.ResumeAnchor != "") {
		return playbackFollowUpIntentRecallBoundary
	}
	if isPlaybackRecapTailIntent(normalized) && (ctx.MissedText != "" || ctx.HeardText != "") {
		return playbackFollowUpIntentRecapTail
	}
	if isPlaybackContinueIntent(normalized) && ctx.HeardText != "" {
		return playbackFollowUpIntentRecallBoundary
	}
	return playbackFollowUpIntentNone
}

func detectLoosePlaybackFollowUpIntent(userText string, ctx previousPlaybackContext) playbackFollowUpIntent {
	if !ctx.actionable() {
		return playbackFollowUpIntentNone
	}
	normalized := normalizePlaybackFollowUpText(userText)
	if normalized == "" {
		return playbackFollowUpIntentNone
	}

	if isPlaybackContinueLikeIntent(normalized) && ctx.MissedText != "" {
		return playbackFollowUpIntentContinue
	}
	if isPlaybackBoundaryRecallLikeIntent(normalized) && (ctx.HeardText != "" || ctx.ResumeAnchor != "") {
		return playbackFollowUpIntentRecallBoundary
	}
	if isPlaybackRecapTailLikeIntent(normalized) && (ctx.MissedText != "" || ctx.HeardText != "") {
		return playbackFollowUpIntentRecapTail
	}
	if isPlaybackContinueLikeIntent(normalized) && ctx.HeardText != "" {
		return playbackFollowUpIntentRecallBoundary
	}
	return playbackFollowUpIntentNone
}

func deterministicPlaybackFollowUpText(userText string, metadata map[string]string) (string, bool) {
	ctx := parsePreviousPlaybackContext(metadata)
	intent := detectDeterministicPlaybackFollowUpIntent(userText, ctx)
	switch intent {
	case playbackFollowUpIntentContinue:
		if ctx.MissedText != "" {
			return ctx.MissedText, true
		}
	case playbackFollowUpIntentRecallBoundary:
		switch {
		case ctx.HeardText != "" && ctx.MissedText != "":
			return "我刚刚说到：" + ctx.HeardText + "。后面还没播完的是：" + ctx.MissedText, true
		case ctx.HeardText != "":
			return "我刚刚说到：" + ctx.HeardText, true
		case ctx.ResumeAnchor != "":
			return "我刚刚说到：" + ctx.ResumeAnchor, true
		}
	case playbackFollowUpIntentRecapTail:
		switch {
		case ctx.MissedText != "":
			return "后面那句是：" + ctx.MissedText, true
		case ctx.HeardText != "":
			return "我刚刚说的是：" + ctx.HeardText, true
		}
	}
	return "", false
}

func bootstrapPlaybackFollowUpText(userText string, metadata map[string]string) (string, bool) {
	return deterministicPlaybackFollowUpText(userText, metadata)
}

func playbackFollowUpMessages(userText string, metadata map[string]string) []ChatMessage {
	ctx := parsePreviousPlaybackContext(metadata)
	intent := detectPlaybackFollowUpIntent(userText, ctx)
	if intent == playbackFollowUpIntentNone {
		return nil
	}

	lines := []string{"Runtime voice follow-up hint:"}
	switch intent {
	case playbackFollowUpIntentContinue:
		lines = append(lines,
			"- The user is asking to continue the previous spoken reply instead of starting over.",
			"- Treat the unheard tail as the canonical continuation whenever it is available.",
			"- Continue naturally from the unheard tail or resume anchor instead of restarting or fully re-summarizing the previous answer.",
		)
	case playbackFollowUpIntentRecallBoundary:
		lines = append(lines,
			"- The user is asking what part of the previous spoken reply was actually reached.",
			"- Answer from the heard boundary first, then mention any unheard tail only if it helps the user resume.",
		)
	case playbackFollowUpIntentRecapTail:
		lines = append(lines,
			"- The user is asking to repeat or clarify the previous unfinished spoken reply.",
			"- Prefer a concise recap of the unheard tail; avoid restarting the whole previous reply unless needed.",
		)
	}
	if ctx.HeardText != "" {
		lines = append(lines, "- Heard boundary: "+ctx.HeardText)
		lines = append(lines, "- Avoid repeating the heard boundary unless a tiny bridge is needed for coherence.")
	}
	if ctx.MissedText != "" {
		lines = append(lines, "- Unheard tail: "+ctx.MissedText)
	}
	if ctx.ResumeAnchor != "" {
		lines = append(lines, "- Resume anchor: "+ctx.ResumeAnchor)
	}
	if intent == playbackFollowUpIntentContinue && ctx.MissedText != "" {
		lines = append(lines, "- Canonical continuation text: "+ctx.MissedText)
	}
	if ctx.ResponseInterrupted || ctx.ResponseTruncated {
		lines = append(lines, "- The previous spoken reply was interrupted before full playback completion.")
	}
	return []ChatMessage{{
		Role:    "system",
		Content: strings.Join(lines, "\n"),
	}}
}

func normalizePlaybackFollowUpText(text string) string {
	trimmed := strings.TrimSpace(strings.ToLower(text))
	if trimmed == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"？", "",
		"。", "",
		"！", "",
		"，", "",
		",", "",
		".", "",
		"!", "",
		"?", "",
		"“", "",
		"”", "",
		"‘", "",
		"’", "",
		"'", "",
		"\"", "",
		" ", "",
	)
	return replacer.Replace(trimmed)
}

func isPlaybackContinueIntent(normalized string) bool {
	switch normalized {
	case "继续", "继续说", "继续讲", "接着说", "接着讲", "接着", "后面呢", "然后呢", "继续吧", "goon", "continue", "keepgoing":
		return true
	default:
		return false
	}
}

func isPlaybackBoundaryRecallIntent(normalized string) bool {
	switch normalized {
	case "你刚刚说到哪了", "你刚刚说到哪", "刚刚说到哪了", "刚刚说到哪", "你刚才说到哪了", "你刚才说到哪", "刚才说到哪了", "刚才说到哪":
		return true
	default:
		return false
	}
}

func isPlaybackRecapTailIntent(normalized string) bool {
	switch normalized {
	case "没听清", "没听清楚", "你刚刚最后一句", "你刚刚最后一句是什么", "刚刚最后一句", "最后一句", "你刚才最后一句", "你刚才最后一句是什么", "再说一遍", "再说一次", "repeatthat", "sayitagain":
		return true
	default:
		return false
	}
}

func isPlaybackContinueLikeIntent(normalized string) bool {
	if isPlaybackContinueIntent(normalized) {
		return true
	}
	for _, prefix := range []string{"继续", "接着", "后面", "然后"} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return strings.HasPrefix(normalized, "continue") || strings.HasPrefix(normalized, "goon")
}

func isPlaybackBoundaryRecallLikeIntent(normalized string) bool {
	if isPlaybackBoundaryRecallIntent(normalized) {
		return true
	}
	return strings.Contains(normalized, "说到哪") || strings.Contains(normalized, "讲到哪")
}

func isPlaybackRecapTailLikeIntent(normalized string) bool {
	if isPlaybackRecapTailIntent(normalized) {
		return true
	}
	return strings.Contains(normalized, "没听清") ||
		strings.Contains(normalized, "最后一句") ||
		strings.Contains(normalized, "再说一遍") ||
		strings.Contains(normalized, "再说一次") ||
		strings.Contains(normalized, "再重复")
}

func metadataFlag(metadata map[string]string, key string) bool {
	return strings.EqualFold(strings.TrimSpace(metadata[key]), "true")
}
