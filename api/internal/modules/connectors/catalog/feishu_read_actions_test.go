package catalog

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func feishu110Fixture(t *testing.T) Definition {
	t.Helper()
	d, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	d.PluginVersion = "1.1.0"
	d.Operations = slices.DeleteFunc(d.Operations, isFeishuReadAction)
	return d
}

func isFeishuReadAction(op Operation) bool {
	return op.ID == "feishu.tenant.get" || op.ID == "feishu.chat.members.list"
}

func TestFeishuReadActionReleasePreservesAcceptedBaseline(t *testing.T) {
	old := feishu110Fixture(t)
	digest, err := DefinitionContractDigest(old)
	if err != nil || digest != "66d7eea4a48346671b7756a11be144fbb5704c60e82eb9557cb26f731a19ff43" || len(old.Operations) != 36 {
		t.Fatalf("accepted 1.1.0 contract drift: %s %v", digest, err)
	}
	operationsDigest, _ := digestJSON(old.Operations)
	if operationsDigest != "406bc1e1f2e7e0e935c6391d03a31b4b8c1e1df090b387bddcdda7b77c67e628" {
		t.Fatalf("accepted operation metadata, including display text, drifted: %s", operationsDigest)
	}
	current, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	if current.PluginVersion != "1.2.0" || len(current.Operations) != 38 {
		t.Fatalf("expected additive 1.2.0 / 38 actions; got %s / %d", current.PluginVersion, len(current.Operations))
	}
	if current.ExtensionSchemaVersion != old.ExtensionSchemaVersion || current.ContractVersion != old.ContractVersion || !reflect.DeepEqual(current.Setup, old.Setup) || !reflect.DeepEqual(current.CredentialFields, old.CredentialFields) {
		t.Fatal("read action release changed credentials or permission template")
	}
	if err := CheckCompatibleUpdate(old, current); err != nil {
		t.Fatalf("additive release rejected: %v", err)
	}
	if reviewedFeishuTemplateExpansion(old, current) {
		t.Fatal("same-template additive update should use ordinary admission")
	}
	assertFeishuTemplateBootstrap(t, old, current)
}

func TestFeishuReadActionContracts(t *testing.T) {
	d, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"feishu.tenant.get", "feishu.chat.members.list"} {
		t.Run(id, func(t *testing.T) {
			index := slices.IndexFunc(d.Operations, func(op Operation) bool { return op.ID == id })
			if index < 0 {
				t.Fatalf("missing action %s", id)
			}
			op := d.Operations[index]
			if op.ToolName != strings.ReplaceAll(id, ".", "_") || op.SkillID != "connector-runtime" || op.CredentialSubject != "app" || op.Effect != OperationEffectRead || !op.Idempotent || !op.DataEgress || op.SensitiveDataAllowed || op.RiskHint != OperationRiskHintLow {
				t.Fatal("action identity or safety contract is incomplete")
			}
			for _, locale := range []string{"en-US", "zh-Hans"} {
				if op.NameI18n[locale] == "" || op.DescriptionI18n[locale] == "" {
					t.Fatalf("missing display locale %s", locale)
				}
			}
			input := compileFeishuTestSchema(t, op.InputSchema)
			output := compileFeishuTestSchema(t, op.OutputSchema)
			var validInput map[string]any
			var validData map[string]any
			if id == "feishu.tenant.get" {
				if !reflect.DeepEqual(op.RequiredScopes, [][]string{{"tenant:tenant:readonly"}}) {
					t.Fatal("tenant scope contract differs")
				}
				validInput = map[string]any{}
				validData = map[string]any{"tenant": map[string]any{"name": "Example", "display_id": "10001", "tenant_key": "tenant-example", "tenant_tag": float64(5)}}
				if input.Validate(map[string]any{"tenant_id": "other"}) == nil {
					t.Fatal("arbitrary tenant selector accepted")
				}
			} else {
				if !reflect.DeepEqual(op.RequiredScopes, [][]string{{"im:chat", "im:chat:readonly", "im:chat.members:read", "im:chat.group_info:readonly"}}) {
					t.Fatal("member scopes must be one OR group")
				}
				validInput = map[string]any{"chat_id": "oc_example", "page_size": float64(100)}
				members := make([]any, 101)
				for i := range members {
					members[i] = map[string]any{"open_id": "ou_example", "name": "Example"}
				}
				validData = map[string]any{"members": members, "has_more": true, "page_token": "next"}
				for _, bad := range []map[string]any{{}, {"chat_id": ""}, {"chat_id": "../other"}, {"chat_id": "oc_example", "page_size": float64(101)}, {"chat_id": "oc_example", "page_size": float64(0)}, {"chat_id": "oc_example", "member_id_type": "user_id"}, {"chat_id": "oc_example", "page_token": strings.Repeat("x", 1025)}} {
					if input.Validate(bad) == nil {
						t.Fatalf("invalid member input accepted: %#v", bad)
					}
				}
				properties := op.InputSchema["properties"].(map[string]any)
				for name, raw := range properties {
					description, _ := raw.(map[string]any)["description"].(string)
					if !strings.Contains(description, " / ") {
						t.Errorf("parameter %s lacks bilingual guidance", name)
					}
				}
			}
			if err := input.Validate(validInput); err != nil {
				t.Fatalf("valid input rejected: %v", err)
			}
			envelope := map[string]any{"provider": "feishu", "operation": id, "data": validData, "truncated": false}
			if err := output.Validate(envelope); err != nil {
				t.Fatalf("complete result rejected: %v", err)
			}
			validData["raw_response"] = map[string]any{"secret": "fixture"}
			if output.Validate(envelope) == nil {
				t.Fatal("unprojected response accepted")
			}
			delete(validData, "raw_response")
			envelope["truncated"] = true
			if output.Validate(envelope) == nil {
				t.Fatal("silent truncation accepted")
			}
		})
	}
}

func compileFeishuTestSchema(t *testing.T, document map[string]any) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("urn:feishu-read-action-test", document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:feishu-read-action-test")
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestFeishuReadActionManifestRoundTrip(t *testing.T) {
	d, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildAgentPluginManifest([]Definition{d})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := newPluginLoaderForTest(t).LoadManifestPackage(context.Background(), manifest)
	if err != nil || len(loaded.Definitions) != 1 || len(loaded.Failures) != 0 {
		t.Fatalf("new release failed normal manifest admission: %v", err)
	}
	if !reflect.DeepEqual(d.Operations, loaded.Definitions[0].Operations) {
		t.Fatal("manifest round trip changed action contract")
	}
}
