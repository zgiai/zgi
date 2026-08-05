package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var integrationIdentifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type CreateConnectionInput struct {
	OrganizationID   uuid.UUID
	IntegrationID    string
	DriverID         string
	Name             string
	CredentialSource ConnectionCredentialSource
	AuthType         ConnectionAuthType
	AuthMethodID     string
	OwnerAccountID   *uuid.UUID
	Credentials      map[string]string `json:"credentials,omitempty"`
	Config           map[string]any
	ExpiresAt        *time.Time
	ActorID          *uuid.UUID
}

type UpdateConnectionInput struct {
	OrganizationID   uuid.UUID
	ConnectionID     uuid.UUID
	ExpectedRevision int
	Name             *string
	Credentials      map[string]string `json:"credentials,omitempty"`
	Config           *map[string]any
	ExpiresAt        *time.Time
	ClearExpiresAt   bool
	Disabled         *bool
	ActorID          *uuid.UUID
}

type ConnectionProfile struct {
	AccountID         string
	DisplayName       string
	GrantedScopes     []string
	ExpiresAt         *time.Time
	ProviderRequestID string
	CostUSD           *float64
}

type ConnectionTester interface {
	ValidateConnection(ctx context.Context, connection *ResolvedConnection) (*ConnectionProfile, error)
}

type ConnectionService interface {
	Create(ctx context.Context, input CreateConnectionInput) (ConnectionView, error)
	Get(ctx context.Context, organizationID, connectionID uuid.UUID) (ConnectionView, error)
	List(ctx context.Context, organizationID uuid.UUID, filter ConnectionListFilter) ([]ConnectionView, error)
	ListPage(ctx context.Context, organizationID uuid.UUID, filter ConnectionListFilter) (ConnectionListPage, error)
	Update(ctx context.Context, input UpdateConnectionInput) (ConnectionView, error)
	Test(ctx context.Context, organizationID, connectionID uuid.UUID, actorID *uuid.UUID) (ConnectionView, *ConnectionProfile, error)
	SetDefault(ctx context.Context, organizationID, connectionID uuid.UUID) (ConnectionView, error)
	Delete(ctx context.Context, organizationID, connectionID uuid.UUID) error
}

