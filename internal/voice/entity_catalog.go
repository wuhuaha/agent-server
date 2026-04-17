package voice

import (
	"context"
	"sort"
	"strings"
	"sync"
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

const (
	// BuiltInEntityCatalogProfileSeedCompanion keeps current smart-home and
	// desktop-assistant seed data as an optional runtime profile instead of a
	// hardwired architecture assumption.
	BuiltInEntityCatalogProfileSeedCompanion = "seed_companion"
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

func (p groundedSemanticSlotParser) TranscriptionHintsForSession(sessionID string) TranscriptionHints {
	provider, ok := p.grounder.(TranscriptionHintProvider)
	if !ok {
		return TranscriptionHints{}
	}
	return provider.TranscriptionHintsForSession(sessionID)
}

type EntityCatalogItem struct {
	EntityID              string
	Namespace             string
	EntityType            string
	CanonicalName         string
	RoomID                string
	DeviceGroup           string
	RiskLevel             string
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
			RiskLevel:             normalizeSemanticRiskLevel(item.RiskLevel),
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

func (c EntityCatalog) itemByID(entityID string) (EntityCatalogItem, bool) {
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return EntityCatalogItem{}, false
	}
	for _, item := range c.items {
		if item.EntityID == entityID {
			return item, true
		}
	}
	return EntityCatalogItem{}, false
}

func DefaultSeedEntityCatalog() EntityCatalog {
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
			EntityID:      "device_group_entry_door_lock",
			Namespace:     entityNamespaceSmartHome,
			EntityType:    entityTypeDeviceGroup,
			CanonicalName: "入户门锁",
			DeviceGroup:   "door_lock",
			RiskLevel:     SemanticRiskLevelHigh,
			Aliases:       []string{"入户门锁", "门锁", "大门锁"},
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

func DefaultDemoEntityCatalog() EntityCatalog {
	return DefaultSeedEntityCatalog()
}

func NewBuiltInEntityCatalogGrounder(profile string) (EntityCatalogGrounder, bool) {
	switch normalizeBuiltInEntityCatalogProfile(profile) {
	case BuiltInEntityCatalogProfileSeedCompanion:
		return NewEntityCatalogGrounder(DefaultSeedEntityCatalog()), true
	default:
		return EntityCatalogGrounder{}, false
	}
}

func normalizeBuiltInEntityCatalogProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "", "off", "none", "disabled":
		return ""
	case "seed", "demo", BuiltInEntityCatalogProfileSeedCompanion:
		return BuiltInEntityCatalogProfileSeedCompanion
	default:
		return strings.ToLower(strings.TrimSpace(profile))
	}
}

type EntityCatalogGrounder struct {
	catalog EntityCatalog
	recent  *entityCatalogRecentStore
}

func NewEntityCatalogGrounder(catalog EntityCatalog) EntityCatalogGrounder {
	return EntityCatalogGrounder{
		catalog: catalog,
		recent: &entityCatalogRecentStore{
			bySession: make(map[string]entityCatalogSessionContext),
		},
	}
}

func NewDefaultEntityCatalogGrounder() EntityCatalogGrounder {
	return NewEntityCatalogGrounder(DefaultSeedEntityCatalog())
}

func (g EntityCatalogGrounder) CatalogSize() int {
	return g.catalog.Len()
}

func (g EntityCatalogGrounder) TranscriptionHintsForSession(sessionID string) TranscriptionHints {
	if g.recent == nil || strings.TrimSpace(sessionID) == "" {
		return TranscriptionHints{}
	}
	context := g.recent.load(sessionID)
	if context == nil {
		return TranscriptionHints{}
	}
	hotwords := make([]string, 0, 6)
	for _, entityID := range context.RecentEntityIDs {
		if item, ok := g.catalog.itemByID(entityID); ok {
			hotwords = appendEntityHint(hotwords, append([]string{item.CanonicalName}, item.Aliases...)...)
		}
	}
	for _, roomID := range context.RecentRoomIDs {
		if item, ok := g.catalog.itemByID(roomID); ok {
			hotwords = appendEntityHint(hotwords, append([]string{item.CanonicalName}, item.Aliases...)...)
		}
	}
	hotwords = limitDistinctHints(hotwords, 8)
	if len(hotwords) == 0 {
		return TranscriptionHints{}
	}
	return TranscriptionHints{
		Hotwords:    hotwords,
		HintPhrases: limitDistinctHints(hotwords, 4),
	}
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
		grounded, changed = g.groundSmartHome(req.SessionID, bestText, grounded)
	case SemanticSlotDomainDesktopAssistant:
		grounded, changed = g.groundDesktopAssistant(req.SessionID, bestText, grounded)
	default:
		return result
	}
	grounded = postProcessSemanticSlotResult(req, grounded)
	if changed {
		grounded.Source = appendSemanticSource(grounded.Source, "entity_catalog_grounder")
	}
	return grounded
}

