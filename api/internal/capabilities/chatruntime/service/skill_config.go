package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func (s *service) catalogSkillMetadata(ctx context.Context, organizationID uuid.UUID) ([]skills.SkillDiscoveryMetadata, error) {
	systemMetadata := []skills.SkillDiscoveryMetadata{}
	if s.skillRuntime != nil {
		var err error
		systemMetadata, err = s.skillRuntime.ListSkills(ctx)
		if err != nil {
			logger.WarnContext(ctx, "failed to list aichat system skills; falling back to best-effort catalog", err)
			systemMetadata, err = s.skillRuntime.ListSystemSkillsBestEffort(ctx)
			if err != nil {
				logger.WarnContext(ctx, "aichat system skill catalog has invalid entries", err)
			}
		}
	}
	customMetadata, err := s.customSkillDiscoveryMetadata(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	metadata := append(systemMetadata, customMetadata...)
	sort.Slice(metadata, func(i, j int) bool { return metadata[i].ID < metadata[j].ID })
	return metadata, nil
}

func (s *service) customSkillCatalogEntries(ctx context.Context, organizationID uuid.UUID) ([]skills.CustomSkillCatalogEntry, error) {
	if s.repos == nil || s.repos.CustomSkill == nil {
		return nil, nil
	}
	customSkills, err := s.repos.CustomSkill.ListByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	entries := make([]skills.CustomSkillCatalogEntry, 0, len(customSkills))
	for _, item := range customSkills {
		if item == nil {
			continue
		}
		entries = append(entries, skills.CustomSkillCatalogEntry{
			SkillID: item.SkillID,
			Root:    item.StoragePath,
		})
	}
	return entries, nil
}

func (s *service) customSkillDiscoveryMetadata(ctx context.Context, organizationID uuid.UUID) ([]skills.SkillDiscoveryMetadata, error) {
	if s.repos == nil || s.repos.CustomSkill == nil {
		return []skills.SkillDiscoveryMetadata{}, nil
	}
	customSkills, err := s.repos.CustomSkill.ListManageableByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	metadata := make([]skills.SkillDiscoveryMetadata, 0, len(customSkills))
	for _, item := range customSkills {
		if item == nil {
			continue
		}
		doc, err := skills.LoadCustomSkillDocument(item.StoragePath)
		if err == nil {
			loaded := skillDiscoveryMetadataPtr(doc)
			metadata = append(metadata, *loaded)
			continue
		}
		metadata = append(metadata, invalidCustomSkillMetadata(item, err))
	}
	return metadata, nil
}

func (s *service) effectiveOrganizationSkillIDs(ctx context.Context, organizationID uuid.UUID, catalog []skills.SkillDiscoveryMetadata) ([]string, error) {
	if len(catalog) == 0 {
		return []string{}, nil
	}
	configs, err := s.repos.SkillConfig.ListByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return defaultEnabledSkillIDs(catalog), nil
	}
	catalogIDs := catalogSkillIDSet(catalog)
	enabled := make([]string, 0, len(configs))
	for _, config := range configs {
		if config == nil || !config.Enabled {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(config.SkillID))
		if _, ok := catalogIDs[id]; ok {
			enabled = append(enabled, id)
		}
	}
	sort.Strings(enabled)
	return enabled, nil
}

func (s *service) effectiveAccountSkillPreferenceIDs(ctx context.Context, scope Scope, callerType string, catalog []skills.SkillDiscoveryMetadata, organizationEnabled []string) ([]string, bool, error) {
	callerType = normalizeCallerType(callerType)
	if callerType != runtimemodel.ConversationCallerAIChat {
		return []string{}, true, nil
	}
	if s.repos == nil || s.repos.SkillPref == nil {
		return filterSkillIDsForCaller(organizationEnabled, catalog, callerType), true, nil
	}
	pref, err := s.repos.SkillPref.Get(ctx, scope.OrganizationID, scope.AccountID, callerType)
	if err != nil {
		return nil, false, err
	}
	if pref == nil {
		return filterSkillIDsForCaller(organizationEnabled, catalog, callerType), true, nil
	}
	return effectiveSkillIDsForCaller(pref.EnabledSkillIDs, catalog, organizationEnabled, callerType, nil), false, nil
}

