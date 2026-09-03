package integrations

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type providerInstallNotificationRecorder struct {
	flowID    uuid.UUID
	code      string
	calls     int
	returnErr error
}

type providerInstallTaskOutbox struct {
	task       OAuthRecoveryTask
	claimed    []OAuthRecoveryTask
	err        error
	ackCalls   int
	retryCalls int
}

func (outbox *providerInstallTaskOutbox) Enqueue(_ context.Context, task OAuthRecoveryTask) error {
	outbox.task = task
	return outbox.err
}

type sequencedProviderInstallProvider struct {
	exchangeCalls int
	firstErr      error
}

func (provider *sequencedProviderInstallProvider) BuildProviderInstallAuthorizationURL(context.Context, ProviderInstallAuthorizationRequest) (string, error) {
	return "https://provider.example/install", nil
}

func (provider *sequencedProviderInstallProvider) ExchangeProviderInstallCode(context.Context, ProviderInstallExchangeRequest) (ProviderInstallGrant, error) {
	provider.exchangeCalls++
	if provider.exchangeCalls == 1 && provider.firstErr != nil {
		return ProviderInstallGrant{}, provider.firstErr
	}
	return ProviderInstallGrant{
		ExternalTenantHash: strings.Repeat("e", 64),
		AccountDisplayName: "Example Corp",
		Credentials:        map[string]string{"permanent_code": "permanent-secret"},
		Config:             map[string]any{"authorization_mode": "suite"},
		GrantedScopes:      []string{"wecom:app"},
	}, nil
}

func newProviderInstallMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func newPendingProviderInstallFlow(now time.Time, state, browserBindingDigest string) *IntegrationProviderInstallFlow {
	return &IntegrationProviderInstallFlow{
		ID:                   uuid.New(),
		FlowDigest:           providerInstallDigest("flow-" + state),
		StateDigest:          providerInstallDigest(state),
		BrowserBindingDigest: browserBindingDigest,
		OrganizationID:       uuid.New(),
		AccountID:            uuid.New(),
		IntegrationID:        "wecom",
		DriverID:             "wecom-suite-open-api",
		AuthMethodID:         "organization_wecom_suite_app",
		CredentialSource:     ConnectionCredentialSourceOrganization,
		ConnectionName:       "Example WeCom",
		ReturnPath:           "/console/integrations",
		Status:               ProviderInstallFlowPending,
		ExpiresAt:            now.Add(10 * time.Minute),
	}
}

func providerInstallFlowRows(flow *IntegrationProviderInstallFlow) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "flow_digest", "state_digest", "browser_binding_digest", "organization_id", "account_id",
		"completed_connection_id", "integration_id", "driver_id", "auth_method_id", "credential_source",
		"connection_name", "return_path", "status", "failure_code", "account_display_name", "expires_at",
		"completed_at", "created_at", "updated_at",
	}).AddRow(
		flow.ID, flow.FlowDigest, flow.StateDigest, flow.BrowserBindingDigest, flow.OrganizationID, flow.AccountID,
		nil, flow.IntegrationID, flow.DriverID, flow.AuthMethodID, flow.CredentialSource,
		flow.ConnectionName, flow.ReturnPath, flow.Status, nil, nil, flow.ExpiresAt,
		nil, flow.CreatedAt, flow.UpdatedAt,
	)
}

func expectProviderInstallFlowByState(mock sqlmock.Sqlmock, flow *IntegrationProviderInstallFlow) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "integration_provider_install_flows" WHERE state_digest = $1 ORDER BY "integration_provider_install_flows"."id" LIMIT $2`)).
		WithArgs(flow.StateDigest, 1).
		WillReturnRows(providerInstallFlowRows(flow))
}

func expectProviderInstallFlowByID(mock sqlmock.Sqlmock, flow *IntegrationProviderInstallFlow, forUpdate bool) {
	query := `SELECT * FROM "integration_provider_install_flows" WHERE id = $1 ORDER BY "integration_provider_install_flows"."id" LIMIT $2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(flow.ID, 1).WillReturnRows(providerInstallFlowRows(flow))
}

