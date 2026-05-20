package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/contracts"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
)

type fakeProviderConfigRepository struct {
	system    []*model.ProviderConfig
	workspace map[uuid.UUID][]*model.ProviderConfig
	err       error
}

func (f *fakeProviderConfigRepository) Create(context.Context, *model.ProviderConfig) error {
	return nil
}

func (f *fakeProviderConfigRepository) GetByID(context.Context, uuid.UUID) (*model.ProviderConfig, error) {
	return nil, nil
}

func (f *fakeProviderConfigRepository) GetByScopeAndKey(context.Context, string, *uuid.UUID, string) (*model.ProviderConfig, error) {
	return nil, nil
}

func (f *fakeProviderConfigRepository) ListByScope(_ context.Context, scope string, workspaceID *uuid.UUID) ([]*model.ProviderConfig, error) {
	if f.err != nil {
		return nil, f.err
	}
	if scope == "workspace" && workspaceID != nil {
		return f.workspace[*workspaceID], nil
	}
	if scope == "system" && workspaceID == nil {
		return f.system, nil
	}
	return nil, nil
}

func (f *fakeProviderConfigRepository) Update(context.Context, *model.ProviderConfig) error {
	return nil
}

func (f *fakeProviderConfigRepository) Delete(context.Context, uuid.UUID) error {
	return nil
}

func TestProviderCatalogResolverMergesSystemAndWorkspaceConfigs(t *testing.T) {
	workspaceID := uuid.New()
	fallback := &contracts.ParseProviderCatalog{Providers: []contracts.ParseProviderConfig{
		{
			Name:        "local",
			DisplayName: "Local Parse",
			Type:        contracts.ParseProviderTypeBuiltin,
			Enabled:     true,
			Priority:    1000,
			Adapter:     "hyperparse_sdk",
			Engine:      contracts.ParseEngineLocal,
			Metadata:    map[string]any{"tier": "builtin"},
		},
		{
			Name:        "reducto",
			DisplayName: "Reducto",
			Type:        contracts.ParseProviderTypeBuiltin,
			Enabled:     false,
			Priority:    100,
			Adapter:     "hyperparse_sdk",
			Engine:      contracts.ParseEngineReducto,
		},
	}}
	repo := &fakeProviderConfigRepository{
		system: []*model.ProviderConfig{
			{
				ID:           uuid.New(),
				Scope:        "system",
				ProviderKey:  "local",
				DisplayName:  "Self Hosted",
				Enabled:      false,
				Priority:     900,
				AdapterName:  "hyperparse_sdk",
				EngineName:   string(contracts.ParseEngineLocal),
				ProviderType: string(contracts.ParseProviderTypeBuiltin),
			},
		},
		workspace: map[uuid.UUID][]*model.ProviderConfig{
			workspaceID: {
				{
					ID:           uuid.New(),
					Scope:        "workspace",
					WorkspaceID:  &workspaceID,
					ProviderKey:  "local",
					DisplayName:  "Workspace Local",
					Enabled:      true,
					Priority:     50,
					AdapterName:  "hyperparse_sdk",
					EngineName:   string(contracts.ParseEngineLocal),
					ProviderType: string(contracts.ParseProviderTypeBuiltin),
					Metadata:     map[string]any{"fallback_only": true},
				},
			},
		},
	}

	catalog, source, err := NewProviderCatalogResolver(repo, fallback).Resolve(context.Background(), &workspaceID)
	if err != nil {
		t.Fatalf("resolve catalog: %v", err)
	}
	if source != ProviderCatalogSourceDatabaseMerged {
		t.Fatalf("expected source %q, got %q", ProviderCatalogSourceDatabaseMerged, source)
	}
	local := findCatalogProvider(t, catalog, "local")
	if local.DisplayName != "Workspace Local" {
		t.Fatalf("expected workspace override display name, got %q", local.DisplayName)
	}
	if !local.Enabled {
		t.Fatal("expected local provider to remain runtime-enabled")
	}
	if !local.FallbackOnly {
		t.Fatal("expected fallback_only metadata to map onto catalog provider")
	}
	if local.Priority != 50 {
		t.Fatalf("expected workspace priority 50, got %d", local.Priority)
	}
	if local.Metadata["tier"] != "builtin" {
		t.Fatal("expected fallback metadata to be preserved")
	}
}

