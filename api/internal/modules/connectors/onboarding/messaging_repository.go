package onboarding

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func (r *GormRepository) SaveProtocolStep(ctx context.Context, lease Flow, device DeviceRecord, next ProtocolResult) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.ProtocolVersion != 1 || !f.StepDispatched || !validProtocolTransition(f, next.Step) || next.Interval < time.Second || next.Interval > 300*time.Second || next.TTL < 0 || next.TTL > 2*time.Hour {
			return registrationError(core.ErrorCodeConflict)
		}
		if device.FlowID != f.ID || device.OrganizationID != f.OrganizationID || device.OwnerAccountID != f.OwnerAccountID || device.Generation != f.Generation {
			return registrationError(core.ErrorCodeAccessDenied)
		}
		if next.TTL > 0 {
			deadline := r.now().Add(next.TTL)
			if deadline.Before(f.ExpiresAt) {
				f.ExpiresAt = deadline
				f.ExpirySource = next.ExpirySource
			}
		}
		device.ExpiresAt = f.ExpiresAt
		if err = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "flow_id"}}, DoUpdates: clause.AssignmentColumns([]string{"encrypted_device", "expires_at"})}).Create(&device).Error; err != nil {
			return err
		}
		f.State = Awaiting
		f.Step = next.Step
		f.StepDispatched = false
		f.PollIntervalSeconds = int(next.Interval.Seconds())
		if next.Step == "awaiting_code" {
			f.ChallengeRevision++
		}
		return r.release(tx, &f)
	})
}
func validProtocolTransition(f Flow, next string) bool {
	if f.ConnectorID == "dingtalk" && f.Step == "init" {
		return next == "begin"
	}
	if f.Step == "begin" {
		return next == "poll"
	}
	if f.Step == "poll" {
		return next == "poll" || next == "verify" || (f.ConnectorID == "weixin" && next == "awaiting_code")
	}
	return false
}
func (r *GormRepository) SubmitChallenge(ctx context.Context, previous Flow, device DeviceRecord, revision, challenge int64) (Flow, error) {
	var out Flow
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := loadFlow(tx, previous.OrganizationID, previous.OwnerAccountID, previous.ID, true)
		if err != nil {
			return err
		}
		if f.ProtocolVersion != 1 || f.ConnectorID != "weixin" || f.State != Awaiting || f.Step != "awaiting_code" || f.LeaseOwner != nil || f.Revision != revision || f.ChallengeRevision != challenge || f.ChallengeAttempts >= 3 || !f.ExpiresAt.After(r.now()) || f.Generation != device.Generation || f.ID != device.FlowID || f.OrganizationID != device.OrganizationID || f.OwnerAccountID != device.OwnerAccountID {
			return registrationError(core.ErrorCodeConflict)
		}
		update := tx.Model(&DeviceRecord{}).Where("flow_id = ? AND organization_id = ? AND owner_account_id = ? AND generation = ?", f.ID, f.OrganizationID, f.OwnerAccountID, f.Generation).Update("encrypted_device", device.EncryptedDevice)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return registrationError(core.ErrorCodeConflict)
		}
		f.Step = "poll"
		f.StepDispatched = false
		f.ChallengeAttempts++
		f.Revision++
		f.UpdatedAt = r.now()
		f.NextPollAt = r.now()
		if err = tx.Save(&f).Error; err != nil {
			return err
		}
		out = f
		return nil
	})
	return out, err
}
