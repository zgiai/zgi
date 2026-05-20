package repository

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSSOGetAccountIntegrateByProviderOpenID(t *testing.T) {
	t.Parallel()

	repo, db := setupAccountRepositorySSOTest(t)

	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO accounts (id, name, email, status, created_at, updated_at, last_active_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"acc-1",
		"Test User",
		"user@example.com",
		string(auth_model.AccountStatusActive),
		now,
		now,
		now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO account_integrates (id, account_id, provider, open_id, encrypted_token, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"int-1",
		"acc-1",
		string(auth_model.ProviderOIDC),
		"oidc-sub-1",
		"",
		now,
		now,
	).Error)

	integration, err := repo.GetAccountIntegrateByProviderOpenID(t.Context(), auth_model.ProviderOIDC, "oidc-sub-1")
	require.NoError(t, err)
	require.Equal(t, "acc-1", integration.AccountID)
	require.Equal(t, "oidc-sub-1", integration.OpenID)
}

func TestSSOGetAccountByNormalizedMobile(t *testing.T) {
	t.Parallel()

	repo, db := setupAccountRepositorySSOTest(t)

	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO accounts (id, name, email, mobile_e164, status, created_at, updated_at, last_active_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"acc-2",
		"Phone User",
		"",
		"+14155552671",
		string(auth_model.AccountStatusActive),
		now,
		now,
		now,
	).Error)

	account, err := repo.GetAccountByNormalizedMobile(t.Context(), "+14155552671")
	require.NoError(t, err)
	require.Equal(t, "acc-2", account.ID)
	require.Equal(t, "+14155552671", *account.MobileE164)
}

func TestSSOGetAccountByEmailIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	repo, db := setupAccountRepositorySSOTest(t)

	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO accounts (id, name, email, status, created_at, updated_at, last_active_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"acc-3",
		"Email User",
		"User@Example.com",
		string(auth_model.AccountStatusActive),
		now,
		now,
		now,
	).Error)

	account, err := repo.GetAccountByEmail(t.Context(), "user@example.com")
	require.NoError(t, err)
	require.Equal(t, "acc-3", account.ID)
	require.Equal(t, "User@Example.com", account.Email)
}

func setupAccountRepositorySSOTest(t *testing.T) (AccountRepository, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("sqlite driver requires cgo in this environment")
		}
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	statements := []string{
		`CREATE TABLE accounts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			mobile_e164 TEXT,
			status TEXT NOT NULL,
			deleted_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			last_active_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE account_integrates (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			open_id TEXT NOT NULL,
			encrypted_token TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	}

	for _, stmt := range statements {
		require.NoError(t, db.Exec(stmt).Error)
	}

	return NewAccountRepository(db), db
}
