package workflow

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	core "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"strings"
)

func workflowConnectorProjectionScope(workflow *Workflow, org uuid.UUID) (connectorbindings.WorkflowProjectionScope, error) {
	if workflow == nil || org == uuid.Nil {
		return connectorbindings.WorkflowProjectionScope{}, fmt.Errorf("Workflow scope is missing")
	}
	if workflow.Version == "draft" {
		id, err := uuid.Parse(workflow.ID)
		return connectorbindings.WorkflowProjectionScope{OrganizationID: org, TargetID: id, Scope: core.ScopeDraft}, err
	}
	if !workflow.VersionUUID.Valid || workflow.VersionUUID.UUID == uuid.Nil {
		return connectorbindings.WorkflowProjectionScope{}, fmt.Errorf("Workflow version is missing")
	}
	return connectorbindings.WorkflowProjectionScope{OrganizationID: org, TargetID: workflow.VersionUUID.UUID, Scope: core.ScopePublished}, nil
}
func workflowProjectionOrganizationID(ctx context.Context, tx *gorm.DB, workflow *Workflow) (uuid.UUID, error) {
	if tx == nil || workflow == nil {
		return uuid.Nil, fmt.Errorf("Workflow context is missing")
	}
	var row struct{ OrganizationID uuid.UUID }
	if err := tx.WithContext(ctx).Table("workspaces").Select("organization_id").Where("id = ? AND status = ?", workflow.TenantID, "normal").Take(&row).Error; err != nil {
		return uuid.Nil, err
	}
	if row.OrganizationID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("Workflow organization is missing")
	}
	return row.OrganizationID, nil
}
func guardWorkflowConnectorTargetEligible(
	ctx context.Context,
	tx *gorm.DB,
	workflow *Workflow,
	organizationID uuid.UUID,
) error {
	if tx == nil || workflow == nil || organizationID == uuid.Nil {
		return fmt.Errorf("workflow Connector target scope is required")
	}
	agentID, err := uuid.Parse(strings.TrimSpace(workflow.AgentID))
	if err != nil || agentID == uuid.Nil {
		return fmt.Errorf("workflow owning Agent id is invalid")
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(workflow.TenantID))
	if err != nil || workspaceID == uuid.Nil {
		return fmt.Errorf("workflow workspace id is invalid")
	}
	var target struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	result := tx.WithContext(ctx).
		Table("agents AS owner").
		Select("owner.id").
		Joins("JOIN workspaces AS workspace ON workspace.id = owner.tenant_id AND workspace.status = ?", "normal").
		Where(
			"owner.id = ? AND owner.tenant_id = ? AND owner.deleted_at IS NULL AND workspace.organization_id = ?",
			agentID, workspaceID, organizationID,
		).
		Take(&target)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("workflow Connector target is unavailable")
	}
	if result.Error != nil {
		return fmt.Errorf("validate workflow Connector target: %w", result.Error)
	}
	if target.ID != agentID {
		return fmt.Errorf("workflow Connector target identity is invalid")
	}
	return nil
}

func guardWorkflowDraftSnapshot(ctx context.Context, tx *gorm.DB, draft *Workflow) error {
	if tx == nil || draft == nil {
		return fmt.Errorf("draft Workflow snapshot is required")
	}
	result := tx.WithContext(ctx).Model(&Workflow{}).
		Where("id = ? AND version = ? AND graph = ? AND updated_at = ?", draft.ID, "draft", draft.Graph, draft.UpdatedAt).
		UpdateColumn("updated_at", gorm.Expr("updated_at"))
	if result.Error != nil {
		return fmt.Errorf("lock draft Workflow snapshot: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("draft Workflow changed during publish")
	}
	return nil
}
