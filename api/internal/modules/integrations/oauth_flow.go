package integrations

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OAuthFlowIntent string

const (
	OAuthFlowIntentConnect      OAuthFlowIntent = "connect"
	OAuthFlowIntentReconnect    OAuthFlowIntent = "reconnect"
	OAuthFlowIntentScopeUpgrade OAuthFlowIntent = "scope_upgrade"
)

type OAuthFlowStatus string

const (
	OAuthFlowPending   OAuthFlowStatus = "pending"
	OAuthFlowSucceeded OAuthFlowStatus = "succeeded"
	OAuthFlowFailed    OAuthFlowStatus = "failed"
	OAuthFlowCancelled OAuthFlowStatus = "cancelled"
	OAuthFlowExpired   OAuthFlowStatus = "expired"
)

type IntegrationOAuthFlow struct {
	ID                    uuid.UUID                  `gorm:"type:uuid;primaryKey" json:"-"`
	FlowDigest            string                     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	BrowserBindingDigest  string                     `gorm:"size:64;not null" json:"-"`
	EncryptedFlowToken    string                     `gorm:"type:text;not null" json:"-"`
	OrganizationID        uuid.UUID                  `gorm:"type:uuid;not null;index" json:"-"`
	AccountID             uuid.UUID                  `gorm:"type:uuid;not null;index" json:"-"`
	ConnectionID          *uuid.UUID                 `gorm:"type:uuid" json:"-"`
	CompletedConnectionID *uuid.UUID                 `gorm:"type:uuid" json:"-"`
	IntegrationID         string                     `gorm:"size:64;not null" json:"-"`
	DriverID              string                     `gorm:"size:64;not null" json:"-"`
	AuthMethodID          string                     `gorm:"size:128;not null" json:"-"`
	CredentialSource      ConnectionCredentialSource `gorm:"size:32;not null" json:"-"`
	Intent                OAuthFlowIntent            `gorm:"size:32;not null" json:"-"`
	ConnectionName        string                     `gorm:"size:128;not null" json:"-"`
	RequestedActionIDs    []string                   `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"-"`
	RequestedScopes       []string                   `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"-"`
	ReturnPath            string                     `gorm:"size:2048;not null" json:"-"`
	Status                OAuthFlowStatus            `gorm:"size:32;not null;index" json:"-"`
	FailureCode           *string                    `gorm:"size:64" json:"-"`
	AccountDisplayName    *string                    `gorm:"size:255" json:"-"`
	ExpiresAt             time.Time                  `gorm:"not null;index" json:"-"`
	CompletedAt           *time.Time                 `json:"-"`
	CreatedAt             time.Time                  `json:"-"`
	UpdatedAt             time.Time                  `json:"-"`
}

func (IntegrationOAuthFlow) TableName() string { return "integration_oauth_flows" }

func (flow *IntegrationOAuthFlow) BeforeCreate(_ *gorm.DB) error {
	if flow.ID == uuid.Nil {
		flow.ID = uuid.New()
	}
	if flow.RequestedActionIDs == nil {
		flow.RequestedActionIDs = []string{}
	}
	if flow.RequestedScopes == nil {
		flow.RequestedScopes = []string{}
	}
	return nil
}

type OAuthFlowRepository interface {
	Create(context.Context, *IntegrationOAuthFlow) error
	CreatePending(context.Context, *IntegrationOAuthFlow, OAuthFlowAdmissionPolicy) error
	GetByID(context.Context, uuid.UUID) (*IntegrationOAuthFlow, error)
	GetForActor(context.Context, string, uuid.UUID, uuid.UUID) (*IntegrationOAuthFlow, error)
	Transition(context.Context, uuid.UUID, OAuthFlowStatus, OAuthFlowStatus, map[string]any) error
}

type OAuthFlowAdmissionPolicy struct {
	Now                time.Time
	MaxPending         int
	StartWindow        time.Duration
	MaxStartsPerWindow int
}

type OAuthClientFlowImpactRepository interface {
	CountPendingOAuthFlows(context.Context, uuid.UUID, string, []string) (int64, error)
}

type GormOAuthFlowRepository struct{ db *gorm.DB }

func NewGormOAuthFlowRepository(db *gorm.DB) *GormOAuthFlowRepository {
	return &GormOAuthFlowRepository{db: db}
}

func (repository *GormOAuthFlowRepository) Create(ctx context.Context, flow *IntegrationOAuthFlow) error {
	if repository == nil || repository.db == nil || flow == nil {
		return fmt.Errorf("integration OAuth flow repository is unavailable")
	}
	db, _ := oauthClientFlowDatabase(ctx, repository.db)
	if err := db.Create(flow).Error; err != nil {
		return fmt.Errorf("create integration OAuth flow: %w", err)
	}
	return nil
}

func (repository *GormOAuthFlowRepository) CreatePending(ctx context.Context, flow *IntegrationOAuthFlow, policy OAuthFlowAdmissionPolicy) error {
	if repository == nil || repository.db == nil || flow == nil {
		return fmt.Errorf("integration OAuth flow repository is unavailable")
	}
	now := policy.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if policy.MaxPending <= 0 {
		policy.MaxPending = 5
	}
	if policy.StartWindow <= 0 {
		policy.StartWindow = time.Minute
	}
	if policy.MaxStartsPerWindow <= 0 {
		policy.MaxStartsPerWindow = 10
	}
	db, alreadyInTransaction := oauthClientFlowDatabase(ctx, repository.db)
	createPending := func(tx *gorm.DB) error {
		// PostgreSQL advisory locking serializes admission for one actor/provider
		// without locking unrelated organizations. SQLite test databases already
		// serialize writers and do not support advisory locks.
		if tx.Dialector.Name() == "postgres" {
			lockKey := flow.OrganizationID.String() + ":" + flow.AccountID.String() + ":" + normalizeOAuthIdentifier(flow.IntegrationID)
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey).Error; err != nil {
				return fmt.Errorf("lock integration OAuth flow admission: %w", err)
			}
		}
		if err := tx.Model(&IntegrationOAuthFlow{}).
			Where("organization_id = ? AND account_id = ? AND integration_id = ? AND status = ? AND expires_at <= ?",
				flow.OrganizationID, flow.AccountID, normalizeOAuthIdentifier(flow.IntegrationID), OAuthFlowPending, now).
			Updates(map[string]any{
				"status": OAuthFlowExpired, "failure_code": ErrorCodeAuthInvalid,
				"completed_at": now, "encrypted_flow_token": "", "updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("expire stale integration OAuth flows before admission: %w", err)
		}
		var pending int64
		if err := tx.Model(&IntegrationOAuthFlow{}).
			Where("organization_id = ? AND account_id = ? AND integration_id = ? AND status = ? AND expires_at > ?",
				flow.OrganizationID, flow.AccountID, normalizeOAuthIdentifier(flow.IntegrationID), OAuthFlowPending, now).
			Count(&pending).Error; err != nil {
			return fmt.Errorf("count pending integration OAuth flows: %w", err)
		}
		if pending >= int64(policy.MaxPending) {
			return NewError(ErrorCodeRateLimited, "too many OAuth authorization flows are already pending", nil)
		}
		var recentStarts int64
		if err := tx.Model(&IntegrationOAuthFlow{}).
			Where("organization_id = ? AND account_id = ? AND integration_id = ? AND created_at >= ?",
				flow.OrganizationID, flow.AccountID, normalizeOAuthIdentifier(flow.IntegrationID), now.Add(-policy.StartWindow)).
			Count(&recentStarts).Error; err != nil {
			return fmt.Errorf("count recent integration OAuth flow starts: %w", err)
		}
		if recentStarts >= int64(policy.MaxStartsPerWindow) {
			return NewError(ErrorCodeRateLimited, "OAuth authorization was started too frequently", nil)
		}
		if err := tx.Create(flow).Error; err != nil {
			return fmt.Errorf("create integration OAuth flow: %w", err)
		}
		return nil
	}
	if alreadyInTransaction {
		return createPending(db)
	}
	return db.Transaction(createPending)
}

