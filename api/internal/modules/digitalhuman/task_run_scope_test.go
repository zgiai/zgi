package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
)

func TestScheduledRuntimeScopeUsesOwnerSubjectWithoutWorkspaceReauthorization(t *testing.T) {
	ownerID, creatorID, workspaceID := uuid.New(), uuid.New(), uuid.New()
	run := &Run{OrganizationID: uuid.New(), WorkspaceID: workspaceID, DigitalHumanID: uuid.New(), ActorID: &creatorID, UserSubjectKind: "account", UserSubjectID: &ownerID, Surface: SurfaceTask}
	scope := runtimeScope(run)
	if scope.AccountID != ownerID || !scope.SkipAccessCheck {
		t.Fatalf("runtime scope = %#v, want owner subject and scheduler preflight authorization", scope)
	}

	run.UserSubjectKind, run.UserSubjectID = "", nil
	scope = runtimeScope(run)
	if scope.AccountID != creatorID || !scope.SkipAccessCheck {
		t.Fatalf("shared/channel runtime scope = %#v", scope)
	}
}
