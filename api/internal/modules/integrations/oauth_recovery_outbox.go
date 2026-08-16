package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	oauthRecoveryPendingKey        = "zgi:integration:oauth-recovery:pending"
	oauthRecoveryProcessingKey     = "zgi:integration:oauth-recovery:processing"
	oauthRecoveryDeadLetterKey     = "zgi:integration:oauth-recovery:dead-letter"
	oauthRecoveryPayloadKeyBase    = "zgi:integration:oauth-recovery:payload:"
	oauthRecoverySchema            = "zgi.integration_oauth_recovery.v1"
	oauthRecoveryLease             = 30 * time.Second
	oauthRecoveryRetention         = 7 * 24 * time.Hour
	oauthRecoveryDeadLetterTTL     = 30 * 24 * time.Hour
	oauthRecoveryInterval          = 5 * time.Second
	oauthRecoveryOperationTimeout  = 20 * time.Second
	oauthRecoveryMaxAttempts       = 168
	oauthRecoveryDeadLetterMaxSize = 1000
)

type OAuthRecoveryTaskKind string

const (
	OAuthRecoveryRevoke  OAuthRecoveryTaskKind = "revoke"
	OAuthRecoveryRefresh OAuthRecoveryTaskKind = "refresh"
)

var claimOAuthRecoveryTasksScript = redis.NewScript(`
local pending = KEYS[1]
local processing = KEYS[2]
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)
local lease_until = now + tonumber(ARGV[1])
local limit = tonumber(ARGV[2])

local expired = redis.call('ZRANGEBYSCORE', processing, '-inf', now, 'LIMIT', 0, limit)
for _, id in ipairs(expired) do
  redis.call('ZREM', processing, id)
  redis.call('ZADD', pending, now, id)
end

local candidates = redis.call('ZRANGEBYSCORE', pending, '-inf', now, 'LIMIT', 0, limit)
local claimed = {}
for _, id in ipairs(candidates) do
  if redis.call('ZREM', pending, id) == 1 then
    redis.call('ZADD', processing, lease_until, id)
    table.insert(claimed, id)
  end
end
return claimed
`)

var enqueueOAuthRecoveryTaskScript = redis.NewScript(`
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
redis.call('ZREM', KEYS[3], ARGV[1])
redis.call('ZADD', KEYS[2], now, ARGV[1])
return 1
`)

// OAuthRecoveryTask contains only an encrypted credential envelope and the
// metadata required to bind its AAD and route it back to the owning provider.
// Plaintext OAuth tokens must never be added to this structure.
type OAuthRecoveryTask struct {
	SchemaVersion              string                `json:"schema_version"`
	ID                         string                `json:"id"`
	Kind                       OAuthRecoveryTaskKind `json:"kind"`
	OrganizationID             uuid.UUID             `json:"organization_id"`
	ConnectionID               uuid.UUID             `json:"connection_id"`
	IntegrationID              string                `json:"integration_id"`
	DriverID                   string                `json:"driver_id"`
	AuthMethodID               string                `json:"auth_method_id"`
	ExpectedConnectionRevision int                   `json:"expected_connection_revision,omitempty"`
	ExpectedCredentialVersion  int                   `json:"expected_credential_version,omitempty"`
	CredentialVersion          int                   `json:"credential_version"`
	EncryptedCredentials       string                `json:"encrypted_credentials"`
	EncryptedClientCredentials string                `json:"encrypted_client_credentials,omitempty"`
	ClientCredentialVersion    int                   `json:"client_credential_version,omitempty"`
	ClientConfig               map[string]any        `json:"client_config,omitempty"`
	ConnectionConfig           map[string]any        `json:"connection_config,omitempty"`
	GrantedScopes              []string              `json:"granted_scopes,omitempty"`
	TokenExpiresAt             *time.Time            `json:"token_expires_at,omitempty"`
	RefreshTokenExpiresAt      *time.Time            `json:"refresh_token_expires_at,omitempty"`
	NextTokenRefreshAt         *time.Time            `json:"next_token_refresh_at,omitempty"`
	AuthStatus                 ConnectionAuthStatus  `json:"auth_status,omitempty"`
	ScopeStatus                ConnectionScopeStatus `json:"scope_status,omitempty"`
	AttentionCode              *string               `json:"attention_code,omitempty"`
	LastErrorCode              *string               `json:"last_error_code,omitempty"`
	CompensationFlowID         *uuid.UUID            `json:"compensation_flow_id,omitempty"`
	Attempts                   int                   `json:"attempts"`
	CreatedAt                  time.Time             `json:"created_at"`
}

type oauthRecoveryDeadLetter struct {
	FailedAt time.Time         `json:"failed_at"`
	Reason   string            `json:"reason"`
	Task     OAuthRecoveryTask `json:"task"`
}