func (repository *GormOAuthFlowRepository) GetByID(ctx context.Context, flowID uuid.UUID) (*IntegrationOAuthFlow, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("integration OAuth flow repository is unavailable")
	}
	var flow IntegrationOAuthFlow
	err := repository.db.WithContext(ctx).Where("id = ?", flowID).First(&flow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration OAuth flow: %w", err)
	}
	return &flow, nil
}

func (repository *GormOAuthFlowRepository) GetForActor(ctx context.Context, flowDigest string, organizationID, accountID uuid.UUID) (*IntegrationOAuthFlow, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("integration OAuth flow repository is unavailable")
	}
	var flow IntegrationOAuthFlow
	err := repository.db.WithContext(ctx).
		Where("flow_digest = ? AND organization_id = ? AND account_id = ?", strings.TrimSpace(flowDigest), organizationID, accountID).
		First(&flow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration OAuth flow for actor: %w", err)
	}
	return &flow, nil
}

func (repository *GormOAuthFlowRepository) Transition(ctx context.Context, flowID uuid.UUID, from, to OAuthFlowStatus, updates map[string]any) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration OAuth flow repository is unavailable")
	}
	updates = cloneAnyMap(updates)
	updates["status"] = to
	updates["updated_at"] = gorm.Expr("CURRENT_TIMESTAMP")
	if to != OAuthFlowPending {
		updates["encrypted_flow_token"] = ""
	}
	result := repository.db.WithContext(ctx).Model(&IntegrationOAuthFlow{}).
		Where("id = ? AND status = ?", flowID, from).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("transition integration OAuth flow: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrConnectionChanged
	}
	return nil
}

func (repository *GormOAuthFlowRepository) CountPendingOAuthFlows(ctx context.Context, organizationID uuid.UUID, integrationID string, authMethodIDs []string) (int64, error) {
	if repository == nil || repository.db == nil {
		return 0, fmt.Errorf("integration OAuth flow repository is unavailable")
	}
	var count int64
	db, _ := oauthClientFlowDatabase(ctx, repository.db)
	query := db.Model(&IntegrationOAuthFlow{}).
		Where("organization_id = ? AND integration_id = ? AND status = ? AND expires_at > ?", organizationID, normalizeOAuthIdentifier(integrationID), OAuthFlowPending, time.Now().UTC())
	if len(authMethodIDs) > 0 {
		query = query.Where("auth_method_id IN ?", authMethodIDs)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count pending integration OAuth flows: %w", err)
	}
	return count, nil
}

type OAuthConnectionCommitter interface {
	CommitOAuthConnection(context.Context, uuid.UUID, *IntegrationConnection, bool, string, time.Time, *OAuthRecoveryTask) error
}

type GormOAuthConnectionCommitter struct{ db *gorm.DB }

func NewGormOAuthConnectionCommitter(db *gorm.DB) *GormOAuthConnectionCommitter {
	return &GormOAuthConnectionCommitter{db: db}
}

