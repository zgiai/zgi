package service

import workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"

func promptWorkspacePermissionCodes() []workspace_model.WorkspacePermissionCode {
	return []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionWorkspaceView,
	}
}

func promptVisiblePermissionCodes() []workspace_model.WorkspacePermissionCode {
	return promptWorkspacePermissionCodes()
}

func promptCreatePermissionCodes() []workspace_model.WorkspacePermissionCode {
	return promptWorkspacePermissionCodes()
}

func promptUpdatePermissionCodes() []workspace_model.WorkspacePermissionCode {
	return promptWorkspacePermissionCodes()
}

func promptVersionManagePermissionCodes() []workspace_model.WorkspacePermissionCode {
	return promptWorkspacePermissionCodes()
}

func promptLabelManagePermissionCodes() []workspace_model.WorkspacePermissionCode {
	return promptWorkspacePermissionCodes()
}

func promptOptimizePermissionCodes() []workspace_model.WorkspacePermissionCode {
	return promptWorkspacePermissionCodes()
}

func promptPlaygroundPermissionCodes() []workspace_model.WorkspacePermissionCode {
	return promptWorkspacePermissionCodes()
}

func promptUsageViewPermissionCodes() []workspace_model.WorkspacePermissionCode {
	return promptWorkspacePermissionCodes()
}
