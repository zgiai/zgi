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
	llmcrypto "github.com/zgiai/zgi/api/internal/modules/llm/shared/crypto"
)

const (
	ProviderCatalogSourceDefault          = "default_catalog"
	ProviderCatalogSourceDatabaseMerged   = "database_merged"
	ProviderCatalogSourceDatabaseFallback = "database_unavailable_local_only_catalog"
)

type ProviderCatalogResolver interface {
	Resolve(ctx context.Context, organizationID, workspaceID *uuid.UUID) (*contracts.ParseProviderCatalog, string, error)
}

type providerCatalogResolver struct {
	repo     repository.ProviderConfigRepository
	fallback *contracts.ParseProviderCatalog
	crypto   llmcrypto.CryptoService
}

func NewProviderCatalogResolver(repo repository.ProviderConfigRepository, fallback *contracts.ParseProviderCatalog, crypto ...llmcrypto.CryptoService) ProviderCatalogResolver {
	var cryptoService llmcrypto.CryptoService
	if len(crypto) > 0 {
		cryptoService = crypto[0]
	}
	return &providerCatalogResolver{
		repo:     repo,
		fallback: cloneParseProviderCatalog(fallback),
		crypto:   cryptoService,
	}
}

func (s *providerCatalogResolver) Resolve(ctx context.Context, organizationID, workspaceID *uuid.UUID) (*contracts.ParseProviderCatalog, string, error) {
	if s == nil {
		return &contracts.ParseProviderCatalog{}, ProviderCatalogSourceDefault, nil
	}
	base := cloneParseProviderCatalog(s.fallback)
	if s.repo == nil {
		return base, ProviderCatalogSourceDefault, nil
	}

	systemItems, err := s.repo.ListByScope(ctx, "system", nil, nil)
	if err != nil {
		return localOnlyProviderCatalog(base), ProviderCatalogSourceDatabaseFallback, nil
	}
	organizationItems := []*model.ProviderConfig{}
	if organizationID != nil {
		organizationItems, err = s.repo.ListByScope(ctx, "organization", organizationID, nil)
		if err != nil {
			return localOnlyProviderCatalog(base), ProviderCatalogSourceDatabaseFallback, nil
		}
	}
	workspaceItems := []*model.ProviderConfig{}
	if workspaceID != nil {
		workspaceItems, err = s.repo.ListByScope(ctx, "workspace", nil, workspaceID)
		if err != nil {
			return localOnlyProviderCatalog(base), ProviderCatalogSourceDatabaseFallback, nil
		}
	}
	if len(systemItems) == 0 && len(organizationItems) == 0 && len(workspaceItems) == 0 {
		return base, ProviderCatalogSourceDefault, nil
	}

	index := map[string]contracts.ParseProviderConfig{}
	for _, provider := range base.Providers {
		index[provider.Name] = provider
	}
	applyProviderConfigItems(index, systemItems, base, s.crypto)
	applyProviderConfigItems(index, organizationItems, base, s.crypto)
	applyProviderConfigItems(index, workspaceItems, base, s.crypto)

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

func applyProviderConfigItems(index map[string]contracts.ParseProviderConfig, items []*model.ProviderConfig, fallback *contracts.ParseProviderCatalog, crypto llmcrypto.CryptoService) {
	for _, item := range items {
		if item == nil {
			continue
		}
		key := strings.TrimSpace(item.ProviderKey)
		if key == "" {
			continue
		}
		base := index[key]
		index[key] = providerConfigToCatalogItem(item, base, fallback, crypto)
	}
}

func providerConfigToCatalogItem(item *model.ProviderConfig, base contracts.ParseProviderConfig, fallback *contracts.ParseProviderCatalog, crypto llmcrypto.CryptoService) contracts.ParseProviderConfig {
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
	if item.OrganizationID != nil {
		out.Metadata["organization_id"] = item.OrganizationID.String()
	}
	if apiKeyEnv := metadataString(out.Metadata, "api_key_env"); apiKeyEnv != "" {
		out.APIKeyEnv = apiKeyEnv
	}
	if fallbackOnly, ok := metadataBool(out.Metadata, "fallback_only"); ok {
		out.FallbackOnly = fallbackOnly
	}

	envOverrides := providerRuntimeEnvOverrides(item, out, crypto)
	if len(envOverrides) > 0 {
		out.Metadata["env_overrides"] = envOverrides
	}
	runtimeReady := providerRuntimeReady(item, out, fallback, envOverrides)
	out.Metadata["runtime_configured"] = runtimeReady
	out.Enabled = item.Enabled && runtimeReady
	return out
}

func providerRuntimeReady(item *model.ProviderConfig, provider contracts.ParseProviderConfig, fallback *contracts.ParseProviderCatalog, envOverrides map[string]string) bool {
	if provider.Engine == contracts.ParseEngineLocal || provider.Name == "local" {
		return true
	}
	if provider.Name == "reducto" || provider.Engine == contracts.ParseEngineReducto {
		return strings.TrimSpace(envOverrides["REDUCTO_API_KEY"]) != ""
	}
	if provider.Name == "mineru" || provider.Engine == contracts.ParseEngineMineru {
		mode := strings.ToLower(strings.TrimSpace(metadataString(item.Metadata, "mode")))
		if mode == "official" {
			return strings.TrimSpace(envOverrides["MINERU_OFFICIAL_TOKEN"]) != ""
		}
		return strings.TrimSpace(envOverrides["MINERU_API_URL"]) != ""
	}
	for _, item := range safeProviderCatalog(fallback).Providers {
		if item.Name == provider.Name {
			return item.Enabled
		}
	}
	return provider.Adapter != "" && provider.Engine == contracts.ParseEngineLocal
}

func providerRuntimeEnvOverrides(item *model.ProviderConfig, provider contracts.ParseProviderConfig, crypto llmcrypto.CryptoService) map[string]string {
	overrides := map[string]string{}
	if item == nil {
		return overrides
	}
	switch strings.ToLower(strings.TrimSpace(item.ProviderKey)) {
	case "reducto":
		overrides["REDUCTO_ENABLED"] = boolString(item.Enabled)
		if item.BaseURL != "" {
			overrides["REDUCTO_BASE_URL"] = strings.TrimRight(strings.TrimSpace(item.BaseURL), "/")
		}
		if item.TimeoutSec > 0 {
			overrides["REDUCTO_TIMEOUT_SECONDS"] = fmt.Sprint(item.TimeoutSec)
		}
		if value := decryptCredential(item.CredentialsCiphertext, "api_key", crypto); value != "" {
			overrides["REDUCTO_API_KEY"] = value
		}
	case "mineru":
		mode := strings.ToLower(strings.TrimSpace(metadataString(item.Metadata, "mode")))
		if mode == "" {
			mode = "sidecar"
		}
		overrides["MINERU_MODE"] = mode
		if item.BaseURL != "" {
			if mode == "official" {
				overrides["MINERU_OFFICIAL_BASE_URL"] = strings.TrimRight(strings.TrimSpace(item.BaseURL), "/")
			} else {
				overrides["MINERU_API_URL"] = strings.TrimRight(strings.TrimSpace(item.BaseURL), "/")
			}
		}
		if item.TimeoutSec > 0 {
			if mode == "official" {
				overrides["MINERU_OFFICIAL_TIMEOUT_SECONDS"] = fmt.Sprint(item.TimeoutSec)
			} else {
				overrides["MINERU_TIMEOUT_SECONDS"] = fmt.Sprint(item.TimeoutSec)
			}
		}
		if mode == "official" {
			if value := decryptCredential(item.CredentialsCiphertext, "official_token", crypto); value != "" {
				overrides["MINERU_OFFICIAL_TOKEN"] = value
			}
			if value := metadataString(item.Metadata, "official_model_version"); value != "" {
				overrides["MINERU_OFFICIAL_MODEL_VERSION"] = value
			}
			if value := metadataString(item.Metadata, "official_poll_interval_seconds"); value != "" {
				overrides["MINERU_OFFICIAL_POLL_INTERVAL_SECONDS"] = value
			}
		}
	}
	return overrides
}

func decryptCredential(credentials map[string]any, key string, crypto llmcrypto.CryptoService) string {
	if len(credentials) == 0 {
		return ""
	}
	raw := metadataString(credentials, key)
	if raw == "" {
		return ""
	}
	if crypto == nil {
		return ""
	}
	plaintext, err := crypto.Decrypt(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(plaintext)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
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
