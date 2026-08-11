package integrations

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memoryConnectionRepository struct {
	mu                  sync.Mutex
	connections         map[uuid.UUID]*IntegrationConnection
	defaultID           uuid.UUID
	getByIDCall         int
	deleteErr           error
	deleteCalls         int
	deleteEvent         func()
	oauthUpdateFailures int
	oauthUpdateErr      error
	oauthUpdateCalls    int
}

func newMemoryConnectionRepository() *memoryConnectionRepository {
	return &memoryConnectionRepository{connections: make(map[uuid.UUID]*IntegrationConnection)}
}

func (repository *memoryConnectionRepository) Create(_ context.Context, connection *IntegrationConnection) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, existing := range repository.connections {
		if existing.OrganizationID == connection.OrganizationID && existing.IntegrationID == connection.IntegrationID && strings.EqualFold(existing.Name, connection.Name) {
			return ErrConnectionNameConflict
		}
	}
	if connection.LoadedCredentialVersion < 1 {
		connection.LoadedCredentialVersion = connection.CredentialVersion
	}
	if connection.Revision < 1 {
		connection.Revision = 1
	}
	if connection.HealthRevision < 1 {
		connection.HealthRevision = 1
	}
	connection.LoadedRevision = connection.Revision
	connection.LoadedHealthRevision = connection.HealthRevision
	repository.connections[connection.ID] = cloneConnectionForTest(connection)
	return nil
}

func (repository *memoryConnectionRepository) GetByID(_ context.Context, organizationID, connectionID uuid.UUID) (*IntegrationConnection, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.getByIDCall++
	connection := repository.connections[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return nil, ErrConnectionNotFound
	}
	return cloneConnectionForTest(connection), nil
}

func (repository *memoryConnectionRepository) List(_ context.Context, organizationID uuid.UUID, filter ConnectionListFilter) ([]*IntegrationConnection, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	result := make([]*IntegrationConnection, 0, len(repository.connections))
	for _, connection := range repository.connections {
		if connection.OrganizationID != organizationID {
			continue
		}
		if filter.IntegrationID != "" && connection.IntegrationID != filter.IntegrationID {
			continue
		}
		if filter.DriverID != "" && connection.DriverID != filter.DriverID {
			continue
		}
		if len(filter.CredentialSources) > 0 {
			matched := false
			for _, source := range filter.CredentialSources {
				matched = matched || connection.CredentialSource == source
			}
			if !matched {
				continue
			}
		}
		if filter.OwnerAccountID != nil && (connection.OwnerAccountID == nil || *connection.OwnerAccountID != *filter.OwnerAccountID) {
			continue
		}
		if len(filter.Statuses) > 0 {
			matched := false
			for _, status := range filter.Statuses {
				matched = matched || connection.Status == status
			}
			if !matched {
				continue
			}
		}
		result = append(result, cloneConnectionForTest(connection))
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].IntegrationID != result[right].IntegrationID {
			return result[left].IntegrationID < result[right].IntegrationID
		}
		if result[left].IsDefault != result[right].IsDefault {
			return result[left].IsDefault
		}
		if result[left].Name != result[right].Name {
			return result[left].Name < result[right].Name
		}
		return result[left].ID.String() < result[right].ID.String()
	})
	if filter.PageSize > 0 {
		page := filter.Page
		if page < 1 {
			page = 1
		}
		start := (page - 1) * filter.PageSize
		if start >= len(result) {
			return []*IntegrationConnection{}, nil
		}
		end := start + filter.PageSize
		if end > len(result) {
			end = len(result)
		}
		result = result[start:end]
	}
	return result, nil
}

func (repository *memoryConnectionRepository) Count(_ context.Context, organizationID uuid.UUID, filter ConnectionListFilter) (int64, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	var total int64
	for _, connection := range repository.connections {
		if connection.OrganizationID != organizationID {
			continue
		}
		if filter.IntegrationID != "" && connection.IntegrationID != filter.IntegrationID {
			continue
		}
		if filter.DriverID != "" && connection.DriverID != filter.DriverID {
			continue
		}
		if len(filter.CredentialSources) > 0 {
			matched := false
			for _, source := range filter.CredentialSources {
				matched = matched || connection.CredentialSource == source
			}
			if !matched {
				continue
			}
		}
		if filter.OwnerAccountID != nil && (connection.OwnerAccountID == nil || *connection.OwnerAccountID != *filter.OwnerAccountID) {
			continue
		}
		if len(filter.Statuses) > 0 {
			matched := false
			for _, status := range filter.Statuses {
				matched = matched || connection.Status == status
			}
			if !matched {
				continue
			}
		}
		total++
	}
	return total, nil
}