func (committer *GormOAuthConnectionCommitter) CommitOAuthConnection(
	ctx context.Context,
	flowID uuid.UUID,
	connection *IntegrationConnection,
	create bool,
	displayName string,
	completedAt time.Time,
	supersededRevocation *OAuthRecoveryTask,
) error {
	if committer == nil || committer.db == nil || connection == nil || flowID == uuid.Nil {
		return fmt.Errorf("integration OAuth connection committer is unavailable")
	}
	if supersededRevocation != nil {
		normalized := normalizeOAuthRecoveryTask(*supersededRevocation, completedAt.UTC())
		if err := validateOAuthRecoveryTask(normalized); err != nil {
			return err
		}
		if create ||
			normalized.Kind != OAuthRecoveryRevoke ||
			normalized.OrganizationID != connection.OrganizationID ||
			normalized.ConnectionID != connection.ID ||
			normalized.CredentialVersion != connection.CredentialVersion-1 {
			return fmt.Errorf("integration OAuth superseded credential revocation is invalid")
		}
		supersededRevocation = &normalized
	}
	commit := func(tx *gorm.DB) error {
		var flow IntegrationOAuthFlow
		if err := lockOAuthFlow(tx).Where("id = ? AND status = ?", flowID, OAuthFlowPending).First(&flow).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConnectionChanged
			}
			return fmt.Errorf("lock integration OAuth flow: %w", err)
		}
		connectionRepository := NewGormConnectionRepository(tx)
		var err error
		if create {
			err = connectionRepository.Create(ctx, connection)
		} else {
			err = connectionRepository.Update(ctx, connection)
		}
		if err != nil {
			return err
		}
		if supersededRevocation != nil {
			record, recordErr := oauthRecoveryRecord(*supersededRevocation, completedAt.UTC())
			if recordErr != nil {
				return recordErr
			}
			if createErr := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error; createErr != nil {
				return fmt.Errorf("record superseded integration OAuth credential revocation: %w", createErr)
			}
		}
		result := tx.Model(&IntegrationOAuthFlow{}).Where("id = ? AND status = ?", flowID, OAuthFlowPending).
			Updates(map[string]any{
				"status": OAuthFlowSucceeded, "completed_connection_id": connection.ID,
				"account_display_name": displayName, "completed_at": completedAt.UTC(),
				"failure_code": nil, "encrypted_flow_token": "", "updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			})
		if result.Error != nil {
			return fmt.Errorf("complete integration OAuth flow: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrConnectionChanged
		}
		return nil
	}
	db, alreadyInTransaction := oauthClientFlowDatabase(ctx, committer.db)
	if alreadyInTransaction {
		return commit(db)
	}
	return db.Transaction(commit)
}

type OAuthCallbackAuthorizer interface {
	AuthorizeOAuthCallback(context.Context, OAuthCallbackAuthorizationRequest) error
}

type OAuthCallbackAuthorizationRequest struct {
	OrganizationID   uuid.UUID
	AccountID        uuid.UUID
	IntegrationID    string
	AuthMethodID     string
	CredentialSource ConnectionCredentialSource
	Intent           OAuthFlowIntent
	ConnectionID     *uuid.UUID
}

type OAuthCallbackAuthorizerFunc func(context.Context, OAuthCallbackAuthorizationRequest) error

func (authorize OAuthCallbackAuthorizerFunc) AuthorizeOAuthCallback(ctx context.Context, request OAuthCallbackAuthorizationRequest) error {
	return authorize(ctx, request)
}

type OAuthFlowStartRequest struct {
	OrganizationID       uuid.UUID
	AccountID            uuid.UUID
	BrowserBindingDigest string
	IntegrationID        string
	AuthMethodID         string
	CredentialSource     ConnectionCredentialSource
	Intent               OAuthFlowIntent
	ConnectionName       string
	ConnectionID         *uuid.UUID
	RequestedActionIDs   []string
	RedirectURI          string
	ReturnPath           string
}

type OAuthFlowStartResult struct {
	FlowID           string          `json:"flow_id"`
	AuthorizationURL string          `json:"authorization_url"`
	Status           OAuthFlowStatus `json:"status"`
	ExpiresAt        time.Time       `json:"expires_at"`
	NextPollAfterMS  int             `json:"next_poll_after_ms"`
}

type OAuthFlowView struct {
	FlowID             string                     `json:"flow_id"`
	IntegrationID      string                     `json:"integration_id"`
	AuthMethodID       string                     `json:"auth_method_id"`
	Intent             OAuthFlowIntent            `json:"intent"`
	Status             OAuthFlowStatus            `json:"status"`
	CredentialSource   ConnectionCredentialSource `json:"credential_source"`
	UsageRulesRequired bool                       `json:"usage_rules_required"`
	AIChatAvailable    bool                       `json:"ai_chat_available"`
	ConnectionName     string                     `json:"connection_name,omitempty"`
	AccountDisplayName string                     `json:"account_display_name,omitempty"`
	ErrorCode          string                     `json:"error_code,omitempty"`
	ExpiresAt          time.Time                  `json:"expires_at"`
	CompletedAt        *time.Time                 `json:"completed_at,omitempty"`
}

type OAuthCallbackRequest struct {
	State                    string
	BrowserBindingDigest     string
	Code                     string
	ProviderError            string
	ProviderErrorDescription string
}

type OAuthCallbackResult struct {
	FlowID     string
	ReturnPath string
	Status     OAuthFlowStatus
	ErrorCode  string
}

type OAuthFlowService struct {
	flows              OAuthFlowRepository
	states             *OAuthStateService
	registry           *Registry
	clients            OAuthClientResolver
	connections        ConnectionRepository
	cipher             CredentialCipher
	committer          OAuthConnectionCommitter
	callbackAuthorizer OAuthCallbackAuthorizer
	maintenance        OAuthArtifactMaintenanceRepository
	recovery           *OAuthRecoveryService
	flowTTL            time.Duration
	refreshWindow      time.Duration
	maxPendingFlows    int
	startWindow        time.Duration
	maxStartsPerWindow int
	clientFlowLocker   OAuthClientFlowLocker
}

func NewOAuthFlowService(flows OAuthFlowRepository, states *OAuthStateService, registry *Registry, clients OAuthClientResolver, connections ConnectionRepository, cipher CredentialCipher) *OAuthFlowService {
	service := &OAuthFlowService{
		flows: flows, states: states, registry: registry, clients: clients, connections: connections, cipher: cipher,
		flowTTL: 10 * time.Minute, refreshWindow: 5 * time.Minute,
		maxPendingFlows: 5, startWindow: time.Minute, maxStartsPerWindow: 10,
	}
	if repository, ok := flows.(*GormOAuthFlowRepository); ok && repository != nil {
		service.committer = NewGormOAuthConnectionCommitter(repository.db)
		service.maintenance = repository
		service.clientFlowLocker = newGormOAuthClientFlowLocker(repository.db)
	}
	return service
}

func (service *OAuthFlowService) WithOAuthClientFlowLocker(locker OAuthClientFlowLocker) *OAuthFlowService {
	if service != nil {
		service.clientFlowLocker = locker
	}
	return service
}

func (service *OAuthFlowService) WithMaintenanceRepository(repository OAuthArtifactMaintenanceRepository) *OAuthFlowService {
	if service != nil {
		service.maintenance = repository
	}
	return service
}

func (service *OAuthFlowService) WithConnectionCommitter(committer OAuthConnectionCommitter) *OAuthFlowService {
	if service != nil {
		service.committer = committer
	}
	return service
}

func (service *OAuthFlowService) WithCallbackAuthorizer(authorizer OAuthCallbackAuthorizer) *OAuthFlowService {
	if service != nil {
		service.callbackAuthorizer = authorizer
	}
	return service
}

func (service *OAuthFlowService) WithOAuthRecovery(recovery *OAuthRecoveryService) *OAuthFlowService {
	if service != nil {
		service.recovery = recovery
	}
	return service
}

func (service *OAuthFlowService) WithFlowTTL(ttl time.Duration) *OAuthFlowService {
	if service != nil && ttl >= time.Minute && ttl <= 30*time.Minute {
		service.flowTTL = ttl
	}
	return service
}

func (service *OAuthFlowService) WithStartPolicy(maxPending int, window time.Duration, maxStarts int) *OAuthFlowService {
	if service == nil {
		return service
	}
	if maxPending > 0 && maxPending <= 20 {
		service.maxPendingFlows = maxPending
	}
	if window >= time.Second && window <= time.Hour {
		service.startWindow = window
	}
	if maxStarts > 0 && maxStarts <= 100 {
		service.maxStartsPerWindow = maxStarts
	}
	return service
}

func (service *OAuthFlowService) Start(ctx context.Context, request OAuthFlowStartRequest) (OAuthFlowStartResult, error) {
	if service == nil || service.flows == nil || service.states == nil || service.registry == nil || service.clients == nil || service.connections == nil || service.cipher == nil {
		return OAuthFlowStartResult{}, NewError(ErrorCodeDisabled, "integration OAuth flow service is unavailable", nil)
	}
	request.IntegrationID = normalizeOAuthIdentifier(request.IntegrationID)
	request.AuthMethodID = normalizeOAuthIdentifier(request.AuthMethodID)
	request.ReturnPath = normalizeOAuthReturnPath(request.ReturnPath)
	if request.Intent == "" {
		request.Intent = OAuthFlowIntentConnect
	}
	if request.OrganizationID == uuid.Nil || request.AccountID == uuid.Nil ||
		!validOAuthBrowserBindingDigest(request.BrowserBindingDigest) || !validOAuthFlowIntent(request.Intent) {
		return OAuthFlowStartResult{}, invalidInput("OAuth flow identity or intent is invalid", nil)
	}
	definition, method, provider, err := service.resolveOAuthMethod(request.IntegrationID, request.AuthMethodID)
	if err != nil {
		return OAuthFlowStartResult{}, err
	}
	if (request.Intent == OAuthFlowIntentConnect && !method.OAuth.ConnectEnabled) ||
		(request.Intent == OAuthFlowIntentReconnect && !method.OAuth.ReconnectEnabled) ||
		(request.Intent == OAuthFlowIntentScopeUpgrade && !method.OAuth.ScopeUpgradeEnabled) {
		return OAuthFlowStartResult{}, NewError(ErrorCodeDisabled, "integration OAuth intent is unavailable for this auth method", nil)
	}
	if request.CredentialSource == "" {
		request.CredentialSource = method.CredentialSource
	}
	if request.CredentialSource != method.CredentialSource {
		return OAuthFlowStartResult{}, invalidInput("OAuth credential source does not match the auth method", nil)
	}
	requestedActions, requestedScopes, err := deriveOAuthScopes(definition, *method, request.RequestedActionIDs)
	if err != nil {
		return OAuthFlowStartResult{}, err
	}
	connectionName, targetConnection, err := service.validateFlowConnection(ctx, request, definition)
	if err != nil {
		return OAuthFlowStartResult{}, err
	}
	if targetConnection != nil {
		existing, lookupErr := service.connections.GetByID(ctx, request.OrganizationID, *targetConnection)
		if lookupErr != nil {
			return OAuthFlowStartResult{}, mapConnectionLookupError(lookupErr)
		}
		requestedScopes = normalizeScopes(append(append([]string(nil), existing.GrantedScopes...), requestedScopes...))
	}
	clientRequest := OAuthClientResolveRequest{
		OrganizationID: request.OrganizationID, IntegrationID: definition.ID,
		DriverID: definition.DriverID, AuthMethodID: method.ID,
	}
	clientConfigID := method.ID
	if method.OAuth != nil && method.OAuth.ClientConfigID != "" {
		clientConfigID = method.OAuth.ClientConfigID
	}
	var client OAuthClient
	var rawFlowID string
	var flow *IntegrationOAuthFlow
	err = withOAuthClientFlowLock(
		ctx,
		service.clientFlowLocker,
		request.OrganizationID,
		definition.ID,
		clientConfigID,
		func(lockedContext context.Context) error {
			var resolveErr error
			client, resolveErr = service.clients.ResolveOAuthClient(lockedContext, clientRequest)
			if resolveErr != nil {
				return resolveErr
			}
			rawFlowID, resolveErr = randomOAuthValue(24)
			if resolveErr != nil {
				client.Destroy()
				return fmt.Errorf("generate integration OAuth flow id: %w", resolveErr)
			}
			flow = &IntegrationOAuthFlow{
				ID: uuid.New(), FlowDigest: oauthStateDigest(rawFlowID), BrowserBindingDigest: request.BrowserBindingDigest,
				OrganizationID: request.OrganizationID, AccountID: request.AccountID,
				ConnectionID: cloneUUIDPointer(targetConnection), IntegrationID: definition.ID, DriverID: definition.DriverID,
				AuthMethodID: method.ID, CredentialSource: method.CredentialSource, Intent: request.Intent,
				ConnectionName: connectionName, RequestedActionIDs: requestedActions, RequestedScopes: requestedScopes,
				ReturnPath: request.ReturnPath, Status: OAuthFlowPending, ExpiresAt: time.Now().UTC().Add(service.flowTTL),
			}
			flow.EncryptedFlowToken, resolveErr = service.cipher.EncryptCredentials(map[string]string{"flow_token": rawFlowID}, CredentialAAD{
				OrganizationID: flow.OrganizationID, ConnectionID: flow.ID,
				IntegrationID: "oauth-flow-" + flow.IntegrationID, CredentialVersion: 1,
			})
			if resolveErr != nil {
				client.Destroy()
				return NewError(ErrorCodeConnectionInvalid, "integration OAuth flow could not be protected", resolveErr)
			}
			if resolveErr = service.flows.CreatePending(lockedContext, flow, OAuthFlowAdmissionPolicy{
				Now: time.Now().UTC(), MaxPending: service.maxPendingFlows,
				StartWindow: service.startWindow, MaxStartsPerWindow: service.maxStartsPerWindow,
			}); resolveErr != nil {
				client.Destroy()
				return resolveErr
			}
			return nil
		},
	)
	if err != nil {
		client.Destroy()
		return OAuthFlowStartResult{}, err
	}
	defer client.Destroy()
	state, err := service.states.Create(ctx, OAuthStateCreateRequest{
		OrganizationID: flow.OrganizationID, AccountID: flow.AccountID, FlowID: flow.ID,
		BrowserBindingDigest: flow.BrowserBindingDigest,
		ConnectionID:         cloneUUIDPointer(flow.ConnectionID), IntegrationID: flow.IntegrationID, DriverID: flow.DriverID,
		AuthMethodID: flow.AuthMethodID, RedirectURI: request.RedirectURI, RequestedScopes: requestedScopes,
	})
	if err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeAuthInvalid)
		return OAuthFlowStartResult{}, err
	}
	authorizationURL, err := provider.AuthorizationURL(OAuthAuthorizationRequest{
		Client: client, RedirectURI: request.RedirectURI, State: state.State,
		CodeChallenge: state.CodeChallenge, CodeChallengeMethod: OAuthPKCEChallengeMethodS256,
		Scopes: append([]string(nil), requestedScopes...), Config: cloneAnyMap(client.Config),
	})
	if err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeUpstream)
		return OAuthFlowStartResult{}, NewError(ErrorCodeUpstream, "integration OAuth authorization could not be started", err)
	}
	if err := validateProviderAuthorizationURL(authorizationURL); err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeResponseInvalid)
		return OAuthFlowStartResult{}, err
	}
	return OAuthFlowStartResult{
		FlowID: rawFlowID, AuthorizationURL: authorizationURL, Status: OAuthFlowPending,
		ExpiresAt: flow.ExpiresAt, NextPollAfterMS: 1000,
	}, nil
}

