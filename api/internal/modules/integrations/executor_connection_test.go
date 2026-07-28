package integrations

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type executorConnectionResolverFunc func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error)

func (fn executorConnectionResolverFunc) Resolve(ctx context.Context, request ConnectionResolveRequest) (*ResolvedConnection, error) {
	return fn(ctx, request)
}

type executorPolicyResolverFunc func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error)

func (fn executorPolicyResolverFunc) Resolve(ctx context.Context, organizationID, integrationID string, action ActionDefinition) (ActionPolicyDecision, error) {
	return fn(ctx, organizationID, integrationID, action)
}

type executorAccessAuthorizerFunc func(context.Context, ConnectionAccessRequest) error

func (fn executorAccessAuthorizerFunc) AuthorizeConnectionUse(ctx context.Context, request ConnectionAccessRequest) error {
	return fn(ctx, request)
}

func (fn executorAccessAuthorizerFunc) AuthorizeAgentConnectionUse(ctx context.Context, request ConnectionAccessRequest) error {
	return fn(ctx, request)
}

func allowExecutorConnectionAccess() ConnectionAccessAuthorizer {
	return executorAccessAuthorizerFunc(func(context.Context, ConnectionAccessRequest) error { return nil })
}

func TestExecutorResolvesConnectionForOneRequestAndDestroysSecrets(t *testing.T) {
	connection := &ResolvedConnection{
		ID:             testConnectionID,
		OrganizationID: testOrganizationID,
		IntegrationID:  IntegrationWebSearch,
		DriverID:       "test-driver",
		Credentials:    map[string]string{"api_key": "request-secret"},
		Config:         map[string]interface{}{"region": "global"},
	}
	adapter := &testAdapter{driverID: "test-driver", execute: func(_ context.Context, request ActionRequest) (*ActionResult, error) {
		if request.Connection != connection || request.Connection.Credentials["api_key"] != "request-secret" {
			t.Fatalf("adapter connection = %#v", request.Connection)
		}
		return &ActionResult{Output: map[string]interface{}{"ok": true}, AttemptCount: 1}, nil
	}}
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, &testQuota{}, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(_ context.Context, request ConnectionResolveRequest) (*ResolvedConnection, error) {
			if request.ConnectionID != testConnectionID || request.OrganizationID != testOrganizationID || request.DriverID != "test-driver" {
				t.Fatalf("resolve request = %#v", request)
			}
			return connection, nil
		})).
		WithConnectionAccessAuthorizer(executorAccessAuthorizerFunc(func(_ context.Context, request ConnectionAccessRequest) error {
			if request.OrganizationID.String() != testOrganizationID || request.WorkspaceID == nil || request.WorkspaceID.String() != testWorkspaceID || request.AccountID.String() != testUserID || request.ConnectionID.String() != testConnectionID || request.IntegrationID != IntegrationWebSearch || request.ActionID != ActionWebSearch {
				t.Fatalf("access request = %#v", request)
			}
			return nil
		}))

	if _, err := executor.Execute(context.Background(), validActionRequest("current events")); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(connection.Credentials) != 0 || len(connection.Config) != 0 {
		t.Fatalf("resolved secrets survived invocation: %#v %#v", connection.Credentials, connection.Config)
	}
	if audit.created == nil || audit.created.ConnectionID == nil || audit.created.ConnectionID.String() != testConnectionID {
		t.Fatalf("audit connection = %#v", audit.created)
	}
}

func TestExecutorExplicitConnectionResolutionFailureNeverCallsAdapterOrFallback(t *testing.T) {
	adapter := &testAdapter{driverID: "test-driver"}
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
	resolverCalls := 0
	executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(_ context.Context, request ConnectionResolveRequest) (*ResolvedConnection, error) {
			resolverCalls++
			if request.ConnectionID == "" {
				t.Fatal("explicit connection was silently converted to default resolution")
			}
			return nil, NewError(ErrorCodeConnectionNotFound, "connection not found", nil)
		})).
		WithConnectionAccessAuthorizer(allowExecutorConnectionAccess())

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeConnectionNotFound {
		t.Fatalf("Execute() result=%#v error=%v code=%s", result, err, ErrorCode(err))
	}
	if resolverCalls != 1 || adapter.calls != 0 {
		t.Fatalf("resolver calls=%d adapter calls=%d", resolverCalls, adapter.calls)
	}
}

