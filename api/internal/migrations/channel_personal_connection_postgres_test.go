package migrations

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	"github.com/zgiai/zgi/api/internal/migrations/digitalhumanpreserve"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

// A PersonalConnection channel resolves a native Connector connection and owns
// no credential envelope. The retirement proof must accept it once retired and
// on a deferred schema1 database, and preservation must leave it alone.
func TestPersonalConnectionChannelPassesRetirementProofPostgres(t *testing.T) {
	t.Run("retired_schema_repeat_verify", func(t *testing.T) {
		db := finalizationTestDatabase(t)
		if err := RunWithDB(db); err != nil {
			t.Fatal(err)
		}
		// The fixture connection is not a fully provisioned Connector, so
		// compare issues with and without the channel instead of requiring none.
		addChannel := seedPersonalConnectionChannel(t, db)
		baseline := verifyIssues(t, db)
		addChannel()
		for range 2 {
			if got := verifyIssues(t, db); !slices.Equal(got, baseline) {
				t.Fatalf("a PersonalConnection channel changed schema2 verification: %v, want %v", got, baseline)
			}
		}
	})
	t.Run("deferred_schema1_plan_and_preserve", func(t *testing.T) {
		t.Setenv(config.ConnectorRetirementModeEnv, "deferred")
		db := finalizationTestDatabase(t)
		ctx := context.Background()
		if err := RunWithDB(db); err != nil {
			t.Fatal(err)
		}
		seedPersonalConnectionChannel(t, db)()
		var head string
		if err := db.Raw("SELECT id FROM public.migrations ORDER BY id DESC LIMIT 1").Scan(&head).Error; err != nil {
			t.Fatal(err)
		}
		plan, err := connectorfinalize.Plan(ctx, db, connectorfinalize.Options{AdvancedCheckpoint: head})
		if err != nil {
			t.Fatal(err)
		}
		for _, issue := range plan.Issues {
			if issue.Table == "channel_installations" || issue.Table == "channel_delivery_outbox" || issue.Table == "channel_processing_markers" {
				t.Fatalf("advanced plan rejected a PersonalConnection channel: %+v", plan.Issues)
			}
		}
		cipher, err := credentialcrypto.NewCredentialKeyring("legacy", map[string]string{"legacy": "0123456789abcdef0123456789abcdef"})
		if err != nil {
			t.Fatal(err)
		}
		preserve, err := digitalhumanpreserve.Plan(ctx, db, cipher)
		if err != nil {
			t.Fatal(err)
		}
		if len(preserve.Items) != 0 || len(preserve.Repairs) != 0 || len(preserve.Issues) != 0 {
			t.Fatalf("preservation treated a PersonalConnection channel as legacy: %+v", preserve)
		}
	})
}

func verifyIssues(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	report, _ := connectorfinalize.Verify(context.Background(), db, connectorfinalize.Options{})
	issues := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		issues = append(issues, issue.Table+":"+issue.Code)
	}
	slices.Sort(issues)
	return issues
}

// seedPersonalConnectionChannel stores a native connection and returns the
// step that adds a PersonalConnection channel installation using it.
func seedPersonalConnectionChannel(t *testing.T, db *gorm.DB) func() {
	t.Helper()
	org, owner, workspace, connection, installation := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	constraint, err := json.Marshal(core.OwnerConstraint(owner, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO public.accounts(id,name,email) VALUES (?, 'Personal channel owner', ?)`, []any{owner, owner.String() + "@example.invalid"}},
		{`INSERT INTO public.organizations(id,name) VALUES (?, 'Personal channel fixture')`, []any{org}},
		{`INSERT INTO public.workspaces(id,name,organization_id) VALUES (?, 'Personal channel workspace', ?)`, []any{workspace, org}},
		{`INSERT INTO public.connector_definitions(connector_id,plugin_id,plugin_version,plugin_digest,provider_id,driver_id,classification,contract_digest) VALUES ('feishu','zgi.feishu','1.0.0','sha256:personal-regression','feishu','feishu-openapi-v1','connector','sha256:personal-regression') ON CONFLICT DO NOTHING`, nil},
		{`INSERT INTO public.connector_connections(id,organization_id,owner_account_id,connector_id,provider_id,driver_id,auth_method_id,name,encrypted_credentials,last_verified_at,usable_operation_ids,connection_constraint) VALUES (?,?,?,'feishu','feishu','feishu-openapi-v1','app_credentials','Personal Feishu','c1.personal-regression',now(),'["message.send"]',?)`, []any{connection, org, owner, string(constraint)}},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return func() {
		t.Helper()
		if err := db.Exec(`INSERT INTO public.channel_installations(id,organization_id,workspace_id,provider,connection_id,name,created_by,updated_by,credential_source,connector_id,connection_owner_id) VALUES (?,?,?,'feishu',?,'Personal Feishu',?,?,'personal_connection','feishu',?)`,
			installation, org, workspace, connection, owner, owner, owner).Error; err != nil {
			t.Fatal(err)
		}
	}
}
