package voice

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	entityNamespaceSmartHome        = "smart_home"
	entityNamespaceDesktopAssistant = "desktop_assistant"

	entityTypeRoom        = "room"
	entityTypeDevice      = "device"
	entityTypeDeviceGroup = "device_group"
	entityTypeApp         = "app"
)

type SemanticSlotGrounder interface {
	GroundPreview(SemanticSlotParseRequest, SemanticSlotParseResult) SemanticSlotParseResult
}

type groundedSemanticSlotParser struct {
	inner    SemanticSlotParser
	grounder SemanticSlotGrounder
}

func NewGroundedSemanticSlotParser(inner SemanticSlotParser, grounder SemanticSlotGrounder) SemanticSlotParser {
	if inner == nil {
		return nil
	}
	if grounder == nil {
		return inner
	}
	return groundedSemanticSlotParser{inner: inner, grounder: grounder}
}

func (p groundedSemanticSlotParser) ParsePreview(ctx context.Context, req SemanticSlotParseRequest) (SemanticSlotParseResult, error) {
	result, err := p.inner.ParsePreview(ctx, req)
	if err != nil {
		return result, err
	}
	return p.grounder.GroundPreview(req, result), nil
}

type EntityCatalogItem struct {
	EntityID              string
	Namespace             string
	EntityType            string
	CanonicalName         string
	RoomID                string
	DeviceGroup           string
	Aliases               []string
	CommonMisrecognitions []string
}

type EntityCatalog struct {
	items []EntityCatalogItem
}

func NewEntityCatalog(items []EntityCatalogItem) EntityCatalog {
	cloned := make([]EntityCatalogItem, 0, len(items))
	for _, item := range items {
		trimmed := EntityCatalogItem{
			EntityID:              strings.TrimSpace(item.EntityID),
			Namespace:             strings.TrimSpace(item.Namespace),
			EntityType:            strings.TrimSpace(item.EntityType),
			CanonicalName:         strings.TrimSpace(item.CanonicalName),
			RoomID:                strings.TrimSpace(item.RoomID),
			DeviceGroup:           strings.TrimSpace(item.DeviceGroup),
			Aliases:               cloneStringSlice(item.Aliases),
			CommonMisrecognitions: cloneStringSlice(item.CommonMisrecognitions),
		}
		if trimmed.EntityID == "" || trimmed.Namespace == "" || trimmed.EntityType == "" || trimmed.CanonicalName == "" {
			continue
		}
		cloned = append(cloned, trimmed)
	}
	return EntityCatalog{items: cloned}
}

func (c EntityCatalog) Len() int {
	return len(c.items)
}

