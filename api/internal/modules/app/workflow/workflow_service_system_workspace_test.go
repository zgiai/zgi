package workflow

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestResolveServiceWebAppRunWorkspaceID_PrefersCurrentWorkspaceForSystemAgent(t *testing.T) {
	agent := &agents.Agent{
		ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID: uuid.Nil,
	}

	workspaceID, err := resolveServiceWebAppRunWorkspaceID(
		context.Background(),
		mockCurrentWorkspaceGetter{
			workspace: &workspace_model.Workspace{ID: "ws-current"},
		},
		"acc-1",
		agent,
	)
	if err != nil {
		t.Fatalf("resolveServiceWebAppRunWorkspaceID returned error: %v", err)
	}
	if workspaceID != "ws-current" {
		t.Fatalf("workspaceID = %q, want %q", workspaceID, "ws-current")
	}
}

func TestEffectiveWorkflowWorkspaceID_UsesRequestedWorkspaceForSystemWorkflow(t *testing.T) {
	workspaceID := effectiveWorkflowWorkspaceID(&Workflow{
		ID:       "33333333-3333-3333-3333-333333333333",
		TenantID: "",
		Internal: true,
	}, "ws-current")

	if workspaceID != "ws-current" {
		t.Fatalf("workspaceID = %q, want %q", workspaceID, "ws-current")
	}
}
