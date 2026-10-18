// Package digitalhumanpreserve converts legacy organization Connection-backed
// channel installations into installation-owned credential envelopes before
// the integrations schema is retired.
package digitalhumanpreserve

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const Version = 1

type Issue struct {
	InstallationID string `json:"installation_id"`
	Code           string `json:"code"`
}

type Item struct {
	InstallationID     string `json:"installation_id"`
	SourceConnectionID string `json:"source_connection_id"`
	SourceDigest       string `json:"source_digest"`
}

// Repair names an installation that already owns its credential while
// existing deliveries or processing markers still carry an older connection
// locator, for example after preservation by an earlier release.
type Repair struct {
	InstallationID string `json:"installation_id"`
	Deliveries     int64  `json:"deliveries"`
	Markers        int64  `json:"markers"`
}

type Report struct {
	Version int      `json:"version"`
	State   string   `json:"state"`
	Items   []Item   `json:"items"`
	Repairs []Repair `json:"repairs"`
	Issues  []Issue  `json:"issues"`
	Digest  string   `json:"digest"`
}

// locatorTables carry a copy of the installation connection locator. The
// channel scope trigger requires an outbox row to match its installation on
// every update, so the locator moves together with the installation.
var locatorTables = []string{"channel_delivery_outbox", "channel_processing_markers"}

func staleLocators(db *gorm.DB, installationID uuid.UUID) (Repair, error) {
	repair := Repair{InstallationID: installationID.String()}
	counts := []*int64{&repair.Deliveries, &repair.Markers}
	for index, table := range locatorTables {
		if !db.Migrator().HasTable(table) {
			continue
		}
		if err := db.Table(table).Where("installation_id = ? AND connection_id <> ?", installationID, installationID).Count(counts[index]).Error; err != nil {
			return repair, fmt.Errorf("inspect %s connection locators: %w", table, err)
		}
	}
	return repair, nil
}

// relocateLocators points every delivery and processing marker of an
// installation at the installation-owned credential locator.
func relocateLocators(tx *gorm.DB, installationID uuid.UUID) error {
	for _, table := range locatorTables {
		if !tx.Migrator().HasTable(table) {
			continue
		}
		if err := tx.Table(table).Where("installation_id = ? AND connection_id <> ?", installationID, installationID).
			Update("connection_id", installationID).Error; err != nil {
			return fmt.Errorf("move %s connection locators: %w", table, err)
		}
	}
	return nil
}

type installation struct {
	ID             uuid.UUID  `gorm:"column:id"`
	OrganizationID uuid.UUID  `gorm:"column:organization_id"`
	WorkspaceID    uuid.UUID  `gorm:"column:workspace_id"`
	Provider       string     `gorm:"column:provider"`
	ConnectionID   uuid.UUID  `gorm:"column:connection_id"`
	Revision       int64      `gorm:"column:revision"`
	CreatedBy      uuid.UUID  `gorm:"column:created_by"`
	UpdatedBy      uuid.UUID  `gorm:"column:updated_by"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
}

type sourceConnection struct {
	ID                   uuid.UUID `gorm:"column:id"`
	OrganizationID       uuid.UUID `gorm:"column:organization_id"`
	IntegrationID        string    `gorm:"column:integration_id"`
	CredentialSource     string    `gorm:"column:credential_source"`
	EncryptedCredentials *string   `gorm:"column:encrypted_credentials"`
	CredentialVersion    int       `gorm:"column:credential_version"`
}

type credential struct {
	InstallationID       uuid.UUID `gorm:"column:installation_id"`
	OrganizationID       uuid.UUID `gorm:"column:organization_id"`
	WorkspaceID          uuid.UUID `gorm:"column:workspace_id"`
	Provider             string    `gorm:"column:provider"`
	EncryptedCredentials string    `gorm:"column:encrypted_credentials"`
	CredentialVersion    int       `gorm:"column:credential_version"`
	CreatedBy            uuid.UUID `gorm:"column:created_by"`
	UpdatedBy            uuid.UUID `gorm:"column:updated_by"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
}

