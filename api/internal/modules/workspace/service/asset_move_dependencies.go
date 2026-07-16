package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

// PreviewDependencies resolves target-independent Agent binding dependencies.
// Target workspace status, destination permissions, folders, and other concrete
// move rules remain part of the target-specific Preview and Move operations.
func (s *WorkspaceAssetMoveService) PreviewDependencies(
	ctx context.Context,
	organizationID, accountID string,
	req dto.WorkspaceAssetMoveDependencyPreviewRequest,
) (*dto.WorkspaceAssetMoveDependencyPreviewResponse, error) {
	organizationID = strings.TrimSpace(organizationID)
	accountID = strings.TrimSpace(accountID)
	if organizationID == "" || accountID == "" || len(req.Items) == 0 || s.authorizationService == nil {
		return nil, ErrAssetMoveInvalidRequest
	}
	if _, err := s.authorizationService.RequireOrganizationMember(ctx, interfaces.OrganizationScopeRequest{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}); err != nil {
		return nil, assetMoveAuthorizationError(err)
	}

	organizationUUID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, ErrAssetMoveInvalidRequest
	}
	moveRequest := agentbindings.MoveDependencyRequest{OrganizationID: organizationUUID}
	seenItems := make(map[string]struct{}, len(req.Items))
	checkedSources := make(map[string]struct{})
	for _, rawItem := range req.Items {
		item := dto.WorkspaceAssetMoveItem{
			Type: strings.ToLower(strings.TrimSpace(rawItem.Type)),
			ID:   strings.TrimSpace(rawItem.ID),
		}
		if item.Type == "" || item.ID == "" {
			return nil, ErrAssetMoveInvalidRequest
		}
		itemKey := item.Type + ":" + item.ID
		if _, exists := seenItems[itemKey]; exists {
			return nil, ErrAssetMoveInvalidRequest
		}
		seenItems[itemKey] = struct{}{}

		scope, err := s.resolveAssetMoveScope(ctx, s.db, organizationID, item)
		if err != nil {
			return nil, err
		}
		if scope.sourceWorkspaceID != "" {
			checkKey := scope.sourceWorkspaceID + ":" + string(scope.permission)
			if _, checked := checkedSources[checkKey]; !checked {
				if err := s.requireAssetMovePermission(ctx, organizationID, scope.sourceWorkspaceID, accountID, scope.permission); err != nil {
					return nil, err
				}
				checkedSources[checkKey] = struct{}{}
			}
		}

		var sourceWorkspaceID *uuid.UUID
		if scope.sourceWorkspaceID != "" {
			parsedWorkspaceID, err := uuid.Parse(scope.sourceWorkspaceID)
			if err != nil {
				return nil, ErrAssetMoveInvalidRequest
			}
			sourceWorkspaceID = &parsedWorkspaceID
		}
		switch item.Type {
		case AssetMoveTypeAgent:
			agentID, err := uuid.Parse(item.ID)
			if err != nil {
				return nil, ErrAssetMoveInvalidRequest
			}
			moveRequest.MovingAgentIDs = append(moveRequest.MovingAgentIDs, agentID)
			if isWorkflowRuntimeAssetType(scope.resolvedAgentType) {
				moveRequest.ResourceRefs = append(moveRequest.ResourceRefs, agentbindings.ResourceRef{
					OrganizationID: organizationUUID,
					WorkspaceID:    sourceWorkspaceID,
					BindingType:    agentbindings.BindingTypeWorkflow,
					ResourceID:     agentID.String(),
				})
			}
		case AssetMoveTypeDataset:
			moveRequest.ResourceRefs = append(moveRequest.ResourceRefs, agentbindings.ResourceRef{
				OrganizationID: organizationUUID,
				WorkspaceID:    sourceWorkspaceID,
				BindingType:    agentbindings.BindingTypeKnowledgeDataset,
				ResourceID:     item.ID,
			})
		case AssetMoveTypeDatabase:
			moveRequest.ResourceRefs = append(moveRequest.ResourceRefs, agentbindings.ResourceRef{
				OrganizationID: organizationUUID,
				WorkspaceID:    sourceWorkspaceID,
				BindingType:    agentbindings.BindingTypeDatabase,
				ResourceID:     item.ID,
			})
		case AssetMoveTypeFile:
			// Files are not Agent-bindable resources.
		default:
			return nil, ErrAssetMoveInvalidRequest
		}
	}

	response := &dto.WorkspaceAssetMoveDependencyPreviewResponse{}
	if s.agentBindings == nil || (len(moveRequest.ResourceRefs) == 0 && len(moveRequest.MovingAgentIDs) == 0) {
		return response, nil
	}
	agents, err := s.agentBindings.PreviewMoveDependencies(ctx, moveRequest)
	if err != nil {
		return nil, fmt.Errorf("preview workspace move dependencies: %w", err)
	}
	if len(agents) > 0 {
		response.AgentBindingImpact = &dto.WorkspaceAssetMoveDependencyImpact{Agents: agents}
	}
	return response, nil
}
