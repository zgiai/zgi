package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

func TestRuntimeWorkspaceCallerScopeRoundTripsThroughTheRun(t *testing.T) {
	caller := runtimeWorkspaceCaller{RuntimeWorkspaceID: uuid.New(), CredentialID: uuid.New(), SubjectID: uuid.New()}
	access := &PublishedAccess{Definition: &Definition{ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()}}
	scope := runtimeWorkspaceScope(caller, access)
	if scope.Surface != SurfaceAPI || scope.PrincipalID != caller.CredentialID || scope.PrincipalType != string(invocation.PrincipalService) ||
		scope.UserSubjectKind != string(usersubject.APIEndUser) || scope.UserSubjectID == nil || *scope.UserSubjectID != caller.SubjectID ||
		scope.ActorID != caller.SubjectID || scope.RuntimeWorkspaceID == nil || *scope.RuntimeWorkspaceID != caller.RuntimeWorkspaceID {
		t.Fatalf("runtime workspace scope = %#v", scope)
	}

	// Execution rebuilds the caller from the stored run and its event.
	a := executionAccess{surface: SurfaceAPI, user: caller.SubjectID, principal: caller.CredentialID, kind: usersubject.APIEndUser,
		principalType: string(invocation.PrincipalService), runtimeWorkspace: caller.RuntimeWorkspaceID}
	rebuilt := a.scope()
	if rebuilt.RuntimeWorkspaceID == nil || *rebuilt.RuntimeWorkspaceID != caller.RuntimeWorkspaceID || rebuilt.PrincipalID != caller.CredentialID {
		t.Fatalf("rebuilt scope = %#v", rebuilt)
	}
	if plain := (executionAccess{surface: SurfaceAPI}).scope(); plain.RuntimeWorkspaceID != nil {
		t.Fatalf("a DigitalHuman API key run gained a Runtime Workspace: %#v", plain)
	}
}

func TestRuntimeWorkspaceEndUserInstancesAreIssuedByTheRuntimeWorkspace(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCES_ENABLED", "true")
	runtimeWorkspaceID, subjectID := uuid.New(), uuid.New()
	trust := string(SourceTrustExternalEndUser)
	run := &Run{ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: uuid.New(), Surface: SurfaceAPI,
		UserSubjectKind: string(usersubject.APIEndUser), UserSubjectID: &subjectID, SourceTrust: &trust}
	instance := endUserInstanceForRun(Scope{PrincipalID: uuid.New(), RuntimeWorkspaceID: &runtimeWorkspaceID}, run, run.CreatedAt)
	if instance == nil || instance.IssuerType != runtimeWorkspaceInstanceIssuer || instance.IssuerID == nil || *instance.IssuerID != runtimeWorkspaceID {
		t.Fatalf("instance = %#v", instance)
	}
	keyInstance := endUserInstanceForRun(Scope{PrincipalID: uuid.New()}, run, run.CreatedAt)
	if keyInstance == nil || keyInstance.IssuerType != SurfaceAPI {
		t.Fatalf("API key instance = %#v", keyInstance)
	}
}
