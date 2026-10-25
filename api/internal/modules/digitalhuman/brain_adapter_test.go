package digitalhuman

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestBrainAdapterBuildsDigitalHumanRunConfigWithoutAgentIdentityOrMemory(t *testing.T) {
	subject := runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()}
	versionID, actorID := uuid.New(), uuid.New()
	now := time.Now().UTC()
	reader := &resolverStoreStub{}
	resolved := &ResolvedExecution{Snapshot: ExecutionSnapshot{
		SchemaVersion: executionSnapshotSchemaV1, Revision: "sha256:snapshot", Subject: subject, DigitalHumanVersion: versionID,
		IdentityName: "Atlas", Persona: "Be careful", Goals: []string{"Find reliable evidence"}, Rules: []string{"Cite evidence"}, Model: "gpt-5",
		Bindings: []resourcebindings.Binding{
			{Type: resourcebindings.BindingTypeSkill, ResourceID: "calculator", AccessMode: resourcebindings.AccessModeExecute, Revision: "sha256:skill"},
			{Type: resourcebindings.BindingTypeKnowledgeDataset, ResourceID: "dataset-1", AccessMode: resourcebindings.AccessModeRead, Revision: "sha256:knowledge"},
			{Type: resourcebindings.BindingTypeIntegrationConnection, ResourceID: "connection-1", ParentResourceID: "feishu", AccessMode: resourcebindings.AccessModeExecute, Revision: "sha256:integration",
				Metadata:      map[string]interface{}{resourcebindings.IntegrationAllowedActionIDsMetadataKey: []interface{}{"message.read"}},
				Authorization: resourcebindings.AuthorizationEvidence{AuthorizedBy: &actorID, AuthorizedAt: &now}},
		},
	}}

	config, err := NewBrainAdapter(reader).RunConfig(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if config.BillingAppType != runtimemodel.ConversationCallerDigitalHuman || config.BillingAppID != subject.ID.String() {
		t.Fatalf("billing identity = %q/%q", config.BillingAppType, config.BillingAppID)
	}
	if config.UseMemory || config.AgentMemoryEnabled || config.AgentMemoryAutoExtractionEnabled || len(config.AgentMemorySlots) != 0 {
		t.Fatalf("memory flags = %#v", config)
	}
	if config.BindingReader == nil || config.BindingSnapshot.Subject != subject || config.BindingSnapshot.VersionID == nil || *config.BindingSnapshot.VersionID != versionID {
		t.Fatalf("binding snapshot = %#v", config.BindingSnapshot)
	}
	if len(config.EnabledSkillIDs) != 1 || config.EnabledSkillIDs[0] != "calculator" || len(config.KnowledgeDatasetIDs) != 1 {
		t.Fatalf("capability projection = %#v", config)
	}
	if config.ConnectorTargetScope != "published" || config.ConnectorTargetVersionID != resolved.Snapshot.DigitalHumanVersion.String() || len(config.BindingAuthorizations) != 3 {
		t.Fatalf("integration projection = %#v", config)
	}
	if strings.Contains(config.SystemPrompt, "agent_id") ||
		!strings.Contains(config.SystemPrompt, "Your name is Atlas.") ||
		!strings.Contains(config.SystemPrompt, "Work goals:\n- Find reliable evidence") ||
		!strings.Contains(config.SystemPrompt, "Rules:\n- Cite evidence") ||
		!strings.Contains(config.SystemPrompt, "Do not use or modify account memory") {
		t.Fatalf("system prompt = %q", config.SystemPrompt)
	}
}

func TestBrainAdapterProjectsSkillBindingsThroughSharedCatalog(t *testing.T) {
	subject := runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()}
	resolved := &ResolvedExecution{Snapshot: ExecutionSnapshot{
		SchemaVersion: executionSnapshotSchemaV1, Revision: "sha256:snapshot", Subject: subject,
		DigitalHumanVersion: uuid.New(), IdentityName: "Atlas", Persona: "Search carefully", Model: "gpt-5",
		Bindings: []resourcebindings.Binding{{
			Type: resourcebindings.BindingTypeSkill, ResourceID: "file-generator",
			AccessMode: resourcebindings.AccessModeExecute, Revision: "sha256:files",
		}},
	}}
	config, err := NewBrainAdapter(&resolverStoreStub{}).RunConfig(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.EnabledSkillIDs) != 1 || config.EnabledSkillIDs[0] != "file-generator" || len(config.BindingAuthorizations) != 1 {
		t.Fatalf("skill binding was not projected through the shared catalog: %#v", config)
	}
}

