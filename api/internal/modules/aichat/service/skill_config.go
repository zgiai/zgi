package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
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

func organizationSkillConfigRows(organizationID uuid.UUID, catalog []skills.SkillDiscoveryMetadata, enabled []string) []*aichatmodel.OrganizationSkillConfig {
	enabledSet := stringSet(enabled)
	rows := make([]*aichatmodel.OrganizationSkillConfig, 0, len(catalog))
	for _, item := range catalog {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if id == "" {
			continue
		}
		_, isEnabled := enabledSet[id]
		rows = append(rows, &aichatmodel.OrganizationSkillConfig{
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

func invalidCustomSkillMetadata(item *aichatmodel.CustomSkill, loadErr error) skills.SkillDiscoveryMetadata {
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

func customSkillDisplayFromRecord(item *aichatmodel.CustomSkill) skills.SkillDisplayMetadata {
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
