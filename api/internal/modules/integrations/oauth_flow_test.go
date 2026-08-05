package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type memoryOAuthClientConfigRepository struct {
	config *IntegrationOAuthClientConfig
}

type serialOAuthClientFlowTestLocker struct {
	mu       sync.Mutex
	attempts chan struct{}
}

func (locker *serialOAuthClientFlowTestLocker) WithinOAuthClientFlowLock(
	ctx context.Context,
	_ uuid.UUID,
	_ string,
	_ string,
	operation func(context.Context) error,
) error {
	if locker.attempts != nil {
		locker.attempts <- struct{}{}
	}
	locker.mu.Lock()
	defer locker.mu.Unlock()
	return operation(ctx)
}

type blockingOAuthClientResolver struct {
	delegate OAuthClientResolver
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (resolver *blockingOAuthClientResolver) ResolveOAuthClient(
	ctx context.Context,
	request OAuthClientResolveRequest,
) (OAuthClient, error) {
	client, err := resolver.delegate.ResolveOAuthClient(ctx, request)
	if err != nil {
		return OAuthClient{}, err
	}
	resolver.once.Do(func() {
		close(resolver.entered)
		<-resolver.release
	})
	return client, nil
}

func (resolver *blockingOAuthClientResolver) OAuthClientConfigured(
	ctx context.Context,
	request OAuthClientResolveRequest,
) bool {
	return resolver.delegate.OAuthClientConfigured(ctx, request)
}

func (repository *memoryOAuthClientConfigRepository) Get(_ context.Context, organizationID uuid.UUID, integrationID, authMethodID string) (*IntegrationOAuthClientConfig, error) {
	if repository.config == nil || repository.config.OrganizationID != organizationID ||
		repository.config.IntegrationID != integrationID || repository.config.AuthMethodID != authMethodID {
		return nil, ErrConnectionNotFound
	}
	copyValue := *repository.config
	copyValue.Config = cloneAnyMap(repository.config.Config)
	return &copyValue, nil
}
func (repository *memoryOAuthClientConfigRepository) Create(_ context.Context, config *IntegrationOAuthClientConfig) error {
	copyValue := *config
	copyValue.Config = cloneAnyMap(config.Config)
	copyValue.CreatedAt = time.Now().UTC()
	copyValue.UpdatedAt = copyValue.CreatedAt
	repository.config = &copyValue
	return nil
}
func (repository *memoryOAuthClientConfigRepository) Update(_ context.Context, config *IntegrationOAuthClientConfig, expectedRevision int) error {
	if repository.config == nil || repository.config.Revision != expectedRevision {
		return ErrConnectionChanged
	}
	copyValue := *config
	copyValue.Revision = expectedRevision + 1
	copyValue.Config = cloneAnyMap(config.Config)
	copyValue.UpdatedAt = time.Now().UTC()
	repository.config = &copyValue
	config.Revision = copyValue.Revision
	return nil
}
func (repository *memoryOAuthClientConfigRepository) Delete(_ context.Context, organizationID uuid.UUID, integrationID, authMethodID string) error {
	if repository.config == nil || repository.config.OrganizationID != organizationID ||
		repository.config.IntegrationID != integrationID || repository.config.AuthMethodID != authMethodID {
		return ErrConnectionNotFound
	}
	repository.config = nil
	return nil
}

type memoryOAuthFlowRepository struct {
	mu    sync.Mutex
	flows map[uuid.UUID]*IntegrationOAuthFlow
}

type memoryOAuthConnectionCommitter struct {
	flows       *memoryOAuthFlowRepository
	connections *memoryConnectionRepository
	revocations []OAuthRecoveryTask
}

type blockingOAuthConnectionCommitter struct {
	delegate OAuthConnectionCommitter
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (committer *blockingOAuthConnectionCommitter) CommitOAuthConnection(
	ctx context.Context,
	flowID uuid.UUID,
	connection *IntegrationConnection,
	create bool,
	displayName string,
	completedAt time.Time,
	supersededRevocation *OAuthRecoveryTask,
) error {
	committer.once.Do(func() { close(committer.entered) })
	select {
	case <-committer.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return committer.delegate.CommitOAuthConnection(ctx, flowID, connection, create, displayName, completedAt, supersededRevocation)
}

type ambiguousOAuthConnectionCommitter struct {
	delegate OAuthConnectionCommitter
}

func (committer *ambiguousOAuthConnectionCommitter) CommitOAuthConnection(
	ctx context.Context,
	flowID uuid.UUID,
	connection *IntegrationConnection,
	create bool,
	displayName string,
	completedAt time.Time,
	supersededRevocation *OAuthRecoveryTask,
) error {
	if err := committer.delegate.CommitOAuthConnection(ctx, flowID, connection, create, displayName, completedAt, supersededRevocation); err != nil {
		return err
	}
	return errors.New("database driver returned an ambiguous commit result")
}

type retainingDeadLetterOAuthOutbox struct {
	*memoryDurableOAuthOutbox
	deadLetters map[string]OAuthRecoveryTask
	reasons     map[string]string
}

type failOnceAckOAuthOutbox struct {
	*memoryDurableOAuthOutbox
	failNext bool
}

type failEnqueueOAuthOutbox struct {
	OAuthRecoveryOutbox
}

func (outbox *failEnqueueOAuthOutbox) Enqueue(context.Context, OAuthRecoveryTask) error {
	return errors.New("durable database unavailable")
}

func (outbox *failOnceAckOAuthOutbox) Ack(ctx context.Context, id string) error {
	if outbox.failNext {
		outbox.failNext = false
		return errors.New("database acknowledgement unavailable")
	}
	return outbox.memoryDurableOAuthOutbox.Ack(ctx, id)
}

func (outbox *retainingDeadLetterOAuthOutbox) DeadLetter(_ context.Context, task OAuthRecoveryTask, reason string) error {
	outbox.store.mu.Lock()
	defer outbox.store.mu.Unlock()
	delete(outbox.store.tasks, task.ID)
	outbox.deadLetters[task.ID] = task
	outbox.reasons[task.ID] = reason
	return nil
}

func (committer *memoryOAuthConnectionCommitter) CommitOAuthConnection(
	ctx context.Context,
	flowID uuid.UUID,
	connection *IntegrationConnection,
	create bool,
	displayName string,
	completedAt time.Time,
	supersededRevocation *OAuthRecoveryTask,
) error {
	var err error
	if create {
		err = committer.connections.Create(ctx, connection)
	} else {
		err = committer.connections.Update(ctx, connection)
	}
	if err != nil {
		return err
	}
	if err := committer.flows.Transition(ctx, flowID, OAuthFlowPending, OAuthFlowSucceeded, map[string]any{
		"completed_connection_id": connection.ID, "account_display_name": displayName, "completed_at": completedAt, "failure_code": nil,
	}); err != nil {
		return err
	}
	if supersededRevocation != nil {
		committer.revocations = append(committer.revocations, *supersededRevocation)
	}
	return nil
}

func newMemoryOAuthFlowRepository() *memoryOAuthFlowRepository {
	return &memoryOAuthFlowRepository{flows: map[uuid.UUID]*IntegrationOAuthFlow{}}
}

func (repository *memoryOAuthFlowRepository) Create(_ context.Context, flow *IntegrationOAuthFlow) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	copyValue := *flow
	copyValue.RequestedActionIDs = append([]string(nil), flow.RequestedActionIDs...)
	copyValue.RequestedScopes = append([]string(nil), flow.RequestedScopes...)
	repository.flows[flow.ID] = &copyValue
	return nil
}

func (repository *memoryOAuthFlowRepository) CreatePending(_ context.Context, flow *IntegrationOAuthFlow, policy OAuthFlowAdmissionPolicy) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	now := policy.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var pending, recent int
	for _, existing := range repository.flows {
		if existing.OrganizationID != flow.OrganizationID || existing.AccountID != flow.AccountID ||
			existing.IntegrationID != flow.IntegrationID {
			continue
		}
		if existing.Status == OAuthFlowPending && !existing.ExpiresAt.After(now) {
			code := ErrorCodeAuthInvalid
			existing.Status = OAuthFlowExpired
			existing.FailureCode = &code
			existing.CompletedAt = cloneTimePointer(&now)
			existing.EncryptedFlowToken = ""
		}
		if existing.Status == OAuthFlowPending && existing.ExpiresAt.After(now) {
			pending++
		}
		if policy.StartWindow > 0 && !existing.CreatedAt.Before(now.Add(-policy.StartWindow)) {
			recent++
		}
	}
	if pending >= policy.MaxPending {
		return NewError(ErrorCodeRateLimited, "too many OAuth authorization flows are already pending", nil)
	}
	if recent >= policy.MaxStartsPerWindow {
		return NewError(ErrorCodeRateLimited, "OAuth authorization was started too frequently", nil)
	}
	copyValue := *flow
	copyValue.RequestedActionIDs = append([]string(nil), flow.RequestedActionIDs...)
	copyValue.RequestedScopes = append([]string(nil), flow.RequestedScopes...)
	copyValue.CreatedAt = now
	copyValue.UpdatedAt = now
	repository.flows[flow.ID] = &copyValue
	return nil
}

func (repository *memoryOAuthFlowRepository) GetByID(_ context.Context, flowID uuid.UUID) (*IntegrationOAuthFlow, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	flow := repository.flows[flowID]
	if flow == nil {
		return nil, ErrConnectionNotFound
	}
	copyValue := *flow
	return &copyValue, nil
}

func (repository *memoryOAuthFlowRepository) GetForActor(_ context.Context, digest string, organizationID, accountID uuid.UUID) (*IntegrationOAuthFlow, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, flow := range repository.flows {
		if flow.FlowDigest == digest && flow.OrganizationID == organizationID && flow.AccountID == accountID {
			copyValue := *flow
			return &copyValue, nil
		}
	}
	return nil, ErrConnectionNotFound
}

func (repository *memoryOAuthFlowRepository) Transition(_ context.Context, flowID uuid.UUID, from, to OAuthFlowStatus, updates map[string]any) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	flow := repository.flows[flowID]
	if flow == nil || flow.Status != from {
		return ErrConnectionChanged
	}
	flow.Status = to
	if to != OAuthFlowPending {
		flow.EncryptedFlowToken = ""
	}
	if value, ok := updates["failure_code"].(string); ok {
		flow.FailureCode = &value
	}
	if updates["failure_code"] == nil {
		flow.FailureCode = nil
	}
	if value, ok := updates["account_display_name"].(string); ok {
		flow.AccountDisplayName = &value
	}
	if value, ok := updates["completed_connection_id"].(uuid.UUID); ok {
		flow.CompletedConnectionID = &value
	}
	if value, ok := updates["completed_at"].(time.Time); ok {
		flow.CompletedAt = &value
	}
	return nil
}

