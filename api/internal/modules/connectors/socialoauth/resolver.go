package socialoauth

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// The connection lock also serializes refresh against manual reconnect/removal.
type connectionRow struct {
	DiagnosticCode        *string
	Revision              int
	LastProviderFailureAt *time.Time
	UpdatedAt             time.Time
	ID                    uuid.UUID
	OrganizationID        uuid.UUID
	OwnerAccountID        uuid.UUID
	ConnectorID           string
	EncryptedCredentials  string `json:"-"`
	CredentialVersion     int
	State                 string
	DeletedAt             *time.Time
}

func (connectionRow) TableName() string { return "connector_connections" }

var _ execution.CredentialResolver = (*Service)(nil)
var _ connections.MaintenanceCredentialResolver = (*Service)(nil)

func (s *Service) ResolveCredentials(ctx context.Context, r execution.CredentialResolutionRequest) (map[string]string, error) {
	return s.resolve(ctx, r, uuid.Nil)
}
func (s *Service) ResolveConnectionCredentials(ctx context.Context, c core.PersonalConnection) (map[string]string, error) {
	if c.OwnerAccountID == uuid.Nil {
		return nil, fail(core.ErrorCodeAccessDenied)
	}
	return s.resolve(ctx, execution.CredentialResolutionRequest{OrganizationID: c.OrganizationID, ConnectionID: c.ID, ConnectorID: c.ConnectorID, CredentialVersion: c.CredentialVersion, EncryptedCredentials: c.EncryptedCredentials}, c.OwnerAccountID)
}
func (s *Service) resolve(ctx context.Context, r execution.CredentialResolutionRequest, owner uuid.UUID) (map[string]string, error) {
	if !supported(r.ConnectorID.String()) {
		return s.vault.ResolveCredentials(ctx, r)
	}
	if r.OrganizationID == uuid.Nil || r.ConnectionID == uuid.Nil || r.CredentialVersion < 1 || r.EncryptedCredentials == "" {
		return nil, fail(core.ErrorCodeConnectionNotReady)
	}
	var values map[string]string
	var terminal error
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c connectionRow
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND organization_id = ? AND connector_id = ? AND deleted_at IS NULL", r.ConnectionID, r.OrganizationID, r.ConnectorID.String())
		if owner != uuid.Nil {
			q = q.Where("owner_account_id = ?", owner)
		}
		if e := q.Take(&c).Error; e != nil {
			return fail(core.ErrorCodeConnectionNotReady)
		}
		if c.State != string(core.ReadinessReady) {
			return fail(core.ErrorCodeConnectionNotReady)
		}
		var g grantRecord
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("connection_id = ? AND organization_id = ? AND connector_id = ?", c.ID, c.OrganizationID, c.ConnectorID).Take(&g).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return fail(core.ErrorCodeInternal)
		}
		oauth := e == nil && g.CredentialVersion == c.CredentialVersion && g.CredentialDigest == digest(c.EncryptedCredentials)
		if !oauth {
			if r.CredentialVersion != c.CredentialVersion || r.EncryptedCredentials != c.EncryptedCredentials {
				return fail(core.ErrorCodeConnectionNotReady)
			}
			var e error
			values, e = s.vault.ResolveCredentials(ctx, r)
			return e
		}
		var token tokenGrant
		if e := s.open(grantSubject(g), g.Envelope, &token); e != nil {
			return e
		}
		if token.ExpiresAt.IsZero() || s.now().Add(time.Minute).Before(token.ExpiresAt) {
			var e error
			values, e = s.vault.ResolveCredentials(ctx, currentRequest(c))
			return e
		}
		if c.ConnectorID == "facebook-social" || token.RefreshToken == "" || (!token.RefreshExpiresAt.IsZero() && !s.now().Before(token.RefreshExpiresAt)) {
			terminal = fail(core.ErrorCodeInvalidKey)
			return markReconnect(tx, c, s.now())
		}
		renewed, e := s.exchange(ctx, c.ConnectorID, token.RefreshToken, "", true)
		if e != nil {
			if core.IsErrorCode(e, core.ErrorCodeInvalidKey) {
				terminal = e
				return markReconnect(tx, c, s.now())
			}
			return e
		}
		if renewed.ExpiresAt.IsZero() {
			return fail(core.ErrorCodeProviderUnavailable)
		}
		if renewed.RefreshToken == "" {
			renewed.RefreshToken = token.RefreshToken
		}
		if renewed.RefreshExpiresAt.IsZero() {
			renewed.RefreshExpiresAt = token.RefreshExpiresAt
		}
		// Renewal retains the authorization generation: identity and grant scopes do
		// not change. Binding and confirmation snapshots remain valid.
		v := map[string]string{"token": renewed.AccessToken}
		defer core.DestroySecretValues(v)
		envelope, e := s.vault.Seal(connections.CredentialSealRequest{OrganizationID: c.OrganizationID, ConnectionID: c.ID, ConnectorID: c.ConnectorID, CredentialVersion: c.CredentialVersion, Values: v})
		if e != nil {
			return e
		}
		g.CredentialDigest = digest(envelope)
		g.CredentialVersion = c.CredentialVersion
		g.Envelope, e = s.seal(grantSubject(g), renewed)
		if e != nil {
			return e
		}
		g.UpdatedAt = s.now()
		if e := tx.Model(&connectionRow{}).Where("id = ?", c.ID).Updates(map[string]any{"encrypted_credentials": envelope, "credential_version": c.CredentialVersion, "updated_at": s.now()}).Error; e != nil {
			return fail(core.ErrorCodeInternal)
		}
		if e := tx.Save(&g).Error; e != nil {
			return fail(core.ErrorCodeInternal)
		}
		values = map[string]string{"token": renewed.AccessToken}
		return nil
	})
	if e != nil {
		core.DestroySecretValues(values)
		return nil, e
	}
	if terminal != nil {
		return nil, terminal
	}
	return values, nil
}
func currentRequest(c connectionRow) execution.CredentialResolutionRequest {
	return execution.CredentialResolutionRequest{OrganizationID: c.OrganizationID, ConnectionID: c.ID, ConnectorID: core.Identifier(c.ConnectorID), CredentialVersion: c.CredentialVersion, EncryptedCredentials: c.EncryptedCredentials}
}

func markReconnect(tx *gorm.DB, c connectionRow, now time.Time) error {
	if e := tx.Model(&connectionRow{}).Where("id = ?", c.ID).Updates(map[string]any{"state": core.ReadinessReconnectRequired, "diagnostic_code": string(core.ErrorCodeInvalidKey), "last_provider_failure_at": now, "updated_at": now, "revision": gorm.Expr("revision + 1")}).Error; e != nil {
		return fail(core.ErrorCodeInternal)
	}
	return nil
}