func digest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func loadSourceJSON(db *gorm.DB, id uuid.UUID) (map[string]any, error) {
	var raw string
	if err := db.Raw("SELECT to_jsonb(t) FROM public.integration_connections t WHERE id = ?", id).Scan(&raw).Error; err != nil {
		return nil, err
	}
	var result map[string]any
	if raw == "" || json.Unmarshal([]byte(raw), &result) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	return result, nil
}

func loadCredentialJSON(db *gorm.DB, id uuid.UUID) (map[string]any, error) {
	var raw string
	if err := db.Raw("SELECT to_jsonb(t) FROM public.channel_installation_credentials t WHERE installation_id = ?", id).Scan(&raw).Error; err != nil {
		return nil, err
	}
	var result map[string]any
	if raw == "" || json.Unmarshal([]byte(raw), &result) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	return result, nil
}

func inspect(ctx context.Context, db *gorm.DB, cipher credentialcrypto.CredentialCipher) (Report, []installation, error) {
	report := Report{Version: Version, State: "ready", Items: []Item{}, Repairs: []Repair{}, Issues: []Issue{}}
	if cipher == nil {
		return report, nil, fmt.Errorf("DigitalHuman credential keyring is required")
	}
	var schema struct {
		SchemaVersion int
		State         string
	}
	if err := db.WithContext(ctx).Table("connector_schema_readiness").Where("id=1").Take(&schema).Error; err != nil || schema.SchemaVersion != 1 || schema.State != "ready" {
		return report, nil, fmt.Errorf("ready Connector schema1 is required")
	}
	var installations []installation
	if err := db.WithContext(ctx).Table("channel_installations").Where("deleted_at IS NULL").Order("id ASC").Find(&installations).Error; err != nil {
		return report, nil, err
	}
	legacy := make([]installation, 0)
	for _, item := range installations {
		if item.ConnectionID == item.ID {
			var stored credential
			err := db.WithContext(ctx).Table("channel_installation_credentials").Where("installation_id = ?", item.ID).Take(&stored).Error
			if err != nil || stored.OrganizationID != item.OrganizationID || stored.WorkspaceID != item.WorkspaceID ||
				!strings.EqualFold(stored.Provider, item.Provider) || stored.CredentialVersion < 1 {
				report.Issues = append(report.Issues, Issue{InstallationID: item.ID.String(), Code: "owned_credential_missing"})
				continue
			}
			values, decryptErr := cipher.DecryptCredentials(stored.EncryptedCredentials, credentialcrypto.CredentialAAD{
				OrganizationID: stored.OrganizationID, ConnectionID: stored.InstallationID,
				IntegrationID: stored.Provider, CredentialVersion: stored.CredentialVersion,
			})
			credentialcrypto.DestroyMap(values)
			if decryptErr != nil {
				report.Issues = append(report.Issues, Issue{InstallationID: item.ID.String(), Code: "owned_credential_unreadable"})
				continue
			}
			repair, err := staleLocators(db.WithContext(ctx), item.ID)
			if err != nil {
				return report, nil, err
			}
			if repair.Deliveries > 0 || repair.Markers > 0 {
				report.Repairs = append(report.Repairs, repair)
			}
			continue
		}
		var source sourceConnection
		err := db.WithContext(ctx).Table("integration_connections").Where("id = ? AND organization_id = ?", item.ConnectionID, item.OrganizationID).Take(&source).Error
		if err != nil || source.EncryptedCredentials == nil || strings.TrimSpace(*source.EncryptedCredentials) == "" ||
			!strings.EqualFold(source.IntegrationID, item.Provider) || source.CredentialSource != "organization" || source.CredentialVersion < 1 {
			report.Issues = append(report.Issues, Issue{InstallationID: item.ID.String(), Code: "legacy_source_invalid"})
			continue
		}
		values, err := cipher.DecryptCredentials(*source.EncryptedCredentials, credentialcrypto.CredentialAAD{
			OrganizationID: source.OrganizationID, ConnectionID: source.ID,
			IntegrationID: source.IntegrationID, CredentialVersion: source.CredentialVersion,
		})
		credentialcrypto.DestroyMap(values)
		if err != nil {
			report.Issues = append(report.Issues, Issue{InstallationID: item.ID.String(), Code: "legacy_credential_unreadable"})
			continue
		}
		sourceJSON, err := loadSourceJSON(db.WithContext(ctx), source.ID)
		if err != nil {
			return report, nil, err
		}
		report.Items = append(report.Items, Item{InstallationID: item.ID.String(), SourceConnectionID: source.ID.String(), SourceDigest: digest(sourceJSON)})
		legacy = append(legacy, item)
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		return report.Issues[i].InstallationID+report.Issues[i].Code < report.Issues[j].InstallationID+report.Issues[j].Code
	})
	report.Digest = ""
	report.Digest = digest(report)
	return report, legacy, nil
}