func (repository *memoryOAuthFlowRepository) CountPendingOAuthFlows(_ context.Context, organizationID uuid.UUID, integrationID string, authMethodIDs []string) (int64, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	allowed := make(map[string]struct{}, len(authMethodIDs))
	for _, methodID := range authMethodIDs {
		allowed[methodID] = struct{}{}
	}
	var count int64
	for _, flow := range repository.flows {
		if flow.OrganizationID != organizationID || flow.IntegrationID != integrationID ||
			flow.Status != OAuthFlowPending || !flow.ExpiresAt.After(time.Now().UTC()) {
			continue
		}
		if len(allowed) > 0 {
			if _, exists := allowed[flow.AuthMethodID]; !exists {
				continue
			}
		}
		count++
	}
	return count, nil
}

type fakeOAuthAdapter struct {
	mu                     sync.Mutex
	authorization          OAuthAuthorizationRequest
	exchangeCalls          int
	refreshCalls           int
	refreshStarted         chan struct{}
	refreshContinue        chan struct{}
	revokeCalls            int
	revokedHint            string
	profileErr             error
	revokeErr              error
	noRevocation           bool
	omitRefreshTokenExpiry bool
}

func (*fakeOAuthAdapter) DriverID() string { return "fake-oauth" }
func (*fakeOAuthAdapter) Execute(context.Context, ActionRequest) (*ActionResult, error) {
	return &ActionResult{Output: map[string]any{}}, nil
}
func (adapter *fakeOAuthAdapter) AuthorizationURL(request OAuthAuthorizationRequest) (string, error) {
	adapter.mu.Lock()
	adapter.authorization = request
	adapter.mu.Unlock()
	query := url.Values{
		"state":          []string{request.State},
		"code_challenge": []string{request.CodeChallenge},
		"redirect_uri":   []string{request.RedirectURI},
	}
	return "https://provider.example/authorize?" + query.Encode(), nil
}
func (adapter *fakeOAuthAdapter) ExchangeCode(_ context.Context, request OAuthCodeExchangeRequest) (OAuthTokenSet, error) {
	adapter.mu.Lock()
	adapter.exchangeCalls++
	scopes := append([]string(nil), adapter.authorization.Scopes...)
	adapter.mu.Unlock()
	expiresAt := time.Now().UTC().Add(time.Hour)
	refreshExpiresAt := time.Now().UTC().Add(24 * time.Hour)
	return OAuthTokenSet{
		AccessToken: "access-token", RefreshToken: "refresh-token", TokenType: "Bearer",
		Scopes: scopes, ExpiresAt: &expiresAt, RefreshTokenExpiresAt: &refreshExpiresAt,
	}, nil
}
func (adapter *fakeOAuthAdapter) RefreshToken(_ context.Context, request OAuthRefreshRequest) (OAuthTokenSet, error) {
	adapter.mu.Lock()
	adapter.refreshCalls++
	started, proceed := adapter.refreshStarted, adapter.refreshContinue
	adapter.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if proceed != nil {
		<-proceed
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	var refreshExpiresAt *time.Time
	if !adapter.omitRefreshTokenExpiry {
		value := time.Now().UTC().Add(24 * time.Hour)
		refreshExpiresAt = &value
	}
	return OAuthTokenSet{
		AccessToken: "refreshed-access", RefreshToken: "rotated-refresh", TokenType: "Bearer",
		Scopes: append([]string(nil), request.Scopes...), ExpiresAt: &expiresAt,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, nil
}
func (adapter *fakeOAuthAdapter) RevokeToken(_ context.Context, request OAuthRevokeRequest) error {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.revokeCalls++
	adapter.revokedHint = request.TokenTypeHint
	return adapter.revokeErr
}
func (adapter *fakeOAuthAdapter) SupportsTokenRevocation() bool {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return !adapter.noRevocation
}
func (adapter *fakeOAuthAdapter) ResolveProfile(context.Context, OAuthProfileRequest) (OAuthProfile, error) {
	adapter.mu.Lock()
	err := adapter.profileErr
	adapter.mu.Unlock()
	if err != nil {
		return OAuthProfile{}, err
	}
	return OAuthProfile{AccountID: "provider-account", DisplayName: "Provider User", Email: "user@example.com"}, nil
}

func oauthTestRegistration(adapter *fakeOAuthAdapter) Registration {
	action := testAction("fake.account.read", "read_fake_account")
	action.RequiredScopes = []string{"account.read"}
	action.SupportedCallers = []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat}
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
	}
	definition := ProviderDefinition{
		ID: "fake", DriverID: "fake-oauth", Name: "Fake OAuth", Description: "OAuth test provider.",
		AuthMethods: []AuthMethodDefinition{{
			ID: "user_oauth", Type: AuthMethodTypeOAuth2, CredentialSource: ConnectionCredentialSourceAccount,
			Label: "Connect account", Available: true,
			OAuth: &OAuthMethodMetadata{
				ConnectEnabled: true, ReconnectEnabled: true, ScopeUpgradeEnabled: true,
				IdentityScopes:   []string{"identity.read"},
				DefaultActionIDs: []string{"fake.account.read"},
				ClientFields: []CredentialFieldDefinition{
					{Key: "client_id", Label: "Client ID", Input: CredentialFieldInputText, Required: true},
					{Key: "client_secret", Label: "Client secret", Input: CredentialFieldInputPassword, Secret: true},
				},
			},
		}},
		Actions: []ActionDefinition{action},
	}
	localizeTestProviderFixture(&definition)
	for index := range definition.AuthMethods[0].OAuth.ClientFields {
		field := &definition.AuthMethods[0].OAuth.ClientFields[index]
		field.LabelI18n = LocalizedText{LocaleEnglishUS: field.Label, LocaleSimplifiedChinese: "OAuth 客户端字段"}
	}
	return Registration{Definition: definition, Adapter: adapter, OAuth2Provider: adapter}
}

func TestOAuthFlowConnectUsesServerDerivedScopesAndPersistsEncryptedTokens(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	stateRepository := &memoryOAuthStateRepository{}
	stateService := NewOAuthStateService(stateRepository, cipher, 5*time.Minute).
		WithAllowedRedirectURIs([]string{"https://app.example.com/api/integrations/oauth/callback"})
	flowRepository := newMemoryOAuthFlowRepository()
	connectionRepository := newMemoryConnectionRepository()
	clientService := NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "client-id", ClientSecret: "client-secret",
	}})
	var authorizedRequest OAuthCallbackAuthorizationRequest
	service := NewOAuthFlowService(flowRepository, stateService, registry, clientService, connectionRepository, cipher).
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(_ context.Context, request OAuthCallbackAuthorizationRequest) error {
			authorizedRequest = request
			return nil
		}))
	withTestOAuthFlowRecovery(service)
	organizationID, accountID := uuid.New(), uuid.New()
	started, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "My account", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback", ReturnPath: "/console/integrations",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(started.AuthorizationURL, "client-secret") || started.FlowID == "" || len(started.FlowID) < 32 {
		t.Fatalf("unsafe OAuth start result = %#v", started)
	}
	if stateRepository.state == nil || len(stateRepository.state.RequestedScopes) != 2 ||
		stateRepository.state.RequestedScopes[0] != "identity.read" ||
		stateRepository.state.RequestedScopes[1] != "account.read" {
		t.Fatalf("server-derived scopes = %#v", stateRepository.state)
	}
	callback, err := service.Callback(context.Background(), OAuthCallbackRequest{
		State: adapter.authorization.State, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1), Code: "one-time-code",
	})
	if err != nil {
		t.Fatal(err)
	}
	if callback.Status != OAuthFlowSucceeded || callback.FlowID != started.FlowID {
		t.Fatalf("callback result = %#v", callback)
	}
	if authorizedRequest.CredentialSource != ConnectionCredentialSourceAccount ||
		authorizedRequest.Intent != OAuthFlowIntentConnect || authorizedRequest.AccountID != accountID {
		t.Fatalf("callback authorization context = %#v", authorizedRequest)
	}
	view, err := service.Poll(context.Background(), started.FlowID, organizationID, accountID)
	if err != nil || view.Status != OAuthFlowSucceeded || view.AccountDisplayName != "Provider User" {
		t.Fatalf("Poll() = %#v, %v", view, err)
	}
	if view.CredentialSource != ConnectionCredentialSourceAccount || view.UsageRulesRequired {
		t.Fatalf("Poll() safe readiness fields = %#v", view)
	}
	if view.CompletedConnectionID == nil || *view.CompletedConnectionID == uuid.Nil {
		t.Fatalf("Poll() did not return the internal setup continuation reference: %#v", view)
	}
	connections, _ := connectionRepository.List(context.Background(), organizationID, ConnectionListFilter{})
	if len(connections) != 1 || connections[0].EncryptedCredentials == nil || strings.Contains(*connections[0].EncryptedCredentials, "access-token") {
		t.Fatalf("stored OAuth connection = %#v", connections)
	}
	if connections[0].RefreshTokenExpiresAt == nil ||
		!connections[0].RefreshTokenExpiresAt.After(time.Now().UTC().Add(20*time.Hour)) {
		t.Fatalf("stored refresh token expiry = %#v", connections[0].RefreshTokenExpiresAt)
	}
	credentials, err := cipher.DecryptCredentials(*connections[0].EncryptedCredentials, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connections[0].ID,
		IntegrationID: "fake", CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if credentials["access_token"] != "access-token" || credentials["refresh_token"] != "refresh-token" {
		t.Fatalf("decrypted credentials keys = %#v", credentials)
	}
	destroyCredentialMap(credentials)
}

