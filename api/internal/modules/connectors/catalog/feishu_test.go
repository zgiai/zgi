package catalog

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestFeishuTemplateV2MatchesReviewedPermissions(t *testing.T) {
	profile := FeishuPermissionTemplate()
	if profile["profile_id"] != "feishu-app-template-v2" || profile["profile_version"] != "2.0.0" {
		t.Error("permission reduction requires a new template version")
	}
	addons := profile["addons"].(map[string]any)
	if addons["preset"] != false {
		t.Fatal("upstream default permissions must not be merged")
	}
	scopes := addons["scopes"].(map[string]any)
	want := strings.Fields(`application:application:self_manage application:bot.menu:write
cardkit:card:write contact:contact.base:readonly docs:document.comment:create
docs:document.comment:delete docs:document.comment:read docs:document.comment:update
docs:document.comment:write_only docx:document.block:convert docx:document:readonly
docx:document:write_only drive:drive.metadata:readonly im:chat.members:bot_access
im:chat:read im:chat:update im:chat:create im:message.group_at_msg.include_bot:readonly
im:message.group_at_msg:readonly im:message.p2p_msg:readonly im:message.pins:read
im:message.pins:write_only im:message.reactions:read im:message.reactions:write_only
im:message:readonly im:message:send_as_bot im:message:send_multi_users
im:message:send_sys_msg im:message:update im:resource application:bot.basic_info:read
wiki:node:read application:app_slash_command:read application:app_slash_command:write`)
	wanted := make([]any, len(want))
	for i, scope := range want {
		wanted[i] = scope
	}
	if !reflect.DeepEqual(scopes["tenant"], wanted) || !reflect.DeepEqual(scopes["user"], []any{"offline_access"}) {
		t.Error("only cardkit:card:read may be removed; no implicit user authorization")
	}
	definition, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := digestJSON(profile)
	if err != nil || definition.Setup.TemplateID != profile["profile_id"] || definition.Setup.TemplateDigest != digest {
		t.Fatal("definition and template differ")
	}
	if definition.PluginVersion != "1.0.2" || len(definition.Operations) != 35 {
		t.Error("version the template change without removing unrelated operations")
	}
	for _, op := range definition.Operations {
		if op.ID == "feishu.card.get" {
			t.Fatal("card reading was removed from scope")
		}
		for _, group := range op.RequiredScopes {
			if slices.Contains(group, "cardkit:card:read") {
				t.Fatal("an operation still requires card reading")
			}
		}
	}
	// The public projection must not mutate the compiled template.
	scopes["tenant"] = []any{"unexpected"}
	if reflect.DeepEqual(profile, FeishuPermissionTemplate()) {
		t.Fatal("template projection is not defensive")
	}
}

func TestFeishuReviewedTemplateReductionCompatibility(t *testing.T) {
	candidate, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	candidate.PluginVersion = "1.0.1"
	for i := range candidate.Operations {
		candidate.Operations[i].SkillID = "external-apps"
	}
	candidate.Setup.TemplateID = "feishu-app-template-v2"
	candidate.Setup.TemplateDigest = "4930f0a57d31ed17ed4d86384fd05219cfce7c5b4f20bbfcadec2329307634cc"
	current := cloneDefinition(candidate)
	current.PluginVersion = "1.0.0"
	current.Setup.TemplateID = "feishu-app-template-v1"
	current.Setup.TemplateDigest = "c651f5dad7b40168c2b4a1fb6f09ec42f2254b758dfbae4242b10f16faac0afc"
	oldDigest, _ := DefinitionContractDigest(current)
	newDigest, _ := DefinitionContractDigest(candidate)
	t.Logf("reviewed contracts: old=%s new=%s", oldDigest, newDigest)
	if err = CheckCompatibleUpdate(current, candidate); err != nil {
		t.Errorf("reviewed template-only upgrade rejected: %v", err)
	}
	for name, change := range map[string]func(*Definition){
		"same version":        func(d *Definition) { d.PluginVersion = current.PluginVersion },
		"different release":   func(d *Definition) { d.PluginVersion = "1.0.2" },
		"unknown template":    func(d *Definition) { d.Setup.TemplateID = "feishu-app-template-v3" },
		"changed permissions": func(d *Definition) { d.Setup.TemplateDigest = strings.Repeat("f", 64) },
		"changed operation":   func(d *Definition) { d.Operations[0].RequiredScopes = [][]string{{"im:chat:read"}} },
		"different plugin":    func(d *Definition) { d.PluginID = "other.feishu" },
	} {
		t.Run(name, func(t *testing.T) {
			mutated := cloneDefinition(candidate)
			change(&mutated)
			if CheckCompatibleUpdate(current, mutated) == nil {
				t.Fatal("unreviewed change accepted")
			}
		})
	}
	if CheckCompatibleUpdate(candidate, current) == nil {
		t.Fatal("permission expansion via downgrade accepted")
	}
	if after, _ := DefinitionContractDigest(current); after != oldDigest {
		t.Fatal("historical accepted contract mutated")
	}
}

