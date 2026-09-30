package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

type workflowConnectorProjectionRepository interface {
	LockWorkflowLineage(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error
	RevokeWorkflowProjections(context.Context, *gorm.DB, []connectorbindings.WorkflowProjectionScope) error
}

// revokeAgentWorkflowConnectorLineage is the canonical-binding half of Agent
// deletion. The caller removes ordinary resource bindings and soft-deletes the
// Agent in the same transaction after this returns.
func (s *agentsService) revokeAgentWorkflowConnectorLineage(
	ctx context.Context,
	tx *gorm.DB,
	organizationID, workspaceID, agentID uuid.UUID,
) error {
	if tx == nil || organizationID == uuid.Nil || workspaceID == uuid.Nil || agentID == uuid.Nil {
		return fmt.Errorf("Workflow connector deletion scope is invalid")
	}
	repository, ok := s.connectorTargetBindings.(workflowConnectorProjectionRepository)
	if !ok || repository == nil {
		return fmt.Errorf("Workflow connector projection is not configured")
	}
	if err := repository.LockWorkflowLineage(ctx, tx, organizationID, agentID); err != nil {
		return err
	}

	var rows []struct {
		ID          uuid.UUID     `gorm:"column:id"`
		Version     string        `gorm:"column:version"`
		VersionUUID uuid.NullUUID `gorm:"column:version_uuid"`
	}
	if err := tx.WithContext(ctx).
		Table("workflows").
		Select("id, version, version_uuid").
		Where("agent_id = ? AND tenant_id = ?", agentID, workspaceID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("load Agent Workflow connector lineage: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	scopes := make([]connectorbindings.WorkflowProjectionScope, 0, len(rows))
	for _, row := range rows {
		targetID := row.ID
		scope := core.ScopeDraft
		if !strings.EqualFold(strings.TrimSpace(row.Version), "draft") {
			if !row.VersionUUID.Valid || row.VersionUUID.UUID == uuid.Nil {
				return fmt.Errorf("published Agent Workflow connector lineage is invalid")
			}
			targetID = row.VersionUUID.UUID
			scope = core.ScopePublished
		}
		scopes = append(scopes, connectorbindings.WorkflowProjectionScope{
			OrganizationID: organizationID,
			TargetID:       targetID, Scope: scope,
		})
	}
	return repository.RevokeWorkflowProjections(ctx, tx, scopes)
}
