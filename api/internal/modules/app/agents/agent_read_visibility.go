package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

var errAgentReadPermissionDenied = errors.New("permission denied")

// workflowAgentTypes lists the agent_type values governed by workflow permissions.
// Every other agent_type is governed by agent permissions.
func workflowAgentTypes() []string {
	return []string{"WORKFLOW", "CONVERSATIONAL_WORKFLOW", "CONVERSATIONAL_AGENT"}
}

// agentVisiblePermissionCodesForType returns the codes that make an asset of the
// given type visible: agent rows need agent codes, workflow rows workflow codes.
func agentVisiblePermissionCodesForType(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return workflowVisiblePermissionCodes()
	}
	return agentVisiblePermissionCodes()
}

// agentDraftVisiblePermissionCodes returns the management codes that let a
// caller see unpublished assets created by other members. View-only codes
// (view, logs view) only expose published assets and the caller's own drafts.
// The sets mirror the console's editor gate (canOpenAgentRuntimeEditor /
// canOpenWorkflowEditor).
func agentDraftVisiblePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowCreate,
			model.WorkspacePermissionWorkflowImport,
			model.WorkspacePermissionWorkflowUpdate,
			model.WorkspacePermissionWorkflowRunDraft,
			model.WorkspacePermissionWorkflowPublish,
			model.WorkspacePermissionWorkflowRuntimeAccessManage,
		}
	}
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentCreate,
		model.WorkspacePermissionAgentUpdate,
		model.WorkspacePermissionAgentPublish,
		model.WorkspacePermissionAgentRuntimeAccessManage,
	}
}

// AgentVisibilityScope limits a list query to the rows of one permission family
// (agent or workflow types) that the caller may read.
type AgentVisibilityScope struct {
	// Workflow selects workflow-governed agent types; false selects all other types.
	Workflow bool
	// WorkspaceIDs are the workspaces where the caller holds a visible code for the family.
	WorkspaceIDs []string
	// DraftWorkspaceIDs are the workspaces where the caller may also see other
	// members' unpublished rows.
	DraftWorkspaceIDs []string
}

func agentAssetKindsForList(kind agentListAssetKind) []bool {
	switch kind {
	case agentListAssetKindAgent:
		return []bool{false}
	case agentListAssetKindWorkflow:
		return []bool{true}
	default:
		return []bool{false, true}
	}
}

func representativeAgentType(workflow bool) string {
	if workflow {
		return "WORKFLOW"
	}
	return "AGENT"
}

// resolveAgentListVisibilityScopes computes per-family visibility for the list
// query, so pagination and totals only count rows the caller may read.
func (s *agentsService) resolveAgentListVisibilityScopes(
	ctx context.Context,
	organizationID, accountID string,
	candidateWorkspaceIDs []string,
	kind agentListAssetKind,
) ([]AgentVisibilityScope, []string, error) {
	scopes := make([]AgentVisibilityScope, 0, 2)
	seen := make(map[string]struct{})
	workspaceIDs := make([]string, 0, len(candidateWorkspaceIDs))

	for _, workflow := range agentAssetKindsForList(kind) {
		agentType := representativeAgentType(workflow)
		visible, err := s.filterWorkspacesByAnyPermission(ctx, organizationID, accountID, candidateWorkspaceIDs, agentVisiblePermissionCodesForType(agentType))
		if err != nil {
			return nil, nil, err
		}
		if len(visible) == 0 {
			continue
		}
		drafts, err := s.filterWorkspacesByAnyPermission(ctx, organizationID, accountID, visible, agentDraftVisiblePermissionCodes(agentType))
		if err != nil {
			return nil, nil, err
		}
		scopes = append(scopes, AgentVisibilityScope{
			Workflow:          workflow,
			WorkspaceIDs:      visible,
			DraftWorkspaceIDs: drafts,
		})
		for _, id := range visible {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			workspaceIDs = append(workspaceIDs, id)
		}
	}
	return scopes, workspaceIDs, nil
}

func (s *agentsService) filterWorkspacesByAnyPermission(
	ctx context.Context,
	organizationID, accountID string,
	workspaceIDs []string,
	codes []model.WorkspacePermissionCode,
) ([]string, error) {
	result := make([]string, 0, len(workspaceIDs))
	if s.enterpriseService == nil {
		return result, nil
	}
	for _, workspaceID := range workspaceIDs {
		allowed, err := s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(ctx, organizationID, workspaceID, accountID, codes...)
		if err != nil {
			return nil, err
		}
		if allowed {
			result = append(result, workspaceID)
		}
	}
	return result, nil
}

// authorizeAgentRead applies the console read rule to one agent row: the caller
// needs a visible code for the row's family in its workspace, and an unpublished
// row is only readable by its creator or by members holding a management code
// (organization admins hold every code). It fails closed when the caller's
// account or organization is unknown.
func (s *agentsService) authorizeAgentRead(ctx context.Context, organizationID, accountID string, ag *Agent) error {
	organizationID = strings.TrimSpace(organizationID)
	accountID = strings.TrimSpace(accountID)
	if ag == nil || organizationID == "" || accountID == "" || s.enterpriseService == nil {
		return errAgentReadPermissionDenied
	}
	workspaceID := ag.TenantID.String()

	canView, err := s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(ctx, organizationID, workspaceID, accountID, agentVisiblePermissionCodesForType(ag.AgentsType)...)
	if err != nil {
		return fmt.Errorf("failed to verify permissions: %w", err)
	}
	if !canView {
		return errAgentReadPermissionDenied
	}

	if ag.CreatedBy != nil && strings.EqualFold(ag.CreatedBy.String(), accountID) {
		return nil
	}
	if published, err := s.hasPublishedVersion(ctx, ag); err != nil {
		return fmt.Errorf("failed to verify agent publication: %w", err)
	} else if published {
		return nil
	}

	canManage, err := s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(ctx, organizationID, workspaceID, accountID, agentDraftVisiblePermissionCodes(ag.AgentsType)...)
	if err != nil {
		return fmt.Errorf("failed to verify permissions: %w", err)
	}
	if !canManage {
		return errAgentReadPermissionDenied
	}
	return nil
}
