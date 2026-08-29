package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

type durableOAuthTaskStore struct {
	mu    sync.Mutex
	tasks map[string]OAuthRecoveryTask
}

type memoryDurableOAuthOutbox struct {
	store *durableOAuthTaskStore
}

func newMemoryDurableOAuthOutbox(store *durableOAuthTaskStore) *memoryDurableOAuthOutbox {
	if store.tasks == nil {
		store.tasks = make(map[string]OAuthRecoveryTask)
	}
	return &memoryDurableOAuthOutbox{store: store}
}

func (outbox *memoryDurableOAuthOutbox) Enqueue(_ context.Context, task OAuthRecoveryTask) error {
	outbox.store.mu.Lock()
	defer outbox.store.mu.Unlock()
	outbox.store.tasks[task.ID] = task
	return nil
}

func (outbox *memoryDurableOAuthOutbox) Claim(_ context.Context, limit int64) ([]OAuthRecoveryTask, error) {
	outbox.store.mu.Lock()
	defer outbox.store.mu.Unlock()
	result := make([]OAuthRecoveryTask, 0)
	for _, task := range outbox.store.tasks {
		if int64(len(result)) >= limit {
			break
		}
		result = append(result, task)
	}
	return result, nil
}

func (outbox *memoryDurableOAuthOutbox) Get(_ context.Context, id string) (*OAuthRecoveryTask, error) {
	outbox.store.mu.Lock()
	defer outbox.store.mu.Unlock()
	task, ok := outbox.store.tasks[id]
	if !ok {
		return nil, redis.Nil
	}
	return &task, nil
}

func (outbox *memoryDurableOAuthOutbox) Ack(_ context.Context, id string) error {
	outbox.store.mu.Lock()
	defer outbox.store.mu.Unlock()
	delete(outbox.store.tasks, id)
	return nil
}

func (outbox *memoryDurableOAuthOutbox) Retry(_ context.Context, task OAuthRecoveryTask, _ string) error {
	outbox.store.mu.Lock()
	defer outbox.store.mu.Unlock()
	task.Attempts++
	outbox.store.tasks[task.ID] = task
	return nil
}

func (outbox *memoryDurableOAuthOutbox) DeadLetter(_ context.Context, task OAuthRecoveryTask, _ string) error {
	outbox.store.mu.Lock()
	defer outbox.store.mu.Unlock()
	delete(outbox.store.tasks, task.ID)
	return nil
}

func (outbox *memoryDurableOAuthOutbox) len() int {
	outbox.store.mu.Lock()
	defer outbox.store.mu.Unlock()
	return len(outbox.store.tasks)
}

type unavailableOAuthOutbox struct{}

func (unavailableOAuthOutbox) Enqueue(context.Context, OAuthRecoveryTask) error {
	return errors.New("redis unavailable")
}
func (unavailableOAuthOutbox) Claim(context.Context, int64) ([]OAuthRecoveryTask, error) {
	return nil, errors.New("redis unavailable")
}
func (unavailableOAuthOutbox) Get(context.Context, string) (*OAuthRecoveryTask, error) {
	return nil, errors.New("redis unavailable")
}
func (unavailableOAuthOutbox) Ack(context.Context, string) error {
	return errors.New("redis unavailable")
}
func (unavailableOAuthOutbox) Retry(context.Context, OAuthRecoveryTask, string) error {
	return errors.New("redis unavailable")
}
func (unavailableOAuthOutbox) DeadLetter(context.Context, OAuthRecoveryTask, string) error {
	return errors.New("redis unavailable")
}

type atomicMemoryConnectionRepository struct {
	*memoryConnectionRepository
	outbox    *memoryDurableOAuthOutbox
	deleteErr error
}

