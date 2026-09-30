package onboarding

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"time"
)

func (r *GormRepository) CheckpointMessaging(ctx context.Context, lease Flow, device DeviceRecord) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.Step != "verify" || !f.StepDispatched || f.CandidateExpiresAt == nil || device.FlowID != f.ID || device.Generation != f.Generation || device.OrganizationID != f.OrganizationID || device.OwnerAccountID != f.OwnerAccountID {
			return registrationError(core.ErrorCodeConflict)
		}
		saved := tx.Model(&DeviceRecord{}).Where("flow_id = ? AND organization_id = ? AND owner_account_id = ? AND generation = ?", f.ID, f.OrganizationID, f.OwnerAccountID, f.Generation).Updates(map[string]any{"encrypted_device": device.EncryptedDevice, "expires_at": *f.CandidateExpiresAt})
		if saved.Error != nil {
			return saved.Error
		}
		if saved.RowsAffected != 1 {
			return registrationError(core.ErrorCodeConflict)
		}
		f.Step = "commit"
		f.StepDispatched = false
		f.VerificationPhase = "commit"
		f.State = Verifying
		f.SafeErrorCode = nil
		f.DiagnosticTraceID = ""
		f.PollIntervalSeconds = 1
		return r.release(tx, &f)
	})
}
func (r *GormRepository) WaitMessaging(ctx context.Context, lease Flow, code core.ErrorCode, phase, trace string) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.CandidateExpiresAt == nil || (f.Step != "verify" && f.Step != "commit") {
			return registrationError(core.ErrorCodeConflict)
		}
		f.State = VerificationWaiting
		f.StepDispatched = false
		f.SafeErrorCode = &code
		f.VerificationPhase = phase
		f.DiagnosticTraceID = trace
		f.VerificationDeadline = nil
		return r.release(tx, &f)
	})
}
func (r *GormRepository) RetryMessaging(ctx context.Context, previous Flow, revision int64, key string) (Flow, error) {
	var out Flow
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := loadFlow(tx, previous.OrganizationID, previous.OwnerAccountID, previous.ID, true)
		if err != nil {
			return err
		}
		if f.RetryKeyHash != nil && *f.RetryKeyHash == key {
			if f.RetryExpectedRevision == nil || *f.RetryExpectedRevision != revision {
				return registrationError(core.ErrorCodeConflict)
			}
			out = f
			return nil
		}
		if len(key) != 64 || f.State != VerificationWaiting || f.Revision != revision || f.LeaseOwner != nil || f.CandidateExpiresAt == nil || !f.CandidateExpiresAt.After(r.now()) {
			return registrationError(core.ErrorCodeConflict)
		}
		f.RetryKeyHash = &key
		f.RetryExpectedRevision = &revision
		f.State = Verifying
		f.StepDispatched = false
		f.SafeErrorCode = nil
		f.DiagnosticTraceID = ""
		f.NextPollAt = r.now()
		f.Revision++
		f.UpdatedAt = r.now()
		if err = tx.Save(&f).Error; err != nil {
			return err
		}
		out = f
		return nil
	})
	return out, err
}
func (r *GormRepository) CompleteMessaging(ctx context.Context, lease Flow, values map[string]string, verified connections.CandidateVerificationResult) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.Step != "commit" || !f.StepDispatched || f.CandidateConnectionID == nil || r.messagingCommit == nil {
			return registrationError(core.ErrorCodeConflict)
		}
		id, err = r.messagingCommit(ctx, tx, f, values, verified)
		if err != nil {
			return err
		}
		if id != *f.CandidateConnectionID || !f.verificationExpiry().After(r.now()) || f.LeaseUntil == nil || !f.LeaseUntil.After(r.now()) {
			return registrationError(core.ErrorCodeConflict)
		}
		if f.ConnectorID == "weixin" {
			changed := tx.Table("connector_messaging_states").Where("organization_id = ? AND owner_account_id = ? AND connection_id = ? AND connector_id = ? AND flow_id = ?", f.OrganizationID, f.OwnerAccountID, id, f.ConnectorID, f.ID).Updates(map[string]any{"flow_id": nil, "updated_at": r.now()})
			if changed.Error != nil {
				return changed.Error
			}
			if changed.RowsAffected != 1 {
				return registrationError(core.ErrorCodeConflict)
			}
		}
		f.ResultConnectionID = &id
		return r.terminal(tx, &f, Ready, "")
	})
	return id, err
}

// A commit-only worker can be recovered from its durable encrypted checkpoint.
// A dispatched consuming probe without a checkpoint must never be replayed.
func (r *GormRepository) recoverMessagingCommit(tx *gorm.DB, f *Flow) (bool, error) {
	if f.ProtocolVersion != 1 || f.Step != "commit" || f.CandidateExpiresAt == nil || !f.CandidateExpiresAt.After(r.now()) {
		return false, nil
	}
	code := core.ErrorCodeVerificationWaiting
	f.State = VerificationWaiting
	f.StepDispatched = false
	f.SafeErrorCode = &code
	f.VerificationPhase = "commit"
	f.VerificationDeadline = nil
	f.PollIntervalSeconds = int(time.Second.Seconds())
	return true, r.release(tx, f)
}