type ConnectionListPage struct {
	Items    []ConnectionView `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    int64            `json:"total"`
	HasMore  bool             `json:"has_more"`
}

type ActorAwareConnectionService interface {
	DeleteAs(ctx context.Context, organizationID, connectionID uuid.UUID, actorID *uuid.UUID) error
	SetDefaultAs(ctx context.Context, organizationID, connectionID uuid.UUID, actorID *uuid.UUID) (ConnectionView, error)
}

type DefaultConnectionService struct {
	repository             ConnectionRepository
	cipher                 CredentialCipher
	catalog                ConnectionCatalog
	resolver               ConnectionResolver
	tester                 ConnectionTester
	credentialValidator    ConnectionCredentialValidator
	healthRecorder         ConnectionHealthObservationRecorder
	healthFailureThreshold int
	revoker                ConnectionRevoker
	oauthRecovery          *OAuthRecoveryService
}

func (service *DefaultConnectionService) WithConnectionRevoker(revoker ConnectionRevoker) *DefaultConnectionService {
	if service != nil {
		service.revoker = revoker
	}
	return service
}

func (service *DefaultConnectionService) WithOAuthRecovery(recovery *OAuthRecoveryService) *DefaultConnectionService {
	if service != nil {
		service.oauthRecovery = recovery
	}
	return service
}

func NewConnectionService(repository ConnectionRepository, cipher CredentialCipher, catalog ConnectionCatalog, resolver ConnectionResolver, tester ConnectionTester) *DefaultConnectionService {
	service := &DefaultConnectionService{repository: repository, cipher: cipher, catalog: catalog, resolver: resolver, tester: tester}
	if validator, ok := catalog.(ConnectionCredentialValidator); ok {
		service.credentialValidator = validator
	}
	return service
}

func (service *DefaultConnectionService) connectionView(connection *IntegrationConnection) ConnectionView {
	view := newConnectionView(connection)
	if connection == nil || service == nil || service.catalog == nil {
		return view
	}
	if catalog, ok := service.catalog.(ConnectionPermissionCatalog); ok {
		if definition, exists := catalog.ProviderDefinition(connection.IntegrationID); exists {
			view.PermissionSummary = BuildConnectionPermissionSummary(connection, definition)
		}
	}
	return view
}

// WithCredentialValidator installs provider-owned credential schema validation.
// Non-API-key authentication fails closed unless a validator is configured.
func (service *DefaultConnectionService) WithCredentialValidator(validator ConnectionCredentialValidator) *DefaultConnectionService {
	if service != nil {
		service.credentialValidator = validator
	}
	return service
}

func (service *DefaultConnectionService) WithHealthRecorder(recorder ConnectionHealthObservationRecorder) *DefaultConnectionService {
	if service != nil {
		service.healthRecorder = recorder
	}
	return service
}

func (service *DefaultConnectionService) WithHealthFailureThreshold(threshold int) *DefaultConnectionService {
	if service != nil && threshold > 0 {
		service.healthFailureThreshold = threshold
	}
	return service
}

func (service *DefaultConnectionService) Create(ctx context.Context, input CreateConnectionInput) (ConnectionView, error) {
	if service == nil || service.repository == nil || service.cipher == nil {
		return ConnectionView{}, fmt.Errorf("integration connection service is unavailable")
	}
	defer destroyCredentialMap(input.Credentials)
	integrationID, driverID, err := service.validateTarget(input.IntegrationID, input.DriverID)
	if err != nil {
		return ConnectionView{}, err
	}
	name, err := normalizeConnectionName(input.Name)
	if err != nil {
		return ConnectionView{}, err
	}
	if input.OrganizationID == uuid.Nil {
		return ConnectionView{}, invalidInput("organization id is required", nil)
	}
	if err := validateConnectionConfig(input.Config); err != nil {
		return ConnectionView{}, err
	}

	connection := &IntegrationConnection{
		ID:                    uuid.New(),
		OrganizationID:        input.OrganizationID,
		IntegrationID:         integrationID,
		DriverID:              driverID,
		Name:                  name,
		CredentialSource:      input.CredentialSource,
		AuthType:              input.AuthType,
		AuthMethodID:          strings.ToLower(strings.TrimSpace(input.AuthMethodID)),
		OwnerAccountID:        cloneUUIDPointer(input.OwnerAccountID),
		Config:                cloneAnyMap(input.Config),
		GrantedScopes:         []string{},
		Status:                ConnectionStatusPending,
		HealthStatus:          ConnectionHealthUnknown,
		AuthStatus:            ConnectionAuthUnknown,
		ScopeStatus:           ConnectionScopeUnknown,
		MissingRequiredScopes: []string{},
		HealthRevision:        1,
		CredentialVersion:     1,
		ExpiresAt:             cloneTimePointer(input.ExpiresAt),
		CreatedBy:             cloneUUIDPointer(input.ActorID),
		UpdatedBy:             cloneUUIDPointer(input.ActorID),
	}
	if connection.CredentialSource == "" {
		connection.CredentialSource = ConnectionCredentialSourceOrganization
	}
	if !supportedConnectionCredentialSource(connection.CredentialSource) {
		return ConnectionView{}, invalidInput("credential source must be organization or account", nil)
	}
	if connection.CredentialSource == ConnectionCredentialSourceAccount {
		if connection.OwnerAccountID == nil || *connection.OwnerAccountID == uuid.Nil {
			return ConnectionView{}, invalidInput("account-owned connections require an owner account", nil)
		}
		if input.ActorID == nil || *input.ActorID == uuid.Nil || *connection.OwnerAccountID != *input.ActorID {
			return ConnectionView{}, NewError(ErrorCodeAccessDenied, "personal integration connections can only be created by their owner", nil)
		}
	} else if connection.OwnerAccountID != nil {
		return ConnectionView{}, invalidInput("only account-owned connections can specify an owner account", nil)
	}
	if connection.AuthType == "" {
		connection.AuthType = ConnectionAuthTypeAPIKey
	}
	if err := service.resolveAuthMethod(connection); err != nil {
		return ConnectionView{}, err
	}
	if err := service.applyCredentialSource(ctx, connection, input.Credentials); err != nil {
		return ConnectionView{}, err
	}
	if err := service.repository.Create(ctx, connection); err != nil {
		return ConnectionView{}, mapConnectionLookupError(err)
	}
	return service.connectionView(connection), nil
}

func (service *DefaultConnectionService) Get(ctx context.Context, organizationID, connectionID uuid.UUID) (ConnectionView, error) {
	connection, err := service.repository.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return ConnectionView{}, mapConnectionLookupError(err)
	}
	if err := rejectLegacyPlatformConnection(connection); err != nil {
		return ConnectionView{}, err
	}
	return service.connectionView(connection), nil
}

func (service *DefaultConnectionService) List(ctx context.Context, organizationID uuid.UUID, filter ConnectionListFilter) ([]ConnectionView, error) {
	if organizationID == uuid.Nil {
		return nil, invalidInput("organization id is required", nil)
	}
	filter, supported := supportedConnectionListFilter(filter)
	if !supported {
		return []ConnectionView{}, nil
	}
	connections, err := service.repository.List(ctx, organizationID, filter)
	if err != nil {
		return nil, err
	}
	views := make([]ConnectionView, 0, len(connections))
	for _, connection := range connections {
		views = append(views, service.connectionView(connection))
	}
	return views, nil
}

func (service *DefaultConnectionService) ListPage(ctx context.Context, organizationID uuid.UUID, filter ConnectionListFilter) (ConnectionListPage, error) {
	if organizationID == uuid.Nil {
		return ConnectionListPage{}, invalidInput("organization id is required", nil)
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	var supported bool
	filter, supported = supportedConnectionListFilter(filter)
	if !supported {
		return ConnectionListPage{
			Items: []ConnectionView{}, Page: filter.Page, PageSize: filter.PageSize,
		}, nil
	}
	total, err := service.repository.Count(ctx, organizationID, filter)
	if err != nil {
		return ConnectionListPage{}, err
	}
	items, err := service.List(ctx, organizationID, filter)
	if err != nil {
		return ConnectionListPage{}, err
	}
	return ConnectionListPage{
		Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total,
		HasMore: int64(filter.Page*filter.PageSize) < total,
	}, nil
}

func (service *DefaultConnectionService) Update(ctx context.Context, input UpdateConnectionInput) (ConnectionView, error) {
	if service == nil || service.repository == nil || service.cipher == nil {
		return ConnectionView{}, fmt.Errorf("integration connection service is unavailable")
	}
	defer destroyCredentialMap(input.Credentials)
	connection, err := service.repository.GetByID(ctx, input.OrganizationID, input.ConnectionID)
	if err != nil {
		return ConnectionView{}, mapConnectionLookupError(err)
	}
	if err := rejectLegacyPlatformConnection(connection); err != nil {
		return ConnectionView{}, err
	}
	if err := authorizePersonalConnectionOwner(connection, input.ActorID); err != nil {
		return ConnectionView{}, err
	}
	if input.ExpectedRevision > 0 && connection.Revision != input.ExpectedRevision {
		return ConnectionView{}, NewError(ErrorCodeConnectionConflict, "integration connection changed; reload it and retry", ErrConnectionChanged)
	}
	if input.Name != nil {
		connection.Name, err = normalizeConnectionName(*input.Name)
		if err != nil {
			return ConnectionView{}, err
		}
	}
	if input.Config != nil {
		if err := validateConnectionConfig(*input.Config); err != nil {
			return ConnectionView{}, err
		}
		connection.Config = cloneAnyMap(*input.Config)
	}
	if input.ClearExpiresAt {
		connection.ExpiresAt = nil
	} else if input.ExpiresAt != nil {
		connection.ExpiresAt = cloneTimePointer(input.ExpiresAt)
	}
	if input.Disabled != nil {
		if *input.Disabled {
			connection.Status = ConnectionStatusDisabled
			connection.IsDefault = false
		} else if connection.Status == ConnectionStatusDisabled {
			connection.Status = ConnectionStatusPending
		}
	}
	if input.Credentials != nil {
		connection.CredentialVersion++
		if err := service.resolveAuthMethod(connection); err != nil {
			return ConnectionView{}, err
		}
		if err := service.applyCredentialSource(ctx, connection, input.Credentials); err != nil {
			return ConnectionView{}, err
		}
		connection.Status = ConnectionStatusPending
		connection.IsDefault = false
		connection.LastErrorCode = nil
		connection.LastTestedAt = nil
		connection.HealthStatus = ConnectionHealthUnknown
		connection.AuthStatus = ConnectionAuthUnknown
		connection.ScopeStatus = ConnectionScopeUnknown
		connection.AttentionCode = nil
		connection.MissingRequiredScopes = []string{}
		connection.ConsecutiveFailures = 0
		connection.LastHealthCheckedAt = nil
		connection.HealthRevision++
	}
	connection.UpdatedBy = cloneUUIDPointer(input.ActorID)
	if err := service.repository.Update(ctx, connection); err != nil {
		return ConnectionView{}, mapConnectionLookupError(err)
	}
	updated, err := service.repository.GetByID(ctx, input.OrganizationID, input.ConnectionID)
	if err != nil {
		return ConnectionView{}, mapConnectionLookupError(err)
	}
	return service.connectionView(updated), nil
}

func (service *DefaultConnectionService) Test(ctx context.Context, organizationID, connectionID uuid.UUID, actorID *uuid.UUID) (ConnectionView, *ConnectionProfile, error) {
	if service == nil || service.repository == nil || service.resolver == nil || service.tester == nil {
		return ConnectionView{}, nil, fmt.Errorf("integration connection testing is unavailable")
	}
	connection, err := service.repository.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return ConnectionView{}, nil, mapConnectionLookupError(err)
	}
	if err := rejectLegacyPlatformConnection(connection); err != nil {
		return ConnectionView{}, nil, err
	}
	if err := authorizePersonalConnectionOwner(connection, actorID); err != nil {
		return ConnectionView{}, nil, err
	}
	testResolver, ok := service.resolver.(interface {
		ResolveRecordForTest(context.Context, *IntegrationConnection) (*ResolvedConnection, error)
	})
	if !ok {
		return service.connectionView(connection), nil, NewError(ErrorCodeConnectionInvalid, "integration connection test resolver is unavailable", nil)
	}
	resolved, connection, err := service.resolveConnectionSnapshotForTest(ctx, testResolver, connection, actorID)
	if err != nil {
		return service.connectionView(connection), nil, err
	}
	defer resolved.Destroy()
	var profile *ConnectionProfile
	var testErr error
	testStartedAt := time.Now()
	if actorAware, ok := service.tester.(interface {
		ValidateConnectionAs(context.Context, *ResolvedConnection, *uuid.UUID) (*ConnectionProfile, error)
	}); ok {
		profile, testErr = actorAware.ValidateConnectionAs(ctx, resolved, actorID)
	} else {
		profile, testErr = service.tester.ValidateConnection(ctx, resolved)
	}
	if testErr != nil && ErrorCode(testErr) == ErrorCodeAuditFailed {
		// Audit infrastructure failures say nothing about credential health. In
		// particular, audit creation can fail before the provider is called, and
		// completion persistence can fail after a successful paid call. Preserve
		// the current status, default selection, and last tested timestamp.
		return service.connectionView(connection), profile, testErr
	}
	now := time.Now().UTC()
	connection.LastTestedAt = &now
	connection.LastHealthCheckedAt = &now
	connection.UpdatedBy = cloneUUIDPointer(actorID)
	connection.LastErrorCode = nil
	if testErr != nil {
		code := ErrorCode(testErr)
		connection.LastErrorCode = &code
		failureThreshold := service.healthFailureThreshold
		if failureThreshold < 1 {
			failureThreshold = 3
		}
		applyConnectionTestFailureSummary(connection, code, failureThreshold)
	} else {
		connection.Status = ConnectionStatusActive
		connection.AuthStatus = ConnectionAuthValid
		connection.ConsecutiveFailures = 0
		connection.LastHealthyAt = &now
		if connection.ScopeStatus == ConnectionScopeDrifted {
			connection.HealthStatus = ConnectionHealthDegraded
			connection.AttentionCode = stringPointer(ConnectionAttentionScopeUpdateRequired)
		} else {
			connection.HealthStatus = ConnectionHealthHealthy
			connection.AttentionCode = nil
		}
		if profile != nil {
			connection.AccountID = optionalBoundedString(profile.AccountID, 255)
			connection.DisplayName = optionalBoundedString(profile.DisplayName, 255)
			connection.GrantedScopes = normalizeScopes(profile.GrantedScopes)
			if profile.GrantedScopes != nil {
				connection.ScopeStatus = ConnectionScopeVerified
				connection.HealthStatus = ConnectionHealthHealthy
				connection.AttentionCode = nil
				connection.ScopeCheckedAt = &now
				connection.MissingRequiredScopes = []string{}
			}
			if profile.ExpiresAt != nil {
				connection.ExpiresAt = cloneTimePointer(profile.ExpiresAt)
				connection.TokenExpiresAt = cloneTimePointer(profile.ExpiresAt)
			}
		}
	}
	connection.HealthRevision++
	if updateErr := service.repository.Update(ctx, connection); updateErr != nil {
		return ConnectionView{}, nil, mapConnectionLookupError(updateErr)
	}
	recordConnectionTestHealth(ctx, service.healthRecorder, connection, profile, testErr, ConnectionHealthSourceManual, actorID, testStartedAt)
	if testErr != nil {
		return service.connectionView(connection), profile, testErr
	}
	return service.connectionView(connection), profile, nil
}

// resolveConnectionSnapshotForTest returns a resolved credential snapshot and
// the exact repository revision from which it was produced. OAuth resolution
// may refresh and persist a token before the provider test starts; reloading
// here prevents that expected self-update from being mistaken for an
// ambiguous connection or a concurrent user edit. A bounded retry also absorbs
// metadata updates that finish before the provider request, while the final
// repository CAS still protects changes made during the request itself.
func (service *DefaultConnectionService) resolveConnectionSnapshotForTest(
	ctx context.Context,
	resolver interface {
		ResolveRecordForTest(context.Context, *IntegrationConnection) (*ResolvedConnection, error)
	},
	connection *IntegrationConnection,
	actorID *uuid.UUID,
) (*ResolvedConnection, *IntegrationConnection, error) {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resolved, err := resolver.ResolveRecordForTest(ctx, connection)
		if err != nil {
			return nil, connection, err
		}
		latest, err := service.repository.GetByID(ctx, connection.OrganizationID, connection.ID)
		if err != nil {
			resolved.Destroy()
			return nil, connection, mapConnectionLookupError(err)
		}
		if err := rejectLegacyPlatformConnection(latest); err != nil {
			resolved.Destroy()
			return nil, latest, err
		}
		if err := authorizePersonalConnectionOwner(latest, actorID); err != nil {
			resolved.Destroy()
			return nil, latest, err
		}
		if latest.Status == ConnectionStatusDisabled {
			resolved.Destroy()
			return nil, latest, NewError(ErrorCodeConnectionConflict, "integration connection changed during test setup; retry", ErrConnectionChanged)
		}
		if resolvedConnectionMatchesRecord(resolved, latest) {
			return resolved, latest, nil
		}
		resolved.Destroy()
		connection = latest
	}
	return nil, connection, NewError(ErrorCodeConnectionConflict, "integration connection kept changing during test setup; retry", ErrConnectionChanged)
}

func resolvedConnectionMatchesRecord(resolved *ResolvedConnection, connection *IntegrationConnection) bool {
	if resolved == nil || connection == nil {
		return false
	}
	return resolved.ID == connection.ID.String() &&
		resolved.OrganizationID == connection.OrganizationID.String() &&
		strings.EqualFold(resolved.IntegrationID, connection.IntegrationID) &&
		strings.EqualFold(resolved.DriverID, connection.DriverID) &&
		resolved.CredentialSource == connection.CredentialSource &&
		resolved.AuthType == connection.AuthType &&
		strings.EqualFold(resolved.AuthMethodID, connection.AuthMethodID) &&
		resolved.CredentialVersion == connection.CredentialVersion &&
		resolved.Revision == connection.Revision
}

func applyConnectionTestFailureSummary(connection *IntegrationConnection, code string, failureThreshold int) {
	if connection == nil {
		return
	}
	if failureThreshold < 1 {
		failureThreshold = 3
	}
	switch code {
	case ErrorCodeAuthInvalid:
		markPendingConnectionInvalid(connection)
		connection.HealthStatus = ConnectionHealthUnhealthy
		connection.AuthStatus = ConnectionAuthReconnectRequired
		connection.AttentionCode = stringPointer(ConnectionAttentionReconnectRequired)
		connection.ConsecutiveFailures++
	case ErrorCodeAccessDenied:
		markPendingConnectionInvalid(connection)
		connection.HealthStatus = ConnectionHealthDegraded
		connection.AttentionCode = stringPointer(ConnectionAttentionAdminCheckRequired)
	case ErrorCodeProviderRejected:
		markPendingConnectionInvalid(connection)
		connection.HealthStatus = ConnectionHealthUnhealthy
		connection.AttentionCode = stringPointer(ConnectionAttentionAdminCheckRequired)
		connection.ConsecutiveFailures++
	case ErrorCodeBudgetExceeded:
		connection.HealthStatus = ConnectionHealthDegraded
		connection.AttentionCode = stringPointer(ConnectionAttentionBillingRequired)
	case ErrorCodeRateLimited:
		connection.HealthStatus = ConnectionHealthDegraded
		connection.ConsecutiveFailures++
	case ErrorCodeTimeout, ErrorCodeUpstream, ErrorCodeResponseInvalid:
		connection.ConsecutiveFailures++
		if connection.ConsecutiveFailures >= failureThreshold {
			connection.HealthStatus = ConnectionHealthDegraded
		}
	}
}

func markPendingConnectionInvalid(connection *IntegrationConnection) {
	if connection != nil && connection.Status == ConnectionStatusPending {
		connection.Status = ConnectionStatusInvalid
	}
}

func (service *DefaultConnectionService) SetDefault(ctx context.Context, organizationID, connectionID uuid.UUID) (ConnectionView, error) {
	return service.SetDefaultAs(ctx, organizationID, connectionID, nil)
}

func (service *DefaultConnectionService) SetDefaultAs(ctx context.Context, organizationID, connectionID uuid.UUID, actorID *uuid.UUID) (ConnectionView, error) {
	connection, err := service.repository.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return ConnectionView{}, mapConnectionLookupError(err)
	}
	if err := rejectLegacyPlatformConnection(connection); err != nil {
		return ConnectionView{}, err
	}
	if connection.CredentialSource == ConnectionCredentialSourceAccount {
		return ConnectionView{}, NewError(ErrorCodeAccessDenied, "personal integration connections cannot be organization defaults", nil)
	}
	if actorAware, ok := service.repository.(interface {
		SetDefaultAs(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error
	}); ok {
		err = actorAware.SetDefaultAs(ctx, organizationID, connectionID, actorID)
	} else {
		err = service.repository.SetDefault(ctx, organizationID, connectionID)
	}
	if err != nil {
		return ConnectionView{}, mapConnectionLookupError(err)
	}
	connection, err = service.repository.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return ConnectionView{}, mapConnectionLookupError(err)
	}
	return service.connectionView(connection), nil
}

func (service *DefaultConnectionService) Delete(ctx context.Context, organizationID, connectionID uuid.UUID) error {
	return service.DeleteAs(ctx, organizationID, connectionID, nil)
}

func (service *DefaultConnectionService) DeleteAs(ctx context.Context, organizationID, connectionID uuid.UUID, actorID *uuid.UUID) error {
	connection, err := service.repository.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return mapConnectionLookupError(err)
	}
	if err := rejectLegacyPlatformConnection(connection); err != nil {
		return err
	}
	if err := authorizePersonalConnectionOwner(connection, actorID); err != nil {
		return err
	}
	var durableRevocationTask *OAuthRecoveryTask
	if connection.AuthType == ConnectionAuthTypeOAuth2 && connection.EncryptedCredentials != nil {
		if durableRepository, ok := service.repository.(OAuthRevocationDeleteRepository); ok {
			if service.oauthRecovery == nil {
				return NewError(
					ErrorCodeConnectionInvalid,
					"integration OAuth durable revocation is unavailable",
					nil,
				)
			}
			task, prepareErr := service.oauthRecovery.PrepareRevocation(ctx, connection)
			if prepareErr != nil {
				return NewError(
					ErrorCodeConnectionInvalid,
					"integration OAuth revocation could not be prepared",
					prepareErr,
				)
			}
			if deleteErr := durableRepository.DeleteWithOAuthRevocation(
				ctx,
				organizationID,
				connectionID,
				actorID,
				task,
			); deleteErr != nil {
				return mapConnectionLookupError(deleteErr)
			}
			durableRevocationTask = &task
		} else if actorAware, ok := service.repository.(interface {
			DeleteAs(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error
		}); ok {
			// Non-Gorm test/in-memory repositories retain the previous behavior.
			// Production repositories must implement OAuthRevocationDeleteRepository.
			err = actorAware.DeleteAs(ctx, organizationID, connectionID, actorID)
		} else {
			err = service.repository.Delete(ctx, organizationID, connectionID)
		}
	} else if actorAware, ok := service.repository.(interface {
		DeleteAs(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error
	}); ok {
		err = actorAware.DeleteAs(ctx, organizationID, connectionID, actorID)
	} else {
		err = service.repository.Delete(ctx, organizationID, connectionID)
	}
	if err != nil {
		return mapConnectionLookupError(err)
	}
	if durableRevocationTask != nil {
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if revokeErr := service.oauthRecovery.AttemptPreparedRevocation(revokeCtx, *durableRevocationTask); revokeErr != nil {
			logger.WarnContext(
				revokeCtx,
				"integration OAuth token could not be revoked after durable local deletion",
				"integration_id", connection.IntegrationID,
				"error_code", ErrorCode(revokeErr),
			)
		}
		return nil
	}
	// Local deletion is the privacy and authorization boundary. It must commit
	// before any fallible provider call so a bound connection or database
	// failure can never leave a visible row whose remote token was already
	// revoked. The repository redacts the encrypted envelope in the same
	// transaction. Remote revocation is therefore best effort and cannot make
	// the local deletion appear to have failed.
	if service.revoker != nil {
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if revokeErr := service.revoker.RevokeConnection(revokeCtx, connection); revokeErr != nil {
			if service.oauthRecovery != nil && connection.EncryptedCredentials != nil {
				queueCtx, queueCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
				queueErr := service.oauthRecovery.EnqueueRevocation(queueCtx, connection)
				queueCancel()
				if queueErr != nil {
					logger.WarnContext(
						revokeCtx,
						"integration OAuth token revocation recovery could not be queued",
						"integration_id", connection.IntegrationID,
						"error_code", ErrorCode(queueErr),
					)
				}
			}
			logger.WarnContext(
				revokeCtx,
				"integration OAuth token could not be revoked after local connection deletion",
				"integration_id", connection.IntegrationID,
				"error_code", ErrorCode(revokeErr),
			)
		}
	}
	return nil
}

func authorizePersonalConnectionOwner(connection *IntegrationConnection, actorID *uuid.UUID) error {
	if connection == nil || connection.CredentialSource != ConnectionCredentialSourceAccount {
		return nil
	}
	if connection.OwnerAccountID == nil || *connection.OwnerAccountID == uuid.Nil || actorID == nil || *actorID == uuid.Nil || *connection.OwnerAccountID != *actorID {
		return NewError(ErrorCodeAccessDenied, "personal integration connection is not available", nil)
	}
	return nil
}

func (service *DefaultConnectionService) validateTarget(integrationID, driverID string) (string, string, error) {
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	driverID = strings.ToLower(strings.TrimSpace(driverID))
	if !integrationIdentifierPattern.MatchString(integrationID) || !integrationIdentifierPattern.MatchString(driverID) {
		return "", "", invalidInput("integration id and driver id are invalid", nil)
	}
	if service.catalog == nil {
		return "", "", NewError(ErrorCodeDisabled, "integration catalog is unavailable", nil)
	}
	registeredDriver, configured := service.catalog.DriverForIntegration(integrationID)
	if !configured || !strings.EqualFold(registeredDriver, driverID) {
		return "", "", NewError(ErrorCodeDisabled, "integration driver is not enabled", nil)
	}
	return integrationID, driverID, nil
}

func (service *DefaultConnectionService) applyCredentialSource(ctx context.Context, connection *IntegrationConnection, credentials map[string]string) error {
	switch connection.CredentialSource {
	case ConnectionCredentialSourceOrganization, ConnectionCredentialSourceAccount:
		if !validStoredConnectionAuthType(connection.AuthType) {
			return invalidInput("stored connection authentication type is invalid", nil)
		}
		normalized, err := normalizeCredentials(credentials)
		if err != nil {
			return err
		}
		defer destroyCredentialMap(normalized)
		if service.credentialValidator != nil {
			if err := service.credentialValidator.ValidateProviderCredentials(ctx, CredentialValidationRequest{
				IntegrationID: connection.IntegrationID,
				DriverID:      connection.DriverID,
				AuthMethodID:  connection.AuthMethodID,
				Credentials:   normalized,
				Config:        cloneAnyMap(connection.Config),
			}); err != nil {
				return invalidInput("credentials do not match the provider authentication schema", err)
			}
		} else if connection.AuthType != ConnectionAuthTypeAPIKey {
			return NewError(ErrorCodeConnectionInvalid, "provider credential schema validation is unavailable", nil)
		}
		envelope, err := service.cipher.EncryptCredentials(normalized, CredentialAAD{
			OrganizationID:    connection.OrganizationID,
			ConnectionID:      connection.ID,
			IntegrationID:     connection.IntegrationID,
			CredentialVersion: connection.CredentialVersion,
		})
		if err != nil {
			return NewError(ErrorCodeConnectionInvalid, "integration credentials could not be encrypted", err)
		}
		connection.EncryptedCredentials = &envelope
	default:
		return invalidInput("credential source is invalid", nil)
	}
	return nil
}

func (service *DefaultConnectionService) resolveAuthMethod(connection *IntegrationConnection) error {
	if connection == nil {
		return invalidInput("integration connection is required", nil)
	}
	methodID := strings.ToLower(strings.TrimSpace(connection.AuthMethodID))
	if catalog, ok := service.catalog.(ConnectionAuthMethodCatalog); ok {
		method, found := catalog.ResolveConnectionAuthMethod(connection.IntegrationID, methodID, connection.AuthType)
		if !found {
			return invalidInput("integration authentication method is unsupported or ambiguous", nil)
		}
		expectedAuthType, mapped := connectionAuthTypeForMethod(method.Type)
		if !mapped || expectedAuthType != connection.AuthType {
			return invalidInput("integration authentication method does not match auth type", nil)
		}
		if method.CredentialSource != connection.CredentialSource {
			return invalidInput("integration authentication method does not match credential ownership", nil)
		}
		connection.AuthMethodID = method.ID
		return nil
	}
	if methodID == "" {
		methodID = string(connection.AuthType)
	}
	if !integrationIdentifierPattern.MatchString(methodID) {
		return invalidInput("integration authentication method is invalid", nil)
	}
	connection.AuthMethodID = methodID
	return nil
}

func connectionAuthTypeForMethod(methodType AuthMethodType) (ConnectionAuthType, bool) {
	switch methodType {
	case AuthMethodTypeAPIKey:
		return ConnectionAuthTypeAPIKey, true
	case AuthMethodTypeOAuth2:
		return ConnectionAuthTypeOAuth2, true
	case AuthMethodTypeCustomCredential:
		return ConnectionAuthTypeCustomCredential, true
	case AuthMethodTypeServiceAccount:
		return ConnectionAuthTypeServiceAccount, true
	default:
		return "", false
	}
}

func supportedConnectionListFilter(filter ConnectionListFilter) (ConnectionListFilter, bool) {
	if len(filter.CredentialSources) == 0 {
		filter.CredentialSources = []ConnectionCredentialSource{
			ConnectionCredentialSourceOrganization,
			ConnectionCredentialSourceAccount,
		}
		return filter, true
	}
	supported := make([]ConnectionCredentialSource, 0, len(filter.CredentialSources))
	seen := make(map[ConnectionCredentialSource]struct{}, len(filter.CredentialSources))
	for _, source := range filter.CredentialSources {
		if !supportedConnectionCredentialSource(source) {
			continue
		}
		if _, duplicated := seen[source]; duplicated {
			continue
		}
		seen[source] = struct{}{}
		supported = append(supported, source)
	}
	filter.CredentialSources = supported
	return filter, len(supported) > 0
}

func rejectLegacyPlatformConnection(connection *IntegrationConnection) error {
	if connection == nil || !supportedConnectionCredentialSource(connection.CredentialSource) {
		return NewError(ErrorCodeConnectionNotFound, "integration connection was not found", ErrConnectionNotFound)
	}
	return nil
}

func validStoredConnectionAuthType(authType ConnectionAuthType) bool {
	switch authType {
	case ConnectionAuthTypeAPIKey, ConnectionAuthTypeOAuth2, ConnectionAuthTypeCustomCredential, ConnectionAuthTypeServiceAccount:
		return true
	default:
		return false
	}
}

func normalizeCredentials(credentials map[string]string) (map[string]string, error) {
	if len(credentials) == 0 || len(credentials) > 16 {
		return nil, invalidInput("credentials are required", nil)
	}
	normalized := make(map[string]string, len(credentials))
	for rawKey, rawValue := range credentials {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		value := strings.TrimSpace(rawValue)
		if !integrationIdentifierPattern.MatchString(key) || len(key) > 64 || value == "" || len(value) > 16*1024 {
			destroyCredentialMap(normalized)
			return nil, invalidInput("credentials are invalid", nil)
		}
		if _, duplicated := normalized[key]; duplicated {
			destroyCredentialMap(normalized)
			return nil, invalidInput("credential names are duplicated", nil)
		}
		normalized[key] = value
	}
	return normalized, nil
}

func normalizeConnectionName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > 128 {
		return "", invalidInput("connection name must contain between 1 and 128 characters", nil)
	}
	return value, nil
}

func validateConnectionConfig(config map[string]any) error {
	if config == nil {
		return nil
	}
	encoded, err := json.Marshal(config)
	if err != nil || len(encoded) > 64*1024 {
		return invalidInput("connection config is invalid or too large", err)
	}
	if configContainsSensitiveData(config) || containsSensitiveValue(config) {
		return invalidInput("credentials and secrets must be supplied through the credentials field", nil)
	}
	return nil
}

func configContainsSensitiveData(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if IsSensitiveQueryKey(key) || configContainsSensitiveData(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if configContainsSensitiveData(nested) {
				return true
			}
		}
	case map[string]string:
		for key, nested := range typed {
			if IsSensitiveQueryKey(key) || containsSensitiveValue(nested) {
				return true
			}
		}
	}
	return false
}

func normalizeScopes(scopes []string) []string {
	seen := make(map[string]struct{}, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || len(scope) > 128 {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	return normalized
}

func mapConnectionLookupError(err error) error {
	if errors.Is(err, ErrConnectionNotFound) {
		return NewError(ErrorCodeConnectionNotFound, "integration connection was not found", err)
	}
	if errors.Is(err, ErrConnectionChanged) {
		return NewError(ErrorCodeConnectionConflict, "integration connection changed; reload it and retry", err)
	}
	if errors.Is(err, ErrConnectionNameConflict) {
		return NewError(ErrorCodeConnectionConflict, "an integration connection with this name already exists", err)
	}
	if errors.Is(err, ErrConnectionInUse) {
		return NewError(ErrorCodeConnectionInUse, "connection is still bound to an Agent", err)
	}
	return err
}

func optionalBoundedString(value string, limit int) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	runes := []rune(value)
	if len(runes) > limit {
		value = string(runes[:limit])
	}
	return &value
}

func cloneUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
