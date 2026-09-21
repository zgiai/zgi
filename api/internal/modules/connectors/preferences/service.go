package preferences

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DefinitionCatalog interface {
	Get(string) (connectorcatalog.Definition, bool)
}
type Service struct {
	db      *gorm.DB
	catalog DefinitionCatalog
}

func NewService(db *gorm.DB, catalog DefinitionCatalog) (*Service, error) {
	if db == nil || catalog == nil {
		return nil, inputError()
	}
	return &Service{db: db, catalog: catalog}, nil
}

// summaryRecord deliberately cannot select or decode a credential envelope.
type summaryRecord struct {
	ID                    uuid.UUID      `gorm:"column:id"`
	ConnectorID           string         `gorm:"column:connector_id"`
	Name                  string         `gorm:"column:name"`
	ExternalDisplayName   string         `gorm:"column:external_display_name"`
	State                 core.Readiness `gorm:"column:state"`
	DriverID              string         `gorm:"column:driver_id"`
	AuthMethodID          string         `gorm:"column:auth_method_id"`
	UsableOperationIDs    datatypes.JSON `gorm:"column:usable_operation_ids"`
	UnavailableOperations datatypes.JSON `gorm:"column:unavailable_operations"`
	ConnectionConstraint  datatypes.JSON `gorm:"column:connection_constraint"`
}

const summaryColumns = "id, connector_id, name, external_display_name, state, driver_id, auth_method_id, usable_operation_ids, unavailable_operations, connection_constraint"

func (service *Service) Get(ctx context.Context, organizationID, ownerID uuid.UUID) (Snapshot, error) {
	if service == nil || service.db == nil || organizationID == uuid.Nil || ownerID == uuid.Nil {
		return Snapshot{}, inputError()
	}
	record, err := readRecord(ctx, service.db, organizationID, ownerID)
	if err != nil {
		return Snapshot{}, err
	}
	return service.snapshot(ctx, service.db, record)
}

func (service *Service) Update(ctx context.Context, command UpdateCommand) (Snapshot, error) {
	if service == nil || service.db == nil {
		return Snapshot{}, inputError()
	}
	if err := validateCommand(command); err != nil {
		return Snapshot{}, err
	}
	var result Snapshot
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockOwner(ctx, tx, command.OrganizationID, command.OwnerAccountID); err != nil {
			return err
		}
		record, err := readRecord(ctx, tx, command.OrganizationID, command.OwnerAccountID)
		if err != nil {
			return err
		}
		keyHash := hash(command.OrganizationID.String() + "/" + command.OwnerAccountID.String() + "/" + command.IdempotencyKey)
		fingerprint := hash(command.ConnectorID + "/" + string(command.Selection.Mode) + "/" + command.Selection.ConnectionID + "/" + strconv.FormatInt(command.ExpectedRevision, 10))
		var receipt mutationRecord
		err = tx.Where("organization_id = ? AND owner_account_id = ? AND key_hash = ?", command.OrganizationID, command.OwnerAccountID, keyHash).Take(&receipt).Error
		if err == nil {
			if receipt.Fingerprint != fingerprint {
				return apperror.New(AppCodeIdempotencyConflict, apperror.WithOperation("connector.preference.replay"))
			}
			// Return current state, not a stale cached default that could resurrect a deleted Key.
			result, err = service.snapshot(ctx, tx, record)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return storageError(err)
		}
		if record.Revision != command.ExpectedRevision {
			return conflictError()
		}
		defaults, err := decodeDefaults(record.Defaults)
		if err != nil {
			return err
		}
		if _, exists := defaults[command.ConnectorID]; !exists && len(defaults) >= MaxDefaults {
			return inputError()
		}
		if command.Selection.Mode == ModeSelected {
			var connection summaryRecord
			err := tx.Table("connector_connections").Select(summaryColumns).Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("organization_id = ? AND owner_account_id = ? AND id = ? AND connector_id = ? AND deleted_at IS NULL", command.OrganizationID, command.OwnerAccountID, command.Selection.ConnectionID, command.ConnectorID).Take(&connection).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperror.New(AppCodeConnectionNotFound, apperror.WithOperation("connector.preference.select"))
			}
			if err != nil {
				return storageError(err)
			}
			if _, available := service.connectionSummary(connection, command.OwnerAccountID); !available {
				return apperror.New(AppCodeConnectionUnavailable, apperror.WithOperation("connector.preference.select"))
			}
		}
		defaults[command.ConnectorID] = command.Selection
		if err := writeRecord(ctx, tx, record, defaults); err != nil {
			return err
		}
		receipt = mutationRecord{OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID, KeyHash: keyHash, Fingerprint: fingerprint, ResultRevision: record.Revision + 1, CreatedAt: time.Now().UTC()}
		if err := tx.Create(&receipt).Error; err != nil {
			return storageError(err)
		}
		record.Revision++
		raw, _ := json.Marshal(defaults)
		record.Defaults = datatypes.JSON(raw)
		result, err = service.snapshot(ctx, tx, record)
		return err
	})
	if err != nil {
		if _, ok := apperror.As(err); ok {
			return Snapshot{}, err
		}
		return Snapshot{}, storageError(err)
	}
	return result, nil
}

