package connections

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConnectionListFilter contains only owner-visible Connector-native fields.
// Target filtering belongs to the bindings repository and service layer.
type ConnectionListFilter struct {
	ConnectorID core.Identifier
	States      []core.Readiness
}

// SetupAttemptListFilter is intended for owner-visible setup status and retry
// flows. It deliberately has no credential or request-payload filters.
type SetupAttemptListFilter struct {
	ConnectorID core.Identifier
	States      []core.SetupState
}

type ConnectionRepository interface {
	CreateConnection(context.Context, core.PersonalConnection) error
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.PersonalConnection, error)
	ListConnections(context.Context, uuid.UUID, uuid.UUID, ConnectionListFilter) ([]core.PersonalConnection, error)
	UpdateConnection(context.Context, uuid.UUID, uuid.UUID, core.PersonalConnection, int) (core.PersonalConnection, error)
}

type SetupAttemptRepository interface {
	ClaimSetupAttempt(context.Context, core.KeySetupAttempt) (core.KeySetupAttempt, bool, error)
	GetSetupAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.KeySetupAttempt, error)
	ListSetupAttempts(context.Context, uuid.UUID, uuid.UUID, SetupAttemptListFilter) ([]core.KeySetupAttempt, error)
	CompleteSetupReady(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, core.PersonalConnection) (core.KeySetupAttempt, error)
	CompleteSetupFailed(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, core.ErrorCode) (core.KeySetupAttempt, error)
	CancelSetupAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.KeySetupAttempt, error)
}

// Repository is the persistence contract used by the Personal Key lifecycle.
// Implementations must never resolve records by a globally supplied UUID alone.
type Repository interface {
	ConnectionRepository
	SetupAttemptRepository
}

// BindingRevisionSnapshot freezes the active-reference set used by an
// impact-token guarded removal.
type BindingRevisionSnapshot struct {
	ID       uuid.UUID
	Revision int
}

// ReconnectCompletion describes an already-verified reconnect result. Same
// identity replaces ExistingConnectionID; changed identity inserts Result as a
// new, unbound Personal Connection.
type ReconnectCompletion struct {
	ExistingConnectionID uuid.UUID
	ExpectedRevision     int
	ReplaceExisting      bool
	Result               core.PersonalConnection
}

type GormRepository struct {
	db *gorm.DB
}

var _ Repository = (*GormRepository)(nil)

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (repository *GormRepository) CreateConnection(ctx context.Context, connection core.PersonalConnection) error {
	if err := repository.available(); err != nil {
		return err
	}
	record, err := connectionRecordFromDomain(connection)
	if err != nil {
		return err
	}
	if err := repository.db.WithContext(ctx).Create(&record).Error; err != nil {
		if isDuplicateDatabaseError(err) {
			return conflictError("create connector connection")
		}
		return internalError("create connector connection")
	}
	return nil
}

