package voice

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	SemanticRiskLevelUnknown = "unknown"
	SemanticRiskLevelLow     = "low"
	SemanticRiskLevelMedium  = "medium"
	SemanticRiskLevelHigh    = "high"
)

var (
	slotTemperaturePattern = regexp.MustCompile(`([0-9]{1,2}|[零一二三四五六七八九十两]{1,4})(?:\s*)(?:度|℃)`)
	slotPercentPattern     = regexp.MustCompile(`(?:百分之\s*)?([0-9]{1,3}|[零一二三四五六七八九十百两]{1,5})(?:\s*)(?:%|％)?`)
)

type normalizedSemanticValue struct {
	Value string
	Unit  string
}

// postProcessSemanticSlotResult keeps the mechanism/data boundary explicit:
//   - normalization may apply small seed-domain rules for currently supported
//     domains such as smart_home
//   - risk gating stays generic and only consumes abstract risk annotations
//     already attached by runtime-owned catalog/policy data
func postProcessSemanticSlotResult(req SemanticSlotParseRequest, result SemanticSlotParseResult) SemanticSlotParseResult {
	result = normalizeSemanticSlotValue(req, result)
	result = applySemanticSlotRiskGating(req, result)
	return result
}

func normalizeSemanticSlotValue(req SemanticSlotParseRequest, result SemanticSlotParseResult) SemanticSlotParseResult {
	text := strings.TrimSpace(firstNonEmpty(req.PartialText, req.StablePrefix))
	if text == "" {
		return result
	}
	var normalized normalizedSemanticValue
	switch normalizeSemanticSlotDomain(result.Domain) {
	case SemanticSlotDomainSmartHome:
		normalized = detectSmartHomeNormalizedValue(text)
	default:
		return result
	}
	if normalized.Value == "" {
		return result
	}
	result.NormalizedValue = normalized.Value
	result.NormalizedValueUnit = normalized.Unit
	missing := semanticSlotListSet(result.MissingSlots)
	delete(missing, "value")
	result.MissingSlots = semanticSlotListFromSet(missing)
	if len(result.MissingSlots) == 0 && len(result.AmbiguousSlots) == 0 {
		if result.Grounded || result.CanonicalTarget != "" || result.CanonicalLocation != "" {
			result.SlotStatus = SemanticSlotStatusComplete
			if result.Actionability == SemanticSlotActionabilityObserveOnly ||
				result.Actionability == SemanticSlotActionabilityDraftOK ||
				result.Actionability == SemanticSlotActionabilityClarifyNeeded {
				result.Actionability = SemanticSlotActionabilityActCandidate
			}
			result.ClarifyNeeded = false
			if result.Reason == "" || result.Reason == "semantic_unknown" || strings.HasPrefix(result.Reason, "missing_") {
				result.Reason = "value_normalized"
			}
		}
	}
	return result
}

func detectSmartHomeNormalizedValue(text string) normalizedSemanticValue {
	text = strings.TrimSpace(text)
	if text == "" {
		return normalizedSemanticValue{}
	}
	for _, candidate := range []struct {
		Token string
		Value string
	}{
		{Token: "暖白", Value: "warm_white"},
		{Token: "冷白", Value: "cool_white"},
		{Token: "睡眠模式", Value: "sleep"},
		{Token: "观影模式", Value: "movie"},
		{Token: "自动模式", Value: "auto"},
		{Token: "静音", Value: "mute"},
	} {
		if strings.Contains(text, candidate.Token) {
			return normalizedSemanticValue{Value: candidate.Value, Unit: "mode"}
		}
	}
	if strings.Contains(text, "最亮") {
		return normalizedSemanticValue{Value: "100", Unit: "percentage"}
	}
	if strings.Contains(text, "最暗") || strings.Contains(text, "最小") {
		return normalizedSemanticValue{Value: "0", Unit: "percentage"}
	}
	if matched := slotTemperaturePattern.FindStringSubmatch(text); len(matched) == 2 {
		if value, ok := parseLooseChineseNumber(matched[1]); ok {
			return normalizedSemanticValue{Value: strconv.Itoa(value), Unit: "temperature_celsius"}
		}
	}
	if strings.Contains(text, "%") || strings.Contains(text, "％") || strings.Contains(text, "百分之") || containsAny(text, "亮度", "音量") {
		if matched := slotPercentPattern.FindStringSubmatch(text); len(matched) == 2 {
			if value, ok := parseLooseChineseNumber(matched[1]); ok {
				if value < 0 {
					value = 0
				}
				if value > 100 {
					value = 100
				}
				return normalizedSemanticValue{Value: strconv.Itoa(value), Unit: "percentage"}
			}
		}
	}
	if strings.Contains(text, "大一点") || strings.Contains(text, "高一点") {
		return normalizedSemanticValue{Value: "+10", Unit: "delta_percentage"}
	}
	if strings.Contains(text, "小一点") || strings.Contains(text, "低一点") {
		return normalizedSemanticValue{Value: "-10", Unit: "delta_percentage"}
	}
	return normalizedSemanticValue{}
}