func (service *Service) snapshot(ctx context.Context, db *gorm.DB, record preferenceRecord) (Snapshot, error) {
	defaults, err := decodeDefaults(record.Defaults)
	if err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{Revision: record.Revision, Defaults: make(map[string]Default, len(defaults))}
	ids := make([]string, 0, len(defaults))
	for id, selection := range defaults {
		entry := Default{Selection: selection}
		if selection.Mode == ModeSelected {
			available := false
			entry.Available = &available
			ids = append(ids, selection.ConnectionID)
		}
		result.Defaults[id] = entry
	}
	if len(ids) == 0 {
		return result, nil
	}
	sort.Strings(ids)
	var records []summaryRecord
	if err := db.WithContext(ctx).Table("connector_connections").Select(summaryColumns).
		Where("organization_id = ? AND owner_account_id = ? AND id IN ? AND deleted_at IS NULL", record.OrganizationID, record.OwnerAccountID, ids).Find(&records).Error; err != nil {
		return Snapshot{}, storageError(err)
	}
	for _, connection := range records {
		entry, exists := result.Defaults[connection.ConnectorID]
		if !exists || entry.Mode != ModeSelected || entry.ConnectionID != connection.ID.String() {
			continue
		}
		summary, available := service.connectionSummary(connection, record.OwnerAccountID)
		entry.Connection = &summary
		entry.Available = &available
		result.Defaults[connection.ConnectorID] = entry
	}
	return result, nil
}

func (service *Service) connectionSummary(record summaryRecord, ownerID uuid.UUID) (ConnectionSummary, bool) {
	operations := []string{}
	decodeErr := json.Unmarshal(record.UsableOperationIDs, &operations)
	if operations == nil {
		operations = []string{}
	}
	summary := ConnectionSummary{ID: record.ID, Name: record.Name, ExternalDisplayName: record.ExternalDisplayName, State: record.State, UsableOperationIDs: operations}
	definition, found := service.catalog.Get(record.ConnectorID)
	if decodeErr != nil || !found || definition.ID != record.ConnectorID || connectorcatalog.Validate(definition) != nil || record.State != core.ReadinessReady || !definition.Supports(connectorcatalog.SurfaceChat) || definition.DriverID != record.DriverID || definition.AuthMethodID != record.AuthMethodID {
		return summary, false
	}
	constraint, err := core.DecodeConstraint(record.ConnectionConstraint)
	var unavailable map[core.Identifier]string
	if err != nil || json.Unmarshal(record.UnavailableOperations, &unavailable) != nil {
		return summary, false
	}
	// A new conversation has no UUID yet. Only a wildcard Chat boundary can
	// serve as its default; target-specific authorization remains on bindings.
	futureChat := core.TargetRef{Type: core.TargetTypeChat, ID: "*", Scope: core.ScopeLive}
	for _, admitted := range definition.Operations {
		if _, denied := unavailable[core.Identifier(admitted.ID)]; denied || !constraint.Allows(ownerID, core.Identifier(admitted.ID), futureChat, time.Now()) {
			continue
		}
		for _, usable := range operations {
			if usable == admitted.ID {
				return summary, true
			}
		}
	}
	return summary, false
}

func hash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
