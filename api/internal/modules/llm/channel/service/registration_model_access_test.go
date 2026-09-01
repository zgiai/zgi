package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	consoleintf "github.com/zgiai/zgi/api/internal/infra/platform/console"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	officialmodel "github.com/zgiai/zgi/api/internal/modules/llm/officialmodel"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openRegistrationModelAccessDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open model access database: %v", err)
	}
	statements := []string{
		`CREATE TABLE llm_credentials (
			id text PRIMARY KEY,
			is_active boolean NOT NULL DEFAULT true,
			deleted_at datetime
		)`,
		`CREATE TABLE llm_routes (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			type text NOT NULL,
			user_credential_id text,
			name text,
			models text DEFAULT '[]',
			provider text,
			api_base_url text,
			native_protocols text DEFAULT '{}',
			model_maps text DEFAULT '{}',
			param_override text DEFAULT '{}',
			header_override text DEFAULT '{}',
			validation_report text DEFAULT '{}',
			tags text DEFAULT '[]',
			description text,
			priority integer NOT NULL DEFAULT 0,
			weight integer NOT NULL DEFAULT 1,
			is_enabled boolean NOT NULL DEFAULT true,
			is_official boolean NOT NULL DEFAULT false,
			auto_ban boolean DEFAULT false,
			sync_mode text DEFAULT 'snapshot',
			last_synced_at datetime,
			balance numeric DEFAULT 0,
			currency text DEFAULT 'USD',
			created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at datetime
		)`,
		`CREATE TABLE llm_official_model_snapshots (
			source_key text PRIMARY KEY,
			effective_models text DEFAULT '[]',
			effective_provider_models text DEFAULT '[]',
			latest_models text DEFAULT '[]',
			previous_models text DEFAULT '[]',
			latest_event_version integer NOT NULL DEFAULT 0,
			latest_synced_at datetime,
			effective_updated_at datetime,
			last_check_status text NOT NULL DEFAULT 'accepted',
			last_reject_reason text,
			created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create model access schema: %v", err)
		}
	}
	return db
}

func TestSelfHostedSharedOrganizationReusesConfiguredRoutesWithoutPerMemberCopy(t *testing.T) {
	ctx := context.Background()
	db := openRegistrationModelAccessDB(t)
	repo := channelrepo.NewTenantRouteRepository(db)
	setupOrganizationID := uuid.New()
	otherOrganizationID := uuid.New()

	configuredRoute := &channelmodel.LLMRoute{
		OrganizationID:  setupOrganizationID,
		Type:            shared.RouteTypePrivate,
		Name:            "Setup administrator channel",
		ChannelProvider: "openai-compatible",
		Models:          []string{"shared-chat-model"},
		IsEnabled:       true,
	}
	if err := repo.Create(ctx, configuredRoute); err != nil {
		t.Fatalf("create setup organization route: %v", err)
	}

	sharedRoutes, err := repo.GetEnabledRoutes(ctx, setupOrganizationID)
	if err != nil {
		t.Fatalf("load shared organization routes: %v", err)
	}
	if len(sharedRoutes) != 1 || !sharedRoutes[0].SupportsModel("shared-chat-model") {
		t.Fatalf("shared organization routes = %#v, want configured chat route", sharedRoutes)
	}

	otherRoutes, err := repo.GetEnabledRoutes(ctx, otherOrganizationID)
	if err != nil {
		t.Fatalf("load other organization routes: %v", err)
	}
	if len(otherRoutes) != 0 {
		t.Fatalf("other organization routes = %#v, want no cross-organization route access", otherRoutes)
	}

	var routeCount int64
	if err := db.Model(&channelmodel.LLMRoute{}).Count(&routeCount).Error; err != nil {
		t.Fatalf("count routes: %v", err)
	}
	if routeCount != 1 {
		t.Fatalf("route count = %d, want reuse of the single organization route", routeCount)
	}
}

func TestCloudOfficialRouteBootstrapperCreatesOneUsableOrganizationRoute(t *testing.T) {
	ctx := context.Background()
	db := openRegistrationModelAccessDB(t)
	organizationID := uuid.New()
	bootstrapper := NewOfficialRouteBootstrapper(
		db,
		consoleintf.NewRemote("http://console.invalid", "test-only-key"),
	)
	if _, err := officialmodel.SyncFromChannels(ctx, db, []officialmodel.UpstreamChannel{{
		ID:       "official-openai",
		Provider: "openai",
		Models:   []string{"cloud-chat-model"},
	}}, officialmodel.SyncMeta{Version: 1, SyncedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed official model snapshot: %v", err)
	}

	if err := bootstrapper.InitOfficialChannel(ctx, organizationID); err != nil {
		t.Fatalf("initialize cloud official route: %v", err)
	}
	if err := bootstrapper.InitOfficialChannel(ctx, organizationID); err != nil {
		t.Fatalf("repeat cloud official route initialization: %v", err)
	}

	var routes []channelmodel.LLMRoute
	if err := db.Where("organization_id = ?", organizationID).Find(&routes).Error; err != nil {
		t.Fatalf("load cloud routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("cloud route count = %d, want one idempotent official route", len(routes))
	}
	route := routes[0]
	if route.Type != shared.RouteTypeZGICloud || !route.IsOfficial || !route.IsEnabled {
		t.Fatalf("cloud route = %#v, want enabled official ZGI_CLOUD route", route)
	}
	if route.ChannelProvider != "zgi-cloud" || route.Priority != 200 || route.Weight != 100 {
		t.Fatalf("cloud route defaults = %#v, want zgi-cloud priority 200 weight 100", route)
	}
	if route.LastSyncedAt == nil {
		t.Fatal("cloud route last_synced_at is nil after idempotent refresh")
	}

	enabledRoutes, err := channelrepo.NewTenantRouteRepository(db).GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		t.Fatalf("load hydrated cloud route: %v", err)
	}
	if len(enabledRoutes) != 1 ||
		!enabledRoutes[0].SupportsModelForProvider("openai", "cloud-chat-model") {
		t.Fatalf("hydrated cloud routes = %#v, want official snapshot model", enabledRoutes)
	}
}

func TestSelfHostedOfficialRouteBootstrapperDoesNotCreateCloudRoute(t *testing.T) {
	ctx := context.Background()
	db := openRegistrationModelAccessDB(t)
	organizationID := uuid.New()
	bootstrapper := NewOfficialRouteBootstrapper(db, consoleintf.NewStandalone())

	if err := bootstrapper.InitOfficialChannel(ctx, organizationID); err != nil {
		t.Fatalf("initialize self-hosted official route: %v", err)
	}

	var routeCount int64
	if err := db.Model(&channelmodel.LLMRoute{}).
		Where("organization_id = ?", organizationID).
		Count(&routeCount).Error; err != nil {
		t.Fatalf("count self-hosted routes: %v", err)
	}
	if routeCount != 0 {
		t.Fatalf("self-hosted route count = %d, want no cloud route", routeCount)
	}
}
