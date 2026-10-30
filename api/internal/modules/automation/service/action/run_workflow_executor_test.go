package action

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

type recordingWorkflowRunner struct {
	calls int
}

func (r *recordingWorkflowRunner) RunAutomationWorkflow(_ context.Context, req WorkflowRunRequest) (*WorkflowRunResult, error) {
	r.calls++
	return &WorkflowRunResult{WorkflowRunID: "run-1", Status: "succeeded"}, nil
}

func workflowActionRequest(createdBy string) ActionExecutionRequest {
	return ActionExecutionRequest{
		Task: &automationmodel.AutomationTask{
			ID: "task-1", OrganizationID: "org-1", WorkspaceID: "ws-1", CreatedBy: createdBy,
		},
		TaskRun: &automationmodel.AutomationTaskRun{ID: "task-run-1"},
		Action: &automationmodel.AutomationTaskAction{
			ID:         "action-1",
			ActionType: automationmodel.AutomationActionTypeRunWorkflow,
			Config:     map[string]interface{}{"workflow_ref": map[string]interface{}{"agent_id": "agent-1"}},
		},
	}
}

func TestRunWorkflowRechecksTaskCreatorAccess(t *testing.T) {
	ctx := context.Background()
	var checked []string
	allowed := true
	runner := &recordingWorkflowRunner{}
	executor := NewRunWorkflowExecutor(runner).WithTaskCreatorAccessChecker(
		func(_ context.Context, organizationID, workspaceID, accountID string) (bool, error) {
			checked = append(checked, organizationID+"/"+workspaceID+"/"+accountID)
			return allowed, nil
		})

	_, err := executor.ExecuteAction(ctx, workflowActionRequest("creator-1"))
	require.NoError(t, err)
	require.Equal(t, 1, runner.calls)
	require.Equal(t, []string{"org-1/ws-1/creator-1"}, checked)

	allowed = false
	_, err = executor.ExecuteAction(ctx, workflowActionRequest("creator-1"))
	require.True(t, apperror.IsCode(err, AppCodeTaskCreatorAccessRevoked), "error = %v", err)
	require.Equal(t, 1, runner.calls, "a revoked creator's workflow must not start")

	// System tasks carry no creator account to recheck.
	_, err = executor.ExecuteAction(ctx, workflowActionRequest(""))
	require.NoError(t, err)
	require.Equal(t, 2, runner.calls)
}

func TestTaskCreatorAccessRevokedIsCataloged(t *testing.T) {
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal,
		append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	require.NoError(t, err)
	for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
		presentation, err := productCatalog.Present(AppCodeTaskCreatorAccessRevoked, locale, nil)
		require.NoError(t, err)
		require.Equal(t, 403, presentation.HTTPStatus)
		require.NotEmpty(t, presentation.Message)
	}
}