func providerInstallRegistry(provider ProviderInstallProvider) *Registry {
	registry := NewRegistry()
	registry.registrations["wecom"] = Registration{
		Definition: ProviderDefinition{
			ID:       "wecom",
			DriverID: "wecom-suite-open-api",
			AuthMethods: []AuthMethodDefinition{{
				ID:               "organization_wecom_suite_app",
				Type:             AuthMethodTypeProviderInstall,
				CredentialSource: ConnectionCredentialSourceOrganization,
				Available:        true,
			}},
		},
		ProviderInstaller: provider,
	}
	return registry
}
func (outbox *providerInstallTaskOutbox) Claim(context.Context, int64) ([]OAuthRecoveryTask, error) {
	tasks := append([]OAuthRecoveryTask(nil), outbox.claimed...)
	outbox.claimed = nil
	return tasks, nil
}
func (*providerInstallTaskOutbox) Get(context.Context, string) (*OAuthRecoveryTask, error) {
	return nil, nil
}
func (outbox *providerInstallTaskOutbox) Ack(context.Context, string) error {
	outbox.ackCalls++
	return nil
}
func (outbox *providerInstallTaskOutbox) Retry(context.Context, OAuthRecoveryTask, string) error {
	outbox.retryCalls++
	return nil
}
func (*providerInstallTaskOutbox) DeadLetter(context.Context, OAuthRecoveryTask, string) error {
	return nil
}

func (recorder *providerInstallNotificationRecorder) CompleteProviderInstallNotification(_ context.Context, flowID uuid.UUID, code string) error {
	recorder.flowID, recorder.code = flowID, code
	recorder.calls++
	return recorder.returnErr
}

func TestProviderInstallOpaqueReferencesAndReturnPath(t *testing.T) {
	firstRef, err := randomProviderInstallReference()
	if err != nil {
		t.Fatal(err)
	}
	secondRef, err := randomProviderInstallReference()
	if err != nil {
		t.Fatal(err)
	}
	state, err := randomProviderInstallState()
	if err != nil {
		t.Fatal(err)
	}
	if firstRef == secondRef || len(firstRef) < 40 || len(state) != 64 || !sha256HexPattern.MatchString(state) {
		t.Fatalf("opaque reference contract failed: first=%q second=%q state=%q", firstRef, secondRef, state)
	}
	if providerInstallDigest(firstRef) == firstRef || !sha256HexPattern.MatchString(providerInstallDigest(firstRef)) {
		t.Fatal("provider install references must be persisted only as SHA-256 digests")
	}
	if got := safeProviderInstallReturnPath("/console/integrations?view=connected"); got != "/console/integrations?view=connected" {
		t.Fatalf("safe return path=%q", got)
	}
	for _, unsafe := range []string{"https://evil.example/", "//evil.example/", "/api/private", "/console/x\r\nLocation:https://evil.example"} {
		if got := safeProviderInstallReturnPath(unsafe); got != "/console/integrations" {
			t.Fatalf("unsafe return path %q accepted as %q", unsafe, got)
		}
	}
}

func TestProviderInstallGrantDestroyClearsSecrets(t *testing.T) {
	grant := ProviderInstallGrant{
		ExternalTenantHash: strings.Repeat("a", 64),
		AccountDisplayName: "Example",
		Credentials:        map[string]string{"permanent_code": "secret"},
		Config:             map[string]any{"authorization_mode": "suite"},
		GrantedScopes:      []string{"wecom:app"},
	}
	grant.Destroy()
	if grant.ExternalTenantHash != "" || grant.AccountDisplayName != "" || grant.Credentials != nil || grant.Config != nil || grant.GrantedScopes != nil {
		t.Fatalf("grant was not cleared: %#v", grant)
	}
}

func TestAuthorizationChangedPreservesConnectionSelectionAndClearsEvidence(t *testing.T) {
	updates := authorizationChangedConnectionUpdates()
	if _, changesStatus := updates["status"]; changesStatus {
		t.Fatal("authorization change must preserve active connection status")
	}
	if _, changesDefault := updates["is_default"]; changesDefault {
		t.Fatal("authorization change must preserve the preferred connection")
	}
	if updates["health_status"] != ConnectionHealthUnknown || updates["scope_status"] != ConnectionScopeUnknown {
		t.Fatalf("authorization change health reset = %#v", updates)
	}
	for _, key := range []string{"verified_action_ids", "denied_action_ids"} {
		value, ok := updates[key].(datatypes.JSON)
		if !ok || string(value) != "[]" {
			t.Fatalf("%s was not cleared: %#v", key, updates[key])
		}
	}
}

