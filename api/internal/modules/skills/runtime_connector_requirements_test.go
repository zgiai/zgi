package skills

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestPromptSkillLoadRequiresCurrentConnectorIdentity(t *testing.T) {
	resolved := &ResolvedSkills{Skills: []SkillDocument{{Metadata: SkillMetadata{ID: "prompt", ConnectorRequirements: []SkillConnectorRequirement{{ConnectorID: "github", OperationIDs: []string{"github.user.get", "github.repository.list"}, Required: true}}}, Instructions: "Summarize available data"}}}
	runtime := &Runtime{}
	if _, _, err := runtime.LoadSkill(context.Background(), resolved, "prompt"); err == nil {
		t.Fatal("missing Connector identity accepted")
	}
	resolver := &connectorIdentityResolverSpy{}
	execution := ExecutionContext{OrganizationID: uuid.NewString(), UserID: uuid.NewString(), ConversationID: uuid.NewString(), MessageID: uuid.NewString(), InvokeFrom: tools.ToolInvokeFromAIChat, ConnectorIdentityResolver: resolver}
	if _, _, err := runtime.LoadSkill(context.Background(), resolved, "prompt", execution); err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 2 {
		t.Fatal("not every required operation was authorized")
	}
	execution.ConnectorIdentityResolver = nil
	if _, _, err := runtime.LoadSkill(context.Background(), resolved, "prompt", execution); err == nil {
		t.Fatal("revoked identity accepted on reload")
	}
	resolved.Skills[0].Metadata.ConnectorRequirements = nil
	if _, _, err := runtime.LoadSkill(context.Background(), resolved, "prompt"); err != nil {
		t.Fatal("ordinary prompt skill requires no Connector", err)
	}
}

func TestSkillParserRejectsRetiredRequirementFields(t *testing.T) {
	for _, metadata := range []string{"integration_requirements: []", "connector_requirements:\n  - integration_id: github\n    required: true", "connector_requirements:\n  - connector_id: github\n    allowed_action_ids: [github.user.get]"} {
		if _, _, err := parseSkillMarkdown([]byte("---\nname: test\ndescription: test\n" + metadata + "\n---\nPrompt")); err == nil {
			t.Fatalf("unsupported requirement accepted: %s", metadata)
		}
	}
}
