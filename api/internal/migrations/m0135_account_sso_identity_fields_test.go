package migrations

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestM0135_account_sso_identity_fields_AddsLookupColumnAndUniqueIndexes(t *testing.T) {
	db := openLegacyAccountSSOMigrationTestDB(t)

	require.NoError(t, M0135_account_sso_identity_fields().Migrate(db))

	require.True(t, db.Migrator().HasColumn("accounts", "mobile_e164"))
	require.True(t, db.Migrator().HasIndex("accounts", "idx_accounts_email_unique_nonempty"))
	require.True(t, db.Migrator().HasIndex("accounts", "idx_accounts_mobile_e164_unique"))

	now := time.Now().UTC()
	insertAccount := func(id, email, mobile string) error {
		return db.Exec(
			`INSERT INTO accounts (id, name, email, status, created_at, updated_at, last_active_at, mobile_e164) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id,
			"SSO User",
			email,
			"active",
			now,
			now,
			now,
			mobile,
		).Error
	}

	require.NoError(t, insertAccount("acc-1", "User@Example.com", ""))
	require.Error(t, insertAccount("acc-2", "user@example.com", ""))
	require.NoError(t, insertAccount("acc-3", "", ""))
	require.NoError(t, insertAccount("acc-4", "", ""))
	require.NoError(t, insertAccount("acc-5", "phone1@example.com", "+14155552671"))
	require.Error(t, insertAccount("acc-6", "phone2@example.com", "+14155552671"))
}

func openLegacyAccountSSOMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "account_sso_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE accounts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			last_active_at TIMESTAMP NOT NULL
		)
	`).Error)

	return db
}