func Plan(ctx context.Context, db *gorm.DB, cipher credentialcrypto.CredentialCipher) (Report, error) {
	var report Report
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		report, _, err = inspect(ctx, tx, cipher)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return report, err
}

func Apply(ctx context.Context, db *gorm.DB, cipher credentialcrypto.CredentialCipher, expected string) (Report, error) {
	var result Report
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout = '10s'").Error; err != nil {
			return err
		}
		var locked bool
		if err := tx.Raw("SELECT pg_try_advisory_xact_lock(hashtext('zgi:migrations'))").Scan(&locked).Error; err != nil || !locked {
			return fmt.Errorf("migration maintenance lock is unavailable")
		}
		if err := tx.Exec("LOCK TABLE public.channel_installations, public.channel_installation_credentials, public.integration_connections, public.connector_conversion_records IN ACCESS EXCLUSIVE MODE").Error; err != nil {
			return fmt.Errorf("freeze DigitalHuman channel inventory: %w", err)
		}
		for _, table := range locatorTables {
			if !tx.Migrator().HasTable(table) {
				continue
			}
			if err := tx.Exec("LOCK TABLE public." + table + " IN ACCESS EXCLUSIVE MODE").Error; err != nil {
				return fmt.Errorf("freeze DigitalHuman channel locators: %w", err)
			}
		}
		report, legacy, err := inspect(ctx, tx, cipher)
		if err != nil {
			return err
		}
		if expected == "" || expected != report.Digest || len(report.Issues) != 0 {
			return fmt.Errorf("reviewed DigitalHuman preservation report changed or contains issues")
		}
		for index := range legacy {
			item := &legacy[index]
			var source sourceConnection
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("integration_connections").Where("id = ? AND organization_id = ?", item.ConnectionID, item.OrganizationID).Take(&source).Error; err != nil {
				return err
			}
			values, err := cipher.DecryptCredentials(*source.EncryptedCredentials, credentialcrypto.CredentialAAD{
				OrganizationID: source.OrganizationID, ConnectionID: source.ID, IntegrationID: source.IntegrationID, CredentialVersion: source.CredentialVersion,
			})
			if err != nil {
				return fmt.Errorf("decrypt legacy channel credential")
			}
			envelope, err := cipher.EncryptCredentials(values, credentialcrypto.CredentialAAD{
				OrganizationID: item.OrganizationID, ConnectionID: item.ID, IntegrationID: strings.ToLower(item.Provider), CredentialVersion: source.CredentialVersion,
			})
			credentialcrypto.DestroyMap(values)
			if err != nil {
				return fmt.Errorf("seal installation channel credential")
			}
			now := time.Now().UTC()
			target := credential{InstallationID: item.ID, OrganizationID: item.OrganizationID, WorkspaceID: item.WorkspaceID,
				Provider: strings.ToLower(item.Provider), EncryptedCredentials: envelope, CredentialVersion: source.CredentialVersion,
				CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy, CreatedAt: item.CreatedAt, UpdatedAt: now}
			if err := tx.Table("channel_installation_credentials").Create(&target).Error; err != nil {
				return fmt.Errorf("create installation channel credential")
			}
			updated := tx.Table("channel_installations").Where("id = ? AND connection_id = ? AND revision = ?", item.ID, item.ConnectionID, item.Revision).
				Updates(map[string]any{"connection_id": item.ID, "revision": item.Revision + 1, "updated_at": now})
			if updated.Error != nil || updated.RowsAffected != 1 {
				return fmt.Errorf("channel installation changed during preservation")
			}
			if err := relocateLocators(tx, item.ID); err != nil {
				return err
			}
			sourceItem := report.Items[index]
			if err := tx.Table("connector_conversion_records").Create(map[string]any{
				"source_kind": "dh_channel_source", "source_id": item.ID.String() + ":" + source.ID.String(),
				"source_digest": sourceItem.SourceDigest, "destination_id": item.ID, "result": "converted",
			}).Error; err != nil {
				return fmt.Errorf("record channel source proof")
			}
			targetJSON, err := loadCredentialJSON(tx, item.ID)
			if err != nil {
				return err
			}
			if err := tx.Table("connector_conversion_records").Create(map[string]any{
				"source_kind": "dh_channel_target", "source_id": item.ID.String(),
				"source_digest": digest(targetJSON), "destination_id": item.ID, "result": "ready",
			}).Error; err != nil {
				return fmt.Errorf("record channel target proof")
			}
		}
		for _, repair := range report.Repairs {
			installationID, err := uuid.Parse(repair.InstallationID)
			if err != nil {
				return fmt.Errorf("invalid installation in reviewed repair")
			}
			if err := relocateLocators(tx, installationID); err != nil {
				return err
			}
		}
		result, _, err = inspect(ctx, tx, cipher)
		if err == nil && (len(result.Items) != 0 || len(result.Repairs) != 0 || len(result.Issues) != 0) {
			err = fmt.Errorf("DigitalHuman channel preservation left work behind")
		}
		result.State = "applied"
		result.Digest = ""
		result.Digest = digest(result)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return result, err
}