func TestProviderCatalogResolverDoesNotEnableDBOnlyRemoteWithoutRuntimeConfig(t *testing.T) {
	fallback := &contracts.ParseProviderCatalog{Providers: []contracts.ParseProviderConfig{
		{
			Name:     "reducto",
			Enabled:  false,
			Adapter:  "hyperparse_sdk",
			Engine:   contracts.ParseEngineReducto,
			Priority: 100,
		},
	}}
	repo := &fakeProviderConfigRepository{system: []*model.ProviderConfig{
		{
			ID:           uuid.New(),
			Scope:        "system",
			ProviderKey:  "reducto",
			DisplayName:  "Reducto Admin",
			Enabled:      true,
			Priority:     10,
			AdapterName:  "hyperparse_sdk",
			EngineName:   string(contracts.ParseEngineReducto),
			ProviderType: string(contracts.ParseProviderTypeBuiltin),
			BaseURL:      "https://platform.reducto.ai",
		},
	}}

	catalog, source, err := NewProviderCatalogResolver(repo, fallback).Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("resolve catalog: %v", err)
	}
	if source != ProviderCatalogSourceDatabaseMerged {
		t.Fatalf("expected database source, got %q", source)
	}
	reducto := findCatalogProvider(t, catalog, "reducto")
	if reducto.Enabled {
		t.Fatal("expected db-configured remote provider to stay disabled until runtime adapter credentials are wired")
	}
	if reducto.Metadata["admin_enabled"] != true {
		t.Fatal("expected admin_enabled metadata to keep operator intent visible")
	}
	if reducto.Metadata["runtime_configured"] != false {
		t.Fatal("expected runtime_configured=false metadata")
	}
}

func TestProviderCatalogResolverFallsBackToLocalOnlyWhenRepositoryUnavailable(t *testing.T) {
	fallback := &contracts.ParseProviderCatalog{Providers: []contracts.ParseProviderConfig{
		{
			Name:     "local",
			Enabled:  true,
			Adapter:  "hyperparse_sdk",
			Engine:   contracts.ParseEngineLocal,
			Priority: 1000,
		},
		{
			Name:     "reducto",
			Enabled:  true,
			Adapter:  "hyperparse_sdk",
			Engine:   contracts.ParseEngineReducto,
			Priority: 100,
		},
	}}
	repo := &fakeProviderConfigRepository{err: errors.New("database unavailable")}

	catalog, source, err := NewProviderCatalogResolver(repo, fallback).Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("resolve catalog: %v", err)
	}
	if source != ProviderCatalogSourceDatabaseFallback {
		t.Fatalf("expected fallback source %q, got %q", ProviderCatalogSourceDatabaseFallback, source)
	}
	if len(catalog.Providers) != 1 {
		t.Fatalf("expected local-only catalog, got %d providers", len(catalog.Providers))
	}
	local := findCatalogProvider(t, catalog, "local")
	if !local.Enabled {
		t.Fatal("expected local provider to remain enabled")
	}
	if local.Metadata["catalog_fallback_reason"] != "provider_config_unavailable" {
		t.Fatal("expected fallback reason metadata on local provider")
	}
}

func findCatalogProvider(t *testing.T, catalog *contracts.ParseProviderCatalog, key string) contracts.ParseProviderConfig {
	t.Helper()
	for _, provider := range catalog.Providers {
		if provider.Name == key {
			return provider
		}
	}
	t.Fatalf("provider %q not found", key)
	return contracts.ParseProviderConfig{}
}
