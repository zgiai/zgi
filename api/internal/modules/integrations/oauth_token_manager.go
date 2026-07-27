package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/pkg/lock"
)

type OAuthRefreshLease interface {
	Release(context.Context) error
}

type OAuthRefreshLocker interface {
	Acquire(context.Context, string, time.Duration) (OAuthRefreshLease, error)
}

type ConnectionRevoker interface {
	RevokeConnection(context.Context, *IntegrationConnection) error
}

type OAuthConnectionRevoker struct {
	cipher   CredentialCipher
	registry *Registry
	clients  OAuthClientResolver
}

func NewOAuthConnectionRevoker(cipher CredentialCipher, registry *Registry, clients OAuthClientResolver) *OAuthConnectionRevoker {
	return &OAuthConnectionRevoker{cipher: cipher, registry: registry, clients: clients}
}

func (revoker *OAuthConnectionRevoker) RevokeConnection(ctx context.Context, connection *IntegrationConnection) error {
	if connection == nil || connection.AuthType != ConnectionAuthTypeOAuth2 {
		return nil
	}
	if revoker == nil || revoker.cipher == nil || revoker.registry == nil || revoker.clients == nil ||
		connection.EncryptedCredentials == nil {
		return NewError(ErrorCodeConnectionInvalid, "integration OAuth revocation is unavailable", nil)
	}
	client, err := revoker.ResolveRevocationClient(ctx, connection)
	if err != nil {
		return err
	}
	defer client.Destroy()
	return revoker.RevokeConnectionWithClient(ctx, connection, client)
}

