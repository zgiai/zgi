package providers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type mailDriverFixture struct {
	calls   int
	usage   string
	partial bool
}

func (*mailDriverFixture) ConnectorID() string { return "gmail" }
func (f *mailDriverFixture) VerifyCredentials(_ context.Context, in CredentialInput) (Verification, error) {
	f.calls++
	f.usage = in.MailUsage
	m := &core.MailEvidence{Version: 1, Provider: "gmail", IdentityKind: "authenticated_login", AuthenticatedLogin: in.Values["email"], Usage: in.MailUsage, CredentialVerified: true, IMAPStatus: "not_requested", SMTPStatus: "verified", CheckedAt: time.Now()}
	result := Verification{AccountID: in.Values["email"], Operations: catalog.MailOperations(in.MailUsage), Evidence: &core.ProviderEvidence{Mail: m, ScopeStatus: "verified", GrantedScopes: []string{"forged_scope"}}}
	if f.partial {
		m.IMAPStatus = "verified"
		m.SMTPStatus = "failed"
		m.CredentialVerified = false
		return result, &Failure{Code: core.ErrorCodeMailAuthenticationRejected}
	}
	return result, nil
}
func (*mailDriverFixture) InvokeCredentials(context.Context, string, CredentialInput, map[string]any) (Response, error) {
	return Response{}, nil
}
func TestRuntimeSetsMailPurposeBeforeDriverAndProjectsOnlyProtocolEvidence(t *testing.T) {
	f := &mailDriverFixture{}
	runtime, err := NewRuntime(&driverFixture{id: "github"}, &driverFixture{id: "web-search"}, &appDriverFixture{}, &serverOpsDriverFixture{}, f)
	if err != nil {
		t.Fatal(err)
	}
	r := connections.CandidateVerificationRequest{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "gmail", DriverID: "gmail-mail-v1", AuthMethodID: "gmail.app_password", Credentials: map[string]string{"email": "user@gmail.com", "app_password": "abcdefghijklmnop"}, MailUsage: "send"}
	result, err := runtime.Verify(context.Background(), r)
	if err != nil || f.usage != "send" || len(result.UsableOperationIDs) != 1 || len(result.Evidence.GrantedScopes) != 0 || result.Evidence.Mail.Operations[0].Availability != "conditional" {
		t.Fatalf("mail runtime: %v", err)
	}
	r.OperationIDs = []string{"gmail.message.get"}
	if _, err = runtime.Verify(context.Background(), r); err == nil || f.calls != 1 {
		t.Fatal("forged verification plan reached driver")
	}
	r.OperationIDs = nil
	r.MailUsage = "read_send"
	f.partial = true
	result, err = runtime.Verify(context.Background(), r)
	if err == nil || result.MailVerification == nil || result.MailVerification.IMAPStatus != "verified" || result.Evidence != nil {
		t.Fatal("partial outcome lost or leaked full evidence")
	}
}