func TestPrepareOAuthConnectionQueuesOnlyDistinctSupersededCredential(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		replacementToken string
		wantRevocation   bool
	}{
		{name: "rotated refresh token", replacementToken: "new-refresh-token", wantRevocation: true},
		{name: "same refresh token", replacementToken: "old-refresh-token", wantRevocation: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			adapter := &fakeOAuthAdapter{}
			registry := NewRegistry()
			if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
				t.Fatal(err)
			}
			cipher, err := NewCredentialCipher("12345678901234567890123456789012")
			if err != nil {
				t.Fatal(err)
			}
			organizationID, accountID, connectionID := uuid.New(), uuid.New(), uuid.New()
			currentCredentials := map[string]string{
				"access_token":  "old-access-token",
				"refresh_token": "old-refresh-token",
				"token_type":    "Bearer",
			}
			envelope, err := cipher.EncryptCredentials(currentCredentials, CredentialAAD{
				OrganizationID: organizationID, ConnectionID: connectionID,
				IntegrationID: "fake", CredentialVersion: 1,
			})
			destroyCredentialMap(currentCredentials)
			if err != nil {
				t.Fatal(err)
			}
			connectionRepository := newMemoryConnectionRepository()
			connection := &IntegrationConnection{
				ID: connectionID, OrganizationID: organizationID,
				IntegrationID: "fake", DriverID: "fake-oauth", Name: "Existing",
				CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &accountID,
				AuthType: ConnectionAuthTypeOAuth2, AuthMethodID: "user_oauth",
				EncryptedCredentials: &envelope, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
				Status: ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy,
				AuthStatus: ConnectionAuthValid, ScopeStatus: ConnectionScopeVerified,
			}
			if err := connectionRepository.Create(context.Background(), connection); err != nil {
				t.Fatal(err)
			}
			clientService := NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
				IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
				ClientID: "client-id", ClientSecret: "client-secret",
			}})
			service := NewOAuthFlowService(
				newMemoryOAuthFlowRepository(),
				NewOAuthStateService(&memoryOAuthStateRepository{}, cipher, time.Minute),
				registry,
				clientService,
				connectionRepository,
				cipher,
			)
			recovery := NewOAuthRecoveryService(
				newMemoryDurableOAuthOutbox(&durableOAuthTaskStore{}),
				connectionRepository,
				NewOAuthConnectionRevoker(cipher, registry, clientService),
				cipher,
			).WithFlowRepository(service.flows)
			service.WithOAuthRecovery(recovery)
			flow := &IntegrationOAuthFlow{
				OrganizationID: organizationID, AccountID: accountID, ConnectionID: &connectionID,
				IntegrationID: "fake", DriverID: "fake-oauth", ConnectionName: "Existing",
				CredentialSource: ConnectionCredentialSourceAccount, AuthMethodID: "user_oauth",
			}
			prepared, create, revocation, err := service.prepareOAuthConnection(
				context.Background(),
				flow,
				OAuthTokenSet{
					AccessToken: "new-access-token", RefreshToken: testCase.replacementToken,
					TokenType: "Bearer", Scopes: []string{"identity.read", "account.read"},
				},
				OAuthProfile{AccountID: "provider-account", DisplayName: "Provider User"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if create || prepared.CredentialVersion != 2 {
				t.Fatalf("prepared connection = %#v, create = %v", prepared, create)
			}
			if (revocation != nil) != testCase.wantRevocation {
				t.Fatalf("revocation = %#v, want present = %v", revocation, testCase.wantRevocation)
			}
			if revocation != nil {
				oldCredentials, decryptErr := cipher.DecryptCredentials(
					revocation.EncryptedCredentials,
					CredentialAAD{
						OrganizationID: organizationID, ConnectionID: connectionID,
						IntegrationID: "fake", CredentialVersion: 1,
					},
				)
				if decryptErr != nil {
					t.Fatal(decryptErr)
				}
				if oldCredentials["refresh_token"] != "old-refresh-token" {
					t.Fatalf("revocation snapshot did not retain the old refresh token")
				}
				destroyCredentialMap(oldCredentials)
			}
		})
	}
}

func TestOAuthFlowDurablyRevokesIssuedTokenWhenPostExchangePersistenceFails(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	provider, ok := service.registry.OAuthProvider("fake", "fake-oauth")
	if !ok {
		t.Fatal("fake OAuth provider is unavailable")
	}
	adapter := provider.(*fakeOAuthAdapter)
	started, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Compensated", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	adapter.profileErr = NewError(ErrorCodeResponseInvalid, "profile failed", nil)
	state := adapter.authorization.State
	adapter.mu.Unlock()
	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: state, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1), Code: "one-time-code",
	})
	if ErrorCode(err) != ErrorCodeResponseInvalid {
		t.Fatalf("Callback() error = %v, code = %q", err, ErrorCode(err))
	}
	adapter.mu.Lock()
	revokeCalls, revokedHint := adapter.revokeCalls, adapter.revokedHint
	adapter.mu.Unlock()
	if revokeCalls != 0 {
		t.Fatalf("callback directly revoked a durably recorded token %d times", revokeCalls)
	}
	flow, err := flowRepository.GetForActor(context.Background(), oauthStateDigest(started.FlowID), organizationID, accountID)
	if err != nil || flow.Status != OAuthFlowFailed || flow.EncryptedFlowToken != "" {
		t.Fatalf("failed flow retained temporary token: %#v, %v", flow, err)
	}
	if err := service.recovery.RecoverBatch(context.Background(), 10); err != nil {
		t.Fatalf("RecoverBatch() error = %v", err)
	}
	adapter.mu.Lock()
	revokeCalls, revokedHint = adapter.revokeCalls, adapter.revokedHint
	adapter.mu.Unlock()
	if revokeCalls != 1 || revokedHint != "refresh_token" {
		t.Fatalf("durable compensating revocation calls = %d, hint = %q", revokeCalls, revokedHint)
	}
}

func TestOAuthFlowPersistsEncryptedCompensationBeforeProfileFailureAndRecoversAfterRestart(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	provider, ok := service.registry.OAuthProvider("fake", "fake-oauth")
	if !ok {
		t.Fatal("fake OAuth provider is unavailable")
	}
	adapter := provider.(*fakeOAuthAdapter)
	revoker := NewOAuthConnectionRevoker(service.cipher, service.registry, service.clients)
	store := &durableOAuthTaskStore{}
	outbox := newMemoryDurableOAuthOutbox(store)
	recovery := NewOAuthRecoveryService(outbox, connectionRepository, revoker, service.cipher).
		WithFlowRepository(flowRepository)
	service.WithOAuthRecovery(recovery)

	_, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Durably compensated", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	adapter.profileErr = NewError(ErrorCodeResponseInvalid, "profile failed", nil)
	state := adapter.authorization.State
	adapter.mu.Unlock()
	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: state, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1), Code: "one-time-code",
	})
	if ErrorCode(err) != ErrorCodeResponseInvalid {
		t.Fatalf("Callback() error = %v, code = %q", err, ErrorCode(err))
	}
	if outbox.len() != 1 {
		t.Fatalf("durable compensation tasks = %d, want 1", outbox.len())
	}
	adapter.mu.Lock()
	revokeCallsBeforeRecovery := adapter.revokeCalls
	adapter.mu.Unlock()
	if revokeCallsBeforeRecovery != 0 {
		t.Fatalf("callback directly revoked a durably recorded token %d times", revokeCallsBeforeRecovery)
	}
	store.mu.Lock()
	var queued OAuthRecoveryTask
	for _, task := range store.tasks {
		queued = task
	}
	payload, marshalErr := json.Marshal(queued)
	store.mu.Unlock()
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if queued.CompensationFlowID == nil || *queued.CompensationFlowID != queued.ConnectionID ||
		queued.EncryptedClientCredentials == "" {
		t.Fatalf("durable compensation task = %#v", queued)
	}
	for _, secret := range []string{"access-token", "refresh-token", "client-secret"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("durable compensation payload contains plaintext secret %q", secret)
		}
	}

	// Simulate a fresh API process draining the PostgreSQL-backed task.
	restarted := NewOAuthRecoveryService(
		newMemoryDurableOAuthOutbox(store),
		connectionRepository,
		revoker,
		service.cipher,
	).WithFlowRepository(flowRepository)
	if err := restarted.RecoverBatch(context.Background(), 10); err != nil {
		t.Fatalf("RecoverBatch() error = %v", err)
	}
	if outbox.len() != 0 {
		t.Fatal("restarted recovery did not acknowledge the revoked compensation token")
	}
	adapter.mu.Lock()
	revokeCalls := adapter.revokeCalls
	adapter.mu.Unlock()
	if revokeCalls != 1 {
		t.Fatalf("provider revocation calls = %d, want durable recovery only", revokeCalls)
	}
}

func TestOAuthFlowFailsClosedWhenPostExchangeCompensationCannotBePersisted(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	provider, ok := service.registry.OAuthProvider("fake", "fake-oauth")
	if !ok {
		t.Fatal("fake OAuth provider is unavailable")
	}
	adapter := provider.(*fakeOAuthAdapter)
	revoker := NewOAuthConnectionRevoker(service.cipher, service.registry, service.clients)
	fallbackOutbox := newMemoryDurableOAuthOutbox(&durableOAuthTaskStore{})
	recovery := NewOAuthRecoveryService(
		&failEnqueueOAuthOutbox{OAuthRecoveryOutbox: fallbackOutbox},
		connectionRepository,
		revoker,
		service.cipher,
	).WithFlowRepository(flowRepository)
	service.WithOAuthRecovery(recovery)

	started, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID,
		IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount,
		Intent:               OAuthFlowIntentConnect,
		ConnectionName:       "No unsafe downgrade",
		RequestedActionIDs:   []string{"fake.account.read"},
		RedirectURI:          "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: adapter.authorization.State, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1), Code: "one-time-code",
	})
	if ErrorCode(err) != ErrorCodeConnectionInvalid {
		t.Fatalf("Callback() error = %v, code = %q, want connection invalid", err, ErrorCode(err))
	}
	adapter.mu.Lock()
	exchangeCalls, revokeCalls := adapter.exchangeCalls, adapter.revokeCalls
	adapter.mu.Unlock()
	if exchangeCalls != 1 || revokeCalls != 1 {
		t.Fatalf("provider calls exchange=%d revoke=%d, want 1/1", exchangeCalls, revokeCalls)
	}
	connections, listErr := connectionRepository.List(context.Background(), organizationID, ConnectionListFilter{})
	if listErr != nil || len(connections) != 0 {
		t.Fatalf("connections after failed durable enqueue = %#v, %v", connections, listErr)
	}
	flow, flowErr := flowRepository.GetForActor(
		context.Background(),
		oauthStateDigest(started.FlowID),
		organizationID,
		accountID,
	)
	if flowErr != nil || flow.Status != OAuthFlowFailed {
		t.Fatalf("flow after failed durable enqueue = %#v, %v", flow, flowErr)
	}
}

func TestOAuthFlowSuccessfulCommitAcknowledgesCompensationWithoutRevocation(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	provider, _ := service.registry.OAuthProvider("fake", "fake-oauth")
	adapter := provider.(*fakeOAuthAdapter)
	store := &durableOAuthTaskStore{}
	recovery := NewOAuthRecoveryService(
		newMemoryDurableOAuthOutbox(store),
		connectionRepository,
		NewOAuthConnectionRevoker(service.cipher, service.registry, service.clients),
		service.cipher,
	).WithFlowRepository(flowRepository)
	service.WithOAuthRecovery(recovery)
	_, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 2),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Committed", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: adapter.authorization.State, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 2), Code: "one-time-code",
	})
	if err != nil {
		t.Fatalf("Callback() error = %v", err)
	}
	if outbox := newMemoryDurableOAuthOutbox(store); outbox.len() != 0 {
		t.Fatal("successful OAuth commit retained its compensation task")
	}
	adapter.mu.Lock()
	revokeCalls := adapter.revokeCalls
	adapter.mu.Unlock()
	if revokeCalls != 0 {
		t.Fatalf("successful OAuth commit revoked live provider tokens %d times", revokeCalls)
	}
}