func (revoker *OAuthConnectionRevoker) ResolveRevocationClient(ctx context.Context, connection *IntegrationConnection) (OAuthClient, error) {
	if revoker == nil || revoker.clients == nil || connection == nil {
		return OAuthClient{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth revocation is unavailable", nil)
	}
	return revoker.clients.ResolveOAuthClient(ctx, OAuthClientResolveRequest{
		OrganizationID: connection.OrganizationID,
		IntegrationID:  connection.IntegrationID,
		DriverID:       connection.DriverID,
		AuthMethodID:   connection.AuthMethodID,
	})
}

// RevokeConnectionWithClient uses an immutable request-scoped OAuth client
// snapshot. Durable recovery calls this method so later client-config changes
// cannot orphan a provider revocation.
func (revoker *OAuthConnectionRevoker) RevokeConnectionWithClient(
	ctx context.Context,
	connection *IntegrationConnection,
	client OAuthClient,
) error {
	if connection == nil || connection.AuthType != ConnectionAuthTypeOAuth2 {
		return nil
	}
	if revoker == nil || revoker.cipher == nil || revoker.registry == nil ||
		connection.EncryptedCredentials == nil || strings.TrimSpace(client.ClientID) == "" {
		return NewError(ErrorCodeConnectionInvalid, "integration OAuth revocation is unavailable", nil)
	}
	provider, ok := revoker.registry.OAuthProvider(connection.IntegrationID, connection.DriverID)
	if !ok {
		return NewError(ErrorCodeDisabled, "integration OAuth provider is unavailable", nil)
	}
	capability, supportsRevocation := provider.(OAuthRevocationCapability)
	if !supportsRevocation || !capability.SupportsTokenRevocation() {
		return fmt.Errorf("%w: provider does not publish a token revocation endpoint", errOAuthManualRevocationRequired)
	}
	credentials, err := revoker.cipher.DecryptCredentials(*connection.EncryptedCredentials, CredentialAAD{
		OrganizationID:    connection.OrganizationID,
		ConnectionID:      connection.ID,
		IntegrationID:     connection.IntegrationID,
		CredentialVersion: connection.CredentialVersion,
	})
	if err != nil {
		return NewError(ErrorCodeConnectionInvalid, "integration OAuth credentials could not be decrypted", err)
	}
	defer destroyCredentialMap(credentials)
	token := strings.TrimSpace(credentials["refresh_token"])
	tokenTypeHint := "refresh_token"
	if token == "" {
		token = strings.TrimSpace(credentials["access_token"])
		tokenTypeHint = "access_token"
	}
	if token == "" {
		return NewError(ErrorCodeReconnectRequired, "integration OAuth connection has no revocable token", nil)
	}
	if err := provider.RevokeToken(ctx, OAuthRevokeRequest{
		Client: client, Token: token, TokenTypeHint: tokenTypeHint, Config: cloneAnyMap(connection.Config),
	}); err != nil {
		if ErrorCode(err) == ErrorCodeAuthInvalid {
			return nil
		}
		return NewError(oauthPublicErrorCode(err), "integration OAuth token revocation failed", err)
	}
	return nil
}

type redisOAuthRefreshLocker struct {
	client *redis.Client
}

func NewRedisOAuthRefreshLocker(client *redis.Client) OAuthRefreshLocker {
	if client == nil {
		return nil
	}
	return &redisOAuthRefreshLocker{client: client}
}

func (locker *redisOAuthRefreshLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (OAuthRefreshLease, error) {
	if locker == nil || locker.client == nil {
		return nil, fmt.Errorf("integration OAuth refresh lock is unavailable")
	}
	lease := lock.NewRedisLock(locker.client, "zgi:integrations:oauth-refresh:"+strings.TrimSpace(key), ttl)
	retryInterval := 100 * time.Millisecond
	maxRetries := int(ttl / retryInterval)
	if maxRetries < 50 {
		maxRetries = 50
	}
	if maxRetries > 300 {
		maxRetries = 300
	}
	acquired, err := lease.AcquireWithRetry(ctx, retryInterval, maxRetries)
	if err != nil {
		return nil, fmt.Errorf("acquire integration OAuth refresh lock: %w", err)
	}
	if !acquired {
		return nil, NewError(ErrorCodeTimeout, "integration OAuth credential refresh is busy", nil)
	}
	return lease, nil
}

// OAuthRefreshingConnectionResolver refreshes OAuth credentials before the
// regular resolver decrypts them. It requires a distributed lock and uses the
// connection repository's credential-version/revision CAS for defense in
// depth, including providers that rotate single-use refresh tokens.
type OAuthRefreshingConnectionResolver struct {
	base          ConnectionResolver
	repository    ConnectionRepository
	cipher        CredentialCipher
	registry      *Registry
	clients       OAuthClientResolver
	locker        OAuthRefreshLocker
	refreshWindow time.Duration
	lockTTL       time.Duration
	now           func() time.Time
	recovery      *OAuthRecoveryService
}

func NewOAuthRefreshingConnectionResolver(base ConnectionResolver, repository ConnectionRepository, cipher CredentialCipher, registry *Registry, clients OAuthClientResolver, locker OAuthRefreshLocker, refreshWindow time.Duration) *OAuthRefreshingConnectionResolver {
	if refreshWindow <= 0 {
		refreshWindow = 5 * time.Minute
	}
	return &OAuthRefreshingConnectionResolver{
		base: base, repository: repository, cipher: cipher, registry: registry, clients: clients, locker: locker,
		refreshWindow: refreshWindow, lockTTL: 30 * time.Second, now: func() time.Time { return time.Now().UTC() },
	}
}

func (resolver *OAuthRefreshingConnectionResolver) WithOAuthRecovery(recovery *OAuthRecoveryService) *OAuthRefreshingConnectionResolver {
	if resolver != nil {
		resolver.recovery = recovery
	}
	return resolver
}

func (resolver *OAuthRefreshingConnectionResolver) Resolve(ctx context.Context, request ConnectionResolveRequest) (*ResolvedConnection, error) {
	if resolver == nil || resolver.base == nil || resolver.repository == nil || resolver.cipher == nil || resolver.registry == nil || resolver.clients == nil {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration OAuth connection resolver is unavailable", nil)
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(request.OrganizationID))
	if err != nil || organizationID == uuid.Nil {
		return nil, invalidInput("organization id is required", err)
	}
	connectionID, err := uuid.Parse(strings.TrimSpace(request.ConnectionID))
	if err != nil || connectionID == uuid.Nil {
		return resolver.base.Resolve(ctx, request)
	}
	connection, err := resolver.repository.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return resolver.base.Resolve(ctx, request)
	}
	if connection.AuthType == ConnectionAuthTypeOAuth2 {
		if err := resolver.recoverPending(ctx, connection); err != nil {
			return nil, err
		}
		connection, err = resolver.repository.GetByID(ctx, organizationID, connectionID)
		if err != nil {
			return nil, mapConnectionLookupError(err)
		}
	}
	if connection.AuthType == ConnectionAuthTypeOAuth2 && resolver.refreshTokenExpired(connection) {
		resolver.markReconnectRequired(ctx, connection, ErrorCodeReconnectRequired)
		return nil, NewError(ErrorCodeReconnectRequired, "integration OAuth connection must be reauthorized", nil)
	}
	if connection.AuthType != ConnectionAuthTypeOAuth2 || !resolver.refreshDue(connection) {
		return resolver.base.Resolve(ctx, request)
	}
	if resolver.locker == nil {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration OAuth refresh lock is unavailable", nil)
	}
	lease, err := resolver.locker.Acquire(ctx, oauthRefreshLockKey(connection), resolver.lockTTL)
	if err != nil {
		return nil, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lease.Release(releaseCtx)
	}()
	connection, err = resolver.repository.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return nil, mapConnectionLookupError(err)
	}
	if connection.AuthType == ConnectionAuthTypeOAuth2 {
		if err := resolver.recoverPending(ctx, connection); err != nil {
			return nil, err
		}
		connection, err = resolver.repository.GetByID(ctx, organizationID, connectionID)
		if err != nil {
			return nil, mapConnectionLookupError(err)
		}
	}
	if connection.AuthType == ConnectionAuthTypeOAuth2 && resolver.refreshTokenExpired(connection) {
		resolver.markReconnectRequired(ctx, connection, ErrorCodeReconnectRequired)
		return nil, NewError(ErrorCodeReconnectRequired, "integration OAuth connection must be reauthorized", nil)
	}
	if connection.AuthType != ConnectionAuthTypeOAuth2 || !resolver.refreshDue(connection) {
		return resolver.base.Resolve(ctx, request)
	}
	if err := resolver.refresh(ctx, connection); err != nil {
		return nil, err
	}
	return resolver.base.Resolve(ctx, request)
}

