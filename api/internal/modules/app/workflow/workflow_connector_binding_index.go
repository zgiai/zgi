package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	core "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type transactionalWorkflowRepository interface {
	DB() *gorm.DB
	WithTx(*gorm.DB) WorkflowRepository
}

func (s *WorkflowService) configureWorkflowBindingIndex() {
	if s == nil || s.repo == nil {
		return
	}
	repo, ok := s.repo.(transactionalWorkflowRepository)
	if ok && repo.DB() != nil {
		s.workflowDB = repo.DB()
		s.connectorBindings = connectorbindings.NewGormRepository(repo.DB())
	}
}
func (s *WorkflowService) persistWorkflowWithBindings(ctx context.Context, workflow *Workflow, selections []workflowConnectorSelection, create bool) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("Workflow repository is unavailable")
	}
	repository, ok := s.repo.(transactionalWorkflowRepository)
	if !ok || s.workflowDB == nil || s.connectorBindings == nil {
		if len(selections) > 0 || len(workflow.ConnectorReferenceCommands) > 0 {
			return fmt.Errorf("Connector repository is unavailable")
		}
		if create {
			return s.repo.Create(ctx, workflow)
		}
		return s.repo.Update(ctx, workflow)
	}
	if workflow.Version != "draft" {
		return fmt.Errorf("Published Workflow is immutable")
	}
	return s.workflowDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org, err := workflowProjectionOrganizationID(ctx, tx, workflow)
		if err != nil {
			return err
		}
		scope, err := workflowConnectorProjectionScope(workflow, org)
		if err != nil {
			return err
		}
		if err = s.connectorBindings.LockWorkflowProjection(ctx, tx, scope); err != nil {
			return err
		}
		if err = guardWorkflowConnectorTargetEligible(ctx, tx, workflow, org); err != nil {
			return err
		}
		if err = s.recordUnconfiguredConnectorReferences(ctx, tx, org, workflow); err != nil {
			return err
		}
		repo := connectorbindings.NewGormRepository(tx)
		placements := make([]connectorbindings.WorkflowPlacement, 0, len(selections))
		if err := s.resolveWorkflowConnectorReferences(ctx, tx, org, workflow); err != nil {
			return err
		}
		if len(workflow.ConnectorReferenceCommands) > 0 {
			actor := workflow.CreatedBy
			if workflow.UpdatedBy != nil {
				actor = *workflow.UpdatedBy
			}
			selections, err = s.workflowConnectorBindingRows(ctx, workflow, workflow.GetGraphDict(), actor)
			if err != nil {
				return err
			}
		}
		existing, err := s.connectorBindings.ListActiveWorkflowProjection(ctx, tx, scope)
		if err != nil {
			return err
		}
		byNode := map[string]core.TargetBinding{}
		for _, binding := range existing {
			byNode[binding.Target.BindingInstanceID] = binding
		}
		for _, selection := range selections {
			constraint := connectorbindings.NewBindingConstraint(selection.OwnerID, selection.ConnectorID)
			if previous, found := byNode[selection.NodeID]; found {
				if previous.OwnerAccountID != selection.OwnerID {
					return fmt.Errorf("Connector binding belongs to another owner")
				}
				constraint = previous.Constraint
			}
			if selection.BindingID != uuid.Nil {
				source, err := repo.GetBinding(ctx, org, selection.OwnerID, selection.BindingID)
				if err != nil {
					return err
				}
				if source.ConnectorID != selection.ConnectorID || source.Target.Type != core.TargetTypeWorkflow {
					return fmt.Errorf("Connector source binding identity changed")
				}
				constraint, err = core.IntersectConstraints(constraint, source.Constraint)
				if err != nil {
					return err
				}
			}
			if constraint.Validate() != nil {
				return fmt.Errorf("Connector binding restrictions are unavailable")
			}
			placements = append(placements, connectorbindings.WorkflowPlacement{OwnerAccountID: selection.OwnerID, ConnectionID: selection.ConnectionID, ConnectorID: selection.ConnectorID, BindingInstanceID: selection.NodeID, Constraint: constraint})
		}
		if err = s.connectorBindings.ReplaceWorkflowDraftProjection(ctx, tx, scope, placements); err != nil {
			return err
		}
		bindings, err := s.connectorBindings.ListActiveWorkflowProjection(ctx, tx, scope)
		if err != nil {
			return err
		}
		if err = rewriteWorkflowConnectorIDs(workflow, bindings); err != nil {
			return err
		}
		if create {
			return repository.WithTx(tx).Create(ctx, workflow)
		}
		return repository.WithTx(tx).Update(ctx, workflow)
	})
}
func (s *WorkflowService) persistPublishedWorkflowWithBindings(ctx context.Context, published, draft *Workflow, selections []workflowConnectorSelection) error {
	repository, ok := s.repo.(transactionalWorkflowRepository)
	if !ok || s.workflowDB == nil || s.connectorBindings == nil {
		return fmt.Errorf("Connector publish repository is unavailable")
	}
	return s.workflowDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org, err := workflowProjectionOrganizationID(ctx, tx, draft)
		if err != nil {
			return err
		}
		source, err := workflowConnectorProjectionScope(draft, org)
		if err != nil {
			return err
		}
		destination, err := workflowConnectorProjectionScope(published, org)
		if err != nil {
			return err
		}
		agent, err := uuid.Parse(draft.AgentID)
		if err != nil {
			return err
		}
		if err = s.connectorBindings.LockWorkflowLineage(ctx, tx, org, agent); err != nil {
			return err
		}
		if err = s.connectorBindings.LockWorkflowProjection(ctx, tx, source); err != nil {
			return err
		}
		if err = guardWorkflowConnectorTargetEligible(ctx, tx, draft, org); err != nil {
			return err
		}
		if err = guardWorkflowDraftSnapshot(ctx, tx, draft); err != nil {
			return err
		}
		head, err := loadWorkflowAgentPublicationHead(ctx, tx, draft, false)
		if err != nil {
			return err
		}
		if err = s.connectorBindings.SnapshotWorkflowPublishedProjection(ctx, tx, source, destination); err != nil {
			return err
		}
		bindings, err := s.connectorBindings.ListActiveWorkflowProjection(ctx, tx, destination)
		if err != nil {
			return err
		}
		if !workflowPublishedBindingsMatch(selections, bindings) {
			return fmt.Errorf("Connector publish snapshot does not match graph")
		}
		if err = rewriteWorkflowConnectorIDs(published, bindings); err != nil {
			return err
		}
		if err = repository.WithTx(tx).CreateWorkflow(ctx, published); err != nil {
			return err
		}
		return advanceWorkflowAgentPublicationHead(ctx, tx, draft, published, head)
	})
}
func (s *WorkflowService) deleteWorkflowWithBindings(ctx context.Context, workflow *Workflow) error {
	repository, ok := s.repo.(transactionalWorkflowRepository)
	if !ok || s.workflowDB == nil || s.connectorBindings == nil {
		return s.repo.Delete(ctx, workflow.ID)
	}
	return s.workflowDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org, err := workflowProjectionOrganizationID(ctx, tx, workflow)
		if err != nil {
			return err
		}
		agent, err := uuid.Parse(workflow.AgentID)
		if err != nil {
			return err
		}
		if err = s.connectorBindings.LockWorkflowLineage(ctx, tx, org, agent); err != nil {
			return err
		}
		var lineage []Workflow
		if err = tx.WithContext(ctx).Where("agent_id = ? AND tenant_id = ?", workflow.AgentID, workflow.TenantID).Find(&lineage).Error; err != nil {
			return err
		}
		scopes := []connectorbindings.WorkflowProjectionScope{}
		for i := range lineage {
			scope, err := workflowConnectorProjectionScope(&lineage[i], org)
			if err != nil {
				return err
			}
			scopes = append(scopes, scope)
		}
		if err = s.connectorBindings.RevokeWorkflowProjections(ctx, tx, scopes); err != nil {
			return err
		}
		return repository.WithTx(tx).Delete(ctx, workflow.ID)
	})
}

