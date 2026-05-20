package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func (s *service) catalogSkillMetadata(ctx context.Context, organizationID uuid.UUID) ([]skills.SkillDiscoveryMetadata, error) {
	if s.skillRuntime == nil {
		return []skills.SkillDiscoveryMetadata{}, nil
	}
	custom, err := s.customSkillCatalogEntries(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return s.skillRuntime.ListSkillsWithCustom(ctx, custom)
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

func catalogSkillIDSet(catalog []skills.SkillDiscoveryMetadata) map[string]struct{} {
	out := make(map[string]struct{}, len(catalog))
	for _, item := range catalog {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
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
