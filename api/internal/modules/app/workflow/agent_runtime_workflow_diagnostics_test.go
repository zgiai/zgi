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
	parentAgentID := uuid.New()
	workflowAgentID := uuid.New()
	messageID := uuid.New()
	conversationID := uuid.New()
	runID := uuid.New().String()
	workflowID := uuid.New().String()
	invocationID := "invocation-1"
	bindingID := "binding-1"
	rawRunError := "provider rejected model route: upstream request id req-private"
	rawNodeError := "provider quota exhausted for deployment private-deployment"
	rawOutputs := `{"failure_reason":"private provider failure","partial":"diagnostic output"}`
	rawNodeOutputs := `{"provider_error":"private node failure"}`
	createdAt := time.Unix(1_700_000_000, 0)
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Status:         runtimemodel.MessageStatusCompleted,
		Answer:         "工作流运行报错了。",
		Metadata: map[string]interface{}{
			"workflow_runs": []interface{}{map[string]interface{}{
				"workflow_run_id": runID,
				"workflow_id":     workflowID,
				"agent_id":        workflowAgentID.String(),
				"invocation_id":   invocationID,
				"binding_id":      bindingID,
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

	handler := &AgentRuntimeLogsHandler{
		workflowRunLogs: &agentRuntimeWorkflowRunDiagnosticStub{logs: map[string]*WorkflowRunLog{
			runID: {
				ID:                   runID,
				TenantID:             workspaceID.String(),
				AgentID:              workflowAgentID.String(),
				WorkflowID:           workflowID,
				Status:               dto.WorkflowRunStatusFailed,
				Version:              "published",
				Outputs:              &rawOutputs,
				Error:                &rawRunError,
				ElapsedTime:          1250,
				CreatedAt:            createdAt,
				ParentConversationID: optionalStringPointer(conversationID.String()),
				ParentMessageID:      optionalStringPointer(messageID.String()),
				ParentInvocationID:   optionalStringPointer(invocationID),
				InvocationBindingID:  optionalStringPointer(bindingID),
			},
		}},
		workflowNodeRuntimeLogs: &agentRuntimeWorkflowNodeDiagnosticStub{logs: map[string][]WorkflowNodeRuntimeLog{
			runID: {{
				ID:            uuid.New().String(),
				TenantID:      workspaceID.String(),
				AgentID:       workflowAgentID.String(),
				NodeID:        "llm-1",
				NodeType:      "llm",
				Title:         "LLM",
				Status:        "exception",
				Outputs:       &rawNodeOutputs,
				Error:         &rawNodeError,
				ElapsedTime:   800,
				CreatedAt:     createdAt,
				CreatedByRole: "account",
			}},
		}},
	}
	enriched := handler.withAgentRuntimeWorkflowDiagnostics(
		context.Background(),
		message,
		runtimeservice.Scope{WorkspaceID: &workspaceID},
		parentAgentID,
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

func TestAgentRuntimeWorkflowDiagnosticsRejectsMismatchedInvocationLineage(t *testing.T) {
	workspaceID := uuid.New()
	parentAgentID := uuid.New()
	messageID := uuid.New()
	conversationID := uuid.New()
	runID := uuid.New().String()
	workflowAgentID := uuid.New().String()
	workflowID := uuid.New().String()
	rawError := "private workflow diagnostic"

	tests := []struct {
		name            string
		parentMessageID string
		bindingID       string
	}{
		{name: "different parent message", parentMessageID: uuid.New().String(), bindingID: "binding-1"},
		{name: "different invocation binding", parentMessageID: messageID.String(), bindingID: "binding-other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &AgentRuntimeLogsHandler{workflowRunLogs: &agentRuntimeWorkflowRunDiagnosticStub{logs: map[string]*WorkflowRunLog{
				runID: {
					ID:                   runID,
					TenantID:             workspaceID.String(),
					AgentID:              workflowAgentID,
					WorkflowID:           workflowID,
					ParentConversationID: optionalStringPointer(conversationID.String()),
					ParentMessageID:      optionalStringPointer(tt.parentMessageID),
					ParentInvocationID:   optionalStringPointer("invocation-1"),
					InvocationBindingID:  optionalStringPointer(tt.bindingID),
					Error:                &rawError,
				},
			}}}
			message := &runtimemodel.Message{
				ID:             messageID,
				ConversationID: conversationID,
				Metadata: map[string]interface{}{"workflow_runs": []interface{}{map[string]interface{}{
					"workflow_run_id": runID,
					"workflow_id":     workflowID,
					"agent_id":        workflowAgentID,
					"invocation_id":   "invocation-1",
					"binding_id":      "binding-1",
					"error":           "workflow run failed",
				}}},
			}

			enriched := handler.withAgentRuntimeWorkflowDiagnostics(
				t.Context(),
				message,
				runtimeservice.Scope{WorkspaceID: &workspaceID},
				parentAgentID,
			)
			runs := runtimeSkillInvocations(enriched.Metadata["workflow_runs"])
			if got := runtimeString(runs[0]["error"]); got != "workflow run failed" {
				t.Fatalf("mismatched lineage diagnostic error = %q, want generic stored value", got)
			}
		})
	}
}
