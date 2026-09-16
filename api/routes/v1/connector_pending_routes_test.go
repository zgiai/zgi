package v1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
)

func (s *onboardingRouteFixture) ListPending(_ context.Context, org, owner uuid.UUID, _ string, _, _ int) (onboarding.PendingList, error) {
	s.gets++
	s.org, s.owner = org, owner
	return onboarding.PendingList{Items: []onboarding.PendingSnapshot{}}, s.err
}
func (s *onboardingRouteFixture) GetPending(_ context.Context, org, owner, id uuid.UUID) (onboarding.PendingSnapshot, error) {
	s.gets++
	s.org, s.owner = org, owner
	return onboarding.PendingSnapshot{Kind: "pending_setup", ID: id, State: onboarding.AwaitingApplication, AppID: "cli_original", AllowedActions: []string{"resume_manual", "cancel"}}, s.err
}
func (s *onboardingRouteFixture) CancelPending(_ context.Context, org, owner, id uuid.UUID, rev int64) (onboarding.PendingSnapshot, error) {
	s.cancels++
	s.org, s.owner = org, owner
	s.revision = rev
	return onboarding.PendingSnapshot{Kind: "pending_setup", ID: id, State: onboarding.PendingCancelled}, s.err
}
func (s *onboardingRouteFixture) ResumePending(_ context.Context, cmd onboarding.ResumeCommand) (onboarding.Snapshot, error) {
	s.starts++
	s.org, s.owner = cmd.OrganizationID, cmd.OwnerAccountID
	s.revision = cmd.ExpectedRevision
	s.resumeCommand = cmd
	return onboarding.Snapshot{ID: uuid.New(), Intent: "resume_app", PendingSetupID: &cmd.PendingSetupID, State: onboarding.Starting}, s.err
}
func (s *onboardingRouteFixture) ResumePendingManual(ctx context.Context, cmd onboarding.ResumeCommand) (onboarding.Snapshot, error) {
	return s.ResumePending(ctx, cmd)
}

func TestConnectorPendingRoutesReadOnlyScopedAndStrict(t *testing.T) {
	s := &onboardingRouteFixture{}
	org, owner, id := uuid.New(), uuid.New(), uuid.New()
	r := onboardingRouteRouter(t, s, org, owner)
	for _, path := range []string{"/console/api/connectors/pending-setups", "/console/api/connectors/pending-setups/" + id.String()} {
		res := connectorRouteRequest(t, r, "GET", path, "", nil)
		if res.Code != 200 || res.Header().Get("Cache-Control") != "no-store" || s.org != org || s.owner != owner || s.starts != 0 {
			t.Fatalf("unsafe pending GET %d %s", res.Code, res.Body)
		}
	}
	path := "/console/api/connectors/pending-setups/" + id.String() + "/onboarding-flows"
	for _, body := range []string{`{"expected_revision":2,"app_id":"cli_other"}`, `{"expected_revision":2,"region":"lark"}`, `{"expected_revision":2,"app_secret":"SENTINEL"}`, `{"expected_revision":0}`, `{"expected_revision":2,"owner_account_id":"other"}`} {
		res := connectorRouteRequest(t, r, "POST", path, body, nil)
		if res.Code != http.StatusBadRequest || s.starts != 0 || strings.Contains(res.Body.String(), "SENTINEL") {
			t.Fatalf("unsafe pending command %d", res.Code)
		}
	}
	res := connectorRouteRequest(t, r, "POST", path, `{"expected_revision":2}`, map[string]string{"Idempotency-Key": "fixture-key"})
	if res.Code != 202 || s.starts != 1 || s.revision != 2 || s.org != org || s.owner != owner {
		t.Fatal("resume actor or CAS lost")
	}
	res = connectorRouteRequest(t, r, "DELETE", "/console/api/connectors/pending-setups/"+id.String(), `{"expected_revision":3}`, nil)
	if res.Code != 200 || s.cancels != 1 || s.revision != 3 {
		t.Fatal("cancel revision lost")
	}
}

func TestConnectorPendingManualReusesCredentialRouteWithoutTarget(t *testing.T) {
	s := &onboardingRouteFixture{}
	org, owner, id := uuid.New(), uuid.New(), uuid.New()
	r := onboardingRouteRouter(t, s, org, owner)
	path := "/console/api/connectors/feishu/app-credential-setups"
	base := `"pending_setup_id":"` + id.String() + `","expected_pending_revision":4,"values":{"app_id":"cli_original","region":"feishu","app_secret":"SENTINEL"}`
	for _, extra := range []string{`,"target":{"type":"chat","id":"` + uuid.NewString() + `"}`, `,"owner_account_id":"other"`, `,"intent":"create_app"`} {
		res := connectorRouteRequest(t, r, "POST", path, `{`+base+extra+`}`, nil)
		if res.Code != http.StatusBadRequest || s.starts != 0 || strings.Contains(res.Body.String(), "SENTINEL") {
			t.Fatal("manual recovery accepted a host target or untrusted routing field")
		}
	}
	res := connectorRouteRequest(t, r, "POST", path, `{`+base+`}`, map[string]string{"Idempotency-Key": "manual-receipt"})
	if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" || s.starts != 1 || s.resumeCommand.PendingSetupID != id || s.resumeCommand.OrganizationID != org || s.resumeCommand.OwnerAccountID != owner || s.resumeCommand.ExpectedRevision != 4 || s.resumeCommand.IdempotencyKey != "manual-receipt" {
		t.Fatalf("manual route did not preserve identity, revision or receipt: %d", res.Code)
	}
	if len(s.resumeCommand.Values) != 0 || strings.Contains(res.Body.String(), "SENTINEL") || !strings.Contains(res.Body.String(), `"intent":"resume_app"`) {
		t.Fatal("manual route retained credentials or returned the wrong contract")
	}
}