func (repository *GormRepository) GetConnection(
	ctx context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
) (core.PersonalConnection, error) {
	if err := repository.available(); err != nil {
		return core.PersonalConnection{}, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || connectionID == uuid.Nil {
		return core.PersonalConnection{}, invalidInputError("get connector connection")
	}
	record, err := getConnectionRecord(ctx, repository.db, organizationID, ownerAccountID, connectionID)
	if err != nil {
		return core.PersonalConnection{}, err
	}
	connection, err := record.toDomain()
	if err != nil {
		return core.PersonalConnection{}, err
	}
	return loadProviderEvidence(ctx, repository.db, connection)
}

func (repository *GormRepository) ListConnections(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	filter ConnectionListFilter,
) ([]core.PersonalConnection, error) {
	if err := repository.available(); err != nil {
		return nil, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || !validConnectionListFilter(filter) {
		return nil, invalidInputError("list connector connections")
	}
	query := repository.db.WithContext(ctx).
		Where("organization_id = ? AND owner_account_id = ? AND deleted_at IS NULL", organizationID, ownerAccountID)
	if filter.ConnectorID != "" {
		query = query.Where("connector_id = ?", filter.ConnectorID.String())
	}
	if len(filter.States) > 0 {
		query = query.Where("state IN ?", filter.States)
	}
	var records []connectionRecord
	if err := query.Order("connector_id ASC, name ASC, created_at ASC, id ASC").Find(&records).Error; err != nil {
		return nil, internalError("list connector connections")
	}
	connections := make([]core.PersonalConnection, 0, len(records))
	for _, record := range records {
		connection, err := record.toDomain()
		if err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	for i := range connections {
		loaded, err := loadProviderEvidence(ctx, repository.db, connections[i])
		if err != nil {
			return nil, err
		}
		connections[i] = loaded
	}
	return connections, nil
}

func (repository *GormRepository) UpdateConnection(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	connection core.PersonalConnection,
	expectedRevision int,
) (core.PersonalConnection, error) {
	if err := repository.available(); err != nil {
		return core.PersonalConnection{}, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil ||
		connection.OrganizationID != organizationID || connection.OwnerAccountID != ownerAccountID ||
		expectedRevision < 1 || connection.Revision != expectedRevision {
		return core.PersonalConnection{}, invalidInputError("update connector connection")
	}
	record, err := connectionRecordFromDomain(connection)
	if err != nil {
		return core.PersonalConnection{}, err
	}
	nextRevision := expectedRevision + 1
	updatedAt := time.Now().UTC()
	updates := map[string]any{
		"connection_constraint":    record.Constraint,
		"name":                     record.Name,
		"external_account_id":      record.ExternalAccountID,
		"external_display_name":    record.ExternalDisplayName,
		"encrypted_credentials":    record.EncryptedCredentials,
		"credential_version":       record.CredentialVersion,
		"state":                    record.State,
		"usable_operation_ids":     record.UsableOperationIDs,
		"unavailable_operations":   record.UnavailableOperations,
		"diagnostic_code":          record.DiagnosticCode,
		"revision":                 nextRevision,
		"last_verified_at":         record.LastVerifiedAt,
		"last_provider_success_at": record.LastProviderSuccessAt,
		"last_provider_failure_at": record.LastProviderFailureAt,
		"updated_at":               updatedAt,
	}
	result := repository.db.WithContext(ctx).Model(&connectionRecord{}).
		Where(
			"organization_id = ? AND owner_account_id = ? AND id = ? AND connector_id = ? AND provider_id = ? AND driver_id = ? AND auth_method_id = ? AND revision = ? AND deleted_at IS NULL",
			organizationID,
			ownerAccountID,
			connection.ID,
			connection.ConnectorID.String(),
			connection.ProviderID.String(),
			connection.DriverID.String(),
			connection.AuthMethodID.String(),
			expectedRevision,
		).
		Updates(updates)
	if result.Error != nil {
		return core.PersonalConnection{}, internalError("update connector connection")
	}
	if result.RowsAffected != 1 {
		exists, err := connectionExistsInScope(ctx, repository.db, organizationID, ownerAccountID, connection.ID)
		if err != nil {
			return core.PersonalConnection{}, err
		}
		if !exists {
			return core.PersonalConnection{}, notFoundError("update connector connection")
		}
		return core.PersonalConnection{}, conflictError("update connector connection")
	}
	connection.Revision = nextRevision
	connection.UpdatedAt = updatedAt
	return connection, nil
}

// CompleteReconnectReady atomically commits either a same-identity credential
// swap or a different-identity unbound Connection together with the terminal
// setup-attempt state. Verification and sealing must finish before this call.
func (repository *GormRepository) CompleteReconnectReady(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
	completion ReconnectCompletion,
) (core.KeySetupAttempt, error) {
	if err := repository.available(); err != nil {
		return core.KeySetupAttempt{}, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || attemptID == uuid.Nil ||
		completion.ExistingConnectionID == uuid.Nil || completion.ExpectedRevision < 1 ||
		completion.Result.OrganizationID != organizationID || completion.Result.OwnerAccountID != ownerAccountID ||
		completion.Result.State != core.ReadinessReady || completion.Result.DeletedAt != nil ||
		completion.ReplaceExisting != (completion.Result.ID == completion.ExistingConnectionID) {
		return core.KeySetupAttempt{}, invalidInputError("complete connector reconnect")
	}
	resultRecord, err := connectionRecordFromDomain(completion.Result)
	if err != nil {
		return core.KeySetupAttempt{}, err
	}

	completed := core.KeySetupAttempt{}
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attemptRecord, lookupErr := getSetupAttemptRecord(ctx, tx, organizationID, ownerAccountID, attemptID, true)
		if lookupErr != nil {
			return lookupErr
		}
		if attemptRecord.State != string(core.SetupStateVerifying) || attemptRecord.ConnectionID == nil ||
			*attemptRecord.ConnectionID != completion.ExistingConnectionID ||
			attemptRecord.ConnectorID != completion.Result.ConnectorID.String() {
			return conflictError("complete connector reconnect")
		}

		if completion.ReplaceExisting {
			existing, lookupErr := getConnectionRecord(
				ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}),
				organizationID, ownerAccountID, completion.ExistingConnectionID,
			)
			if lookupErr != nil {
				return lookupErr
			}
			if existing.Revision != completion.ExpectedRevision ||
				existing.ConnectorID != resultRecord.ConnectorID || existing.ProviderID != resultRecord.ProviderID ||
				existing.DriverID != resultRecord.DriverID || existing.AuthMethodID != resultRecord.AuthMethodID {
				return conflictError("complete connector reconnect")
			}
			now := time.Now().UTC()
			update := tx.Model(&connectionRecord{}).
				Where(
					"organization_id = ? AND owner_account_id = ? AND id = ? AND revision = ? AND deleted_at IS NULL",
					organizationID, ownerAccountID, completion.ExistingConnectionID, completion.ExpectedRevision,
				).
				Updates(map[string]any{
					"connection_constraint":    resultRecord.Constraint,
					"external_account_id":      resultRecord.ExternalAccountID,
					"external_display_name":    resultRecord.ExternalDisplayName,
					"encrypted_credentials":    resultRecord.EncryptedCredentials,
					"credential_version":       resultRecord.CredentialVersion,
					"state":                    resultRecord.State,
					"usable_operation_ids":     resultRecord.UsableOperationIDs,
					"unavailable_operations":   resultRecord.UnavailableOperations,
					"diagnostic_code":          nil,
					"revision":                 completion.ExpectedRevision + 1,
					"last_verified_at":         resultRecord.LastVerifiedAt,
					"last_provider_success_at": resultRecord.LastProviderSuccessAt,
					"last_provider_failure_at": nil,
					"updated_at":               now,
				})
			if update.Error != nil {
				return internalError("complete connector reconnect")
			}
			if update.RowsAffected != 1 {
				return conflictError("complete connector reconnect")
			}
		} else {
			if resultRecord.ID == completion.ExistingConnectionID {
				return conflictError("complete connector reconnect")
			}
			if err := tx.Create(&resultRecord).Error; err != nil {
				if isDuplicateDatabaseError(err) {
					return conflictError("complete connector reconnect")
				}
				return internalError("complete connector reconnect")
			}
		}

		now := time.Now().UTC()
		if err := writeProviderEvidence(ctx, tx, completion.Result); err != nil {
			return err
		}
		resultConnectionID := completion.Result.ID
		updateAttempt := tx.Model(&setupAttemptRecord{}).
			Where(
				"organization_id = ? AND owner_account_id = ? AND id = ? AND state = ?",
				organizationID, ownerAccountID, attemptID, core.SetupStateVerifying,
			).
			Updates(map[string]any{
				"state": core.SetupStateReady, "error_code": nil,
				"result_connection_id": resultConnectionID, "updated_at": now,
			})
		if updateAttempt.Error != nil {
			return internalError("complete connector reconnect")
		}
		if updateAttempt.RowsAffected != 1 {
			return conflictError("complete connector reconnect")
		}
		attemptRecord.State = string(core.SetupStateReady)
		attemptRecord.ErrorCode = nil
		attemptRecord.ResultConnectionID = &resultConnectionID
		attemptRecord.UpdatedAt = now
		completed, lookupErr = attemptRecord.toDomain()
		return lookupErr
	})
	if err != nil {
		return core.KeySetupAttempt{}, err
	}
	return completed, nil
}

// ForceRemoveConnection rechecks the complete active-binding snapshot and then
// marks every reference as needing replacement while destroying the stored
// credential authorization in one transaction. An empty expected snapshot is
// the normal remove path and succeeds only when there are no active references.
func (repository *GormRepository) ForceRemoveConnection(
	ctx context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	expectedRevision int,
	expectedBindings []BindingRevisionSnapshot,
) error {
	if err := repository.available(); err != nil {
		return err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || connectionID == uuid.Nil || expectedRevision < 1 {
		return invalidInputError("remove connector connection")
	}
	expected := append([]BindingRevisionSnapshot(nil), expectedBindings...)
	sortBindingSnapshots(expected)
	for index, item := range expected {
		if item.ID == uuid.Nil || item.Revision < 1 || (index > 0 && expected[index-1].ID == item.ID) {
			return invalidInputError("remove connector connection")
		}
	}

	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := preferences.LockOwner(ctx, tx, organizationID, ownerAccountID); err != nil {
			return err
		}
		connection, err := getConnectionRecord(
			ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}),
			organizationID, ownerAccountID, connectionID,
		)
		if err != nil {
			return err
		}
		if connection.Revision != expectedRevision {
			return conflictError("remove connector connection")
		}

		type activeBindingRecord struct {
			ID       uuid.UUID `gorm:"column:id"`
			Revision int       `gorm:"column:revision"`
		}
		var active []activeBindingRecord
		if err := tx.Table("connector_target_bindings").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "revision").
			Where(
				"organization_id = ? AND owner_account_id = ? AND connection_id = ? AND status = ?",
				organizationID, ownerAccountID, connectionID, core.BindingStatusActive,
			).
			Order("id ASC").
			Find(&active).Error; err != nil {
			return internalError("inspect connector removal bindings")
		}
		actual := make([]BindingRevisionSnapshot, 0, len(active))
		for _, item := range active {
			actual = append(actual, BindingRevisionSnapshot{ID: item.ID, Revision: item.Revision})
		}
		if !equalBindingSnapshots(expected, actual) {
			return conflictError("remove connector connection")
		}

		now := time.Now().UTC()
		if len(active) > 0 {
			ids := make([]uuid.UUID, 0, len(active))
			for _, item := range active {
				ids = append(ids, item.ID)
			}
			updated := tx.Table("connector_target_bindings").
				Where(
					"organization_id = ? AND owner_account_id = ? AND connection_id = ? AND id IN ? AND status = ?",
					organizationID, ownerAccountID, connectionID, ids, core.BindingStatusActive,
				).
				Updates(map[string]any{
					"status":      core.BindingStatusNeedsReplacement,
					"disabled_at": now,
					"revision":    gorm.Expr("revision + 1"),
					"updated_at":  now,
				})
			if updated.Error != nil {
				return internalError("disable connector removal bindings")
			}
			if updated.RowsAffected != int64(len(active)) {
				return conflictError("remove connector connection")
			}
		}

		removed := tx.Model(&connectionRecord{}).
			Where(
				"organization_id = ? AND owner_account_id = ? AND id = ? AND revision = ? AND deleted_at IS NULL",
				organizationID, ownerAccountID, connectionID, expectedRevision,
			).
			Updates(map[string]any{
				"encrypted_credentials": "removed",
				"state":                 core.ReadinessUnavailable,
				"usable_operation_ids":  datatypes.JSON([]byte("[]")),
				"diagnostic_code":       "connection_removed",
				"revision":              expectedRevision + 1,
				"deleted_at":            now,
				"updated_at":            now,
			})
		if removed.Error != nil {
			return internalError("remove connector connection")
		}
		if removed.RowsAffected != 1 {
			return conflictError("remove connector connection")
		}
		return preferences.ClearRemovedConnection(ctx, tx, organizationID, ownerAccountID, connectionID)
	})
}