func TestExecutorAIChatConnectionACLRejectsBeforeCredentialResolution(t *testing.T) {
	adapter := &testAdapter{driverID: "test-driver"}
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
	resolverCalls := 0
	authorizerCalls := 0
	executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
			resolverCalls++
			return nil, errors.New("credentials must not be decrypted before connection ACL authorization")
		})).
		WithConnectionAccessAuthorizer(executorAccessAuthorizerFunc(func(_ context.Context, request ConnectionAccessRequest) error {
			authorizerCalls++
			if request.ConnectionID != uuid.MustParse(testConnectionID) || request.AccountID != uuid.MustParse(testUserID) || request.ActionID != ActionWebSearch {
				t.Fatalf("access request = %#v", request)
			}
			return NewError(ErrorCodeAccessDenied, "grant revoked", nil)
		}))

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("Execute() result=%#v error=%v", result, err)
	}
	if authorizerCalls != 1 || resolverCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls: authorizer=%d resolver=%d adapter=%d", authorizerCalls, resolverCalls, adapter.calls)
	}
}

func TestExecutorResourceConstrainedGrantFailsClosedWithoutResourceExtraction(t *testing.T) {
	adapter := &testAdapter{driverID: "test-driver"}
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
	organizationID := uuid.MustParse(testOrganizationID)
	connectionID := uuid.MustParse(testConnectionID)
	accountID := uuid.MustParse(testUserID)
	workspaceID := uuid.MustParse(testWorkspaceID)
	connections := newMemoryConnectionRepository()
	if err := connections.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: "test-driver", Name: "Scoped",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{{
		ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID,
		AccessMode: ConnectionGrantAccessRead, AllowedActionIDs: []string{ActionWebSearch},
		ResourceConstraints: map[string]any{"resource_ids": []string{"repo-a"}},
	}}}
	resolverCalls := 0
	executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
			resolverCalls++
			return nil, errors.New("resource-constrained authorization must run before credentials")
		})).
		WithConnectionAccessAuthorizer(NewConnectionAccessService(connections, grants))

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("Execute() result=%#v error=%v", result, err)
	}
	if resolverCalls != 0 || adapter.calls != 0 {
		t.Fatalf("resource-constrained denial reached credentials/provider: resolver=%d adapter=%d account=%s", resolverCalls, adapter.calls, accountID)
	}
}

func TestExecutorRequiredScopesFailBeforeQuotaAuditAndProvider(t *testing.T) {
	action := testAction(ActionWebSearch, "search_web")
	action.RequiredScopes = []string{"issues:write", "repo:read"}
	adapter := &testAdapter{driverID: "test-driver"}
	registry := registerTestAction(t, action, adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	connection := &ResolvedConnection{
		ID: testConnectionID, OrganizationID: testOrganizationID, IntegrationID: IntegrationWebSearch,
		DriverID: "test-driver", AuthType: ConnectionAuthTypeOAuth2, GrantedScopes: []string{"repo:read"}, Credentials: map[string]string{"token": "secret"},
	}
	executor := NewExecutor(registry, audit, quota, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
			return connection, nil
		})).
		WithConnectionAccessAuthorizer(allowExecutorConnectionAccess())

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeInsufficientScope {
		t.Fatalf("Execute() result=%#v error=%v code=%s", result, err, ErrorCode(err))
	}
	if quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("scope-denied invocation reached side effects: quota=%d audit=%d adapter=%d", quota.calls, audit.createCalls, adapter.calls)
	}
	if connection.Credentials != nil {
		t.Fatalf("request-scoped credentials were not destroyed: %#v", connection.Credentials)
	}
}