func DefaultDemoEntityCatalog() EntityCatalog {
	return NewEntityCatalog([]EntityCatalogItem{
		{
			EntityID:      "room_living_room",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeRoom,
			CanonicalName: "客厅",
			Aliases:       []string{"客厅", "大厅"},
		},
		{
			EntityID:      "room_study",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeRoom,
			CanonicalName: "书房",
			Aliases:       []string{"书房"},
		},
		{
			EntityID:      "room_bedroom",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeRoom,
			CanonicalName: "卧室",
			Aliases:       []string{"卧室", "主卧"},
		},
		{
			EntityID:      "device_group_living_room_light",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeDeviceGroup,
			CanonicalName: "客厅灯",
			RoomID:        "room_living_room",
			DeviceGroup:   "light",
			Aliases:       []string{"客厅灯", "客厅主灯", "灯"},
		},
		{
			EntityID:              "device_living_room_downlight",
			Namespace:             entityNamespaceSmartHome,
			EntityType:            entityTypeDevice,
			CanonicalName:         "客厅筒灯",
			RoomID:                "room_living_room",
			DeviceGroup:           "light",
			Aliases:               []string{"客厅筒灯", "筒灯"},
			CommonMisrecognitions: []string{"同灯"},
		},
		{
			EntityID:      "device_group_study_light",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeDeviceGroup,
			CanonicalName: "书房灯",
			RoomID:        "room_study",
			DeviceGroup:   "light",
			Aliases:       []string{"书房灯", "台灯", "灯"},
		},
		{
			EntityID:      "device_group_living_room_air_conditioner",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeDeviceGroup,
			CanonicalName: "客厅空调",
			RoomID:        "room_living_room",
			DeviceGroup:   "air_conditioner",
			Aliases:       []string{"客厅空调", "空调"},
		},
		{
			EntityID:      "device_group_bedroom_air_conditioner",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeDeviceGroup,
			CanonicalName: "卧室空调",
			RoomID:        "room_bedroom",
			DeviceGroup:   "air_conditioner",
			Aliases:       []string{"卧室空调", "主卧空调", "空调"},
		},
		{
			EntityID:      "device_group_living_room_curtain",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeDeviceGroup,
			CanonicalName: "客厅窗帘",
			RoomID:        "room_living_room",
			DeviceGroup:   "curtain",
			Aliases:       []string{"客厅窗帘", "窗帘"},
		},
		{
			EntityID:      "device_group_study_curtain",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeDeviceGroup,
			CanonicalName: "书房窗帘",
			RoomID:        "room_study",
			DeviceGroup:   "curtain",
			Aliases:       []string{"书房窗帘", "窗帘"},
		},
		{
			EntityID:      "app_vscode",
			Namespace:     entityNamespaceDesktopAssistant,
			EntityType:    entityTypeApp,
			CanonicalName: "Visual Studio Code",
			Aliases:       []string{"vscode", "vs code", "visual studio code", "代码编辑器"},
		},
		{
			EntityID:      "app_browser",
			Namespace:     entityNamespaceDesktopAssistant,
			EntityType:    entityTypeApp,
			CanonicalName: "浏览器",
			Aliases:       []string{"浏览器", "chrome", "谷歌浏览器"},
		},
		{
			EntityID:      "app_terminal",
			Namespace:     entityNamespaceDesktopAssistant,
			EntityType:    entityTypeApp,
			CanonicalName: "终端",
			Aliases:       []string{"终端", "terminal", "命令行"},
		},
	})
}

type EntityCatalogGrounder struct {
	catalog EntityCatalog
}

func NewEntityCatalogGrounder(catalog EntityCatalog) EntityCatalogGrounder {
	return EntityCatalogGrounder{catalog: catalog}
}

func NewDefaultEntityCatalogGrounder() EntityCatalogGrounder {
	return NewEntityCatalogGrounder(DefaultDemoEntityCatalog())
}

func (g EntityCatalogGrounder) CatalogSize() int {
	return g.catalog.Len()
}

func (g EntityCatalogGrounder) GroundPreview(req SemanticSlotParseRequest, result SemanticSlotParseResult) SemanticSlotParseResult {
	if g.catalog.Len() == 0 {
		return result
	}
	bestText := firstNonEmpty(req.StablePrefix, req.PartialText)
	if strings.TrimSpace(bestText) == "" {
		return result
	}
	grounded := result
	changed := false
	switch normalizeSemanticSlotDomain(result.Domain) {
	case SemanticSlotDomainSmartHome:
		grounded, changed = g.groundSmartHome(bestText, grounded)
	case SemanticSlotDomainDesktopAssistant:
		grounded, changed = g.groundDesktopAssistant(bestText, grounded)
	default:
		return result
	}
	if changed {
		grounded.Source = appendSemanticSource(grounded.Source, "entity_catalog_grounder")
	}
	return grounded
}

type entityCatalogMatch struct {
	Item     EntityCatalogItem
	Alias    string
	AliasLen int
}