type OAuthRecoveryOutbox interface {
	Enqueue(context.Context, OAuthRecoveryTask) error
	Claim(context.Context, int64) ([]OAuthRecoveryTask, error)
	Get(context.Context, string) (*OAuthRecoveryTask, error)
	Ack(context.Context, string) error
	Retry(context.Context, OAuthRecoveryTask, string) error
	DeadLetter(context.Context, OAuthRecoveryTask, string) error
}

type RedisOAuthRecoveryOutbox struct {
	client        *redis.Client
	now           func() time.Time
	leaseDuration time.Duration
	retention     time.Duration
	maxAttempts   int
	deadLetterTTL time.Duration
	deadLetterMax int64
}

func NewRedisOAuthRecoveryOutbox(client *redis.Client) *RedisOAuthRecoveryOutbox {
	return &RedisOAuthRecoveryOutbox{
		client:        client,
		now:           func() time.Time { return time.Now().UTC() },
		leaseDuration: oauthRecoveryLease,
		retention:     oauthRecoveryRetention,
		maxAttempts:   oauthRecoveryMaxAttempts,
		deadLetterTTL: oauthRecoveryDeadLetterTTL,
		deadLetterMax: oauthRecoveryDeadLetterMaxSize,
	}
}

func (outbox *RedisOAuthRecoveryOutbox) Enqueue(ctx context.Context, task OAuthRecoveryTask) error {
	if outbox == nil || outbox.client == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	task = normalizeOAuthRecoveryTask(task, outbox.now())
	if err := validateOAuthRecoveryTask(task); err != nil {
		return err
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("encode integration OAuth recovery task: %w", err)
	}
	retention := outbox.retention
	if retention <= 0 {
		retention = oauthRecoveryRetention
	}
	_, err = enqueueOAuthRecoveryTaskScript.Run(
		ctx,
		outbox.client,
		[]string{oauthRecoveryPayloadKey(task.ID), oauthRecoveryPendingKey, oauthRecoveryProcessingKey},
		task.ID,
		payload,
		retention.Milliseconds(),
	).Result()
	if err != nil {
		return fmt.Errorf("enqueue integration OAuth recovery task: %w", err)
	}
	return nil
}

func (outbox *RedisOAuthRecoveryOutbox) Claim(ctx context.Context, limit int64) ([]OAuthRecoveryTask, error) {
	if outbox == nil || outbox.client == nil {
		return nil, fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	if limit <= 0 {
		return nil, nil
	}
	leaseDuration := outbox.leaseDuration
	if leaseDuration <= 0 {
		leaseDuration = oauthRecoveryLease
	}
	ids, err := claimOAuthRecoveryTasksScript.Run(
		ctx,
		outbox.client,
		[]string{oauthRecoveryPendingKey, oauthRecoveryProcessingKey},
		leaseDuration.Milliseconds(),
		limit,
	).StringSlice()
	if err != nil {
		return nil, fmt.Errorf("claim integration OAuth recovery tasks: %w", err)
	}
	tasks := make([]OAuthRecoveryTask, 0, len(ids))
	var claimErr error
	for _, id := range ids {
		task, getErr := outbox.Get(ctx, id)
		if getErr == nil {
			tasks = append(tasks, *task)
			continue
		}
		if errors.Is(getErr, redis.Nil) {
			_ = outbox.remove(ctx, id)
			continue
		}
		claimErr = errors.Join(claimErr, getErr)
	}
	return tasks, claimErr
}

func (outbox *RedisOAuthRecoveryOutbox) Get(ctx context.Context, id string) (*OAuthRecoveryTask, error) {
	if outbox == nil || outbox.client == nil {
		return nil, fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("integration OAuth recovery task id is required")
	}
	payload, err := outbox.client.Get(ctx, oauthRecoveryPayloadKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, redis.Nil
		}
		return nil, fmt.Errorf("read integration OAuth recovery task: %w", err)
	}
	var task OAuthRecoveryTask
	if err := json.Unmarshal(payload, &task); err != nil {
		_ = outbox.deadLetterRaw(ctx, id, "invalid_payload", payload)
		return nil, fmt.Errorf("decode integration OAuth recovery task: %w", err)
	}
	if err := validateOAuthRecoveryTask(task); err != nil {
		_ = outbox.deadLetterRaw(ctx, id, "invalid_task", payload)
		return nil, err
	}
	return &task, nil
}

func (outbox *RedisOAuthRecoveryOutbox) Ack(ctx context.Context, id string) error {
	if outbox == nil || outbox.client == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	return outbox.remove(ctx, strings.TrimSpace(id))
}

