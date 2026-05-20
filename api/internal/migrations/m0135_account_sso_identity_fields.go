package migrations

import (
	"database/sql"
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0135ID = "202603200135"

// M0135_account_sso_identity_fields makes SSO identity resolution stable:
// email stays optional-by-convention via empty string, and mobile gets a
// dedicated normalized column for lookup.
func M0135_account_sso_identity_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0135ID,
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasTable("accounts") {
				return nil
			}

			if err := addAccountsMobileColumnIfMissing(tx); err != nil {
				return err
			}
			if err := normalizeAccountsEmailDefaults(tx); err != nil {
				return err
			}
			if err := backfillAccountsMobileE164(tx); err != nil {
				return err
			}
			if err := ensureUniqueAccountEmails(tx); err != nil {
				return err
			}
			if err := ensureUniqueAccountMobiles(tx); err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func addAccountsMobileColumnIfMissing(tx *gorm.DB) error {
	if tx.Migrator().HasColumn("accounts", "mobile_e164") {
		return nil
	}

	columnType := "VARCHAR(32)"
	if tx.Dialector.Name() == "sqlite" {
		columnType = "TEXT"
	}

	return tx.Exec(fmt.Sprintf(`ALTER TABLE accounts ADD COLUMN mobile_e164 %s`, columnType)).Error
}

func normalizeAccountsEmailDefaults(tx *gorm.DB) error {
	if err := tx.Exec(`UPDATE accounts SET email = '' WHERE email IS NULL`).Error; err != nil {
		return err
	}

	if tx.Dialector.Name() != "postgres" {
		return nil
	}

	for _, stmt := range []string{
		`ALTER TABLE accounts ALTER COLUMN email SET DEFAULT ''`,
		`ALTER TABLE accounts ALTER COLUMN email SET NOT NULL`,
	} {
		if err := tx.Exec(stmt).Error; err != nil {
			return err
		}
	}

	return nil
}

func backfillAccountsMobileE164(tx *gorm.DB) error {
	if tx.Dialector.Name() != "postgres" || !tx.Migrator().HasColumn("accounts", "extensions") {
		return nil
	}

	return tx.Exec(`
		WITH candidates AS (
			SELECT
				id,
				TRIM(extensions->>'mobile') AS mobile
			FROM accounts
			WHERE mobile_e164 IS NULL
				AND COALESCE(TRIM(extensions->>'mobile'), '') <> ''
				AND TRIM(extensions->>'mobile') ~ '^\+[1-9][0-9]{1,30}$'
		),
		unique_candidates AS (
			SELECT mobile
			FROM candidates
			GROUP BY mobile
			HAVING COUNT(*) = 1
		)
		UPDATE accounts AS a
		SET mobile_e164 = c.mobile
		FROM candidates AS c
		INNER JOIN unique_candidates AS u ON u.mobile = c.mobile
		WHERE a.id = c.id
	`).Error
}

func ensureUniqueAccountEmails(tx *gorm.DB) error {
	const duplicateEmailQuery = `
		SELECT LOWER(email)
		FROM accounts
		WHERE TRIM(COALESCE(email, '')) <> ''
		GROUP BY LOWER(email)
		HAVING COUNT(*) > 1
		LIMIT 1
	`

	var duplicateEmail sql.NullString
	if err := tx.Raw(duplicateEmailQuery).Scan(&duplicateEmail).Error; err != nil {
		return err
	}
	if duplicateEmail.Valid {
		return fmt.Errorf("accounts has duplicate email %q after normalization", duplicateEmail.String)
	}

	return tx.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_email_unique_nonempty
		ON accounts (LOWER(email))
		WHERE TRIM(COALESCE(email, '')) <> ''
	`).Error
}

func ensureUniqueAccountMobiles(tx *gorm.DB) error {
	const duplicateMobileQuery = `
		SELECT mobile_e164
		FROM accounts
		WHERE TRIM(COALESCE(mobile_e164, '')) <> ''
		GROUP BY mobile_e164
		HAVING COUNT(*) > 1
		LIMIT 1
	`

	var duplicateMobile sql.NullString
	if err := tx.Raw(duplicateMobileQuery).Scan(&duplicateMobile).Error; err != nil {
		return err
	}
	if duplicateMobile.Valid {
		return fmt.Errorf("accounts has duplicate mobile_e164 %q", duplicateMobile.String)
	}

	return tx.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_mobile_e164_unique
		ON accounts (mobile_e164)
		WHERE TRIM(COALESCE(mobile_e164, '')) <> ''
	`).Error
}