func (g EntityCatalogGrounder) groundSmartHome(text string, result SemanticSlotParseResult) (SemanticSlotParseResult, bool) {
	roomMatches := mostSpecificCatalogMatches(g.matchEntities(text, entityNamespaceSmartHome, entityTypeRoom))
	targetMatches := g.matchEntities(text, entityNamespaceSmartHome, entityTypeDevice, entityTypeDeviceGroup)
	if len(roomMatches) == 1 {
		filtered := filterCatalogMatchesByRoom(targetMatches, roomMatches[0].Item.EntityID)
		if len(filtered) > 0 {
			targetMatches = filtered
		}
	}
	targetMatches = mostSpecificCatalogMatches(targetMatches)

	groundedLocation, locationAmbiguous := singleCatalogMatch(roomMatches)
	groundedTarget, targetAmbiguous := singleCatalogMatch(targetMatches)
	if groundedLocation == nil && groundedTarget == nil && !locationAmbiguous && !targetAmbiguous {
		return result, false
	}

	updated := result
	missing := semanticSlotListSet(updated.MissingSlots)
	ambiguous := semanticSlotListSet(updated.AmbiguousSlots)

	if groundedLocation != nil {
		updated.Grounded = true
		updated.CanonicalLocation = groundedLocation.CanonicalName
		delete(missing, "location")
		delete(ambiguous, "location")
	}
	if groundedTarget != nil {
		updated.Grounded = true
		updated.CanonicalTarget = groundedTarget.CanonicalName
		delete(missing, "target")
		delete(ambiguous, "target")
	}
	if locationAmbiguous {
		delete(missing, "location")
		ambiguous["location"] = struct{}{}
		updated.ClarifyNeeded = true
		updated.SlotStatus = SemanticSlotStatusAmbiguous
		updated.Actionability = SemanticSlotActionabilityClarifyNeeded
		updated.Reason = "catalog_location_ambiguous"
	}
	if targetAmbiguous {
		delete(missing, "target")
		ambiguous["target"] = struct{}{}
		updated.ClarifyNeeded = true
		updated.SlotStatus = SemanticSlotStatusAmbiguous
		updated.Actionability = SemanticSlotActionabilityClarifyNeeded
		updated.Reason = "catalog_target_ambiguous"
	}

	updated.MissingSlots = semanticSlotListFromSet(missing)
	updated.AmbiguousSlots = semanticSlotListFromSet(ambiguous)
	if groundedTarget != nil && !locationAmbiguous && !targetAmbiguous {
		if len(updated.MissingSlots) == 0 {
			updated.SlotStatus = SemanticSlotStatusComplete
			updated.Actionability = SemanticSlotActionabilityActCandidate
			updated.ClarifyNeeded = false
			if updated.Reason == "" || updated.Reason == "semantic_unknown" || strings.HasPrefix(updated.Reason, "missing_") {
				updated.Reason = "catalog_target_grounded"
			}
		} else if updated.Actionability == SemanticSlotActionabilityObserveOnly {
			updated.Actionability = SemanticSlotActionabilityDraftOK
		}
	}
	return updated, true
}

func (g EntityCatalogGrounder) groundDesktopAssistant(text string, result SemanticSlotParseResult) (SemanticSlotParseResult, bool) {
	appMatches := mostSpecificCatalogMatches(g.matchEntities(text, entityNamespaceDesktopAssistant, entityTypeApp))
	groundedApp, appAmbiguous := singleCatalogMatch(appMatches)
	if groundedApp == nil && !appAmbiguous {
		return result, false
	}

	updated := result
	missing := semanticSlotListSet(updated.MissingSlots)
	ambiguous := semanticSlotListSet(updated.AmbiguousSlots)
	if groundedApp != nil {
		updated.Grounded = true
		updated.CanonicalTarget = groundedApp.CanonicalName
		delete(missing, "target_app")
		delete(missing, "target")
		delete(ambiguous, "target_app")
		delete(ambiguous, "target")
		if len(missing) == 0 {
			updated.SlotStatus = SemanticSlotStatusComplete
			updated.Actionability = SemanticSlotActionabilityActCandidate
			updated.ClarifyNeeded = false
			if updated.Reason == "" || updated.Reason == "semantic_unknown" || strings.HasPrefix(updated.Reason, "missing_") {
				updated.Reason = "catalog_app_grounded"
			}
		}
	}
	if appAmbiguous {
		delete(missing, "target_app")
		delete(missing, "target")
		ambiguous["target_app"] = struct{}{}
		updated.ClarifyNeeded = true
		updated.SlotStatus = SemanticSlotStatusAmbiguous
		updated.Actionability = SemanticSlotActionabilityClarifyNeeded
		updated.Reason = "catalog_target_app_ambiguous"
	}
	updated.MissingSlots = semanticSlotListFromSet(missing)
	updated.AmbiguousSlots = semanticSlotListFromSet(ambiguous)
	return updated, true
}