func (outbox *RedisOAuthRecoveryOutbox) Retry(ctx context.Context, task OAuthRecoveryTask, reason string) error {
	if outbox == nil || outbox.client == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	task.Attempts++
	maxAttempts := outbox.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = oauthRecoveryMaxAttempts
	}
	retention := outbox.retention
	if retention <= 0 {
		retention = oauthRecoveryRetention
	}
	if task.Attempts >= maxAttempts || outbox.now().Sub(task.CreatedAt) >= retention {
		return outbox.DeadLetter(ctx, task, reason)
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("encode integration OAuth recovery retry: %w", err)
	}
	delay := oauthRecoveryRetryDelay(task.Attempts)
	remaining := retention - outbox.now().Sub(task.CreatedAt)
	if remaining <= 0 {
		return outbox.DeadLetter(ctx, task, reason)
	}
	_, err = outbox.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, oauthRecoveryPayloadKey(task.ID), payload, remaining)
		pipe.ZRem(ctx, oauthRecoveryProcessingKey, task.ID)
		pipe.ZAdd(ctx, oauthRecoveryPendingKey, redis.Z{
			Score:  float64(outbox.now().Add(delay).UnixMilli()),
			Member: task.ID,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("retry integration OAuth recovery task: %w", err)
	}
	return nil
}

func (outbox *RedisOAuthRecoveryOutbox) DeadLetter(ctx context.Context, task OAuthRecoveryTask, reason string) error {
	record, err := json.Marshal(oauthRecoveryDeadLetter{
		FailedAt: outbox.now(),
		Reason:   oauthRecoverySafeReason(reason),
		Task:     task,
	})
	if err != nil {
		return fmt.Errorf("encode integration OAuth recovery dead letter: %w", err)
	}
	maxSize := outbox.deadLetterMax
	if maxSize <= 0 {
		maxSize = oauthRecoveryDeadLetterMaxSize
	}
	ttl := outbox.deadLetterTTL
	if ttl <= 0 {
		ttl = oauthRecoveryDeadLetterTTL
	}
	_, err = outbox.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, oauthRecoveryDeadLetterKey, record)
		pipe.LTrim(ctx, oauthRecoveryDeadLetterKey, 0, maxSize-1)
		pipe.ExpireNX(ctx, oauthRecoveryDeadLetterKey, ttl)
		pipe.Del(ctx, oauthRecoveryPayloadKey(task.ID))
		pipe.ZRem(ctx, oauthRecoveryPendingKey, task.ID)
		pipe.ZRem(ctx, oauthRecoveryProcessingKey, task.ID)
		return nil
	})
	if err != nil {
		return fmt.Errorf("dead-letter integration OAuth recovery task: %w", err)
	}
	return nil
}

func (outbox *RedisOAuthRecoveryOutbox) deadLetterRaw(ctx context.Context, id, reason string, payload []byte) error {
	// Corrupt payloads must not be copied into a long-lived dead letter because
	// their schema cannot prove that they contain only encrypted credentials.
	record, err := json.Marshal(map[string]any{
		"failed_at": outbox.now(),
		"reason":    oauthRecoverySafeReason(reason),
		"task_id":   id,
	})
	if err != nil {
		return err
	}
	_ = payload
	maxSize := outbox.deadLetterMax
	if maxSize <= 0 {
		maxSize = oauthRecoveryDeadLetterMaxSize
	}
	ttl := outbox.deadLetterTTL
	if ttl <= 0 {
		ttl = oauthRecoveryDeadLetterTTL
	}
	_, err = outbox.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, oauthRecoveryDeadLetterKey, record)
		pipe.LTrim(ctx, oauthRecoveryDeadLetterKey, 0, maxSize-1)
		pipe.ExpireNX(ctx, oauthRecoveryDeadLetterKey, ttl)
		pipe.Del(ctx, oauthRecoveryPayloadKey(id))
		pipe.ZRem(ctx, oauthRecoveryPendingKey, id)
		pipe.ZRem(ctx, oauthRecoveryProcessingKey, id)
		return nil
	})
	return err
}

func (outbox *RedisOAuthRecoveryOutbox) remove(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := outbox.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, oauthRecoveryPayloadKey(id))
		pipe.ZRem(ctx, oauthRecoveryPendingKey, id)
		pipe.ZRem(ctx, oauthRecoveryProcessingKey, id)
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete integration OAuth recovery task: %w", err)
	}
	return nil
}

func normalizeOAuthRecoveryTask(task OAuthRecoveryTask, now time.Time) OAuthRecoveryTask {
	task.SchemaVersion = oauthRecoverySchema
	task.IntegrationID = strings.ToLower(strings.TrimSpace(task.IntegrationID))
	task.DriverID = strings.ToLower(strings.TrimSpace(task.DriverID))
	task.AuthMethodID = strings.ToLower(strings.TrimSpace(task.AuthMethodID))
	task.EncryptedCredentials = strings.TrimSpace(task.EncryptedCredentials)
	task.EncryptedClientCredentials = strings.TrimSpace(task.EncryptedClientCredentials)
	task.ClientConfig = cloneAnyMap(task.ClientConfig)
	task.ConnectionConfig = cloneAnyMap(task.ConnectionConfig)
	task.GrantedScopes = normalizeScopes(task.GrantedScopes)
	task.TokenExpiresAt = cloneTimePointer(task.TokenExpiresAt)
	task.RefreshTokenExpiresAt = cloneTimePointer(task.RefreshTokenExpiresAt)
	task.NextTokenRefreshAt = cloneTimePointer(task.NextTokenRefreshAt)
	task.AttentionCode = cloneStringPointer(task.AttentionCode)
	task.LastErrorCode = cloneStringPointer(task.LastErrorCode)
	task.CompensationFlowID = cloneUUIDPointer(task.CompensationFlowID)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now.UTC()
	}
	return task
}

