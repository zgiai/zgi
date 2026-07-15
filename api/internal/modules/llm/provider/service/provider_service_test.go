package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestToggleProviderUpdatesGlobalProviderConfig(t *testing.T) {
	svc, db := newProviderTestService(t)
	ctx := context.Background()
	organizationID := uuid.New()
	providerID := uuid.New()

	require.NoError(t, db.Create(&model.LLMProvider{
		ID:           providerID,
		Provider:     "openai",
		ProviderName: "OpenAI",
		IsActive:     true,
	}).Error)

	require.NoError(t, svc.ToggleProvider(ctx, organizationID, "openai", false))

	var config model.ProviderConfig
	require.NoError(t, db.Where("organization_id = ? AND provider_id = ?", organizationID, providerID).First(&config).Error)
	require.False(t, config.IsEnabled)
}

func TestToggleProviderUpdatesCustomProviderActiveState(t *testing.T) {
	svc, db := newProviderTestService(t)
	ctx := context.Background()
	organizationID := uuid.New()
	customID := uuid.New()

	insertCustomProvider(t, db, customID, organizationID, "test1", "Test 1", true)

	require.NoError(t, svc.ToggleProvider(ctx, organizationID, "test1", false))

	var provider model.CustomProvider
	require.NoError(t, db.First(&provider, "id = ? AND organization_id = ?", customID, organizationID).Error)
	require.False(t, provider.IsActive)
}

func TestListTenantProvidersIncludesDisabledCustomProvider(t *testing.T) {
	svc, db := newProviderTestService(t)
	ctx := context.Background()
	organizationID := uuid.New()

	insertCustomProvider(t, db, uuid.New(), organizationID, "test1", "Test 1", false)

	providers, err := svc.ListTenantProviders(ctx, organizationID)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, "test1", providers[0].Name)
	require.Equal(t, "custom", providers[0].ProviderType)
	require.False(t, providers[0].IsEnabled)
}

func TestGetTenantProviderReturnsDisabledCustomProvider(t *testing.T) {
	svc, db := newProviderTestService(t)
	ctx := context.Background()
	organizationID := uuid.New()
	customID := uuid.New()

	insertCustomProvider(t, db, customID, organizationID, "test1", "Test 1", false)

	byName, err := svc.GetTenantProvider(ctx, organizationID, "test1")
	require.NoError(t, err)
	require.Equal(t, "test1", byName.Name)
	require.Equal(t, "custom", byName.ProviderType)
	require.False(t, byName.IsEnabled)

	byID, err := svc.GetTenantProvider(ctx, organizationID, customID.String())
	require.NoError(t, err)
	require.Equal(t, "test1", byID.Name)
	require.Equal(t, "custom", byID.ProviderType)
	require.False(t, byID.IsEnabled)
}

func TestToggleProviderMissingProviderReturnsProviderNotFound(t *testing.T) {
	svc, _ := newProviderTestService(t)

	err := svc.ToggleProvider(context.Background(), uuid.New(), "missing-provider", false)
	require.ErrorIs(t, err, ErrProviderNotFound)
}

func TestProviderViewsCountOnlyActiveModels(t *testing.T) {
	svc, db := newProviderTestService(t)
	organizationID := uuid.New()
	require.NoError(t, db.Create(&model.LLMProvider{
		ID:           uuid.New(),
		Provider:     "siliconflow",
		ProviderName: "SiliconFlow",
		IsActive:     true,
	}).Error)

	require.NoError(t, db.Exec(`
INSERT INTO llm_models (provider, status, is_active, deleted_at) VALUES
	('siliconflow', 'active', true, NULL),
	('siliconflow', 'deprecated', true, NULL),
	('siliconflow', 'active', false, NULL),
	('siliconflow', 'active', true, CURRENT_TIMESTAMP)
`).Error)

	providers, err := svc.ListTenantProviders(context.Background(), organizationID)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, 1, providers[0].ModelCount)

	provider, err := svc.GetTenantProvider(context.Background(), organizationID, "siliconflow")
	require.NoError(t, err)
	require.Equal(t, 1, provider.ModelCount)
}

func newProviderTestService(t *testing.T) (ProviderService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, createProviderTestTables(db))

	svc := NewProviderService(
		db,
		repository.NewProviderRepository(db),
		repository.NewProviderConfigRepository(db),
		repository.NewCustomProviderRepository(db),
		nil,
		nil,
		nil,
	)
	return svc, db
}

func createProviderTestTables(db *gorm.DB) error {
	if err := db.Exec(`
CREATE TABLE llm_providers (
	id text primary key,
	object text,
	provider text not null unique,
	provider_name text not null,
	logo_url text,
	website text,
	documentation_url text,
	pricing_url text,
	country_code text,
	tagline text,
	description text,
	metadata text,
	created_at datetime,
	updated_at datetime,
	founded_year integer,
	api_base_url text,
	provider_type text,
	is_active boolean,
	sort_order integer,
	deleted_at datetime
)`).Error; err != nil {
		return err
	}

	if err := db.Exec(`
CREATE TABLE llm_provider_configs (
	id text primary key,
	organization_id text not null,
	provider_id text not null,
	is_enabled boolean,
	custom_display_name text,
	custom_api_base_url text,
	custom_logo_url text,
	sort_order integer,
	metadata text,
	created_at datetime,
	updated_at datetime,
	deleted_at datetime
)`).Error; err != nil {
		return err
	}

	if err := db.Exec(`
CREATE TABLE llm_custom_providers (
	id text primary key,
	organization_id text not null,
	provider text not null,
	provider_name text not null,
	api_base_url text,
	logo_url text,
	documentation_url text,
	description text,
	is_active boolean,
	sort_order integer,
	metadata text,
	created_at datetime,
	updated_at datetime,
	deleted_at datetime
)`).Error; err != nil {
		return err
	}

	if err := db.Exec(`
CREATE TABLE llm_routes (
	provider text,
	organization_id text,
	is_enabled boolean,
	deleted_at datetime
)`).Error; err != nil {
		return err
	}

	return db.Exec(`
CREATE TABLE llm_models (
	provider text,
	status text,
	is_active boolean,
	deleted_at datetime
)`).Error
}

func insertCustomProvider(t *testing.T, db *gorm.DB, id, organizationID uuid.UUID, provider, providerName string, isActive bool) {
	t.Helper()

	require.NoError(t, db.Exec(
		`INSERT INTO llm_custom_providers (id, organization_id, provider, provider_name, is_active) VALUES (?, ?, ?, ?, ?)`,
		id.String(),
		organizationID.String(),
		provider,
		providerName,
		isActive,
	).Error)
}
