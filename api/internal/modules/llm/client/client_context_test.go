package client

import "testing"

func TestAppContextValidate_RequiresWorkspaceID(t *testing.T) {
	ctx := &AppContext{
		AppID:     "app-1",
		AppType:   "workflow",
		AccountID: "acc-1",
	}

	err := ctx.Validate()
	if err != ErrWorkspaceIDRequired {
		t.Fatalf("Validate err = %v, want %v", err, ErrWorkspaceIDRequired)
	}
}

func TestAppContextValidate_WithWorkspaceID(t *testing.T) {
	ctx := &AppContext{
		AppID:       "app-1",
		AppType:     "workflow",
		AccountID:   "acc-1",
		WorkspaceID: "ws-1",
	}

	if err := ctx.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}
