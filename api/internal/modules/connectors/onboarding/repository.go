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

// CommitCandidate is provided by the composition root and runs the existing
// Connection lifecycle against this transaction; it must not create bindings.
type CommitCandidate func(context.Context, *gorm.DB, Flow, map[string]string) (uuid.UUID, error)
type GormRepository struct {
	db     *gorm.DB
	commit CommitCandidate
	now    func() time.Time
}

func NewGormRepository(db *gorm.DB, commit CommitCandidate) *GormRepository {
	return &GormRepository{db: db, commit: commit, now: func() time.Time { return time.Now().UTC() }}
}
func (r *GormRepository) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	if r == nil || r.db == nil || ctx == nil {
		return registrationError(core.ErrorCodeInternal)
	}
	err := r.db.WithContext(ctx).Transaction(fn)
	if err == nil {
		return nil
	}
	var safe *core.Error
	if errors.As(err, &safe) {
		return safe
	}
	return registrationError(core.ErrorCodeInternal)
}
func budgetLock(tx *gorm.DB) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended('connector-onboarding-budget', 0))").Error
}
func scoped(tx *gorm.DB, org, owner, id uuid.UUID) *gorm.DB {
	return tx.Where("organization_id = ? AND owner_account_id = ? AND id = ?", org, owner, id)
}
func loadFlow(tx *gorm.DB, org, owner, id uuid.UUID, lock bool) (Flow, error) {
	if org == uuid.Nil || owner == uuid.Nil || id == uuid.Nil {
		return Flow{}, registrationError(core.ErrorCodeNotFound)
	}
	q := scoped(tx, org, owner, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var f Flow
	if err := q.Take(&f).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Flow{}, registrationError(core.ErrorCodeNotFound)
		}
		return Flow{}, registrationError(core.ErrorCodeInternal)
	}
	return f, nil
}
func (r *GormRepository) Get(ctx context.Context, org, owner, id uuid.UUID) (Flow, error) {
	if r == nil || r.db == nil || ctx == nil {
		return Flow{}, registrationError(core.ErrorCodeInternal)
	}
	return loadFlow(r.db.WithContext(ctx), org, owner, id, false)
}