func validateSkillIDsForCaller(input []string, catalog []skills.SkillDiscoveryMetadata, organizationEnabled []string, callerType string, runConfig *RunConfig) ([]string, error) {
	callerType = normalizeCallerType(callerType)
	catalogByID := catalogSkillByID(catalog)
	orgEnabled := stringSet(organizationEnabled)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, raw := range input {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if skills.IsHiddenSystemSkill(id) {
			return nil, fmt.Errorf("%w: skill %s is managed by runtime configuration", ErrInvalidInput, id)
		}
		item, ok := catalogByID[id]
		if !ok {
			return nil, fmt.Errorf("%w: unknown skill id %s", ErrInvalidInput, id)
		}
		if _, ok := orgEnabled[id]; !ok {
			return nil, fmt.Errorf("%w: skill %s is not enabled by organization", ErrInvalidInput, id)
		}
		if !skillSupportsCaller(item, callerType) {
			return nil, fmt.Errorf("%w: skill %s is not available for %s", ErrInvalidInput, id, callerType)
		}
		if err := validateSkillRequiredConfig(item, runConfig); err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func effectiveAgentSkillIDs(input []string, catalog []skills.SkillDiscoveryMetadata, runConfig *RunConfig) []string {
	catalogByID := catalogSkillByID(catalog)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input)+1)
	for _, raw := range input {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" || skills.IsHiddenSystemSkill(id) {
			continue
		}
		item, ok := catalogByID[id]
		if !ok || !skillSupportsCaller(item, runtimemodel.ConversationCallerAgent) {
			continue
		}
		if validateSkillRequiredConfig(item, runConfig) != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if runConfigHasKnowledgeDatasets(runConfig) && agentKnowledgeAvailable(catalog) {
		id := skills.SkillAgentKnowledge
		if _, ok := seen[id]; !ok {
			out = append(out, id)
		}
	}
	if runConfigHasAgentMemory(runConfig) && agentMemoryAvailable(catalog) {
		id := skills.SkillAgentMemory
		if _, ok := seen[id]; !ok {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func filterSkillIDsForCaller(input []string, catalog []skills.SkillDiscoveryMetadata, callerType string) []string {
	callerType = normalizeCallerType(callerType)
	catalogByID := catalogSkillByID(catalog)
	out := make([]string, 0, len(input))
	for _, raw := range input {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" || skills.IsHiddenSystemSkill(id) {
			continue
		}
		item, ok := catalogByID[id]
		if !ok || !skillSupportsCaller(item, callerType) {
			continue
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func effectiveSkillIDsForCaller(input []string, catalog []skills.SkillDiscoveryMetadata, organizationEnabled []string, callerType string, runConfig *RunConfig) []string {
	callerType = normalizeCallerType(callerType)
	catalogByID := catalogSkillByID(catalog)
	orgEnabled := stringSet(organizationEnabled)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, raw := range input {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if skills.IsHiddenSystemSkill(id) {
			continue
		}
		item, ok := catalogByID[id]
		if !ok {
			continue
		}
		if _, ok := orgEnabled[id]; !ok {
			continue
		}
		if !skillSupportsCaller(item, callerType) {
			continue
		}
		if validateSkillRequiredConfig(item, runConfig) != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func skillSupportsCaller(item skills.SkillDiscoveryMetadata, callerType string) bool {
	callerType = normalizeCallerType(callerType)
	if len(item.SupportedCallers) == 0 {
		return true
	}
	for _, raw := range item.SupportedCallers {
		if strings.EqualFold(strings.TrimSpace(raw), callerType) {
			return true
		}
	}
	return false
}

func validateSkillRequiredConfig(item skills.SkillDiscoveryMetadata, runConfig *RunConfig) error {
	for _, raw := range item.RequiredConfig {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case skills.SkillRequiredConfigAgentKnowledge:
			if runConfig == nil || len(normalizedSkillIDs(runConfig.KnowledgeDatasetIDs)) == 0 {
				return fmt.Errorf("%w: skill %s requires configured knowledge datasets", ErrInvalidInput, item.ID)
			}
		}
	}
	return nil
}

func catalogSkillByID(catalog []skills.SkillDiscoveryMetadata) map[string]skills.SkillDiscoveryMetadata {
	out := make(map[string]skills.SkillDiscoveryMetadata, len(catalog))
	for _, item := range catalog {
		if item.Status == skills.SkillStatusInvalid {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if id != "" {
			out[id] = item
		}
	}
	return out
}

func (s *service) isOrganizationSkillEnabled(ctx context.Context, organizationID uuid.UUID, skillID string) bool {
	catalog, err := s.catalogSkillMetadata(ctx, organizationID)
	if err != nil {
		return false
	}
	enabled, err := s.effectiveOrganizationSkillIDs(ctx, organizationID, catalog)
	if err != nil {
		return false
	}
	enabledSet := stringSet(enabled)
	_, ok := enabledSet[strings.ToLower(strings.TrimSpace(skillID))]
	return ok
}

func validateSkillConfigIDs(input []string, catalog []skills.SkillDiscoveryMetadata) ([]string, error) {
	catalogIDs := catalogSkillIDSet(catalog)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, raw := range input {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if _, ok := catalogIDs[id]; !ok {
			return nil, fmt.Errorf("%w: unknown skill id %s", ErrInvalidInput, id)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func organizationSkillConfigRows(organizationID uuid.UUID, catalog []skills.SkillDiscoveryMetadata, enabled []string) []*runtimemodel.OrganizationSkillConfig {
	enabledSet := stringSet(enabled)
	rows := make([]*runtimemodel.OrganizationSkillConfig, 0, len(catalog))
	for _, item := range catalog {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if id == "" {
			continue
		}
		_, isEnabled := enabledSet[id]
		rows = append(rows, &runtimemodel.OrganizationSkillConfig{
			OrganizationID: organizationID,
			SkillID:        id,
			Enabled:        isEnabled,
		})
	}
	return rows
}

func markEnabledSkills(metadata []skills.SkillDiscoveryMetadata, enabled []string) {
	enabledSet := stringSet(enabled)
	for idx := range metadata {
		id := strings.ToLower(strings.TrimSpace(metadata[idx].ID))
		if metadata[idx].Status == skills.SkillStatusInvalid {
			metadata[idx].Enabled = false
			continue
		}
		_, metadata[idx].Enabled = enabledSet[id]
	}
}

func filterSkillsForModel(enabled []string, catalog []skills.SkillDiscoveryMetadata, parts *chatRequestParts) ([]string, []string) {
	if parts == nil || !parts.FunctionCallingKnown || !parts.ModelSupportsFunctionCalling {
		return []string{}, []string{}
	}
	metadataByID := map[string]skills.SkillDiscoveryMetadata{}
	for _, item := range catalog {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if id != "" {
			metadataByID[id] = item
		}
	}
	effective := make([]string, 0, len(enabled))
	toolRunnable := make([]string, 0, len(enabled))
	for _, raw := range enabled {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		metadata, ok := metadataByID[id]
		if !ok {
			continue
		}
		if metadata.Status == skills.SkillStatusInvalid {
			continue
		}
		runtimeType := strings.ToLower(strings.TrimSpace(metadata.RuntimeType))
		if runtimeType == "" {
			runtimeType = skills.SkillRuntimeTypePrompt
		}
		effective = append(effective, id)
		if runtimeType == skills.SkillRuntimeTypeTool || runtimeType == skills.SkillRuntimeTypeHybrid {
			toolRunnable = append(toolRunnable, id)
		}
	}
	sort.Strings(effective)
	sort.Strings(toolRunnable)
	return effective, toolRunnable
}

func defaultEnabledSkillIDs(catalog []skills.SkillDiscoveryMetadata) []string {
	catalogIDs := catalogSkillIDSet(catalog)
	enabled := make([]string, 0, len(defaultSystemSkillIDs))
	for _, id := range defaultSystemSkillIDs {
		normalized := strings.ToLower(strings.TrimSpace(id))
		if _, ok := catalogIDs[normalized]; ok {
			enabled = append(enabled, normalized)
		}
	}
	sort.Strings(enabled)
	return enabled
}

func visibleSkillMetadata(metadata []skills.SkillDiscoveryMetadata) []skills.SkillDiscoveryMetadata {
	out := make([]skills.SkillDiscoveryMetadata, 0, len(metadata))
	for _, item := range metadata {
		if skills.IsHiddenSystemSkill(item.ID) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func runConfigHasKnowledgeDatasets(runConfig *RunConfig) bool {
	return runConfig != nil && len(normalizedSkillIDs(runConfig.KnowledgeDatasetIDs)) > 0
}

func runConfigHasAgentMemory(runConfig *RunConfig) bool {
	return runConfig != nil && runConfig.AgentMemoryEnabled && len(normalizeAgentMemorySlots(runConfig.AgentMemorySlots)) > 0
}

func agentMemoryAvailable(catalog []skills.SkillDiscoveryMetadata) bool {
	for _, item := range catalog {
		if strings.EqualFold(strings.TrimSpace(item.ID), skills.SkillAgentMemory) && item.Status != skills.SkillStatusInvalid {
			return skillSupportsCaller(item, runtimemodel.ConversationCallerAgent)
		}
	}
	return false
}

func agentKnowledgeAvailable(catalog []skills.SkillDiscoveryMetadata) bool {
	for _, item := range catalog {
		if strings.EqualFold(strings.TrimSpace(item.ID), skills.SkillAgentKnowledge) && item.Status != skills.SkillStatusInvalid && skillSupportsCaller(item, runtimemodel.ConversationCallerAgent) {
			return true
		}
	}
	return false
}

func catalogSkillIDSet(catalog []skills.SkillDiscoveryMetadata) map[string]struct{} {
	out := make(map[string]struct{}, len(catalog))
	for _, item := range catalog {
		if item.Status == skills.SkillStatusInvalid {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func invalidCustomSkillMetadata(item *runtimemodel.CustomSkill, loadErr error) skills.SkillDiscoveryMetadata {
	validationError := strings.TrimSpace(item.ValidationError)
	if validationError == "" && loadErr != nil {
		validationError = loadErr.Error()
	}
	runtimeType := strings.TrimSpace(item.RuntimeType)
	if runtimeType == "" {
		runtimeType = skills.SkillRuntimeTypePrompt
	}
	return skills.SkillDiscoveryMetadata{
		ID:               strings.ToLower(strings.TrimSpace(item.SkillID)),
		Source:           skills.SkillSourceCustom,
		Name:             strings.TrimSpace(item.Name),
		Description:      strings.TrimSpace(item.Description),
		WhenToUse:        strings.TrimSpace(item.WhenToUse),
		Display:          customSkillDisplayFromRecord(item),
		RuntimeType:      runtimeType,
		Enabled:          false,
		HasTools:         false,
		HasReferences:    false,
		HasScripts:       boolManifestValue(item.Manifest, "has_scripts"),
		ScriptsSupported: false,
		MaxCallsPerTurn:  0,
		TimeoutSeconds:   0,
		Status:           skills.SkillStatusInvalid,
		ValidationError:  validationError,
	}
}

func customSkillDisplayFromRecord(item *runtimemodel.CustomSkill) skills.SkillDisplayMetadata {
	if item == nil || len(item.Display) == 0 {
		return skills.SkillDisplayMetadata{}
	}
	data, err := json.Marshal(item.Display)
	if err != nil {
		return skills.SkillDisplayMetadata{}
	}
	var display skills.SkillDisplayMetadata
	if err := json.Unmarshal(data, &display); err != nil {
		return skills.SkillDisplayMetadata{}
	}
	return display
}

func boolManifestValue(manifest map[string]interface{}, key string) bool {
	if manifest == nil {
		return false
	}
	value, ok := manifest[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}
