package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gorm.io/gorm"
)

type transactionalWorkflowRepository interface {
	DB() *gorm.DB
	WithTx(tx *gorm.DB) WorkflowRepository
}

func (s *WorkflowService) configureWorkflowBindingIndex() {
	if s == nil || s.repo == nil {
		return
	}
	repository, ok := s.repo.(transactionalWorkflowRepository)
	if !ok || repository.DB() == nil {
		return
	}
	s.workflowDB = repository.DB()
	s.workflowBindings = agentbindings.NewRepository(repository.DB())
}

func (s *WorkflowService) workflowConnectorBindingRows(
	ctx context.Context,
	workflow *Workflow,
	graph map[string]interface{},
	accountID string,
) ([]agentbindings.Binding, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow is required for connector binding extraction")
	}

	organizationID := ""
	dependencies, err := extractWorkflowConnectorDependencies(ctx, graph, func(ctx context.Context, integrationID, toolName string) (workflowConnectorAction, error) {
		if organizationID == "" {
			organizationID = strings.TrimSpace(s.getOrganizationIDByWorkspace(ctx, workflow.TenantID))
			if organizationID == "" {
				return workflowConnectorAction{}, fmt.Errorf("organization for workspace %s was not found", workflow.TenantID)
			}
		}
		return s.resolveWorkflowConnectorAction(ctx, organizationID, integrationID, toolName)
	})
	if err != nil {
		return nil, err
	}
	if len(dependencies) == 0 {
		return []agentbindings.Binding{}, nil
	}

	organizationUUID, err := uuid.Parse(organizationID)
	if err != nil || organizationUUID == uuid.Nil {
		return nil, fmt.Errorf("workflow organization id is invalid")
	}
	workspaceUUID, err := uuid.Parse(strings.TrimSpace(workflow.TenantID))
	if err != nil || workspaceUUID == uuid.Nil {
		return nil, fmt.Errorf("workflow workspace id is invalid")
	}
	agentUUID, err := uuid.Parse(strings.TrimSpace(workflow.AgentID))
	if err != nil || agentUUID == uuid.Nil {
		return nil, fmt.Errorf("workflow owning Agent id is invalid")
	}
	workflowUUID, err := uuid.Parse(strings.TrimSpace(workflow.ID))
	if err != nil || workflowUUID == uuid.Nil {
		return nil, fmt.Errorf("workflow id is invalid")
	}

	var authorizedBy *uuid.UUID
	if actor, parseErr := uuid.Parse(strings.TrimSpace(accountID)); parseErr == nil && actor != uuid.Nil {
		authorizedBy = &actor
	}
	authorizedAt := time.Now()
	bindings := make([]agentbindings.Binding, 0, len(dependencies))
	for _, dependency := range dependencies {
		bindings = append(bindings, agentbindings.Binding{
			AgentID:           agentUUID,
			ConsumerType:      agentbindings.ConsumerTypeWorkflow,
			ConsumerID:        workflowUUID,
			BindingInstanceID: dependency.NodeID,
			OrganizationID:    organizationUUID,
			WorkspaceID:       workspaceUUID,
			BindingType:       agentbindings.BindingTypeIntegrationConnection,
			ResourceID:        dependency.ConnectionID,
			ParentResourceID:  dependency.IntegrationID,
			DisplayName:       dependency.NodeTitle,
			AccessMode:        dependency.AccessMode,
			Metadata: map[string]interface{}{
				agentbindings.IntegrationAllowedActionIDsMetadataKey: []string{dependency.ActionID},
				"tool_name": dependency.ToolName,
			},
			AuthorizedBy: authorizedBy,
			AuthorizedAt: &authorizedAt,
		})
	}
	return bindings, nil
}

func (s *WorkflowService) resolveWorkflowConnectorAction(
	ctx context.Context,
	organizationID string,
	integrationID string,
	toolName string,
) (workflowConnectorAction, error) {
	if s == nil || s.executor == nil {
		return workflowConnectorAction{}, fmt.Errorf("workflow tool runtime is not configured")
	}
	toolEngine, ok := s.executor.GetToolEngine().(*tools.ToolEngine)
	if !ok || toolEngine == nil {
		return workflowConnectorAction{}, fmt.Errorf("workflow ToolEngine is not configured")
	}
	entity, err := toolEngine.ResolveToolEntity(
		ctx,
		tools.ToolProviderTypeConnector,
		strings.TrimSpace(integrationID),
		strings.TrimSpace(toolName),
		strings.TrimSpace(organizationID),
	)
	if err != nil {
		return workflowConnectorAction{}, err
	}
	if entity == nil || entity.Governance == nil || strings.TrimSpace(entity.Governance.ToolID) == "" {
		return workflowConnectorAction{}, fmt.Errorf("connector tool has no provider Action identity")
	}
	if len(entity.SupportedCallers) > 0 && !workflowCallerSupported(entity.SupportedCallers) {
		return workflowConnectorAction{}, fmt.Errorf("connector Action does not support Workflow callers")
	}
	accessMode := "write"
	if strings.EqualFold(strings.TrimSpace(entity.Governance.Effect), string(toolgovernance.EffectRead)) {
		accessMode = "read"
	}
	return workflowConnectorAction{ActionID: entity.Governance.ToolID, AccessMode: accessMode}, nil
}