func (service *OAuthFlowService) Poll(ctx context.Context, rawFlowID string, organizationID, accountID uuid.UUID) (OAuthFlowView, error) {
	if service == nil || service.flows == nil || service.cipher == nil {
		return OAuthFlowView{}, NewError(ErrorCodeDisabled, "integration OAuth flow service is unavailable", nil)
	}
	if organizationID == uuid.Nil || accountID == uuid.Nil || !validRawOAuthFlowID(rawFlowID) {
		return OAuthFlowView{}, NewError(ErrorCodeConnectionNotFound, "integration OAuth flow was not found", nil)
	}
	flow, err := service.flows.GetForActor(ctx, oauthStateDigest(rawFlowID), organizationID, accountID)
	if err != nil {
		return OAuthFlowView{}, NewError(ErrorCodeConnectionNotFound, "integration OAuth flow was not found", err)
	}
	if flow.Status == OAuthFlowPending && !flow.ExpiresAt.After(time.Now().UTC()) {
		now := time.Now().UTC()
		_ = service.flows.Transition(ctx, flow.ID, OAuthFlowPending, OAuthFlowExpired, map[string]any{
			"failure_code": ErrorCodeAuthInvalid, "completed_at": now,
		})
		flow.Status = OAuthFlowExpired
		code := ErrorCodeAuthInvalid
		flow.FailureCode = &code
		flow.CompletedAt = &now
	}
	return flowPublicView(rawFlowID, flow), nil
}

func (service *OAuthFlowService) Cancel(ctx context.Context, rawFlowID string, organizationID, accountID uuid.UUID) error {
	if !validRawOAuthFlowID(rawFlowID) || organizationID == uuid.Nil || accountID == uuid.Nil {
		return NewError(ErrorCodeConnectionNotFound, "integration OAuth flow was not found", nil)
	}
	flow, err := service.flows.GetForActor(ctx, oauthStateDigest(rawFlowID), organizationID, accountID)
	if err != nil {
		return NewError(ErrorCodeConnectionNotFound, "integration OAuth flow was not found", err)
	}
	now := time.Now().UTC()
	if err := service.flows.Transition(ctx, flow.ID, OAuthFlowPending, OAuthFlowCancelled, map[string]any{"completed_at": now}); err != nil && !errors.Is(err, ErrConnectionChanged) {
		return err
	}
	return nil
}