func validateOAuthRecoveryTask(task OAuthRecoveryTask) error {
	if task.SchemaVersion != oauthRecoverySchema ||
		strings.TrimSpace(task.ID) == "" ||
		task.OrganizationID == uuid.Nil ||
		task.ConnectionID == uuid.Nil ||
		!integrationIdentifierPattern.MatchString(task.IntegrationID) ||
		!integrationIdentifierPattern.MatchString(task.DriverID) ||
		!integrationIdentifierPattern.MatchString(task.AuthMethodID) ||
		task.CredentialVersion < 1 ||
		task.EncryptedCredentials == "" ||
		len(task.EncryptedCredentials) > 512*1024 ||
		len(task.EncryptedClientCredentials) > 512*1024 ||
		len(task.GrantedScopes) > 256 ||
		task.Attempts < 0 ||
		task.Attempts > oauthRecoveryMaxAttempts ||
		task.CreatedAt.IsZero() {
		return fmt.Errorf("integration OAuth recovery task is invalid")
	}
	switch task.Kind {
	case OAuthRecoveryRevoke:
		if task.ExpectedCredentialVersion != 0 {
			return fmt.Errorf("integration OAuth revocation recovery task is invalid")
		}
		if task.EncryptedClientCredentials != "" && task.ClientCredentialVersion < 1 {
			return fmt.Errorf("integration OAuth revocation client snapshot is invalid")
		}
		if task.EncryptedClientCredentials == "" && task.ClientCredentialVersion != 0 {
			return fmt.Errorf("integration OAuth revocation client snapshot is invalid")
		}
		if validateConnectionConfig(task.ClientConfig) != nil ||
			validateConnectionConfig(task.ConnectionConfig) != nil {
			return fmt.Errorf("integration OAuth revocation config snapshot is invalid")
		}
		if task.ID != oauthRevocationRecoveryTaskID(task.OrganizationID, task.ConnectionID, task.CredentialVersion) {
			return fmt.Errorf("integration OAuth revocation recovery task id is invalid")
		}
		if task.CompensationFlowID != nil && *task.CompensationFlowID != task.ConnectionID {
			return fmt.Errorf("integration OAuth revocation flow guard is invalid")
		}
	case OAuthRecoveryRefresh:
		if task.ExpectedCredentialVersion < 1 ||
			task.CredentialVersion != task.ExpectedCredentialVersion+1 ||
			task.ID != oauthRefreshRecoveryTaskID(task.OrganizationID, task.ConnectionID, task.ExpectedCredentialVersion) ||
			task.AuthStatus != ConnectionAuthValid ||
			task.ScopeStatus != ConnectionScopeVerified {
			return fmt.Errorf("integration OAuth refresh recovery task is invalid")
		}
	default:
		return fmt.Errorf("integration OAuth recovery task kind is invalid")
	}
	return nil
}

func oauthRecoveryPayloadKey(id string) string {
	return oauthRecoveryPayloadKeyBase + strings.TrimSpace(id)
}

func oauthRecoveryRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second << min(attempt-1, 12)
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func oauthRecoverySafeReason(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if !integrationIdentifierPattern.MatchString(reason) {
		return "recovery_failed"
	}
	return reason
}

// OAuthRecoveryService durably recovers provider revocations and encrypted
// rotating-token updates. It never receives or stores plaintext OAuth tokens.
type OAuthRecoveryService struct {
	outbox     OAuthRecoveryOutbox
	repository ConnectionRepository
	revoker    ConnectionRevoker
	cipher     CredentialCipher
	flows      interface {
		GetByID(context.Context, uuid.UUID) (*IntegrationOAuthFlow, error)
	}
}

var (
	errOAuthManualRevocationRequired = errors.New("OAuth provider access must be removed manually")
	errOAuthCompensationFlowPending  = errors.New("OAuth compensation flow is still pending")
)

type oauthRecoverySnapshotRevoker interface {
	ResolveRevocationClient(context.Context, *IntegrationConnection) (OAuthClient, error)
	RevokeConnectionWithClient(context.Context, *IntegrationConnection, OAuthClient) error
}

func NewOAuthRecoveryService(outbox OAuthRecoveryOutbox, repository ConnectionRepository, revoker ConnectionRevoker, cipher CredentialCipher) *OAuthRecoveryService {
	return &OAuthRecoveryService{outbox: outbox, repository: repository, revoker: revoker, cipher: cipher}
}

func (service *OAuthRecoveryService) WithFlowRepository(repository interface {
	GetByID(context.Context, uuid.UUID) (*IntegrationOAuthFlow, error)
}) *OAuthRecoveryService {
	if service != nil {
		service.flows = repository
	}
	return service
}

