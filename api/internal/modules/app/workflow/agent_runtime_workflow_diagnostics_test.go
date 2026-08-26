package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
)

type agentRuntimeWorkflowRunDiagnosticStub struct {
	logs map[string]*WorkflowRunLog
}

func (stub *agentRuntimeWorkflowRunDiagnosticStub) GetByID(_ context.Context, id string) (*WorkflowRunLog, error) {
	return stub.logs[id], nil
}

type agentRuntimeWorkflowNodeDiagnosticStub struct {
	logs map[string][]WorkflowNodeRuntimeLog
}

func (stub *agentRuntimeWorkflowNodeDiagnosticStub) GetByWorkflowRunID(_ context.Context, runID string) ([]WorkflowNodeRuntimeLog, error) {
	return stub.logs[runID], nil
}

func TestAgentRuntimeWorkflowDiagnosticsRestoresDetailedFailureOnlyForAuthorizedLogs(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()
	runID := uuid.New().String()
	rawRunError := "provider rejected model route: upstream request id req-private"
	rawNodeError := "provider quota exhausted for deployment private-deployment"
	rawOutputs := `{"failure_reason":"private provider failure","partial":"diagnostic output"}`
	rawNodeOutputs := `{"provider_error":"private node failure"}`
	createdAt := time.Unix(1_700_000_000, 0)

	handler := &AgentRuntimeLogsHandler{
		workflowRunLogs: &agentRuntimeWorkflowRunDiagnosticStub{logs: map[string]*WorkflowRunLog{
			runID: {
				ID:          runID,
				TenantID:    workspaceID.String(),
				AgentID:     agentID.String(),
				WorkflowID:  uuid.New().String(),
				Status:      dto.WorkflowRunStatusFailed,
				Version:     "published",
				Outputs:     &rawOutputs,
				Error:       &rawRunError,
				ElapsedTime: 1250,
				CreatedAt:   createdAt,
			},
		}},
		workflowNodeRuntimeLogs: &agentRuntimeWorkflowNodeDiagnosticStub{logs: map[string][]WorkflowNodeRuntimeLog{
			runID: {{
				ID:            uuid.New().String(),
				TenantID:      workspaceID.String(),
				AgentID:       agentID.String(),
				NodeID:        "llm-1",
				NodeType:      "llm",
				Title:         "LLM",
				Status:        "failed",
				Outputs:       &rawNodeOutputs,
				Error:         &rawNodeError,
				ElapsedTime:   800,
				CreatedAt:     createdAt,
				CreatedByRole: "account",
			}},
		}},
	}
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Status:         runtimemodel.MessageStatusCompleted,
		Answer:         "工作流运行报错了。",
		Metadata: map[string]interface{}{
			"workflow_runs": []interface{}{map[string]interface{}{
				"workflow_run_id": runID,
				"status":          "failed",
				"error":           "workflow run failed",
				"outputs":         map[string]interface{}{},
				"nodes": []interface{}{map[string]interface{}{
					"node_id": "llm-1",
					"status":  "failed",
					"error":   "workflow run failed",
				}},
			}},
		},
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(2 * time.Second),
	}

	enriched := handler.withAgentRuntimeWorkflowDiagnostics(
		context.Background(),
		message,
		runtimeservice.Scope{WorkspaceID: &workspaceID},
		agentID,
	)
	steps := buildAgentRuntimeSteps(enriched)
	if len(steps) != 2 {
		t.Fatalf("steps len = %d, want workflow run and model answer", len(steps))
	}
	workflowStep := steps[0]
	if workflowStep.Error != rawRunError {
		t.Fatalf("workflow log error = %q, want raw diagnostic %q", workflowStep.Error, rawRunError)
	}
	output := workflowStep.Output.(map[string]interface{})
	if output["failure_reason"] != "private provider failure" {
		t.Fatalf("workflow log output = %#v, want raw failure reason", output)
	}
	nodes := workflowStep.Process["nodes"].([]interface{})
	if len(nodes) != 1 || nodes[0].(map[string]interface{})["error"] != rawNodeError {
		t.Fatalf("workflow node diagnostics = %#v, want raw node error", nodes)
	}

	publicRuns := runtimeSkillInvocations(message.Metadata["workflow_runs"])
	if got := runtimeString(publicRuns[0]["error"]); got != "workflow run failed" {
		t.Fatalf("user-visible metadata was mutated: error = %q", got)
	}
	if strings.Contains(runtimeString(publicRuns[0]["error"]), "provider") {
		t.Fatalf("user-visible metadata contains private diagnostics: %#v", publicRuns[0])
	}
}

func TestAgentRuntimeWorkflowDiagnosticsRejectsRunOutsideAuthorizedScope(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()
	runID := uuid.New().String()
	rawError := "private error from another workspace"
	handler := &AgentRuntimeLogsHandler{
		workflowRunLogs: &agentRuntimeWorkflowRunDiagnosticStub{logs: map[string]*WorkflowRunLog{
			runID: {
				ID:       runID,
				TenantID: uuid.New().String(),
				AgentID:  agentID.String(),
				Error:    &rawError,
			},
		}},
	}
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"workflow_runs": []interface{}{map[string]interface{}{
			"workflow_run_id": runID,
			"status":          "failed",
			"error":           "workflow run failed",
		}},
	}}

	enriched := handler.withAgentRuntimeWorkflowDiagnostics(
		context.Background(),
		message,
		runtimeservice.Scope{WorkspaceID: &workspaceID},
		agentID,
	)
	runs := runtimeSkillInvocations(enriched.Metadata["workflow_runs"])
	if got := runtimeString(runs[0]["error"]); got != "workflow run failed" {
		t.Fatalf("cross-workspace diagnostic error = %q, want generic stored value", got)
	}
}
