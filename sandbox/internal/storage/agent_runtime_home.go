package storage

import (
	"context"
	"database/sql"
	"errors"

	agentruntime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

var _ agentruntime.HomeSnapshotReader = (*Store)(nil)

func (s *Store) HomeSnapshot(ctx context.Context, id string) (agentruntime.HomeSnapshot, error) {
	var snapshot agentruntime.HomeSnapshot
	err := s.db.QueryRowContext(ctx, `
		SELECT id, organization_id, subject_id, profile_id, backend, backend_ref, status
		FROM agent_runtime_home_snapshots WHERE id = $1
	`, id).Scan(
		&snapshot.ID, &snapshot.OrganizationID, &snapshot.SubjectID,
		&snapshot.ProfileID, &snapshot.Backend, &snapshot.BackendRef, &snapshot.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.HomeSnapshot{}, agentruntime.ErrNotFound
	}
	return snapshot, err
}