func TestOAuthFlowSucceededGuardPreventsRevocationWhenImmediateAcknowledgementFails(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	provider, _ := service.registry.OAuthProvider("fake", "fake-oauth")
	adapter := provider.(*fakeOAuthAdapter)
	store := &durableOAuthTaskStore{}
	outbox := &failOnceAckOAuthOutbox{
		memoryDurableOAuthOutbox: newMemoryDurableOAuthOutbox(store),
		failNext:                 true,
	}
	recovery := NewOAuthRecoveryService(
		outbox,
		connectionRepository,
		NewOAuthConnectionRevoker(service.cipher, service.registry, service.clients),
		service.cipher,
	).WithFlowRepository(flowRepository)
	service.WithOAuthRecovery(recovery)
	_, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 4),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Ack retry", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: adapter.authorization.State, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 4), Code: "one-time-code",
	})
	if err != nil {
		t.Fatalf("Callback() error = %v", err)
	}
	if outbox.len() != 1 {
		t.Fatalf("failed immediate acknowledgement retained tasks = %d, want 1", outbox.len())
	}
	if err := recovery.RecoverBatch(context.Background(), 10); err != nil {
		t.Fatalf("RecoverBatch() error = %v", err)
	}
	if outbox.len() != 0 {
		t.Fatal("succeeded flow guard task was not acknowledged by recovery")
	}
	adapter.mu.Lock()
	revokeCalls := adapter.revokeCalls
	adapter.mu.Unlock()
	if revokeCalls != 0 {
		t.Fatalf("succeeded flow guard revoked live provider tokens %d times", revokeCalls)
	}
}

func TestOAuthFlowAmbiguousCommitNeverRevokesLiveTokensAndRecoveryAcknowledges(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&ambiguousOAuthConnectionCommitter{
			delegate: &memoryOAuthConnectionCommitter{
				flows: flowRepository, connections: connectionRepository,
			},
		}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	provider, _ := service.registry.OAuthProvider("fake", "fake-oauth")
	adapter := provider.(*fakeOAuthAdapter)
	store := &durableOAuthTaskStore{}
	outbox := newMemoryDurableOAuthOutbox(store)
	recovery := NewOAuthRecoveryService(
		outbox,
		connectionRepository,
		NewOAuthConnectionRevoker(service.cipher, service.registry, service.clients),
		service.cipher,
	).WithFlowRepository(flowRepository)
	service.WithOAuthRecovery(recovery)

	started, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID,
		IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 42),
		CredentialSource:     ConnectionCredentialSourceAccount,
		Intent:               OAuthFlowIntentConnect,
		ConnectionName:       "Ambiguous commit",
		RequestedActionIDs:   []string{"fake.account.read"},
		RedirectURI:          "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: adapter.authorization.State, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 42), Code: "one-time-code",
	})
	if ErrorCode(err) != ErrorCodeConnectionConflict {
		t.Fatalf("Callback() error = %v, code = %q, want connection conflict", err, ErrorCode(err))
	}
	flow, flowErr := flowRepository.GetForActor(
		context.Background(),
		oauthStateDigest(started.FlowID),
		organizationID,
		accountID,
	)
	if flowErr != nil || flow.Status != OAuthFlowSucceeded || flow.CompletedConnectionID == nil {
		t.Fatalf("ambiguously committed flow = %#v, %v", flow, flowErr)
	}
	connections, listErr := connectionRepository.List(context.Background(), organizationID, ConnectionListFilter{})
	if listErr != nil || len(connections) != 1 {
		t.Fatalf("ambiguously committed connections = %#v, %v", connections, listErr)
	}
	if outbox.len() != 1 {
		t.Fatalf("ambiguous commit compensation tasks = %d, want 1", outbox.len())
	}
	adapter.mu.Lock()
	revokeCalls := adapter.revokeCalls
	adapter.mu.Unlock()
	if revokeCalls != 0 {
		t.Fatalf("ambiguous commit revoked live provider tokens %d times", revokeCalls)
	}

	if err := recovery.RecoverBatch(context.Background(), 10); err != nil {
		t.Fatalf("RecoverBatch() error = %v", err)
	}
	if outbox.len() != 0 {
		t.Fatal("succeeded ambiguous commit task was not acknowledged")
	}
	adapter.mu.Lock()
	revokeCalls = adapter.revokeCalls
	adapter.mu.Unlock()
	if revokeCalls != 0 {
		t.Fatalf("recovery revoked ambiguously committed live tokens %d times", revokeCalls)
	}
}

func TestOAuthFlowWithoutProviderRevocationRetainsManualRemediation(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	provider, _ := service.registry.OAuthProvider("fake", "fake-oauth")
	adapter := provider.(*fakeOAuthAdapter)
	adapter.mu.Lock()
	adapter.noRevocation = true
	adapter.profileErr = NewError(ErrorCodeResponseInvalid, "profile failed", nil)
	adapter.mu.Unlock()
	store := &durableOAuthTaskStore{}
	outbox := &retainingDeadLetterOAuthOutbox{
		memoryDurableOAuthOutbox: newMemoryDurableOAuthOutbox(store),
		deadLetters:              map[string]OAuthRecoveryTask{},
		reasons:                  map[string]string{},
	}
	recovery := NewOAuthRecoveryService(
		outbox,
		connectionRepository,
		NewOAuthConnectionRevoker(service.cipher, service.registry, service.clients),
		service.cipher,
	).WithFlowRepository(flowRepository)
	service.WithOAuthRecovery(recovery)
	_, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 3),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Manual cleanup", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: adapter.authorization.State, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 3), Code: "one-time-code",
	})
	if ErrorCode(err) != ErrorCodeResponseInvalid {
		t.Fatalf("Callback() error = %v", err)
	}
	if err := recovery.RecoverBatch(context.Background(), 10); err != nil {
		t.Fatalf("RecoverBatch() error = %v", err)
	}
	if len(outbox.deadLetters) != 1 {
		t.Fatalf("manual remediation dead letters = %d, want 1", len(outbox.deadLetters))
	}
	for id, reason := range outbox.reasons {
		if reason != oauthRecoveryManualReason || !strings.HasPrefix(id, "revoke-") {
			t.Fatalf("manual remediation = %q, %q", id, reason)
		}
	}
	adapter.mu.Lock()
	revokeCalls := adapter.revokeCalls
	adapter.mu.Unlock()
	if revokeCalls != 0 {
		t.Fatalf("provider without revocation endpoint received %d fabricated revoke calls", revokeCalls)
	}
}

func TestOAuthFlowRequiresStartingBrowserBindingBeforeTokenExchange(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	provider, ok := service.registry.OAuthProvider("fake", "fake-oauth")
	if !ok {
		t.Fatal("fake OAuth provider is unavailable")
	}
	adapter := provider.(*fakeOAuthAdapter)
	browserA := testOAuthBrowserBindingDigest(t, 10)
	browserB := testOAuthBrowserBindingDigest(t, 11)
	started, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID,
		BrowserBindingDigest: browserA,
		IntegrationID:        "fake", AuthMethodID: "user_oauth",
		CredentialSource: ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Browser-bound", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	state := adapter.authorization.State

	if _, err := service.Callback(context.Background(), OAuthCallbackRequest{
		State: state, BrowserBindingDigest: browserB, Code: "one-time-code",
	}); ErrorCode(err) != ErrorCodeAuthInvalid {
		t.Fatalf("Browser B Callback() error = %v, code = %q", err, ErrorCode(err))
	}
	adapter.mu.Lock()
	exchangeCalls := adapter.exchangeCalls
	adapter.mu.Unlock()
	if exchangeCalls != 0 {
		t.Fatalf("cross-browser callback reached provider token exchange %d times", exchangeCalls)
	}
	connections, _ := connectionRepository.List(context.Background(), organizationID, ConnectionListFilter{})
	if len(connections) != 0 {
		t.Fatalf("cross-browser callback created connections = %#v", connections)
	}
	flow, err := flowRepository.GetForActor(context.Background(), oauthStateDigest(started.FlowID), organizationID, accountID)
	if err != nil || flow.Status != OAuthFlowPending {
		t.Fatalf("cross-browser callback consumed flow = %#v, %v", flow, err)
	}

	result, err := service.Callback(context.Background(), OAuthCallbackRequest{
		State: state, BrowserBindingDigest: browserA, Code: "one-time-code",
	})
	if err != nil || result.Status != OAuthFlowSucceeded {
		t.Fatalf("Browser A Callback() = %#v, %v", result, err)
	}
	adapter.mu.Lock()
	exchangeCalls = adapter.exchangeCalls
	adapter.mu.Unlock()
	if exchangeCalls != 1 {
		t.Fatalf("same-browser callback exchange calls = %d, want 1", exchangeCalls)
	}
}

