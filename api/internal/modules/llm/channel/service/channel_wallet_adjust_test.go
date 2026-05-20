package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/channel/dto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openChannelWalletAdjustTestDB(t *testing.T) *gorm.DB {
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
			is_official BOOLEAN NOT NULL DEFAULT FALSE,
			balance DECIMAL(15,4) NOT NULL DEFAULT 0,
			deleted_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE channel_wallets (
			channel_id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			balance BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE channel_wallet_transactions (
			id TEXT PRIMARY KEY,
			channel_id TEXT NOT NULL,
			attempt_id TEXT,
			type TEXT NOT NULL,
			amount BIGINT NOT NULL,
			balance_before BIGINT NOT NULL,
			balance_after BIGINT NOT NULL,
			metadata TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`,
	}

	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("failed to init table: %v", err)
		}
	}

	return db
}

func seedPrivateRoute(t *testing.T, db *gorm.DB, routeID uuid.UUID, orgID uuid.UUID, balance float64) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO llm_routes(id, organization_id, type, is_official, balance, updated_at) VALUES (?, ?, 'PRIVATE', FALSE, ?, ?)`,
		routeID.String(), orgID.String(), balance, time.Now(),
	).Error; err != nil {
		t.Fatalf("failed to seed route: %v", err)
	}
}

func seedChannelWallet(t *testing.T, db *gorm.DB, routeID uuid.UUID, orgID uuid.UUID, balance int64, status string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO channel_wallets(channel_id, organization_id, balance, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		routeID.String(), orgID.String(), balance, status, time.Now(), time.Now(),
	).Error; err != nil {
		t.Fatalf("failed to seed wallet: %v", err)
	}
}

func TestAdjustChannelWallet_IncreaseExistingWallet(t *testing.T) {
	db := openChannelWalletAdjustTestDB(t)
	svc := &channelService{db: db}

	orgID := uuid.New()
	routeID := uuid.New()
	seedPrivateRoute(t, db, routeID, orgID, 100)
	seedChannelWallet(t, db, routeID, orgID, 100, "ACTIVE")

	resp, err := svc.AdjustChannelWallet(
		context.Background(),
		orgID,
		routeID,
		&dto.AdjustChannelWalletRequest{Amount: 50, Note: "manual adjust"},
	)
	if err != nil {
		t.Fatalf("AdjustChannelWallet returned error: %v", err)
	}

	if resp.BalanceBefore != 100 || resp.BalanceAfter != 150 {
		t.Fatalf("unexpected balance change: before=%d after=%d", resp.BalanceBefore, resp.BalanceAfter)
	}

	var walletBalance int64
	if err := db.Raw(`SELECT balance FROM channel_wallets WHERE channel_id = ?`, routeID.String()).Scan(&walletBalance).Error; err != nil {
		t.Fatalf("query wallet balance failed: %v", err)
	}
	if walletBalance != 150 {
		t.Fatalf("wallet balance = %d, want 150", walletBalance)
	}

	var routeBalance float64
	if err := db.Raw(`SELECT balance FROM llm_routes WHERE id = ?`, routeID.String()).Scan(&routeBalance).Error; err != nil {
		t.Fatalf("query route balance failed: %v", err)
	}
	if routeBalance != 150 {
		t.Fatalf("route balance = %v, want 150", routeBalance)
	}
}

func TestAdjustChannelWallet_CreateWalletWhenMissing(t *testing.T) {
	db := openChannelWalletAdjustTestDB(t)
	svc := &channelService{db: db}

	orgID := uuid.New()
	routeID := uuid.New()
	seedPrivateRoute(t, db, routeID, orgID, 20)

	resp, err := svc.AdjustChannelWallet(
		context.Background(),
		orgID,
		routeID,
		&dto.AdjustChannelWalletRequest{Amount: 30},
	)
	if err != nil {
		t.Fatalf("AdjustChannelWallet returned error: %v", err)
	}

	if resp.BalanceBefore != 20 || resp.BalanceAfter != 50 {
		t.Fatalf("unexpected balance change: before=%d after=%d", resp.BalanceBefore, resp.BalanceAfter)
	}

	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM channel_wallet_transactions WHERE channel_id = ?`, routeID.String()).Scan(&count).Error; err != nil {
		t.Fatalf("query tx count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("tx count = %d, want 1", count)
	}
}

func TestAdjustChannelWallet_DecreaseToDebt(t *testing.T) {
	db := openChannelWalletAdjustTestDB(t)
	svc := &channelService{db: db}

	orgID := uuid.New()
	routeID := uuid.New()
	seedPrivateRoute(t, db, routeID, orgID, 10)
	seedChannelWallet(t, db, routeID, orgID, 10, "ACTIVE")

	resp, err := svc.AdjustChannelWallet(
		context.Background(),
		orgID,
		routeID,
		&dto.AdjustChannelWalletRequest{Amount: -30, Note: "manual correction"},
	)
	if err != nil {
		t.Fatalf("AdjustChannelWallet returned error: %v", err)
	}

	if resp.BalanceBefore != 10 || resp.BalanceAfter != -20 {
		t.Fatalf("unexpected balance change: before=%d after=%d", resp.BalanceBefore, resp.BalanceAfter)
	}
	if resp.Status != "DEBT" {
		t.Fatalf("status=%s, want DEBT", resp.Status)
	}
}

func TestAdjustChannelWallet_RejectZeroAmount(t *testing.T) {
	db := openChannelWalletAdjustTestDB(t)
	svc := &channelService{db: db}

	orgID := uuid.New()
	routeID := uuid.New()
	seedPrivateRoute(t, db, routeID, orgID, 10)

	_, err := svc.AdjustChannelWallet(
		context.Background(),
		orgID,
		routeID,
		&dto.AdjustChannelWalletRequest{Amount: 0},
	)
	if err == nil {
		t.Fatalf("expected zero-amount adjustment to fail")
	}
	if !strings.Contains(err.Error(), "must not be zero") {
		t.Fatalf("unexpected error: %v", err)
	}
}
