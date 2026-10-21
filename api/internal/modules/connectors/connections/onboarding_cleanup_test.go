package connections

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestAppRemovalCleanupLocksAndClearsOnlyOwnedApplicationFlows(t *testing.T) {
	for _, failAt := range []int{-1, 0, 1, 2} {
		t.Run(fmt.Sprintf("failure_at_%d", failAt), func(t *testing.T) {
			repo, mock := openRepositoryMock(t)
			c := readyConnection(uuid.New(), uuid.New(), uuid.New())
			// Legacy connections must never require the new tables.
			if err := cancelAppOnboarding(context.Background(), repo.db, c); err != nil {
				t.Fatal(err)
			}
			c.AuthMethodID = "feishu.app_credentials"
			queries := []string{`SELECT pg_advisory_xact_lock`, `(?s)UPDATE connector_onboarding_flows SET state = 'cancelled'.*revision = revision \+ 1.*lease_owner = NULL.*WHERE organization_id = \$1 AND owner_account_id = \$2 AND connection_id = \$3 AND state IN`, `(?s)DELETE FROM connector_onboarding_secrets WHERE organization_id = \$1 AND owner_account_id = \$2.*WHERE organization_id = \$3 AND owner_account_id = \$4 AND connection_id = \$5`}
			for i, query := range queries {
				e := mock.ExpectExec(query)
				if i == 1 {
					e.WithArgs(c.OrganizationID, c.OwnerAccountID, c.ID)
				} else if i == 2 {
					e.WithArgs(c.OrganizationID, c.OwnerAccountID, c.OrganizationID, c.OwnerAccountID, c.ID)
				}
				if i == failAt {
					e.WillReturnError(errors.New("private database failure"))
					break
				}
				e.WillReturnResult(sqlmock.NewResult(0, 1))
			}
			err := cancelAppOnboarding(context.Background(), repo.db, c)
			if (err != nil) != (failAt >= 0) {
				t.Fatalf("cleanup fail-closed: %v", err)
			}
			assertMock(t, mock)
		})
	}
}

func TestAppEvidenceIsOwnerAndCredentialVersionScoped(t *testing.T) {
	repo, mock := openRepositoryMock(t)
	c := readyConnection(uuid.New(), uuid.New(), uuid.New())
	c.AuthMethodID, c.ConnectorID, c.CredentialVersion = "feishu.app_credentials", "feishu", 7
	mock.ExpectQuery(`SELECT .*connector_connection_provider_evidence.*connection_id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4 AND credential_version = \$5`).
		WithArgs(c.ID, c.OrganizationID, c.OwnerAccountID, "feishu", 7, 1).
		WillReturnRows(sqlmock.NewRows([]string{"evidence"}))
	result, err := loadProviderEvidence(context.Background(), repo.db, c)
	if err != nil || result.ProviderEvidence != nil {
		t.Fatal("missing current-version evidence must not reuse old evidence")
	}
	identity := "feishu:cli_expected"
	c.ExternalAccountID = &identity
	c.ProviderEvidence = &core.ProviderEvidence{Region: "feishu", AppID: "cli_other"}
	if err := writeProviderEvidence(context.Background(), repo.db, c); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatal("different application evidence accepted")
	}
	c.ProviderEvidence.AppID, c.ProviderEvidence.EventsEnabled = "cli_expected", true
	if err := writeProviderEvidence(context.Background(), repo.db, c); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatal("configuration presented as active event consumption")
	}
	assertMock(t, mock)
}
