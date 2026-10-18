package digitalhuman

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

// bindingInputFromRow and seedHistoricalBinding are shared with
// connector_bindings_postgres_test.go, which currently co-locates them because
// only that file was in scope for the Connector host pick. They encode a
// resource row back into the public DTO and seed a pre-existing authorization;
// keeping one copy avoids a duplicate declaration in this package.

func TestResourceBindingMetadataUsesExactJSONValues(t *testing.T) {
	for _, test := range []struct {
		left, right string
		equal       bool
	}{
		{`{"limit":1.0,"nested":{"value":1e2}}`, `{"nested":{"value":100},"limit":1}`, true},
		{`{"limit":9007199254740992}`, `{"limit":9007199254740993}`, false},
		{`{"operations":["first","second"]}`, `{"operations":["second","first"]}`, false},
	} {
		if got := equivalentResourceMetadata([]byte(test.left), []byte(test.right)); got != test.equal {
			t.Fatalf("metadata comparison = %v for %s, %s", got, test.left, test.right)
		}
	}
}

func TestConnectorCandidatesAllowFullResourceLimit(t *testing.T) {
	for _, test := range []struct {
		kind        resourcebindings.BindingType
		limit, want int
	}{
		{resourcebindings.BindingTypeConnector, 512, 512},
		{resourcebindings.BindingTypeConnector, 1024, 512},
		{resourcebindings.BindingTypeSkill, 512, maxPageSize},
	} {
		query, err := normalizeResourceCandidateQuery(ResourceCandidateQuery{Type: test.kind, Limit: test.limit})
		if err != nil || query.Limit != test.want {
			t.Fatalf("candidate limit for %s: %#v, %v", test.kind, query, err)
		}
	}
}

func TestResourceBindingIdentityLegacyKeepRemoveAndRejectWrites(t *testing.T) {
	service, repo := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "legacy keep")
	row := seedHistoricalBinding(t, repo, scope, detail, resourcebindings.BindingTypeIntegrationConnection)
	input := bindingInputFromRow(t, row)
	kept, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: 1, Items: []ResourceBindingInput{input}})
	if err != nil {
		t.Fatal(err)
	}
	if len(kept.Items) != 1 || kept.Items[0].ID != row.ID || *kept.Items[0].AuthorizedBy != *row.AuthorizedBy || !kept.Items[0].AuthorizedAt.Equal(*row.AuthorizedAt) || kept.Items[0].BindingRevision != row.BindingRevision {
		t.Fatalf("historical row was reauthorized: %#v", kept.Items)
	}
	for _, field := range []string{"display_name", "resource_id", "parent_resource_id", "resource_revision", "credential_ref", "metadata", "id", "new"} {
		t.Run(field, func(t *testing.T) {
			changed := bindingInputFromRow(t, row)
			switch field {
			case "display_name":
				changed.DisplayName = "renamed"
			case "resource_id":
				changed.ResourceID = uuid.NewString()
			case "parent_resource_id":
				changed.ParentResourceID = "other"
			case "resource_revision":
				changed.ResourceRevision = "connector:2"
			case "credential_ref":
				changed.CredentialRef = "other"
			case "metadata":
				changed.Metadata["extra"] = true
			case "id", "new":
				raw, _ := json.Marshal(changed)
				var fields map[string]any
				_ = json.Unmarshal(raw, &fields)
				if field == "new" {
					delete(fields, "id")
				} else {
					fields["id"] = uuid.NewString()
				}
				raw, _ = json.Marshal(fields)
				changed = ResourceBindingInput{}
				_ = json.Unmarshal(raw, &changed)
			}
			_, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: kept.DraftLockVersion, Items: []ResourceBindingInput{changed}})
			if !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
				t.Fatalf("changed legacy %s: %v", field, err)
			}
		})
	}
	removed, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: kept.DraftLockVersion})
	if err != nil || len(removed.Items) != 0 {
		t.Fatalf("remove legacy: %#v, %v", removed, err)
	}
}

func TestResourceBindingIdentityRejectsForeignDuplicateAndResourceSwap(t *testing.T) {
	service, repo := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "binding identity")
	other := createPublishableDigitalHuman(t, service, scope, "other identity")
	row := seedHistoricalBinding(t, repo, scope, detail, resourcebindings.BindingTypeSkill)
	foreign := seedHistoricalBinding(t, repo, scope, other, resourcebindings.BindingTypeSkill)
	for _, test := range []struct {
		name  string
		items []ResourceBindingInput
	}{
		{"foreign", []ResourceBindingInput{bindingInputFromRow(t, foreign)}},
		{"duplicate", []ResourceBindingInput{bindingInputFromRow(t, row), bindingInputFromRow(t, row)}},
		{"swap", func() []ResourceBindingInput {
			input := bindingInputFromRow(t, row)
			input.ResourceID = "other"
			return []ResourceBindingInput{input}
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: 1, Items: test.items})
			if !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
				t.Fatalf("%s: %v", test.name, err)
			}
		})
	}
	if _, err := normalizeResourceCandidateQuery(ResourceCandidateQuery{Type: resourcebindings.BindingTypeIntegrationConnection}); err == nil {
		t.Fatal("legacy candidates still accepted")
	}
}