func applySemanticSlotRiskGating(req SemanticSlotParseRequest, result SemanticSlotParseResult) SemanticSlotParseResult {
	text := strings.TrimSpace(firstNonEmpty(req.PartialText, req.StablePrefix))
	riskLevel, riskReason := detectSemanticRisk(text, result)
	if riskLevel == SemanticRiskLevelUnknown {
		return result
	}
	result.RiskLevel = strongerSemanticRiskLevel(result.RiskLevel, riskLevel)
	if result.RiskReason == "" || result.RiskReason == "semantic_unknown" {
		result.RiskReason = riskReason
	}
	if riskLevel == SemanticRiskLevelHigh {
		result.RiskConfirmRequired = true
		if result.Actionability == SemanticSlotActionabilityActCandidate ||
			result.Actionability == SemanticSlotActionabilityDraftOK {
			result.Actionability = SemanticSlotActionabilityClarifyNeeded
			result.ClarifyNeeded = true
			if result.Reason == "" || result.Reason == "semantic_unknown" || result.Reason == "catalog_target_grounded" || result.Reason == "value_normalized" {
				result.Reason = riskReason
			}
		}
	}
	return result
}

func detectSemanticRisk(_ string, result SemanticSlotParseResult) (string, string) {
	if riskLevel := normalizeSemanticRiskLevel(result.RiskLevel); riskLevel != SemanticRiskLevelUnknown {
		switch riskLevel {
		case SemanticRiskLevelHigh:
			return riskLevel, "catalog_high_risk_target"
		case SemanticRiskLevelMedium:
			return riskLevel, "catalog_medium_risk_target"
		default:
			return riskLevel, "catalog_low_risk_target"
		}
	}
	return SemanticRiskLevelUnknown, ""
}

func normalizeSemanticRiskLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SemanticRiskLevelLow:
		return SemanticRiskLevelLow
	case SemanticRiskLevelMedium:
		return SemanticRiskLevelMedium
	case SemanticRiskLevelHigh:
		return SemanticRiskLevelHigh
	default:
		return SemanticRiskLevelUnknown
	}
}

func strongerSemanticRiskLevel(left, right string) string {
	levels := map[string]int{
		SemanticRiskLevelUnknown: 0,
		SemanticRiskLevelLow:     1,
		SemanticRiskLevelMedium:  2,
		SemanticRiskLevelHigh:    3,
	}
	left = normalizeSemanticRiskLevel(left)
	right = normalizeSemanticRiskLevel(right)
	if levels[right] > levels[left] {
		return right
	}
	return left
}

func parseLooseChineseNumber(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if parsed, err := strconv.Atoi(raw); err == nil {
		return parsed, true
	}
	mapping := map[rune]int{
		'零': 0,
		'一': 1,
		'二': 2,
		'两': 2,
		'三': 3,
		'四': 4,
		'五': 5,
		'六': 6,
		'七': 7,
		'八': 8,
		'九': 9,
	}
	if raw == "十" {
		return 10, true
	}
	if strings.ContainsRune(raw, '百') {
		parts := strings.Split(raw, "百")
		hundred := 1
		if strings.TrimSpace(parts[0]) != "" {
			value, ok := mapping[[]rune(parts[0])[0]]
			if !ok {
				return 0, false
			}
			hundred = value
		}
		rest := 0
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			value, ok := parseLooseChineseNumber(parts[1])
			if !ok {
				return 0, false
			}
			rest = value
		}
		return hundred*100 + rest, true
	}
	if strings.ContainsRune(raw, '十') {
		parts := strings.Split(raw, "十")
		tens := 1
		if strings.TrimSpace(parts[0]) != "" {
			value, ok := mapping[[]rune(parts[0])[0]]
			if !ok {
				return 0, false
			}
			tens = value
		}
		ones := 0
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			value, ok := mapping[[]rune(parts[1])[0]]
			if !ok {
				return 0, false
			}
			ones = value
		}
		return tens*10 + ones, true
	}
	runes := []rune(raw)
	if len(runes) == 1 {
		value, ok := mapping[runes[0]]
		return value, ok
	}
	return 0, false
}

func containsAny(text string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}