func TestDeriveOAuthScopesIncludesIdentityScopesForWriteOnlySelection(t *testing.T) {
	method := AuthMethodDefinition{
		ID: "user_oauth", Type: AuthMethodTypeOAuth2,
		OAuth: &OAuthMethodMetadata{IdentityScopes: []string{"identity.read", "identity.read"}},
	}
	write := testAction("fake.message.send", "send_fake_message")
	write.RequiredScopes = []string{"message.send"}
	actionIDs, scopes, err := deriveOAuthScopes(
		ProviderDefinition{Actions: []ActionDefinition{write}},
		method,
		[]string{"fake.message.send"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actionIDs, []string{"fake.message.send"}) ||
		!reflect.DeepEqual(scopes, []string{"identity.read", "message.send"}) {
		t.Fatalf("actions = %#v, scopes = %#v", actionIDs, scopes)
	}
}

func TestDeriveOAuthScopesRequestsPreferredAlternativeOnly(t *testing.T) {
	method := AuthMethodDefinition{
		ID: "user_oauth", Type: AuthMethodTypeOAuth2,
		OAuth: &OAuthMethodMetadata{IdentityScopes: []string{"identity.read"}},
	}
	action := testAction("fake.message.list", "list_fake_messages")
	action.RequiredScopes = []string{"message.base"}
	action.RequiredAnyScopes = []string{"message.read", "message.history"}
	action.PreferredScopes = []string{"message.read"}

	actionIDs, scopes, err := deriveOAuthScopes(
		ProviderDefinition{Actions: []ActionDefinition{action}},
		method,
		[]string{action.ID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actionIDs, []string{action.ID}) ||
		!reflect.DeepEqual(scopes, []string{"identity.read", "message.base", "message.read"}) {
		t.Fatalf("actions = %#v, scopes = %#v", actionIDs, scopes)
	}
	if slices.Contains(scopes, "message.history") {
		t.Fatalf("OAuth requested every alternative instead of the preferred scope: %#v", scopes)
	}
}

func TestDeriveOAuthScopesRejectsActionIncompatibleWithAuthMethod(t *testing.T) {
	method := AuthMethodDefinition{
		ID: "user_oauth", Type: AuthMethodTypeOAuth2,
		OAuth: &OAuthMethodMetadata{IdentityScopes: []string{"identity.read"}},
	}
	tenantOnly := testAction("fake.message.send_as_app", "send_fake_app_message")
	tenantOnly.RequiredScopes = []string{"message.send_as_app"}
	tenantOnly.SupportedAuthMethodIDs = []string{"tenant_app"}

	_, _, err := deriveOAuthScopes(
		ProviderDefinition{Actions: []ActionDefinition{tenantOnly}},
		method,
		[]string{"fake.message.send_as_app"},
	)
	if ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("deriveOAuthScopes() error = %v (%s), want invalid input", err, ErrorCode(err))
	}
}

func TestOAuthFlowRejectsUnknownActionsBeforeAuthorization(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	service := NewOAuthFlowService(newMemoryOAuthFlowRepository(), NewOAuthStateService(&memoryOAuthStateRepository{}, cipher, time.Minute).
		WithAllowedRedirectURIs([]string{"https://app.example.com/api/integrations/oauth/callback"}), registry,
		NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
			IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
		}}), newMemoryConnectionRepository(), cipher)
	_, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: uuid.New(), AccountID: uuid.New(), IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Invalid", RequestedActionIDs: []string{"fake.future.delete"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestOAuthFlowStartLimitsPendingFlowsPerActorProvider(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	service.WithStartPolicy(2, time.Minute, 10)
	for index := 0; index < 2; index++ {
		if _, err := service.Start(context.Background(), OAuthFlowStartRequest{
			OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
			BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
			CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
			ConnectionName: "Pending", RequestedActionIDs: []string{"fake.account.read"},
			RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
		}); err != nil {
			t.Fatalf("Start(%d) error = %v", index, err)
		}
	}
	_, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Blocked", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if ErrorCode(err) != ErrorCodeRateLimited {
		t.Fatalf("pending flow limit error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestOAuthFlowStartLimitsRapidRestartsAfterCancellation(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	service.WithStartPolicy(5, time.Minute, 2)
	for index := 0; index < 2; index++ {
		started, err := service.Start(context.Background(), OAuthFlowStartRequest{
			OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
			BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
			CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
			ConnectionName: "Restart", RequestedActionIDs: []string{"fake.account.read"},
			RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
		})
		if err != nil {
			t.Fatalf("Start(%d) error = %v", index, err)
		}
		if err := service.Cancel(context.Background(), started.FlowID, organizationID, accountID); err != nil {
			t.Fatalf("Cancel(%d) error = %v", index, err)
		}
	}
	_, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID, IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Too frequent", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if ErrorCode(err) != ErrorCodeRateLimited {
		t.Fatalf("rapid restart limit error = %v, code = %q", err, ErrorCode(err))
	}
}

func newOAuthFlowAdmissionTestService(t *testing.T) (*OAuthFlowService, uuid.UUID, uuid.UUID) {
	t.Helper()
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	service := NewOAuthFlowService(
		newMemoryOAuthFlowRepository(),
		NewOAuthStateService(&memoryOAuthStateRepository{}, cipher, time.Minute).
			WithAllowedRedirectURIs([]string{"https://app.example.com/api/integrations/oauth/callback"}),
		registry,
		NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
			IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
		}}),
		newMemoryConnectionRepository(),
		cipher,
	)
	withTestOAuthFlowRecovery(service)
	return service, uuid.New(), uuid.New()
}

func withTestOAuthFlowRecovery(service *OAuthFlowService) *OAuthFlowService {
	if service == nil {
		return service
	}
	flowRepository := service.flows
	outbox := newMemoryDurableOAuthOutbox(&durableOAuthTaskStore{})
	revoker := NewOAuthConnectionRevoker(service.cipher, service.registry, service.clients)
	recovery := NewOAuthRecoveryService(outbox, service.connections, revoker, service.cipher).
		WithFlowRepository(flowRepository)
	return service.WithOAuthRecovery(recovery)
}

func TestOAuthFlowCallbackFailsClosedBeforeExchangeWithoutDurableRecovery(t *testing.T) {
	service, organizationID, accountID := newOAuthFlowAdmissionTestService(t)
	flowRepository := service.flows.(*memoryOAuthFlowRepository)
	connectionRepository := service.connections.(*memoryConnectionRepository)
	service.
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))

	started, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID,
		IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount,
		Intent:               OAuthFlowIntentConnect,
		ConnectionName:       "Fail closed",
		RequestedActionIDs:   []string{"fake.account.read"},
		RedirectURI:          "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := service.registry.OAuthProvider("fake", "fake-oauth")
	if !ok {
		t.Fatal("fake OAuth provider is unavailable")
	}
	adapter := provider.(*fakeOAuthAdapter)
	service.recovery = nil

	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: adapter.authorization.State, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1), Code: "one-time-code",
	})
	if ErrorCode(err) != ErrorCodeDisabled {
		t.Fatalf("Callback() error = %v, code = %q, want disabled", err, ErrorCode(err))
	}
	adapter.mu.Lock()
	exchangeCalls := adapter.exchangeCalls
	adapter.mu.Unlock()
	if exchangeCalls != 0 {
		t.Fatalf("token exchange calls = %d, want 0", exchangeCalls)
	}
	flow, err := flowRepository.GetForActor(
		context.Background(),
		oauthStateDigest(started.FlowID),
		organizationID,
		accountID,
	)
	if err != nil || flow.Status != OAuthFlowPending {
		t.Fatalf("flow after unavailable recovery = %#v, %v", flow, err)
	}
}

func TestOAuthFlowRejectsStateAndFlowIdentityMismatch(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	stateRepository := &memoryOAuthStateRepository{}
	flowRepository := newMemoryOAuthFlowRepository()
	connectionRepository := newMemoryConnectionRepository()
	service := NewOAuthFlowService(
		flowRepository,
		NewOAuthStateService(stateRepository, cipher, time.Minute).
			WithAllowedRedirectURIs([]string{"https://app.example.com/api/integrations/oauth/callback"}),
		registry,
		NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
			IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
		}}),
		connectionRepository,
		cipher,
	).
		WithConnectionCommitter(&memoryOAuthConnectionCommitter{flows: flowRepository, connections: connectionRepository}).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(func(context.Context, OAuthCallbackAuthorizationRequest) error {
			return nil
		}))
	withTestOAuthFlowRecovery(service)
	organizationID := uuid.New()
	started, err := service.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: uuid.New(), IntegrationID: "fake", AuthMethodID: "user_oauth",
		BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1),
		CredentialSource:     ConnectionCredentialSourceAccount, Intent: OAuthFlowIntentConnect,
		ConnectionName: "Mismatch", RequestedActionIDs: []string{"fake.account.read"},
		RedirectURI: "https://app.example.com/api/integrations/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	stateRepository.state.DriverID = "different-driver"
	_, err = service.Callback(context.Background(), OAuthCallbackRequest{
		State: adapter.authorization.State, BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 1), Code: "one-time-code",
	})
	if ErrorCode(err) != ErrorCodeAuthInvalid {
		t.Fatalf("Callback() error = %v", err)
	}
	flow, err := flowRepository.GetForActor(context.Background(), oauthStateDigest(started.FlowID),
		stateRepository.state.OrganizationID, stateRepository.state.AccountID)
	if err != nil || flow.Status != OAuthFlowFailed {
		t.Fatalf("mismatched flow = %#v, %v", flow, err)
	}
	connections, _ := connectionRepository.List(context.Background(), organizationID, ConnectionListFilter{})
	if len(connections) != 0 {
		t.Fatalf("mismatched state created connections = %#v", connections)
	}
}

func TestOAuthFlowOrganizationSuccessRequiresUsageRules(t *testing.T) {
	flow := &IntegrationOAuthFlow{
		IntegrationID: "fake", AuthMethodID: "organization_oauth", Intent: OAuthFlowIntentConnect,
		Status: OAuthFlowSucceeded, CredentialSource: ConnectionCredentialSourceOrganization,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	view := flowPublicView("opaque-flow-id-that-is-long-enough", flow)
	if !view.UsageRulesRequired {
		t.Fatalf("organization OAuth readiness = %#v", view)
	}
}

func TestOAuthClientConfigIsWriteOnlyAndPreservesOmittedClientFields(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	repository := &memoryOAuthClientConfigRepository{}
	service := NewOAuthClientConfigService(repository, cipher, registry, nil).
		WithFlowImpactRepository(newMemoryOAuthFlowRepository())
	organizationID := uuid.New()
	view, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "visible-client-identifier", ClientSecret: "private-client-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "visible-client-identifier") || strings.Contains(string(encoded), "private-client-secret") ||
		view.ClientIDMasked == "" || !view.HasSecret {
		t.Fatalf("unsafe OAuth client config view = %s", encoded)
	}
	updated, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		Revision: view.Revision,
	})
	if err != nil {
		t.Fatalf("Put() preserving write-only fields error = %v", err)
	}
	if updated.Revision != view.Revision+1 {
		t.Fatalf("updated revision = %d", updated.Revision)
	}
	resolved, err := service.ResolveOAuthClient(context.Background(), OAuthClientResolveRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ClientID != "visible-client-identifier" || resolved.ClientSecret != "private-client-secret" {
		t.Fatalf("write-only values were not preserved")
	}
	resolved.Destroy()
	if _, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		Revision: updated.Revision, Config: map[string]any{"unexpected": "value"},
	}); ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("unknown OAuth client field error = %v", err)
	}
	if _, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "stale-client-id",
	}); ErrorCode(err) != ErrorCodeConnectionConflict {
		t.Fatalf("revisionless OAuth client update error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestOAuthClientConfigViewFailsClosedWhenEncryptedCredentialsAreUnreadable(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	repository := &memoryOAuthClientConfigRepository{}
	service := NewOAuthClientConfigService(repository, cipher, registry, nil).
		WithFlowImpactRepository(newMemoryOAuthFlowRepository())
	organizationID := uuid.New()
	if _, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "client-id", ClientSecret: "client-secret",
	}); err != nil {
		t.Fatal(err)
	}
	repository.config.EncryptedCredentials = "corrupted"

	if _, err := service.GetView(context.Background(), OAuthClientResolveRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
	}); ErrorCode(err) != ErrorCodeConnectionInvalid {
		t.Fatalf("GetView() error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestOAuthClientConfigAliasesShareOneOAuthApplication(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registration := oauthTestRegistration(adapter)
	registration.Definition.AuthMethods[0].OAuth.ClientConfigID = "shared_oauth_app"
	organizationMethod := registration.Definition.AuthMethods[0]
	organizationMethod.ID = "organization_oauth"
	organizationMethod.CredentialSource = ConnectionCredentialSourceOrganization
	organizationMethod.Label = "Connect organization account"
	organizationMethod.LabelI18n = LocalizedText{
		LocaleEnglishUS: "Connect organization account", LocaleSimplifiedChinese: "连接组织账号",
	}
	registration.Definition.AuthMethods = append(registration.Definition.AuthMethods, organizationMethod)
	registration.Definition.Actions[0].SupportedAuthMethodIDs = []string{"user_oauth", "organization_oauth"}
	registry := NewRegistry()
	if err := registry.Register(registration); err != nil {
		t.Fatal(err)
	}
	service := NewOAuthClientConfigService(nil, nil, registry, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "shared-client", ClientSecret: "shared-secret",
	}})
	for _, methodID := range []string{"user_oauth", "organization_oauth"} {
		client, err := service.ResolveOAuthClient(context.Background(), OAuthClientResolveRequest{
			OrganizationID: uuid.New(), IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: methodID,
		})
		if err != nil {
			t.Fatalf("ResolveOAuthClient(%s) error = %v", methodID, err)
		}
		if client.ClientID != "shared-client" {
			t.Fatalf("ResolveOAuthClient(%s) client id mismatch", methodID)
		}
		client.Destroy()
	}
}

