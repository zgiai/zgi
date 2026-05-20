package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListByOrganizationID_MissingChannelWalletTableReturnsEmpty(t *testing.T) {
	db := mustOpenSQLiteDB(t)
	repo := NewPrivateChannelFundRepository(db)

	items, err := repo.ListByOrganizationID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("expected no error when channel_wallets is missing, got: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty result when channel_wallets is missing, got %d items", len(items))
	}
}

func TestListByOrganizationID_FiltersToEnabledPrivateRoutesOnly(t *testing.T) {
	db := mustOpenSQLiteDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	orgID := uuid.New()

	enabledPrivateID := uuid.New()
	disabledPrivateID := uuid.New()
	deletedPrivateID := uuid.New()
	officialID := uuid.New()
	orphanWalletID := uuid.New()

	mustExec(t, db, `
		CREATE TABLE channel_wallets (
			channel_id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			balance BIGINT NOT NULL,
			status TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	mustExec(t, db, `
		CREATE TABLE llm_routes (
			id TEXT PRIMARY KEY,
			name TEXT,
			currency TEXT,
			type TEXT NOT NULL,
			is_official BOOLEAN NOT NULL,
			is_enabled BOOLEAN NOT NULL,
			deleted_at DATETIME NULL
		)
	`)

	mustExec(t, db, `
		INSERT INTO llm_routes (id, name, currency, type, is_official, is_enabled, deleted_at)
		VALUES (?, ?, ?, 'PRIVATE', FALSE, TRUE, NULL)
	`, enabledPrivateID.String(), "enabled-private", "USD")
	mustExec(t, db, `
		INSERT INTO llm_routes (id, name, currency, type, is_official, is_enabled, deleted_at)
		VALUES (?, ?, ?, 'PRIVATE', FALSE, FALSE, NULL)
	`, disabledPrivateID.String(), "disabled-private", "USD")
	mustExec(t, db, `
		INSERT INTO llm_routes (id, name, currency, type, is_official, is_enabled, deleted_at)
		VALUES (?, ?, ?, 'PRIVATE', FALSE, TRUE, ?)
	`, deletedPrivateID.String(), "deleted-private", "USD", now)
	mustExec(t, db, `
		INSERT INTO llm_routes (id, name, currency, type, is_official, is_enabled, deleted_at)
		VALUES (?, ?, ?, 'PRIVATE', TRUE, TRUE, NULL)
	`, officialID.String(), "official-route", "USD")

	for _, tc := range []struct {
		channelID uuid.UUID
		balance   int64
		status    string
	}{
		{channelID: enabledPrivateID, balance: 120, status: "ACTIVE"},
		{channelID: disabledPrivateID, balance: 80, status: "ACTIVE"},
		{channelID: deletedPrivateID, balance: 60, status: "ACTIVE"},
		{channelID: officialID, balance: 40, status: "ACTIVE"},
		{channelID: orphanWalletID, balance: 30, status: "ACTIVE"},
	} {
		mustExec(t, db, `
			INSERT INTO channel_wallets (channel_id, organization_id, balance, status, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, tc.channelID.String(), orgID.String(), tc.balance, tc.status, now)
	}

	repo := NewPrivateChannelFundRepository(db)
	items, err := repo.ListByOrganizationID(context.Background(), orgID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 enabled private item, got %d", len(items))
	}
	if items[0].ChannelID != enabledPrivateID {
		t.Fatalf("expected enabled private route %s, got %s", enabledPrivateID, items[0].ChannelID)
	}
	if items[0].ChannelName != "enabled-private" {
		t.Fatalf("expected channel_name=enabled-private, got %q", items[0].ChannelName)
	}
	if items[0].Balance != 120 {
		t.Fatalf("expected balance=120, got %d", items[0].Balance)
	}
}

func mustOpenSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	return db
}

func mustExec(t *testing.T, db *gorm.DB, sql string, args ...interface{}) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec failed: %v\nsql=%s", err, sql)
	}
}