func (service *OAuthFlowService) Callback(ctx context.Context, request OAuthCallbackRequest) (OAuthCallbackResult, error) {
	if service == nil || service.flows == nil || service.states == nil || service.registry == nil || service.clients == nil || service.connections == nil || service.cipher == nil {
		return OAuthCallbackResult{}, NewError(ErrorCodeDisabled, "integration OAuth flow service is unavailable", nil)
	}
	if service.committer == nil {
		return OAuthCallbackResult{}, NewError(ErrorCodeDisabled, "integration OAuth connection commit service is unavailable", nil)
	}
	if service.recovery == nil || !service.recovery.DurableRevocationReady() {
		return OAuthCallbackResult{}, NewError(
			ErrorCodeDisabled,
			"integration OAuth durable recovery is unavailable",
			nil,
		)
	}
	if !validOAuthBrowserBindingDigest(request.BrowserBindingDigest) {
		return OAuthCallbackResult{}, NewError(ErrorCodeAuthInvalid, "integration OAuth browser binding is invalid", nil)
	}
	consumed, err := service.states.Consume(ctx, request.State, request.BrowserBindingDigest)
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	flow, err := service.flows.GetByID(ctx, consumed.FlowID)
	if err != nil || flow.Status != OAuthFlowPending || !flow.ExpiresAt.After(time.Now().UTC()) {
		return OAuthCallbackResult{}, NewError(ErrorCodeAuthInvalid, "integration OAuth flow is expired or already completed", err)
	}
	if !oauthStateMatchesFlow(consumed, flow) {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeAuthInvalid)
		return OAuthCallbackResult{}, NewError(ErrorCodeAuthInvalid, "integration OAuth flow identity is invalid", nil)
	}
	result := OAuthCallbackResult{ReturnPath: flow.ReturnPath, Status: OAuthFlowFailed}
	result.FlowID, err = service.decryptFlowToken(flow)
	if err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeAuthInvalid)
		return result, err
	}
	if service.callbackAuthorizer == nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeAccessDenied)
		return result, NewError(ErrorCodeAccessDenied, "integration OAuth callback authorization is unavailable", nil)
	}
	if err := service.callbackAuthorizer.AuthorizeOAuthCallback(ctx, OAuthCallbackAuthorizationRequest{
		OrganizationID: consumed.OrganizationID, AccountID: consumed.AccountID,
		IntegrationID: flow.IntegrationID, AuthMethodID: flow.AuthMethodID,
		CredentialSource: flow.CredentialSource, Intent: flow.Intent, ConnectionID: cloneUUIDPointer(flow.ConnectionID),
	}); err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeAccessDenied)
		result.ErrorCode = ErrorCodeAccessDenied
		return result, NewError(ErrorCodeAccessDenied, "integration OAuth callback is no longer authorized", err)
	}
	if strings.TrimSpace(request.ProviderError) != "" {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeAuthInvalid)
		result.ErrorCode = ErrorCodeAuthInvalid
		return result, NewError(ErrorCodeAuthInvalid, "integration OAuth authorization was declined", nil)
	}
	code := strings.TrimSpace(request.Code)
	if code == "" || len(code) > 8192 {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeAuthInvalid)
		result.ErrorCode = ErrorCodeAuthInvalid
		return result, NewError(ErrorCodeAuthInvalid, "integration OAuth authorization code is invalid", nil)
	}
	definition, method, provider, err := service.resolveOAuthMethod(consumed.IntegrationID, consumed.AuthMethodID)
	if err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeDisabled)
		result.ErrorCode = ErrorCodeDisabled
		return result, err
	}
	client, err := service.clients.ResolveOAuthClient(ctx, OAuthClientResolveRequest{
		OrganizationID: consumed.OrganizationID, IntegrationID: consumed.IntegrationID,
		DriverID: consumed.DriverID, AuthMethodID: consumed.AuthMethodID,
	})
	if err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeDisabled)
		result.ErrorCode = ErrorCodeDisabled
		return result, err
	}
	defer client.Destroy()
	tokens, err := provider.ExchangeCode(ctx, OAuthCodeExchangeRequest{
		Client: client, Code: code, RedirectURI: consumed.RedirectURI,
		CodeVerifier: consumed.CodeVerifier, Scopes: append([]string(nil), consumed.RequestedScopes...),
		Config: cloneAnyMap(client.Config),
	})
	if err != nil {
		errorCode := oauthPublicErrorCode(err)
		_ = service.failFlow(ctx, flow.ID, errorCode)
		result.ErrorCode = errorCode
		return result, NewError(errorCode, "integration OAuth token exchange failed", err)
	}
	defer tokens.Destroy()
	committed := false
	durableCompensationRecorded := false
	var compensationTask *OAuthRecoveryTask
	defer func() {
		// Once the encrypted compensation task has been durably recorded, only
		// the recovery worker may revoke it. The worker re-reads the flow and
		// acknowledges a succeeded flow, which makes an ambiguous local commit
		// safe. Direct best-effort revocation is reserved for failures that
		// happened before durable recording and therefore before any commit
		// attempt.
		if !committed && !durableCompensationRecorded {
			service.revokeUncommittedOAuthTokens(provider, client, tokens)
		}
	}()
	if service.recovery != nil {
		task, prepareErr := service.recovery.PrepareUncommittedRevocation(flow, tokens, client)
		if prepareErr != nil {
			_ = service.failFlow(ctx, flow.ID, ErrorCodeConnectionInvalid)
			result.ErrorCode = ErrorCodeConnectionInvalid
			return result, NewError(
				ErrorCodeConnectionInvalid,
				"integration OAuth token cleanup could not be protected",
				prepareErr,
			)
		}
		if enqueueErr := service.recovery.EnqueuePreparedRevocation(ctx, task); enqueueErr != nil {
			_ = service.failFlow(ctx, flow.ID, ErrorCodeConnectionInvalid)
			result.ErrorCode = ErrorCodeConnectionInvalid
			return result, NewError(
				ErrorCodeConnectionInvalid,
				"integration OAuth token cleanup could not be recorded",
				enqueueErr,
			)
		}
		compensationTask = &task
		durableCompensationRecorded = true
	}
	now := time.Now().UTC()
	if strings.TrimSpace(tokens.AccessToken) == "" ||
		(tokens.ExpiresAt != nil && !tokens.ExpiresAt.After(now)) ||
		(tokens.RefreshTokenExpiresAt != nil && !tokens.RefreshTokenExpiresAt.After(now)) {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeResponseInvalid)
		result.ErrorCode = ErrorCodeResponseInvalid
		return result, NewError(ErrorCodeResponseInvalid, "integration OAuth token response is invalid", nil)
	}
	tokens.Scopes = normalizeScopes(tokens.Scopes)
	if missing := missingScopes(consumed.RequestedScopes, tokens.Scopes); len(missing) > 0 {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeInsufficientScope)
		result.ErrorCode = ErrorCodeInsufficientScope
		return result, NewError(ErrorCodeInsufficientScope, "integration OAuth authorization did not grant the required scopes", nil)
	}
	profile, err := provider.ResolveProfile(ctx, OAuthProfileRequest{
		AccessToken: tokens.AccessToken, TokenType: tokens.TokenType, Config: cloneAnyMap(client.Config),
	})
	if err != nil || strings.TrimSpace(profile.AccountID) == "" {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeResponseInvalid)
		result.ErrorCode = ErrorCodeResponseInvalid
		return result, NewError(ErrorCodeResponseInvalid, "integration OAuth account profile is unavailable", err)
	}
	connection, createConnection, supersededRevocation, err := service.prepareOAuthConnection(ctx, flow, tokens, profile)
	if err != nil {
		errorCode := oauthPublicErrorCode(err)
		_ = service.failFlow(ctx, flow.ID, errorCode)
		result.ErrorCode = errorCode
		return result, err
	}
	now = time.Now().UTC()
	displayName := safeOAuthDisplayName(profile)
	clientConfigID := method.ID
	if method.OAuth != nil && strings.TrimSpace(method.OAuth.ClientConfigID) != "" {
		clientConfigID = method.OAuth.ClientConfigID
	}
	commitErr := withOAuthClientFlowLock(
		ctx,
		service.clientFlowLocker,
		flow.OrganizationID,
		definition.ID,
		clientConfigID,
		func(lockedContext context.Context) error {
			return service.committer.CommitOAuthConnection(
				lockedContext,
				flow.ID,
				connection,
				createConnection,
				displayName,
				now,
				supersededRevocation,
			)
		},
	)
	if commitErr != nil {
		_ = service.failFlow(ctx, flow.ID, oauthPublicErrorCode(commitErr))
		return result, NewError(ErrorCodeConnectionConflict, "integration OAuth flow changed before completion", commitErr)
	}
	committed = true
	if compensationTask != nil {
		ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		ackErr := service.recovery.AcknowledgePreparedRevocation(ackCtx, compensationTask.ID)
		cancel()
		if ackErr != nil {
			// The durable task is guarded by the succeeded flow row, so a
			// recovery worker will acknowledge it without revoking live tokens.
			logger.WarnContext(
				ctx,
				"integration OAuth committed cleanup guard could not be acknowledged",
				"operation_ref", compensationTask.ID,
				"organization_id", flow.OrganizationID.String(),
				"integration_id", flow.IntegrationID,
			)
		}
	}
	result.Status = OAuthFlowSucceeded
	result.ErrorCode = ""
	return result, nil
}