func TestOAuthClientConfigCannotBeRemovedOrReidentifiedWhileConnectionsDependOnIt(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	configRepository := &memoryOAuthClientConfigRepository{}
	connectionRepository := newMemoryConnectionRepository()
	service := NewOAuthClientConfigService(configRepository, cipher, registry, nil).
		WithConnectionRepository(connectionRepository).
		WithFlowImpactRepository(newMemoryOAuthFlowRepository())
	organizationID, accountID := uuid.New(), uuid.New()
	view, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "original-client", ClientSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	connection := &IntegrationConnection{
		ID: uuid.New(), OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth",
		Name: "Dependent account", CredentialSource: ConnectionCredentialSourceAccount,
		AuthType: ConnectionAuthTypeOAuth2, AuthMethodID: "user_oauth", OwnerAccountID: &accountID,
		Config: map[string]any{}, GrantedScopes: []string{"account.read"}, Status: ConnectionStatusActive,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}
	if err := connectionRepository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	request := OAuthClientResolveRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
	}
	impact, err := service.Impact(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if impact.DependentConnections != 1 || impact.ActiveConnections != 1 || impact.CanRemove {
		t.Fatalf("Impact() = %+v, want one active dependency and removal blocked", impact)
	}
	if err := service.Delete(context.Background(), request); ErrorCode(err) != ErrorCodeConnectionInUse {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "replacement-client", Revision: view.Revision,
	}); ErrorCode(err) != ErrorCodeConnectionInUse {
		t.Fatalf("Put() replacing client id error = %v", err)
	}
}

type pendingOAuthClientConfigFixture struct {
	service        *OAuthClientConfigService
	flowRepository *memoryOAuthFlowRepository
	organizationID uuid.UUID
	flowID         uuid.UUID
	view           OAuthClientConfigView
}

func newPendingOAuthClientConfigFixture(t *testing.T, deployment []OAuthDeploymentClient) pendingOAuthClientConfigFixture {
	t.Helper()
	adapter := &fakeOAuthAdapter{}
	registration := oauthTestRegistration(adapter)
	providerConfigField := CredentialFieldDefinition{
		Key: "tenant_domain", Label: "Tenant domain", Input: CredentialFieldInputText,
		LabelI18n: LocalizedText{
			LocaleEnglishUS: "Tenant domain", LocaleSimplifiedChinese: "租户域名",
		},
	}
	registration.Definition.AuthMethods[0].OAuth.ClientFields = append(
		registration.Definition.AuthMethods[0].OAuth.ClientFields,
		providerConfigField,
	)
	registry := NewRegistry()
	if err := registry.Register(registration); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	configRepository := &memoryOAuthClientConfigRepository{}
	connectionRepository := newMemoryConnectionRepository()
	flowRepository := newMemoryOAuthFlowRepository()
	service := NewOAuthClientConfigService(configRepository, cipher, registry, deployment).
		WithConnectionRepository(connectionRepository).
		WithFlowImpactRepository(flowRepository)
	organizationID := uuid.New()
	view, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "original-client", ClientSecret: "original-secret",
		Config: map[string]any{"tenant_domain": "tenant.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	flowID := uuid.New()
	if err := flowRepository.Create(context.Background(), &IntegrationOAuthFlow{
		ID: flowID, FlowDigest: strings.Repeat("a", 64), EncryptedFlowToken: "v2.encrypted",
		OrganizationID: organizationID, AccountID: uuid.New(), IntegrationID: "fake", DriverID: "fake-oauth",
		AuthMethodID: "user_oauth", CredentialSource: ConnectionCredentialSourceAccount,
		Intent: OAuthFlowIntentConnect, ConnectionName: "Pending", Status: OAuthFlowPending,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	return pendingOAuthClientConfigFixture{
		service: service, flowRepository: flowRepository, organizationID: organizationID,
		flowID: flowID, view: view,
	}
}

func (fixture pendingOAuthClientConfigFixture) putRequest() PutOAuthClientConfigRequest {
	return PutOAuthClientConfigRequest{
		OrganizationID: fixture.organizationID, IntegrationID: "fake", DriverID: "fake-oauth",
		AuthMethodID: "user_oauth", Revision: fixture.view.Revision,
		Config: map[string]any{"tenant_domain": "tenant.example.com"},
	}
}

func TestOAuthClientConfigCannotChangeMaterialWhileAuthorizationFlowIsPending(t *testing.T) {
	t.Run("client secret only", func(t *testing.T) {
		fixture := newPendingOAuthClientConfigFixture(t, nil)
		request := fixture.putRequest()
		request.ClientSecret = "replacement-secret"
		if _, err := fixture.service.Put(context.Background(), request); ErrorCode(err) != ErrorCodeConnectionInUse {
			t.Fatalf("Put() secret-only change error = %v, code = %q", err, ErrorCode(err))
		}
	})

	t.Run("provider config only", func(t *testing.T) {
		fixture := newPendingOAuthClientConfigFixture(t, nil)
		request := fixture.putRequest()
		request.Config = map[string]any{"tenant_domain": "other.example.com"}
		if _, err := fixture.service.Put(context.Background(), request); ErrorCode(err) != ErrorCodeConnectionInUse {
			t.Fatalf("Put() config-only change error = %v, code = %q", err, ErrorCode(err))
		}
	})

	t.Run("client id only", func(t *testing.T) {
		fixture := newPendingOAuthClientConfigFixture(t, nil)
		request := fixture.putRequest()
		request.ClientID = "replacement-client"
		if _, err := fixture.service.Put(context.Background(), request); ErrorCode(err) != ErrorCodeConnectionInUse {
			t.Fatalf("Put() client-id change error = %v, code = %q", err, ErrorCode(err))
		}
	})

	t.Run("canonical no-op", func(t *testing.T) {
		fixture := newPendingOAuthClientConfigFixture(t, nil)
		request := fixture.putRequest()
		request.Config = map[string]any{" TENANT_DOMAIN ": " tenant.example.com "}
		updated, err := fixture.service.Put(context.Background(), request)
		if err != nil {
			t.Fatalf("Put() canonical no-op error = %v", err)
		}
		if updated.Revision != fixture.view.Revision+1 {
			t.Fatalf("Put() canonical no-op revision = %d, want %d", updated.Revision, fixture.view.Revision+1)
		}
	})

	t.Run("delete", func(t *testing.T) {
		fixture := newPendingOAuthClientConfigFixture(t, nil)
		err := fixture.service.Delete(context.Background(), OAuthClientResolveRequest{
			OrganizationID: fixture.organizationID, IntegrationID: "fake",
			DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		})
		if ErrorCode(err) != ErrorCodeConnectionInUse {
			t.Fatalf("Delete() during pending flow error = %v, code = %q", err, ErrorCode(err))
		}
	})
}

func TestOAuthFlowStartSerializesWithOAuthClientMaterialMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *OAuthClientConfigService, OAuthClientConfigView, uuid.UUID) error
	}{
		{
			name: "update",
			mutate: func(
				ctx context.Context,
				clients *OAuthClientConfigService,
				view OAuthClientConfigView,
				organizationID uuid.UUID,
			) error {
				_, err := clients.Put(ctx, PutOAuthClientConfigRequest{
					OrganizationID: organizationID,
					IntegrationID:  "fake",
					DriverID:       "fake-oauth",
					AuthMethodID:   "user_oauth",
					ClientSecret:   "replacement-secret",
					Revision:       view.Revision,
				})
				return err
			},
		},
		{
			name: "delete",
			mutate: func(
				ctx context.Context,
				clients *OAuthClientConfigService,
				_ OAuthClientConfigView,
				organizationID uuid.UUID,
			) error {
				return clients.Delete(ctx, OAuthClientResolveRequest{
					OrganizationID: organizationID,
					IntegrationID:  "fake",
					DriverID:       "fake-oauth",
					AuthMethodID:   "user_oauth",
				})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := &fakeOAuthAdapter{}
			registry := NewRegistry()
			if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
				t.Fatal(err)
			}
			cipher, err := NewCredentialCipher("12345678901234567890123456789012")
			if err != nil {
				t.Fatal(err)
			}
			organizationID, accountID := uuid.New(), uuid.New()
			configRepository := &memoryOAuthClientConfigRepository{}
			flowRepository := newMemoryOAuthFlowRepository()
			connectionRepository := newMemoryConnectionRepository()
			locker := &serialOAuthClientFlowTestLocker{attempts: make(chan struct{}, 4)}
			clients := NewOAuthClientConfigService(configRepository, cipher, registry, nil).
				WithConnectionRepository(connectionRepository).
				WithFlowImpactRepository(flowRepository).
				WithOAuthClientFlowLocker(locker)
			view, err := clients.Put(context.Background(), PutOAuthClientConfigRequest{
				OrganizationID: organizationID,
				IntegrationID:  "fake",
				DriverID:       "fake-oauth",
				AuthMethodID:   "user_oauth",
				ClientID:       "original-client",
				ClientSecret:   "original-secret",
			})
			if err != nil {
				t.Fatal(err)
			}
			waitOAuthClientFlowLockAttempt(t, locker.attempts)

			blockingClients := &blockingOAuthClientResolver{
				delegate: clients,
				entered:  make(chan struct{}),
				release:  make(chan struct{}),
			}
			stateRepository := &memoryOAuthStateRepository{}
			browserBindingDigest := testOAuthBrowserBindingDigest(t, 41)
			flowService := NewOAuthFlowService(
				flowRepository,
				NewOAuthStateService(stateRepository, cipher, 5*time.Minute).
					WithAllowedRedirectURIs([]string{"https://app.example.com/api/integrations/oauth/callback"}),
				registry,
				blockingClients,
				connectionRepository,
				cipher,
			).
				WithOAuthClientFlowLocker(locker).
				WithConnectionCommitter(&memoryOAuthConnectionCommitter{
					flows: flowRepository, connections: connectionRepository,
				}).
				WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(
					func(context.Context, OAuthCallbackAuthorizationRequest) error { return nil },
				))
			withTestOAuthFlowRecovery(flowService)

			startResult := make(chan struct {
				result OAuthFlowStartResult
				err    error
			}, 1)
			go func() {
				result, startErr := flowService.Start(context.Background(), OAuthFlowStartRequest{
					OrganizationID:       organizationID,
					AccountID:            accountID,
					IntegrationID:        "fake",
					AuthMethodID:         "user_oauth",
					BrowserBindingDigest: browserBindingDigest,
					CredentialSource:     ConnectionCredentialSourceAccount,
					Intent:               OAuthFlowIntentConnect,
					ConnectionName:       "Concurrent account",
					RequestedActionIDs:   []string{"fake.account.read"},
					RedirectURI:          "https://app.example.com/api/integrations/oauth/callback",
					ReturnPath:           "/console/integrations",
				})
				startResult <- struct {
					result OAuthFlowStartResult
					err    error
				}{result: result, err: startErr}
			}()
			select {
			case <-blockingClients.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("OAuth flow start did not resolve the original client")
			}
			waitOAuthClientFlowLockAttempt(t, locker.attempts)

			mutationResult := make(chan error, 1)
			go func() {
				mutationResult <- test.mutate(context.Background(), clients, view, organizationID)
			}()
			waitOAuthClientFlowLockAttempt(t, locker.attempts)
			select {
			case mutationErr := <-mutationResult:
				t.Fatalf("OAuth client mutation passed the in-flight start lock: %v", mutationErr)
			default:
			}

			close(blockingClients.release)
			var started OAuthFlowStartResult
			select {
			case outcome := <-startResult:
				if outcome.err != nil {
					t.Fatalf("Start() error = %v", outcome.err)
				}
				started = outcome.result
			case <-time.After(2 * time.Second):
				t.Fatal("OAuth flow start did not finish")
			}
			select {
			case mutationErr := <-mutationResult:
				if ErrorCode(mutationErr) != ErrorCodeConnectionInUse {
					t.Fatalf("concurrent OAuth client mutation error = %v, code = %q", mutationErr, ErrorCode(mutationErr))
				}
			case <-time.After(2 * time.Second):
				t.Fatal("OAuth client mutation did not finish")
			}
			if adapter.authorization.Client.ClientID != "original-client" {
				t.Fatalf("authorization client id = %q, want original client", adapter.authorization.Client.ClientID)
			}
			callback, err := flowService.Callback(context.Background(), OAuthCallbackRequest{
				State:                adapter.authorization.State,
				BrowserBindingDigest: browserBindingDigest,
				Code:                 "one-time-code",
			})
			if err != nil {
				t.Fatalf("Callback() after concurrent %s error = %v", test.name, err)
			}
			if callback.Status != OAuthFlowSucceeded || callback.FlowID != started.FlowID {
				t.Fatalf("Callback() after concurrent %s = %#v", test.name, callback)
			}
		})
	}
}