type entityCatalogMatch struct {
	Item     EntityCatalogItem
	Alias    string
	AliasLen int
	Score    int
}

func (g EntityCatalogGrounder) groundSmartHome(sessionID, text string, result SemanticSlotParseResult) (SemanticSlotParseResult, bool) {
	roomMatches := mostSpecificCatalogMatches(g.matchEntities(text, entityNamespaceSmartHome, entityTypeRoom))
	targetMatches := g.matchEntities(text, entityNamespaceSmartHome, entityTypeDevice, entityTypeDeviceGroup)
	if len(roomMatches) == 1 {
		filtered := filterCatalogMatchesByRoom(targetMatches, roomMatches[0].Item.EntityID)
		if len(filtered) > 0 {
			targetMatches = filtered
		}
	}
	roomMatches = g.preferSessionContextMatches(sessionID, roomMatches)
	targetMatches = g.preferSessionContextMatches(sessionID, targetMatches)
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
		updated.RiskLevel = strongerSemanticRiskLevel(updated.RiskLevel, groundedLocation.RiskLevel)
		delete(missing, "location")
		delete(ambiguous, "location")
	}
	if groundedTarget != nil {
		updated.Grounded = true
		updated.CanonicalTarget = groundedTarget.CanonicalName
		updated.RiskLevel = strongerSemanticRiskLevel(updated.RiskLevel, groundedTarget.RiskLevel)
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
	if groundedLocation != nil || groundedTarget != nil {
		g.noteSessionGrounding(sessionID, groundedLocation, groundedTarget)
	}
	return updated, true
}

func (g EntityCatalogGrounder) groundDesktopAssistant(sessionID, text string, result SemanticSlotParseResult) (SemanticSlotParseResult, bool) {
	appMatches := mostSpecificCatalogMatches(g.matchEntities(text, entityNamespaceDesktopAssistant, entityTypeApp))
	appMatches = g.preferSessionContextMatches(sessionID, appMatches)
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
		updated.RiskLevel = strongerSemanticRiskLevel(updated.RiskLevel, groundedApp.RiskLevel)
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
	if groundedApp != nil {
		g.noteSessionGrounding(sessionID, nil, groundedApp)
	}
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
		if deduped[i].Score != deduped[j].Score {
			return deduped[i].Score > deduped[j].Score
		}
		if deduped[i].AliasLen != deduped[j].AliasLen {
			return deduped[i].AliasLen > deduped[j].AliasLen
		}
		return deduped[i].Item.EntityID < deduped[j].Item.EntityID
	})
	return deduped
}

type entityCatalogRecentStore struct {
	mu        sync.Mutex
	bySession map[string]entityCatalogSessionContext
}

type entityCatalogSessionContext struct {
	LastNamespace   string
	LastRoomID      string
	LastEntityID    string
	LastDeviceGroup string
	RecentEntityIDs []string
	RecentRoomIDs   []string
}