func (resolver *OAuthRefreshingConnectionResolver) ResolveRecordForTest(ctx context.Context, connection *IntegrationConnection) (*ResolvedConnection, error) {
	testResolver, ok := resolver.base.(interface {
		ResolveRecordForTest(context.Context, *IntegrationConnection) (*ResolvedConnection, error)
	})
	if !ok {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection test resolver is unavailable", nil)
	}
	if connection != nil && connection.AuthType == ConnectionAuthTypeOAuth2 &&
		(resolver.refreshTokenExpired(connection) || resolver.refreshDue(connection)) {
		request := ConnectionResolveRequest{
			OrganizationID: connection.OrganizationID.String(), IntegrationID: connection.IntegrationID,
			DriverID: connection.DriverID, ConnectionID: connection.ID.String(),
		}
		return resolver.Resolve(ctx, request)
	}
	return testResolver.ResolveRecordForTest(ctx, connection)
}

func (resolver *OAuthRefreshingConnectionResolver) refreshDue(connection *IntegrationConnection) bool {
	if connection == nil || connection.AuthType != ConnectionAuthTypeOAuth2 {
		return false
	}
	now := resolver.now()
	if connection.NextTokenRefreshAt != nil && !connection.NextTokenRefreshAt.After(now) {
		return true
	}
	return connection.TokenExpiresAt != nil && !connection.TokenExpiresAt.After(now.Add(resolver.refreshWindow))
}

func (resolver *OAuthRefreshingConnectionResolver) refreshTokenExpired(connection *IntegrationConnection) bool {
	return connection != nil &&
		connection.AuthType == ConnectionAuthTypeOAuth2 &&
		connection.RefreshTokenExpiresAt != nil &&
		!connection.RefreshTokenExpiresAt.After(resolver.now())
}

