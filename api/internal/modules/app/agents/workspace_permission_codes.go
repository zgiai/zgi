package agents

import "github.com/zgiai/zgi/api/internal/modules/workspace/model"

func agentAssetVisiblePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentLogsView,
		model.WorkspacePermissionAgentStatsView,
		model.WorkspacePermissionAgentConversationView,
		model.WorkspacePermissionAgentUpdate,
		model.WorkspacePermissionAgentDelete,
		model.WorkspacePermissionAgentLock,
		model.WorkspacePermissionAgentMove,
		model.WorkspacePermissionAgentCopy,
		model.WorkspacePermissionAgentExport,
		model.WorkspacePermissionAgentPublish,
		model.WorkspacePermissionAgentRuntimeConfigManage,
		model.WorkspacePermissionAgentRuntimeAccessManage,
		model.WorkspacePermissionAgentConversationManage,
	}
}

func agentManageGatePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentUpdate,
		model.WorkspacePermissionAgentDelete,
		model.WorkspacePermissionAgentLock,
		model.WorkspacePermissionAgentMove,
		model.WorkspacePermissionAgentCopy,
		model.WorkspacePermissionAgentExport,
		model.WorkspacePermissionAgentPublish,
		model.WorkspacePermissionAgentRuntimeConfigManage,
		model.WorkspacePermissionAgentRuntimeAccessManage,
		model.WorkspacePermissionAgentConversationManage,
	}
}

func agentUpdatePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentUpdate,
	}
}

func agentDeletePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentDelete,
	}
}

func agentPublishPermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentPublish,
	}
}

func agentRuntimeConfigManagePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentRuntimeConfigManage,
	}
}

func agentRuntimeAccessManagePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentRuntimeAccessManage,
	}
}
