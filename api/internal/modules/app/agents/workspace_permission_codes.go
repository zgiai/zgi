package agents

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func agentAssetVisiblePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentCreate,
		model.WorkspacePermissionAgentImport,
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
		model.WorkspacePermissionWorkflowCreate,
		model.WorkspacePermissionWorkflowImport,
		model.WorkspacePermissionWorkflowView,
		model.WorkspacePermissionWorkflowLogsView,
		model.WorkspacePermissionWorkflowStatsView,
		model.WorkspacePermissionWorkflowEventsView,
		model.WorkspacePermissionWorkflowUpdate,
		model.WorkspacePermissionWorkflowDelete,
		model.WorkspacePermissionWorkflowMove,
		model.WorkspacePermissionWorkflowCopy,
		model.WorkspacePermissionWorkflowExport,
		model.WorkspacePermissionWorkflowRunDraft,
		model.WorkspacePermissionWorkflowRunStop,
		model.WorkspacePermissionWorkflowDebug,
		model.WorkspacePermissionWorkflowPublish,
		model.WorkspacePermissionWorkflowRuntimeConfigManage,
		model.WorkspacePermissionWorkflowRuntimeAccessManage,
	}
}

func agentManageGatePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowUpdate,
			model.WorkspacePermissionWorkflowDelete,
			model.WorkspacePermissionWorkflowMove,
			model.WorkspacePermissionWorkflowCopy,
			model.WorkspacePermissionWorkflowExport,
			model.WorkspacePermissionWorkflowRunDraft,
			model.WorkspacePermissionWorkflowRunStop,
			model.WorkspacePermissionWorkflowDebug,
			model.WorkspacePermissionWorkflowPublish,
			model.WorkspacePermissionWorkflowRuntimeConfigManage,
			model.WorkspacePermissionWorkflowRuntimeAccessManage,
		}
	}

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

func agentUpdatePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowUpdate,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentUpdate,
	}
}

func agentDeletePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowDelete,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentDelete,
	}
}

func agentPublishPermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowPublish,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentPublish,
	}
}

func agentRuntimeConfigManagePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowRuntimeConfigManage,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentRuntimeConfigManage,
	}
}

func agentRuntimeAccessManagePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowRuntimeAccessManage,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentRuntimeAccessManage,
	}
}

func isWorkflowRuntimePermissionType(agentType string) bool {
	switch strings.ToUpper(strings.TrimSpace(agentType)) {
	case "WORKFLOW", "CONVERSATIONAL_WORKFLOW", "CONVERSATIONAL_AGENT":
		return true
	default:
		return false
	}
}
