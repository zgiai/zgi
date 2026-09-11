package catalog

import (
	"encoding/json"
	"testing"
)

// This baseline protects the reviewed permission template while unrelated
// credential families are added to the same catalog.
func TestFeishuPermissionBaselinePreservesRequiredGrants(t *testing.T) {
	var template struct {
		Addons struct {
			Scopes struct {
				Tenant []string `json:"tenant"`
				User   []string `json:"user"`
			} `json:"scopes"`
		} `json:"addons"`
		UserEventDependencies []struct {
			UserScope string `json:"user_scope"`
		} `json:"user_event_dependencies"`
	}
	if err := json.Unmarshal(feishuPermissionsJSON, &template); err != nil {
		t.Fatal(err)
	}
	tenant := map[string]bool{}
	for _, scope := range template.Addons.Scopes.Tenant {
		if tenant[scope] {
			t.Fatalf("duplicate scope %s", scope)
		}
		tenant[scope] = true
	}
	if len(tenant) != 40 {
		t.Fatalf("reviewed application scope count = %d, want 40", len(tenant))
	}
	for _, scope := range []string{"tenant:tenant:readonly", "im:message.p2p_msg:readonly", "im:message", "im:message:readonly", "im:message.group_msg", "im:chat.members:read", "im:message:send_as_bot"} {
		if !tenant[scope] {
			t.Fatalf("required scope removed: %s", scope)
		}
	}
	if len(template.Addons.Scopes.User) != 1 || template.Addons.Scopes.User[0] != "offline_access" {
		t.Fatal("user grant baseline changed")
	}
	if len(template.UserEventDependencies) != 3 {
		t.Fatal("unimplemented user event dependencies lost")
	}
	definition, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	if len(definition.Operations) != 38 {
		t.Fatalf("reviewed action count = %d, want 38", len(definition.Operations))
	}
	direct := map[string]bool{}
	for _, operation := range definition.Operations {
		for _, alternatives := range operation.RequiredScopes {
			for _, scope := range alternatives {
				if tenant[scope] {
					direct[scope] = true
				}
			}
		}
	}
	if len(direct) != 31 {
		t.Fatalf("directly referenced template scopes = %d, want 31; configuration is not runtime support", len(direct))
	}
}