func sortBindingSnapshots(items []BindingRevisionSnapshot) {
	sort.Slice(items, func(i, j int) bool { return items[i].ID.String() < items[j].ID.String() })
}

func equalBindingSnapshots(expected, actual []BindingRevisionSnapshot) bool {
	if len(expected) != len(actual) {
		return false
	}
	for index := range expected {
		if expected[index] != actual[index] {
			return false
		}
	}
	return true
}

// ClaimSetupAttempt atomically creates one verifying attempt or returns the
// prior owner-scoped attempt for the same idempotency hash. Reusing a hash for
// a different server-keyed request fingerprint is a conflict, never a retry.
func (repository *GormRepository) ClaimSetupAttempt(
	ctx context.Context,
	attempt core.KeySetupAttempt,
) (core.KeySetupAttempt, bool, error) {
	if err := repository.available(); err != nil {
		return core.KeySetupAttempt{}, false, err
	}
	if err := attempt.Validate(); err != nil || attempt.State != core.SetupStateVerifying ||
		attempt.ErrorCode != "" || attempt.ResultConnectionID != uuid.Nil {
		return core.KeySetupAttempt{}, false, invalidInputError("claim connector key setup")
	}
	record := setupAttemptRecordFromDomain(attempt)
	claimed := core.KeySetupAttempt{}
	created := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "organization_id"},
				{Name: "owner_account_id"},
				{Name: "idempotency_key_hash"},
			},
			DoNothing: true,
		}).Create(&record)
		if result.Error != nil {
			if isDuplicateDatabaseError(result.Error) {
				return conflictError("claim connector key setup")
			}
			return internalError("claim connector key setup")
		}
		if result.RowsAffected == 1 {
			var err error
			claimed, err = record.toDomain()
			if err != nil {
				return err
			}
			created = true
			return nil
		}

		var existing setupAttemptRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"organization_id = ? AND owner_account_id = ? AND idempotency_key_hash = ?",
				attempt.OrganizationID,
				attempt.OwnerAccountID,
				attempt.IdempotencyKeyHash,
			).
			Take(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFoundError("claim connector key setup")
			}
			return internalError("claim connector key setup")
		}
		if existing.RequestFingerprint != attempt.RequestFingerprint {
			return conflictError("claim connector key setup")
		}
		var err error
		claimed, err = existing.toDomain()
		return err
	})
	if err != nil {
		return core.KeySetupAttempt{}, false, err
	}
	return claimed, created, nil
}

