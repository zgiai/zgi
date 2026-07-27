package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrConnectionNotFound     = errors.New("integration connection not found")
	ErrConnectionChanged      = errors.New("integration connection changed concurrently")
	ErrConnectionNameConflict = errors.New("integration connection name already exists")
	ErrConnectionInUse        = errors.New("integration connection is bound to an Agent")
)

type ConnectionListFilter struct {
	IntegrationID     string
	DriverID          string
	CredentialSources []ConnectionCredentialSource
	OwnerAccountID    *uuid.UUID
	Statuses          []ConnectionStatus
	Page              int
	PageSize          int
}

type ConnectionRepository interface {
	Create(ctx context.Context, connection *IntegrationConnection) error
	GetByID(ctx context.Context, organizationID, connectionID uuid.UUID) (*IntegrationConnection, error)
	List(ctx context.Context, organizationID uuid.UUID, filter ConnectionListFilter) ([]*IntegrationConnection, error)
	Count(ctx context.Context, organizationID uuid.UUID, filter ConnectionListFilter) (int64, error)
	GetDefault(ctx context.Context, organizationID uuid.UUID, integrationID, driverID string) (*IntegrationConnection, error)
	Update(ctx context.Context, connection *IntegrationConnection) error
	SetDefault(ctx context.Context, organizationID, connectionID uuid.UUID) error
	Delete(ctx context.Context, organizationID, connectionID uuid.UUID) error
}

// OAuthCredentialRepository updates only the rotating OAuth credential
// material and its directly-derived token metadata. The compare-and-swap is
// intentionally scoped to credential_version so unrelated name, grant, or
// health updates cannot discard a provider-issued single-use refresh token.
type OAuthCredentialRepository interface {
	UpdateOAuthCredentials(ctx context.Context, connection *IntegrationConnection, expectedCredentialVersion int) error
}

type GormConnectionRepository struct {
	db *gorm.DB
}

func NewGormConnectionRepository(db *gorm.DB) *GormConnectionRepository {
	return &GormConnectionRepository{db: db}
}

func (repository *GormConnectionRepository) Create(ctx context.Context, connection *IntegrationConnection) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration connection repository is unavailable")
	}
	if connection == nil {
		return fmt.Errorf("integration connection is required")
	}
	if err := repository.db.WithContext(ctx).Create(connection).Error; err != nil {
		if isDuplicateConnectionError(err) {
			return ErrConnectionNameConflict
		}
		return fmt.Errorf("create integration connection: %w", err)
	}
	return nil
}

func (repository *GormConnectionRepository) GetByID(ctx context.Context, organizationID, connectionID uuid.UUID) (*IntegrationConnection, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("integration connection repository is unavailable")
	}
	var connection IntegrationConnection
	err := repository.db.WithContext(ctx).
		Where("organization_id = ? AND id = ?", organizationID, connectionID).
		First(&connection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration connection: %w", err)
	}
	return &connection, nil
}

func (repository *GormConnectionRepository) List(ctx context.Context, organizationID uuid.UUID, filter ConnectionListFilter) ([]*IntegrationConnection, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("integration connection repository is unavailable")
	}
	query := connectionListQuery(repository.db.WithContext(ctx), organizationID, filter)
	if filter.PageSize > 0 {
		page := filter.Page
		if page < 1 {
			page = 1
		}
		query = query.Limit(filter.PageSize).Offset((page - 1) * filter.PageSize)
	}
	var connections []*IntegrationConnection
	if err := query.Order("integration_id ASC, is_default DESC, name ASC, created_at ASC").Find(&connections).Error; err != nil {
		return nil, fmt.Errorf("list integration connections: %w", err)
	}
	return connections, nil
}

