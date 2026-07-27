package integrations

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type recordingConnectionTester struct {
	apiKey  string
	calls   int
	profile *ConnectionProfile
	err     error
	before  func()
}

type recordingConnectionRevoker struct {
	calls  int
	err    error
	events *[]string
}

func (revoker *recordingConnectionRevoker) RevokeConnection(_ context.Context, _ *IntegrationConnection) error {
	revoker.calls++
	if revoker.events != nil {
		*revoker.events = append(*revoker.events, "revoke")
	}
	return revoker.err
}

func (tester *recordingConnectionTester) ValidateConnection(_ context.Context, connection *ResolvedConnection) (*ConnectionProfile, error) {
	tester.calls++
	if connection != nil {
		tester.apiKey = connection.Credentials["api_key"]
	}
	if tester.before != nil {
		tester.before()
	}
	return tester.profile, tester.err
}

func newConnectionServiceForTest(t *testing.T, repository *memoryConnectionRepository, tester ConnectionTester) (*DefaultConnectionService, CredentialCipher) {
	t.Helper()
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	catalog := staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{{ID: ActionWebSearch, Name: "Search", DataEgress: true}}}
	resolver := NewConnectionResolver(repository, cipher)
	return NewConnectionService(repository, cipher, catalog, resolver, tester), cipher
}

func TestConnectionServiceCreatesEncryptedOrganizationConnectionAndTestsIt(t *testing.T) {
	repository := newMemoryConnectionRepository()
	tester := &recordingConnectionTester{profile: &ConnectionProfile{
		AccountID: "exa-account", DisplayName: "Research Key", GrantedScopes: []string{"search", "search", "contents"},
	}}
	service, cipher := newConnectionServiceForTest(t, repository, tester)
	organizationID := uuid.New()
	actorID := uuid.New()
	credentials := map[string]string{"api_key": "organization-exa-key"}
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		Name: "Research", CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Credentials: credentials, Config: map[string]any{"region": "global"}, ActorID: &actorID,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(credentials) != 0 {
		t.Fatalf("Create() retained caller credentials: %#v", credentials)
	}
	if !view.CredentialConfigured || view.Status != ConnectionStatusPending {
		t.Fatalf("created view = %#v", view)
	}
	if tester.calls != 0 {
		t.Fatalf("Create() called provider %d times; connection checks must be manual", tester.calls)
	}
	stored := repository.stored(view.ID)
	if stored.EncryptedCredentials == nil || strings.Contains(*stored.EncryptedCredentials, "organization-exa-key") {
		t.Fatalf("stored credential envelope = %#v", stored.EncryptedCredentials)
	}
	decrypted, err := cipher.DecryptCredentials(*stored.EncryptedCredentials, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: view.ID, IntegrationID: IntegrationWebSearch, CredentialVersion: 1,
	})
	if err != nil || decrypted["api_key"] != "organization-exa-key" {
		t.Fatalf("DecryptCredentials() = %#v, %v", decrypted, err)
	}
	destroyCredentialMap(decrypted)

	tested, profile, err := service.Test(context.Background(), organizationID, view.ID, &actorID)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if tester.apiKey != "organization-exa-key" {
		t.Fatalf("tester api key = %q", tester.apiKey)
	}
	if tester.calls != 1 {
		t.Fatalf("manual Test() provider calls = %d, want 1", tester.calls)
	}
	if profile == nil || tested.Status != ConnectionStatusActive || tested.AccountID == nil || *tested.AccountID != "exa-account" {
		t.Fatalf("tested connection = %#v, profile = %#v", tested, profile)
	}
	if len(tested.GrantedScopes) != 2 {
		t.Fatalf("granted scopes = %#v", tested.GrantedScopes)
	}
	defaulted, err := service.SetDefault(context.Background(), organizationID, view.ID)
	if err != nil || !defaulted.IsDefault {
		t.Fatalf("SetDefault() = %#v, %v", defaulted, err)
	}
}

