package connections

import (
	"context"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
	"time"
)

func TestManagedSocialAccountEvidencePersistsOnlyForExactCredentialOwner(t *testing.T) {
	repo, mock := openRepositoryMock(t)
	c := readyConnection(uuid.New(), uuid.New(), uuid.New())
	for _, d := range catalog.SocialMCPDefinitions() {
		if d.ID == "linkedin-social" {
			c.ConnectorID = core.Identifier(d.ID)
			c.ProviderID = core.Identifier(d.ProviderID)
			c.DriverID = core.Identifier(d.DriverID)
			c.AuthMethodID = core.Identifier(d.AuthMethodID)
		}
	}
	actor := "42"
	c.ExternalAccountID = &actor
	c.CredentialVersion = 3
	c.ProviderEvidence = &core.ProviderEvidence{Social: &core.SocialEvidence{Version: 1, Provider: "linkedin-social", Identity: actor, AccountIDs: []string{"urn:li:organization:123"}, CheckedAt: time.Now()}}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO.*connector_connection_provider_evidence").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := writeProviderEvidence(context.Background(), repo.db, c); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(c.ProviderEvidence)
	mock.ExpectQuery(`SELECT .*connector_connection_provider_evidence.*connection_id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4 AND credential_version = \$5`).WithArgs(c.ID, c.OrganizationID, c.OwnerAccountID, "linkedin-social", 3, 1).WillReturnRows(sqlmock.NewRows([]string{"evidence"}).AddRow(data))
	loaded, err := loadProviderEvidence(context.Background(), repo.db, c)
	if err != nil || loaded.ProviderEvidence == nil || loaded.ProviderEvidence.Social.AccountIDs[0] != "urn:li:organization:123" {
		t.Fatal(loaded.ProviderEvidence, err)
	}
	c.ProviderEvidence.Social.AccountIDs = []string{"urn:li:person:123"}
	if err := writeProviderEvidence(context.Background(), repo.db, c); err == nil {
		t.Fatal("person persisted as company")
	}
	assertMock(t, mock)
}