func workflowCallerSupported(callers []tools.ToolInvokeFrom) bool {
	for _, caller := range callers {
		if caller == tools.ToolInvokeFromWorkflow {
			return true
		}
	}
	return false
}

func workflowBindingScopeRef(workflow *Workflow) (agentbindings.WorkflowScopeRef, error) {
	if workflow == nil {
		return agentbindings.WorkflowScopeRef{}, fmt.Errorf("workflow is required")
	}
	agentID, err := uuid.Parse(strings.TrimSpace(workflow.AgentID))
	if err != nil || agentID == uuid.Nil {
		return agentbindings.WorkflowScopeRef{}, fmt.Errorf("workflow owning Agent id is invalid")
	}
	workflowID, err := uuid.Parse(strings.TrimSpace(workflow.ID))
	if err != nil || workflowID == uuid.Nil {
		return agentbindings.WorkflowScopeRef{}, fmt.Errorf("workflow id is invalid")
	}
	ref := agentbindings.WorkflowScopeRef{AgentID: agentID, WorkflowID: workflowID}
	if strings.EqualFold(strings.TrimSpace(workflow.Version), "draft") {
		ref.Scope = agentbindings.ScopeDraft
		return ref, nil
	}
	if !workflow.VersionUUID.Valid || workflow.VersionUUID.UUID == uuid.Nil {
		return agentbindings.WorkflowScopeRef{}, fmt.Errorf("published workflow version UUID is required")
	}
	versionUUID := workflow.VersionUUID.UUID
	ref.Scope = agentbindings.ScopePublished
	ref.PublishedVersionUUID = &versionUUID
	return ref, nil
}

func (s *WorkflowService) persistWorkflowWithBindings(
	ctx context.Context,
	workflow *Workflow,
	bindings []agentbindings.Binding,
	create bool,
) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("workflow repository not initialized")
	}
	ref, err := workflowBindingScopeRef(workflow)
	if err != nil {
		return err
	}
	repository, transactional := s.repo.(transactionalWorkflowRepository)
	if !transactional || s.workflowDB == nil || s.workflowBindings == nil {
		if len(bindings) > 0 {
			return fmt.Errorf("workflow connector binding index is not configured")
		}
		if create {
			if ref.Scope == agentbindings.ScopePublished {
				return s.repo.CreateWorkflow(ctx, workflow)
			}
			return s.repo.Create(ctx, workflow)
		}
		return s.repo.Update(ctx, workflow)
	}

	return s.workflowDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := repository.WithTx(tx)
		// Replace the derived ledger first so every mutation acquires canonical
		// resource locks before touching the Workflow row. Connection detach and
		// deletion use the same order, which prevents a graph-row/resource-lock
		// deadlock. Both writes remain invisible until this transaction commits.
		if ref.Scope == agentbindings.ScopePublished {
			if err := s.workflowBindings.ReplaceWorkflowPublishedHead(ctx, tx, ref, bindings); err != nil {
				return err
			}
		} else if err := s.workflowBindings.ReplaceWorkflowScope(ctx, tx, ref, bindings); err != nil {
			return err
		}
		if create {
			if ref.Scope == agentbindings.ScopePublished {
				if err := txRepo.CreateWorkflow(ctx, workflow); err != nil {
					return err
				}
			} else if err := txRepo.Create(ctx, workflow); err != nil {
				return err
			}
		} else if err := txRepo.Update(ctx, workflow); err != nil {
			return err
		}
		workflowID, err := uuid.Parse(strings.TrimSpace(workflow.ID))
		if err != nil || workflowID == uuid.Nil {
			return fmt.Errorf("workflow id is invalid")
		}
		if err := completeWorkflowConnectorBindingBackfill(ctx, tx, workflowID, false); err != nil {
			return err
		}
		return nil
	})
}

func (s *WorkflowService) deleteWorkflowWithBindings(ctx context.Context, workflow *Workflow) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("workflow repository not initialized")
	}
	ref, err := workflowBindingScopeRef(workflow)
	if err != nil {
		return err
	}
	repository, transactional := s.repo.(transactionalWorkflowRepository)
	if !transactional || s.workflowDB == nil || s.workflowBindings == nil {
		return s.repo.Delete(ctx, workflow.ID)
	}
	return s.workflowDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Revoke and lock the exact scope before deleting the graph row. This
		// matches connection detach lock ordering and remains atomic on rollback.
		if err := s.workflowBindings.ReplaceWorkflowScope(ctx, tx, ref, nil); err != nil {
			return err
		}
		return repository.WithTx(tx).Delete(ctx, workflow.ID)
	})
}