func (repository *GormConnectionRepository) Count(ctx context.Context, organizationID uuid.UUID, filter ConnectionListFilter) (int64, error) {
	if repository == nil || repository.db == nil {
		return 0, fmt.Errorf("integration connection repository is unavailable")
	}
	var total int64
	if err := connectionListQuery(repository.db.WithContext(ctx).Model(&IntegrationConnection{}), organizationID, filter).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("count integration connections: %w", err)
	}
	return total, nil
}

func connectionListQuery(query *gorm.DB, organizationID uuid.UUID, filter ConnectionListFilter) *gorm.DB {
	query = query.Where("organization_id = ?", organizationID)
	if integrationID := strings.ToLower(strings.TrimSpace(filter.IntegrationID)); integrationID != "" {
		query = query.Where("integration_id = ?", integrationID)
	}
	if driverID := strings.ToLower(strings.TrimSpace(filter.DriverID)); driverID != "" {
		query = query.Where("driver_id = ?", driverID)
	}
	if len(filter.CredentialSources) > 0 {
		query = query.Where("credential_source IN ?", filter.CredentialSources)
	}
	if filter.OwnerAccountID != nil {
		query = query.Where("owner_account_id = ?", *filter.OwnerAccountID)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	return query
}

func (repository *GormConnectionRepository) GetDefault(ctx context.Context, organizationID uuid.UUID, integrationID, driverID string) (*IntegrationConnection, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("integration connection repository is unavailable")
	}
	query := repository.db.WithContext(ctx).
		Where("organization_id = ? AND integration_id = ? AND is_default = ? AND status = ?", organizationID, strings.ToLower(strings.TrimSpace(integrationID)), true, ConnectionStatusActive)
	if normalizedDriverID := strings.ToLower(strings.TrimSpace(driverID)); normalizedDriverID != "" {
		query = query.Where("driver_id = ?", normalizedDriverID)
	}
	var connection IntegrationConnection
	err := query.First(&connection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get default integration connection: %w", err)
	}
	return &connection, nil
}

func (repository *GormConnectionRepository) Update(ctx context.Context, connection *IntegrationConnection) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration connection repository is unavailable")
	}
	if connection == nil || connection.ID == uuid.Nil || connection.OrganizationID == uuid.Nil {
		return fmt.Errorf("integration connection identity is required")
	}
	configJSON, err := json.Marshal(connection.Config)
	if err != nil {
		return fmt.Errorf("encode integration connection config: %w", err)
	}
	scopesJSON, err := json.Marshal(connection.GrantedScopes)
	if err != nil {
		return fmt.Errorf("encode integration connection scopes: %w", err)
	}
	missingScopesJSON, err := json.Marshal(connection.MissingRequiredScopes)
	if err != nil {
		return fmt.Errorf("encode integration connection missing scopes: %w", err)
	}
	updates := map[string]any{
		"name":                     connection.Name,
		"credential_source":        connection.CredentialSource,
		"auth_type":                connection.AuthType,
		"auth_method_id":           connection.AuthMethodID,
		"owner_account_id":         connection.OwnerAccountID,
		"encrypted_credentials":    connection.EncryptedCredentials,
		"config":                   datatypes.JSON(configJSON),
		"account_id":               connection.AccountID,
		"display_name":             connection.DisplayName,
		"granted_scopes":           datatypes.JSON(scopesJSON),
		"status":                   connection.Status,
		"is_default":               connection.IsDefault,
		"credential_version":       connection.CredentialVersion,
		"last_tested_at":           connection.LastTestedAt,
		"last_error_code":          connection.LastErrorCode,
		"expires_at":               connection.ExpiresAt,
		"health_status":            connection.HealthStatus,
		"auth_status":              connection.AuthStatus,
		"scope_status":             connection.ScopeStatus,
		"attention_code":           connection.AttentionCode,
		"missing_required_scopes":  datatypes.JSON(missingScopesJSON),
		"last_health_checked_at":   connection.LastHealthCheckedAt,
		"last_healthy_at":          connection.LastHealthyAt,
		"last_runtime_success_at":  connection.LastRuntimeSuccessAt,
		"last_runtime_failure_at":  connection.LastRuntimeFailureAt,
		"scope_checked_at":         connection.ScopeCheckedAt,
		"consecutive_failures":     connection.ConsecutiveFailures,
		"health_revision":          connection.HealthRevision,
		"token_expires_at":         connection.TokenExpiresAt,
		"refresh_token_expires_at": connection.RefreshTokenExpiresAt,
		"next_token_refresh_at":    connection.NextTokenRefreshAt,
		"updated_by":               connection.UpdatedBy,
		"updated_at":               gorm.Expr("CURRENT_TIMESTAMP"),
	}
	expectedCredentialVersion := connection.LoadedCredentialVersion
	if expectedCredentialVersion < 1 {
		expectedCredentialVersion = connection.CredentialVersion
	}
	expectedRevision := connection.LoadedRevision
	if expectedRevision < 1 {
		expectedRevision = connection.Revision
	}
	if expectedRevision < 1 {
		expectedRevision = 1
	}
	expectedHealthRevision := connection.LoadedHealthRevision
	if expectedHealthRevision < 1 {
		expectedHealthRevision = connection.HealthRevision
	}
	if expectedHealthRevision < 1 {
		expectedHealthRevision = 1
	}
	nextRevision := expectedRevision + 1
	updates["revision"] = nextRevision
	result := repository.db.WithContext(ctx).Model(&IntegrationConnection{}).
		Where("organization_id = ? AND id = ? AND credential_version = ? AND revision = ? AND health_revision = ?", connection.OrganizationID, connection.ID, expectedCredentialVersion, expectedRevision, expectedHealthRevision).
		Updates(updates)
	if result.Error != nil {
		if isDuplicateConnectionError(result.Error) {
			return ErrConnectionNameConflict
		}
		return fmt.Errorf("update integration connection: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrConnectionChanged
	}
	connection.LoadedCredentialVersion = connection.CredentialVersion
	connection.Revision = nextRevision
	connection.LoadedRevision = nextRevision
	connection.LoadedHealthRevision = connection.HealthRevision
	return nil
}

