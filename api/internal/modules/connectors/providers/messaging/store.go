// Package messaging stores private conversation capabilities inside Connectors.
package messaging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type Message struct {
	ID, Recipient, Text, ContextToken string
	ReceivedAt                        time.Time
}
type State struct {
	Cursor   string
	Messages []Message
	Seen     []string
}
type Conversation struct{ Recipient, ContextToken string }
type Store interface {
	Update(context.Context, providers.ResourceScope, string, string, uuid.UUID, func(context.Context, *State) error) error
	Put(context.Context, providers.ResourceScope, string, Conversation) (string, error)
	Resolve(context.Context, providers.ResourceScope, string, string) (Conversation, error)
}
type stateRow struct {
	ID                                           uuid.UUID `gorm:"primaryKey"`
	OrganizationID, OwnerAccountID, ConnectionID uuid.UUID
	ConnectorID, CredentialDigest                string
	FlowID                                       *uuid.UUID
	Revision                                     int64
	EncryptedState                               string `json:"-"`
	ExpiresAt, UpdatedAt                         time.Time
}

func (stateRow) TableName() string { return "connector_messaging_states" }

type contextRow struct {
	ID                                                      uuid.UUID `gorm:"primaryKey"`
	OrganizationID, OwnerAccountID, ConnectionID, BindingID uuid.UUID
	ConnectorID                                             string
	CredentialVersion, BindingRevision                      int
	TargetDigest, EncryptedContext                          string
	ExpiresAt                                               time.Time
}

func (contextRow) TableName() string { return "connector_messaging_contexts" }

type DBStore struct {
	db      *gorm.DB
	secrets *credentials.Store
}

func NewStore(db *gorm.DB, secrets *credentials.Store) *DBStore { return &DBStore{db, secrets} }

type transactionKey struct{}