func (r *GormRepository) FindRequest(ctx context.Context, org, owner uuid.UUID, hash string) (Flow, bool, error) {
	if r == nil || r.db == nil || org == uuid.Nil || owner == uuid.Nil || len(hash) != 64 {
		return Flow{}, false, registrationError(core.ErrorCodeInvalidInput)
	}
	var f Flow
	err := r.db.WithContext(ctx).Where("organization_id = ? AND owner_account_id = ? AND idempotency_key_hash = ?", org, owner, hash).Take(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Flow{}, false, nil
	}
	if err != nil {
		return Flow{}, false, registrationError(core.ErrorCodeInternal)
	}
	return f, true, nil
}
func (r *GormRepository) Admit(ctx context.Context, input Flow) (Flow, error) {
	var result Flow
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		if err := budgetLock(tx); err != nil {
			return err
		}
		var prior Flow
		err := tx.Where("organization_id = ? AND owner_account_id = ? AND idempotency_key_hash = ?", input.OrganizationID, input.OwnerAccountID, input.IdempotencyKeyHash).Take(&prior).Error
		if err == nil {
			if prior.RequestFingerprint != input.RequestFingerprint {
				return registrationError(core.ErrorCodeConflict)
			}
			result = prior
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := r.cleanup(tx); err != nil {
			return err
		}
		var counts struct{ OwnerActive, OrgActive, OwnerRecent, OrgRecent, SameIntent int64 }
		err = tx.Raw(`SELECT
 count(*) FILTER (WHERE state IN ('starting','awaiting_authorization','verifying') AND owner_account_id = ?) AS owner_active,
 count(*) FILTER (WHERE state IN ('starting','awaiting_authorization','verifying')) AS org_active,
 count(*) FILTER (WHERE created_at > ? AND owner_account_id = ?) AS owner_recent,
 count(*) FILTER (WHERE created_at > ?) AS org_recent,
 count(*) FILTER (WHERE state IN ('starting','awaiting_authorization','verifying') AND owner_account_id = ? AND
 ((connection_id IS NULL AND ? IN ('create_app','register_bot','login_session') AND connector_id = ?) OR (connection_id IS NOT NULL AND ? IN ('update_app','reauthorize') AND connection_id = ?))) AS same_intent
 FROM connector_onboarding_flows WHERE organization_id = ?`, input.OwnerAccountID, r.now().Add(-time.Minute), input.OwnerAccountID, r.now().Add(-time.Minute), input.OwnerAccountID, input.Intent, input.ConnectorID, input.Intent, input.ConnectionID, input.OrganizationID).Scan(&counts).Error
		if err != nil {
			return err
		}
		if counts.OwnerActive >= 3 || counts.OrgActive >= 20 || counts.OwnerRecent >= 5 || counts.OrgRecent >= 20 || counts.SameIntent > 0 {
			return registrationError(core.ErrorCodeRateLimited)
		}
		if err = tx.Create(&input).Error; err != nil {
			return err
		}
		result = input
		return nil
	})
	return result, err
}
func (r *GormRepository) Cancel(ctx context.Context, org, owner, id uuid.UUID, revision int64) (Flow, error) {
	var result Flow
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := loadFlow(tx, org, owner, id, true)
		if err != nil {
			return err
		}
		if !f.State.Active() {
			result = f
			return nil
		}
		if revision != f.Revision {
			return registrationError(core.ErrorCodeConflict)
		}
		if err = r.terminal(tx, &f, Cancelled, ""); err != nil {
			return err
		}
		result = f
		return nil
	})
	return result, err
}
func (r *GormRepository) terminal(tx *gorm.DB, f *Flow, state State, code core.ErrorCode) error {
	now := r.now()
	f.State = state
	f.TerminalAt = &now
	f.UpdatedAt = now
	f.Revision++
	f.LeaseOwner = nil
	f.LeaseUntil = nil
	if code != "" {
		f.SafeErrorCode = &code
	} else {
		f.SafeErrorCode = nil
	}
	if err := scoped(tx, f.OrganizationID, f.OwnerAccountID, f.ID).Select("state", "terminal_at", "updated_at", "revision", "lease_owner", "lease_until", "safe_error_code", "result_connection_id").Updates(f).Error; err != nil {
		return err
	}
	return tx.Where("flow_id = ? AND organization_id = ? AND owner_account_id = ?", f.ID, f.OrganizationID, f.OwnerAccountID).Delete(&DeviceRecord{}).Error
}
func (r *GormRepository) cleanup(tx *gorm.DB) error {
	now := r.now()
	var stale []Flow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("state IN ? AND (expires_at <= ? OR lease_until <= ?)", activeStates, now, now).Limit(128).Find(&stale).Error; err != nil {
		return err
	}
	for i := range stale {
		state, code := Expired, core.ErrorCodeFlowExpired
		if stale[i].LeaseUntil != nil {
			state, code = RecoveryRequired, core.ErrorCodeOutcomeUnknown
		}
		if err := r.terminal(tx, &stale[i], state, code); err != nil {
			return err
		}
	}
	// Device secrets expire immediately even if the associated flow was skipped
	// by a concurrent final transaction. Receipts retain cancellation/rate history.
	if err := tx.Where("expires_at <= ?", now).Delete(&DeviceRecord{}).Error; err != nil {
		return err
	}
	return tx.Where("terminal_at < ?", now.Add(-7*24*time.Hour)).Delete(&Flow{}).Error
}
func (r *GormRepository) Claim(ctx context.Context, worker uuid.UUID) (Flow, bool, error) {
	var result Flow
	found := false
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		if worker == uuid.Nil {
			return registrationError(core.ErrorCodeInvalidInput)
		}
		if err := budgetLock(tx); err != nil {
			return err
		}
		if err := r.cleanup(tx); err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&Flow{}).Where("lease_until > ?", r.now()).Count(&active).Error; err != nil {
			return err
		}
		if active >= 32 {
			return nil
		}
		var f Flow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("state IN ? AND next_poll_at <= ? AND lease_owner IS NULL AND protocol_version IN (0,1) AND step <> 'awaiting_code'", []State{Starting, Awaiting}, r.now()).Order("next_poll_at, id").Take(&f).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if (f.ProtocolVersion == 0 && f.State == Starting && f.BeginDispatched) || (f.ProtocolVersion != 0 && f.StepDispatched) {
			return r.terminal(tx, &f, RecoveryRequired, core.ErrorCodeOutcomeUnknown)
		}
		until := r.now().Add(45 * time.Second)
		if f.ProtocolVersion != 0 {
			until = r.now().Add(60 * time.Second)
			f.StepDispatched = true
		}
		f.LeaseOwner = &worker
		f.LeaseUntil = &until
		f.Fencing++
		f.Revision++
		f.UpdatedAt = r.now()
		if f.State == Starting {
			f.BeginDispatched = true
		}
		if err = tx.Save(&f).Error; err != nil {
			return err
		}
		result = f
		found = true
		return nil
	})
	return result, found, err
}
func (r *GormRepository) fenced(tx *gorm.DB, lease Flow) (Flow, error) {
	f, err := loadFlow(tx, lease.OrganizationID, lease.OwnerAccountID, lease.ID, true)
	if err != nil {
		return f, err
	}
	if !f.State.Active() || f.LeaseOwner == nil || lease.LeaseOwner == nil || *f.LeaseOwner != *lease.LeaseOwner || f.Fencing != lease.Fencing || f.Generation != lease.Generation || f.LeaseUntil == nil || !f.LeaseUntil.After(r.now()) || !f.ExpiresAt.After(r.now()) {
		return f, registrationError(core.ErrorCodeConflict)
	}
	return f, nil
}
func (r *GormRepository) Device(ctx context.Context, f Flow) (DeviceRecord, error) {
	current, err := r.Get(ctx, f.OrganizationID, f.OwnerAccountID, f.ID)
	if err != nil {
		return DeviceRecord{}, err
	}
	if !current.State.Active() || current.Generation != f.Generation || !current.ExpiresAt.After(r.now()) {
		return DeviceRecord{}, registrationError(core.ErrorCodeFlowExpired)
	}
	var record DeviceRecord
	err = r.db.WithContext(ctx).Where("flow_id = ? AND organization_id = ? AND owner_account_id = ? AND generation = ? AND expires_at > ?", f.ID, f.OrganizationID, f.OwnerAccountID, f.Generation, r.now()).Take(&record).Error
	if err != nil {
		return DeviceRecord{}, registrationError(core.ErrorCodeNotFound)
	}
	return record, nil
}
func (r *GormRepository) AwaitAuthorization(ctx context.Context, lease Flow, device DeviceRecord, interval, ttl time.Duration) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.State != Starting || device.FlowID != f.ID || device.OrganizationID != f.OrganizationID || device.OwnerAccountID != f.OwnerAccountID || device.Generation != f.Generation {
			return registrationError(core.ErrorCodeConflict)
		}
		expiry := r.now().Add(ttl)
		if expiry.Before(f.ExpiresAt) {
			f.ExpiresAt = expiry
		}
		device.ExpiresAt = f.ExpiresAt
		if err = tx.Create(&device).Error; err != nil {
			return err
		}
		f.State = Awaiting
		f.PollIntervalSeconds = max(5, int(interval.Seconds()))
		return r.release(tx, &f)
	})
}
func (r *GormRepository) release(tx *gorm.DB, f *Flow) error {
	f.LeaseOwner = nil
	f.LeaseUntil = nil
	f.Revision++
	f.UpdatedAt = r.now()
	f.NextPollAt = r.now().Add(time.Duration(f.PollIntervalSeconds) * time.Second)
	return tx.Save(f).Error
}
func (r *GormRepository) Advance(ctx context.Context, lease Flow, region string, switched bool, interval int) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.State != Awaiting || registrationOrigin(region) == "" || interval < 5 || interval > 300 {
			return registrationError(core.ErrorCodeConflict)
		}
		if f.Intent == "update_app" && region != f.Region {
			return registrationError(core.ErrorCodeConflict)
		}
		f.Region = region
		f.SwitchedRegion = switched
		f.PollIntervalSeconds = interval
		return r.release(tx, &f)
	})
}
func (r *GormRepository) Terminate(ctx context.Context, lease Flow, state State, code core.ErrorCode) error {
	if state.Active() || state == Ready {
		return registrationError(core.ErrorCodeInvalidInput)
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		return r.terminal(tx, &f, state, code)
	})
}
func (r *GormRepository) Complete(ctx context.Context, lease Flow, values map[string]string) (uuid.UUID, error) {
	defer core.DestroySecretValues(values)
	var id uuid.UUID
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		f, err := r.fenced(tx, lease)
		if err != nil {
			return err
		}
		if f.State != Awaiting || r.commit == nil {
			return registrationError(core.ErrorCodeConflict)
		}
		f.State = Verifying
		if err = tx.Save(&f).Error; err != nil {
			return err
		}
		id, err = r.commit(ctx, tx, f, values)
		if err != nil {
			return err
		}
		if id == uuid.Nil || !f.ExpiresAt.After(r.now()) || !f.LeaseUntil.After(r.now()) {
			return registrationError(core.ErrorCodeConflict)
		}
		f.ResultConnectionID = &id
		return r.terminal(tx, &f, Ready, "")
	})
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
