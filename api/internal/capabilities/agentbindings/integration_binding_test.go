package agentbindings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPruneAgentModeResourceRemovesOnlyMatchingIntegrationConnection(t *testing.T) {
	raw := `{"integration_bindings":[{"connection_id":"connection-1","integration_id":"web-search","allowed_action_ids":["web.search"]},{"connection_id":"connection-2","integration_id":"crm","allowed_action_ids":["contacts.read"]}],"home_title":"kept"}`
	updated, changed, err := pruneAgentModeResource(&raw, ResourceRef{
		BindingType:      BindingTypeIntegrationConnection,
		ResourceID:       "connection-1",
		ParentResourceID: "web-search",
	})
	if err != nil || !changed {
		t.Fatalf("pruneAgentModeResource() changed = %v, error = %v", changed, err)
	}
	var mode map[string]interface{}
	if err := json.Unmarshal([]byte(updated), &mode); err != nil {
		t.Fatalf("decode updated mode: %v", err)
	}
	bindings := mode["integration_bindings"].([]interface{})
	if len(bindings) != 1 || bindings[0].(map[string]interface{})["connection_id"] != "connection-2" {
		t.Fatalf("integration_bindings = %#v, want only connection-2", bindings)
	}
	if mode["home_title"] != "kept" {
		t.Fatalf("home_title = %#v, want kept", mode["home_title"])
	}
}

func TestBindingImpactRevisionIncludesIntegrationActionAllowlist(t *testing.T) {
	binding := Binding{
		BindingType:      BindingTypeIntegrationConnection,
		ResourceID:       "connection-1",
		ParentResourceID: "web-search",
		AccessMode:       "read",
		Metadata: map[string]interface{}{
			IntegrationAllowedActionIDsMetadataKey: []string{"web.search"},
		},
	}
	before := bindingImpactRevision([]Binding{binding})
	binding.Metadata[IntegrationAllowedActionIDsMetadataKey] = []string{"web.fetch", "web.search"}
	after := bindingImpactRevision([]Binding{binding})
	if before == after {
		t.Fatal("binding impact revision did not change when action allowlist changed")
	}
}

func TestIntegrationBindingActionAllowlistIsNormalizedAndDenyByDefault(t *testing.T) {
	binding := Binding{
		BindingType: BindingTypeIntegrationConnection,
		Metadata: map[string]interface{}{
			IntegrationAllowedActionIDsMetadataKey: []interface{}{" WEB.SEARCH ", "web.fetch", "web.search", 123},
		},
	}
	got := IntegrationAllowedActionIDs(binding)
	if len(got) != 2 || got[0] != "web.fetch" || got[1] != "web.search" {
		t.Fatalf("IntegrationAllowedActionIDs() = %#v", got)
	}
	if !binding.AllowsIntegrationAction("web.search") {
		t.Fatal("expected web.search to be allowed")
	}
	if binding.AllowsIntegrationAction("web.delete") || (Binding{BindingType: BindingTypeIntegrationConnection}).AllowsIntegrationAction("web.search") {
		t.Fatal("missing actions or metadata must deny")
	}
}

func TestRepositoryAllowsIntegrationActionUsesPersistedMetadata(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	agentID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	now := time.Now()
	columns := []string{
		"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode", "metadata",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE .*agent_id = \$1.*binding_type = \$3.*published_version_uuid IS NULL.*LIMIT \$6`).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			uuid.New(), agentID, ScopeDraft, organizationID, workspaceID, nil,
			BindingTypeIntegrationConnection, "connection-1", "web-search", "", "read", `{"allowed_action_ids":["web.search"]}`,
			uuid.New(), now, now, now,
		))
	repo := NewRepositoryWithTokenSecret(db, "integration-binding-secret")
	allowed, err := repo.AllowsIntegrationAction(context.Background(), ScopeRef{AgentID: agentID, Scope: ScopeDraft}, "connection-1", "web-search", "web.search")
	if err != nil {
		t.Fatalf("AllowsIntegrationAction() error = %v", err)
	}
	if !allowed {
		t.Fatal("AllowsIntegrationAction() = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