func WithTransaction(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, transactionKey{}, tx)
}
func (s *DBStore) database(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(transactionKey{}).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return s.db.WithContext(ctx)
}
func (s *DBStore) transaction(ctx context.Context, fn func(context.Context, *gorm.DB) error) error {
	if s == nil || s.db == nil || s.secrets == nil {
		return fail(core.ErrorCodeConnectorUnavailable)
	}
	if tx, ok := ctx.Value(transactionKey{}).(*gorm.DB); ok {
		return fn(ctx, tx.WithContext(ctx))
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(WithTransaction(ctx, tx), tx) })
	if err == nil {
		return nil
	}
	var f *providers.Failure
	if errors.As(err, &f) {
		return f
	}
	return fail(core.ErrorCodeProviderUnavailable)
}
func CredentialDigest(values map[string]string) string {
	raw, _ := json.Marshal(values)
	defer clear(raw)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validScope(s providers.ResourceScope) bool {
	return s.OrganizationID != uuid.Nil && s.OwnerAccountID != uuid.Nil && s.ConnectionID != uuid.Nil
}
func targetScope(s providers.ResourceScope) bool {
	return validScope(s) && s.CredentialVersion > 0 && s.BindingID != uuid.Nil && s.BindingRevision > 0 && s.Target.Validate() == nil
}
func targetDigest(s providers.ResourceScope) string {
	raw, _ := json.Marshal(s.Target)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func fail(c core.ErrorCode) error { return &providers.Failure{Code: c} }

func (s *DBStore) Update(ctx context.Context, scope providers.ResourceScope, provider, digest string, flow uuid.UUID, fn func(context.Context, *State) error) error {
	if !validScope(scope) || provider != "weixin" || len(digest) != 64 || fn == nil {
		return fail(core.ErrorCodeAccessDenied)
	}
	return s.transaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		key := scope.OrganizationID.String() + ":" + scope.OwnerAccountID.String() + ":" + scope.ConnectionID.String() + ":" + provider
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", key).Error; err != nil {
				return err
			}
		}
		var row stateRow
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND owner_account_id = ? AND connection_id = ? AND connector_id = ? AND credential_digest = ?", scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID, provider, digest)
		err := query.Take(&row).Error
		state := State{Messages: []Message{}, Seen: []string{}}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if flow == uuid.Nil {
				return fail(core.ErrorCodeConnectionNotReady)
			}
			row = stateRow{ID: uuid.New(), OrganizationID: scope.OrganizationID, OwnerAccountID: scope.OwnerAccountID, ConnectionID: scope.ConnectionID, ConnectorID: provider, CredentialDigest: digest, FlowID: &flow, Revision: 1}
		} else if err != nil {
			return err
		} else {
			if !row.ExpiresAt.After(time.Now()) {
				return fail(core.ErrorCodeConnectionNotReady)
			}
			if flow == uuid.Nil && row.FlowID != nil {
				var ready int64
				if err = tx.Table("connector_onboarding_flows").Where("id = ? AND organization_id = ? AND owner_account_id = ? AND state = 'ready' AND result_connection_id = ?", *row.FlowID, scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID).Count(&ready).Error; err != nil {
					return err
				}
				if ready != 1 {
					return fail(core.ErrorCodeConnectionNotReady)
				}
				row.FlowID = nil
			}
			plain, err := s.secrets.Open(credentials.Subject{Purpose: credentials.MessagingState, OrganizationID: row.OrganizationID, ID: row.ID, ConnectorID: provider, Version: 1}, row.EncryptedState)
			if err != nil {
				return fail(core.ErrorCodeConnectionNotReady)
			}
			err = json.Unmarshal(plain, &state)
			clear(plain)
			if err != nil {
				return fail(core.ErrorCodeConnectionNotReady)
			}
		}
		if err = fn(ctx, &state); err != nil {
			return err
		}
		if len(state.Messages) > 100 || len(state.Seen) > 256 || len(state.Cursor) > 32768 {
			return fail(core.ErrorCodeProviderUnavailable)
		}
		plain, err := json.Marshal(state)
		if err != nil {
			return fail(core.ErrorCodeProviderUnavailable)
		}
		defer clear(plain)
		row.EncryptedState, err = s.secrets.Protect(credentials.Subject{Purpose: credentials.MessagingState, OrganizationID: row.OrganizationID, ID: row.ID, ConnectorID: provider, Version: 1}, plain)
		if err != nil {
			return fail(core.ErrorCodeProviderUnavailable)
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		row.ExpiresAt = row.UpdatedAt.Add(30 * 24 * time.Hour)
		return tx.Save(&row).Error
	})
}
func (s *DBStore) Put(ctx context.Context, scope providers.ResourceScope, provider string, value Conversation) (string, error) {
	if s == nil || s.db == nil || s.secrets == nil {
		return "", fail(core.ErrorCodeConnectorUnavailable)
	}
	if !targetScope(scope) || (provider != "wecom" && provider != "weixin") || value.Recipient == "" || len(value.Recipient) > 8192 || len(value.ContextToken) > 16384 {
		return "", fail(core.ErrorCodeAccessDenied)
	}
	id := uuid.New()
	raw, _ := json.Marshal(value)
	defer clear(raw)
	sealed, err := s.secrets.Protect(credentials.Subject{Purpose: credentials.MessagingContext, OrganizationID: scope.OrganizationID, ID: id, ConnectorID: provider, Version: scope.CredentialVersion}, raw)
	if err != nil {
		return "", fail(core.ErrorCodeProviderUnavailable)
	}
	row := contextRow{ID: id, OrganizationID: scope.OrganizationID, OwnerAccountID: scope.OwnerAccountID, ConnectionID: scope.ConnectionID, BindingID: scope.BindingID, BindingRevision: scope.BindingRevision, ConnectorID: provider, CredentialVersion: scope.CredentialVersion, TargetDigest: targetDigest(scope), EncryptedContext: sealed, ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if err = s.database(ctx).Create(&row).Error; err != nil {
		return "", fail(core.ErrorCodeProviderUnavailable)
	}
	return id.String(), nil
}
func (s *DBStore) Resolve(ctx context.Context, scope providers.ResourceScope, provider, handle string) (Conversation, error) {
	if s == nil || s.db == nil || s.secrets == nil {
		return Conversation{}, fail(core.ErrorCodeConnectorUnavailable)
	}
	var out Conversation
	id, err := uuid.Parse(handle)
	if err != nil || !targetScope(scope) {
		return out, fail(core.ErrorCodeResourceAccessDenied)
	}
	var row contextRow
	err = s.database(ctx).Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connection_id = ? AND connector_id = ? AND credential_version = ? AND binding_id = ? AND binding_revision = ? AND target_digest = ? AND expires_at > ?", id, scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID, provider, scope.CredentialVersion, scope.BindingID, scope.BindingRevision, targetDigest(scope), time.Now()).Take(&row).Error
	if err != nil {
		return out, fail(core.ErrorCodeResourceAccessDenied)
	}
	raw, err := s.secrets.Open(credentials.Subject{Purpose: credentials.MessagingContext, OrganizationID: scope.OrganizationID, ID: id, ConnectorID: provider, Version: scope.CredentialVersion}, row.EncryptedContext)
	if err != nil {
		return out, fail(core.ErrorCodeResourceAccessDenied)
	}
	defer clear(raw)
	if json.Unmarshal(raw, &out) != nil {
		return Conversation{}, fail(core.ErrorCodeResourceAccessDenied)
	}
	return out, nil
}

// Cleanup removes a bounded batch. Contexts cannot be used after expiry even
// when this worker has not yet physically removed their encrypted records.
func (s *DBStore) Cleanup(ctx context.Context) error {
	return s.transaction(ctx, func(_ context.Context, tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM connector_messaging_contexts WHERE id IN
 (SELECT id FROM connector_messaging_contexts WHERE expires_at <= ? ORDER BY expires_at LIMIT 100 FOR UPDATE SKIP LOCKED)`, time.Now()).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM connector_messaging_states WHERE id IN
 (SELECT s.id FROM connector_messaging_states s WHERE s.expires_at <= ? OR
 (NOT EXISTS (SELECT 1 FROM connector_connections c WHERE c.id=s.connection_id AND c.organization_id=s.organization_id AND c.owner_account_id=s.owner_account_id)
 AND NOT EXISTS (SELECT 1 FROM connector_onboarding_flows f WHERE f.id=s.flow_id AND f.state IN ('starting','awaiting_authorization','verifying')))
 ORDER BY s.expires_at LIMIT 100 FOR UPDATE SKIP LOCKED)`, time.Now()).Error
	})
}