func TestOAuthFlowCallbackCommitSerializesWithOAuthClientDelete(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	organizationID, accountID := uuid.New(), uuid.New()
	configRepository := &memoryOAuthClientConfigRepository{}
	flowRepository := newMemoryOAuthFlowRepository()
	connectionRepository := newMemoryConnectionRepository()
	locker := &serialOAuthClientFlowTestLocker{attempts: make(chan struct{}, 8)}
	clients := NewOAuthClientConfigService(configRepository, cipher, registry, nil).
		WithConnectionRepository(connectionRepository).
		WithFlowImpactRepository(flowRepository).
		WithOAuthClientFlowLocker(locker)
	if _, err := clients.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID,
		IntegrationID:  "fake",
		DriverID:       "fake-oauth",
		AuthMethodID:   "user_oauth",
		ClientID:       "original-client",
		ClientSecret:   "original-secret",
	}); err != nil {
		t.Fatal(err)
	}
	waitOAuthClientFlowLockAttempt(t, locker.attempts)

	blockingCommitter := &blockingOAuthConnectionCommitter{
		delegate: &memoryOAuthConnectionCommitter{
			flows: flowRepository, connections: connectionRepository,
		},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	browserBindingDigest := testOAuthBrowserBindingDigest(t, 43)
	flowService := NewOAuthFlowService(
		flowRepository,
		NewOAuthStateService(&memoryOAuthStateRepository{}, cipher, 5*time.Minute).
			WithAllowedRedirectURIs([]string{"https://app.example.com/api/integrations/oauth/callback"}),
		registry,
		clients,
		connectionRepository,
		cipher,
	).
		WithOAuthClientFlowLocker(locker).
		WithConnectionCommitter(blockingCommitter).
		WithCallbackAuthorizer(OAuthCallbackAuthorizerFunc(
			func(context.Context, OAuthCallbackAuthorizationRequest) error { return nil },
		))
	withTestOAuthFlowRecovery(flowService)

	if _, err := flowService.Start(context.Background(), OAuthFlowStartRequest{
		OrganizationID:       organizationID,
		AccountID:            accountID,
		IntegrationID:        "fake",
		AuthMethodID:         "user_oauth",
		BrowserBindingDigest: browserBindingDigest,
		CredentialSource:     ConnectionCredentialSourceAccount,
		Intent:               OAuthFlowIntentConnect,
		ConnectionName:       "Serialized callback",
		RequestedActionIDs:   []string{"fake.account.read"},
		RedirectURI:          "https://app.example.com/api/integrations/oauth/callback",
	}); err != nil {
		t.Fatal(err)
	}
	waitOAuthClientFlowLockAttempt(t, locker.attempts)

	callbackResult := make(chan error, 1)
	go func() {
		_, callbackErr := flowService.Callback(context.Background(), OAuthCallbackRequest{
			State:                adapter.authorization.State,
			BrowserBindingDigest: browserBindingDigest,
			Code:                 "one-time-code",
		})
		callbackResult <- callbackErr
	}()
	waitOAuthClientFlowLockAttempt(t, locker.attempts)
	select {
	case <-blockingCommitter.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("OAuth callback did not enter its locked local commit")
	}

	deleteResult := make(chan error, 1)
	go func() {
		deleteResult <- clients.Delete(context.Background(), OAuthClientResolveRequest{
			OrganizationID: organizationID,
			IntegrationID:  "fake",
			DriverID:       "fake-oauth",
			AuthMethodID:   "user_oauth",
		})
	}()
	waitOAuthClientFlowLockAttempt(t, locker.attempts)
	select {
	case deleteErr := <-deleteResult:
		t.Fatalf("OAuth client delete passed the callback commit lock: %v", deleteErr)
	default:
	}

	close(blockingCommitter.release)
	select {
	case callbackErr := <-callbackResult:
		if callbackErr != nil {
			t.Fatalf("Callback() error = %v", callbackErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OAuth callback did not finish")
	}
	select {
	case deleteErr := <-deleteResult:
		if ErrorCode(deleteErr) != ErrorCodeConnectionInUse {
			t.Fatalf("Delete() after callback transition error = %v, code = %q", deleteErr, ErrorCode(deleteErr))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OAuth client delete did not finish")
	}
	connections, err := connectionRepository.List(context.Background(), organizationID, ConnectionListFilter{})
	if err != nil || len(connections) != 1 {
		t.Fatalf("committed connections = %#v, %v", connections, err)
	}
}

func waitOAuthClientFlowLockAttempt(t *testing.T, attempts <-chan struct{}) {
	t.Helper()
	select {
	case <-attempts:
	case <-time.After(2 * time.Second):
		t.Fatal("OAuth client-flow lock was not attempted")
	}
}

func TestOAuthClientConfigChangeAllowedAfterFlowIsTerminalOrExpired(t *testing.T) {
	t.Run("terminal", func(t *testing.T) {
		fixture := newPendingOAuthClientConfigFixture(t, nil)
		if err := fixture.flowRepository.Transition(
			context.Background(),
			fixture.flowID,
			OAuthFlowPending,
			OAuthFlowCancelled,
			map[string]any{"completed_at": time.Now().UTC()},
		); err != nil {
			t.Fatal(err)
		}
		request := fixture.putRequest()
		request.ClientSecret = "replacement-secret"
		if _, err := fixture.service.Put(context.Background(), request); err != nil {
			t.Fatalf("Put() after terminal flow error = %v", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		fixture := newPendingOAuthClientConfigFixture(t, nil)
		fixture.flowRepository.mu.Lock()
		fixture.flowRepository.flows[fixture.flowID].ExpiresAt = time.Now().UTC().Add(-time.Second)
		fixture.flowRepository.mu.Unlock()
		request := fixture.putRequest()
		request.Config = map[string]any{"tenant_domain": "other.example.com"}
		if _, err := fixture.service.Put(context.Background(), request); err != nil {
			t.Fatalf("Put() after expired flow error = %v", err)
		}
	})
}

func TestOAuthClientConfigDeleteFallsBackToDeploymentAfterPendingFlowFinishes(t *testing.T) {
	fixture := newPendingOAuthClientConfigFixture(t, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth",
		ClientID: "deployment-client", ClientSecret: "deployment-secret",
		Config: map[string]any{"tenant_domain": "deployment.example.com"},
	}})
	request := OAuthClientResolveRequest{
		OrganizationID: fixture.organizationID, IntegrationID: "fake",
		DriverID: "fake-oauth", AuthMethodID: "user_oauth",
	}
	if err := fixture.service.Delete(context.Background(), request); ErrorCode(err) != ErrorCodeConnectionInUse {
		t.Fatalf("Delete() during pending flow error = %v, code = %q", err, ErrorCode(err))
	}
	if err := fixture.flowRepository.Transition(
		context.Background(),
		fixture.flowID,
		OAuthFlowPending,
		OAuthFlowCancelled,
		map[string]any{"completed_at": time.Now().UTC()},
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.Delete(context.Background(), request); err != nil {
		t.Fatalf("Delete() after terminal flow error = %v", err)
	}
	resolved, err := fixture.service.ResolveOAuthClient(context.Background(), request)
	if err != nil {
		t.Fatalf("ResolveOAuthClient() deployment fallback error = %v", err)
	}
	defer resolved.Destroy()
	if resolved.Source != OAuthClientSourceDeployment ||
		resolved.ClientID != "deployment-client" ||
		resolved.ClientSecret != "deployment-secret" ||
		resolved.Config["tenant_domain"] != "deployment.example.com" {
		t.Fatalf("ResolveOAuthClient() deployment fallback = %#v", resolved)
	}
}

type serialOAuthRefreshLocker struct{ mu sync.Mutex }
type serialOAuthRefreshLease struct{ mu *sync.Mutex }

func (locker *serialOAuthRefreshLocker) Acquire(context.Context, string, time.Duration) (OAuthRefreshLease, error) {
	locker.mu.Lock()
	return &serialOAuthRefreshLease{mu: &locker.mu}, nil
}
func (lease *serialOAuthRefreshLease) Release(context.Context) error {
	lease.mu.Unlock()
	return nil
}

func TestOAuthRefreshingResolverFailsClosedWhenRefreshTokenExpired(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	repository := newMemoryConnectionRepository()
	now := time.Now().UTC()
	organizationID, accountID, connectionID := uuid.New(), uuid.New(), uuid.New()
	accessExpiry := now.Add(30 * time.Minute)
	refreshExpiry := now.Add(-time.Minute)
	connection := &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth",
		Name: "Expired refresh token", CredentialSource: ConnectionCredentialSourceAccount, AuthType: ConnectionAuthTypeOAuth2,
		AuthMethodID: "user_oauth", OwnerAccountID: &accountID, Config: map[string]any{},
		GrantedScopes: []string{"account.read"}, Status: ConnectionStatusActive, AuthStatus: ConnectionAuthValid,
		ScopeStatus: ConnectionScopeVerified, HealthStatus: ConnectionHealthHealthy,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1,
		TokenExpiresAt: &accessExpiry, RefreshTokenExpiresAt: &refreshExpiry,
	}
	envelope, _ := cipher.EncryptCredentials(map[string]string{
		"access_token": "still-valid-access", "refresh_token": "expired-refresh", "token_type": "Bearer",
	}, CredentialAAD{OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 1})
	connection.EncryptedCredentials = &envelope
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	clients := NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
	}})
	resolver := NewOAuthRefreshingConnectionResolver(
		NewConnectionResolver(repository, cipher),
		repository,
		cipher,
		registry,
		clients,
		&serialOAuthRefreshLocker{},
		5*time.Minute,
	)
	resolver.now = func() time.Time { return now }

	resolved, err := resolver.Resolve(context.Background(), ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: "fake",
		DriverID: "fake-oauth", ConnectionID: connectionID.String(),
	})
	if resolved != nil || ErrorCode(err) != ErrorCodeReconnectRequired {
		t.Fatalf("Resolve() = %#v, %v; want reconnect required", resolved, err)
	}
	adapter.mu.Lock()
	refreshCalls := adapter.refreshCalls
	adapter.mu.Unlock()
	if refreshCalls != 0 {
		t.Fatalf("provider refresh calls = %d, want 0", refreshCalls)
	}
	stored := repository.stored(connectionID)
	if stored == nil || stored.AuthStatus != ConnectionAuthReconnectRequired ||
		stored.HealthStatus != ConnectionHealthDegraded ||
		stored.AttentionCode == nil || *stored.AttentionCode != ConnectionAttentionReconnectRequired ||
		stored.NextTokenRefreshAt != nil {
		t.Fatalf("expired refresh token health = %#v", stored)
	}
}