func (repository *GormRepository) GetSetupAttempt(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
) (core.KeySetupAttempt, error) {
	if err := repository.available(); err != nil {
		return core.KeySetupAttempt{}, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || attemptID == uuid.Nil {
		return core.KeySetupAttempt{}, invalidInputError("get connector key setup")
	}
	record, err := getSetupAttemptRecord(ctx, repository.db, organizationID, ownerAccountID, attemptID, false)
	if err != nil {
		return core.KeySetupAttempt{}, err
	}
	return record.toDomain()
}

func (repository *GormRepository) ListSetupAttempts(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	filter SetupAttemptListFilter,
) ([]core.KeySetupAttempt, error) {
	if err := repository.available(); err != nil {
		return nil, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || !validSetupAttemptListFilter(filter) {
		return nil, invalidInputError("list connector key setups")
	}
	query := repository.db.WithContext(ctx).
		Where("organization_id = ? AND owner_account_id = ?", organizationID, ownerAccountID)
	if filter.ConnectorID != "" {
		query = query.Where("connector_id = ?", filter.ConnectorID.String())
	}
	if len(filter.States) > 0 {
		query = query.Where("state IN ?", filter.States)
	}
	var records []setupAttemptRecord
	if err := query.Order("created_at DESC, id DESC").Find(&records).Error; err != nil {
		return nil, internalError("list connector key setups")
	}
	attempts := make([]core.KeySetupAttempt, 0, len(records))
	for _, record := range records {
		attempt, err := record.toDomain()
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, nil
}

// CompleteSetupReady commits the encrypted ready Connection and the terminal
// attempt state in one database transaction. Provider verification and
// encryption must already have completed before this method is called.
func (repository *GormRepository) CompleteSetupReady(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
	connection core.PersonalConnection,
) (core.KeySetupAttempt, error) {
	if err := repository.available(); err != nil {
		return core.KeySetupAttempt{}, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || attemptID == uuid.Nil ||
		connection.OrganizationID != organizationID || connection.OwnerAccountID != ownerAccountID ||
		connection.State != core.ReadinessReady || connection.DeletedAt != nil || len(connection.UsableOperationIDs) == 0 {
		return core.KeySetupAttempt{}, invalidInputError("complete connector key setup")
	}
	connectionRecord, err := connectionRecordFromDomain(connection)
	if err != nil {
		return core.KeySetupAttempt{}, err
	}
	completed := core.KeySetupAttempt{}
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attemptRecord, err := getSetupAttemptRecord(ctx, tx, organizationID, ownerAccountID, attemptID, true)
		if err != nil {
			return err
		}
		if attemptRecord.State != string(core.SetupStateVerifying) {
			return conflictError("complete connector key setup")
		}
		if attemptRecord.ConnectorID != connection.ConnectorID.String() {
			return conflictError("complete connector key setup")
		}
		if err := tx.Create(&connectionRecord).Error; err != nil {
			if isDuplicateDatabaseError(err) {
				return conflictError("complete connector key setup")
			}
			return internalError("complete connector key setup")
		}
		now := time.Now().UTC()
		if err := writeProviderEvidence(ctx, tx, connection); err != nil {
			return err
		}
		resultConnectionID := connection.ID
		result := tx.Model(&setupAttemptRecord{}).
			Where(
				"organization_id = ? AND owner_account_id = ? AND id = ? AND state = ?",
				organizationID,
				ownerAccountID,
				attemptID,
				core.SetupStateVerifying,
			).
			Updates(map[string]any{
				"state":                core.SetupStateReady,
				"error_code":           nil,
				"result_connection_id": resultConnectionID,
				"updated_at":           now,
			})
		if result.Error != nil {
			return internalError("complete connector key setup")
		}
		if result.RowsAffected != 1 {
			return conflictError("complete connector key setup")
		}
		attemptRecord.State = string(core.SetupStateReady)
		attemptRecord.ErrorCode = nil
		attemptRecord.ResultConnectionID = &resultConnectionID
		attemptRecord.UpdatedAt = now
		completed, err = attemptRecord.toDomain()
		if err != nil {
			return err
		}
		return recordKeySetupAudit(ctx, tx, completed, connection.ID, now)
	})
	if err != nil {
		return core.KeySetupAttempt{}, err
	}
	return completed, nil
}

func (repository *GormRepository) CompleteSetupFailed(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
	errorCode core.ErrorCode,
) (core.KeySetupAttempt, error) {
	if !errorCode.Valid() {
		return core.KeySetupAttempt{}, invalidInputError("fail connector key setup")
	}
	return repository.transitionSetupAttempt(
		ctx,
		organizationID,
		ownerAccountID,
		attemptID,
		core.SetupStateFailed,
		errorCode,
	)
}

func (repository *GormRepository) CancelSetupAttempt(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
) (core.KeySetupAttempt, error) {
	return repository.transitionSetupAttempt(
		ctx,
		organizationID,
		ownerAccountID,
		attemptID,
		core.SetupStateCancelled,
		"",
	)
}

func (repository *GormRepository) transitionSetupAttempt(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
	state core.SetupState,
	errorCode core.ErrorCode,
) (core.KeySetupAttempt, error) {
	if err := repository.available(); err != nil {
		return core.KeySetupAttempt{}, err
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || attemptID == uuid.Nil ||
		(state != core.SetupStateFailed && state != core.SetupStateCancelled) ||
		(state == core.SetupStateFailed && !errorCode.Valid()) ||
		(state == core.SetupStateCancelled && errorCode != "") {
		return core.KeySetupAttempt{}, invalidInputError("transition connector key setup")
	}
	completed := core.KeySetupAttempt{}
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := getSetupAttemptRecord(ctx, tx, organizationID, ownerAccountID, attemptID, true)
		if err != nil {
			return err
		}
		if record.State != string(core.SetupStateVerifying) {
			return conflictError("transition connector key setup")
		}
		now := time.Now().UTC()
		updates := map[string]any{
			"state":                state,
			"result_connection_id": nil,
			"updated_at":           now,
		}
		if state == core.SetupStateFailed {
			code := string(errorCode)
			updates["error_code"] = code
			record.ErrorCode = &code
		} else {
			updates["error_code"] = nil
			record.ErrorCode = nil
		}
		result := tx.Model(&setupAttemptRecord{}).
			Where(
				"organization_id = ? AND owner_account_id = ? AND id = ? AND state = ?",
				organizationID,
				ownerAccountID,
				attemptID,
				core.SetupStateVerifying,
			).
			Updates(updates)
		if result.Error != nil {
			return internalError("transition connector key setup")
		}
		if result.RowsAffected != 1 {
			return conflictError("transition connector key setup")
		}
		record.State = string(state)
		record.ResultConnectionID = nil
		record.UpdatedAt = now
		completed, err = record.toDomain()
		if err != nil {
			return err
		}
		return recordKeySetupAudit(ctx, tx, completed, uuid.Nil, now)
	})
	if err != nil {
		return core.KeySetupAttempt{}, err
	}
	return completed, nil
}

func (repository *GormRepository) available() error {
	if repository == nil || repository.db == nil {
		return internalError("connector connection repository unavailable")
	}
	return nil
}

func getConnectionRecord(
	ctx context.Context,
	db *gorm.DB,
	organizationID, ownerAccountID, connectionID uuid.UUID,
) (connectionRecord, error) {
	var record connectionRecord
	err := db.WithContext(ctx).
		Where(
			"organization_id = ? AND owner_account_id = ? AND id = ? AND deleted_at IS NULL",
			organizationID,
			ownerAccountID,
			connectionID,
		).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return connectionRecord{}, notFoundError("get connector connection")
	}
	if err != nil {
		return connectionRecord{}, internalError("get connector connection")
	}
	return record, nil
}

