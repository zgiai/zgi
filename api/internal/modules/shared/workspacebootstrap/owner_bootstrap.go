package workspacebootstrap

import (
	"context"
	"fmt"

	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
)

type OwnerWorkspaceBootstrapper interface {
	CreateWorkspaceMember(ctx context.Context, workspaceID string, accountID string, role string) error
	SwitchWorkspace(ctx context.Context, accountID, workspaceID string) error
}

func EnsureOwnerWorkspaceMember(ctx context.Context, bootstrapper OwnerWorkspaceBootstrapper, accountID, workspaceID string) error {
	if err := bootstrapper.CreateWorkspaceMember(ctx, workspaceID, accountID, string(workspace_model.WorkspaceRoleOwner)); err != nil {
		return fmt.Errorf("create workspace owner member: %w", err)
	}

	return nil
}

func EnsureOwnerWorkspaceReady(ctx context.Context, bootstrapper OwnerWorkspaceBootstrapper, accountID, workspaceID string) error {
	if err := EnsureOwnerWorkspaceMember(ctx, bootstrapper, accountID, workspaceID); err != nil {
		return err
	}

	if err := bootstrapper.SwitchWorkspace(ctx, accountID, workspaceID); err != nil {
		return fmt.Errorf("switch current workspace: %w", err)
	}

	return nil
}
