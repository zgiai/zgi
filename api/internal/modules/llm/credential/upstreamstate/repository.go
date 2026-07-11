package upstreamstate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	Get(ctx context.Context, organizationID, credentialID uuid.UUID) (*State, error)
	GetMany(ctx context.Context, organizationID uuid.UUID, credentialIDs []uuid.UUID) (map[uuid.UUID]*State, error)
	Ensure(ctx context.Context, organizationID, credentialID uuid.UUID) (*State, error)
	Update(ctx context.Context, state *State, fields map[string]any) (bool, error)
	UpdateUnversioned(ctx context.Context, organizationID, credentialID uuid.UUID, fields map[string]any) error
	AcquireCheckLease(ctx context.Context, organizationID, credentialID uuid.UUID, now, leaseUntil time.Time) (bool, error)
	AcquireHalfOpenLease(ctx context.Context, state *State, now, leaseUntil time.Time, manual bool) (bool, error)
	RequestManualRetry(ctx context.Context, state *State, now time.Time) (bool, error)
	ListDueChecks(ctx context.Context, now time.Time, limit int) ([]DueCheck, error)
	DueStats(ctx context.Context, now time.Time) (DueStats, error)
}

type DueCheck struct {
	CredentialID   uuid.UUID
	OrganizationID uuid.UUID
}

type DueStats struct {
	Backlog      int64
	OldestDueAge time.Duration
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Get(ctx context.Context, organizationID, credentialID uuid.UUID) (*State, error) {
	var state State
	if err := r.db.WithContext(ctx).
		Where("credential_id = ? AND organization_id = ?", credentialID, organizationID).
		First(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStateNotFound
		}
		return nil, fmt.Errorf("load upstream state: %w", err)
	}
	return &state, nil
}

func (r *gormRepository) GetMany(ctx context.Context, organizationID uuid.UUID, credentialIDs []uuid.UUID) (map[uuid.UUID]*State, error) {
	statesByCredential := make(map[uuid.UUID]*State, len(credentialIDs))
	if len(credentialIDs) == 0 {
		return statesByCredential, nil
	}
	var states []*State
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND credential_id IN ?", organizationID, credentialIDs).
		Find(&states).Error; err != nil {
		return nil, fmt.Errorf("load upstream states: %w", err)
	}
	for _, state := range states {
		statesByCredential[state.CredentialID] = state
	}
	return statesByCredential, nil
}

func (r *gormRepository) Ensure(ctx context.Context, organizationID, credentialID uuid.UUID) (*State, error) {
	state := &State{
		CredentialID:      credentialID,
		OrganizationID:    organizationID,
		Generation:        1,
		BalanceCapability: BalanceCapabilityUnknown,
		Availability:      AvailabilityUnknown,
		LastCheckStatus:   CheckStatusUnknown,
		WarningThresholds: []WarningThreshold{},
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(state).Error; err != nil {
		return nil, fmt.Errorf("create upstream state: %w", err)
	}
	return r.Get(ctx, organizationID, credentialID)
}

func (r *gormRepository) Update(ctx context.Context, state *State, fields map[string]any) (bool, error) {
	result := r.db.WithContext(ctx).Model(&State{}).
		Where("credential_id = ? AND organization_id = ? AND generation = ?", state.CredentialID, state.OrganizationID, state.Generation).
		Updates(fields)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *gormRepository) UpdateUnversioned(ctx context.Context, organizationID, credentialID uuid.UUID, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&State{}).
		Where("credential_id = ? AND organization_id = ?", credentialID, organizationID).
		Updates(fields).Error
}

func (r *gormRepository) AcquireCheckLease(ctx context.Context, organizationID, credentialID uuid.UUID, now, leaseUntil time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&State{}).
		Where("credential_id = ? AND organization_id = ?", credentialID, organizationID).
		Where("check_lease_until IS NULL OR check_lease_until <= ?", now).
		Update("check_lease_until", leaseUntil)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *gormRepository) AcquireHalfOpenLease(ctx context.Context, state *State, now, leaseUntil time.Time, manual bool) (bool, error) {
	query := r.db.WithContext(ctx).Model(&State{}).
		Where("credential_id = ? AND organization_id = ? AND generation = ?", state.CredentialID, state.OrganizationID, state.Generation).
		Where("block_reason <> ''").
		Where("half_open_lease_until IS NULL OR half_open_lease_until <= ?", now)
	if manual {
		query = query.Where("manual_retry_requested_at IS NOT NULL")
	} else {
		query = query.Where("cooldown_until IS NOT NULL AND cooldown_until <= ?", now)
	}
	result := query.Updates(map[string]any{
		"half_open_lease_until":     leaseUntil,
		"manual_retry_requested_at": nil,
		"updated_at":                now,
	})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *gormRepository) RequestManualRetry(ctx context.Context, state *State, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&State{}).
		Where("credential_id = ? AND organization_id = ? AND generation = ?", state.CredentialID, state.OrganizationID, state.Generation).
		Where("block_reason <> ''").
		Where("half_open_lease_until IS NULL OR half_open_lease_until <= ?", now).
		Updates(map[string]any{
			"manual_retry_requested_at": now,
			"updated_at":                now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *gormRepository) ListDueChecks(ctx context.Context, now time.Time, limit int) ([]DueCheck, error) {
	if limit <= 0 {
		return []DueCheck{}, nil
	}
	var checks []DueCheck
	err := r.db.WithContext(ctx).Model(&State{}).
		Select("credential_id", "organization_id").
		Where("next_check_at IS NOT NULL AND next_check_at <= ?", now).
		Where("check_lease_until IS NULL OR check_lease_until <= ?", now).
		Order("next_check_at ASC").
		Limit(limit).
		Scan(&checks).Error
	if err != nil {
		return nil, fmt.Errorf("list due upstream checks: %w", err)
	}
	return checks, nil
}

func (r *gormRepository) DueStats(ctx context.Context, now time.Time) (DueStats, error) {
	type dueStatsRow struct {
		Backlog     int64      `gorm:"column:backlog"`
		OldestDueAt *time.Time `gorm:"column:oldest_due_at"`
	}
	var row dueStatsRow
	err := r.db.WithContext(ctx).Model(&State{}).
		Select("COUNT(*) AS backlog, MIN(next_check_at) AS oldest_due_at").
		Where("next_check_at IS NOT NULL AND next_check_at <= ?", now).
		Scan(&row).Error
	if err != nil {
		return DueStats{}, fmt.Errorf("load upstream check backlog: %w", err)
	}
	stats := DueStats{Backlog: row.Backlog}
	if row.OldestDueAt != nil && now.After(*row.OldestDueAt) {
		stats.OldestDueAge = now.Sub(*row.OldestDueAt)
	}
	return stats, nil
}
