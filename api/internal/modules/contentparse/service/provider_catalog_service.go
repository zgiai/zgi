package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
)

const (
	ProviderCatalogSourceDefault          = "default_catalog"
	ProviderCatalogSourceDatabaseMerged   = "database_merged"
	ProviderCatalogSourceDatabaseFallback = "database_unavailable_local_only_catalog"
)

type ProviderCatalogResolver interface {
	Resolve(ctx context.Context, workspaceID *uuid.UUID) (*contracts.ParseProviderCatalog, string, error)
}

type providerCatalogResolver struct {
	repo     repository.ProviderConfigRepository
	fallback *contracts.ParseProviderCatalog
}

func NewProviderCatalogResolver(repo repository.ProviderConfigRepository, fallback *contracts.ParseProviderCatalog) ProviderCatalogResolver {
	return &providerCatalogResolver{
		repo:     repo,
		fallback: cloneParseProviderCatalog(fallback),
	}
}

func (s *providerCatalogResolver) Resolve(ctx context.Context, workspaceID *uuid.UUID) (*contracts.ParseProviderCatalog, string, error) {
	if s == nil {
		return &contracts.ParseProviderCatalog{}, ProviderCatalogSourceDefault, nil
	}
	base := cloneParseProviderCatalog(s.fallback)
	if s.repo == nil {
		return base, ProviderCatalogSourceDefault, nil
	}

	systemItems, err := s.repo.ListByScope(ctx, "system", nil)
	if err != nil {
		return localOnlyProviderCatalog(base), ProviderCatalogSourceDatabaseFallback, nil
	}
	workspaceItems := []*model.ProviderConfig{}
	if workspaceID != nil {
		workspaceItems, err = s.repo.ListByScope(ctx, "workspace", workspaceID)
		if err != nil {
			return localOnlyProviderCatalog(base), ProviderCatalogSourceDatabaseFallback, nil
		}
	}
	if len(systemItems) == 0 && len(workspaceItems) == 0 {
		return base, ProviderCatalogSourceDefault, nil
	}

	index := map[string]contracts.ParseProviderConfig{}
	for _, provider := range base.Providers {
		index[provider.Name] = provider
	}
	applyProviderConfigItems(index, systemItems, base)
	applyProviderConfigItems(index, workspaceItems, base)

	out := &contracts.ParseProviderCatalog{Providers: make([]contracts.ParseProviderConfig, 0, len(index))}
	for _, provider := range index {
		out.Providers = append(out.Providers, provider)
	}
	sort.SliceStable(out.Providers, func(i, j int) bool {
		if out.Providers[i].Priority != out.Providers[j].Priority {
			return out.Providers[i].Priority < out.Providers[j].Priority
		}
		return out.Providers[i].Name < out.Providers[j].Name
	})
	return out, ProviderCatalogSourceDatabaseMerged, nil
}

func localOnlyProviderCatalog(catalog *contracts.ParseProviderCatalog) *contracts.ParseProviderCatalog {
	out := &contracts.ParseProviderCatalog{}
	for _, provider := range safeProviderCatalog(catalog).Providers {
		if provider.Engine != contracts.ParseEngineLocal && provider.Name != "local" {
			continue
		}
		clone := provider
		clone.Metadata = cloneProviderMetadata(provider.Metadata)
		clone.Metadata["catalog_fallback_reason"] = "provider_config_unavailable"
		clone.Enabled = true
		out.Providers = append(out.Providers, clone)
	}
	return out
}

func applyProviderConfigItems(index map[string]contracts.ParseProviderConfig, items []*model.ProviderConfig, fallback *contracts.ParseProviderCatalog) {
	for _, item := range items {
		if item == nil {
			continue
		}
		key := strings.TrimSpace(item.ProviderKey)
		if key == "" {
			continue
		}
		base := index[key]
		index[key] = providerConfigToCatalogItem(item, base, fallback)
	}
}

func providerConfigToCatalogItem(item *model.ProviderConfig, base contracts.ParseProviderConfig, fallback *contracts.ParseProviderCatalog) contracts.ParseProviderConfig {
	out := base
	if out.Name == "" {
		out.Name = strings.TrimSpace(item.ProviderKey)
	}
	if item.DisplayName != "" {
		out.DisplayName = item.DisplayName
	}
	if item.ProviderType != "" {
		out.Type = contracts.ParseProviderType(item.ProviderType)
	}
	if item.Priority != 0 {
		out.Priority = item.Priority
	}
	if item.AdapterName != "" {
		out.Adapter = item.AdapterName
	}
	if item.EngineName != "" {
		out.Engine = contracts.ParseEngine(item.EngineName)
	}
	if item.BaseURL != "" {
		out.BaseURL = item.BaseURL
	}
	if item.TimeoutSec != 0 {
		out.TimeoutSec = item.TimeoutSec
	}
	out.Metadata = mergeProviderMetadata(base.Metadata, item.Metadata)
	out.Metadata["provider_config_id"] = item.ID.String()
	out.Metadata["config_scope"] = item.Scope
	out.Metadata["admin_enabled"] = item.Enabled
	if item.WorkspaceID != nil {
		out.Metadata["workspace_id"] = item.WorkspaceID.String()
	}
	if apiKeyEnv := metadataString(out.Metadata, "api_key_env"); apiKeyEnv != "" {
		out.APIKeyEnv = apiKeyEnv
	}
	if fallbackOnly, ok := metadataBool(out.Metadata, "fallback_only"); ok {
		out.FallbackOnly = fallbackOnly
	}

	runtimeReady := providerRuntimeReady(out, fallback)
	out.Metadata["runtime_configured"] = runtimeReady
	out.Enabled = item.Enabled && runtimeReady
	return out
}

func providerRuntimeReady(provider contracts.ParseProviderConfig, fallback *contracts.ParseProviderCatalog) bool {
	if provider.Engine == contracts.ParseEngineLocal || provider.Name == "local" {
		return true
	}
	for _, item := range safeProviderCatalog(fallback).Providers {
		if item.Name == provider.Name {
			return item.Enabled
		}
	}
	return provider.Adapter != "" && provider.Engine == contracts.ParseEngineLocal
}

func cloneParseProviderCatalog(catalog *contracts.ParseProviderCatalog) *contracts.ParseProviderCatalog {
	out := &contracts.ParseProviderCatalog{}
	for _, provider := range safeProviderCatalog(catalog).Providers {
		clone := provider
		clone.Metadata = cloneProviderMetadata(provider.Metadata)
		out.Providers = append(out.Providers, clone)
	}
	return out
}

func safeProviderCatalog(catalog *contracts.ParseProviderCatalog) *contracts.ParseProviderCatalog {
	if catalog != nil {
		return catalog
	}
	return &contracts.ParseProviderCatalog{}
}

func mergeProviderMetadata(base, override map[string]any) map[string]any {
	out := cloneProviderMetadata(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range override {
		out[key] = value
	}
	return out
}

func cloneProviderMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func metadataBool(metadata map[string]any, key string) (bool, bool) {
	if len(metadata) == 0 {
		return false, false
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		}
	}
	return false, false
}