func (repository *memoryConnectionRepository) GetDefault(_ context.Context, organizationID uuid.UUID, integrationID, driverID string) (*IntegrationConnection, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	connection := repository.connections[repository.defaultID]
	if connection == nil || connection.OrganizationID != organizationID || connection.IntegrationID != integrationID || connection.DriverID != driverID || !connection.IsDefault {
		return nil, ErrConnectionNotFound
	}
	return cloneConnectionForTest(connection), nil
}

func (repository *memoryConnectionRepository) Update(_ context.Context, connection *IntegrationConnection) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if _, exists := repository.connections[connection.ID]; !exists {
		return ErrConnectionNotFound
	}
	stored := repository.connections[connection.ID]
	expectedVersion := connection.LoadedCredentialVersion
	if expectedVersion < 1 {
		expectedVersion = connection.CredentialVersion
	}
	if stored.CredentialVersion != expectedVersion {
		return ErrConnectionChanged
	}
	expectedRevision := connection.LoadedRevision
	if expectedRevision < 1 {
		expectedRevision = connection.Revision
	}
	if stored.Revision != expectedRevision {
		return ErrConnectionChanged
	}
	expectedHealthRevision := connection.LoadedHealthRevision
	if expectedHealthRevision < 1 {
		expectedHealthRevision = connection.HealthRevision
	}
	if stored.HealthRevision != expectedHealthRevision {
		return ErrConnectionChanged
	}
	connection.LoadedCredentialVersion = connection.CredentialVersion
	connection.Revision = expectedRevision + 1
	connection.LoadedRevision = connection.Revision
	connection.LoadedHealthRevision = connection.HealthRevision
	repository.connections[connection.ID] = cloneConnectionForTest(connection)
	if connection.IsDefault {
		repository.defaultID = connection.ID
	} else if repository.defaultID == connection.ID {
		repository.defaultID = uuid.Nil
	}
	return nil
}

func (repository *memoryConnectionRepository) UpdateOAuthCredentials(_ context.Context, connection *IntegrationConnection, expectedCredentialVersion int) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.oauthUpdateCalls++
	if repository.oauthUpdateFailures > 0 {
		repository.oauthUpdateFailures--
		if repository.oauthUpdateErr != nil {
			return repository.oauthUpdateErr
		}
		return fmt.Errorf("temporary OAuth credential storage failure")
	}
	stored := repository.connections[connection.ID]
	if stored == nil || stored.OrganizationID != connection.OrganizationID {
		return ErrConnectionNotFound
	}
	if stored.CredentialVersion != expectedCredentialVersion {
		return ErrConnectionChanged
	}
	if connection.CredentialVersion != expectedCredentialVersion+1 || connection.EncryptedCredentials == nil {
		return fmt.Errorf("integration OAuth credential update is invalid")
	}
	stored.EncryptedCredentials = cloneStringPointer(connection.EncryptedCredentials)
	stored.CredentialVersion = connection.CredentialVersion
	stored.GrantedScopes = append([]string(nil), connection.GrantedScopes...)
	stored.VerifiedActionIDs = append([]string(nil), connection.VerifiedActionIDs...)
	stored.DeniedActionIDs = append([]string(nil), connection.DeniedActionIDs...)
	stored.TokenExpiresAt = cloneTimePointer(connection.TokenExpiresAt)
	stored.RefreshTokenExpiresAt = cloneTimePointer(connection.RefreshTokenExpiresAt)
	stored.NextTokenRefreshAt = cloneTimePointer(connection.NextTokenRefreshAt)
	stored.AuthStatus = connection.AuthStatus
	stored.ScopeStatus = connection.ScopeStatus
	stored.AttentionCode = cloneStringPointer(connection.AttentionCode)
	stored.LastErrorCode = cloneStringPointer(connection.LastErrorCode)
	stored.UpdatedBy = nil
	stored.Revision++
	stored.LoadedCredentialVersion = stored.CredentialVersion
	stored.LoadedRevision = stored.Revision
	connection.LoadedCredentialVersion = connection.CredentialVersion
	return nil
}

