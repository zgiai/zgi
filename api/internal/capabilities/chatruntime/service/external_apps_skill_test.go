package service

import (
	"reflect"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	integrationmetatools "github.com/zgiai/zgi/api/internal/modules/integrations/metatools"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestAddAIChatExternalAppsSkillIDRequiresSelectedConnection(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{{
		ID: skills.SkillExternalApps, Status: skills.SkillStatusActive,
		SupportedCallers: []string{runtimemodel.ConversationCallerAIChat},
		Exposure:         skills.SystemSkillExposureProfile(skills.SkillExternalApps),
	}}
	if got := addAIChatExternalAppsSkillID([]string{"time"}, catalog, nil); !reflect.DeepEqual(got, []string{"time"}) {
		t.Fatalf("nil config enabled = %#v", got)
	}
	if got := addAIChatExternalAppsSkillID([]string{"time"}, catalog, &RunConfig{}); !reflect.DeepEqual(got, []string{"time"}) {
		t.Fatalf("empty config enabled = %#v", got)
	}
	got := addAIChatExternalAppsSkillID([]string{"time"}, catalog, &RunConfig{
		IntegrationSelectedConnectionIDs: map[string][]string{"github": {"connection-1", "connection-2"}},
	})
	if !reflect.DeepEqual(got, []string{skills.SkillExternalApps, "time"}) {
		t.Fatalf("selected config enabled = %#v", got)
	}
	got = addAIChatExternalAppsSkillID(nil, catalog, &RunConfig{
		IntegrationConnectionIDs: map[string]string{"github": "connection-1"},
	})
	if !reflect.DeepEqual(got, []string{skills.SkillExternalApps}) {
		t.Fatalf("preferred compatibility config enabled = %#v", got)
	}
}

func TestRuntimeParametersPreserveFullIntegrationSelectionSet(t *testing.T) {
	config := RuntimeCapabilityConfig{
		IntegrationConnectionIDs: map[string]string{"GitHub": "CONNECTION-2"},
		IntegrationSelectedConnectionIDs: map[string][]string{
			"GitHub": {"CONNECTION-2", "connection-1", "connection-1"},
		},
	}
	params := config.RuntimeParameters(Scope{}, runtimemodel.ConversationCallerAIChat)
	preferred, ok := params["integration_connection_ids"].(map[string]string)
	if !ok || preferred["github"] != "connection-2" {
		t.Fatalf("preferred runtime connections = %#v", params["integration_connection_ids"])
	}
	selected, ok := params["integration_selected_connection_ids"].(map[string][]string)
	if !ok || !reflect.DeepEqual(selected["github"], []string{"connection-1", "connection-2"}) {
		t.Fatalf("selected runtime connections = %#v", params["integration_selected_connection_ids"])
	}
}

func TestEffectiveAgentSkillIDsAddsExternalAppsOnlyForAuthorizedBinding(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{{
		ID: skills.SkillExternalApps, Status: skills.SkillStatusActive,
		SupportedCallers: []string{runtimemodel.ConversationCallerAIChat, runtimemodel.ConversationCallerAgent},
		Exposure:         skills.SystemSkillExposureProfile(skills.SkillExternalApps),
	}}
	config := &RunConfig{
		BillingAppType:           runtimemodel.ConversationCallerAgent,
		IntegrationConnectionIDs: map[string]string{"github": "connection-1"},
		BindingAuthorizations: []ResourceBindingAuthorization{{
			BindingType: "integration_connection", ResourceID: "connection-1", ParentResourceID: "github",
			AccessMode: "read", AllowedActionIDs: []string{"github.issue.list"},
			BoundByAccountID: "account-1", BoundAtUnix: 123,
		}},
	}
	if got := effectiveAgentSkillIDs(nil, catalog, nil, config); !reflect.DeepEqual(got, []string{skills.SkillExternalApps}) {
		t.Fatalf("authorized Agent skills = %#v", got)
	}

	config.BindingAuthorizations[0].AllowedActionIDs = nil
	if got := effectiveAgentSkillIDs(nil, catalog, nil, config); len(got) != 0 {
		t.Fatalf("Agent external apps enabled without an Action allowlist: %#v", got)
	}

	config.BindingAuthorizations[0].AllowedActionIDs = []string{"github.issue.list"}
	config.BillingAppType = runtimemodel.ConversationCallerAIChat
	if got := effectiveAgentSkillIDs(nil, catalog, nil, config); len(got) != 0 {
		t.Fatalf("Agent external apps enabled for a non-Agent RunConfig: %#v", got)
	}
}

func TestExternalActionNativeProjectionRequiresCompleteFrozenIdentityAndActiveSkill(t *testing.T) {
	valid := integrationmetatools.ActionProjection{
		IntegrationID: "WeCom", ActionID: "WeCom.Message.Send", ToolName: "wecom_send_message",
		Description: "Send a message.",
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]interface{}{"content": map[string]interface{}{"type": "string"}},
		},
		SchemaHash: "hash-1", SchemaRevision: "schema-1", CatalogRevision: "catalog-1",
	}
	incomplete := valid
	incomplete.ToolName = "missing_revision"
	incomplete.CatalogRevision = ""
	projections := externalActionNativeToolProjections([]integrationmetatools.ActionProjection{valid, incomplete})
	if len(projections) != 1 {
		t.Fatalf("externalActionNativeToolProjections() = %#v, want only complete projection", projections)
	}
	binding := projections[0].Binding
	if binding.SkillID != skills.SkillExternalApps || binding.ToolName != integrationmetatools.ToolExecuteAction ||
		binding.ArgumentEnvelope != "arguments" {
		t.Fatalf("projection binding = %#v", binding)
	}
	for key, want := range map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send",
		"action_schema_hash": "hash-1", "action_schema_revision": "schema-1", "catalog_revision": "catalog-1",
	} {
		if got := binding.FixedArguments[key]; got != want {
			t.Fatalf("fixed argument %s = %#v, want %#v", key, got, want)
		}
	}

	withoutSkill := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := projectExternalActionNativeTools(&withoutSkill, projections, nil); added != 0 || len(withoutSkill.ProviderTools) != 0 {
		t.Fatalf("projection bypassed inactive external-apps skill: added=%d tools=%#v", added, withoutSkill.ProviderTools)
	}
	withSkill := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := projectExternalActionNativeTools(&withSkill, projections, nil); added != 1 || len(withSkill.ProviderTools) != 1 {
		t.Fatalf("active projection = added=%d tools=%#v", added, withSkill.ProviderTools)
	}
}