func TestExecutorRequiresAllScopesAndOneAlternative(t *testing.T) {
	action := testAction(ActionWebSearch, "search_web")
	action.RequiredScopes = []string{"repo:read"}
	action.RequiredAnyScopes = []string{"issues:read", "pulls:read"}
	action.PreferredScopes = []string{"issues:read"}

	for _, test := range []struct {
		name      string
		granted   []string
		wantCode  string
		wantCalls int
	}{
		{name: "preferred alternative", granted: []string{"repo:read", "issues:read"}, wantCalls: 1},
		{name: "non-preferred alternative", granted: []string{"repo:read", "pulls:read"}, wantCalls: 1},
		{name: "missing all-of", granted: []string{"issues:read"}, wantCode: ErrorCodeInsufficientScope},
		{name: "missing any-of", granted: []string{"repo:read"}, wantCode: ErrorCodeInsufficientScope},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := &testAdapter{driverID: "test-driver"}
			registry := registerTestAction(t, action, adapter)
			connection := &ResolvedConnection{
				ID: testConnectionID, OrganizationID: testOrganizationID, IntegrationID: IntegrationWebSearch,
				DriverID: "test-driver", AuthType: ConnectionAuthTypeOAuth2,
				GrantedScopes: append([]string(nil), test.granted...), Credentials: map[string]string{"token": "secret"},
			}
			executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0).
				WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
					return connection, nil
				})).
				WithConnectionAccessAuthorizer(allowExecutorConnectionAccess())

			result, err := executor.Execute(context.Background(), validActionRequest("current events"))
			if ErrorCode(err) != test.wantCode {
				t.Fatalf("Execute() result=%#v error=%v code=%s, want %s", result, err, ErrorCode(err), test.wantCode)
			}
			if adapter.calls != test.wantCalls {
				t.Fatalf("adapter calls = %d, want %d", adapter.calls, test.wantCalls)
			}
		})
	}
}

func TestExecutorRejectsConnectionAuthMethodMismatchBeforeQuotaAuditAndProvider(t *testing.T) {
	action := testAction(ActionWebSearch, "search_web")
	action.SupportedAuthMethodIDs = []string{"test_api_key"}
	adapter := &testAdapter{driverID: "test-driver"}
	registration := localizedTestRegistration(IntegrationWebSearch, adapter, []ActionDefinition{action})
	registration.Definition.AuthMethods = append(registration.Definition.AuthMethods, AuthMethodDefinition{
		ID:               "test_api_key",
		Type:             AuthMethodTypeAPIKey,
		CredentialSource: ConnectionCredentialSourceOrganization,
		Label:            "Test API key",
		LabelI18n: LocalizedText{
			LocaleEnglishUS:         "Test API key",
			LocaleSimplifiedChinese: "测试 API 密钥",
		},
		Available: true,
		Fields: []CredentialFieldDefinition{{
			Key:   "api_key",
			Label: "API key",
			LabelI18n: LocalizedText{
				LocaleEnglishUS:         "API key",
				LocaleSimplifiedChinese: "API 密钥",
			},
			Input:    CredentialFieldInputPassword,
			Required: true,
			Secret:   true,
		}},
	})
	registry := NewRegistry()
	if err := registry.Register(registration); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	quota := &testQuota{}
	audit := &testAudit{}
	connection := &ResolvedConnection{
		ID: testConnectionID, OrganizationID: testOrganizationID, IntegrationID: IntegrationWebSearch,
		DriverID: "test-driver", AuthMethodID: "tenant_app",
		Credentials: map[string]string{"access_token": "secret"},
	}
	executor := NewExecutor(registry, audit, quota, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
			return connection, nil
		})).
		WithConnectionAccessAuthorizer(allowExecutorConnectionAccess())

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("Execute() result=%#v error=%v code=%s", result, err, ErrorCode(err))
	}
	if quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("auth-method-denied invocation reached side effects: quota=%d audit=%d adapter=%d", quota.calls, audit.createCalls, adapter.calls)
	}
	if connection.Credentials != nil {
		t.Fatalf("request-scoped credentials were not destroyed: %#v", connection.Credentials)
	}
}

