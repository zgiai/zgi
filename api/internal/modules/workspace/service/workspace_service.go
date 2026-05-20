package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WorkspaceServiceImpl implements the WorkspaceService interface
type WorkspaceServiceImpl struct {
	workspaceRepo repository.WorkspaceRepository
	// accountRepo   repository.AccountRepository
	// workspaceRepo    repository.workspaceRepository
	// fileRepo      repository.FileRepository
}

// NewWorkspaceService creates a new workspace service
func NewWorkspaceService(workspaceRepo repository.WorkspaceRepository) *WorkspaceServiceImpl {
	return &WorkspaceServiceImpl{
		workspaceRepo: workspaceRepo,
	}
}

// CreateWorkspace creates a new workspace
func (s *WorkspaceServiceImpl) CreateWorkspace(ctx context.Context, name string, ownerAccountID string) error {
	// Create workspace
	workspace, err := s.workspaceRepo.CreateWorkspace(ctx, name)
	if err != nil {
		return err
	}

	// Create owner relationship
	err = s.workspaceRepo.CreateWorkspaceMember(ctx, workspace.ID, ownerAccountID, string(model.WorkspaceRoleOwner), true)
	if err != nil {
		return err
	}

	// TODO: Send workspace creation event
	logger.InfoContext(ctx, "Workspace created",
		zap.String("workspace_id", workspace.ID),
		zap.String("owner_account_id", ownerAccountID),
	)

	return nil
}

// UpdateWorkspace updates workspace information
func (s *WorkspaceServiceImpl) UpdateWorkspace(ctx context.Context, workspaceID, name string, status *model.WorkspaceStatus, userID string, hasAdminPermission bool) (*model.WorkspaceUpdateResponse, error) {
	workspace, err := s.workspaceRepo.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("workspace not found")
		}
		return nil, err
	}

	if status != nil {
		if *status != model.WorkspaceStatusNormal && *status != model.WorkspaceStatusArchived {
			return nil, errors.New("invalid workspace status")
		}
	}

	// Check permissions
	if !hasAdminPermission {
		join, err := s.workspaceRepo.GetWorkspaceMember(ctx, workspaceID, userID)
		if err != nil {
			return nil, errors.New("no permission to update workspace name")
		}
		if join.Role != model.WorkspaceRoleOwner && join.Role != model.WorkspaceRoleAdmin {
			return nil, errors.New("no permission to update workspace name")
		}
	}

	nameChanged := name != "" && name != workspace.Name
	statusChanged := status != nil && *status != workspace.Status

	if !nameChanged && !statusChanged {
		return &model.WorkspaceUpdateResponse{
			Result: "success",
			Tenant: struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
			}{
				ID:     workspace.ID,
				Name:   workspace.Name,
				Status: string(workspace.Status),
			},
		}, nil
	}

	if nameChanged {
		// Check if workspace name already exists in the same organization.
		organizationID, err := s.workspaceRepo.GetWorkspaceOrganizationID(ctx, workspaceID)
		if err == nil && organizationID != "" {
			exists, err := s.workspaceRepo.CheckWorkspaceNameExists(ctx, organizationID, name, workspaceID)
			if err != nil {
				return nil, err
			}
			if exists {
				return nil, fmt.Errorf("workspace name '%s' already exists in the organization", name)
			}
		}

		err = s.workspaceRepo.UpdateWorkspaceName(ctx, workspaceID, name)
		if err != nil {
			return nil, err
		}
		workspace.Name = name
	}

	if statusChanged {
		err = s.workspaceRepo.UpdateWorkspaceStatus(ctx, workspaceID, *status)
		if err != nil {
			return nil, err
		}
		workspace.Status = *status
	}

	return &model.WorkspaceUpdateResponse{
		Result: "success",
		Tenant: struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		}{
			ID:     workspaceID,
			Name:   workspace.Name,
			Status: string(workspace.Status),
		},
	}, nil
}

// GetWorkspaceStatistics gets workspace statistics including member and entity counts
func (s *WorkspaceServiceImpl) GetWorkspaceStatistics(ctx context.Context, workspaceID string) (*model.WorkspaceInfo, error) {
	workspace, err := s.workspaceRepo.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("workspace not found")
		}
		return nil, err
	}

	// Get statistics
	adminsCount, membersCount, datasetsCount, agentsCount, err := s.workspaceRepo.GetWorkspaceStatistics(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	return &model.WorkspaceInfo{
		ID:             workspace.ID,
		Name:           workspace.Name,
		Plan:           workspace.Plan,
		Status:         string(workspace.Status),
		CreatedAt:      workspace.CreatedAt.Unix(),
		Role:           nil,
		InTrial:        nil,
		TrialEndReason: nil,
		CustomConfig:   nil,
		AdminsCount:    adminsCount,
		MembersCount:   membersCount,
		DatasetsCount:  datasetsCount,
		AgentsCount:    agentsCount,
	}, nil
}
