package catalog

import (
	"github.com/santhosh-tekuri/jsonschema/v6"
	"testing"
)

func TestSocialMCPBuiltinContracts(t *testing.T) {
	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"x-social", "linkedin-social", "facebook-social"} {
		var found *Definition
		for i := range definitions {
			if definitions[i].ID == id {
				found = &definitions[i]
			}
		}
		if found == nil {
			t.Errorf("missing %s native connector", id)
			continue
		}
		metadata, ok := PresentationMetadataFor(id)
		if !ok || metadata.Category != ConnectorCategoryCommunication {
			t.Errorf("missing social presentation %s", id)
		}
		if err := Validate(*found); err != nil {
			t.Fatal(err)
		}
		if !found.Supports(SurfaceDigitalHuman) || found.SetupMethod != SetupMethodKey {
			t.Fatalf("invalid social lifecycle %s", id)
		}
		if err := ValidateBuiltinPackage([]Definition{*found}); err != nil {
			t.Fatal(err)
		}
		operations := map[string]Operation{}
		for _, op := range found.Operations {
			operations[op.ID] = op
		}
		for _, action := range []string{"account.get", "account.list", "post.list", "post.get", "post.create", "comment.list", "comment.reply", "conversation.list", "message.list", "message.send", "metrics.get"} {
			op, ok := operations[id+"."+action]
			if !ok {
				t.Errorf("missing %s.%s", id, action)
				continue
			}
			if op.NameI18n["zh-Hans"] == "" || op.NameI18n["en-US"] == "" {
				t.Error("missing localization")
			}
			if action == "post.create" || action == "comment.reply" || action == "message.send" {
				if op.Idempotent || op.Effect == OperationEffectRead || op.RiskHint != OperationRiskHintHigh {
					t.Errorf("unsafe write contract: %s", op.ID)
				}
			}
		}
	}
}

func TestSocialMCPRejectsGenericDispatchAndUnboundedReads(t *testing.T) {
	for _, definition := range SocialMCPDefinitions() {
		for _, op := range definition.Operations {
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("urn:social", op.InputSchema); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile("urn:social")
			if err != nil {
				t.Fatal(err)
			}
			if schema.Validate(map[string]any{"command": "arbitrary", "url": "https://example.com", "tool": "delete"}) == nil {
				t.Errorf("generic dispatch admitted: %s", op.ID)
			}
			if schema.Validate(map[string]any{"limit": float64(100000)}) == nil {
				t.Errorf("unbounded read admitted: %s", op.ID)
			}
		}
	}
}