func getSetupAttemptRecord(
	ctx context.Context,
	db *gorm.DB,
	organizationID, ownerAccountID, attemptID uuid.UUID,
	lock bool,
) (setupAttemptRecord, error) {
	query := db.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record setupAttemptRecord
	err := query.Where(
		"organization_id = ? AND owner_account_id = ? AND id = ?",
		organizationID,
		ownerAccountID,
		attemptID,
	).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return setupAttemptRecord{}, notFoundError("get connector key setup")
	}
	if err != nil {
		return setupAttemptRecord{}, internalError("get connector key setup")
	}
	return record, nil
}

func connectionExistsInScope(
	ctx context.Context,
	db *gorm.DB,
	organizationID, ownerAccountID, connectionID uuid.UUID,
) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&connectionRecord{}).
		Where(
			"organization_id = ? AND owner_account_id = ? AND id = ? AND deleted_at IS NULL",
			organizationID,
			ownerAccountID,
			connectionID,
		).
		Count(&count).Error; err != nil {
		return false, internalError("check connector connection")
	}
	return count == 1, nil
}

func validConnectionListFilter(filter ConnectionListFilter) bool {
	if filter.ConnectorID != "" && !filter.ConnectorID.Valid() {
		return false
	}
	for _, state := range filter.States {
		if !state.Valid() {
			return false
		}
	}
	return true
}

