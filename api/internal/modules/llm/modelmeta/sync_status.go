package modelmeta

import (
	"context"
	"fmt"
	"time"

	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

// SyncStatusResponse represents the high-level sync status
type SyncStatusResponse struct {
	HasUpdates     bool                      `json:"has_updates"`
	Degraded       bool                      `json:"degraded"`
	UpstreamSource string                    `json:"upstream_source"`
	CheckedAt      time.Time                 `json:"checked_at"`
	Providers      StatusSummary             `json:"providers"`
	Models         StatusSummary             `json:"models"`
	ProviderErrors []SyncStatusProviderError `json:"provider_errors,omitempty"`
}

type SyncStatusProviderError struct {
	Provider string `json:"provider"`
	Error    string `json:"error"`
}

// StatusSummary provides counts for each diff status
type StatusSummary struct {
	Upstream  int `json:"upstream"`
	Local     int `json:"local"`
	New       int `json:"new"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	LocalOnly int `json:"local_only"`
}

// ProviderDiffResponse is the response for provider-level diff
type ProviderDiffResponse struct {
	CheckedAt time.Time          `json:"checked_at"`
	Summary   StatusSummary      `json:"summary"`
	Items     []ProviderDiffItem `json:"items"`
}

// ProviderDiffItem represents a single provider's diff result
type ProviderDiffItem struct {
	Provider      string   `json:"provider"`
	Name          string   `json:"name"`
	Status        string   `json:"status"` // new, updated, unchanged, local_only
	ChangedFields []string `json:"changed_fields,omitempty"`
}

// localProvider holds comparable fields of a local provider row
type localProvider struct {
	Name        string `gorm:"column:provider"`
	DisplayName string `gorm:"column:provider_name"`
	LogoURL     string `gorm:"column:logo_url"`
	Website     string `gorm:"column:website"`
	Tagline     string `gorm:"column:tagline"`
	Description string `gorm:"column:description"`
	CountryCode string `gorm:"column:country_code"`
}

// GetSyncStatus returns a lightweight summary of local vs upstream differences.
func (s *Service) GetSyncStatus(ctx context.Context) (*SyncStatusResponse, error) {
	upstreamProviders, err := s.fetchProviders()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch upstream providers: %w", err)
	}

	// Count local providers
	var localProviderCount int64
	if err := s.db.WithContext(ctx).Table("llm_providers").
		Where("deleted_at IS NULL").Count(&localProviderCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count local providers: %w", err)
	}

	// Build local provider name set
	var localProviderNames []string
	if err := s.db.WithContext(ctx).Table("llm_providers").
		Where("deleted_at IS NULL").Pluck("provider", &localProviderNames).Error; err != nil {
		return nil, fmt.Errorf("failed to list local provider names: %w", err)
	}
	localSet := make(map[string]bool, len(localProviderNames))
	for _, n := range localProviderNames {
		localSet[n] = true
	}
	upstreamSet := make(map[string]bool, len(upstreamProviders))

	providerSummary := StatusSummary{
		Upstream: len(upstreamProviders),
		Local:    int(localProviderCount),
	}
	for _, up := range upstreamProviders {
		upstreamSet[up.Provider] = true
		if localSet[up.Provider] {
			providerSummary.Unchanged++
		} else {
			providerSummary.New++
		}
	}
	for _, n := range localProviderNames {
		if !upstreamSet[n] {
			providerSummary.LocalOnly++
		}
	}

	// Aggregate model stats across all upstream providers
	modelSummary := StatusSummary{}
	providerErrors := make([]SyncStatusProviderError, 0)
	for _, up := range upstreamProviders {
		ms, err := s.computeModelSummary(ctx, up.Provider)
		if err != nil {
			providerErrors = append(providerErrors, SyncStatusProviderError{
				Provider: up.Provider,
				Error:    err.Error(),
			})
			continue
		}
		modelSummary.Upstream += ms.Upstream
		modelSummary.Local += ms.Local
		modelSummary.New += ms.New
		modelSummary.Updated += ms.Updated
		modelSummary.Unchanged += ms.Unchanged
		modelSummary.LocalOnly += ms.LocalOnly
	}

	hasUpdates := providerSummary.New > 0 ||
		modelSummary.New > 0 ||
		modelSummary.Updated > 0 ||
		modelSummary.LocalOnly > 0

	return &SyncStatusResponse{
		HasUpdates:     hasUpdates,
		Degraded:       len(providerErrors) > 0,
		UpstreamSource: s.apiBaseURL,
		CheckedAt:      time.Now(),
		Providers:      providerSummary,
		Models:         modelSummary,
		ProviderErrors: providerErrors,
	}, nil
}

// DiffProviders returns a detailed diff of all providers (local vs upstream).
func (s *Service) DiffProviders(ctx context.Context) (*ProviderDiffResponse, error) {
	upstreamProviders, err := s.fetchProviders()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch upstream providers: %w", err)
	}

	// Fetch local providers
	var locals []localProvider
	if err := s.db.WithContext(ctx).Table("llm_providers").
		Where("deleted_at IS NULL").
		Find(&locals).Error; err != nil {
		return nil, fmt.Errorf("failed to list local providers: %w", err)
	}

	localMap := make(map[string]*localProvider, len(locals))
	for i := range locals {
		localMap[locals[i].Name] = &locals[i]
	}
	upstreamSet := make(map[string]bool, len(upstreamProviders))

	summary := StatusSummary{
		Upstream: len(upstreamProviders),
		Local:    len(locals),
	}
	items := make([]ProviderDiffItem, 0, len(upstreamProviders)+len(locals))

	for _, up := range upstreamProviders {
		upstreamSet[up.Provider] = true
		local, exists := localMap[up.Provider]

		item := ProviderDiffItem{
			Provider: up.Provider,
			Name:     up.Name,
		}

		if !exists {
			item.Status = "new"
			summary.New++
		} else {
			changed := diffProviderFields(local, &up)
			if len(changed) > 0 {
				item.Status = "updated"
				item.ChangedFields = changed
				summary.Updated++
			} else {
				item.Status = "unchanged"
				summary.Unchanged++
			}
		}
		items = append(items, item)
	}

	for _, lp := range locals {
		if !upstreamSet[lp.Name] {
			items = append(items, ProviderDiffItem{
				Provider: lp.Name,
				Name:     lp.DisplayName,
				Status:   "local_only",
			})
			summary.LocalOnly++
		}
	}

	return &ProviderDiffResponse{
		CheckedAt: time.Now(),
		Summary:   summary,
		Items:     items,
	}, nil
}

// computeModelSummary computes a lightweight model diff summary for one provider
func (s *Service) computeModelSummary(ctx context.Context, provider string) (StatusSummary, error) {
	remoteModels, err := s.fetchProviderModels(provider)
	if err != nil {
		return StatusSummary{}, err
	}

	var localModels []llmmodel.LLMModel
	if err := s.db.WithContext(ctx).
		Where("provider = ? AND status = ? AND deleted_at IS NULL", provider, llmmodel.ModelStatusActive).
		Find(&localModels).Error; err != nil {
		return StatusSummary{}, err
	}

	localMap := make(map[string]*llmmodel.LLMModel, len(localModels))
	for i := range localModels {
		localMap[localModels[i].Model] = &localModels[i]
	}
	upstreamSet := make(map[string]bool, len(remoteModels))

	summary := StatusSummary{
		Upstream: len(remoteModels),
		Local:    len(localModels),
	}

	for _, rm := range remoteModels {
		upstreamSet[rm.Model] = true
		local, exists := localMap[rm.Model]
		if !exists {
			summary.New++
		} else {
			if s.hasChanges(local, &rm) {
				summary.Updated++
			} else {
				summary.Unchanged++
			}
		}
	}

	for _, m := range localModels {
		if !upstreamSet[m.Model] {
			summary.LocalOnly++
		}
	}

	return summary, nil
}

// diffProviderFields compares local provider fields against upstream
func diffProviderFields(local *localProvider, upstream *ModelMetaProvider) []string {
	var changed []string
	if local.DisplayName != upstream.Name {
		changed = append(changed, "display_name")
	}
	if local.LogoURL != upstream.LogoURL {
		changed = append(changed, "logo_url")
	}
	if local.Website != upstream.Website {
		changed = append(changed, "website")
	}
	if local.Tagline != upstream.Tagline {
		changed = append(changed, "tagline")
	}
	if local.Description != upstream.Description {
		changed = append(changed, "description")
	}
	if local.CountryCode != upstream.CountryCode {
		changed = append(changed, "country_code")
	}
	return changed
}