// A DigitalHuman that binds only tables, plus a workflow binding published
// before the parent was keyed by the workflow Agent id, must pass the same
// tool governance as an Agent, without widening access beyond the bindings.
func TestBrainAdapterBindingEvidencePassesToolGovernance(t *testing.T) {
	subject := runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()}
	binder, laterBinder := uuid.New(), uuid.New()
	boundAt := time.Date(2026, 9, 29, 12, 43, 55, 0, time.UTC)
	laterAt := boundAt.Add(time.Hour)
	evidence := func(actor *uuid.UUID, at *time.Time) resourcebindings.AuthorizationEvidence {
		return resourcebindings.AuthorizationEvidence{AuthorizedBy: actor, AuthorizedAt: at}
	}
	resolved := &ResolvedExecution{Snapshot: ExecutionSnapshot{
		SchemaVersion: executionSnapshotSchemaV1, Revision: "sha256:snapshot", Subject: subject, DigitalHumanVersion: uuid.New(),
		IdentityName: "Atlas", Persona: "Be careful", Model: "gpt-5",
		Bindings: []resourcebindings.Binding{
			{Type: resourcebindings.BindingTypeDatabaseTable, ResourceID: "scores", ParentResourceID: "database-1", AccessMode: resourcebindings.AccessModeRead, Revision: "sha256:t1", Authorization: evidence(&binder, &boundAt)},
			{Type: resourcebindings.BindingTypeDatabaseTable, ResourceID: "orders", ParentResourceID: "database-1", AccessMode: resourcebindings.AccessModeRead, Revision: "sha256:t2", Authorization: evidence(&laterBinder, &laterAt)},
			{Type: resourcebindings.BindingTypeWorkflow, ResourceID: "workflow-agent-1", AccessMode: resourcebindings.AccessModeExecute, Revision: "sha256:w1",
				Metadata:      map[string]interface{}{"workflow_agent_id": "Workflow-Agent-1", "workflow_id": "workflow-1", "version_uuid": "version-1"},
				Authorization: evidence(&binder, &boundAt)},
		},
	}}
	config, err := NewBrainAdapter(&resolverStoreStub{}).RunConfig(resolved)
	if err != nil {
		t.Fatal(err)
	}
	var derived *chatruntime.ResourceBindingAuthorization
	for i, authorization := range config.BindingAuthorizations {
		switch authorization.BindingType {
		case "database":
			derived = &config.BindingAuthorizations[i]
		case "workflow":
			if authorization.ParentResourceID != "Workflow-Agent-1" {
				t.Fatalf("workflow evidence parent = %q, want the workflow Agent id", authorization.ParentResourceID)
			}
		}
	}
	if derived == nil || derived.ResourceID != "database-1" || derived.AccessMode != "read" ||
		derived.BoundByAccountID != laterBinder.String() || derived.BoundAtUnix != laterAt.Unix() {
		t.Fatalf("derived database evidence = %#v", derived)
	}

	params := chatruntime.RuntimeCapabilityConfig{
		BindingAuthorizations: config.BindingAuthorizations,
		Database:              chatruntime.RuntimeDatabaseCapability{Bindings: config.DatabaseBindings},
		Workflow:              chatruntime.RuntimeWorkflowCapability{Bindings: config.WorkflowBindings},
	}.RuntimeParameters(chatruntime.Scope{OrganizationID: subject.OrganizationID}, runtimemodel.ConversationCallerDigitalHuman)
	params["tool_governance"] = map[string]interface{}{"caller_type": skills.SkillCallerDigitalHuman, "permission_tier": "basic", "runtime_surface": "external_page_chat"}

	tests := []struct {
		name, skillID, toolName string
		arguments               map[string]interface{}
		want                    toolgovernance.DecisionStatus
	}{
		{"list databases", skills.SkillAgentDatabase, "list_accessible_databases", map[string]interface{}{}, toolgovernance.DecisionStatusAllowed},
		{"list tables", skills.SkillAgentDatabase, "list_database_tables", map[string]interface{}{"data_source_id": "database-1"}, toolgovernance.DecisionStatusAllowed},
		{"describe bound table", skills.SkillAgentDatabase, "describe_database_table", map[string]interface{}{"data_source_id": "database-1", "table_id": "scores"}, toolgovernance.DecisionStatusAllowed},
		{"query bound table", skills.SkillAgentDatabase, "query_table_records", map[string]interface{}{"data_source_id": "database-1", "table_id": "orders"}, toolgovernance.DecisionStatusAllowed},
		{"query unbound table", skills.SkillAgentDatabase, "query_table_records", map[string]interface{}{"data_source_id": "database-1", "table_id": "salaries"}, toolgovernance.DecisionStatusDenied},
		{"list unbound database", skills.SkillAgentDatabase, "list_database_tables", map[string]interface{}{"data_source_id": "database-2"}, toolgovernance.DecisionStatusDenied},
		{"write read-only table", skills.SkillAgentDatabase, "insert_table_records", map[string]interface{}{"data_source_id": "database-1", "table_id": "orders", "records": []map[string]interface{}{{"id": "1"}}}, toolgovernance.DecisionStatusDenied},
		{"run bound workflow", skills.SkillAgentWorkflow, "run_agent_workflow", map[string]interface{}{"binding_id": "workflow-agent-1", "inputs": map[string]interface{}{"query": "run"}}, toolgovernance.DecisionStatusAllowed},
		{"run unbound workflow", skills.SkillAgentWorkflow, "run_agent_workflow", map[string]interface{}{"binding_id": "workflow-agent-2", "inputs": map[string]interface{}{"query": "run"}}, toolgovernance.DecisionStatusDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, ok := skills.SystemSkillToolGovernanceManifest(tt.skillID, tt.toolName)
			if !ok {
				t.Fatalf("no governance manifest for %s/%s", tt.skillID, tt.toolName)
			}
			decision, err := skills.NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()).DecideSkillTool(context.Background(), skills.ToolGovernanceRequest{
				Manifest: manifest, SkillID: tt.skillID, ToolName: tt.toolName, Arguments: tt.arguments,
				ExecutionContext: skills.ExecutionContext{RuntimeParameters: params},
			})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Status != tt.want {
				t.Fatalf("decision = %s (%#v), want %s", decision.Status, decision.Preauthorization, tt.want)
			}
		})
	}
}
