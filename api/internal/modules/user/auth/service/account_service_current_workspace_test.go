package service

import (
	"context"
	"errors"
	"testing"

	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type mockCurrentWorkspaceLookup struct {
	currentJoin      *workspace_model.WorkspaceMember
	currentJoinErr   error
	workspace        *workspace_model.Workspace
	workspaceErr     error
	getWorkspaceCall int
}

func (m *mockCurrentWorkspaceLookup) GetCurrentWorkspace(ctx context.Context, accountID string) (*workspace_model.WorkspaceMember, error) {
	return m.currentJoin, m.currentJoinErr
}

func (m *mockCurrentWorkspaceLookup) GetWorkspaceByID(ctx context.Context, id string) (*workspace_model.Workspace, error) {
	m.getWorkspaceCall++
	return m.workspace, m.workspaceErr
}

func TestGetCurrentWorkspaceStrict_ReturnsWorkspaceFromCurrentJoin(t *testing.T) {
	lookup := &mockCurrentWorkspaceLookup{
		currentJoin: &workspace_model.WorkspaceMember{WorkspaceID: "ws-current"},
		workspace:   &workspace_model.Workspace{ID: "ws-current"},
	}

	workspace, err := getCurrentWorkspaceStrict(context.Background(), lookup, "acc-1")
	if err != nil {
		t.Fatalf("getCurrentWorkspaceStrict returned error: %v", err)
	}
	if workspace == nil || workspace.ID != "ws-current" {
		t.Fatalf("workspace = %#v, want ID %q", workspace, "ws-current")
	}
	if lookup.getWorkspaceCall != 1 {
		t.Fatalf("getWorkspaceCall = %d, want 1", lookup.getWorkspaceCall)
	}
}

func TestGetCurrentWorkspaceStrict_ReturnsNilWhenCurrentJoinMissing(t *testing.T) {
	lookup := &mockCurrentWorkspaceLookup{}

	workspace, err := getCurrentWorkspaceStrict(context.Background(), lookup, "acc-1")
	if err != nil {
		t.Fatalf("getCurrentWorkspaceStrict returned error: %v", err)
	}
	if workspace != nil {
		t.Fatalf("workspace = %#v, want nil", workspace)
	}
	if lookup.getWorkspaceCall != 0 {
		t.Fatalf("getWorkspaceCall = %d, want 0", lookup.getWorkspaceCall)
	}
}

func TestGetCurrentWorkspaceStrict_PropagatesLookupError(t *testing.T) {
	lookup := &mockCurrentWorkspaceLookup{
		currentJoinErr: errors.New("boom"),
	}

	workspace, err := getCurrentWorkspaceStrict(context.Background(), lookup, "acc-1")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v, want boom", err)
	}
	if workspace != nil {
		t.Fatalf("workspace = %#v, want nil", workspace)
	}
}