func (service *OAuthFlowService) revokeUncommittedOAuthTokens(provider OAuth2Provider, client OAuthClient, tokens OAuthTokenSet) {
	capability, supported := provider.(OAuthRevocationCapability)
	if !supported || !capability.SupportsTokenRevocation() {
		return
	}
	token := strings.TrimSpace(tokens.RefreshToken)
	hint := "refresh_token"
	if token == "" {
		token = strings.TrimSpace(tokens.AccessToken)
		hint = "access_token"
	}
	if token == "" {
		return
	}
	revokeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Compensation is deliberately best effort: the original safe error is
	// returned to the browser and no token or provider response is logged.
	_ = provider.RevokeToken(revokeCtx, OAuthRevokeRequest{
		Client: client, Token: token, TokenTypeHint: hint, Config: cloneAnyMap(client.Config),
	})
}

func (service *OAuthFlowService) resolveOAuthMethod(integrationID, authMethodID string) (ProviderDefinition, *AuthMethodDefinition, OAuth2Provider, error) {
	definition, ok := service.registry.ProviderDefinition(integrationID)
	if !ok {
		return ProviderDefinition{}, nil, nil, NewError(ErrorCodeDisabled, "integration OAuth provider is unavailable", nil)
	}
	for index := range definition.AuthMethods {
		method := &definition.AuthMethods[index]
		if method.ID != authMethodID {
			continue
		}
		if method.Type != AuthMethodTypeOAuth2 || !method.Available || method.OAuth == nil {
			return ProviderDefinition{}, nil, nil, NewError(ErrorCodeDisabled, "integration OAuth method is unavailable", nil)
		}
		provider, exists := service.registry.OAuthProvider(definition.ID, definition.DriverID)
		if !exists {
			return ProviderDefinition{}, nil, nil, NewError(ErrorCodeDisabled, "integration OAuth provider is unavailable", nil)
		}
		return definition, method, provider, nil
	}
	return ProviderDefinition{}, nil, nil, invalidInput("integration OAuth auth method is unsupported", nil)
}

func (service *OAuthFlowService) validateFlowConnection(ctx context.Context, request OAuthFlowStartRequest, definition ProviderDefinition) (string, *uuid.UUID, error) {
	if request.Intent == OAuthFlowIntentConnect {
		if request.ConnectionID != nil {
			return "", nil, invalidInput("new OAuth connections cannot target an existing connection", nil)
		}
		name, err := normalizeConnectionName(request.ConnectionName)
		return name, nil, err
	}
	if request.ConnectionID == nil || *request.ConnectionID == uuid.Nil {
		return "", nil, invalidInput("OAuth reconnect and scope upgrade require an existing connection", nil)
	}
	connection, err := service.connections.GetByID(ctx, request.OrganizationID, *request.ConnectionID)
	if err != nil || connection.AuthType != ConnectionAuthTypeOAuth2 || !strings.EqualFold(connection.IntegrationID, definition.ID) ||
		!strings.EqualFold(connection.DriverID, definition.DriverID) || !strings.EqualFold(connection.AuthMethodID, request.AuthMethodID) {
		return "", nil, NewError(ErrorCodeConnectionNotFound, "integration OAuth connection was not found", err)
	}
	if connection.CredentialSource != request.CredentialSource {
		return "", nil, NewError(ErrorCodeAccessDenied, "integration OAuth connection scope does not match", nil)
	}
	if connection.CredentialSource == ConnectionCredentialSourceAccount &&
		(connection.OwnerAccountID == nil || *connection.OwnerAccountID != request.AccountID) {
		return "", nil, NewError(ErrorCodeAccessDenied, "integration OAuth connection is not owned by the current account", nil)
	}
	return connection.Name, cloneUUIDPointer(request.ConnectionID), nil
}