func TestProviderInstallNotificationRecoveryDecryptsOnlyInsideWorker(t *testing.T) {
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	flow := &IntegrationProviderInstallFlow{
		ID: uuid.New(), OrganizationID: uuid.New(), IntegrationID: "wecom",
		DriverID: "wecom-suite-open-api", AuthMethodID: "organization_wecom_suite_app",
	}
	envelope, err := cipher.EncryptCredentials(map[string]string{"authorization_code": "one-time-secret"}, CredentialAAD{
		OrganizationID: flow.OrganizationID, ConnectionID: flow.ID,
		IntegrationID: flow.IntegrationID, CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := newProviderInstallRecoveryTask(flow, envelope, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if task.Kind != OAuthRecoveryProviderInstall || task.ID != "install-"+flow.ID.String() {
		t.Fatalf("task identity = %#v", task)
	}
	if strings.Contains(task.EncryptedCredentials, "one-time-secret") {
		t.Fatal("provider authorization code was stored in plaintext")
	}
	recorder := &providerInstallNotificationRecorder{}
	service := NewOAuthRecoveryService(nil, nil, nil, cipher).WithProviderInstallNotifications(recorder)
	if err := service.apply(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 || recorder.flowID != flow.ID || recorder.code != "one-time-secret" {
		t.Fatalf("notification completion = %#v", recorder)
	}
}

func TestProviderInstallNotificationIsEncryptedAndBoundToMatchingFlow(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	flowID, organizationID := uuid.New(), uuid.New()
	state := strings.Repeat("a", 64)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "integration_provider_install_flows" WHERE state_digest = $1 AND integration_id = $2 ORDER BY "integration_provider_install_flows"."id" LIMIT $3`)).
		WithArgs(providerInstallDigest(state), "wecom", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "integration_id", "driver_id", "auth_method_id", "status", "expires_at"}).
			AddRow(flowID, organizationID, "wecom", "wecom-suite-open-api", "organization_wecom_suite_app", ProviderInstallFlowPending, now.Add(10*time.Minute)))
	outbox := &providerInstallTaskOutbox{}
	service := NewProviderInstallFlowService(db, cipher, nil, 10*time.Minute).WithNotificationOutbox(outbox)
	service.now = func() time.Time { return now }
	if err := service.EnqueueAuthorizationNotification(context.Background(), "wecom", state, "one-time-secret"); err != nil {
		t.Fatal(err)
	}
	if outbox.task.Kind != OAuthRecoveryProviderInstall || outbox.task.ConnectionID != flowID || outbox.task.OrganizationID != organizationID {
		t.Fatalf("queued task = %#v", outbox.task)
	}
	if strings.Contains(outbox.task.EncryptedCredentials, "one-time-secret") {
		t.Fatal("queued provider authorization code was plaintext")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderInstallBrowserCallbackQueuesEncryptedCodeAndReturnsPending(t *testing.T) {
	db, mock := newProviderInstallMockDB(t)
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := strings.Repeat("c", 64)
	browserBindingDigest := strings.Repeat("d", 64)
	flow := newPendingProviderInstallFlow(now, state, browserBindingDigest)
	expectProviderInstallFlowByState(mock, flow)
	outbox := &providerInstallTaskOutbox{}
	// The browser callback must not need a provider registry because it only
	// commits the encrypted authorization code to durable recovery.
	service := NewProviderInstallFlowService(db, cipher, nil, 10*time.Minute).WithNotificationOutbox(outbox)
	service.now = func() time.Time { return now }

	result, err := service.Callback(context.Background(), state, browserBindingDigest, "one-time-browser-secret")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ProviderInstallFlowPending || result.ReturnPath != flow.ReturnPath || result.ErrorCode != "" {
		t.Fatalf("callback result = %#v", result)
	}
	if outbox.task.Kind != OAuthRecoveryProviderInstall || outbox.task.ConnectionID != flow.ID {
		t.Fatalf("queued task = %#v", outbox.task)
	}
	if strings.Contains(outbox.task.EncryptedCredentials, "one-time-browser-secret") {
		t.Fatal("browser authorization code was stored in plaintext")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderInstallBrowserCallbackQueueFailureIsNormalizedAndSecretSafe(t *testing.T) {
	db, mock := newProviderInstallMockDB(t)
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := strings.Repeat("f", 64)
	browserBindingDigest := strings.Repeat("1", 64)
	flow := newPendingProviderInstallFlow(now, state, browserBindingDigest)
	expectProviderInstallFlowByState(mock, flow)
	outbox := &providerInstallTaskOutbox{err: errors.New("redis temporarily unavailable")}
	service := NewProviderInstallFlowService(db, cipher, nil, 10*time.Minute).WithNotificationOutbox(outbox)
	service.now = func() time.Time { return now }
	secret := "one-time-queue-failure-secret"

	result, err := service.Callback(context.Background(), state, browserBindingDigest, secret)
	if ErrorCode(err) != ErrorCodeUpstream || result.Status != ProviderInstallFlowFailed || result.ErrorCode != ErrorCodeUpstream {
		t.Fatalf("callback result = %#v, error = %v", result, err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(fmt.Sprint(result), secret) || strings.Contains(outbox.task.EncryptedCredentials, secret) {
		t.Fatal("queue failure exposed the one-time authorization code")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderInstallRecoveryRetriesTransientExchangeThenSucceeds(t *testing.T) {
	db, mock := newProviderInstallMockDB(t)
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	flow := newPendingProviderInstallFlow(now, strings.Repeat("2", 64), strings.Repeat("3", 64))
	provider := &sequencedProviderInstallProvider{firstErr: NewProviderError(
		ErrorCodeUpstream,
		"provider install exchange is temporarily unavailable",
		errors.New("temporary transport failure"),
		ProviderDiagnostics{ErrorCode: "system_busy", RequestID: "wecom-request-1", HTTPStatus: 503},
	)}
	service := NewProviderInstallFlowService(db, cipher, providerInstallRegistry(provider), 10*time.Minute)
	service.now = func() time.Time { return now }

	expectProviderInstallFlowByID(mock, flow, false)
	if err := service.CompleteProviderInstallNotification(context.Background(), flow.ID, "one-time-retry-code"); ErrorCode(err) != ErrorCodeUpstream {
		t.Fatalf("first completion error = %v, code = %q", err, ErrorCode(err))
	}

	expectProviderInstallFlowByID(mock, flow, false)
	mock.ExpectBegin()
	expectProviderInstallFlowByID(mock, flow, true)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "integration_provider_authorizations"`).
		WithArgs(flow.IntegrationID, strings.Repeat("e", 64), true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`INSERT INTO "integration_connections"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectExec(`INSERT INTO "integration_provider_authorizations"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "integration_provider_install_flows" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.CompleteProviderInstallNotification(context.Background(), flow.ID, "one-time-retry-code"); err != nil {
		t.Fatal(err)
	}
	if provider.exchangeCalls != 2 {
		t.Fatalf("exchange calls=%d, want 2", provider.exchangeCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderInstallRecoveryAcknowledgesPersistedTerminalFailure(t *testing.T) {
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	flow := newPendingProviderInstallFlow(time.Now().UTC(), strings.Repeat("4", 64), strings.Repeat("5", 64))
	envelope, err := cipher.EncryptCredentials(map[string]string{"authorization_code": "terminal-one-time-secret"}, CredentialAAD{
		OrganizationID:    flow.OrganizationID,
		ConnectionID:      flow.ID,
		IntegrationID:     flow.IntegrationID,
		CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := newProviderInstallRecoveryTask(flow, envelope, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	outbox := &providerInstallTaskOutbox{claimed: []OAuthRecoveryTask{task}}
	recorder := &providerInstallNotificationRecorder{returnErr: errors.Join(
		NewProviderError(
			ErrorCodeAuthInvalid,
			"provider rejected the authorization code",
			errors.New("provider detail must remain private"),
			ProviderDiagnostics{ErrorCode: "invalid_auth_code", RequestID: "request-2", HTTPStatus: 400},
		),
		errProviderInstallTerminal,
	)}
	service := NewOAuthRecoveryService(outbox, nil, nil, cipher).WithProviderInstallNotifications(recorder)

	if err := service.RecoverBatch(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 || outbox.ackCalls != 1 || outbox.retryCalls != 0 {
		t.Fatalf("calls=%d ack=%d retry=%d", recorder.calls, outbox.ackCalls, outbox.retryCalls)
	}
}

func TestProviderInstallRecoveryDiagnosticsContainOnlySafeStructuredFields(t *testing.T) {
	task := OAuthRecoveryTask{
		Kind:          OAuthRecoveryProviderInstall,
		IntegrationID: "wecom",
		Attempts:      2,
	}
	err := NewProviderError(
		ErrorCodeUpstream,
		"safe public message",
		errors.New("authorization_code=operator-secret raw-provider-body"),
		ProviderDiagnostics{ErrorCode: "system_busy", RequestID: "request-3", HTTPStatus: 503},
	)
	fields := providerInstallRecoveryDiagnosticFields(task, err)
	if fields["integration_id"] != "wecom" || fields["error_code"] != ErrorCodeUpstream ||
		fields["attempt_number"] != 3 || fields["retryable"] != true ||
		fields["provider_error_code"] != "system_busy" || fields["provider_request_id"] != "request-3" ||
		fields["provider_http_status"] != 503 {
		t.Fatalf("diagnostic fields = %#v", fields)
	}
	serialized := fmt.Sprint(fields)
	for _, forbidden := range []string{"operator-secret", "raw-provider-body", "authorization_code", "connection_id", "encrypted_credentials"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("diagnostic fields leaked %q: %s", forbidden, serialized)
		}
	}
}