// DurableRevocationReady reports whether an OAuth token issued during a
// callback can be encrypted, queued, guarded by its flow state, and later
// revoked after a process restart. OAuthFlowService checks this before calling
// the provider token endpoint so a wiring error cannot downgrade compensation
// to a best-effort in-process revoke.
func (service *OAuthRecoveryService) DurableRevocationReady() bool {
	return service != nil &&
		service.outbox != nil &&
		service.cipher != nil &&
		service.revoker != nil &&
		service.flows != nil
}

func (service *OAuthRecoveryService) EnqueueRevocation(ctx context.Context, connection *IntegrationConnection) error {
	if service == nil || service.outbox == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	var task OAuthRecoveryTask
	var err error
	if _, ok := service.revoker.(oauthRecoverySnapshotRevoker); ok {
		task, err = service.PrepareRevocation(ctx, connection)
	} else {
		// Compatibility for tests and legacy non-OAuthConnectionRevoker
		// implementations. Production durable deletion requires a snapshot.
		task, err = newOAuthRevocationRecoveryTask(connection, "", 0, nil)
	}
	if err != nil {
		return err
	}
	return service.outbox.Enqueue(ctx, task)
}

// PrepareRevocation snapshots the OAuth application identity and secrets into
// a task-specific encrypted envelope. A later client-config rotation or
// deletion therefore cannot make an already-committed revocation impossible.
func (service *OAuthRecoveryService) PrepareRevocation(ctx context.Context, connection *IntegrationConnection) (OAuthRecoveryTask, error) {
	if service == nil || service.cipher == nil {
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth recovery is unavailable")
	}
	snapshotter, ok := service.revoker.(oauthRecoverySnapshotRevoker)
	if !ok {
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth revocation snapshot is unavailable")
	}
	client, err := snapshotter.ResolveRevocationClient(ctx, connection)
	if err != nil {
		return OAuthRecoveryTask{}, err
	}
	defer client.Destroy()
	return service.prepareRevocationWithClient(connection, client)
}

func (service *OAuthRecoveryService) PrepareUncommittedRevocation(
	flow *IntegrationOAuthFlow,
	tokens OAuthTokenSet,
	client OAuthClient,
) (OAuthRecoveryTask, error) {
	if service == nil || service.cipher == nil || flow == nil {
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth recovery is unavailable")
	}
	credentials := tokens.credentialMap()
	if strings.TrimSpace(credentials["access_token"]) == "" && strings.TrimSpace(credentials["refresh_token"]) == "" {
		destroyCredentialMap(credentials)
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth revocation token is unavailable")
	}
	defer destroyCredentialMap(credentials)
	envelope, err := service.cipher.EncryptCredentials(credentials, CredentialAAD{
		OrganizationID: flow.OrganizationID, ConnectionID: flow.ID,
		IntegrationID: flow.IntegrationID, CredentialVersion: 1,
	})
	if err != nil {
		return OAuthRecoveryTask{}, fmt.Errorf("protect uncommitted integration OAuth token: %w", err)
	}
	connection := &IntegrationConnection{
		ID: flow.ID, OrganizationID: flow.OrganizationID,
		IntegrationID: flow.IntegrationID, DriverID: flow.DriverID,
		AuthType: ConnectionAuthTypeOAuth2, AuthMethodID: flow.AuthMethodID,
		EncryptedCredentials: &envelope, CredentialVersion: 1, Revision: 1,
		Config: cloneAnyMap(client.Config),
	}
	task, err := service.prepareRevocationWithClient(connection, client)
	if err != nil {
		return OAuthRecoveryTask{}, err
	}
	task.CompensationFlowID = cloneUUIDPointer(&flow.ID)
	task = normalizeOAuthRecoveryTask(task, task.CreatedAt)
	return task, validateOAuthRecoveryTask(task)
}

func (service *OAuthRecoveryService) prepareRevocationWithClient(
	connection *IntegrationConnection,
	client OAuthClient,
) (OAuthRecoveryTask, error) {
	if service == nil || service.cipher == nil || connection == nil {
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth recovery is unavailable")
	}
	clientCredentials := map[string]string{"client_id": strings.TrimSpace(client.ClientID)}
	if strings.TrimSpace(client.ClientSecret) != "" {
		clientCredentials["client_secret"] = client.ClientSecret
	}
	if clientCredentials["client_id"] == "" {
		destroyCredentialMap(clientCredentials)
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth revocation client is incomplete")
	}
	clientCredentialVersion := 1
	clientEnvelope, err := service.cipher.EncryptCredentials(
		clientCredentials,
		oauthRecoveryClientAAD(connection.OrganizationID, connection.ID, connection.IntegrationID, connection.AuthMethodID, clientCredentialVersion),
	)
	destroyCredentialMap(clientCredentials)
	if err != nil {
		return OAuthRecoveryTask{}, fmt.Errorf("protect integration OAuth revocation client: %w", err)
	}
	return newOAuthRevocationRecoveryTask(connection, clientEnvelope, clientCredentialVersion, client.Config)
}

