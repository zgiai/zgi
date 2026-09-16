package preferences

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// LockOwner serializes changes even when no preference row exists. Every
// default mutation, removal, and first-chat revision check acquires this lock
// BEFORE connection/binding locks. It must be called inside a transaction.
func LockOwner(ctx context.Context, tx *gorm.DB, organizationID, ownerID uuid.UUID) error {
	if tx == nil || organizationID == uuid.Nil || ownerID == uuid.Nil {
		return inputError()
	}
	if tx.Dialector.Name() != "postgres" {
		return storageError(fmt.Errorf("preference transaction locks require PostgreSQL"))
	}
	digest := sha256.Sum256([]byte("zgi/connector-chat-preferences/v1/" + organizationID.String() + "/" + ownerID.String()))
	lockKey := int64(binary.BigEndian.Uint64(digest[:8]))
	return storageError(tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", lockKey).Error)
}

func readRecord(ctx context.Context, db *gorm.DB, organizationID, ownerID uuid.UUID) (preferenceRecord, error) {
	var record preferenceRecord
	err := db.WithContext(ctx).Where("organization_id = ? AND owner_account_id = ?", organizationID, ownerID).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return preferenceRecord{OrganizationID: organizationID, OwnerAccountID: ownerID, Defaults: datatypes.JSON([]byte("{}"))}, nil
	}
	if err != nil {
		return preferenceRecord{}, storageError(err)
	}
	if record.Revision < 1 {
		return preferenceRecord{}, storageError(fmt.Errorf("invalid preference revision"))
	}
	return record, nil
}

// ValidateChatRevision is a narrow initialization port. It does not select
// connections or create any bindings, including for a revision-zero caller.
func ValidateChatRevision(ctx context.Context, tx *gorm.DB, organizationID, ownerID uuid.UUID, expectedRevision int64) error {
	if expectedRevision < 0 {
		return inputError()
	}
	if err := LockOwner(ctx, tx, organizationID, ownerID); err != nil {
		return err
	}
	record, err := readRecord(ctx, tx, organizationID, ownerID)
	if err != nil {
		return err
	}
	if record.Revision != expectedRevision {
		return conflictError()
	}
	return nil
}

// ClearRemovedConnection runs after LockOwner but in the same transaction as
// removal. A reference becomes explicit none, never a guessed replacement.
func ClearRemovedConnection(ctx context.Context, tx *gorm.DB, organizationID, ownerID, connectionID uuid.UUID) error {
	if tx == nil || organizationID == uuid.Nil || ownerID == uuid.Nil || connectionID == uuid.Nil {
		return inputError()
	}
	record, err := readRecord(ctx, tx, organizationID, ownerID)
	if err != nil || record.Revision == 0 {
		return err
	}
	defaults, err := decodeDefaults(record.Defaults)
	if err != nil {
		return err
	}
	changed := false
	for id, selection := range defaults {
		if selection.Mode == ModeSelected && selection.ConnectionID == connectionID.String() {
			defaults[id] = Selection{Mode: ModeNone}
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeRecord(ctx, tx, record, defaults)
}

func writeRecord(ctx context.Context, tx *gorm.DB, record preferenceRecord, defaults map[string]Selection) error {
	raw, err := json.Marshal(defaults)
	if err != nil {
		return storageError(err)
	}
	now := time.Now().UTC()
	if record.Revision == 0 {
		record.Defaults = datatypes.JSON(raw)
		record.Revision = 1
		record.CreatedAt = now
		record.UpdatedAt = now
		return storageError(tx.WithContext(ctx).Create(&record).Error)
	}
	result := tx.WithContext(ctx).Model(&preferenceRecord{}).
		Where("organization_id = ? AND owner_account_id = ? AND revision = ?", record.OrganizationID, record.OwnerAccountID, record.Revision).
		Updates(map[string]any{"defaults": datatypes.JSON(raw), "revision": record.Revision + 1, "updated_at": now})
	if result.Error != nil {
		return storageError(result.Error)
	}
	if result.RowsAffected != 1 {
		return conflictError()
	}
	return nil
}