func TestResourceBindingIdentityUnchangedNativeDoesNotGainAuthority(t *testing.T) {
	service, repo := newTestService(t)
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "missing native")
	row := seedHistoricalBinding(t, repo, scope, detail, resourcebindings.BindingTypeConnector)
	if err := repo.db.Exec(`CREATE TABLE connector_target_bindings (id text, organization_id text, owner_account_id text, target_type text, target_id text, target_scope text, target_version_id text, binding_instance_id text, revision integer, status text)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"missing", "disabled", "active"} {
		t.Run(status, func(t *testing.T) {
			if err := repo.db.Exec("DELETE FROM connector_target_bindings").Error; err != nil {
				t.Fatal(err)
			}
			if status != "missing" {
				if err := repo.db.Exec("INSERT INTO connector_target_bindings VALUES (?, ?, ?, 'digital_human', ?, 'draft', '', ?, 7, ?)", uuid.New(), scope.OrganizationID, row.AuthorizedBy, detail.Definition.ID, row.ID, status).Error; err != nil {
					t.Fatal(err)
				}
			}
			current, err := service.ListDraftResourceBindings(t.Context(), scope, detail.Definition.ID)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.ReplaceDraftResourceBindings(t.Context(), scope, detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: current.DraftLockVersion, Items: []ResourceBindingInput{bindingInputFromRow(t, row), {Type: resourcebindings.BindingTypeSkill, ResourceID: "time", AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "time:1"}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 2 || result.Items[0].ID != row.ID || *result.Items[0].AuthorizedBy != *row.AuthorizedBy {
				t.Fatalf("native retained incorrectly: %#v", result.Items)
			}
			var states []struct {
				Status   string
				Revision int
			}
			if err := repo.db.Table("connector_target_bindings").Find(&states).Error; err != nil {
				t.Fatal(err)
			}
			if status == "missing" && len(states) != 0 || status != "missing" && (len(states) != 1 || states[0].Status != status || states[0].Revision != 7) {
				t.Fatalf("retention altered authority: %#v", states)
			}
		})
	}
}

func TestWorkflowBindingParentFollowsWorkflowAgentID(t *testing.T) {
	scope := testScope()
	agentID := "0782665b-5d7b-4972-9ea6-0f5cc2a3a710"
	input := func(parent string, metadata map[string]any) ResourceBindingInput {
		return ResourceBindingInput{
			Type: resourcebindings.BindingTypeWorkflow, ResourceID: agentID, ParentResourceID: parent,
			AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "sha256:rev", Metadata: metadata,
		}
	}
	metadata := map[string]any{"workflow_agent_id": agentID, "workflow_id": "workflow", "version_uuid": "version"}
	for _, parent := range []string{"", agentID} {
		items, err := buildResourceBindings(scope, uuid.New(), uuid.New(), []ResourceBindingInput{input(parent, metadata)}, nil, time.Now())
		if err != nil || len(items) != 1 || items[0].ParentResourceID != agentID {
			t.Fatalf("parent %q: items = %#v, err = %v", parent, items, err)
		}
	}
	for name, test := range map[string]ResourceBindingInput{
		"mismatched parent": input(uuid.NewString(), metadata),
		"missing agent id":  input("", map[string]any{"workflow_id": "workflow"}),
	} {
		if _, err := buildResourceBindings(scope, uuid.New(), uuid.New(), []ResourceBindingInput{test}, nil, time.Now()); !errors.Is(err, ErrResourceBindingInvalid) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

func TestLegacyWorkflowBindingWithEmptyParentIsSavedAndUpgraded(t *testing.T) {
	scope := testScope()
	digitalHumanID, draftID := uuid.New(), uuid.New()
	agentID := "0782665b-5d7b-4972-9ea6-0f5cc2a3a710"
	metadata := map[string]any{"workflow_agent_id": agentID, "workflow_id": "workflow", "version_uuid": "version"}
	rawMetadata, err := safeJSON(metadata)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().Add(-time.Hour)
	legacy := &ResourceBinding{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		DigitalHumanID: digitalHumanID, DigitalHumanVersionID: draftID,
		BindingType: resourcebindings.BindingTypeWorkflow, ResourceID: agentID, ParentResourceID: "",
		DisplayName: "Approval", AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "sha256:rev",
		Metadata: rawMetadata, CreatedAt: created,
	}
	input := ResourceBindingInput{
		ID: &legacy.ID, Type: resourcebindings.BindingTypeWorkflow, ResourceID: agentID,
		DisplayName: "Approval", AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "sha256:rev", Metadata: metadata,
	}
	items, err := buildResourceBindings(scope, digitalHumanID, draftID, []ResourceBindingInput{input}, []*ResourceBinding{legacy}, time.Now())
	if err != nil {
		t.Fatalf("saving an unchanged legacy workflow binding failed: %v", err)
	}
	if len(items) != 1 || items[0] == legacy || items[0].ID != legacy.ID || !items[0].CreatedAt.Equal(created) || items[0].ParentResourceID != agentID {
		t.Fatalf("items = %#v, want the legacy row rewritten with the workflow Agent parent", items)
	}
}