type workflowAgentPublicationHead struct {
	WorkflowID     *uuid.UUID `gorm:"column:workflow_id"`
	WorkflowConfig *string    `gorm:"column:workflow_config"`
}

func loadWorkflowAgentPublicationHead(
	ctx context.Context,
	tx *gorm.DB,
	workflow *Workflow,
	lock bool,
) (workflowAgentPublicationHead, error) {
	if tx == nil || workflow == nil {
		return workflowAgentPublicationHead{}, fmt.Errorf("workflow publication head is required")
	}
	agentID, err := uuid.Parse(strings.TrimSpace(workflow.AgentID))
	if err != nil || agentID == uuid.Nil {
		return workflowAgentPublicationHead{}, fmt.Errorf("workflow owning Agent id is invalid")
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(workflow.TenantID))
	if err != nil || workspaceID == uuid.Nil {
		return workflowAgentPublicationHead{}, fmt.Errorf("workflow workspace id is invalid")
	}
	query := tx.WithContext(ctx).
		Table("agents").
		Select("workflow_id, workflow_config").
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", agentID, workspaceID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var head workflowAgentPublicationHead
	if err := query.Take(&head).Error; err != nil {
		return workflowAgentPublicationHead{}, fmt.Errorf("load Workflow publication head: %w", err)
	}
	return head, nil
}

