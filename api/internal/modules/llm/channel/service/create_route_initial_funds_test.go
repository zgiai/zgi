package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	channeldto "github.com/zgiai/ginext/internal/modules/llm/channel/dto"
	channelrepo "github.com/zgiai/ginext/internal/modules/llm/channel/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openCreateRouteInitialFundsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("sqlite driver requires cgo in this environment")
		}
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	statements := []string{
		`CREATE TABLE llm_routes (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			type TEXT NOT NULL,
			user_credential_id TEXT,
			name TEXT NOT NULL,
			models TEXT NOT NULL DEFAULT '[]',
			native_protocols TEXT NOT NULL DEFAULT '{}',
			api_base_url TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			model_maps TEXT NOT NULL DEFAULT '{}',
			param_override TEXT NOT NULL DEFAULT '{}',
			header_override TEXT NOT NULL DEFAULT '{}',
			validation_report TEXT NOT NULL DEFAULT '{}',
			tags TEXT NOT NULL DEFAULT '[]',
			description TEXT NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
			weight INTEGER NOT NULL DEFAULT 1,
			is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			is_official BOOLEAN NOT NULL DEFAULT FALSE,
			auto_ban BOOLEAN NOT NULL DEFAULT FALSE,
			sync_mode TEXT NOT NULL DEFAULT 'snapshot',
			last_synced_at DATETIME,
			balance DECIMAL(15,4) NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'USD',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
		`CREATE TABLE channel_wallets (
			channel_id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			balance BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE llm_credentials (
			id TEXT PRIMARY KEY,
			deleted_at DATETIME
		);`,
		`CREATE TABLE llm_official_model_snapshots (
			source_key TEXT PRIMARY KEY,
			effective_models TEXT NOT NULL DEFAULT '[]',
			latest_models TEXT NOT NULL DEFAULT '[]',
			previous_models TEXT NOT NULL DEFAULT '[]',
			latest_event_version BIGINT NOT NULL DEFAULT 0,
			latest_synced_at DATETIME,
			effective_updated_at DATETIME,
			last_check_status TEXT NOT NULL DEFAULT 'accepted',
			last_reject_reason TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
	}

	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("failed to init table: %v", err)
		}
	}

	return db
}

func TestCreateRoute_InitialFundsInitializesRouteBalanceAndWallet(t *testing.T) {
	db := openCreateRouteInitialFundsTestDB(t)

	svc := &channelService{
		tenantRouteRepo:   channelrepo.NewTenantRouteRepository(db),
		tenantCredService: &fakeTenantCredentialService{},
		validator:         &fakeChannelValidator{},
		modelRepo:         &fakeModelRepo{},
		db:                db,
	}

	var req channeldto.CreateRouteRequest
	if err := json.Unmarshal([]byte(`{
		"name":"private-openai",
		"channel_provider":"openai",
		"api_key":"sk-test",
		"api_base_url":"https://api.openai.com/v1",
		"models":["gpt-4o-mini"],
		"initial_funds":120
	}`), &req); err != nil {
		t.Fatalf("failed to unmarshal create request: %v", err)
	}

	orgID := uuid.New()
	view, err := svc.CreateRoute(context.Background(), orgID, &req)
	if err != nil {
		t.Fatalf("CreateRoute returned error: %v", err)
	}
	if view == nil {
		t.Fatalf("CreateRoute returned nil view")
	}
	if view.RemainingFunds != 120 {
		t.Fatalf("view remaining_funds = %d, want 120", view.RemainingFunds)
	}

	var routeBalance int64
	if err := db.Raw(`SELECT CAST(balance AS INTEGER) FROM llm_routes WHERE id = ?`, view.ID.String()).Scan(&routeBalance).Error; err != nil {
		t.Fatalf("query route balance failed: %v", err)
	}
	if routeBalance != 120 {
		t.Fatalf("route balance = %d, want 120", routeBalance)
	}

	var walletBalance int64
	if err := db.Raw(`SELECT balance FROM channel_wallets WHERE channel_id = ?`, view.ID.String()).Scan(&walletBalance).Error; err != nil {
		t.Fatalf("query wallet balance failed: %v", err)
	}
	if walletBalance != 120 {
		t.Fatalf("wallet balance = %d, want 120", walletBalance)
	}
}