func (repository *GormConnectionRepository) UpdateOAuthCredentials(ctx context.Context, connection *IntegrationConnection, expectedCredentialVersion int) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration connection repository is unavailable")
	}
	if connection == nil || connection.ID == uuid.Nil || connection.OrganizationID == uuid.Nil ||
		expectedCredentialVersion < 1 || connection.CredentialVersion != expectedCredentialVersion+1 ||
		connection.EncryptedCredentials == nil {
		return fmt.Errorf("integration OAuth credential update is invalid")
	}
	scopesJSON, err := json.Marshal(connection.GrantedScopes)
	if err != nil {
		return fmt.Errorf("encode integration connection scopes: %w", err)
	}
	updates := map[string]any{
		"encrypted_credentials":    connection.EncryptedCredentials,
		"credential_version":       connection.CredentialVersion,
		"granted_scopes":           datatypes.JSON(scopesJSON),
		"token_expires_at":         connection.TokenExpiresAt,
		"refresh_token_expires_at": connection.RefreshTokenExpiresAt,
		"next_token_refresh_at":    connection.NextTokenRefreshAt,
		"auth_status":              connection.AuthStatus,
		"scope_status":             connection.ScopeStatus,
		"attention_code":           connection.AttentionCode,
		"last_error_code":          connection.LastErrorCode,
		"updated_by":               nil,
		"revision":                 gorm.Expr("revision + 1"),
		"updated_at":               gorm.Expr("CURRENT_TIMESTAMP"),
	}
	result := repository.db.WithContext(ctx).Model(&IntegrationConnection{}).
		Where("organization_id = ? AND id = ? AND credential_version = ?", connection.OrganizationID, connection.ID, expectedCredentialVersion).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update integration OAuth credentials: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrConnectionChanged
	}
	connection.LoadedCredentialVersion = connection.CredentialVersion
	connection.LoadedRevision = 0
	return nil
}

