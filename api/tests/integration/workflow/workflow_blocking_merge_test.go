package workflow_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/app/agents"
	workflowpkg "github.com/zgiai/ginext/internal/modules/app/workflow"
)

func TestBlockingPathRootVariableMergeConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, err := initTestDB()
	require.NoError(t, err)

	workflowRepo := workflowpkg.NewWorkflowRepository(db)
	agentsRepo := agents.NewAgentsRepository(db)
	service := workflowpkg.NewWorkflowService(workflowRepo, agentsRepo, nil, nil, nil)

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer setupCancel()

	tenantID := uuid.NewString()
	agentID := uuid.NewString()
	accountID := uuid.NewString()
	createdBy := uuid.NewString()
	versionUUID := uuid.NewString()
	webAppID := uuid.New()

	createdByUUID := uuid.MustParse(createdBy)
	agent := &agents.Agent{
		ID:          uuid.MustParse(agentID),
		TenantID:    uuid.MustParse(tenantID),
		Name:        "wf-merge-test-agent",
		Description: "",
		AgentsType:  "workflow",
		EnableAPI:   true,
		WebAppID:    webAppID,
		CreatedBy:   &createdByUUID,
	}
	require.NoError(t, agentsRepo.Create(setupCtx, agent))

	graphOnly := map[string]any{
		"nodes": buildAssignerWorkflowGraph(
			nil,
			nil,
			[]map[string]any{
				{
					"variable_selector": []any{"environment", "threshold"},
					"input_type":        "constant",
					"operation":         "over-write",
					"value":             2,
				},
			},
		)["nodes"],
		"edges": buildAssignerWorkflowGraph(
			nil,
			nil,
			[]map[string]any{
				{
					"variable_selector": []any{"environment", "threshold"},
					"input_type":        "constant",
					"operation":         "over-write",
					"value":             2,
				},
			},
		)["edges"],
	}
	graphJSON, err := json.Marshal(graphOnly)
	require.NoError(t, err)

	envVarsJSON, err := json.Marshal([]map[string]any{
		{
			"name":       "threshold",
			"value_type": "number",
			"value":      1,
		},
	})
	require.NoError(t, err)

	workflow := &workflowpkg.Workflow{
		ID:                    uuid.NewString(),
		TenantID:              tenantID,
		AppID:                 agentID,
		AgentID:               agentID,
		Type:                  dto.WorkflowTypeWorkflow,
		Version:               "v1",
		Graph:                 string(graphJSON),
		Features:              "{}",
		CreatedBy:             createdBy,
		EnvironmentVariables:  string(envVarsJSON),
		ConversationVariables: "[]",
		Internal:              false,
	}
	require.NoError(t, workflowRepo.Create(setupCtx, workflow))
	require.NoError(t, db.WithContext(setupCtx).
		Model(&workflowpkg.Workflow{}).
		Where("id = ?", workflow.ID).
		Update("version_uuid", versionUUID).Error)

	t.Run("run_published_workflow", func(t *testing.T) {
		runCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		req := &dto.DraftWorkflowRunRequest{
			Inputs: map[string]any{},
		}
		_, runErr := service.RunPublishedWorkflow(runCtx, tenantID, agentID, req, accountID)
		require.NoError(t, runErr)
	})

	t.Run("run_workflow_by_version_uuid", func(t *testing.T) {
		runCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		req := &dto.DraftWorkflowRunRequest{
			Inputs: map[string]any{},
		}
		_, runErr := service.RunWorkflowByVersionUUID(runCtx, versionUUID, req, accountID)
		require.NoError(t, runErr)
	})

	t.Run("run_workflow_by_web_app_id", func(t *testing.T) {
		runCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		req := &dto.DraftWorkflowRunRequest{
			Inputs: map[string]any{},
		}
		_, runErr := service.RunWorkflowByWebAppID(runCtx, webAppID.String(), req, accountID)
		require.NoError(t, runErr)
	})
}