func (repository *atomicMemoryConnectionRepository) DeleteWithOAuthRevocation(
	_ context.Context,
	organizationID, connectionID uuid.UUID,
	_ *uuid.UUID,
	task OAuthRecoveryTask,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.deleteErr != nil {
		return repository.deleteErr
	}
	connection := repository.connections[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return ErrConnectionNotFound
	}
	if connection.CredentialVersion != task.CredentialVersion ||
		connection.EncryptedCredentials == nil ||
		*connection.EncryptedCredentials != task.EncryptedCredentials {
		return ErrConnectionChanged
	}
	repository.outbox.store.mu.Lock()
	repository.outbox.store.tasks[task.ID] = task
	delete(repository.connections, connectionID)
	repository.outbox.store.mu.Unlock()
	return nil
}

type snapshotRecoveryRevoker struct {
	mu              sync.Mutex
	currentClient   OAuthClient
	revokeErr       error
	revokeCalls     int
	revokedClientID string
}

func (revoker *snapshotRecoveryRevoker) ResolveRevocationClient(context.Context, *IntegrationConnection) (OAuthClient, error) {
	revoker.mu.Lock()
	defer revoker.mu.Unlock()
	return OAuthClient{
		ClientID:     revoker.currentClient.ClientID,
		ClientSecret: revoker.currentClient.ClientSecret,
		Config:       cloneAnyMap(revoker.currentClient.Config),
	}, nil
}

func (revoker *snapshotRecoveryRevoker) RevokeConnection(ctx context.Context, connection *IntegrationConnection) error {
	client, err := revoker.ResolveRevocationClient(ctx, connection)
	if err != nil {
		return err
	}
	defer client.Destroy()
	return revoker.RevokeConnectionWithClient(ctx, connection, client)
}

func (revoker *snapshotRecoveryRevoker) RevokeConnectionWithClient(
	_ context.Context,
	_ *IntegrationConnection,
	client OAuthClient,
) error {
	revoker.mu.Lock()
	defer revoker.mu.Unlock()
	revoker.revokeCalls++
	revoker.revokedClientID = client.ClientID
	return revoker.revokeErr
}

func TestDurableRevocationSurvivesProviderRedisFailureAndClientConfigChange(t *testing.T) {
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	store := &durableOAuthTaskStore{}
	durableOutbox := newMemoryDurableOAuthOutbox(store)
	repository := &atomicMemoryConnectionRepository{
		memoryConnectionRepository: newMemoryConnectionRepository(),
		outbox:                     durableOutbox,
	}
	connection := encryptedOAuthRecoveryConnection(t, cipher)
	connection.Name = "Durable OAuth"
	connection.CredentialSource = ConnectionCredentialSourceOrganization
	connection.Status = ConnectionStatusActive
	connection.Revision = 1
	connection.HealthRevision = 1
	connection.Config = map[string]any{"tenant": "original"}
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	revoker := &snapshotRecoveryRevoker{
		currentClient: OAuthClient{
			ClientID:     "old-client-id",
			ClientSecret: "old-client-secret",
			Config:       map[string]any{"audience": "old"},
		},
		revokeErr: errors.New("provider unavailable"),
	}
	split := NewSplitOAuthRecoveryOutbox(durableOutbox, unavailableOAuthOutbox{})
	recovery := NewOAuthRecoveryService(split, repository, revoker, cipher)
	service := NewConnectionService(
		repository,
		cipher,
		staticConnectionCatalog{},
		NewConnectionResolver(repository, cipher),
		nil,
	).WithConnectionRevoker(revoker).WithOAuthRecovery(recovery)

	if err := service.DeleteAs(context.Background(), connection.OrganizationID, connection.ID, nil); err != nil {
		t.Fatalf("DeleteAs() error = %v", err)
	}
	if repository.stored(connection.ID) != nil {
		t.Fatal("local connection survived committed durable deletion")
	}
	if durableOutbox.len() != 1 {
		t.Fatalf("durable recovery tasks = %d, want 1", durableOutbox.len())
	}
	durableOutbox.store.mu.Lock()
	for _, queued := range durableOutbox.store.tasks {
		payload, marshalErr := json.Marshal(queued)
		if marshalErr != nil {
			durableOutbox.store.mu.Unlock()
			t.Fatal(marshalErr)
		}
		raw := string(payload)
		if strings.Contains(raw, "plain-access-token") ||
			strings.Contains(raw, "plain-refresh-token") ||
			strings.Contains(raw, "old-client-secret") {
			durableOutbox.store.mu.Unlock()
			t.Fatal("durable OAuth recovery payload contains plaintext secret material")
		}
	}
	durableOutbox.store.mu.Unlock()

	// Simulate both a process restart and an OAuth client-config rotation.
	revoker.mu.Lock()
	revoker.currentClient = OAuthClient{ClientID: "new-client-id", ClientSecret: "new-client-secret"}
	revoker.revokeErr = nil
	revoker.mu.Unlock()
	restartedOutbox := newMemoryDurableOAuthOutbox(store)
	restarted := NewOAuthRecoveryService(
		NewSplitOAuthRecoveryOutbox(restartedOutbox, unavailableOAuthOutbox{}),
		repository,
		revoker,
		cipher,
	)
	// The refresh-token accelerator remains unavailable, so the batch may
	// report that unrelated Redis failure after it has processed the durable
	// database revocation.
	_ = restarted.RecoverBatch(context.Background(), 10)
	revoker.mu.Lock()
	calls, clientID := revoker.revokeCalls, revoker.revokedClientID
	revoker.mu.Unlock()
	if calls != 2 {
		t.Fatalf("provider revoke calls = %d, want immediate plus recovered", calls)
	}
	if clientID != "old-client-id" {
		t.Fatalf("recovered revocation used client %q, want immutable old-client-id", clientID)
	}
	if restartedOutbox.len() != 0 {
		t.Fatal("successfully recovered durable revocation was not acknowledged")
	}
}

