package digitalhuman

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

func TestEndUserInstanceForRunCoversOnlyExternalEndUsersWithASubject(t *testing.T) {
	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	keyID, webAppID, subjectID := uuid.New(), uuid.New(), uuid.New()
	run := func(surface string, trust SourceTrust, subject *uuid.UUID) *Run {
		kind := ""
		if subject != nil {
			kind = string(usersubject.APIEndUser)
		}
		return &Run{
			OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: uuid.New(), Surface: surface,
			SourceTrust: sourceTrustPointer(trust), UserSubjectKind: kind, UserSubjectID: subject,
		}
	}
	scope := Scope{PrincipalID: keyID, WebAppID: &webAppID}

	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCES_ENABLED", "false")
	if instance := endUserInstanceForRun(scope, run(SurfaceAPI, SourceTrustExternalEndUser, &subjectID), now); instance != nil {
		t.Fatalf("instance with the flag off = %#v", instance)
	}

	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCES_ENABLED", "true")
	api := endUserInstanceForRun(scope, run(SurfaceAPI, SourceTrustExternalEndUser, &subjectID), now)
	if api == nil || api.SubjectID != subjectID || api.IssuerType != SurfaceAPI || api.IssuerID == nil || *api.IssuerID != keyID ||
		api.Status != EndUserInstanceActive || !api.FirstSeenAt.Equal(now) || !api.LastActiveAt.Equal(now) {
		t.Fatalf("API instance = %#v", api)
	}
	webApp := endUserInstanceForRun(scope, run(SurfaceWebApp, SourceTrustExternalEndUser, &subjectID), now)
	if webApp == nil || webApp.IssuerType != SurfaceWebApp || webApp.IssuerID == nil || *webApp.IssuerID != webAppID {
		t.Fatalf("WebApp instance = %#v", webApp)
	}

	for name, candidate := range map[string]*Run{
		"anonymous WebApp session": run(SurfaceWebApp, SourceTrustExternalEndUser, nil),
		"console member":           run(SurfaceConsole, SourceTrustWorkspaceMember, &subjectID),
		"channel participant":      run(SurfaceChannel, SourceTrustChannelParticipant, &subjectID),
		"scheduled task run":       run(SurfaceTask, SourceTrustExternalEndUser, &subjectID),
	} {
		if instance := endUserInstanceForRun(scope, candidate, now); instance != nil {
			t.Fatalf("%s got an instance: %#v", name, instance)
		}
	}
}