func (service *OAuthRecoveryService) EnqueuePreparedRevocation(ctx context.Context, task OAuthRecoveryTask) error {
	if service == nil || service.outbox == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	return service.outbox.Enqueue(ctx, task)
}

func (service *OAuthRecoveryService) AcknowledgePreparedRevocation(ctx context.Context, taskID string) error {
	if service == nil || service.outbox == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	return service.outbox.Ack(ctx, taskID)
}

// AttemptPreparedRevocation performs the immediate best-effort provider call.
// The durable task already exists at this point; failures intentionally leave
// it pending for a later leased worker.
func (service *OAuthRecoveryService) AttemptPreparedRevocation(ctx context.Context, task OAuthRecoveryTask) error {
	if service == nil || service.outbox == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	if err := service.apply(ctx, task); err != nil {
		return err
	}
	return service.outbox.Ack(ctx, task.ID)
}

func (service *OAuthRecoveryService) EnqueueRefresh(ctx context.Context, connection *IntegrationConnection, expectedCredentialVersion int) error {
	if service == nil || service.outbox == nil {
		return fmt.Errorf("integration OAuth recovery outbox is unavailable")
	}
	task, err := newOAuthRefreshRecoveryTask(connection, expectedCredentialVersion)
	if err != nil {
		return err
	}
	return service.outbox.Enqueue(ctx, task)
}

// RecoverConnection applies a queued encrypted credential update before any
// additional provider refresh can run for the same connection.
func (service *OAuthRecoveryService) RecoverConnection(ctx context.Context, organizationID, connectionID uuid.UUID, credentialVersion int) error {
	if service == nil || service.outbox == nil {
		return nil
	}
	taskID := oauthRefreshRecoveryTaskID(organizationID, connectionID, credentialVersion)
	task, err := service.outbox.Get(ctx, taskID)
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return NewError(ErrorCodeConnectionInvalid, "integration OAuth credential recovery is unavailable", err)
	}
	if err := service.apply(ctx, *task); err != nil {
		retryErr := service.outbox.Retry(ctx, *task, oauthRecoveryReason(err))
		return NewError(ErrorCodeConnectionInvalid, "integration OAuth credential recovery is pending", errors.Join(err, retryErr))
	}
	if err := service.outbox.Ack(ctx, task.ID); err != nil {
		return NewError(ErrorCodeConnectionInvalid, "integration OAuth credential recovery could not be finalized", err)
	}
	return nil
}

func (service *OAuthRecoveryService) RecoverBatch(ctx context.Context, limit int64) error {
	if service == nil || service.outbox == nil {
		return nil
	}
	tasks, claimErr := service.outbox.Claim(ctx, limit)
	var batchErr error
	for _, task := range tasks {
		if err := service.apply(ctx, task); err != nil {
			if errors.Is(err, errOAuthManualRevocationRequired) {
				if deadLetterErr := service.outbox.DeadLetter(ctx, task, oauthRecoveryManualReason); deadLetterErr != nil {
					batchErr = errors.Join(batchErr, err, deadLetterErr)
				}
				continue
			}
			if retryErr := service.outbox.Retry(ctx, task, oauthRecoveryReason(err)); retryErr != nil {
				batchErr = errors.Join(batchErr, err, retryErr)
			}
			continue
		}
		if err := service.outbox.Ack(ctx, task.ID); err != nil {
			batchErr = errors.Join(batchErr, err)
		}
	}
	return errors.Join(claimErr, batchErr)
}

