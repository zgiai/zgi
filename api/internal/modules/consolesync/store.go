package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Store persists the sync cursor and the idempotency ledger.
type Store struct {
	db *gorm.DB
}

// NewStore creates the local state store.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// LoadState returns the singleton state row, creating it when missing.
func (s *Store) LoadState(ctx context.Context) (*PreSyncState, error) {
	var state PreSyncState
	err := s.db.WithContext(ctx).Where("id = ?", syncStateSingletonID).First(&state).Error
	if err == nil {
		// The driver returns timestamptz in its own location (pgx: time.Local). Pin it to
		// the real UTC instant so write and read agree everywhere downstream.
		state.normalizeTimesUTC()
		return &state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load sync state: %w", err)
	}

	now := time.Now().UTC()
	state = PreSyncState{ID: syncStateSingletonID, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&state).Error; err != nil {
		return nil, fmt.Errorf("create sync state: %w", err)
	}
	return &state, nil
}

// EnsureStateWith creates the singleton state row when it is missing, using an explicit
// handle so a caller inside a transaction sees its own write.
func (s *Store) EnsureStateWith(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		db = s.db
	}
	var count int64
	if err := db.WithContext(ctx).Model(&PreSyncState{}).
		Where("id = ?", syncStateSingletonID).Count(&count).Error; err != nil {
		return fmt.Errorf("check sync state: %w", err)
	}
	if count > 0 {
		return nil
	}
	now := time.Now().UTC()
	return db.WithContext(ctx).Create(&PreSyncState{
		ID:        syncStateSingletonID,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error
}

// SaveState writes the mutable fields of the singleton state row.
//
// Every timestamp is normalised to UTC before it reaches the driver, so a caller that
// passes a local-zone time still stores the real instant that LoadState reads back.
func (s *Store) SaveState(ctx context.Context, state *PreSyncState) error {
	if state == nil {
		return nil
	}
	state.normalizeTimesUTC()
	state.UpdatedAt = time.Now().UTC()
	return s.db.WithContext(ctx).
		Model(&PreSyncState{}).
		Where("id = ?", syncStateSingletonID).
		Updates(map[string]interface{}{
			"downstream_client_id": state.DownstreamClientID,
			"cursor_seq":           state.CursorSeq,
			"applied_version":      state.AppliedVersion,
			"last_heartbeat_at":    state.LastHeartbeatAt,
			"last_pull_at":         state.LastPullAt,
			"consecutive_failures": state.ConsecutiveFailures,
			"next_poll_at":         state.NextPollAt,
			"credential_status":    state.CredentialStatus,
			"last_error":           state.LastError,
			"updated_at":           state.UpdatedAt,
		}).Error
}

// WasApplied reports whether the command was already processed.
func (s *Store) WasApplied(ctx context.Context, commandID uuid.UUID) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&PreAppliedCommand{}).
		Where("command_id = ?", commandID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check applied command: %w", err)
	}
	return count > 0, nil
}

// LastAppliedSeq returns the highest seq recorded in the idempotency ledger.
func (s *Store) LastAppliedSeq(ctx context.Context) (int64, error) {
	var seq *int64
	err := s.db.WithContext(ctx).
		Model(&PreAppliedCommand{}).
		Select("MAX(seq)").
		Scan(&seq).Error
	if err != nil {
		return 0, fmt.Errorf("read last applied seq: %w", err)
	}
	if seq == nil {
		return 0, nil
	}
	return *seq, nil
}

// RecordApplied stores the idempotency ledger row for a processed command.
func (s *Store) RecordApplied(ctx context.Context, entry *PreAppliedCommand) error {
	if entry == nil {
		return nil
	}
	if entry.AppliedAt.IsZero() {
		entry.AppliedAt = time.Now().UTC()
	}
	return s.db.WithContext(ctx).Create(entry).Error
}

// ControlledSnapshot captures the local values of the fields console owns, keyed by the
// same field names the desired state uses. It is the input of the light reconciler.
type ControlledSnapshot struct {
	OrganizationID   string
	OrganizationName string
	AccountStatus    string
	WorkspaceQuotas  map[string]int64
	APIKeyStatuses   map[string]string
}