func isDuplicateConnectionError(err error) bool {
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

func (repository *GormConnectionRepository) SetDefault(ctx context.Context, organizationID, connectionID uuid.UUID) error {
	return repository.SetDefaultAs(ctx, organizationID, connectionID, nil)
}

func (repository *GormConnectionRepository) SetDefaultAs(ctx context.Context, organizationID, connectionID uuid.UUID, actorID *uuid.UUID) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration connection repository is unavailable")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var targetIdentity IntegrationConnection
		err := tx.Select("id", "integration_id", "credential_source").
			Where("organization_id = ? AND id = ?", organizationID, connectionID).
			First(&targetIdentity).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConnectionNotFound
		}
		if err != nil {
			return fmt.Errorf("find integration connection for default selection: %w", err)
		}
		if targetIdentity.CredentialSource == ConnectionCredentialSourceAccount {
			return ErrConnectionNotFound
		}

		// Lock every connection in a stable order so concurrent default changes
		// for the same organization/integration cannot race the partial unique
		// index or deadlock by taking target rows in opposite order.
		var connections []IntegrationConnection
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND integration_id = ?", organizationID, targetIdentity.IntegrationID).
			Order("id ASC").
			Find(&connections).Error; err != nil {
			return fmt.Errorf("lock integration connections for default selection: %w", err)
		}
		var target *IntegrationConnection
		for index := range connections {
			if connections[index].ID == connectionID {
				target = &connections[index]
				break
			}
		}
		if target == nil {
			return ErrConnectionNotFound
		}
		if target.Status != ConnectionStatusActive {
			return NewError(ErrorCodeConnectionInvalid, "only an active integration connection can be the default", nil)
		}
		if target.ExpiresAt != nil && !target.ExpiresAt.After(time.Now().UTC()) {
			return NewError(ErrorCodeConnectionInvalid, "an expired integration connection cannot be the default", nil)
		}
		clearUpdates := map[string]any{"is_default": false, "revision": gorm.Expr("revision + 1"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
		setUpdates := map[string]any{"is_default": true, "revision": gorm.Expr("revision + 1"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
		if actorID != nil && *actorID != uuid.Nil {
			clearUpdates["updated_by"] = *actorID
			setUpdates["updated_by"] = *actorID
		}
		if err := tx.Model(&IntegrationConnection{}).
			Where("organization_id = ? AND integration_id = ? AND is_default = ?", organizationID, target.IntegrationID, true).
			Updates(clearUpdates).Error; err != nil {
			return fmt.Errorf("clear default integration connection: %w", err)
		}
		result := tx.Model(&IntegrationConnection{}).
			Where("organization_id = ? AND id = ? AND status = ?", organizationID, connectionID, ConnectionStatusActive).
			Updates(setUpdates)
		if result.Error != nil {
			return fmt.Errorf("set default integration connection: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrConnectionNotFound
		}
		return nil
	})
}

func (repository *GormConnectionRepository) Delete(ctx context.Context, organizationID, connectionID uuid.UUID) error {
	return repository.DeleteAs(ctx, organizationID, connectionID, nil)
}

func (repository *GormConnectionRepository) DeleteAs(ctx context.Context, organizationID, connectionID uuid.UUID, actorID *uuid.UUID) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration connection repository is unavailable")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := deleteIntegrationConnectionTx(tx, organizationID, connectionID, actorID, nil)
		return err
	})
}

