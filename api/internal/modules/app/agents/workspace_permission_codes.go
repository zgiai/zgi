package agents

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func agentAssetVisiblePermissionCodes() []model.WorkspacePermissionCode {
	codes := agentVisiblePermissionCodes()
	return append(codes, workflowVisiblePermissionCodes()...)
}

func agentVisiblePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentView,
		model.WorkspacePermissionAgentCreate,
		model.WorkspacePermissionAgentLogsView,
		model.WorkspacePermissionAgentUpdate,
		model.WorkspacePermissionAgentDelete,
		model.WorkspacePermissionAgentMove,
		model.WorkspacePermissionAgentPublish,
		model.WorkspacePermissionAgentRuntimeAccessManage,
	}
}

func workflowVisiblePermissionCodes() []model.WorkspacePermissionCode {
	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionWorkflowCreate,
		model.WorkspacePermissionWorkflowImport,
		model.WorkspacePermissionWorkflowView,
		model.WorkspacePermissionWorkflowLogsView,
		model.WorkspacePermissionWorkflowUpdate,
		model.WorkspacePermissionWorkflowDelete,
		model.WorkspacePermissionWorkflowMove,
		model.WorkspacePermissionWorkflowRunDraft,
		model.WorkspacePermissionWorkflowPublish,
		model.WorkspacePermissionWorkflowRuntimeAccessManage,
	}
}

func agentManageGatePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowUpdate,
			model.WorkspacePermissionWorkflowDelete,
			model.WorkspacePermissionWorkflowMove,
			model.WorkspacePermissionWorkflowImport,
			model.WorkspacePermissionWorkflowRunDraft,
			model.WorkspacePermissionWorkflowPublish,
			model.WorkspacePermissionWorkflowRuntimeAccessManage,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentUpdate,
		model.WorkspacePermissionAgentDelete,
		model.WorkspacePermissionAgentMove,
		model.WorkspacePermissionAgentPublish,
		model.WorkspacePermissionAgentRuntimeAccessManage,
	}
}

func agentCreatePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowCreate,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentCreate,
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

func agentMovePermissionCodes(agentType string) []model.WorkspacePermissionCode {
	if isWorkflowRuntimePermissionType(agentType) {
		return []model.WorkspacePermissionCode{
			model.WorkspacePermissionWorkflowMove,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentMove,
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
			model.WorkspacePermissionWorkflowUpdate,
		}
	}

	return []model.WorkspacePermissionCode{
		model.WorkspacePermissionAgentUpdate,
	}
}

func agentRuntimeConfigReadPermissionCodes(agentType string) []model.WorkspacePermissionCode {
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