func Verify(ctx context.Context, db *gorm.DB, cipher credentialcrypto.CredentialCipher) (Report, error) {
	report, err := Plan(ctx, db, cipher)
	if err != nil {
		return report, err
	}
	report.State = "verified"
	if len(report.Items) != 0 || len(report.Repairs) != 0 || len(report.Issues) != 0 {
		return report, fmt.Errorf("DigitalHuman channel preservation is incomplete")
	}
	var targets []struct {
		SourceID      string    `gorm:"column:source_id"`
		SourceDigest  string    `gorm:"column:source_digest"`
		DestinationID uuid.UUID `gorm:"column:destination_id"`
	}
	if err := db.WithContext(ctx).Table("connector_conversion_records").Where("source_kind = ?", "dh_channel_target").Find(&targets).Error; err != nil {
		return report, err
	}
	for _, proof := range targets {
		targetJSON, targetErr := loadCredentialJSON(db.WithContext(ctx), proof.DestinationID)
		if proof.DestinationID == uuid.Nil || proof.SourceID != proof.DestinationID.String() ||
			targetErr != nil || digest(targetJSON) != proof.SourceDigest {
			return report, fmt.Errorf("DigitalHuman channel target proof is inconsistent")
		}
		var sourceProof struct {
			SourceID     string `gorm:"column:source_id"`
			SourceDigest string `gorm:"column:source_digest"`
		}
		if err := db.WithContext(ctx).Table("connector_conversion_records").Where("source_kind = ? AND source_id LIKE ? AND destination_id = ? AND result = ?", "dh_channel_source", proof.SourceID+":%", proof.DestinationID, "converted").Take(&sourceProof).Error; err != nil {
			return report, fmt.Errorf("DigitalHuman channel source proof is missing")
		}
		parts := strings.Split(sourceProof.SourceID, ":")
		if len(parts) != 2 || parts[0] != proof.SourceID {
			return report, fmt.Errorf("DigitalHuman channel source proof is inconsistent")
		}
		sourceID, parseErr := uuid.Parse(parts[1])
		sourceJSON, sourceErr := loadSourceJSON(db.WithContext(ctx), sourceID)
		if parseErr != nil || sourceErr != nil || digest(sourceJSON) != sourceProof.SourceDigest {
			return report, fmt.Errorf("DigitalHuman channel source proof is inconsistent")
		}
	}
	report.Digest = ""
	report.Digest = digest(report)
	return report, nil
}