func TestConnectionServiceSuccessfulTestPreservesConfiguredExpiryWhenProviderOmitsIt(t *testing.T) {
	repository := newMemoryConnectionRepository()
	tester := &recordingConnectionTester{profile: &ConnectionProfile{}}
	service, _ := newConnectionServiceForTest(t, repository, tester)
	organizationID := uuid.New()
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Expiring",
		Credentials: map[string]string{"api_key": "valid-key"}, ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	tested, _, err := service.Test(context.Background(), organizationID, view.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tested.ExpiresAt == nil || !tested.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("provider omitted expiry; got %#v, want %s", tested.ExpiresAt, expiresAt)
	}
}

func TestConnectionServiceCredentialRotationChangesAADVersionAndRemovesDefault(t *testing.T) {
	repository := newMemoryConnectionRepository()
	tester := &recordingConnectionTester{profile: &ConnectionProfile{}}
	service, cipher := newConnectionServiceForTest(t, repository, tester)
	organizationID := uuid.New()
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Default",
		Credentials: map[string]string{"api_key": "first-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Test(context.Background(), organizationID, view.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetDefault(context.Background(), organizationID, view.ID); err != nil {
		t.Fatal(err)
	}
	oldEnvelope := *repository.stored(view.ID).EncryptedCredentials
	replacement := map[string]string{"api_key": "second-key"}
	updated, err := service.Update(context.Background(), UpdateConnectionInput{
		OrganizationID: organizationID, ConnectionID: view.ID, Credentials: replacement,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(replacement) != 0 {
		t.Fatalf("Update() retained replacement credentials: %#v", replacement)
	}
	if updated.CredentialVersion != 2 || updated.Status != ConnectionStatusPending || updated.IsDefault {
		t.Fatalf("rotated view = %#v", updated)
	}
	if tester.calls != 1 {
		t.Fatalf("credential Update() triggered a provider check; calls = %d, want only the explicit test", tester.calls)
	}
	newEnvelope := *repository.stored(view.ID).EncryptedCredentials
	newAAD := CredentialAAD{OrganizationID: organizationID, ConnectionID: view.ID, IntegrationID: IntegrationWebSearch, CredentialVersion: 2}
	credentials, err := cipher.DecryptCredentials(newEnvelope, newAAD)
	if err != nil || credentials["api_key"] != "second-key" {
		t.Fatalf("new envelope decrypt = %#v, %v", credentials, err)
	}
	destroyCredentialMap(credentials)
	if _, err := cipher.DecryptCredentials(oldEnvelope, newAAD); err == nil {
		t.Fatal("old envelope decrypted with new credential version")
	}
}

func TestConnectionServiceRejectsSecretsInConfigAndPlatformStoredCredentials(t *testing.T) {
	repository := newMemoryConnectionRepository()
	service, _ := newConnectionServiceForTest(t, repository, nil)
	organizationID := uuid.New()
	_, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Unsafe",
		Credentials: map[string]string{"api_key": "organization-key"}, Config: map[string]any{"access_token": "do-not-store-here"},
	})
	if err == nil {
		t.Fatal("Create() secret config error = nil")
	}
	_, err = service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Platform",
		CredentialSource: ConnectionCredentialSourcePlatform, AuthType: ConnectionAuthTypePlatform,
		Credentials: map[string]string{"api_key": "must-not-be-stored"},
	})
	if err == nil {
		t.Fatal("Create() platform credentials error = nil")
	}
	if len(repository.connections) != 0 {
		t.Fatalf("unsafe connections were persisted: %#v", repository.connections)
	}
}

