package onboarding

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func loadPending(tx *gorm.DB, org, owner, id uuid.UUID, lock bool) (PendingSetup, error) {
	if org == uuid.Nil || owner == uuid.Nil || id == uuid.Nil {
		return PendingSetup{}, registrationError(core.ErrorCodeNotFound)
	}
	q := scoped(tx, org, owner, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var p PendingSetup
	if err := q.Take(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return p, registrationError(core.ErrorCodeNotFound)
		}
		return p, registrationError(core.ErrorCodeInternal)
	}
	return p, nil
}
func (r *GormRepository) GetPending(ctx context.Context, org, owner, id uuid.UUID) (PendingSetup, *uuid.UUID, error) {
	p, err := loadPending(r.db.WithContext(ctx), org, owner, id, false)
	if err != nil {
		return p, nil, err
	}
	var f Flow
	err = r.db.WithContext(ctx).Select("id").Where("organization_id = ? AND owner_account_id = ? AND pending_setup_id = ? AND state IN ? AND COALESCE(verification_deadline, expires_at) > ?", org, owner, id, activeStates, r.now()).Take(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p, nil, nil
	}
	if err != nil {
		return p, nil, registrationError(core.ErrorCodeInternal)
	}
	return p, &f.ID, nil
}
func (r *GormRepository) ListPending(ctx context.Context, org, owner uuid.UUID, connector string, limit, offset int) ([]PendingSetup, int64, error) {
	if org == uuid.Nil || owner == uuid.Nil {
		return nil, 0, registrationError(core.ErrorCodeAccessDenied)
	}
	q := r.db.WithContext(ctx).Model(&PendingSetup{}).Where("organization_id = ? AND owner_account_id = ? AND terminal_at IS NULL AND resume_expires_at > ?", org, owner, r.now())
	if connector != "" {
		q = q.Where("connector_id = ?", connector)
	}
	var count int64
	var items []PendingSetup
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, registrationError(core.ErrorCodeInternal)
	}
	if err := q.Order("created_at DESC, id").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, registrationError(core.ErrorCodeInternal)
	}
	return items, count, nil
}
func pendingFlow(p PendingSetup) Flow {
	return Flow{OrganizationID: p.OrganizationID, OwnerAccountID: p.OwnerAccountID, ConnectorID: p.ConnectorID, Intent: "resume_app", Region: p.Region, RequestedRegion: p.Region, ExpectedAppID: &p.AppID, PendingSetupID: &p.ID, TemplateID: p.TemplateID, TemplateDigest: p.TemplateDigest, AcceptedDefinitionDigest: p.AcceptedDefinitionDigest}
}
func pendingMatches(p PendingSetup, f Flow) bool {
	return p.OrganizationID == f.OrganizationID && p.OwnerAccountID == f.OwnerAccountID && p.ConnectorID == f.ConnectorID && p.Region == f.Region && p.TemplateID == f.TemplateID && p.TemplateDigest == f.TemplateDigest && p.AcceptedDefinitionDigest == f.AcceptedDefinitionDigest && (f.ExpectedAppID == nil || *f.ExpectedAppID == p.AppID)
}
func (r *GormRepository) pendingForLease(tx *gorm.DB, f Flow) (*PendingSetup, error) {
	if f.PendingSetupID == nil {
		return nil, nil
	}
	p, err := loadPending(tx, f.OrganizationID, f.OwnerAccountID, *f.PendingSetupID, true)
	if err != nil {
		return nil, err
	}
	if !pendingMatches(p, f) || !p.State.Active() || !p.ResumeExpiresAt.After(r.now()) {
		return nil, registrationError(core.ErrorCodeConflict)
	}
	return &p, nil
}