func (resolver *OAuthRefreshingConnectionResolver) refresh(ctx context.Context, connection *IntegrationConnection) error {
	if resolver.refreshTokenExpired(connection) {
		resolver.markReconnectRequired(ctx, connection, ErrorCodeReconnectRequired)
		return NewError(ErrorCodeReconnectRequired, "integration OAuth connection must be reauthorized", nil)
	}
	if connection.EncryptedCredentials == nil || strings.TrimSpace(*connection.EncryptedCredentials) == "" {
		return NewError(ErrorCodeReconnectRequired, "integration OAuth connection must be reauthorized", nil)
	}
	credentials, err := resolver.cipher.DecryptCredentials(*connection.EncryptedCredentials, CredentialAAD{
		OrganizationID: connection.OrganizationID, ConnectionID: connection.ID,
		IntegrationID: connection.IntegrationID, CredentialVersion: connection.CredentialVersion,
	})
	if err != nil {
		return NewError(ErrorCodeConnectionInvalid, "integration OAuth credentials could not be decrypted", err)
	}
	defer destroyCredentialMap(credentials)
	refreshToken := strings.TrimSpace(credentials["refresh_token"])
	if refreshToken == "" {
		resolver.markReconnectRequired(ctx, connection, ErrorCodeReconnectRequired)
		return NewError(ErrorCodeReconnectRequired, "integration OAuth connection must be reauthorized", nil)
	}
	provider, ok := resolver.registry.OAuthProvider(connection.IntegrationID, connection.DriverID)
	if !ok {
		return NewError(ErrorCodeDisabled, "integration OAuth provider is unavailable", nil)
	}
	client, err := resolver.clients.ResolveOAuthClient(ctx, OAuthClientResolveRequest{
		OrganizationID: connection.OrganizationID, IntegrationID: connection.IntegrationID,
		DriverID: connection.DriverID, AuthMethodID: connection.AuthMethodID,
	})
	if err != nil {
		return err
	}
	defer client.Destroy()
	tokens, err := provider.RefreshToken(ctx, OAuthRefreshRequest{
		Client: client, RefreshToken: refreshToken, Scopes: append([]string(nil), connection.GrantedScopes...),
		Config: cloneAnyMap(connection.Config),
	})
	if err != nil {
		if code := ErrorCode(err); code == ErrorCodeAuthInvalid || code == ErrorCodeReconnectRequired {
			resolver.markReconnectRequired(ctx, connection, code)
			return NewError(ErrorCodeReconnectRequired, "integration OAuth connection must be reauthorized", err)
		}
		return NewError(oauthPublicErrorCode(err), "integration OAuth token refresh failed", err)
	}
	defer tokens.Destroy()
	now := resolver.now()
	if strings.TrimSpace(tokens.AccessToken) == "" ||
		(tokens.ExpiresAt != nil && !tokens.ExpiresAt.After(now)) ||
		(tokens.RefreshTokenExpiresAt != nil && !tokens.RefreshTokenExpiresAt.After(now)) {
		return NewError(ErrorCodeResponseInvalid, "integration OAuth refresh response is invalid", nil)
	}
	if strings.TrimSpace(tokens.RefreshToken) == "" {
		tokens.RefreshToken = refreshToken
	}
	if tokens.RefreshTokenExpiresAt == nil {
		// A provider omitting refresh-token lifetime metadata must not erase a
		// previously known safety boundary. This is especially important when
		// the provider leaves the existing refresh token in place.
		tokens.RefreshTokenExpiresAt = cloneTimePointer(connection.RefreshTokenExpiresAt)
	}
	if len(tokens.Scopes) == 0 {
		tokens.Scopes = append([]string(nil), connection.GrantedScopes...)
	}
	tokens.Scopes = normalizeScopes(tokens.Scopes)
	if missing := missingScopes(connection.GrantedScopes, tokens.Scopes); len(missing) > 0 {
		resolver.markReconnectRequired(ctx, connection, ErrorCodeInsufficientScope)
		return NewError(ErrorCodeInsufficientScope, "integration OAuth refresh lost required scopes", nil)
	}
	nextCredentials := tokens.credentialMap()
	defer destroyCredentialMap(nextCredentials)
	expectedCredentialVersion := connection.CredentialVersion
	connection.CredentialVersion++
	envelope, err := resolver.cipher.EncryptCredentials(nextCredentials, CredentialAAD{
		OrganizationID: connection.OrganizationID, ConnectionID: connection.ID,
		IntegrationID: connection.IntegrationID, CredentialVersion: connection.CredentialVersion,
	})
	if err != nil {
		return NewError(ErrorCodeConnectionInvalid, "integration OAuth refreshed credentials could not be protected", err)
	}
	connection.EncryptedCredentials = &envelope
	connection.GrantedScopes = append([]string(nil), tokens.Scopes...)
	connection.TokenExpiresAt = cloneTimePointer(tokens.ExpiresAt)
	connection.RefreshTokenExpiresAt = cloneTimePointer(tokens.RefreshTokenExpiresAt)
	connection.NextTokenRefreshAt = oauthNextRefreshAt(tokens.ExpiresAt, resolver.refreshWindow)
	connection.AuthStatus = ConnectionAuthValid
	connection.ScopeStatus = ConnectionScopeVerified
	connection.AttentionCode = nil
	connection.LastErrorCode = nil
	connection.UpdatedBy = nil
	persistErr := resolver.persistRefreshedCredentials(ctx, connection, expectedCredentialVersion)
	if persistErr != nil {
		if errors.Is(persistErr, ErrConnectionChanged) {
			// A reconnect or another refresh may have won the credential CAS.
			// Accept that outcome only when the stored credential version
			// actually advanced. An unrelated metadata/health update is not
			// evidence that the provider-issued rotating token was persisted.
			latest, reloadErr := resolver.repository.GetByID(ctx, connection.OrganizationID, connection.ID)
			if reloadErr == nil && latest.CredentialVersion > expectedCredentialVersion {
				return nil
			}
			if queueErr := resolver.enqueueRefreshRecovery(ctx, connection, expectedCredentialVersion); queueErr != nil {
				return NewError(ErrorCodeConnectionInvalid, "integration OAuth refreshed credentials could not be recovered", errors.Join(persistErr, queueErr))
			}
			if reloadErr != nil {
				return mapConnectionLookupError(reloadErr)
			}
			return NewError(ErrorCodeConnectionConflict, "integration OAuth credentials changed during refresh", persistErr)
		}
		if queueErr := resolver.enqueueRefreshRecovery(ctx, connection, expectedCredentialVersion); queueErr != nil {
			return NewError(ErrorCodeConnectionInvalid, "integration OAuth refreshed credentials could not be recovered", errors.Join(persistErr, queueErr))
		}
		return mapConnectionLookupError(persistErr)
	}
	return nil
}