func TestConnectionServicePersistsExplicitAuthMethodAndUsesItsCredentialSchema(t *testing.T) {
	repository := newMemoryConnectionRepository()
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	action := testAction("github.issue.list", "list_github_issues")
	action.SupportedAuthMethodIDs = []string{"github_pat", "github_app_token"}
	definition := ProviderDefinition{
		ID: "github", DriverID: "test-driver", Name: "GitHub",
		AuthMethods: []AuthMethodDefinition{
			{ID: "github_pat", Type: AuthMethodTypeAPIKey, CredentialSource: ConnectionCredentialSourceOrganization, Label: "PAT", Available: true, Fields: []CredentialFieldDefinition{{Key: "token", Label: "Token", Input: CredentialFieldInputPassword, Required: true, Secret: true}}},
			{ID: "github_app_token", Type: AuthMethodTypeAPIKey, CredentialSource: ConnectionCredentialSourceOrganization, Label: "App token", Available: true, Fields: []CredentialFieldDefinition{{Key: "app_token", Label: "App token", Input: CredentialFieldInputPassword, Required: true, Secret: true}}},
			{ID: "github_future_oauth", Type: AuthMethodTypeOAuth2, CredentialSource: ConnectionCredentialSourceOrganization, Label: "OAuth (coming soon)", Available: false},
		},
		Actions: []ActionDefinition{action},
	}
	localizeTestProviderFixture(&definition)
	if err := registry.Register(Registration{
		Definition: definition,
		Adapter:    &testAdapter{driverID: "test-driver"},
	}); err != nil {
		t.Fatal(err)
	}
	service := NewConnectionService(repository, cipher, registry, NewConnectionResolver(repository, cipher), registry).WithCredentialValidator(registry)
	organizationID := uuid.New()
	_, err = service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: "github", DriverID: "test-driver", Name: "Ambiguous",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Credentials: map[string]string{"token": "secret"},
	})
	if err == nil {
		t.Fatal("Create() without auth_method_id succeeded despite multiple API-key methods")
	}
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: "github", DriverID: "test-driver", Name: "PAT",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey, AuthMethodID: "github_pat",
		Credentials: map[string]string{"token": "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.AuthMethodID != "github_pat" || view.AuthType != ConnectionAuthTypeAPIKey {
		t.Fatalf("connection auth identity = %#v", view)
	}
	_, err = service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: "github", DriverID: "test-driver", Name: "Wrong schema",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey, AuthMethodID: "github_pat",
		Credentials: map[string]string{"app_token": "secret"},
	})
	if err == nil {
		t.Fatal("Create() accepted credential fields from a different auth method")
	}
	_, err = service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: "github", DriverID: "test-driver", Name: "Unavailable OAuth",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeOAuth2, AuthMethodID: "github_future_oauth",
		Credentials: map[string]string{"token": "secret"},
	})
	if ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("Create() unavailable auth method error = %v, want invalid input", err)
	}
}

func TestConnectionServiceAuthFailureRequiresReconnectWithoutDisablingConnection(t *testing.T) {
	repository := newMemoryConnectionRepository()
	tester := &recordingConnectionTester{err: NewError(ErrorCodeAuthInvalid, "provider rejected credentials", nil)}
	service, _ := newConnectionServiceForTest(t, repository, tester)
	organizationID := uuid.New()
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Invalid",
		Credentials: map[string]string{"api_key": "invalid-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, _, err := service.Test(context.Background(), organizationID, view.ID, nil)
	if err == nil || ErrorCode(err) != ErrorCodeAuthInvalid {
		t.Fatalf("Test() error = %v", err)
	}
	if failed.Status != ConnectionStatusPending || failed.HealthStatus != ConnectionHealthUnhealthy || failed.AuthStatus != ConnectionAuthReconnectRequired || failed.LastErrorCode == nil || *failed.LastErrorCode != ErrorCodeAuthInvalid {
		t.Fatalf("failed connection = %#v", failed)
	}
}

func TestConnectionServiceForbiddenAndBillingFailuresDoNotInvalidateAuthentication(t *testing.T) {
	for _, test := range []struct {
		name      string
		code      string
		attention string
	}{
		{name: "forbidden", code: ErrorCodeAccessDenied, attention: ConnectionAttentionAdminCheckRequired},
		{name: "billing", code: ErrorCodeBudgetExceeded, attention: ConnectionAttentionBillingRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := newMemoryConnectionRepository()
			tester := &recordingConnectionTester{profile: &ConnectionProfile{}}
			service, _ := newConnectionServiceForTest(t, repository, tester)
			organizationID := uuid.New()
			created, err := service.Create(context.Background(), CreateConnectionInput{
				OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: test.name,
				Credentials: map[string]string{"api_key": "key"},
			})
			if err != nil {
				t.Fatal(err)
			}
			active, _, err := service.Test(context.Background(), organizationID, created.ID, nil)
			if err != nil || active.AuthStatus != ConnectionAuthValid || active.Status != ConnectionStatusActive {
				t.Fatalf("activation = %#v, %v", active, err)
			}
			tester.profile = nil
			tester.err = NewError(test.code, "provider response", nil)
			failed, _, err := service.Test(context.Background(), organizationID, created.ID, nil)
			if ErrorCode(err) != test.code {
				t.Fatalf("Test() error = %v", err)
			}
			if failed.Status != ConnectionStatusActive || failed.AuthStatus != ConnectionAuthValid || failed.HealthStatus != ConnectionHealthDegraded || failed.AttentionCode == nil || *failed.AttentionCode != test.attention {
				t.Fatalf("orthogonal status = %#v", failed)
			}
		})
	}
}

