package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

func newOAuthRecoveryRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client, *RedisOAuthRecoveryOutbox) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client, NewRedisOAuthRecoveryOutbox(client)
}

func encryptedOAuthRecoveryConnection(t *testing.T, cipher CredentialCipher) *IntegrationConnection {
	t.Helper()
	connection := &IntegrationConnection{
		ID:                uuid.New(),
		OrganizationID:    uuid.New(),
		IntegrationID:     "fake",
		DriverID:          "fake-oauth",
		AuthMethodID:      "user_oauth",
		AuthType:          ConnectionAuthTypeOAuth2,
		CredentialVersion: 1,
		GrantedScopes:     []string{"account.read"},
	}
	envelope, err := cipher.EncryptCredentials(map[string]string{
		"access_token":  "plain-access-token",
		"refresh_token": "plain-refresh-token",
		"token_type":    "Bearer",
	}, CredentialAAD{
		OrganizationID:    connection.OrganizationID,
		ConnectionID:      connection.ID,
		IntegrationID:     connection.IntegrationID,
		CredentialVersion: connection.CredentialVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	connection.EncryptedCredentials = &envelope
	return connection
}

func TestRedisOAuthRecoveryOutboxStoresOnlyEncryptedCredentialMaterial(t *testing.T) {
	server, client, outbox := newOAuthRecoveryRedis(t)
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	connection := encryptedOAuthRecoveryConnection(t, cipher)
	task, err := newOAuthRevocationRecoveryTask(connection, "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.Enqueue(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	raw, err := server.Get(oauthRecoveryPayloadKey(task.ID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "plain-access-token") || strings.Contains(raw, "plain-refresh-token") {
		t.Fatalf("OAuth recovery payload contains plaintext token: %s", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := decoded["access_token"]; exists {
		t.Fatal("OAuth recovery payload exposes access_token field")
	}
	if _, exists := decoded["refresh_token"]; exists {
		t.Fatal("OAuth recovery payload exposes refresh_token field")
	}
	if decoded["encrypted_credentials"] == "" {
		t.Fatal("OAuth recovery payload did not preserve encrypted envelope")
	}
	// A newly constructed worker must be able to claim work written by the
	// previous instance; no in-process memory participates in recovery.
	restarted := NewRedisOAuthRecoveryOutbox(client)
	claimed, err := restarted.Claim(context.Background(), 1)
	if err != nil || len(claimed) != 1 || claimed[0].ID != task.ID {
		t.Fatalf("restarted outbox Claim() = %#v, %v", claimed, err)
	}
}

func TestRedisOAuthRecoveryOutboxLeaseRetryAndBoundedDeadLetter(t *testing.T) {
	server, client, outbox := newOAuthRecoveryRedis(t)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	outbox.now = func() time.Time { return now }
	outbox.leaseDuration = time.Minute
	outbox.maxAttempts = 1
	outbox.deadLetterMax = 2
	outbox.deadLetterTTL = time.Hour
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")

	var tasks []OAuthRecoveryTask
	for range 3 {
		task, err := newOAuthRevocationRecoveryTask(encryptedOAuthRecoveryConnection(t, cipher), "", 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		task.CreatedAt = now
		if err := outbox.Enqueue(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
	}
	claim, err := outbox.Claim(context.Background(), 1)
	if err != nil || len(claim) != 1 {
		t.Fatalf("Claim() = %#v, %v", claim, err)
	}
	second, err := outbox.Claim(context.Background(), 1)
	if err != nil || len(second) != 1 || second[0].ID == claim[0].ID {
		t.Fatalf("second Claim() = %#v, %v", second, err)
	}
	if err := outbox.Retry(context.Background(), claim[0], "provider_unavailable"); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Retry(context.Background(), second[0], "provider_unavailable"); err != nil {
		t.Fatal(err)
	}
	third, err := outbox.Claim(context.Background(), 1)
	if err != nil || len(third) != 1 {
		t.Fatalf("third Claim() = %#v, %v", third, err)
	}
	if err := outbox.Retry(context.Background(), third[0], "provider_unavailable"); err != nil {
		t.Fatal(err)
	}
	if got := client.LLen(context.Background(), oauthRecoveryDeadLetterKey).Val(); got != 2 {
		t.Fatalf("dead-letter length = %d, want bounded size 2", got)
	}
	for _, record := range client.LRange(context.Background(), oauthRecoveryDeadLetterKey, 0, -1).Val() {
		if strings.Contains(record, "plain-access-token") || strings.Contains(record, "plain-refresh-token") {
			t.Fatal("OAuth recovery dead letter contains plaintext token")
		}
	}
	if ttl := server.TTL(oauthRecoveryDeadLetterKey); ttl <= 0 || ttl > time.Hour {
		t.Fatalf("dead-letter TTL = %v, want positive bounded TTL", ttl)
	}
}

func TestRedisOAuthRecoveryOutboxReclaimsExpiredLease(t *testing.T) {
	server, _, outbox := newOAuthRecoveryRedis(t)
	redisNow := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	server.SetTime(redisNow)
	outbox.leaseDuration = time.Minute
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	task, err := newOAuthRevocationRecoveryTask(encryptedOAuthRecoveryConnection(t, cipher), "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.Enqueue(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	first, err := outbox.Claim(context.Background(), 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first Claim() = %#v, %v", first, err)
	}
	second, err := outbox.Claim(context.Background(), 1)
	if err != nil || len(second) != 0 {
		t.Fatalf("Claim() before lease expiry = %#v, %v", second, err)
	}
	server.SetTime(redisNow.Add(time.Minute + time.Millisecond))
	reclaimed, err := outbox.Claim(context.Background(), 1)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != task.ID {
		t.Fatalf("Claim() after lease expiry = %#v, %v", reclaimed, err)
	}
}

type recoveringRevoker struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (revoker *recoveringRevoker) RevokeConnection(context.Context, *IntegrationConnection) error {
	revoker.mu.Lock()
	defer revoker.mu.Unlock()
	revoker.calls++
	return revoker.err
}

func (revoker *recoveringRevoker) callCount() int {
	revoker.mu.Lock()
	defer revoker.mu.Unlock()
	return revoker.calls
}

func (revoker *recoveringRevoker) setError(err error) {
	revoker.mu.Lock()
	defer revoker.mu.Unlock()
	revoker.err = err
}

func TestConnectionDeletionQueuesEncryptedRevocationAfterLocalCommit(t *testing.T) {
	_, _, outbox := newOAuthRecoveryRedis(t)
	repository := newMemoryConnectionRepository()
	service, cipher := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	connection := encryptedOAuthRecoveryConnection(t, cipher)
	connection.Name = "OAuth"
	connection.CredentialSource = ConnectionCredentialSourceOrganization
	connection.Status = ConnectionStatusActive
	connection.Revision = 1
	connection.HealthRevision = 1
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	revoker := &recoveringRevoker{err: NewError(ErrorCodeUpstream, "provider unavailable", nil)}
	recovery := NewOAuthRecoveryService(outbox, repository, revoker, cipher)
	service.WithConnectionRevoker(revoker).WithOAuthRecovery(recovery)

	if err := service.DeleteAs(context.Background(), connection.OrganizationID, connection.ID, nil); err != nil {
		t.Fatalf("DeleteAs() error = %v", err)
	}
	if repository.stored(connection.ID) != nil {
		t.Fatal("local connection survived remote revocation failure")
	}
	taskID := oauthRevocationRecoveryTaskID(connection.OrganizationID, connection.ID, connection.CredentialVersion)
	queued, err := outbox.Get(context.Background(), taskID)
	if err != nil || queued.Kind != OAuthRecoveryRevoke {
		t.Fatalf("queued revocation = %#v, %v", queued, err)
	}
	if strings.Contains(queued.EncryptedCredentials, "plain-refresh-token") {
		t.Fatal("queued revocation contains plaintext token")
	}
	revoker.setError(nil)
	if err := recovery.RecoverBatch(context.Background(), 10); err != nil {
		t.Fatalf("RecoverBatch() error = %v", err)
	}
	if revoker.callCount() != 2 {
		t.Fatalf("revoker calls = %d, want immediate attempt plus recovery", revoker.callCount())
	}
}

func TestConnectionDeletionFailureNeverRevokesOrQueues(t *testing.T) {
	_, _, outbox := newOAuthRecoveryRedis(t)
	repository := newMemoryConnectionRepository()
	service, cipher := newConnectionServiceForTest(t, repository, &recordingConnectionTester{})
	connection := encryptedOAuthRecoveryConnection(t, cipher)
	connection.Name = "Bound OAuth"
	connection.CredentialSource = ConnectionCredentialSourceOrganization
	connection.Status = ConnectionStatusActive
	connection.Revision = 1
	connection.HealthRevision = 1
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	repository.deleteErr = ErrConnectionInUse
	revoker := &recoveringRevoker{err: errors.New("must not be called")}
	recovery := NewOAuthRecoveryService(outbox, repository, revoker, cipher)
	service.WithConnectionRevoker(revoker).WithOAuthRecovery(recovery)

	if err := service.DeleteAs(context.Background(), connection.OrganizationID, connection.ID, nil); ErrorCode(err) != ErrorCodeConnectionInUse {
		t.Fatalf("DeleteAs() error = %v", err)
	}
	if revoker.callCount() != 0 {
		t.Fatalf("revoker calls = %d, want 0", revoker.callCount())
	}
	claimed, err := outbox.Claim(context.Background(), 10)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("queued work after failed local delete = %#v, %v", claimed, err)
	}
}

func TestQueuedRevocationRetainsAADAndClientRoutingAfterLocalDeletion(t *testing.T) {
	_, _, outbox := newOAuthRecoveryRedis(t)
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	repository := newMemoryConnectionRepository()
	connection := encryptedOAuthRecoveryConnection(t, cipher)
	connection.Name = "Deleted OAuth"
	connection.CredentialSource = ConnectionCredentialSourceOrganization
	connection.Status = ConnectionStatusActive
	connection.Revision = 1
	connection.HealthRevision = 1
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	task, err := newOAuthRevocationRecoveryTask(connection, "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(context.Background(), connection.OrganizationID, connection.ID); err != nil {
		t.Fatal(err)
	}
	adapter := &fakeOAuthAdapter{}
	registry := NewRegistry()
	if err := registry.Register(oauthTestRegistration(adapter)); err != nil {
		t.Fatal(err)
	}
	clients := NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
	}})
	revoker := NewOAuthConnectionRevoker(cipher, registry, clients)
	recovery := NewOAuthRecoveryService(outbox, repository, revoker, cipher)
	if err := outbox.Enqueue(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := recovery.RecoverBatch(context.Background(), 10); err != nil {
		t.Fatalf("RecoverBatch() error = %v", err)
	}
	adapter.mu.Lock()
	revokeCalls, revokedHint := adapter.revokeCalls, adapter.revokedHint
	adapter.mu.Unlock()
	if revokeCalls != 1 || revokedHint != "refresh_token" {
		t.Fatalf("recovered revocation calls=%d hint=%q", revokeCalls, revokedHint)
	}
}

func TestOAuthRefreshRecoveryRunsBeforeAnotherProviderRefresh(t *testing.T) {
	server, _, outbox := newOAuthRecoveryRedis(t)
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
		Name: "Recover rotating OAuth", CredentialSource: ConnectionCredentialSourceAccount, AuthType: ConnectionAuthTypeOAuth2,
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
	repository.oauthUpdateFailures = 3
	repository.oauthUpdateErr = errors.New("database unavailable")
	clients := NewOAuthClientConfigService(nil, cipher, registry, []OAuthDeploymentClient{{
		IntegrationID: "fake", DriverID: "fake-oauth", AuthMethodID: "user_oauth", ClientID: "client",
	}})
	recovery := NewOAuthRecoveryService(outbox, repository, &recoveringRevoker{}, cipher)
	resolver := NewOAuthRefreshingConnectionResolver(
		NewConnectionResolver(repository, cipher),
		repository,
		cipher,
		registry,
		clients,
		&serialOAuthRefreshLocker{},
		5*time.Minute,
	).WithOAuthRecovery(recovery)
	request := ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: "fake", DriverID: "fake-oauth", ConnectionID: connectionID.String(),
	}

	if resolved, err := resolver.Resolve(context.Background(), request); err == nil || resolved != nil {
		t.Fatalf("first Resolve() = %#v, %v; want queued persistence failure", resolved, err)
	}
	if adapter.refreshCalls != 1 {
		t.Fatalf("provider refresh calls = %d, want 1", adapter.refreshCalls)
	}
	recoveryID := oauthRefreshRecoveryTaskID(organizationID, connectionID, 1)
	rawRecovery, err := server.Get(oauthRecoveryPayloadKey(recoveryID))
	if err != nil {
		t.Fatalf("queued refresh recovery payload: %v", err)
	}
	if strings.Contains(rawRecovery, "rotated-refresh") || strings.Contains(rawRecovery, "refreshed-access") {
		t.Fatal("queued refresh recovery payload contains plaintext provider token")
	}
	repository.oauthUpdateFailures = 0
	resolved, err := resolver.Resolve(context.Background(), request)
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	resolved.Destroy()
	if adapter.refreshCalls != 1 {
		t.Fatalf("provider refresh calls after recovery = %d, want no second refresh", adapter.refreshCalls)
	}
	stored := repository.stored(connectionID)
	if stored == nil || stored.CredentialVersion != 2 ||
		stored.RefreshTokenExpiresAt == nil ||
		!stored.RefreshTokenExpiresAt.After(time.Now().UTC().Add(20*time.Hour)) {
		t.Fatalf("recovered connection = %#v", stored)
	}
	credentials, err := cipher.DecryptCredentials(*stored.EncryptedCredentials, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer destroyCredentialMap(credentials)
	if credentials["refresh_token"] != "rotated-refresh" {
		t.Fatal("durable recovery did not restore rotated refresh token")
	}
}

func TestOAuthRefreshRecoveryRejectsChangedConnectionIdentity(t *testing.T) {
	_, _, outbox := newOAuthRecoveryRedis(t)
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	repository := newMemoryConnectionRepository()
	organizationID, connectionID := uuid.New(), uuid.New()
	currentEnvelope, _ := cipher.EncryptCredentials(map[string]string{
		"access_token": "old-access", "refresh_token": "old-refresh",
	}, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 1,
	})
	current := &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "fake", DriverID: "replacement-driver",
		Name: "Changed identity", CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeOAuth2,
		AuthMethodID: "replacement_oauth", EncryptedCredentials: &currentEnvelope, CredentialVersion: 1,
		Revision: 1, HealthRevision: 1, Status: ConnectionStatusActive,
	}
	if err := repository.Create(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	nextEnvelope, _ := cipher.EncryptCredentials(map[string]string{
		"access_token": "new-access", "refresh_token": "new-refresh",
	}, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 2,
	})
	next := &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "fake", DriverID: "fake-oauth",
		AuthType: ConnectionAuthTypeOAuth2, AuthMethodID: "user_oauth", EncryptedCredentials: &nextEnvelope,
		CredentialVersion: 2, GrantedScopes: []string{"account.read"},
		AuthStatus: ConnectionAuthValid, ScopeStatus: ConnectionScopeVerified,
	}
	recovery := NewOAuthRecoveryService(outbox, repository, &recoveringRevoker{}, cipher)
	if err := recovery.EnqueueRefresh(context.Background(), next, 1); err != nil {
		t.Fatal(err)
	}
	if err := recovery.RecoverConnection(context.Background(), organizationID, connectionID, 1); err == nil {
		t.Fatal("RecoverConnection() accepted a changed provider/auth identity")
	}
	stored := repository.stored(connectionID)
	if stored == nil || stored.CredentialVersion != 1 || stored.DriverID != "replacement-driver" {
		t.Fatalf("identity mismatch mutated connection: %#v", stored)
	}
	credentials, err := cipher.DecryptCredentials(*stored.EncryptedCredentials, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionID, IntegrationID: "fake", CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer destroyCredentialMap(credentials)
	if credentials["refresh_token"] != "old-refresh" {
		t.Fatal("identity mismatch overwrote current credential")
	}
}

func TestOAuthRecoveryWorkerProcessesQueuedRevocationAndStops(t *testing.T) {
	_, _, outbox := newOAuthRecoveryRedis(t)
	cipher, _ := NewCredentialCipher("12345678901234567890123456789012")
	task, err := newOAuthRevocationRecoveryTask(encryptedOAuthRecoveryConnection(t, cipher), "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.Enqueue(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	revoker := &recoveringRevoker{}
	service := NewOAuthRecoveryService(outbox, newMemoryConnectionRepository(), revoker, cipher)
	workerCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.RunOAuthRecovery(workerCtx)
	}()
	deadline := time.After(time.Second)
	for revoker.callCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("OAuth recovery worker did not process queued revocation")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("OAuth recovery worker did not stop")
	}
	if _, err := outbox.Get(context.Background(), task.ID); !errors.Is(err, redis.Nil) {
		t.Fatalf("recovered task remained in outbox: %v", err)
	}
}