// Persist the non-secret app identity in a short transaction BEFORE candidate
// verification. A crash cannot cause the worker to retrieve credentials again.
func (r *GormRepository) ObserveApplication(ctx context.Context, lease Flow, appID string) (Flow, error) {
	var result Flow
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		p, err := r.pendingForLease(tx, lease)
		if err != nil {
			return err
		}
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.ProtocolVersion != 0 || f.ConnectorID != "feishu" || f.State != Awaiting || (f.Intent != "create_app" && f.Intent != "resume_app") || !appIdentifier.MatchString(appID) || (f.ExpectedAppID != nil && *f.ExpectedAppID != appID) {
			return registrationError(core.ErrorCodeConflict)
		}
		now := r.now()
		if p == nil {
			p = &PendingSetup{ID: uuid.New(), OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectorID: f.ConnectorID, Region: f.Region, AppID: appID, SourceFlowID: f.ID, TemplateID: f.TemplateID, TemplateDigest: f.TemplateDigest, AcceptedDefinitionDigest: f.AcceptedDefinitionDigest, State: VerificationRequired, Revision: 1, ResumeExpiresAt: now.Add(30 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}
			if err = tx.Create(p).Error; err != nil {
				return err
			}
		} else if p.AppID != appID {
			return registrationError(core.ErrorCodeConflict)
		}
		deadline := now.Add(30 * time.Second)
		if f.LeaseUntil.Before(deadline) {
			deadline = *f.LeaseUntil
		}
		f.State = Verifying
		f.PendingSetupID = &p.ID
		f.VerificationDeadline = &deadline
		f.Revision++
		f.UpdatedAt = now
		if err = tx.Save(&f).Error; err != nil {
			return err
		}
		if err = tx.Where("flow_id = ? AND organization_id = ? AND owner_account_id = ?", f.ID, f.OrganizationID, f.OwnerAccountID).Delete(&DeviceRecord{}).Error; err != nil {
			return err
		}
		result = f
		return nil
	})
	return result, err
}
func (r *GormRepository) DeferApplication(ctx context.Context, lease Flow, code core.ErrorCode) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		p, err := r.pendingForLease(tx, lease)
		if err != nil {
			return err
		}
		if p == nil {
			return registrationError(core.ErrorCodeConflict)
		}
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.State != Verifying {
			return registrationError(core.ErrorCodeConflict)
		}
		p.State = VerificationRequired
		if code == core.ErrorCodeApplicationNotReady {
			p.State = AwaitingApplication
		}
		now := r.now()
		p.SafeReasonCode = &code
		p.LastVerificationAt = &now
		p.UpdatedAt = now
		p.Revision++
		if err = tx.Save(p).Error; err != nil {
			return err
		}
		return r.terminal(tx, &f, PendingCompletion, code)
	})
}
func (r *GormRepository) CancelPending(ctx context.Context, org, owner, id uuid.UUID, revision int64) (PendingSetup, error) {
	var result PendingSetup
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		p, err := loadPending(tx, org, owner, id, true)
		if err != nil {
			return err
		}
		if !p.State.Active() {
			result = p
			return nil
		}
		if p.Revision != revision {
			return registrationError(core.ErrorCodeConflict)
		}
		if err = r.endPending(tx, &p, PendingCancelled); err != nil {
			return err
		}
		result = p
		return nil
	})
	return result, err
}
func (r *GormRepository) endPending(tx *gorm.DB, p *PendingSetup, state PendingState) error {
	var flows []Flow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND owner_account_id = ? AND pending_setup_id = ? AND state IN ?", p.OrganizationID, p.OwnerAccountID, p.ID, activeStates).Find(&flows).Error; err != nil {
		return err
	}
	for i := range flows {
		if err := r.terminal(tx, &flows[i], Cancelled, ""); err != nil {
			return err
		}
	}
	now := r.now()
	p.State = state
	p.TerminalAt = &now
	p.UpdatedAt = now
	p.Revision++
	return tx.Save(p).Error
}
func (r *GormRepository) cleanupPending(tx *gorm.DB) error {
	var expired []PendingSetup
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("terminal_at IS NULL AND resume_expires_at <= ?", r.now()).Limit(128).Find(&expired).Error; err != nil {
		return err
	}
	for i := range expired {
		if err := r.endPending(tx, &expired[i], PendingExpired); err != nil {
			return err
		}
	}
	// Receipts referenced by a newer attempt survive until that receipt expires.
	return tx.Where("terminal_at < ? AND NOT EXISTS (SELECT 1 FROM connector_onboarding_flows f WHERE f.pending_setup_id = connector_pending_setups.id)", r.now().Add(-7*24*time.Hour)).Delete(&PendingSetup{}).Error
}

func (r *GormRepository) admitPending(tx *gorm.DB, f Flow) (*PendingSetup, error) {
	if f.PendingSetupID != nil {
		p, err := loadPending(tx, f.OrganizationID, f.OwnerAccountID, *f.PendingSetupID, true)
		if err != nil {
			return nil, err
		}
		if !pendingMatches(p, f) {
			return nil, registrationError(core.ErrorCodeConflict)
		}
		if p.State == PendingCompleted {
			return &p, nil
		}
		if !p.State.Active() || !p.ResumeExpiresAt.After(r.now()) || f.ExpectedPendingRevision == nil || p.Revision != *f.ExpectedPendingRevision {
			return nil, registrationError(core.ErrorCodeConflict)
		}
		return &p, nil
	}
	if f.Intent != "create_app" || f.ConnectorID != "feishu" {
		return nil, nil
	}
	var counts struct{ OwnerPending, OrgPending int64 }
	err := tx.Raw(`SELECT count(*) FILTER (WHERE owner_account_id = ?) AS owner_pending, count(*) AS org_pending FROM (
 SELECT owner_account_id FROM connector_pending_setups WHERE organization_id = ? AND terminal_at IS NULL AND resume_expires_at > ?
 UNION ALL SELECT owner_account_id FROM connector_onboarding_flows WHERE organization_id = ? AND connector_id = 'feishu' AND intent = 'create_app' AND pending_setup_id IS NULL AND state IN ('starting','awaiting_authorization','verifying')
) reserved`, f.OwnerAccountID, f.OrganizationID, r.now(), f.OrganizationID).Scan(&counts).Error
	if err != nil {
		return nil, err
	}
	if counts.OwnerPending >= 20 || counts.OrgPending >= 200 {
		return nil, registrationError(core.ErrorCodeRateLimited)
	}
	return nil, nil
}