func (repository *memoryConnectionRepository) SetDefault(_ context.Context, organizationID, connectionID uuid.UUID) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	target := repository.connections[connectionID]
	if target == nil || target.OrganizationID != organizationID {
		return ErrConnectionNotFound
	}
	if target.Status != ConnectionStatusActive {
		return ErrConnectionNotFound
	}
	if target.ExpiresAt != nil && !target.ExpiresAt.After(time.Now().UTC()) {
		return ErrConnectionNotFound
	}
	for _, connection := range repository.connections {
		if connection.OrganizationID == organizationID && connection.IntegrationID == target.IntegrationID {
			connection.IsDefault = false
			connection.Revision++
			connection.LoadedRevision = connection.Revision
		}
	}
	target.IsDefault = true
	target.Revision++
	target.LoadedRevision = target.Revision
	repository.defaultID = connectionID
	return nil
}

func (repository *memoryConnectionRepository) Delete(_ context.Context, organizationID, connectionID uuid.UUID) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.deleteCalls++
	if repository.deleteEvent != nil {
		repository.deleteEvent()
	}
	if repository.deleteErr != nil {
		return repository.deleteErr
	}
	connection := repository.connections[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return ErrConnectionNotFound
	}
	delete(repository.connections, connectionID)
	if repository.defaultID == connectionID {
		repository.defaultID = uuid.Nil
	}
	return nil
}

func (repository *memoryConnectionRepository) stored(connectionID uuid.UUID) *IntegrationConnection {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return cloneConnectionForTest(repository.connections[connectionID])
}

func cloneConnectionForTest(connection *IntegrationConnection) *IntegrationConnection {
	if connection == nil {
		return nil
	}
	copyValue := *connection
	copyValue.EncryptedCredentials = cloneStringPointer(connection.EncryptedCredentials)
	copyValue.Config = cloneAnyMap(connection.Config)
	copyValue.GrantedScopes = append([]string(nil), connection.GrantedScopes...)
	copyValue.VerifiedActionIDs = append([]string(nil), connection.VerifiedActionIDs...)
	copyValue.DeniedActionIDs = append([]string(nil), connection.DeniedActionIDs...)
	copyValue.AccountID = cloneStringPointer(connection.AccountID)
	copyValue.DisplayName = cloneStringPointer(connection.DisplayName)
	copyValue.LastTestedAt = cloneTimePointer(connection.LastTestedAt)
	copyValue.LastErrorCode = cloneStringPointer(connection.LastErrorCode)
	copyValue.ExpiresAt = cloneTimePointer(connection.ExpiresAt)
	copyValue.OwnerAccountID = cloneUUIDPointer(connection.OwnerAccountID)
	copyValue.AttentionCode = cloneStringPointer(connection.AttentionCode)
	copyValue.MissingRequiredScopes = append([]string(nil), connection.MissingRequiredScopes...)
	copyValue.LastHealthCheckedAt = cloneTimePointer(connection.LastHealthCheckedAt)
	copyValue.LastHealthyAt = cloneTimePointer(connection.LastHealthyAt)
	copyValue.LastRuntimeSuccessAt = cloneTimePointer(connection.LastRuntimeSuccessAt)
	copyValue.LastRuntimeFailureAt = cloneTimePointer(connection.LastRuntimeFailureAt)
	copyValue.ScopeCheckedAt = cloneTimePointer(connection.ScopeCheckedAt)
	copyValue.TokenExpiresAt = cloneTimePointer(connection.TokenExpiresAt)
	copyValue.RefreshTokenExpiresAt = cloneTimePointer(connection.RefreshTokenExpiresAt)
	copyValue.NextTokenRefreshAt = cloneTimePointer(connection.NextTokenRefreshAt)
	copyValue.CreatedBy = cloneUUIDPointer(connection.CreatedBy)
	copyValue.UpdatedBy = cloneUUIDPointer(connection.UpdatedBy)
	return &copyValue
}

type staticConnectionCatalog struct {
	driver  string
	actions []ActionDefinition
}

func (catalog staticConnectionCatalog) DriverForIntegration(integrationID string) (string, bool) {
	if integrationID != IntegrationWebSearch || catalog.driver == "" {
		return "", false
	}
	return catalog.driver, true
}

func (catalog staticConnectionCatalog) Actions(integrationID string) []ActionDefinition {
	if integrationID != IntegrationWebSearch {
		return nil
	}
	return append([]ActionDefinition(nil), catalog.actions...)
}