func TestAtomicOAuthDeletionFailureNeverCallsProviderOrQueues(t *testing.T) {
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	store := &durableOAuthTaskStore{}
	durableOutbox := newMemoryDurableOAuthOutbox(store)
	repository := &atomicMemoryConnectionRepository{
		memoryConnectionRepository: newMemoryConnectionRepository(),
		outbox:                     durableOutbox,
		deleteErr:                  errors.New("database unavailable"),
	}
	connection := encryptedOAuthRecoveryConnection(t, cipher)
	connection.Name = "Undeletable OAuth"
	connection.CredentialSource = ConnectionCredentialSourceOrganization
	connection.Status = ConnectionStatusActive
	connection.Revision = 1
	connection.HealthRevision = 1
	if err := repository.Create(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	revoker := &snapshotRecoveryRevoker{
		currentClient: OAuthClient{ClientID: "client-id", ClientSecret: "client-secret"},
		revokeErr:     errors.New("must not be called"),
	}
	recovery := NewOAuthRecoveryService(
		NewSplitOAuthRecoveryOutbox(durableOutbox, unavailableOAuthOutbox{}),
		repository,
		revoker,
		cipher,
	)
	service := NewConnectionService(
		repository,
		cipher,
		staticConnectionCatalog{},
		NewConnectionResolver(repository, cipher),
		nil,
	).WithConnectionRevoker(revoker).WithOAuthRecovery(recovery)

	if err := service.DeleteAs(context.Background(), connection.OrganizationID, connection.ID, nil); err == nil {
		t.Fatal("DeleteAs() accepted a database transaction failure")
	}
	revoker.mu.Lock()
	calls := revoker.revokeCalls
	revoker.mu.Unlock()
	if calls != 0 {
		t.Fatalf("provider revoke calls = %d, want 0", calls)
	}
	if durableOutbox.len() != 0 {
		t.Fatal("failed local deletion queued a durable provider operation")
	}
	if repository.stored(connection.ID) == nil {
		t.Fatal("failed local deletion removed the connection")
	}
}

func TestDatabaseOAuthRecoveryOutboxClaimsWithCrossInstanceLease(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	connection := encryptedOAuthRecoveryConnection(t, cipher)
	task, err := newOAuthRevocationRecoveryTask(connection, "v2.test.client-envelope", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC)
	record, err := oauthRecoveryRecord(task, now)
	if err != nil {
		t.Fatal(err)
	}
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	outbox.now = func() time.Time { return now }
	outbox.workerID = uuid.New()
	outbox.leaseDuration = time.Minute

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "integration_oauth_recovery_operations".*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kind", "organization_id", "connection_id", "integration_id", "driver_id",
			"auth_method_id", "payload", "status", "attempts", "available_at", "created_at", "updated_at",
		}).AddRow(
			record.ID, record.Kind, record.OrganizationID, record.ConnectionID, record.IntegrationID,
			record.DriverID, record.AuthMethodID, []byte(record.Payload), oauthRecoveryStatusPending,
			0, now, task.CreatedAt, now,
		))
	mock.ExpectExec(`UPDATE "integration_oauth_recovery_operations" SET .*"lease_owner"=.*"lease_until"=.*"status"=.*WHERE id IN`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	claimed, err := outbox.Claim(context.Background(), 1)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != task.ID {
		t.Fatalf("Claim() = %#v", claimed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDatabaseOAuthRecoveryOutboxNeverAutoDeletesDeadLettersWhileClaiming(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	outbox.now = func() time.Time { return time.Date(2026, 8, 23, 16, 0, 0, 0, time.UTC) }

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "integration_oauth_recovery_operations".*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	claimed, err := outbox.Claim(context.Background(), 10)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("Claim() = %#v, want no tasks", claimed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDatabaseOAuthRecoveryOutboxAckRequiresCurrentLeaseOrPendingTask(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	outbox.workerID = uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "integration_oauth_recovery_operations".*lease_owner`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := outbox.Ack(context.Background(), "revoke-owned"); err == nil {
		t.Fatal("Ack() accepted a task owned by another worker")
	}

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "integration_oauth_recovery_operations".*lease_owner`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := outbox.Ack(context.Background(), "revoke-pending"); err != nil {
		t.Fatalf("Ack() pending immediate task error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDatabaseOAuthRecoveryOutboxDeadLetterRejectsLostLease(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	outbox.workerID = uuid.New()
	task := OAuthRecoveryTask{ID: "revoke-owned"}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "integration_oauth_recovery_operations".*lease_owner`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := outbox.DeadLetter(context.Background(), task, "upstream"); err == nil {
		t.Fatal("DeadLetter() accepted a task owned by another worker")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDatabaseOAuthRecoveryOutboxRejectsRetryAfterLeaseOwnershipChanges(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	task, err := newOAuthRevocationRecoveryTask(
		encryptedOAuthRecoveryConnection(t, cipher),
		"v2.test.client-envelope",
		1,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	outbox.workerID = uuid.New()
	outbox.now = func() time.Time { return task.CreatedAt.Add(time.Minute) }
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "integration_oauth_recovery_operations" SET .*lease_owner.*WHERE .*lease_owner`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := outbox.Retry(context.Background(), task, "provider_unavailable"); err == nil {
		t.Fatal("Retry() accepted a task leased by another worker")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDatabaseOAuthRecoverySummaryIsSecretFreeAndRetainsUnacknowledgedDeadLetters(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	organizationID := uuid.New()
	failedAt := time.Date(2026, 7, 23, 18, 0, 0, 0, time.UTC)
	reason := oauthRecoveryManualReason

	mock.ExpectQuery(`SELECT count\(\*\) FROM "integration_oauth_recovery_operations".*status IN`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "integration_oauth_recovery_operations".*acknowledged_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "integration_oauth_recovery_operations".*last_error_code`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT \* FROM "integration_oauth_recovery_operations".*ORDER BY dead_lettered_at DESC`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kind", "organization_id", "connection_id", "integration_id", "driver_id",
			"auth_method_id", "status", "attempts", "last_error_code", "dead_lettered_at",
			"created_at", "updated_at",
		}).AddRow(
			"revoke-safe-reference", string(OAuthRecoveryRevoke), organizationID, uuid.New(),
			"feishu", "feishu-rest", "user_oauth", oauthRecoveryStatusDeadLetter, 1,
			reason, failedAt, failedAt.Add(-time.Minute), failedAt,
		))

	summary, err := outbox.OAuthRecoverySummary(context.Background(), organizationID, 20)
	if err != nil {
		t.Fatalf("OAuthRecoverySummary() error = %v", err)
	}
	if summary.PendingRevocations != 2 || summary.UnresolvedDeadLetters != 1 ||
		summary.ManualActionRequired != 1 || summary.FailedRevocations != 0 ||
		len(summary.RemediationOperations) != 1 {
		t.Fatalf("OAuthRecoverySummary() = %#v", summary)
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"encrypted_credentials", "client_secret", "connection_id", "payload"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("OAuth recovery summary leaked %q: %s", forbidden, payload)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDatabaseOAuthRecoveryAcknowledgementRequiresExplicitResolution(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	outbox.now = func() time.Time { return time.Date(2026, 7, 23, 19, 0, 0, 0, time.UTC) }
	organizationID, actorID := uuid.New(), uuid.New()

	if err := outbox.AcknowledgeOAuthRecovery(
		context.Background(),
		organizationID,
		"revoke-safe-reference",
		actorID,
		"ignore",
	); err == nil {
		t.Fatal("AcknowledgeOAuthRecovery() accepted an unsafe resolution")
	}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "integration_oauth_recovery_operations".*acknowledged_at.*acknowledged_by.*payload.*resolution_code`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := outbox.AcknowledgeOAuthRecovery(
		context.Background(),
		organizationID,
		"revoke-safe-reference",
		actorID,
		OAuthRecoveryResolutionProviderAccessRemoved,
	); err != nil {
		t.Fatalf("AcknowledgeOAuthRecovery() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDatabaseOAuthRecoveryAcknowledgementAtomicallyRedactsPayloadAndKeepsAuditFields(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	now := time.Date(2026, 7, 23, 20, 0, 0, 0, time.UTC)
	outbox.now = func() time.Time { return now }
	organizationID, actorID := uuid.New(), uuid.New()
	operationRef := "revoke-redaction-audit"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "integration_oauth_recovery_operations" SET "acknowledged_at"=\$1,"acknowledged_by"=\$2,"payload"=\$3,"resolution_code"=\$4,"updated_at"=\$5 WHERE id = \$6 AND organization_id = \$7 AND kind = \$8 AND status = \$9 AND acknowledged_at IS NULL`).
		WithArgs(
			now,
			actorID,
			nil,
			OAuthRecoveryResolutionProviderAccessRemoved,
			now,
			operationRef,
			organizationID,
			OAuthRecoveryRevoke,
			oauthRecoveryStatusDeadLetter,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := outbox.AcknowledgeOAuthRecovery(
		context.Background(),
		organizationID,
		operationRef,
		actorID,
		OAuthRecoveryResolutionProviderAccessRemoved,
	); err != nil {
		t.Fatalf("AcknowledgeOAuthRecovery() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDatabaseOAuthRecoveryAcknowledgementRejectsCrossOrganizationAndInvalidResolution(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	now := time.Date(2026, 7, 23, 20, 30, 0, 0, time.UTC)
	outbox.now = func() time.Time { return now }
	organizationID, otherOrganizationID, actorID := uuid.New(), uuid.New(), uuid.New()
	operationRef := "revoke-denied-ack"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "integration_oauth_recovery_operations".*organization_id`).
		WithArgs(
			now,
			actorID,
			nil,
			OAuthRecoveryResolutionTokenExpired,
			now,
			operationRef,
			otherOrganizationID,
			OAuthRecoveryRevoke,
			oauthRecoveryStatusDeadLetter,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := outbox.AcknowledgeOAuthRecovery(
		context.Background(),
		otherOrganizationID,
		operationRef,
		actorID,
		OAuthRecoveryResolutionTokenExpired,
	); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("cross-organization acknowledgement error = %v, want ErrConnectionNotFound", err)
	}
	if err := outbox.AcknowledgeOAuthRecovery(
		context.Background(),
		organizationID,
		operationRef,
		actorID,
		"ignore",
	); err == nil {
		t.Fatal("acknowledgement accepted an invalid resolution")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestAcknowledgedOAuthRecoveryTombstoneCannotDecodeAsActionableTask(t *testing.T) {
	acknowledgedAt := time.Date(2026, 7, 23, 21, 0, 0, 0, time.UTC)
	actorID := uuid.New()
	resolution := OAuthRecoveryResolutionProviderAccessRemoved
	record := IntegrationOAuthRecoveryOperation{
		ID:             "revoke-secret-free-tombstone",
		Kind:           string(OAuthRecoveryRevoke),
		OrganizationID: uuid.New(),
		ConnectionID:   uuid.New(),
		IntegrationID:  "feishu",
		DriverID:       "feishu-rest",
		AuthMethodID:   "user_oauth",
		Payload:        nil,
		Status:         oauthRecoveryStatusDeadLetter,
		Attempts:       7,
		DeadLetteredAt: &acknowledgedAt,
		AcknowledgedAt: &acknowledgedAt,
		AcknowledgedBy: &actorID,
		ResolutionCode: &resolution,
		CreatedAt:      acknowledgedAt.Add(-time.Hour),
		UpdatedAt:      acknowledgedAt,
	}
	if _, err := decodeOAuthRecoveryRecord(record); err == nil {
		t.Fatal("decodeOAuthRecoveryRecord() accepted an acknowledged secret-free tombstone as an actionable task")
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"encrypted_credentials",
		"encrypted_client_credentials",
		"encrypted-access-marker",
		"encrypted-client-secret-marker",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("acknowledged OAuth recovery audit tombstone retained %q: %s", forbidden, encoded)
		}
	}
	if record.OrganizationID == uuid.Nil ||
		record.IntegrationID != "feishu" ||
		record.AuthMethodID != "user_oauth" ||
		record.Attempts != 7 ||
		record.AcknowledgedBy == nil ||
		*record.AcknowledgedBy != actorID ||
		record.ResolutionCode == nil ||
		*record.ResolutionCode != resolution {
		t.Fatalf("secret-free OAuth recovery tombstone lost audit history: %#v", record)
	}
}

func TestDatabaseOAuthRecoverySummaryExcludesAcknowledgedAuditTombstones(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	outbox := NewDatabaseOAuthRecoveryOutbox(db)
	organizationID := uuid.New()

	mock.ExpectQuery(`SELECT count\(\*\) FROM "integration_oauth_recovery_operations".*status IN`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "integration_oauth_recovery_operations".*acknowledged_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "integration_oauth_recovery_operations".*last_error_code`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT \* FROM "integration_oauth_recovery_operations".*ORDER BY dead_lettered_at DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	summary, err := outbox.OAuthRecoverySummary(context.Background(), organizationID, 20)
	if err != nil {
		t.Fatalf("OAuthRecoverySummary() error = %v", err)
	}
	if summary.PendingRevocations != 0 ||
		summary.UnresolvedDeadLetters != 0 ||
		summary.ManualActionRequired != 0 ||
		summary.FailedRevocations != 0 ||
		len(summary.RemediationOperations) != 0 {
		t.Fatalf("OAuthRecoverySummary() included acknowledged tombstone: %#v", summary)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