func TestOAuthRefreshingResolverPreservesKnownRefreshTokenExpiryWhenProviderOmitsIt(t *testing.T) {
	adapter := &fakeOAuthAdapter{omitRefreshTokenExpiry: true}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	repository := newMemoryConnectionRepository()
	now := time.Now().UTC()
	organizationID, accountID, connectionID := uuid.New(), uuid.New(), uuid.New()
	accessExpiry := now.Add(time.Minute)
	refreshExpiry := now.Add(2 * time.Hour)
	connection := &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth",
		Name: "Known refresh expiry", CredentialSource: ConnectionCredentialSourceAccount, AuthType: ConnectionAuthTypeOAuth2,
		AuthMethodID: "user_oauth", OwnerAccountID: &accountID, Config: map[string]any{},
		GrantedScopes: []string{"account.read"}, Status: ConnectionStatusActive, AuthStatus: ConnectionAuthValid,
		ScopeStatus: ConnectionScopeVerified, HealthStatus: ConnectionHealthHealthy,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1,
		TokenExpiresAt: &accessExpiry, RefreshTokenExpiresAt: &refreshExpiry,
	}
	envelope, _ := cipher.EncryptCredentials(map[string]string{
		"access_token": "old-access", "refresh_token": "old-refresh", "token_type": "Bearer",
	}, CredentialAAD{OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 1})
	connection.EncryptedCredentials = &envelope
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	clients := NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
	}})
	resolver := NewOAuthRefreshingConnectionResolver(
		NewConnectionResolver(repository, cipher),
		repository,
		cipher,
		registry,
		clients,
		&serialOAuthRefreshLocker{},
		5*time.Minute,
	)
	resolver.now = func() time.Time { return now }

	resolved, err := resolver.Resolve(context.Background(), ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: "fake",
		DriverID: "fake-oauth", ConnectionID: connectionID.String(),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	resolved.Destroy()
	stored := repository.stored(connectionID)
	if stored == nil || stored.RefreshTokenExpiresAt == nil ||
		!stored.RefreshTokenExpiresAt.Equal(refreshExpiry) {
		t.Fatalf("preserved refresh token expiry = %#v", stored)
	}
}

func TestOAuthRefreshingResolverSerializesRotatingRefreshToken(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	repository := newMemoryConnectionRepository()
	organizationID, accountID, connectionID := uuid.New(), uuid.New(), uuid.New()
	expiry := time.Now().UTC().Add(time.Minute)
	connection := &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth",
		Name: "OAuth", CredentialSource: ConnectionCredentialSourceAccount, AuthType: ConnectionAuthTypeOAuth2,
		AuthMethodID: "user_oauth", OwnerAccountID: &accountID, Config: map[string]any{},
		GrantedScopes: []string{"account.read"}, Status: ConnectionStatusActive, AuthStatus: ConnectionAuthValid,
		ScopeStatus: ConnectionScopeVerified, HealthStatus: ConnectionHealthHealthy,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1, TokenExpiresAt: &expiry,
	}
	envelope, _ := cipher.EncryptCredentials(map[string]string{
		"access_token": "old-access", "refresh_token": "single-use-refresh", "token_type": "Bearer",
	}, CredentialAAD{OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 1})
	connection.EncryptedCredentials = &envelope
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	clients := NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
	}})
	base := NewConnectionResolver(repository, cipher)
	resolver := NewOAuthRefreshingConnectionResolver(base, repository, cipher, registry, clients, &serialOAuthRefreshLocker{}, 5*time.Minute)
	request := ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: "fake", DriverID: "fake-oauth", ConnectionID: connectionID.String(),
	}
	var wait sync.WaitGroup
	errorsChannel := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			resolved, err := resolver.Resolve(context.Background(), request)
			if resolved != nil {
				resolved.Destroy()
			}
			errorsChannel <- err
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
	}
	adapter.mu.Lock()
	refreshCalls := adapter.refreshCalls
	adapter.mu.Unlock()
	if refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls)
	}
	stored := repository.stored(connectionID)
	if stored.CredentialVersion != 2 || stored.TokenExpiresAt == nil ||
		!stored.TokenExpiresAt.After(time.Now().UTC().Add(30*time.Minute)) ||
		stored.RefreshTokenExpiresAt == nil ||
		!stored.RefreshTokenExpiresAt.After(time.Now().UTC().Add(20*time.Hour)) {
		t.Fatalf("stored refreshed connection = %#v", stored)
	}
	credentials, err := cipher.DecryptCredentials(*stored.EncryptedCredentials, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if credentials["refresh_token"] != "rotated-refresh" {
		t.Fatalf("rotated refresh token was not persisted")
	}
	destroyCredentialMap(credentials)
}

func TestOAuthRefreshingResolverRetriesRotatingTokenPersistence(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	repository := newMemoryConnectionRepository()
	organizationID, accountID, connectionID := uuid.New(), uuid.New(), uuid.New()
	expiry := time.Now().UTC().Add(time.Minute)
	connection := &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth",
		Name: "Rotating OAuth", CredentialSource: ConnectionCredentialSourceAccount, AuthType: ConnectionAuthTypeOAuth2,
		AuthMethodID: "user_oauth", OwnerAccountID: &accountID, Config: map[string]any{},
		GrantedScopes: []string{"account.read"}, Status: ConnectionStatusActive, AuthStatus: ConnectionAuthValid,
		ScopeStatus: ConnectionScopeVerified, HealthStatus: ConnectionHealthHealthy,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1, TokenExpiresAt: &expiry,
	}
	envelope, _ := cipher.EncryptCredentials(map[string]string{
		"access_token": "old-access", "refresh_token": "single-use-refresh", "token_type": "Bearer",
	}, CredentialAAD{OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 1})
	connection.EncryptedCredentials = &envelope
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	repository.oauthUpdateFailures = 1
	repository.oauthUpdateErr = errors.New("temporary database interruption")
	clients := NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
	}})
	base := NewConnectionResolver(repository, cipher)
	resolver := NewOAuthRefreshingConnectionResolver(base, repository, cipher, registry, clients, &serialOAuthRefreshLocker{}, 5*time.Minute)

	resolved, err := resolver.Resolve(context.Background(), ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: "fake", DriverID: "fake-oauth", ConnectionID: connectionID.String(),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	resolved.Destroy()
	if repository.oauthUpdateCalls != 2 {
		t.Fatalf("OAuth credential update calls = %d, want one retry", repository.oauthUpdateCalls)
	}
	adapter.mu.Lock()
	refreshCalls := adapter.refreshCalls
	adapter.mu.Unlock()
	if refreshCalls != 1 {
		t.Fatalf("provider refresh calls = %d, want 1", refreshCalls)
	}
	stored := repository.stored(connectionID)
	if stored == nil || stored.CredentialVersion != 2 {
		t.Fatalf("stored refreshed connection = %#v", stored)
	}
	credentials, err := cipher.DecryptCredentials(*stored.EncryptedCredentials, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer destroyCredentialMap(credentials)
	if credentials["refresh_token"] != "rotated-refresh" {
		t.Fatalf("rotated refresh token was not persisted after retry")
	}
}