func (g EntityCatalogGrounder) matchEntities(text, namespace string, entityTypes ...string) []entityCatalogMatch {
	normalizedText := normalizeEntityCatalogText(text)
	if normalizedText == "" {
		return nil
	}
	allowedTypes := make(map[string]struct{}, len(entityTypes))
	for _, entityType := range entityTypes {
		allowedTypes[strings.TrimSpace(entityType)] = struct{}{}
	}
	matches := make([]entityCatalogMatch, 0)
	for _, item := range g.catalog.items {
		if item.Namespace != namespace {
			continue
		}
		if _, ok := allowedTypes[item.EntityType]; !ok {
			continue
		}
		alias, aliasLen := bestEntityCatalogAlias(item, normalizedText)
		if alias == "" {
			continue
		}
		matches = append(matches, entityCatalogMatch{Item: item, Alias: alias, AliasLen: aliasLen})
	}
	return dedupeCatalogMatches(matches)
}

func bestEntityCatalogAlias(item EntityCatalogItem, normalizedText string) (string, int) {
	bestAlias := ""
	bestLen := 0
	for _, candidate := range entityCatalogCandidates(item) {
		normalized := normalizeEntityCatalogText(candidate)
		if normalized == "" || !strings.Contains(normalizedText, normalized) {
			continue
		}
		candidateLen := utf8.RuneCountInString(normalized)
		if candidateLen > bestLen {
			bestAlias = candidate
			bestLen = candidateLen
		}
	}
	return bestAlias, bestLen
}

func entityCatalogCandidates(item EntityCatalogItem) []string {
	candidates := []string{item.CanonicalName}
	candidates = append(candidates, item.Aliases...)
	candidates = append(candidates, item.CommonMisrecognitions...)
	return candidates
}

func dedupeCatalogMatches(matches []entityCatalogMatch) []entityCatalogMatch {
	if len(matches) <= 1 {
		return matches
	}
	bestByID := make(map[string]entityCatalogMatch, len(matches))
	for _, match := range matches {
		current, ok := bestByID[match.Item.EntityID]
		if !ok || match.AliasLen > current.AliasLen {
			bestByID[match.Item.EntityID] = match
		}
	}
	deduped := make([]entityCatalogMatch, 0, len(bestByID))
	for _, match := range bestByID {
		deduped = append(deduped, match)
	}
	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].AliasLen != deduped[j].AliasLen {
			return deduped[i].AliasLen > deduped[j].AliasLen
		}
		return deduped[i].Item.EntityID < deduped[j].Item.EntityID
	})
	return deduped
}

func mostSpecificCatalogMatches(matches []entityCatalogMatch) []entityCatalogMatch {
	if len(matches) <= 1 {
		return matches
	}
	maxAliasLen := 0
	for _, match := range matches {
		if match.AliasLen > maxAliasLen {
			maxAliasLen = match.AliasLen
		}
	}
	if maxAliasLen == 0 {
		return matches
	}
	filtered := make([]entityCatalogMatch, 0, len(matches))
	for _, match := range matches {
		if match.AliasLen == maxAliasLen {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func filterCatalogMatchesByRoom(matches []entityCatalogMatch, roomID string) []entityCatalogMatch {
	if roomID == "" || len(matches) == 0 {
		return matches
	}
	filtered := make([]entityCatalogMatch, 0, len(matches))
	for _, match := range matches {
		if strings.TrimSpace(match.Item.RoomID) == roomID {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func singleCatalogMatch(matches []entityCatalogMatch) (*EntityCatalogItem, bool) {
	switch len(matches) {
	case 0:
		return nil, false
	case 1:
		return &matches[0].Item, false
	default:
		return nil, true
	}
}

func normalizeEntityCatalogText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsSpace(r):
			continue
		case unicode.IsPunct(r), unicode.IsSymbol(r):
			continue
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func semanticSlotListSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range normalizeSemanticSlotList(values) {
		set[value] = struct{}{}
	}
	return set
}

func semanticSlotListFromSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func appendSemanticSource(current, suffix string) string {
	current = strings.TrimSpace(current)
	suffix = strings.TrimSpace(suffix)
	if current == "" {
		return suffix
	}
	if suffix == "" || strings.Contains(current, suffix) {
		return current
	}
	return current + "+" + suffix
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cloned = append(cloned, trimmed)
		}
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}