func TestExecutorOrganizationPolicyBlocksBeforeQuotaAndAudit(t *testing.T) {
	adapter := &testAdapter{driverID: "test-driver"}
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	resolverCalls := 0
	executor := NewExecutor(registry, audit, quota, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
			resolverCalls++
			return nil, errors.New("credentials must not be resolved for a disabled action")
		})).
		WithActionPolicyResolver(executorPolicyResolverFunc(func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{Enabled: false, DataEgressAllowed: true}, nil
		}))

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeDisabled {
		t.Fatalf("Execute() result=%#v error=%v", result, err)
	}
	if resolverCalls != 0 || quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("blocked call reached credentials or side effects: resolver=%d quota=%d audit=%d adapter=%d", resolverCalls, quota.calls, audit.createCalls, adapter.calls)
	}
}

func TestExecutorAgentExplicitConnectionRequiresCurrentActionAuthorization(t *testing.T) {
	adapter := &testAdapter{driverID: "test-driver", execute: func(context.Context, ActionRequest) (*ActionResult, error) {
		return &ActionResult{Output: map[string]interface{}{"ok": true}}, nil
	}}
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
	resolverCalls := 0
	executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
			resolverCalls++
			return nil, errors.New("credentials must not be resolved before Agent authorization")
		}))
	req := validActionRequest("current events")
	req.InvokeFrom = tools.ToolInvokeFromAgent
	req.AgentID = "55555555-5555-4555-8555-555555555555"
	req.VerifyAgentConnection = func(_ context.Context, authorization AgentConnectionAuthorizationRequest) (bool, error) {
		if authorization.ConnectionID != testConnectionID || authorization.ActionID != ActionWebSearch || authorization.AgentID != req.AgentID {
			t.Fatalf("authorization request = %#v", authorization)
		}
		return false, errors.New("action removed from binding")
	}

	result, err := executor.Execute(context.Background(), req)
	if result != nil || ErrorCode(err) != ErrorCodeAccessDenied || resolverCalls != 0 || adapter.calls != 0 {
		t.Fatalf("Execute() result=%#v error=%v resolver calls=%d adapter calls=%d", result, err, resolverCalls, adapter.calls)
	}
}

func TestExecutorAgentRechecksSharedGrantOnEveryInvocation(t *testing.T) {
	tests := []struct {
		name   string
		revoke func(*memoryConnectionGrantRepository)
	}{
		{name: "grant revoked", revoke: func(repository *memoryConnectionGrantRepository) {
			repository.grants = nil
		}},
		{name: "action narrowed", revoke: func(repository *memoryConnectionGrantRepository) {
			repository.grants[0].AllowedActionIDs = []string{ActionWebFetch}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			organizationID := uuid.MustParse(testOrganizationID)
			workspaceID := uuid.MustParse(testWorkspaceID)
			accountID := uuid.MustParse(testUserID)
			connectionID := uuid.MustParse(testConnectionID)
			connections := newMemoryConnectionRepository()
			if err := connections.Create(t.Context(), &IntegrationConnection{
				ID: connectionID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: "test-driver", Name: "Shared",
				CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
				Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
			}); err != nil {
				t.Fatal(err)
			}
			grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{{
				ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
				PrincipalType: ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID,
				AccessMode: ConnectionGrantAccessRead, AllowedActionIDs: []string{ActionWebSearch}, ResourceConstraints: map[string]any{},
			}}}
			adapter := &testAdapter{driverID: "test-driver", execute: func(context.Context, ActionRequest) (*ActionResult, error) {
				return &ActionResult{Output: map[string]interface{}{"ok": true}}, nil
			}}
			registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
			resolverCalls := 0
			executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0).
				WithConnectionResolver(executorConnectionResolverFunc(func(_ context.Context, request ConnectionResolveRequest) (*ResolvedConnection, error) {
					resolverCalls++
					if !request.DisallowAccountCredential {
						t.Fatal("Agent credential resolution did not enable the personal-credential guard")
					}
					return &ResolvedConnection{
						ID: connectionID.String(), OrganizationID: organizationID.String(), IntegrationID: IntegrationWebSearch,
						DriverID: "test-driver", CredentialSource: ConnectionCredentialSourceOrganization, Credentials: map[string]string{"token": "request-scoped"},
					}, nil
				})).
				WithConnectionAccessAuthorizer(NewConnectionAccessService(connections, grants))
			request := validActionRequest("current events")
			request.InvokeFrom = tools.ToolInvokeFromAgent
			request.AgentID = "55555555-5555-4555-8555-555555555555"
			request.UserID = accountID.String()
			request.VerifyAgentConnection = func(context.Context, AgentConnectionAuthorizationRequest) (bool, error) {
				return true, nil
			}
			if _, err := executor.Execute(t.Context(), request); err != nil {
				t.Fatalf("initial authorized invocation error = %v", err)
			}
			if resolverCalls != 1 || adapter.calls != 1 {
				t.Fatalf("initial invocation calls: resolver=%d adapter=%d", resolverCalls, adapter.calls)
			}

			tt.revoke(grants)
			result, err := executor.Execute(t.Context(), request)
			if result != nil || ErrorCode(err) != ErrorCodeAccessDenied {
				t.Fatalf("invocation after grant change result=%#v error=%v", result, err)
			}
			if resolverCalls != 1 || adapter.calls != 1 {
				t.Fatalf("grant denial reached credentials/provider: resolver=%d adapter=%d", resolverCalls, adapter.calls)
			}
		})
	}
}

