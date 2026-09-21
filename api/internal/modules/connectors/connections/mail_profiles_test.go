package connections

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestMailProfileLifecycleScopesAndReplay(t *testing.T) {
	for _, id := range []string{"qqmail", "netease163"} {
		for _, usage := range []string{"read", "send", "read_send"} {
			t.Run(id+usage, func(t *testing.T) {
				s, _, _, repo := newKeyLifecycleTestService(t)
				d, _ := catalog.MailDefinition(id)
				s.catalog, _ = catalog.New(d)
				profile, _ := catalog.MailProfileFor(id)
				login := "User+tag@" + profile.Domain
				calls := 0
				s.verifier = mailVerifierFunc(func(_ context.Context, r CandidateVerificationRequest) (CandidateVerificationResult, error) {
					calls++
					if r.ConnectorID != id || r.DriverID != d.DriverID || r.AuthMethodID != d.AuthMethodID || r.MailUsage != usage || r.Credentials["authorization_code"] != "AbcdefghijklMNOP" || !reflect.DeepEqual(r.OperationIDs, catalog.MailOperationsFor(id, usage)) {
						t.Fatal("verification scope")
					}
					v := mailResult(usage)
					v.ExternalAccountID = login
					v.ExternalDisplayName = login
					v.UsableOperationIDs = catalog.MailOperationsFor(id, usage)
					v.Evidence.Mail.Provider = id
					v.Evidence.Mail.AuthenticatedLogin = login
					return v, nil
				})
				values := func() map[string]string {
					return map[string]string{"email": login, "authorization_code": " AbcdefghijklMNOP "}
				}
				cmd := KeySetupCommand{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: id, IdempotencyKey: uuid.NewString(), Values: values(), MailUsage: usage}
				first, err := s.StartKeySetup(context.Background(), cmd)
				if err != nil || first.Connection == nil {
					t.Fatal(err)
				}
				if mailUsage(*first.Connection) != usage || len(first.Connection.Constraint.AllowedOperationIDs) != len(catalog.MailOperationsFor(id, usage)) {
					t.Fatal("local scope")
				}
				cmd.Values = values()
				again, err := s.StartKeySetup(context.Background(), cmd)
				if err != nil || again.Connection.ID != first.Connection.ID || calls != 1 {
					t.Fatal("replay", err)
				}
				cmd.Values = values()
				cmd.MailUsage = "send"
				if usage == "send" {
					cmd.MailUsage = "read"
				}
				if _, err = s.StartKeySetup(context.Background(), cmd); !core.IsErrorCode(err, core.ErrorCodeConflict) || calls != 1 || len(repo.connections) != 1 {
					t.Fatal("usage reused idempotency key")
				}
			})
		}
	}
}
