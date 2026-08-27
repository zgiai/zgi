package digitalhuman

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
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
	if config.IntegrationConnectionIDs["feishu"] != "connection-1" || len(config.BindingAuthorizations) != 3 {
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

func TestBrainAdapterLeavesDirectSkillForRequestScopedRuntime(t *testing.T) {
	subject := runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()}
	resolved := &ResolvedExecution{Snapshot: ExecutionSnapshot{
		SchemaVersion: executionSnapshotSchemaV1, Revision: "sha256:snapshot", Subject: subject,
		DigitalHumanVersion: uuid.New(), IdentityName: "Atlas", Persona: "Search carefully", Model: "gpt-5",
		Bindings: []resourcebindings.Binding{{
			Type: resourcebindings.BindingTypeSkill, ResourceID: DirectSkillExaWebSearch,
			AccessMode: resourcebindings.AccessModeExecute, Revision: "sha256:exa",
		}},
	}}
	config, err := NewBrainAdapter(&resolverStoreStub{}).RunConfig(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.EnabledSkillIDs) != 0 || len(config.BindingAuthorizations) != 1 {
		t.Fatalf("direct Skill was projected through shared catalog: %#v", config)
	}
}
