package voice

import "strings"

const (
	SemanticTaskFamilyUnknown           = "unknown"
	SemanticTaskFamilyDialogue          = "dialogue"
	SemanticTaskFamilyKnowledgeQuery    = "knowledge_query"
	SemanticTaskFamilyStructuredCommand = "structured_command"
	SemanticTaskFamilyStructuredQuery   = "structured_query"
	SemanticTaskFamilyCorrection        = "correction"
	SemanticTaskFamilyBackchannel       = "backchannel"
)

func normalizeSemanticTaskFamily(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SemanticTaskFamilyDialogue:
		return SemanticTaskFamilyDialogue
	case SemanticTaskFamilyKnowledgeQuery:
		return SemanticTaskFamilyKnowledgeQuery
	case SemanticTaskFamilyStructuredCommand:
		return SemanticTaskFamilyStructuredCommand
	case SemanticTaskFamilyStructuredQuery:
		return SemanticTaskFamilyStructuredQuery
	case SemanticTaskFamilyCorrection:
		return SemanticTaskFamilyCorrection
	case SemanticTaskFamilyBackchannel:
		return SemanticTaskFamilyBackchannel
	default:
		return SemanticTaskFamilyUnknown
	}
}

func taskFamilyRequiresSlotReadiness(family string) bool {
	return normalizeSemanticTaskFamily(family) == SemanticTaskFamilyStructuredCommand
}

func inferLexicalTaskFamily(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return SemanticTaskFamilyUnknown
	}
	if looksLikeBackchannel(trimmed) {
		return SemanticTaskFamilyBackchannel
	}
	if looksCorrectionPending(trimmed) || looksLikeTakeoverLexicon(trimmed) {
		return SemanticTaskFamilyCorrection
	}
	if looksQuestionClosure(trimmed) || matchesAnySuffix(trimmed, chineseQuestionSuffixes) {
		return SemanticTaskFamilyKnowledgeQuery
	}
	if hasAnyPrefix(trimmed, chineseStructuredQueryPrefixes) || hasAnyPrefix(strings.ToLower(trimmed), englishStructuredQueryPrefixes) {
		return SemanticTaskFamilyStructuredQuery
	}
	if hasAnyPrefix(trimmed, chineseStructuredCommandPrefixes) || hasAnyPrefix(strings.ToLower(trimmed), englishStructuredCommandPrefixes) {
		return SemanticTaskFamilyStructuredCommand
	}
	return SemanticTaskFamilyDialogue
}

func inferSemanticTaskFamily(snapshot InputPreview, judgement SemanticTurnJudgement) string {
	switch normalizeSemanticIntent(judgement.InterruptionIntent) {
	case SemanticIntentBackchannel:
		return SemanticTaskFamilyBackchannel
	case SemanticIntentCorrection:
		return SemanticTaskFamilyCorrection
	case SemanticIntentQuestion:
		return SemanticTaskFamilyKnowledgeQuery
	case SemanticIntentRequest, SemanticIntentTakeover, SemanticIntentContinue:
		bestText := strings.TrimSpace(firstNonEmpty(snapshot.PartialText, snapshot.StablePrefix))
		family := inferLexicalTaskFamily(bestText)
		if family == SemanticTaskFamilyDialogue || family == SemanticTaskFamilyUnknown {
			return SemanticTaskFamilyStructuredCommand
		}
		return family
	default:
		bestText := strings.TrimSpace(firstNonEmpty(snapshot.PartialText, snapshot.StablePrefix))
		return inferLexicalTaskFamily(bestText)
	}
}

func inferSlotTaskFamily(snapshot InputPreview, result SemanticSlotParseResult) string {
	if family := normalizeSemanticTaskFamily(result.TaskFamily); family != SemanticTaskFamilyUnknown {
		return family
	}
	switch normalizeSemanticSlotDomain(result.Domain) {
	case SemanticSlotDomainGeneralChat:
		if inferLexicalTaskFamily(firstNonEmpty(snapshot.PartialText, snapshot.StablePrefix)) == SemanticTaskFamilyKnowledgeQuery {
			return SemanticTaskFamilyKnowledgeQuery
		}
		return SemanticTaskFamilyDialogue
	case SemanticSlotDomainSmartHome, SemanticSlotDomainDesktopAssistant:
		if strings.Contains(strings.ToLower(strings.TrimSpace(result.Intent)), "query") {
			return SemanticTaskFamilyStructuredQuery
		}
		if normalizeSemanticSlotActionability(result.Actionability) == SemanticSlotActionabilityObserveOnly &&
			normalizeSemanticSlotStatus(result.SlotStatus) == SemanticSlotStatusNotApplicable {
			return SemanticTaskFamilyStructuredQuery
		}
		return SemanticTaskFamilyStructuredCommand
	default:
		if snapshot.Arbitration.SemanticIntent == SemanticIntentQuestion {
			return SemanticTaskFamilyKnowledgeQuery
		}
		return inferLexicalTaskFamily(firstNonEmpty(snapshot.PartialText, snapshot.StablePrefix))
	}
}

func hasAnyPrefix(text string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

var chineseQuestionSuffixes = []string{
	"吗",
	"呢",
	"么",
	"几",
	"多少",
	"几点",
	"几号",
	"几个",
	"几岁",
	"哪里",
	"哪儿",
	"哪边",
	"为什么",
	"怎么回事",
}

var chineseStructuredCommandPrefixes = []string{
	"打开",
	"关闭",
	"关掉",
	"启动",
	"停止",
	"暂停",
	"继续",
	"设置",
	"设成",
	"设到",
	"调成",
	"调到",
	"调亮",
	"调暗",
	"切到",
	"切换到",
	"换成",
	"改成",
	"帮我",
	"请帮我",
	"麻烦帮我",
	"把",
	"给",
}

var englishStructuredCommandPrefixes = []string{
	"turn ",
	"switch ",
	"set ",
	"change ",
	"open ",
	"close ",
	"pause ",
	"resume ",
	"start ",
	"stop ",
	"please ",
}

var chineseStructuredQueryPrefixes = []string{
	"查询",
	"查一下",
	"查查",
	"帮我查",
	"帮我看",
	"看看",
	"告诉我",
}

var englishStructuredQueryPrefixes = []string{
	"check ",
	"query ",
	"show ",
	"tell me ",
	"what's ",
	"whats ",
}