func validSetupAttemptListFilter(filter SetupAttemptListFilter) bool {
	if filter.ConnectorID != "" && !filter.ConnectorID.Valid() {
		return false
	}
	for _, state := range filter.States {
		if !state.Valid() {
			return false
		}
	}
	return true
}

func invalidInputError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeInvalidInput, false, nil))
}

func notFoundError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeNotFound, false, nil))
}

func conflictError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeConflict, false, nil))
}

func internalError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeInternal, true, nil))
}

func isDuplicateDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "violates unique constraint") ||
		strings.Contains(message, "unique constraint failed")
}

type connectionRecord struct {
	Constraint            datatypes.JSON `gorm:"column:connection_constraint;type:jsonb" json:"-"`
	ID                    uuid.UUID      `gorm:"type:uuid;primaryKey"`
	OrganizationID        uuid.UUID      `gorm:"type:uuid;not null;index:idx_connector_connections_owner_catalog,priority:1"`
	OwnerAccountID        uuid.UUID      `gorm:"type:uuid;not null;index:idx_connector_connections_owner_catalog,priority:2"`
	ConnectorID           string         `gorm:"size:128;not null;index:idx_connector_connections_owner_catalog,priority:3"`
	ProviderID            string         `gorm:"size:128;not null"`
	DriverID              string         `gorm:"size:128;not null"`
	AuthMethodID          string         `gorm:"size:128;not null"`
	Name                  string         `gorm:"size:255;not null"`
	ExternalAccountID     *string        `gorm:"size:255"`
	ExternalDisplayName   *string        `gorm:"size:255"`
	EncryptedCredentials  string         `gorm:"type:text;not null" json:"-"`
	CredentialVersion     int            `gorm:"not null"`
	State                 string         `gorm:"size:32;not null;index:idx_connector_connections_owner_catalog,priority:4"`
	UsableOperationIDs    datatypes.JSON `gorm:"type:jsonb;not null"`
	UnavailableOperations datatypes.JSON `gorm:"type:jsonb;not null"`
	DiagnosticCode        *string        `gorm:"size:64"`
	Revision              int            `gorm:"not null"`
	LastVerifiedAt        time.Time      `gorm:"not null"`
	LastProviderSuccessAt *time.Time
	LastProviderFailureAt *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time `gorm:"index"`
}

func (connectionRecord) TableName() string { return "connector_connections" }