func (service *OAuthFlowService) prepareOAuthConnection(
	ctx context.Context,
	flow *IntegrationOAuthFlow,
	tokens OAuthTokenSet,
	profile OAuthProfile,
) (*IntegrationConnection, bool, *OAuthRecoveryTask, error) {
	credentials := tokens.credentialMap()
	if strings.TrimSpace(credentials["access_token"]) == "" {
		destroyCredentialMap(credentials)
		return nil, false, nil, NewError(ErrorCodeResponseInvalid, "integration OAuth token response is invalid", nil)
	}
	defer destroyCredentialMap(credentials)
	now := time.Now().UTC()
	if flow.ConnectionID == nil {
		connection := &IntegrationConnection{
			ID: uuid.New(), OrganizationID: flow.OrganizationID, IntegrationID: flow.IntegrationID, DriverID: flow.DriverID,
			Name: flow.ConnectionName, CredentialSource: flow.CredentialSource, AuthType: ConnectionAuthTypeOAuth2,
			AuthMethodID: flow.AuthMethodID, Config: map[string]any{}, AccountID: optionalBoundedString(profile.AccountID, 255),
			DisplayName: optionalBoundedString(safeOAuthDisplayName(profile), 255), GrantedScopes: normalizeScopes(tokens.Scopes),
			Status: ConnectionStatusActive, AuthStatus: ConnectionAuthValid, ScopeStatus: ConnectionScopeVerified,
			HealthStatus: ConnectionHealthHealthy, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
			LastHealthyAt: &now, LastTestedAt: &now, TokenExpiresAt: cloneTimePointer(tokens.ExpiresAt),
			RefreshTokenExpiresAt: cloneTimePointer(tokens.RefreshTokenExpiresAt),
			NextTokenRefreshAt:    oauthNextRefreshAt(tokens.ExpiresAt, service.refreshWindow),
			CreatedBy:             cloneUUIDPointer(&flow.AccountID), UpdatedBy: cloneUUIDPointer(&flow.AccountID),
		}
		if flow.CredentialSource == ConnectionCredentialSourceAccount {
			connection.OwnerAccountID = cloneUUIDPointer(&flow.AccountID)
		}
		envelope, err := service.cipher.EncryptCredentials(credentials, CredentialAAD{
			OrganizationID: connection.OrganizationID, ConnectionID: connection.ID,
			IntegrationID: connection.IntegrationID, CredentialVersion: connection.CredentialVersion,
		})
		if err != nil {
			return nil, false, nil, NewError(ErrorCodeConnectionInvalid, "integration OAuth credentials could not be protected", err)
		}
		connection.EncryptedCredentials = &envelope
		return connection, true, nil, nil
	}
	connection, err := service.connections.GetByID(ctx, flow.OrganizationID, *flow.ConnectionID)
	if err != nil {
		return nil, false, nil, mapConnectionLookupError(err)
	}
	if connection.AuthType != ConnectionAuthTypeOAuth2 ||
		!strings.EqualFold(connection.IntegrationID, flow.IntegrationID) ||
		!strings.EqualFold(connection.DriverID, flow.DriverID) ||
		!strings.EqualFold(connection.AuthMethodID, flow.AuthMethodID) ||
		connection.CredentialSource != flow.CredentialSource {
		return nil, false, nil, NewError(ErrorCodeConnectionConflict, "integration OAuth connection changed during authorization", nil)
	}
	if connection.CredentialSource == ConnectionCredentialSourceAccount &&
		(connection.OwnerAccountID == nil || *connection.OwnerAccountID != flow.AccountID) {
		return nil, false, nil, NewError(ErrorCodeAccessDenied, "integration OAuth connection is not owned by the current account", nil)
	}
	supersededRevocation, err := service.prepareSupersededOAuthRevocation(ctx, connection, credentials)
	if err != nil {
		return nil, false, nil, err
	}
	connection.CredentialVersion++
	envelope, err := service.cipher.EncryptCredentials(credentials, CredentialAAD{
		OrganizationID: connection.OrganizationID, ConnectionID: connection.ID,
		IntegrationID: connection.IntegrationID, CredentialVersion: connection.CredentialVersion,
	})
	if err != nil {
		return nil, false, nil, NewError(ErrorCodeConnectionInvalid, "integration OAuth credentials could not be protected", err)
	}
	connection.EncryptedCredentials = &envelope
	connection.AccountID = optionalBoundedString(profile.AccountID, 255)
	connection.DisplayName = optionalBoundedString(safeOAuthDisplayName(profile), 255)
	connection.GrantedScopes = normalizeScopes(tokens.Scopes)
	connection.Status = ConnectionStatusActive
	connection.AuthStatus = ConnectionAuthValid
	connection.ScopeStatus = ConnectionScopeVerified
	connection.HealthStatus = ConnectionHealthHealthy
	connection.AttentionCode = nil
	connection.MissingRequiredScopes = []string{}
	connection.LastErrorCode = nil
	connection.LastHealthyAt = &now
	connection.LastTestedAt = &now
	connection.TokenExpiresAt = cloneTimePointer(tokens.ExpiresAt)
	connection.RefreshTokenExpiresAt = cloneTimePointer(tokens.RefreshTokenExpiresAt)
	connection.NextTokenRefreshAt = oauthNextRefreshAt(tokens.ExpiresAt, service.refreshWindow)
	connection.UpdatedBy = cloneUUIDPointer(&flow.AccountID)
	return connection, false, supersededRevocation, nil
}

func (service *OAuthFlowService) prepareSupersededOAuthRevocation(
	ctx context.Context,
	connection *IntegrationConnection,
	replacement map[string]string,
) (*OAuthRecoveryTask, error) {
	if service == nil || service.cipher == nil || service.recovery == nil ||
		connection == nil || connection.EncryptedCredentials == nil {
		return nil, NewError(
			ErrorCodeConnectionInvalid,
			"integration OAuth credential replacement cleanup is unavailable",
			nil,
		)
	}
	current, err := service.cipher.DecryptCredentials(*connection.EncryptedCredentials, CredentialAAD{
		OrganizationID: connection.OrganizationID, ConnectionID: connection.ID,
		IntegrationID: connection.IntegrationID, CredentialVersion: connection.CredentialVersion,
	})
	if err != nil {
		return nil, NewError(
			ErrorCodeConnectionInvalid,
			"integration OAuth existing credentials could not be protected during replacement",
			err,
		)
	}
	defer destroyCredentialMap(current)
	currentToken := oauthRevocationToken(current)
	replacementToken := oauthRevocationToken(replacement)
	if currentToken == "" {
		return nil, nil
	}
	if replacementToken != "" &&
		len(currentToken) == len(replacementToken) &&
		subtle.ConstantTimeCompare([]byte(currentToken), []byte(replacementToken)) == 1 {
		return nil, nil
	}
	task, err := service.recovery.PrepareRevocation(ctx, connection)
	if err != nil {
		return nil, NewError(
			ErrorCodeConnectionInvalid,
			"integration OAuth existing credential revocation could not be prepared",
			err,
		)
	}
	return &task, nil
}

func oauthRevocationToken(credentials map[string]string) string {
	if token := strings.TrimSpace(credentials["refresh_token"]); token != "" {
		return token
	}
	return strings.TrimSpace(credentials["access_token"])
}

func (service *OAuthFlowService) decryptFlowToken(flow *IntegrationOAuthFlow) (string, error) {
	credentials, err := service.cipher.DecryptCredentials(flow.EncryptedFlowToken, CredentialAAD{
		OrganizationID: flow.OrganizationID, ConnectionID: flow.ID,
		IntegrationID: "oauth-flow-" + flow.IntegrationID, CredentialVersion: 1,
	})
	if err != nil {
		return "", NewError(ErrorCodeAuthInvalid, "integration OAuth flow is unavailable", err)
	}
	defer destroyCredentialMap(credentials)
	value := strings.TrimSpace(credentials["flow_token"])
	if !validRawOAuthFlowID(value) {
		return "", NewError(ErrorCodeAuthInvalid, "integration OAuth flow is unavailable", nil)
	}
	return value, nil
}

