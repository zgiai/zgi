package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type readLease struct {
	OrganizationID, OwnerAccountID, ConnectionID uuid.UUID `gorm:"primaryKey"`
	ConnectorID                                  string    `gorm:"primaryKey"`
	Owner                                        *uuid.UUID
	Until                                        *time.Time
	Fence                                        int64
	Uncertain                                    bool
	CredentialDigest                             string
}

func (readLease) TableName() string { return "connector_messaging_read_leases" }
func leaseScope(tx *gorm.DB, scope providers.ResourceScope, provider string) *gorm.DB {
	return tx.Where("organization_id = ? AND owner_account_id = ? AND connection_id = ? AND connector_id = ?", scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID, provider)
}

func lockCandidate(tx *gorm.DB, scope providers.ResourceScope, flow uuid.UUID) error {
	if flow == uuid.Nil {
		return nil
	}
	var found struct{ ID uuid.UUID }
	err := tx.Table("connector_onboarding_flows").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ? AND organization_id = ? AND owner_account_id = ? AND state IN ('awaiting_authorization','verifying','verification_waiting') AND candidate_connection_id = ? AND candidate_expires_at > ?", flow, scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID, time.Now()).Take(&found).Error
	if err != nil {
		return fail(core.ErrorCodeConflict)
	}
	return nil
}

// Update serializes consuming reads with a durable lease. The callback runs
// outside a SQL transaction; only claim and encrypted checkpoint hold locks.
// An expired dispatched read is uncertain and cannot be automatically replayed.
func (s *DBStore) Update(ctx context.Context, scope providers.ResourceScope, provider, digest string, flow uuid.UUID, fn func(context.Context, *State) error) error {
	if !validScope(scope) || provider != "weixin" || len(digest) != 64 || fn == nil {
		return fail(core.ErrorCodeAccessDenied)
	}
	if _, nested := ctx.Value(transactionKey{}).(*gorm.DB); nested {
		return fail(core.ErrorCodeConflict)
	}
	var row stateRow
	state := State{Messages: []Message{}, Seen: []string{}}
	owner := uuid.New()
	var fence int64
	err := s.transaction(ctx, func(_ context.Context, tx *gorm.DB) error {
		if err := lockCandidate(tx, scope, flow); err != nil {
			return err
		}
		lease := readLease{OrganizationID: scope.OrganizationID, OwnerAccountID: scope.OwnerAccountID, ConnectionID: scope.ConnectionID, ConnectorID: provider}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lease).Error; err != nil {
			return err
		}
		if err := leaseScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), scope, provider).Take(&lease).Error; err != nil {
			return err
		}
		if flow != uuid.Nil && lease.CredentialDigest != digest && (lease.Owner == nil || lease.Until != nil && !lease.Until.After(time.Now())) {
			lease.Uncertain = false
			lease.Owner = nil
			lease.Until = nil
		}
		if lease.Uncertain || lease.Owner != nil && (lease.Until == nil || !lease.Until.After(time.Now())) {
			return &providers.Failure{Code: core.ErrorCodeVerificationUncertain, Unknown: true}
		}
		if lease.Owner != nil {
			return &providers.Failure{Code: core.ErrorCodeVerificationWaiting, Retryable: true}
		}
		err := leaseScope(tx, scope, provider).Where("credential_digest = ?", digest).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if flow == uuid.Nil {
				return fail(core.ErrorCodeConnectionNotReady)
			}
			row = stateRow{ID: uuid.New(), OrganizationID: scope.OrganizationID, OwnerAccountID: scope.OwnerAccountID, ConnectionID: scope.ConnectionID, ConnectorID: provider, CredentialDigest: digest, FlowID: &flow}
		} else if err != nil {
			return err
		} else {
			if !row.ExpiresAt.After(time.Now()) {
				return fail(core.ErrorCodeConnectionNotReady)
			}
			if flow != uuid.Nil && (row.FlowID == nil || *row.FlowID != flow) {
				return fail(core.ErrorCodeConflict)
			}
			if flow == uuid.Nil && row.FlowID != nil {
				var ready int64
				if err := tx.Table("connector_onboarding_flows").Where("id = ? AND organization_id = ? AND owner_account_id = ? AND state = 'ready' AND result_connection_id = ?", *row.FlowID, scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID).Count(&ready).Error; err != nil {
					return err
				}
				if ready != 1 {
					return fail(core.ErrorCodeConnectionNotReady)
				}
				row.FlowID = nil
			}
			raw, err := s.secrets.Open(credentials.Subject{Purpose: credentials.MessagingState, OrganizationID: row.OrganizationID, ID: row.ID, ConnectorID: provider, Version: 1}, row.EncryptedState)
			if err != nil {
				return fail(core.ErrorCodeConnectionNotReady)
			}
			defer clear(raw)
			if json.Unmarshal(raw, &state) != nil {
				return fail(core.ErrorCodeConnectionNotReady)
			}
		}
		duration := 60 * time.Second
		if provider == "weixin" {
			duration = 65 * time.Second
		}
		until := time.Now().UTC().Add(duration)
		lease.Owner = &owner
		lease.CredentialDigest = digest
		lease.Until = &until
		lease.Fence++
		fence = lease.Fence
		return tx.Save(&lease).Error
	})
	if err != nil {
		return err
	}
	err = fn(ctx, &state)
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err != nil {
		uncertain := providers.FailureOf(err).Unknown
		_ = s.database(cleanup).Model(&readLease{}).Where("organization_id = ? AND owner_account_id = ? AND connection_id = ? AND connector_id = ? AND owner = ? AND fence = ?", scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID, provider, owner, fence).Updates(map[string]any{"owner": nil, "until": nil, "uncertain": uncertain}).Error
		return err
	}
	if len(state.Messages) > 100 || len(state.Seen) > 256 || len(state.Cursor) > 32768 {
		return &providers.Failure{Code: core.ErrorCodeVerificationUncertain, Unknown: true}
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	defer clear(raw)
	row.EncryptedState, err = s.secrets.Protect(credentials.Subject{Purpose: credentials.MessagingState, OrganizationID: row.OrganizationID, ID: row.ID, ConnectorID: provider, Version: 1}, raw)
	if err != nil {
		return &providers.Failure{Code: core.ErrorCodeVerificationUncertain, Unknown: true}
	}
	err = s.transaction(cleanup, func(_ context.Context, tx *gorm.DB) error {
		if err := lockCandidate(tx, scope, flow); err != nil {
			return err
		}
		var lease readLease
		if err := leaseScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), scope, provider).Take(&lease).Error; err != nil {
			return err
		}
		if lease.Owner == nil || *lease.Owner != owner || lease.Fence != fence || lease.Until == nil || !lease.Until.After(time.Now()) {
			return fail(core.ErrorCodeConflict)
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		row.ExpiresAt = row.UpdatedAt.Add(30 * 24 * time.Hour)
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		lease.Owner = nil
		lease.Until = nil
		return tx.Save(&lease).Error
	})
	if err != nil {
		return &providers.Failure{Code: core.ErrorCodeVerificationUncertain, Unknown: true}
	}
	return nil
}