func TestExecutorAgentRejectsPersistedPersonalBindingBeforeCredentialResolution(t *testing.T) {
	organizationID := uuid.MustParse(testOrganizationID)
	accountID := uuid.MustParse(testUserID)
	connectionID := uuid.MustParse(testConnectionID)
	connections := newMemoryConnectionRepository()
	if err := connections.Create(t.Context(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: "test-driver", Name: "Personal",
		CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &accountID, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{{
		ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalOrganization,
		AccessMode:    ConnectionGrantAccessWrite, AllowedActionIDs: []string{"*"}, ResourceConstraints: map[string]any{},
	}}}
	adapter := &testAdapter{driverID: "test-driver"}
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
	resolverCalls := 0
	executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
			resolverCalls++
			return nil, errors.New("personal credentials must not be resolved for Agent execution")
		})).
		WithConnectionAccessAuthorizer(NewConnectionAccessService(connections, grants))
	request := validActionRequest("current events")
	request.InvokeFrom = tools.ToolInvokeFromAgent
	request.AgentID = "55555555-5555-4555-8555-555555555555"
	request.VerifyAgentConnection = func(context.Context, AgentConnectionAuthorizationRequest) (bool, error) {
		return true, nil
	}
	result, err := executor.Execute(t.Context(), request)
	if result != nil || ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("personal Agent invocation result=%#v error=%v", result, err)
	}
	if resolverCalls != 0 || adapter.calls != 0 {
		t.Fatalf("personal Agent invocation reached credentials/provider: resolver=%d adapter=%d", resolverCalls, adapter.calls)
	}
}

func TestExecutorAgentWithoutExplicitConnectionFailsClosedBeforeCredentialResolution(t *testing.T) {
	adapter := &testAdapter{driverID: "test-driver"}
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), adapter)
	resolverCalls := 0
	executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0).
		WithConnectionResolver(executorConnectionResolverFunc(func(context.Context, ConnectionResolveRequest) (*ResolvedConnection, error) {
			resolverCalls++
			return nil, errors.New("Agent without a binding must not resolve default credentials")
		}))
	req := validActionRequest("current events")
	req.InvokeFrom = tools.ToolInvokeFromAgent
	req.AgentID = "55555555-5555-4555-8555-555555555555"
	req.ConnectionID = ""

	result, err := executor.Execute(context.Background(), req)
	if result != nil || ErrorCode(err) != ErrorCodeAccessDenied || resolverCalls != 0 || adapter.calls != 0 {
		t.Fatalf("Execute() result=%#v error=%v resolver calls=%d adapter calls=%d", result, err, resolverCalls, adapter.calls)
	}
}