func TestConnectionServiceAuditFailurePreservesActiveDefaultHealth(t *testing.T) {
	repository := newMemoryConnectionRepository()
	tester := &recordingConnectionTester{profile: &ConnectionProfile{}}
	service, _ := newConnectionServiceForTest(t, repository, tester)
	organizationID := uuid.New()
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Audited",
		Credentials: map[string]string{"api_key": "valid-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Test(context.Background(), organizationID, view.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetDefault(context.Background(), organizationID, view.ID); err != nil {
		t.Fatal(err)
	}
	before := repository.stored(view.ID)
	tester.err = NewError(ErrorCodeAuditFailed, "audit unavailable", nil)
	tester.profile = nil
	after, _, err := service.Test(context.Background(), organizationID, view.ID, nil)
	if err == nil || ErrorCode(err) != ErrorCodeAuditFailed {
		t.Fatalf("Test() error = %v, want audit failure", err)
	}
	if after.Status != ConnectionStatusActive || !after.IsDefault || !sameOptionalTime(after.LastTestedAt, before.LastTestedAt) {
		t.Fatalf("audit failure changed connection health: before=%#v after=%#v", before, after)
	}
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func TestConnectionServiceTestCannotOverwriteConcurrentCredentialRotation(t *testing.T) {
	repository := newMemoryConnectionRepository()
	tester := &recordingConnectionTester{profile: &ConnectionProfile{}}
	service, cipher := newConnectionServiceForTest(t, repository, tester)
	organizationID := uuid.New()
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Concurrent",
		Credentials: map[string]string{"api_key": "first-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var rotateErr error
	tester.before = func() {
		_, rotateErr = service.Update(context.Background(), UpdateConnectionInput{
			OrganizationID: organizationID, ConnectionID: view.ID,
			Credentials: map[string]string{"api_key": "rotated-key"},
		})
	}
	_, _, err = service.Test(context.Background(), organizationID, view.ID, nil)
	if rotateErr != nil {
		t.Fatalf("concurrent Update() error = %v", rotateErr)
	}
	if err == nil || ErrorCode(err) != ErrorCodeConnectionConflict {
		t.Fatalf("Test() error = %v, want connection conflict", err)
	}
	stored := repository.stored(view.ID)
	if stored.CredentialVersion != 2 || stored.Status != ConnectionStatusPending {
		t.Fatalf("concurrent rotation was overwritten: %#v", stored)
	}
	credentials, decryptErr := cipher.DecryptCredentials(*stored.EncryptedCredentials, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: view.ID, IntegrationID: IntegrationWebSearch, CredentialVersion: 2,
	})
	if decryptErr != nil || credentials["api_key"] != "rotated-key" {
		t.Fatalf("rotated credential = %#v, %v", credentials, decryptErr)
	}
	destroyCredentialMap(credentials)
}

func TestConnectionServiceTestCannotReactivateConcurrentlyDisabledConnection(t *testing.T) {
	repository := newMemoryConnectionRepository()
	tester := &recordingConnectionTester{profile: &ConnectionProfile{}}
	service, _ := newConnectionServiceForTest(t, repository, tester)
	organizationID := uuid.New()
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Concurrent disable",
		Credentials: map[string]string{"api_key": "first-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var disableErr error
	tester.before = func() {
		disabled := true
		_, disableErr = service.Update(context.Background(), UpdateConnectionInput{
			OrganizationID: organizationID, ConnectionID: view.ID, Disabled: &disabled,
		})
	}
	_, _, err = service.Test(context.Background(), organizationID, view.ID, nil)
	if disableErr != nil {
		t.Fatalf("concurrent disable error = %v", disableErr)
	}
	if err == nil || ErrorCode(err) != ErrorCodeConnectionConflict {
		t.Fatalf("Test() error = %v, want connection conflict", err)
	}
	if stored := repository.stored(view.ID); stored.Status != ConnectionStatusDisabled {
		t.Fatalf("concurrent disable was overwritten: %#v", stored)
	}
}

func TestConnectionServiceCreateMapsDuplicateNameToConflict(t *testing.T) {
	repository := newMemoryConnectionRepository()
	service, _ := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	organizationID := uuid.New()
	input := CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Shared name",
		Credentials: map[string]string{"api_key": "first-key"},
	}
	if _, err := service.Create(context.Background(), input); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	input.Credentials = map[string]string{"api_key": "second-key"}
	_, err := service.Create(context.Background(), input)
	if ErrorCode(err) != ErrorCodeConnectionConflict {
		t.Fatalf("duplicate Create() error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestConnectionServiceRejectsStaleClientRevisionBeforeApplyingFields(t *testing.T) {
	repository := newMemoryConnectionRepository()
	service, _ := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	organizationID := uuid.New()
	view, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa, Name: "Original",
		Credentials: map[string]string{"api_key": "first-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	currentName := "Current"
	current, err := service.Update(context.Background(), UpdateConnectionInput{
		OrganizationID: organizationID, ConnectionID: view.ID, ExpectedRevision: view.Revision, Name: &currentName,
	})
	if err != nil {
		t.Fatalf("current Update() error = %v", err)
	}
	staleName := "Stale overwrite"
	_, err = service.Update(context.Background(), UpdateConnectionInput{
		OrganizationID: organizationID, ConnectionID: view.ID, ExpectedRevision: view.Revision, Name: &staleName,
	})
	if ErrorCode(err) != ErrorCodeConnectionConflict {
		t.Fatalf("stale Update() error = %v, code = %q", err, ErrorCode(err))
	}
	stored := repository.stored(view.ID)
	if stored.Name != current.Name || stored.Revision != current.Revision {
		t.Fatalf("stale update changed connection: stored=%#v current=%#v", stored, current)
	}
}

func TestConnectionServiceListPageReturnsBoundedOrganizationScopedMetadata(t *testing.T) {
	repository := newMemoryConnectionRepository()
	service, _ := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	organizationID := uuid.New()
	otherOrganizationID := uuid.New()
	for index := 0; index < 3; index++ {
		if _, err := service.Create(context.Background(), CreateConnectionInput{
			OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
			Name: fmt.Sprintf("Connection %d", index), Credentials: map[string]string{"api_key": fmt.Sprintf("key-%d", index)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: otherOrganizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		Name: "Other organization", Credentials: map[string]string{"api_key": "other-key"},
	}); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListPage(context.Background(), organizationID, ConnectionListFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListPage() error = %v", err)
	}
	if len(page.Items) != 2 || page.Total != 3 || page.Page != 1 || page.PageSize != 2 || !page.HasMore {
		t.Fatalf("ListPage() = %#v", page)
	}
	second, err := service.ListPage(context.Background(), organizationID, ConnectionListFilter{Page: 2, PageSize: 2})
	if err != nil || len(second.Items) != 1 || second.HasMore {
		t.Fatalf("second ListPage() = %#v, %v", second, err)
	}
}

func TestConnectionServiceListPageHidesLegacyPlatformAndSeparatesPersonalScopes(t *testing.T) {
	repository := newMemoryConnectionRepository()
	service, _ := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	organizationID, ownerID, otherOwnerID := uuid.New(), uuid.New(), uuid.New()
	legacyPlatformID := uuid.New()
	for _, connection := range []*IntegrationConnection{
		{
			ID: legacyPlatformID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
			Name: "Platform", CredentialSource: ConnectionCredentialSourcePlatform, Status: ConnectionStatusActive,
		},
		{
			ID: uuid.New(), OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
			Name: "Organization", CredentialSource: ConnectionCredentialSourceOrganization, Status: ConnectionStatusActive,
		},
		{
			ID: uuid.New(), OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
			Name: "Owner personal", CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &ownerID,
			Status: ConnectionStatusActive,
		},
		{
			ID: uuid.New(), OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
			Name: "Other personal", CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &otherOwnerID,
			Status: ConnectionStatusActive,
		},
	} {
		if err := repository.Create(context.Background(), connection); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	managed, err := service.ListPage(context.Background(), organizationID, ConnectionListFilter{
		CredentialSources: []ConnectionCredentialSource{ConnectionCredentialSourcePlatform, ConnectionCredentialSourceOrganization},
		Page:              1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("managed ListPage() error = %v", err)
	}
	if managed.Total != 1 || len(managed.Items) != 1 || managed.Items[0].CredentialSource != ConnectionCredentialSourceOrganization {
		t.Fatalf("managed ListPage() = %#v", managed)
	}
	for _, item := range managed.Items {
		if item.CredentialSource == ConnectionCredentialSourceAccount || item.OwnerAccountID != nil {
			t.Fatalf("managed list exposed personal metadata: %#v", item)
		}
	}

	legacyOnly, err := service.ListPage(context.Background(), organizationID, ConnectionListFilter{
		CredentialSources: []ConnectionCredentialSource{ConnectionCredentialSourcePlatform},
		Page:              1, PageSize: 20,
	})
	if err != nil || legacyOnly.Total != 0 || len(legacyOnly.Items) != 0 {
		t.Fatalf("legacy platform ListPage() = %#v, %v", legacyOnly, err)
	}
	if _, err := service.Get(context.Background(), organizationID, legacyPlatformID); ErrorCode(err) != ErrorCodeConnectionNotFound {
		t.Fatalf("Get(legacy platform) error = %v", err)
	}

	personal, err := service.ListPage(context.Background(), organizationID, ConnectionListFilter{
		CredentialSources: []ConnectionCredentialSource{ConnectionCredentialSourceAccount},
		OwnerAccountID:    &ownerID, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("personal ListPage() error = %v", err)
	}
	if personal.Total != 1 || len(personal.Items) != 1 || personal.Items[0].OwnerAccountID == nil || *personal.Items[0].OwnerAccountID != ownerID {
		t.Fatalf("personal ListPage() = %#v", personal)
	}
}

func TestConnectionServicePersonalMutationsRequireOwnerActor(t *testing.T) {
	repository := newMemoryConnectionRepository()
	tester := &recordingConnectionTester{}
	service, _ := newConnectionServiceForTest(t, repository, tester)
	organizationID, ownerID, otherAccountID, connectionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	credentials := map[string]string{"api_key": "must-be-cleared"}
	if _, err := service.Create(context.Background(), CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		Name: "Forged owner", CredentialSource: ConnectionCredentialSourceAccount,
		OwnerAccountID: &ownerID, ActorID: &otherAccountID, Credentials: credentials,
	}); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("forged-owner Create() error = %v, code = %q", err, ErrorCode(err))
	}
	if len(credentials) != 0 {
		t.Fatalf("forged-owner Create() retained credentials: %#v", credentials)
	}
	connection := &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		Name: "Owner personal", CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &ownerID,
		Status: ConnectionStatusActive, Revision: 1, CredentialVersion: 1, HealthRevision: 1,
	}
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	newName := "Unauthorized rename"
	if _, err := service.Update(context.Background(), UpdateConnectionInput{
		OrganizationID: organizationID, ConnectionID: connectionID, ExpectedRevision: 1,
		Name: &newName, ActorID: &otherAccountID,
	}); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("non-owner Update() error = %v, code = %q", err, ErrorCode(err))
	}
	if _, _, err := service.Test(context.Background(), organizationID, connectionID, &otherAccountID); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("non-owner Test() error = %v, code = %q", err, ErrorCode(err))
	}
	if tester.apiKey != "" {
		t.Fatalf("non-owner Test() reached provider with credential %q", tester.apiKey)
	}
	if _, err := service.SetDefaultAs(context.Background(), organizationID, connectionID, &ownerID); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("personal SetDefaultAs() error = %v, code = %q", err, ErrorCode(err))
	}
	if err := service.DeleteAs(context.Background(), organizationID, connectionID, &otherAccountID); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("non-owner DeleteAs() error = %v, code = %q", err, ErrorCode(err))
	}
	if stored := repository.stored(connectionID); stored == nil || stored.Name != connection.Name {
		t.Fatalf("non-owner mutation changed personal connection: %#v", stored)
	}
	if err := service.DeleteAs(context.Background(), organizationID, connectionID, &ownerID); err != nil {
		t.Fatalf("owner DeleteAs() error = %v", err)
	}
	if stored := repository.stored(connectionID); stored != nil {
		t.Fatalf("owner DeleteAs() left connection: %#v", stored)
	}
}

func TestConnectionServiceDeletesLocallyBeforeRemoteRevocation(t *testing.T) {
	repository := newMemoryConnectionRepository()
	service, _ := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	organizationID, connectionID := uuid.New(), uuid.New()
	encrypted := "encrypted-oauth-envelope"
	if err := repository.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		Name: "OAuth connection", CredentialSource: ConnectionCredentialSourceOrganization,
		AuthType: ConnectionAuthTypeOAuth2, EncryptedCredentials: &encrypted,
		Status: ConnectionStatusActive, Revision: 1, CredentialVersion: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	events := []string{}
	repository.deleteEvent = func() { events = append(events, "delete") }
	revoker := &recordingConnectionRevoker{events: &events}
	service.WithConnectionRevoker(revoker)

	if err := service.DeleteAs(context.Background(), organizationID, connectionID, nil); err != nil {
		t.Fatalf("DeleteAs() error = %v", err)
	}
	if got := strings.Join(events, ","); got != "delete,revoke" {
		t.Fatalf("deletion events = %q, want local delete before revoke", got)
	}
	if stored := repository.stored(connectionID); stored != nil {
		t.Fatalf("DeleteAs() left connection: %#v", stored)
	}
}

func TestConnectionServiceDoesNotRevokeWhenLocalDeletionFails(t *testing.T) {
	repository := newMemoryConnectionRepository()
	service, _ := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	organizationID, connectionID := uuid.New(), uuid.New()
	if err := repository.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		Name: "Bound OAuth connection", CredentialSource: ConnectionCredentialSourceOrganization,
		AuthType: ConnectionAuthTypeOAuth2, Status: ConnectionStatusActive,
		Revision: 1, CredentialVersion: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	repository.deleteErr = ErrConnectionInUse
	revoker := &recordingConnectionRevoker{}
	service.WithConnectionRevoker(revoker)

	if err := service.DeleteAs(context.Background(), organizationID, connectionID, nil); ErrorCode(err) != ErrorCodeConnectionInUse {
		t.Fatalf("DeleteAs() error = %v, code = %q", err, ErrorCode(err))
	}
	if revoker.calls != 0 {
		t.Fatalf("DeleteAs() revoked provider token before failed local delete")
	}
	if stored := repository.stored(connectionID); stored == nil {
		t.Fatal("DeleteAs() removed connection despite local repository failure")
	}
}

func TestConnectionServiceRemoteRevocationFailureDoesNotRestoreDeletedCredential(t *testing.T) {
	repository := newMemoryConnectionRepository()
	service, _ := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	organizationID, connectionID := uuid.New(), uuid.New()
	if err := repository.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		Name: "Provider unavailable", CredentialSource: ConnectionCredentialSourceOrganization,
		AuthType: ConnectionAuthTypeOAuth2, Status: ConnectionStatusActive,
		Revision: 1, CredentialVersion: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	revoker := &recordingConnectionRevoker{err: NewError(ErrorCodeUpstream, "provider unavailable", nil)}
	service.WithConnectionRevoker(revoker)

	if err := service.DeleteAs(context.Background(), organizationID, connectionID, nil); err != nil {
		t.Fatalf("DeleteAs() error = %v", err)
	}
	if revoker.calls != 1 {
		t.Fatalf("RevokeConnection() calls = %d, want 1", revoker.calls)
	}
	if stored := repository.stored(connectionID); stored != nil {
		t.Fatalf("revocation failure restored local connection: %#v", stored)
	}
}