func (resolver *OAuthRefreshingConnectionResolver) recoverPending(ctx context.Context, connection *IntegrationConnection) error {
	if resolver == nil || resolver.recovery == nil || connection == nil {
		return nil
	}
	recoveryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return resolver.recovery.RecoverConnection(
		recoveryCtx,
		connection.OrganizationID,
		connection.ID,
		connection.CredentialVersion,
	)
}

func (resolver *OAuthRefreshingConnectionResolver) enqueueRefreshRecovery(ctx context.Context, connection *IntegrationConnection, expectedCredentialVersion int) error {
	if resolver == nil || resolver.recovery == nil {
		return fmt.Errorf("integration OAuth credential recovery outbox is unavailable")
	}
	queueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	return resolver.recovery.EnqueueRefresh(queueCtx, connection, expectedCredentialVersion)
}

func (resolver *OAuthRefreshingConnectionResolver) persistRefreshedCredentials(ctx context.Context, connection *IntegrationConnection, expectedCredentialVersion int) error {
	// A provider may rotate and immediately invalidate the old refresh token.
	// Persisting the encrypted replacement is therefore part of the refresh
	// operation, not an ordinary metadata update. Every attempt gets its own
	// bounded context so one cancelled request or one timed-out database call
	// cannot consume the full recovery budget before the encrypted outbox
	// fallback is attempted.
	var persistErr error
	for attempt := 0; attempt < 3; attempt++ {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		if credentialRepository, ok := resolver.repository.(OAuthCredentialRepository); ok {
			persistErr = credentialRepository.UpdateOAuthCredentials(persistCtx, connection, expectedCredentialVersion)
		} else {
			persistErr = resolver.repository.Update(persistCtx, connection)
		}
		cancel()
		if persistErr == nil || errors.Is(persistErr, ErrConnectionChanged) || errors.Is(persistErr, ErrConnectionNotFound) {
			return persistErr
		}
		if attempt == 2 {
			break
		}
		delay := time.Duration(attempt+1) * 50 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			// Persistence is intentionally detached from request cancellation;
			// skip only the cosmetic delay and proceed to the next bounded
			// attempt.
		case <-timer.C:
		}
	}
	return persistErr
}

func (resolver *OAuthRefreshingConnectionResolver) markReconnectRequired(ctx context.Context, connection *IntegrationConnection, code string) {
	connection.AuthStatus = ConnectionAuthReconnectRequired
	connection.HealthStatus = ConnectionHealthDegraded
	attention := ConnectionAttentionReconnectRequired
	connection.AttentionCode = &attention
	safeCode := oauthSafeErrorCode(code)
	connection.LastErrorCode = &safeCode
	connection.NextTokenRefreshAt = nil
	_ = resolver.repository.Update(ctx, connection)
}

func oauthRefreshLockKey(connection *IntegrationConnection) string {
	if connection == nil {
		return "invalid"
	}
	digest := sha256.Sum256([]byte(connection.OrganizationID.String() + "\x00" + connection.ID.String()))
	return hex.EncodeToString(digest[:])
}
