package interfaces

import (
	"context"

	"github.com/zgiai/ginext/internal/dto"
)

// WorkflowService defines the interface for workflow-related operations
type WorkflowService interface {
	GetAgentWorkspaceID(ctx context.Context, agentID string) (string, error)
	GetAgentTenantID(ctx context.Context, agentID string) (string, error)
	GetDraftWorkflow(ctx context.Context, agentID string, hideSecrets ...bool) (interface{}, error)
	SyncDraftWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error)
	GenerateDraftWorkflowSuggestedQuestions(ctx context.Context, workspaceID, agentID string, req *dto.GenerateSuggestedQuestionsRequest, accountID string) (*dto.GenerateSuggestedQuestionsResponse, error)
	GetWorkflowConfig(ctx context.Context, workspaceID, agentID string) (interface{}, error)
	RunDraftWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error)
	RunPublishedWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error)
	RunAdvancedChatDraftWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error)
	RunAdvancedChatWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error)
	StopWorkflowTask(ctx context.Context, tenantID, agentID, taskID string, accountID string) error
	GetWorkflowRunByID(ctx context.Context, runID string) (interface{}, error)
	RunDraftWorkflowNode(ctx context.Context, workspaceID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error)
	RunAdvancedChatDraftIterationNode(ctx context.Context, tenantID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error)
	RunDraftIterationNode(ctx context.Context, tenantID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error)
	RunAdvancedChatDraftLoopNode(ctx context.Context, tenantID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error)
	RunDraftLoopNode(ctx context.Context, tenantID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error)
	GetDraftWorkflowNodeLastRun(ctx context.Context, tenantID, agentID, nodeID string, accountID string) (interface{}, error)
	PublishWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error)
	GetPublishedWorkflows(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetLatestPublishedWorkflow(ctx context.Context, requestedWorkspaceID, agentID string, hideSecrets ...bool) (interface{}, error)
	GetWorkflowByID(ctx context.Context, workspaceID, agentID, workflowID string) (interface{}, error)
	DeleteWorkflow(ctx context.Context, tenantID, agentID, workflowID string, accountID string) error
	GetDefaultBlockConfigs(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetDefaultBlockConfig(ctx context.Context, tenantID, agentID, blockType string) (interface{}, error)
	ConvertToWorkflow(ctx context.Context, tenantID, agentID string, req interface{}, accountID string) (interface{}, error)
	GetWorkflowVariables(ctx context.Context, tenantID, agentID string) (interface{}, error)
	DeleteWorkflowVariables(ctx context.Context, tenantID, agentID string, accountID string) error
	GetNodeVariables(ctx context.Context, tenantID, agentID, nodeID string) (interface{}, error)
	DeleteNodeVariables(ctx context.Context, tenantID, agentID, nodeID string, accountID string) error
	GetVariable(ctx context.Context, tenantID, agentID, variableID string) (interface{}, error)
	UpdateVariable(ctx context.Context, tenantID, agentID, variableID string, req interface{}, accountID string) (interface{}, error)
	DeleteVariable(ctx context.Context, tenantID, agentID, variableID string, accountID string) error
	ResetVariable(ctx context.Context, tenantID, agentID, variableID string, accountID string) (interface{}, error)
	GetConversationVariables(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetSystemVariables(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetEnvironmentVariables(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetAdvancedChatWorkflowRuns(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetWorkflowRuns(ctx context.Context, agentID string, req *dto.WorkflowRunsRequest, appWorkspaceID string, accountID string) (*dto.WorkflowRunsResponse, error)
	GetWorkflowRunDetail(ctx context.Context, tenantID, agentID, runID string) (*dto.WorkflowRunDetailResponse, error)
	GetWorkflowRunNodeExecutions(ctx context.Context, tenantID, agentID, runID string) (interface{}, error)
	GetWorkflowDailyRuns(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetWorkflowDailyTerminals(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetWorkflowDailyTokenCost(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetWorkflowAverageAppInteraction(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetWorkflowAppLogs(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetExecutor() interface{}

	// Version-specific workflow execution
	RunWorkflowByVersionUUID(ctx context.Context, versionUUID string, req interface{}, accountID string) (interface{}, error)
	GetWorkflowByVersionUUID(ctx context.Context, versionUUID string) (interface{}, error)

	// Web app workflow execution
	RunWorkflowByWebAppID(ctx context.Context, webAppID string, req interface{}, accountID string) (interface{}, error)

	// Logging methods
	CreateWorkflowRunLog(ctx context.Context, tenantID, agentID, workflowID string, triggeredFrom string, inputs map[string]interface{}, accountID string) (interface{}, error)
	CreateWorkflowRunLogWithVersion(ctx context.Context, tenantID, agentID, workflowID string, triggeredFrom string, versionUUID string, inputs map[string]interface{}, accountID string) (interface{}, error)
	CreateWorkflowRunLogWithWebAppID(ctx context.Context, tenantID, agentID, workflowID string, triggeredFrom string, webAppID string, inputs map[string]interface{}, accountID string) (interface{}, error)
	UpdateWorkflowRunLogStatus(ctx context.Context, workflowRunLogID string, status string, outputs map[string]interface{}, elapsedTime float64, totalTokens int64, totalSteps int, errorMsg string) error
	CreateWorkflowNodeRuntimeLog(ctx context.Context, tenantID, agentID, workflowID, triggeredFrom, workflowRunID, nodeID, nodeType, title string, index int, predecessorNodeID *string, inputs map[string]interface{}, accountID string) (interface{}, error)
	UpdateWorkflowNodeRuntimeLog(ctx context.Context, nodeLogID string, status string, outputs map[string]interface{}, processData map[string]interface{}, executionMetadata map[string]interface{}, elapsedTime float64, errorMsg string) error

	GetLatestWorkflowVersion(ctx context.Context, workspaceID, agentID string) (string, error)
	ManualDiagnoseNode(ctx context.Context, nodeLogID string, model string, lang string) (interface{}, error)
}