// DeleteWithOAuthRevocation atomically persists the encrypted revocation
// operation and removes the local connection. No provider call is allowed
// before this transaction commits.
func (repository *GormConnectionRepository) DeleteWithOAuthRevocation(
	ctx context.Context,
	organizationID, connectionID uuid.UUID,
	actorID *uuid.UUID,
	task OAuthRecoveryTask,
) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration connection repository is unavailable")
	}
	task = normalizeOAuthRecoveryTask(task, time.Now().UTC())
	if err := validateOAuthRecoveryTask(task); err != nil {
		return err
	}
	if task.Kind != OAuthRecoveryRevoke ||
		task.EncryptedClientCredentials == "" ||
		task.ClientCredentialVersion < 1 ||
		task.ExpectedConnectionRevision < 1 ||
		task.OrganizationID != organizationID ||
		task.ConnectionID != connectionID {
		return fmt.Errorf("integration OAuth durable revocation task is invalid")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := deleteIntegrationConnectionTx(tx, organizationID, connectionID, actorID, &task)
		return err
	})
}

func deleteIntegrationConnectionTx(
	tx *gorm.DB,
	organizationID, connectionID uuid.UUID,
	actorID *uuid.UUID,
	recoveryTask *OAuthRecoveryTask,
) (*IntegrationConnection, error) {
	var target IntegrationConnection
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("organization_id = ? AND id = ?", organizationID, connectionID).
		First(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock integration connection for deletion: %w", err)
	}
	if target.CredentialSource == ConnectionCredentialSourceAccount &&
		(target.OwnerAccountID == nil || actorID == nil || *actorID == uuid.Nil || *target.OwnerAccountID != *actorID) {
		return nil, ErrConnectionNotFound
	}
	var boundAgentCount int64
	if err := tx.Table("agent_resource_bindings").
		Where("organization_id = ? AND binding_type = ? AND resource_id = ?", organizationID, "integration_connection", connectionID.String()).
		Count(&boundAgentCount).Error; err != nil {
		return nil, fmt.Errorf("check Agent integration connection bindings: %w", err)
	}
	if boundAgentCount > 0 {
		return nil, ErrConnectionInUse
	}
	if recoveryTask != nil {
		if target.AuthType != ConnectionAuthTypeOAuth2 ||
			target.CredentialVersion != recoveryTask.CredentialVersion ||
			target.Revision != recoveryTask.ExpectedConnectionRevision ||
			target.EncryptedCredentials == nil ||
			*target.EncryptedCredentials != recoveryTask.EncryptedCredentials ||
			!strings.EqualFold(target.IntegrationID, recoveryTask.IntegrationID) ||
			!strings.EqualFold(target.DriverID, recoveryTask.DriverID) ||
			!strings.EqualFold(target.AuthMethodID, recoveryTask.AuthMethodID) {
			return nil, ErrConnectionChanged
		}
		record, err := oauthRecoveryRecord(*recoveryTask, time.Now().UTC().Add(15*time.Second))
		if err != nil {
			return nil, err
		}
		if err := tx.Create(record).Error; err != nil {
			return nil, fmt.Errorf("persist integration OAuth revocation before deletion: %w", err)
		}
	}
	updates := map[string]any{
		"is_default":         false,
		"status":             ConnectionStatusDisabled,
		"credential_version": gorm.Expr("credential_version + 1"),
		"revision":           gorm.Expr("revision + 1"),
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if target.CredentialSource == ConnectionCredentialSourceOrganization || target.CredentialSource == ConnectionCredentialSourceAccount {
		// Remove the encrypted envelope before the metadata row is soft
		// deleted. The tombstone satisfies the storage constraint but is
		// deliberately not a valid credential envelope.
		updates["encrypted_credentials"] = "deleted"
	}
	if actorID != nil && *actorID != uuid.Nil {
		updates["updated_by"] = *actorID
	}
	if err := tx.Model(&IntegrationConnection{}).
		Where("organization_id = ? AND id = ?", organizationID, connectionID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("redact integration connection before deletion: %w", err)
	}
	result := tx.Where("organization_id = ? AND id = ?", organizationID, connectionID).
		Delete(&IntegrationConnection{})
	if result.Error != nil {
		return nil, fmt.Errorf("delete integration connection: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, ErrConnectionNotFound
	}
	return &target, nil
}