func (g EntityCatalogGrounder) preferSessionContextMatches(sessionID string, matches []entityCatalogMatch) []entityCatalogMatch {
	if len(matches) <= 1 || g.recent == nil || strings.TrimSpace(sessionID) == "" {
		return matches
	}
	context := g.recent.load(sessionID)
	if context == nil {
		return matches
	}
	scored := append([]entityCatalogMatch(nil), matches...)
	for index := range scored {
		scored[index].Score = scoreEntityCatalogMatch(*context, scored[index])
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		if scored[i].AliasLen != scored[j].AliasLen {
			return scored[i].AliasLen > scored[j].AliasLen
		}
		return scored[i].Item.EntityID < scored[j].Item.EntityID
	})
	if len(scored) >= 2 && scored[0].Score > scored[1].Score && scored[0].Score > 0 {
		return []entityCatalogMatch{scored[0]}
	}
	return scored
}

func scoreEntityCatalogMatch(context entityCatalogSessionContext, match entityCatalogMatch) int {
	score := 0
	if match.Item.Namespace != "" && match.Item.Namespace == context.LastNamespace {
		score += 5
	}
	if match.Item.EntityID != "" {
		for index, entityID := range context.RecentEntityIDs {
			if entityID == match.Item.EntityID {
				score += maxInt(60-(index*15), 10)
				break
			}
		}
	}
	if match.Item.RoomID != "" {
		for index, roomID := range context.RecentRoomIDs {
			if roomID == match.Item.RoomID {
				score += maxInt(30-(index*10), 10)
				break
			}
		}
	}
	if context.LastDeviceGroup != "" && match.Item.DeviceGroup == context.LastDeviceGroup {
		score += 15
	}
	if context.LastEntityID != "" && match.Item.EntityID == context.LastEntityID {
		score += 80
	}
	if context.LastRoomID != "" && match.Item.RoomID == context.LastRoomID {
		score += 20
	}
	return score
}

func (g EntityCatalogGrounder) noteSessionGrounding(sessionID string, locationItem, targetItem *EntityCatalogItem) {
	if g.recent == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	g.recent.note(sessionID, locationItem, targetItem)
}

func (s *entityCatalogRecentStore) load(sessionID string) *entityCatalogSessionContext {
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	context, ok := s.bySession[strings.TrimSpace(sessionID)]
	if !ok {
		return nil
	}
	cloned := context
	cloned.RecentEntityIDs = append([]string(nil), context.RecentEntityIDs...)
	cloned.RecentRoomIDs = append([]string(nil), context.RecentRoomIDs...)
	return &cloned
}

func (s *entityCatalogRecentStore) note(sessionID string, locationItem, targetItem *EntityCatalogItem) {
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.TrimSpace(sessionID)
	context := s.bySession[key]
	if targetItem != nil {
		context.LastNamespace = targetItem.Namespace
		context.LastEntityID = targetItem.EntityID
		context.LastDeviceGroup = targetItem.DeviceGroup
		context.RecentEntityIDs = prependUniqueString(context.RecentEntityIDs, targetItem.EntityID, 4)
		if targetItem.RoomID != "" {
			context.LastRoomID = targetItem.RoomID
			context.RecentRoomIDs = prependUniqueString(context.RecentRoomIDs, targetItem.RoomID, 4)
		}
	}
	if locationItem != nil {
		context.LastNamespace = locationItem.Namespace
		context.LastRoomID = locationItem.EntityID
		context.RecentRoomIDs = prependUniqueString(context.RecentRoomIDs, locationItem.EntityID, 4)
	}
	s.bySession[key] = context
}

func prependUniqueString(values []string, value string, maxLen int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	next := make([]string, 0, len(values)+1)
	next = append(next, value)
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			continue
		}
		next = append(next, existing)
		if maxLen > 0 && len(next) >= maxLen {
			break
		}
	}
	return next
}

func appendEntityHint(values []string, candidates ...string) []string {
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

func limitDistinctHints(values []string, maxLen int) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		filtered = append(filtered, trimmed)
		if maxLen > 0 && len(filtered) >= maxLen {
			break
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
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