func connectionRecordFromDomain(connection core.PersonalConnection) (connectionRecord, error) {
	constraint, err := json.Marshal(connection.Constraint)
	if err != nil {
		return connectionRecord{}, invalidInputError("encode native constraint")
	}
	if err := connection.Validate(); err != nil ||
		(connection.State == core.ReadinessReady && len(connection.UsableOperationIDs) == 0) ||
		len(connection.EncryptedCredentials) <= 3 {
		return connectionRecord{}, invalidInputError("persist connector connection")
	}
	usableOperationIDs := make([]string, 0, len(connection.UsableOperationIDs))
	for _, operationID := range connection.UsableOperationIDs {
		usableOperationIDs = append(usableOperationIDs, operationID.String())
	}
	usableJSON, err := json.Marshal(usableOperationIDs)
	if err != nil {
		return connectionRecord{}, invalidInputError("encode connector usable operations")
	}
	unavailableOperations := make(map[string]string, len(connection.UnavailableOperations))
	for operationID, code := range connection.UnavailableOperations {
		unavailableOperations[operationID.String()] = code
	}
	unavailableJSON, err := json.Marshal(unavailableOperations)
	if err != nil {
		return connectionRecord{}, invalidInputError("encode connector unavailable operations")
	}
	return connectionRecord{
		Constraint:            datatypes.JSON(constraint),
		ID:                    connection.ID,
		OrganizationID:        connection.OrganizationID,
		OwnerAccountID:        connection.OwnerAccountID,
		ConnectorID:           connection.ConnectorID.String(),
		ProviderID:            connection.ProviderID.String(),
		DriverID:              connection.DriverID.String(),
		AuthMethodID:          connection.AuthMethodID.String(),
		Name:                  connection.Name,
		ExternalAccountID:     cloneString(connection.ExternalAccountID),
		ExternalDisplayName:   cloneString(connection.ExternalDisplayName),
		EncryptedCredentials:  connection.EncryptedCredentials,
		CredentialVersion:     connection.CredentialVersion,
		State:                 string(connection.State),
		UsableOperationIDs:    datatypes.JSON(usableJSON),
		UnavailableOperations: datatypes.JSON(unavailableJSON),
		DiagnosticCode:        cloneString(connection.DiagnosticCode),
		Revision:              connection.Revision,
		LastVerifiedAt:        connection.LastVerifiedAt,
		LastProviderSuccessAt: cloneTime(connection.LastProviderSuccessAt),
		LastProviderFailureAt: cloneTime(connection.LastProviderFailureAt),
		CreatedAt:             connection.CreatedAt,
		UpdatedAt:             connection.UpdatedAt,
		DeletedAt:             cloneTime(connection.DeletedAt),
	}, nil
}

func (record connectionRecord) toDomain() (core.PersonalConnection, error) {
	var constraint *core.AccessConstraint
	if len(record.Constraint) > 0 && string(record.Constraint) != "null" {
		var err error
		constraint, err = core.DecodeConstraint(record.Constraint)
		if err != nil {
			return core.PersonalConnection{}, internalError("decode native constraint")
		}
	}
	var usableStrings []string
	if err := json.Unmarshal(record.UsableOperationIDs, &usableStrings); err != nil {
		return core.PersonalConnection{}, internalError("decode connector usable operations")
	}
	usableOperationIDs := make([]core.Identifier, 0, len(usableStrings))
	for _, value := range usableStrings {
		identifier, err := core.NormalizeIdentifier(value)
		if err != nil {
			return core.PersonalConnection{}, internalError("decode connector usable operations")
		}
		usableOperationIDs = append(usableOperationIDs, identifier)
	}
	var unavailableStrings map[string]string
	if err := json.Unmarshal(record.UnavailableOperations, &unavailableStrings); err != nil {
		return core.PersonalConnection{}, internalError("decode connector unavailable operations")
	}
	if unavailableStrings == nil {
		unavailableStrings = map[string]string{}
	}
	unavailableOperations := make(map[core.Identifier]string, len(unavailableStrings))
	for value, code := range unavailableStrings {
		identifier, err := core.NormalizeIdentifier(value)
		if err != nil {
			return core.PersonalConnection{}, internalError("decode connector unavailable operations")
		}
		unavailableOperations[identifier] = code
	}
	connectorID, err := core.NormalizeIdentifier(record.ConnectorID)
	if err != nil {
		return core.PersonalConnection{}, internalError("decode connector connection identity")
	}
	providerID, err := core.NormalizeIdentifier(record.ProviderID)
	if err != nil {
		return core.PersonalConnection{}, internalError("decode connector provider identity")
	}
	driverID, err := core.NormalizeIdentifier(record.DriverID)
	if err != nil {
		return core.PersonalConnection{}, internalError("decode connector driver identity")
	}
	authMethodID, err := core.NormalizeIdentifier(record.AuthMethodID)
	if err != nil {
		return core.PersonalConnection{}, internalError("decode connector auth method identity")
	}
	connection := core.PersonalConnection{
		Constraint:            constraint,
		ID:                    record.ID,
		OrganizationID:        record.OrganizationID,
		OwnerAccountID:        record.OwnerAccountID,
		ConnectorID:           connectorID,
		ProviderID:            providerID,
		DriverID:              driverID,
		AuthMethodID:          authMethodID,
		Name:                  record.Name,
		ExternalAccountID:     cloneString(record.ExternalAccountID),
		ExternalDisplayName:   cloneString(record.ExternalDisplayName),
		EncryptedCredentials:  record.EncryptedCredentials,
		CredentialVersion:     record.CredentialVersion,
		State:                 core.Readiness(record.State),
		UsableOperationIDs:    usableOperationIDs,
		UnavailableOperations: unavailableOperations,
		DiagnosticCode:        cloneString(record.DiagnosticCode),
		Revision:              record.Revision,
		LastVerifiedAt:        record.LastVerifiedAt,
		LastProviderSuccessAt: cloneTime(record.LastProviderSuccessAt),
		LastProviderFailureAt: cloneTime(record.LastProviderFailureAt),
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
		DeletedAt:             cloneTime(record.DeletedAt),
	}
	if err := connection.Validate(); err != nil ||
		(connection.State == core.ReadinessReady && len(connection.UsableOperationIDs) == 0) ||
		len(connection.EncryptedCredentials) <= 3 {
		return core.PersonalConnection{}, internalError("decode connector connection")
	}
	return connection, nil
}