// SnapshotControlledFields reads the controlled fields from the real tables.
//
// A Cloud Pre deployment can host multiple organizations. Heartbeat is an
// instance-level signal, so it must not fail just because there is more than one
// organization. For a multi-organization deployment we only report account status
// when every organization has the same status; an empty value means "mixed" and
// avoids pretending that one tenant represents the whole instance.
func (s *Store) SnapshotControlledFields(ctx context.Context) (*ControlledSnapshot, error) {
	snapshot := &ControlledSnapshot{
		WorkspaceQuotas: map[string]int64{},
		APIKeyStatuses:  map[string]string{},
	}

	organizationIDs, err := s.ResolveOrganizationIDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(organizationIDs) == 0 {
		return snapshot, nil
	}
	if len(organizationIDs) == 1 {
		snapshot.OrganizationID = organizationIDs[0]
	}

	var orgs []struct {
		ID     string `gorm:"column:id"`
		Name   string `gorm:"column:name"`
		Status string `gorm:"column:status"`
	}
	if err := s.db.WithContext(ctx).
		Table("organizations").
		Select("id, name, status").
		Where("id IN ?", organizationIDs).
		Scan(&orgs).Error; err != nil {
		return nil, fmt.Errorf("read organization state: %w", err)
	}
	if len(orgs) == 1 {
		snapshot.OrganizationName = orgs[0].Name
		snapshot.AccountStatus = orgs[0].Status
	} else if len(orgs) > 1 {
		status := strings.TrimSpace(orgs[0].Status)
		allSame := status != ""
		for _, org := range orgs[1:] {
			if strings.TrimSpace(org.Status) != status {
				allSame = false
				break
			}
		}
		if allSame {
			snapshot.AccountStatus = status
		}
	}

	var quotas []struct {
		WorkspaceID string `gorm:"column:workspace_id"`
		QuotaLimit  *int64 `gorm:"column:quota_limit"`
	}
	if err := s.db.WithContext(ctx).
		Table("llm_workspace_quotas").
		Select("workspace_id, quota_limit").
		Where("organization_id IN ?", organizationIDs).
		Scan(&quotas).Error; err != nil {
		return nil, fmt.Errorf("read workspace quotas: %w", err)
	}
	for _, quota := range quotas {
		if quota.QuotaLimit != nil {
			snapshot.WorkspaceQuotas[quota.WorkspaceID] = *quota.QuotaLimit
		}
	}

	var keys []struct {
		ID     string `gorm:"column:id"`
		Status string `gorm:"column:status"`
	}
	if err := s.db.WithContext(ctx).
		Table("llm_organization_api_keys").
		Select("id, status").
		Where("organization_id IN ?", organizationIDs).
		Scan(&keys).Error; err != nil {
		return nil, fmt.Errorf("read api key states: %w", err)
	}
	for _, key := range keys {
		snapshot.APIKeyStatuses[key.ID] = key.Status
	}

	return snapshot, nil
}

// ResolveOrganizationID returns the organization this deployment serves.
func (s *Store) ResolveOrganizationID(ctx context.Context) (string, error) {
	return s.ResolveOrganizationIDWith(ctx, s.db)
}

// ResolveOrganizationIDs returns every organization hosted by this Pre instance.
func (s *Store) ResolveOrganizationIDs(ctx context.Context) ([]string, error) {
	return s.ResolveOrganizationIDsWith(ctx, s.db)
}

func (s *Store) ResolveOrganizationIDsWith(ctx context.Context, db *gorm.DB) ([]string, error) {
	if db == nil {
		db = s.db
	}
	var ids []string
	if err := db.WithContext(ctx).
		Table("organizations").
		Select("id").
		Order("created_at ASC").
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("resolve organizations: %w", err)
	}
	return ids, nil
}

// ResolveOrganizationIDWith resolves the organization using an explicit handle so callers
// inside a transaction read their own uncommitted state.
//
// A pre deployment hosts exactly one organization (plan §1.1). If the table holds a
// different number of rows the caller gets a clear error instead of a silent guess.
func (s *Store) ResolveOrganizationIDWith(ctx context.Context, db *gorm.DB) (string, error) {
	if db == nil {
		db = s.db
	}
	var ids []string
	if err := db.WithContext(ctx).
		Table("organizations").
		Select("id").
		Order("created_at ASC").
		Limit(2).
		Pluck("id", &ids).Error; err != nil {
		return "", fmt.Errorf("resolve organization: %w", err)
	}
	if len(ids) == 0 {
		return "", nil
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("console sync requires exactly one organization, found more than one")
	}
	return ids[0], nil
}

// MarshalState encodes reported state for storage.
func MarshalState(value interface{}) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