func (service *OAuthFlowService) failFlow(ctx context.Context, flowID uuid.UUID, code string) error {
	now := time.Now().UTC()
	return service.flows.Transition(ctx, flowID, OAuthFlowPending, OAuthFlowFailed, map[string]any{
		"failure_code": oauthSafeErrorCode(code), "completed_at": now,
	})
}

func deriveOAuthScopes(definition ProviderDefinition, method AuthMethodDefinition, requested []string) ([]string, []string, error) {
	actionIDs := normalizeCatalogStringList(requested, 64)
	if len(actionIDs) == 0 && method.OAuth != nil {
		actionIDs = append([]string(nil), method.OAuth.DefaultActionIDs...)
	}
	if len(actionIDs) == 0 {
		return nil, nil, invalidInput("at least one OAuth action must be selected", nil)
	}
	actionsByID := make(map[string]ActionDefinition, len(definition.Actions))
	for _, action := range definition.Actions {
		actionsByID[action.ID] = action
	}
	scopes := make([]string, 0, len(actionIDs)*2)
	if method.OAuth != nil {
		scopes = append(scopes, method.OAuth.IdentityScopes...)
	}
	for _, actionID := range actionIDs {
		action, exists := actionsByID[actionID]
		if !exists {
			return nil, nil, invalidInput("OAuth action selection contains an unknown action", nil)
		}
		if !ActionSupportsAuthMethod(action, method.ID) {
			return nil, nil, invalidInput("OAuth action selection is incompatible with the selected authentication method", nil)
		}
		scopes = append(scopes, ActionPreferredOAuthScopes(action)...)
	}
	scopes = normalizeScopes(scopes)
	if len(scopes) == 0 {
		return nil, nil, invalidInput("selected OAuth actions do not declare required scopes", nil)
	}
	return actionIDs, scopes, nil
}

func flowPublicView(rawFlowID string, flow *IntegrationOAuthFlow) OAuthFlowView {
	view := OAuthFlowView{
		FlowID: rawFlowID, IntegrationID: flow.IntegrationID, AuthMethodID: flow.AuthMethodID,
		Intent: flow.Intent, Status: flow.Status, CredentialSource: flow.CredentialSource, ConnectionName: flow.ConnectionName,
		ExpiresAt: flow.ExpiresAt, CompletedAt: cloneTimePointer(flow.CompletedAt),
	}
	if flow.Status == OAuthFlowSucceeded {
		view.UsageRulesRequired = flow.CredentialSource == ConnectionCredentialSourceOrganization
		view.AIChatAvailable = flow.CredentialSource == ConnectionCredentialSourceAccount
	}
	if flow.AccountDisplayName != nil {
		view.AccountDisplayName = *flow.AccountDisplayName
	}
	if flow.FailureCode != nil {
		view.ErrorCode = oauthSafeErrorCode(*flow.FailureCode)
	}
	return view
}

func missingScopes(required, granted []string) []string {
	grantedSet := make(map[string]struct{}, len(granted))
	for _, scope := range normalizeScopes(granted) {
		grantedSet[scope] = struct{}{}
	}
	missing := make([]string, 0)
	for _, scope := range normalizeScopes(required) {
		if _, exists := grantedSet[scope]; !exists {
			missing = append(missing, scope)
		}
	}
	return missing
}

func oauthStateMatchesFlow(state ConsumedOAuthState, flow *IntegrationOAuthFlow) bool {
	if flow == nil || state.FlowID != flow.ID || state.OrganizationID != flow.OrganizationID ||
		!secureOAuthDigestEqual(state.BrowserBindingDigest, flow.BrowserBindingDigest) ||
		state.AccountID != flow.AccountID || !strings.EqualFold(state.IntegrationID, flow.IntegrationID) ||
		!strings.EqualFold(state.DriverID, flow.DriverID) || !strings.EqualFold(state.AuthMethodID, flow.AuthMethodID) ||
		!oauthOptionalUUIDEqual(state.ConnectionID, flow.ConnectionID) {
		return false
	}
	return len(missingScopes(state.RequestedScopes, flow.RequestedScopes)) == 0 &&
		len(missingScopes(flow.RequestedScopes, state.RequestedScopes)) == 0
}

func secureOAuthDigestEqual(left, right string) bool {
	leftBytes, leftErr := hex.DecodeString(strings.TrimSpace(left))
	rightBytes, rightErr := hex.DecodeString(strings.TrimSpace(right))
	return leftErr == nil && rightErr == nil && len(leftBytes) == sha256.Size && len(rightBytes) == sha256.Size &&
		subtle.ConstantTimeCompare(leftBytes, rightBytes) == 1
}

func oauthOptionalUUIDEqual(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func safeOAuthDisplayName(profile OAuthProfile) string {
	if displayName := strings.TrimSpace(profile.DisplayName); displayName != "" {
		return string([]rune(displayName)[:min(len([]rune(displayName)), 255)])
	}
	if email := strings.TrimSpace(profile.Email); email != "" {
		return string([]rune(email)[:min(len([]rune(email)), 255)])
	}
	return "Connected account"
}

func oauthNextRefreshAt(expiresAt *time.Time, refreshWindow time.Duration) *time.Time {
	if expiresAt == nil {
		return nil
	}
	if refreshWindow <= 0 {
		refreshWindow = 5 * time.Minute
	}
	value := expiresAt.UTC().Add(-refreshWindow)
	return &value
}

func normalizeOAuthReturnPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 2048 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") ||
		strings.ContainsAny(value, "\r\n\\") {
		return "/console/integrations"
	}
	return value
}

func validOAuthFlowIntent(intent OAuthFlowIntent) bool {
	switch intent {
	case OAuthFlowIntentConnect, OAuthFlowIntentReconnect, OAuthFlowIntentScopeUpgrade:
		return true
	default:
		return false
	}
}

func validRawOAuthFlowID(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 32 && len(value) <= 128
}

func validateProviderAuthorizationURL(value string) error {
	if !validOAuthRedirectURI(value) || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "https://") {
		return NewError(ErrorCodeResponseInvalid, "integration OAuth provider returned an invalid authorization URL", nil)
	}
	return nil
}

func oauthPublicErrorCode(err error) string {
	code := ErrorCode(err)
	return oauthSafeErrorCode(code)
}

func oauthSafeErrorCode(code string) string {
	switch code {
	case ErrorCodeDisabled, ErrorCodeInvalidInput, ErrorCodeAuthInvalid, ErrorCodeAccessDenied,
		ErrorCodeRateLimited, ErrorCodeTimeout, ErrorCodeUpstream, ErrorCodeResponseInvalid,
		ErrorCodeReconnectRequired, ErrorCodeConnectionExpired, ErrorCodeInsufficientScope,
		ErrorCodeConnectionConflict, ErrorCodeConnectionInvalid:
		return code
	default:
		return ErrorCodeUpstream
	}
}

// Ensure PostgreSQL callback transitions cannot race each other when a route
// implementation needs to perform additional state-dependent work.
func lockOAuthFlow(query *gorm.DB) *gorm.DB {
	return query.Clauses(clause.Locking{Strength: "UPDATE"})
}