type setupAttemptRecord struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	OrganizationID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_connector_key_setups_owner_idempotency,priority:1;index:idx_connector_key_setups_owner_state,priority:1"`
	OwnerAccountID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_connector_key_setups_owner_idempotency,priority:2;index:idx_connector_key_setups_owner_state,priority:2"`
	ConnectorID        string     `gorm:"size:128;not null"`
	ConnectionID       *uuid.UUID `gorm:"type:uuid"`
	TargetType         *string    `gorm:"size:16"`
	TargetID           *string    `gorm:"size:255"`
	BindingInstanceID  *string    `gorm:"size:255"`
	IdempotencyKeyHash string     `gorm:"size:128;not null;uniqueIndex:idx_connector_key_setups_owner_idempotency,priority:3" json:"-"`
	RequestFingerprint string     `gorm:"size:128;not null" json:"-"`
	State              string     `gorm:"size:32;not null;index:idx_connector_key_setups_owner_state,priority:3"`
	ErrorCode          *string    `gorm:"size:64"`
	ResultConnectionID *uuid.UUID `gorm:"type:uuid"`
	ExpiresAt          time.Time  `gorm:"not null;index:idx_connector_key_setups_owner_state,priority:4"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (setupAttemptRecord) TableName() string { return "connector_key_setup_attempts" }

func setupAttemptRecordFromDomain(attempt core.KeySetupAttempt) setupAttemptRecord {
	record := setupAttemptRecord{
		ID:                 attempt.ID,
		OrganizationID:     attempt.OrganizationID,
		OwnerAccountID:     attempt.OwnerAccountID,
		ConnectorID:        attempt.ConnectorID.String(),
		ConnectionID:       uuidPointer(attempt.ConnectionID),
		IdempotencyKeyHash: attempt.IdempotencyKeyHash,
		RequestFingerprint: attempt.RequestFingerprint,
		State:              string(attempt.State),
		ResultConnectionID: uuidPointer(attempt.ResultConnectionID),
		ExpiresAt:          attempt.ExpiresAt,
		CreatedAt:          attempt.CreatedAt,
		UpdatedAt:          attempt.UpdatedAt,
	}
	if attempt.Target != nil {
		targetType := string(attempt.Target.Type)
		targetID := attempt.Target.ID
		record.TargetType = &targetType
		record.TargetID = &targetID
		if attempt.Target.BindingInstanceID != "" {
			bindingInstanceID := attempt.Target.BindingInstanceID
			record.BindingInstanceID = &bindingInstanceID
		}
	}
	if attempt.ErrorCode != "" {
		errorCode := string(attempt.ErrorCode)
		record.ErrorCode = &errorCode
	}
	return record
}

func (record setupAttemptRecord) toDomain() (core.KeySetupAttempt, error) {
	connectorID, err := core.NormalizeIdentifier(record.ConnectorID)
	if err != nil {
		return core.KeySetupAttempt{}, internalError("decode connector key setup identity")
	}
	attempt := core.KeySetupAttempt{
		ID:                 record.ID,
		OrganizationID:     record.OrganizationID,
		OwnerAccountID:     record.OwnerAccountID,
		ConnectorID:        connectorID,
		ConnectionID:       uuidValue(record.ConnectionID),
		IdempotencyKeyHash: record.IdempotencyKeyHash,
		RequestFingerprint: record.RequestFingerprint,
		State:              core.SetupState(record.State),
		ResultConnectionID: uuidValue(record.ResultConnectionID),
		ExpiresAt:          record.ExpiresAt,
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
	}
	if record.TargetType != nil || record.TargetID != nil || record.BindingInstanceID != nil {
		if record.TargetType == nil || record.TargetID == nil {
			return core.KeySetupAttempt{}, internalError("decode connector key setup target")
		}
		attempt.Target = &core.TargetRef{
			Type:              core.TargetType(*record.TargetType),
			ID:                *record.TargetID,
			BindingInstanceID: stringValue(record.BindingInstanceID),
		}
	}
	if record.ErrorCode != nil {
		attempt.ErrorCode = core.ErrorCode(*record.ErrorCode)
	}
	if err := attempt.Validate(); err != nil {
		return core.KeySetupAttempt{}, internalError("decode connector key setup")
	}
	return attempt, nil
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	copyValue := value
	return &copyValue
}

func uuidValue(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