func advanceWorkflowAgentPublicationHead(
	ctx context.Context,
	tx *gorm.DB,
	draft, published *Workflow,
	expected workflowAgentPublicationHead,
) error {
	locked, err := loadWorkflowAgentPublicationHead(ctx, tx, draft, true)
	if err != nil {
		return err
	}
	if !sameOptionalUUID(expected.WorkflowID, locked.WorkflowID) {
		return fmt.Errorf("Workflow publication head changed during publish")
	}

	config := make(map[string]any)
	if locked.WorkflowConfig != nil && strings.TrimSpace(*locked.WorkflowConfig) != "" {
		if err := json.Unmarshal([]byte(*locked.WorkflowConfig), &config); err != nil || config == nil {
			return fmt.Errorf("Workflow publication head config is invalid")
		}
	}
	config["workflow_id"] = published.ID
	if _, ok := config["variable_config"]; !ok {
		config["variable_config"] = map[string]any{}
	}
	if _, ok := config["history_window_size"]; !ok {
		config["history_window_size"] = 0
	}
	if _, ok := config["conversation_id"]; !ok {
		config["conversation_id"] = ""
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode Workflow publication head config: %w", err)
	}

	agentID, err := uuid.Parse(strings.TrimSpace(draft.AgentID))
	if err != nil || agentID == uuid.Nil {
		return fmt.Errorf("workflow owning Agent id is invalid")
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(draft.TenantID))
	if err != nil || workspaceID == uuid.Nil {
		return fmt.Errorf("workflow workspace id is invalid")
	}
	publishedID, err := uuid.Parse(strings.TrimSpace(published.ID))
	if err != nil || publishedID == uuid.Nil {
		return fmt.Errorf("published workflow id is invalid")
	}
	query := tx.WithContext(ctx).
		Table("agents").
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", agentID, workspaceID)
	if expected.WorkflowID == nil || *expected.WorkflowID == uuid.Nil {
		query = query.Where("workflow_id IS NULL")
	} else {
		query = query.Where("workflow_id = ?", *expected.WorkflowID)
	}
	result := query.Updates(map[string]any{
		"workflow_id":     publishedID,
		"workflow_config": string(configJSON),
		"updated_at":      time.Now().UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("advance Workflow publication head: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("Workflow publication head changed during publish")
	}
	return nil
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if left == nil || *left == uuid.Nil {
		return right == nil || *right == uuid.Nil
	}
	return right != nil && *right != uuid.Nil && *left == *right
}

func workflowPublishedBindingsMatch(selections []workflowConnectorSelection, bindings []core.TargetBinding) bool {
	if len(bindings) != len(selections) {
		return false
	}
	byNode := map[string]core.TargetBinding{}
	for _, binding := range bindings {
		if _, found := byNode[binding.Target.BindingInstanceID]; found {
			return false
		}
		byNode[binding.Target.BindingInstanceID] = binding
	}
	for _, selection := range selections {
		binding, found := byNode[selection.NodeID]
		if !found || binding.OwnerAccountID != selection.OwnerID || binding.ConnectionID != selection.ConnectionID || binding.ConnectorID != selection.ConnectorID || !binding.Constraint.Allows(selection.OwnerID, selection.OperationID, binding.Target, time.Now()) {
			return false
		}
	}
	return true
}
