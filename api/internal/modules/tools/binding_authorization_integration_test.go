package tools

import "testing"

func TestAgentBindingAuthorizationsPreserveIntegrationActionAllowlist(t *testing.T) {
	authorizations := AgentBindingAuthorizations(map[string]interface{}{
		AgentBindingAuthorizationsParameter: []map[string]interface{}{{
			"binding_type":        "integration_connection",
			"resource_id":         "connection-1",
			"parent_resource_id":  "web-search",
			"access_mode":         "read",
			"allowed_action_ids":  []string{"WEB.SEARCH", "web.fetch", "web.search"},
			"bound_by_account_id": "account-1",
			"bound_at_unix":       123,
		}},
	})
	if len(authorizations) != 1 {
		t.Fatalf("authorizations = %#v", authorizations)
	}
	if !authorizations[0].AllowsAction("web.search") || authorizations[0].AllowsAction("web.delete") {
		t.Fatalf("action allowlist = %#v", authorizations[0].AllowedActionIDs)
	}
}
