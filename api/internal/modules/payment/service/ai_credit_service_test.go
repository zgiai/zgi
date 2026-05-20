package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/zgiai/ginext/internal/modules/payment/repository"
)

type fakeOfficialCreditChecker struct {
	balance int64
	err     error
}

func (f fakeOfficialCreditChecker) GetOfficialBalance(context.Context, uuid.UUID) (int64, error) {
	return f.balance, f.err
}

func TestGetMyAccountOverview_PrivateChannelFundsTrackRouteLifecycle(t *testing.T) {
	db := mustOpenAICreditSQLiteDB(t)
	now := time.Now().UTC().Truncate(time.Second)

	accountID := uuid.New()
	groupID := uuid.New()
	channelID := uuid.New()

	mustExecAICredit(t, db, `
		CREATE TABLE group_ai_credit_accounts (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			subscription_credits BIGINT NOT NULL DEFAULT 0,
			purchased_credits BIGINT NOT NULL DEFAULT 0,
			total_earned BIGINT NOT NULL DEFAULT 0,
			total_spent BIGINT NOT NULL DEFAULT 0,
			last_reset_at DATETIME NULL,
			next_reset_at DATETIME NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	mustExecAICredit(t, db, `
		CREATE TABLE channel_wallets (
			channel_id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			balance BIGINT NOT NULL,
			status TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	mustExecAICredit(t, db, `
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

	mustExecAICredit(t, db, `
		INSERT INTO group_ai_credit_accounts (
			id, account_id, group_id, subscription_credits, purchased_credits,
			total_earned, total_spent, last_reset_at, next_reset_at, created_at, updated_at
		) VALUES (?, ?, ?, 0, 0, 0, 0, NULL, NULL, ?, ?)
	`, uuid.NewString(), accountID.String(), groupID.String(), now, now)
	mustExecAICredit(t, db, `
		INSERT INTO llm_routes (id, name, currency, type, is_official, is_enabled, deleted_at)
		VALUES (?, ?, ?, 'PRIVATE', FALSE, TRUE, NULL)
	`, channelID.String(), "lifecycle-private", "USD")
	mustExecAICredit(t, db, `
		INSERT INTO channel_wallets (channel_id, organization_id, balance, status, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, channelID.String(), groupID.String(), 120, "ACTIVE", now)

	svc := NewAICreditService(
		db,
		repository.NewGroupAICreditAccountRepository(db),
		nil,
		fakeOfficialCreditChecker{balance: 888},
		repository.NewPrivateChannelFundRepository(db),
	)

	assertOverviewPrivateFunds(t, svc, accountID, groupID, 120, 1)

	mustExecAICredit(t, db, `UPDATE llm_routes SET is_enabled = FALSE WHERE id = ?`, channelID.String())
	assertOverviewPrivateFunds(t, svc, accountID, groupID, 0, 0)

	mustExecAICredit(t, db, `UPDATE llm_routes SET is_enabled = TRUE WHERE id = ?`, channelID.String())
	assertOverviewPrivateFunds(t, svc, accountID, groupID, 120, 1)

	mustExecAICredit(t, db, `UPDATE llm_routes SET deleted_at = ? WHERE id = ?`, now.Add(time.Minute), channelID.String())
	assertOverviewPrivateFunds(t, svc, accountID, groupID, 0, 0)
}

func assertOverviewPrivateFunds(
	t *testing.T,
	svc *AICreditService,
	accountID uuid.UUID,
	groupID uuid.UUID,
	wantTotal int64,
	wantChannels int,
) {
	t.Helper()

	overview, err := svc.GetMyAccountOverview(context.Background(), accountID, groupID)
	if err != nil {
		t.Fatalf("GetMyAccountOverview returned error: %v", err)
	}
	if overview.OfficialAICredits.Balance != 888 {
		t.Fatalf("official balance = %d, want 888", overview.OfficialAICredits.Balance)
	}
	if overview.PrivateChannelFunds.Total != wantTotal {
		t.Fatalf("private total = %d, want %d", overview.PrivateChannelFunds.Total, wantTotal)
	}
	if len(overview.PrivateChannelFunds.Channels) != wantChannels {
		t.Fatalf("private channels len = %d, want %d", len(overview.PrivateChannelFunds.Channels), wantChannels)
	}

	var sum int64
	for _, item := range overview.PrivateChannelFunds.Channels {
		sum += item.Balance
	}
	if sum != overview.PrivateChannelFunds.Total {
		t.Fatalf("private total = %d, but channels sum = %d", overview.PrivateChannelFunds.Total, sum)
	}
}

func mustOpenAICreditSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	return db
}

func mustExecAICredit(t *testing.T, db *gorm.DB, sql string, args ...interface{}) {
	t.Helper()

	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec failed: %v\nsql=%s", err, sql)
	}
}