func (service *OAuthRecoveryService) RunOAuthRecovery(ctx context.Context) {
	if service == nil || service.outbox == nil {
		return
	}
	run := func() {
		operationCtx, cancel := context.WithTimeout(ctx, oauthRecoveryOperationTimeout)
		defer cancel()
		if err := service.RecoverBatch(operationCtx, 50); err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "failed to recover integration OAuth operations", err)
		}
	}
	run()
	ticker := time.NewTicker(oauthRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (service *OAuthRecoveryService) apply(ctx context.Context, task OAuthRecoveryTask) error {
	switch task.Kind {
	case OAuthRecoveryRevoke:
		if task.CompensationFlowID != nil {
			if service.flows == nil {
				return errOAuthCompensationFlowPending
			}
			flow, err := service.flows.GetByID(ctx, *task.CompensationFlowID)
			if errors.Is(err, ErrConnectionNotFound) {
				return fmt.Errorf("%w: OAuth compensation flow could not be verified", errOAuthManualRevocationRequired)
			}
			if err != nil {
				return fmt.Errorf("read OAuth compensation flow: %w", err)
			}
			if flow.Status == OAuthFlowSucceeded {
				return nil
			}
			if flow.Status == OAuthFlowPending && flow.ExpiresAt.After(time.Now().UTC()) {
				return errOAuthCompensationFlowPending
			}
		}
		if service.revoker == nil {
			return fmt.Errorf("OAuth revoker is unavailable")
		}
		envelope := task.EncryptedCredentials
		connection := &IntegrationConnection{
			ID:                   task.ConnectionID,
			OrganizationID:       task.OrganizationID,
			IntegrationID:        task.IntegrationID,
			DriverID:             task.DriverID,
			AuthType:             ConnectionAuthTypeOAuth2,
			AuthMethodID:         task.AuthMethodID,
			EncryptedCredentials: &envelope,
			CredentialVersion:    task.CredentialVersion,
			Config:               cloneAnyMap(task.ConnectionConfig),
		}
		if task.EncryptedClientCredentials == "" {
			// Backward compatibility for Redis tasks emitted before the durable
			// client snapshot was introduced.
			return service.revoker.RevokeConnection(ctx, connection)
		}
		snapshotter, ok := service.revoker.(oauthRecoverySnapshotRevoker)
		if !ok || service.cipher == nil {
			return fmt.Errorf("OAuth revocation snapshot executor is unavailable")
		}
		clientCredentials, err := service.cipher.DecryptCredentials(
			task.EncryptedClientCredentials,
			oauthRecoveryClientAAD(
				task.OrganizationID,
				task.ConnectionID,
				task.IntegrationID,
				task.AuthMethodID,
				task.ClientCredentialVersion,
			),
		)
		if err != nil {
			return fmt.Errorf("OAuth revocation client snapshot is invalid: %w", err)
		}
		client := OAuthClient{
			ClientID:     clientCredentials["client_id"],
			ClientSecret: clientCredentials["client_secret"],
			Config:       cloneAnyMap(task.ClientConfig),
		}
		destroyCredentialMap(clientCredentials)
		defer client.Destroy()
		return snapshotter.RevokeConnectionWithClient(ctx, connection, client)
	case OAuthRecoveryRefresh:
		return service.applyRefresh(ctx, task)
	default:
		return fmt.Errorf("OAuth recovery task kind is invalid")
	}
}

func (service *OAuthRecoveryService) applyRefresh(ctx context.Context, task OAuthRecoveryTask) error {
	if service.repository == nil || service.cipher == nil {
		return fmt.Errorf("OAuth credential repository is unavailable")
	}
	current, err := service.repository.GetByID(ctx, task.OrganizationID, task.ConnectionID)
	if errors.Is(err, ErrConnectionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.CredentialVersion > task.ExpectedCredentialVersion {
		return nil
	}
	if current.CredentialVersion < task.ExpectedCredentialVersion {
		return fmt.Errorf("OAuth credential version regressed")
	}
	if current.AuthType != ConnectionAuthTypeOAuth2 ||
		!strings.EqualFold(current.IntegrationID, task.IntegrationID) ||
		!strings.EqualFold(current.DriverID, task.DriverID) ||
		!strings.EqualFold(current.AuthMethodID, task.AuthMethodID) {
		return fmt.Errorf("OAuth recovery connection identity no longer matches")
	}
	credentials, err := service.cipher.DecryptCredentials(task.EncryptedCredentials, CredentialAAD{
		OrganizationID:    task.OrganizationID,
		ConnectionID:      task.ConnectionID,
		IntegrationID:     task.IntegrationID,
		CredentialVersion: task.CredentialVersion,
	})
	if err != nil {
		return fmt.Errorf("OAuth recovery credential envelope is invalid: %w", err)
	}
	destroyCredentialMap(credentials)
	credentialRepository, ok := service.repository.(OAuthCredentialRepository)
	if !ok {
		return fmt.Errorf("OAuth credential repository does not support recovery")
	}
	envelope := task.EncryptedCredentials
	update := &IntegrationConnection{
		ID:                    task.ConnectionID,
		OrganizationID:        task.OrganizationID,
		EncryptedCredentials:  &envelope,
		CredentialVersion:     task.CredentialVersion,
		GrantedScopes:         append([]string(nil), task.GrantedScopes...),
		TokenExpiresAt:        cloneTimePointer(task.TokenExpiresAt),
		RefreshTokenExpiresAt: cloneTimePointer(task.RefreshTokenExpiresAt),
		NextTokenRefreshAt:    cloneTimePointer(task.NextTokenRefreshAt),
		AuthStatus:            task.AuthStatus,
		ScopeStatus:           task.ScopeStatus,
		AttentionCode:         cloneStringPointer(task.AttentionCode),
		LastErrorCode:         cloneStringPointer(task.LastErrorCode),
	}
	if err := credentialRepository.UpdateOAuthCredentials(ctx, update, task.ExpectedCredentialVersion); err != nil {
		if errors.Is(err, ErrConnectionChanged) {
			latest, reloadErr := service.repository.GetByID(ctx, task.OrganizationID, task.ConnectionID)
			if errors.Is(reloadErr, ErrConnectionNotFound) || (reloadErr == nil && latest.CredentialVersion > task.ExpectedCredentialVersion) {
				return nil
			}
		}
		return err
	}
	return nil
}

func newOAuthRevocationRecoveryTask(
	connection *IntegrationConnection,
	encryptedClientCredentials string,
	clientCredentialVersion int,
	clientConfig map[string]any,
) (OAuthRecoveryTask, error) {
	if connection == nil || connection.EncryptedCredentials == nil {
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth revocation recovery requires encrypted credentials")
	}
	task := OAuthRecoveryTask{
		SchemaVersion:              oauthRecoverySchema,
		Kind:                       OAuthRecoveryRevoke,
		OrganizationID:             connection.OrganizationID,
		ConnectionID:               connection.ID,
		IntegrationID:              connection.IntegrationID,
		DriverID:                   connection.DriverID,
		AuthMethodID:               connection.AuthMethodID,
		ExpectedConnectionRevision: connection.Revision,
		CredentialVersion:          connection.CredentialVersion,
		EncryptedCredentials:       *connection.EncryptedCredentials,
		EncryptedClientCredentials: strings.TrimSpace(encryptedClientCredentials),
		ClientCredentialVersion:    clientCredentialVersion,
		ClientConfig:               cloneAnyMap(clientConfig),
		ConnectionConfig:           cloneAnyMap(connection.Config),
		CreatedAt:                  time.Now().UTC(),
	}
	task.ID = oauthRevocationRecoveryTaskID(task.OrganizationID, task.ConnectionID, task.CredentialVersion)
	task = normalizeOAuthRecoveryTask(task, task.CreatedAt)
	return task, validateOAuthRecoveryTask(task)
}

func newOAuthRefreshRecoveryTask(connection *IntegrationConnection, expectedCredentialVersion int) (OAuthRecoveryTask, error) {
	if connection == nil || connection.EncryptedCredentials == nil {
		return OAuthRecoveryTask{}, fmt.Errorf("integration OAuth refresh recovery requires encrypted credentials")
	}
	task := OAuthRecoveryTask{
		SchemaVersion:             oauthRecoverySchema,
		Kind:                      OAuthRecoveryRefresh,
		OrganizationID:            connection.OrganizationID,
		ConnectionID:              connection.ID,
		IntegrationID:             connection.IntegrationID,
		DriverID:                  connection.DriverID,
		AuthMethodID:              connection.AuthMethodID,
		ExpectedCredentialVersion: expectedCredentialVersion,
		CredentialVersion:         connection.CredentialVersion,
		EncryptedCredentials:      *connection.EncryptedCredentials,
		GrantedScopes:             append([]string(nil), connection.GrantedScopes...),
		TokenExpiresAt:            cloneTimePointer(connection.TokenExpiresAt),
		RefreshTokenExpiresAt:     cloneTimePointer(connection.RefreshTokenExpiresAt),
		NextTokenRefreshAt:        cloneTimePointer(connection.NextTokenRefreshAt),
		AuthStatus:                connection.AuthStatus,
		ScopeStatus:               connection.ScopeStatus,
		AttentionCode:             cloneStringPointer(connection.AttentionCode),
		LastErrorCode:             cloneStringPointer(connection.LastErrorCode),
		CreatedAt:                 time.Now().UTC(),
	}
	task.ID = oauthRefreshRecoveryTaskID(task.OrganizationID, task.ConnectionID, expectedCredentialVersion)
	task = normalizeOAuthRecoveryTask(task, task.CreatedAt)
	return task, validateOAuthRecoveryTask(task)
}

func oauthRefreshRecoveryTaskID(organizationID, connectionID uuid.UUID, expectedCredentialVersion int) string {
	return "refresh-" + oauthRecoveryDigest(organizationID, connectionID, expectedCredentialVersion)
}

func oauthRevocationRecoveryTaskID(organizationID, connectionID uuid.UUID, credentialVersion int) string {
	return "revoke-" + oauthRecoveryDigest(organizationID, connectionID, credentialVersion)
}

func oauthRecoveryClientAAD(
	organizationID, connectionID uuid.UUID,
	integrationID, authMethodID string,
	credentialVersion int,
) CredentialAAD {
	return CredentialAAD{
		OrganizationID: organizationID,
		ConnectionID:   connectionID,
		IntegrationID: "oauth-recovery-client-" +
			normalizeOAuthIdentifier(integrationID) + "-" +
			normalizeOAuthIdentifier(authMethodID),
		CredentialVersion: credentialVersion,
	}
}

func oauthRecoveryDigest(organizationID, connectionID uuid.UUID, version int) string {
	sum := sha256.Sum256([]byte(organizationID.String() + "\x00" + connectionID.String() + "\x00" + fmt.Sprint(version)))
	return fmt.Sprintf("%x", sum)
}

func oauthRecoveryReason(err error) string {
	if err == nil {
		return "unknown"
	}
	if code := strings.TrimSpace(ErrorCode(err)); code != "" {
		return code
	}
	if errors.Is(err, ErrConnectionChanged) {
		return "connection_changed"
	}
	if errors.Is(err, ErrConnectionNotFound) {
		return "connection_not_found"
	}
	return "recovery_failed"
}
