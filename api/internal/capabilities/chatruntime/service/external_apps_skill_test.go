package service

import (
	"reflect"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
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
		IntegrationID: "WeCom", ActionID: "WeCom.Message.Send", ConnectionID: "connection-wecom", ToolName: "wecom_send_message",
		Description: "Send a message.", Effect: "external_send", IntentMatched: true, TargetArgumentPaths: []string{"recipient_ref"},
		PreparationActionIDs: []string{"wecom.contact.search"},
		PreparationHints: []integrationmetatools.ActionProjectionPreparationHint{{
			ActionID: "wecom.contact.search", Relation: "resolve_target",
			TargetArguments: []string{"recipient_ref"}, ResultPaths: []string{"members[].recipient_ref"},
		}},
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
		binding.ArgumentEnvelope != "arguments" || binding.Effect != "external_send" || !binding.IntentMatched || binding.PlanPhaseArgument != "plan_phase_id" ||
		!reflect.DeepEqual(binding.TargetArgumentPaths, []string{"recipient_ref"}) ||
		!reflect.DeepEqual(binding.PreparationActionIDs, []string{"wecom.contact.search"}) ||
		binding.ConnectionBinding != skills.NativeExternalActionConnectionBindingHash("connection-wecom") ||
		!reflect.DeepEqual(binding.PreparationHints, []skills.NativeExternalActionPreparationHint{{
			ActionID: "wecom.contact.search", Relation: "resolve_target",
			TargetArguments: []string{"recipient_ref"}, ResultPaths: []string{"members[].recipient_ref"},
		}}) {
		t.Fatalf("projection binding = %#v", binding)
	}
	properties := mapFromOperationContext(projections[0].InputSchema["properties"])
	if _, ok := properties["plan_phase_id"]; !ok {
		t.Fatalf("projection schema = %#v, want plan_phase_id control property", projections[0].InputSchema)
	}
	if original := mapFromOperationContext(valid.InputSchema["properties"]); original["plan_phase_id"] != nil {
		t.Fatalf("source Action schema was mutated: %#v", valid.InputSchema)
	}
	for key, want := range map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send",
		"connection_id":      "connection-wecom",
		"action_schema_hash": "hash-1", "action_schema_revision": "schema-1", "catalog_revision": "catalog-1",
	} {
		if got := binding.FixedArguments[key]; got != want {
			t.Fatalf("fixed argument %s = %#v, want %#v", key, got, want)
		}
	}

	withoutSkill := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := projectExternalActionNativeTools(&withoutSkill, projections, nil, nil); added != 0 || len(withoutSkill.ProviderTools) != 0 {
		t.Fatalf("projection bypassed inactive external-apps skill: added=%d tools=%#v", added, withoutSkill.ProviderTools)
	}
	withSkill := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := projectExternalActionNativeTools(&withSkill, projections, nil, nil); added != 1 || len(withSkill.ProviderTools) != 1 {
		t.Fatalf("active projection = added=%d tools=%#v", added, withSkill.ProviderTools)
	}
}

func TestExternalActionNativePreparationHintsPreserveServerTransform(t *testing.T) {
	got := externalActionNativePreparationHints([]integrationmetatools.ActionProjectionPreparationHint{{
		ActionID: "GitHub.Repository.Search", Relation: "Resolve_Target",
		TargetArguments: []string{"owner", "repo"}, ResultPaths: []string{"repositories[].full_name"},
		ResultTransform: "Split_Slash_Pair",
	}})
	want := []skills.NativeExternalActionPreparationHint{{
		ActionID: "github.repository.search", Relation: "resolve_target",
		TargetArguments: []string{"owner", "repo"}, ResultPaths: []string{"repositories[].full_name"},
		ResultTransform: "split_slash_pair",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native preparation hints = %#v, want %#v", got, want)
	}
}

func TestExternalActionProjectionReservesContextArtifactReaderCaseInsensitively(t *testing.T) {
	reserved := nativeProjectionReservedToolNames(nil)
	found := false
	for _, name := range reserved {
		found = found || strings.EqualFold(name, skillloop.ContextArtifactToolName)
	}
	if !found {
		t.Fatalf("native projection reserved names = %#v, want %q", reserved, skillloop.ContextArtifactToolName)
	}
	projections := externalActionNativeToolProjections([]integrationmetatools.ActionProjection{{
		IntegrationID: "wecom", ActionID: "wecom.context.read", ToolName: "READ_CONTEXT_ARTIFACT",
		Description: "Read external context.", Effect: "read",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false},
		SchemaHash:  "hash-1", SchemaRevision: "schema-1", CatalogRevision: "catalog-1",
	}})
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := projectExternalActionNativeTools(&toolSet, projections, reserved, nil); added != 1 {
		t.Fatalf("projectExternalActionNativeTools() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	if strings.EqualFold(toolSet.ProviderTools[0].Function.Name, skillloop.ContextArtifactToolName) {
		t.Fatalf("external Action shadowed context artifact reader with %q", toolSet.ProviderTools[0].Function.Name)
	}
}
