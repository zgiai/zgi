package digitalhuman

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

func TestExternalRunOwnershipUsesSubjectNotAPIKeyPrincipal(t *testing.T) {
	subjectID := uuid.New()
	run := &Run{
		UserSubjectKind: string(usersubject.APIEndUser),
		UserSubjectID:   &subjectID,
		Surface:         SurfaceAPI,
		PrincipalID:     uuidPointer(uuid.New()),
	}
	scopeAfterKeyRotation := Scope{
		UserSubjectKind: string(usersubject.APIEndUser),
		UserSubjectID:   &subjectID,
		PrincipalID:     uuid.New(),
	}
	if !externalRunOwnedByScope(run, scopeAfterKeyRotation, SurfaceAPI) {
		t.Fatal("the same user subject must retain run access after API key rotation")
	}

	otherSubjectID := uuid.New()
	scopeAfterKeyRotation.UserSubjectID = &otherSubjectID
	if externalRunOwnedByScope(run, scopeAfterKeyRotation, SurfaceAPI) {
		t.Fatal("a different external user subject must not access the run")
	}

	scopeAfterKeyRotation.UserSubjectID = &subjectID
	if externalRunOwnedByScope(run, scopeAfterKeyRotation, SurfaceWebApp) {
		t.Fatal("a different surface must not reuse the run")
	}
}

func TestAnonymousWebAppRunOwnershipUsesSignedSessionSubject(t *testing.T) {
	sessionSubjectID := uuid.New()
	run := &Run{
		PrincipalID: &sessionSubjectID,
		Surface:     SurfaceWebApp,
	}
	scope := Scope{ActorID: sessionSubjectID, PrincipalID: sessionSubjectID}
	if !externalRunOwnedByScope(run, scope, SurfaceWebApp) {
		t.Fatal("the issuing anonymous session must own its webapp run")
	}
	otherSession := uuid.New()
	scope.ActorID, scope.PrincipalID = otherSession, otherSession
	if externalRunOwnedByScope(run, scope, SurfaceWebApp) {
		t.Fatal("a different anonymous session must not own the webapp run")
	}
}

func TestPersistedRunActorIDKeepsOnlyAccountBackedActors(t *testing.T) {
	accountID := uuid.New()
	accountScope := Scope{ActorID: accountID}
	if actorID := persistedRunActorID(accountScope, string(invocation.PrincipalAccount), SurfaceWebApp); actorID == nil || *actorID != accountID {
		t.Fatal("an authenticated WebApp run must retain its account actor")
	}

	apiScope := Scope{ActorID: uuid.New()}
	if actorID := persistedRunActorID(apiScope, string(invocation.PrincipalService), SurfaceAPI); actorID != nil {
		t.Fatal("an API end-user subject must not be persisted in the account actor foreign key")
	}

	anonymousScope := Scope{ActorID: uuid.New()}
	if actorID := persistedRunActorID(anonymousScope, string(invocation.PrincipalExternalUser), SurfaceWebApp); actorID != nil {
		t.Fatal("an anonymous WebApp session must not be persisted in the account actor foreign key")
	}

	digitalHumanID := uuid.New()
	taskScope := Scope{ActorID: digitalHumanID, PrincipalID: digitalHumanID}
	if actorID := persistedRunActorID(taskScope, string(invocation.PrincipalSystem), SurfaceTask); actorID != nil {
		t.Fatal("a system task must not store its digital human id in the account actor foreign key")
	}
	accountTaskScope := Scope{ActorID: accountID, PrincipalID: digitalHumanID, UserSubjectKind: string(usersubject.Account), UserSubjectID: &accountID}
	if actorID := persistedRunActorID(accountTaskScope, string(invocation.PrincipalSystem), SurfaceTask); actorID == nil || *actorID != accountID {
		t.Fatal("an account-owned task must retain its actual account actor")
	}
	apiSubjectID := uuid.New()
	apiTaskScope := Scope{ActorID: apiSubjectID, PrincipalID: apiSubjectID, UserSubjectKind: string(usersubject.APIEndUser), UserSubjectID: &apiSubjectID}
	if actorID := persistedRunActorID(apiTaskScope, string(invocation.PrincipalSystem), SurfaceTask); actorID != nil {
		t.Fatal("an API end-user task must not store its subject id in the account actor foreign key")
	}
}

func TestSystemTaskRunScopeUsesPrincipalWithoutAccountActor(t *testing.T) {
	digitalHumanID := uuid.New()
	run := &Run{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: digitalHumanID,
		PrincipalType: string(invocation.PrincipalSystem), PrincipalID: &digitalHumanID, Surface: SurfaceTask,
	}
	if scope := runScope(run); scope.ActorID != digitalHumanID || scope.PrincipalID != digitalHumanID {
		t.Fatal("system task run must retain a usable non-account principal")
	}
	if scope := runtimeScope(run); scope.AccountID != digitalHumanID || !scope.SkipAccessCheck {
		t.Fatal("system task runtime must retain scoped conversation identity without an account actor")
	}
}

func TestRunScopeFallsBackToExternalPrincipalWithoutPersistingAccountActor(t *testing.T) {
	principalID := uuid.New()
	subjectID := uuid.New()
	run := &Run{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), PrincipalType: string(invocation.PrincipalService),
		PrincipalID: &principalID, UserSubjectKind: string(usersubject.APIEndUser),
		UserSubjectID: &subjectID, Surface: SurfaceAPI,
	}
	scope := runScope(run)
	if scope.ActorID != principalID || scope.PrincipalID != principalID || scope.UserSubjectID == nil || *scope.UserSubjectID != subjectID {
		t.Fatal("external runtime scope must retain principal and user-subject boundaries")
	}
}

func TestRuntimeScopeFallsBackToAnonymousWebAppPrincipal(t *testing.T) {
	principalID := uuid.New()
	workspaceID := uuid.New()
	run := &Run{
		OrganizationID: uuid.New(), WorkspaceID: workspaceID,
		PrincipalType: string(invocation.PrincipalExternalUser), PrincipalID: &principalID,
		Surface: SurfaceWebApp,
	}
	scope := runtimeScope(run)
	if scope.AccountID != principalID || scope.WorkspaceID == nil || *scope.WorkspaceID != workspaceID || !scope.SkipAccessCheck {
		t.Fatal("anonymous WebApp runtime scope must use its signed principal as the conversation owner")
	}
}

func TestExternalRunResponseDoesNotExposeInternalIdentityOrSnapshots(t *testing.T) {
	now := time.Now().UTC()
	run := &Run{
		ID: uuid.New(), PrincipalType: "service", PrincipalID: uuidPointer(uuid.New()),
		UserSubjectKind: "api_end_user", UserSubjectID: uuidPointer(uuid.New()),
		RequestFingerprint: "sha256:private", IdempotencyKey: "private",
		Query: "private message", Status: RunStatusSucceeded, CreatedAt: now, UpdatedAt: now,
	}
	encoded, err := json.Marshal(externalRunResponse(run))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"principal", "subject", "fingerprint", "idempotency", "private message"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("external run response leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestExternalRunTerminalStatuses(t *testing.T) {
	for _, status := range []RunStatus{RunStatusSucceeded, RunStatusFailed, RunStatusCanceled, RunStatusUnknown} {
		if !isTerminalRunStatus(status) {
			t.Fatalf("status %q must be terminal", status)
		}
	}
	for _, status := range []RunStatus{RunStatusQueued, RunStatusRunning, RunStatusCanceling} {
		if isTerminalRunStatus(status) {
			t.Fatalf("status %q must not be terminal", status)
		}
	}
}
