package workspacebootstrap

import (
	"context"
	"errors"
	"testing"
)

type mockWorkspaceOwnerBootstrapper struct {
	calls        []string
	createErr    error
	switchErr    error
	createInputs struct{ workspaceID, accountID, role string }
	switchInputs struct{ accountID, workspaceID string }
}

func (m *mockWorkspaceOwnerBootstrapper) CreateWorkspaceMember(ctx context.Context, workspaceID, accountID, role string) error {
	m.calls = append(m.calls, "create")
	m.createInputs.workspaceID = workspaceID
	m.createInputs.accountID = accountID
	m.createInputs.role = role
	return m.createErr
}

func (m *mockWorkspaceOwnerBootstrapper) SwitchWorkspace(ctx context.Context, accountID, workspaceID string) error {
	m.calls = append(m.calls, "switch")
	m.switchInputs.accountID = accountID
	m.switchInputs.workspaceID = workspaceID
	return m.switchErr
}

func TestEnsureOwnerWorkspaceReady_CreatesOwnerJoinThenSwitchesWorkspace(t *testing.T) {
	mock := &mockWorkspaceOwnerBootstrapper{}

	err := EnsureOwnerWorkspaceReady(context.Background(), mock, "acc-1", "ws-1")
	if err != nil {
		t.Fatalf("EnsureOwnerWorkspaceReady returned error: %v", err)
	}

	if got, want := len(mock.calls), 2; got != want {
		t.Fatalf("len(calls) = %d, want %d", got, want)
	}
	if mock.calls[0] != "create" || mock.calls[1] != "switch" {
		t.Fatalf("calls = %v, want [create switch]", mock.calls)
	}
	if mock.createInputs.workspaceID != "ws-1" || mock.createInputs.accountID != "acc-1" || mock.createInputs.role != "owner" {
		t.Fatalf("createInputs = %#v, want workspace ws-1, account acc-1, role owner", mock.createInputs)
	}
	if mock.switchInputs.accountID != "acc-1" || mock.switchInputs.workspaceID != "ws-1" {
		t.Fatalf("switchInputs = %#v, want account acc-1, workspace ws-1", mock.switchInputs)
	}
}

func TestEnsureOwnerWorkspaceMember_CreatesOwnerJoinWithoutSwitchingWorkspace(t *testing.T) {
	mock := &mockWorkspaceOwnerBootstrapper{}

	err := EnsureOwnerWorkspaceMember(context.Background(), mock, "acc-1", "ws-1")
	if err != nil {
		t.Fatalf("EnsureOwnerWorkspaceMember returned error: %v", err)
	}

	if got, want := len(mock.calls), 1; got != want {
		t.Fatalf("len(calls) = %d, want %d", got, want)
	}
	if mock.calls[0] != "create" {
		t.Fatalf("calls = %v, want [create]", mock.calls)
	}
	if mock.createInputs.workspaceID != "ws-1" || mock.createInputs.accountID != "acc-1" || mock.createInputs.role != "owner" {
		t.Fatalf("createInputs = %#v, want workspace ws-1, account acc-1, role owner", mock.createInputs)
	}
}

func TestEnsureOwnerWorkspaceReady_StopsWhenCreateWorkspaceMemberFails(t *testing.T) {
	mock := &mockWorkspaceOwnerBootstrapper{createErr: errors.New("create failed")}

	err := EnsureOwnerWorkspaceReady(context.Background(), mock, "acc-1", "ws-1")
	if err == nil || err.Error() != "create workspace owner member: create failed" {
		t.Fatalf("err = %v, want create workspace owner member: create failed", err)
	}
	if got, want := len(mock.calls), 1; got != want {
		t.Fatalf("len(calls) = %d, want %d", got, want)
	}
}

func TestEnsureOwnerWorkspaceReady_PropagatesSwitchWorkspaceError(t *testing.T) {
	mock := &mockWorkspaceOwnerBootstrapper{switchErr: errors.New("switch failed")}

	err := EnsureOwnerWorkspaceReady(context.Background(), mock, "acc-1", "ws-1")
	if err == nil || err.Error() != "switch current workspace: switch failed" {
		t.Fatalf("err = %v, want switch current workspace: switch failed", err)
	}
	if got, want := len(mock.calls), 2; got != want {
		t.Fatalf("len(calls) = %d, want %d", got, want)
	}
}

func TestEnsureOwnerWorkspaceMember_StopsWhenCreateWorkspaceMemberFails(t *testing.T) {
	mock := &mockWorkspaceOwnerBootstrapper{createErr: errors.New("create failed")}

	err := EnsureOwnerWorkspaceMember(context.Background(), mock, "acc-1", "ws-1")
	if err == nil || err.Error() != "create workspace owner member: create failed" {
		t.Fatalf("err = %v, want create workspace owner member: create failed", err)
	}
	if got, want := len(mock.calls), 1; got != want {
		t.Fatalf("len(calls) = %d, want %d", got, want)
	}
}
