package service

import (
	"reflect"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
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