func TestFeishuTemplateUpgradeUsesNormalBootstrapAndPreservesDisabledStatus(t *testing.T) {
	updated, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	updated.PluginVersion = "1.0.1"
	for i := range updated.Operations {
		updated.Operations[i].SkillID = "external-apps"
	}
	current := cloneDefinition(updated)
	current.PluginVersion = "1.0.0"
	current.Setup.TemplateID = "feishu-app-template-v1"
	current.Setup.TemplateDigest = "c651f5dad7b40168c2b4a1fb6f09ec42f2254b758dfbae4242b10f16faac0afc"
	// Match the composition root: built-ins are loaded as standard manifests
	// before bootstrap, with immutable package digests assigned by the loader.
	loadRelease := func(d Definition) Definition {
		manifest, err := BuildAgentPluginManifest([]Definition{d})
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := newPluginLoaderForTest(t).LoadManifestPackage(context.Background(), manifest)
		if err != nil || len(loaded.Failures) != 0 || len(loaded.Definitions) != 1 {
			t.Fatalf("load release: %v", err)
		}
		return loaded.Definitions[0]
	}
	current, updated = loadRelease(current), loadRelease(updated)
	for _, status := range []PluginDefinitionStatus{PluginDefinitionStatusActive, PluginDefinitionStatusDisabled} {
		t.Run(string(status), func(t *testing.T) {
			definitions, err := New(updated)
			if err != nil {
				t.Fatal(err)
			}
			store := &memoryPluginDefinitionStore{state: PluginRuntimeState{
				Definitions: []Definition{current},
				Statuses:    map[string]PluginDefinitionStatus{current.PluginID: status},
			}}
			// This regression exercises the frozen 1.0.0 -> 1.0.1 permission
			// release. Today's built-in release has a distinct runtime Skill.
			frozenPolicy := func(candidates []Definition) error {
				if len(candidates) != 1 {
					return ErrInvalidDefinition
				}
				expected, _ := DefinitionContractDigest(updated)
				actual, err := DefinitionContractDigest(candidates[0])
				if err != nil || actual != expected {
					return ErrInvalidDefinition
				}
				return nil
			}
			manager, err := NewPluginManager(newPluginLoaderForTest(t), definitions, store, nil, WithPluginPackageAdmissionPolicy(frozenPolicy))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Bootstrap(context.Background(), nil); err != nil {
				t.Fatal(err)
			}
			active, found := manager.Get(updated.ID)
			if status == PluginDefinitionStatusDisabled {
				if found || store.admitCalls != 0 {
					t.Fatal("template update re-enabled a disabled connector")
				}
				return
			}
			if !found || active.Setup.TemplateID != updated.Setup.TemplateID || active.PluginVersion != "1.0.1" || store.admitCalls != 1 {
				t.Fatal("reviewed update not admitted through the existing lifecycle")
			}
			if !reflect.DeepEqual(current.Operations, active.Operations) || current.Setup.TemplateID != "feishu-app-template-v1" {
				t.Fatal("operation contracts or historical definition changed")
			}
		})
	}
}
