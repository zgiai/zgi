package workflow_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/dto"
	workflowpkg "github.com/zgiai/ginext/internal/modules/app/workflow"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/database"
)

func TestWorkflowMinimalLoopIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, err := initTestDB()
	require.NoError(t, err)

	repo := workflowpkg.NewWorkflowRepository(db)
	executor := workflowpkg.NewWorkflowExecutor()

	workflowID := uuid.NewString()
	tenantID := uuid.NewString()
	appID := uuid.NewString()
	agentID := uuid.NewString()
	createdBy := uuid.NewString()

	graphData := buildMinimalLoopGraph()
	graphJSON, err := json.Marshal(graphData)
	require.NoError(t, err)

	workflow := &workflowpkg.Workflow{
		ID:                    workflowID,
		TenantID:              tenantID,
		AppID:                 appID,
		AgentID:               agentID,
		Type:                  dto.WorkflowTypeWorkflow,
		Version:               "draft",
		Graph:                 string(graphJSON),
		Features:              "{}",
		CreatedBy:             createdBy,
		EnvironmentVariables:  "[]",
		ConversationVariables: "[]",
		Internal:              false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = repo.Create(ctx, workflow)
	require.NoError(t, err)

	loaded, err := repo.GetDraftWorkflow(ctx, agentID)
	require.NoError(t, err)
	graphMap := loaded.GetGraphDict()
	require.NotEmpty(t, graphMap)

	result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", graphMap, map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "succeeded", result.Status)

	startStatus, ok := result.NodeResults["start"].(map[string]any)
	require.True(t, ok)
	assertStatusSucceeded(t, startStatus)

	loopStatus, ok := result.NodeResults["loop"].(map[string]any)
	require.True(t, ok)
	assertStatusSucceeded(t, loopStatus)

	endStatus, ok := result.NodeResults["end"].(map[string]any)
	require.True(t, ok)
	assertStatusSucceeded(t, endStatus)
}

func buildMinimalLoopGraph() map[string]any {
	startNodeID := "start"
	loopNodeID := "loop"
	loopStartID := "loop-start"
	loopEndID := "loop-end"
	endNodeID := "end"

	nodes := []map[string]any{
		{
			"id": startNodeID,
			"data": map[string]any{
				"type":      "start",
				"title":     "Start",
				"desc":      "",
				"variables": []any{},
			},
		},
		{
			"id": loopNodeID,
			"data": map[string]any{
				"type":             "loop",
				"title":            "Loop",
				"desc":             "",
				"start_node_id":    loopStartID,
				"loop_count":       1,
				"break_conditions": []any{},
				"logical_operator": "and",
				"loop_variables":   []any{},
				"outputs":          map[string]any{},
			},
		},
		{
			"id":       loopStartID,
			"parentId": loopNodeID,
			"data": map[string]any{
				"type":    "loop-start",
				"loop_id": loopNodeID,
			},
		},
		{
			"id":       loopEndID,
			"parentId": loopNodeID,
			"data": map[string]any{
				"type":    "loop-end",
				"loop_id": loopNodeID,
			},
		},
		{
			"id": endNodeID,
			"data": map[string]any{
				"type":    "end",
				"outputs": []any{},
			},
		},
	}

	edges := []map[string]any{
		{"source": startNodeID, "target": loopNodeID},
		{"source": loopNodeID, "target": endNodeID},
		{"source": loopStartID, "target": loopEndID},
	}

	return map[string]any{
		"nodes": nodes,
		"edges": edges,
	}
}

func initTestDB() (*gorm.DB, error) {
	return database.InitDB(config.Current().Database)
}

func assertStatusSucceeded(t *testing.T, statusMap map[string]any) {
	t.Helper()
	status, ok := statusMap["status"]
	require.True(t, ok)
	if typed, ok := status.(shared.WorkflowNodeExecutionStatus); ok {
		assert.Equal(t, shared.SUCCEEDED, typed)
		return
	}
	if str, ok := status.(string); ok {
		assert.Equal(t, string(shared.SUCCEEDED), str)
		return
	}
	assert.Fail(t, "unexpected status type")
}
