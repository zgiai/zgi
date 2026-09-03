package tools

import "testing"

func TestAgentBindingAuthorizationsRejectRetiredIntegrationEvidence(t *testing.T) {
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
	if len(authorizations) != 0 {
		t.Fatalf("authorizations = %#v", authorizations)
	}
}
