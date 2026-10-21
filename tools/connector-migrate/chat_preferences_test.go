package main

import "testing"

func chatPreferenceFixture(t *testing.T) databaseSnapshot {
	t.Helper()
	source := fixture(t)
	source["agent_resource_bindings"] = nil
	source["agents_configs"] = nil
	return source
}

func addPreferenceConnection(t *testing.T, source databaseSnapshot, id string) {
	t.Helper()
	connection := clone(source["integration_connections"][0])
	connection["id"] = id
	connection["encrypted_credentials"] = oldEnvelope(t, "v2", secretSubject{orgID, id, "github", "connection-key", 1}, []byte(`{"token":"`+secretMarker+`"}`))
	source["integration_connections"] = append(source["integration_connections"], connection)
}

func TestChatPreferencesPreserveExactWorkspaceScope(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "global_first", true: "global_last"}[reverse], func(t *testing.T) {
			source := chatPreferenceFixture(t)
			workspaceConnection := deterministicID("test", "workspace-connection")
			otherConnection := deterministicID("test", "other-connection")
			otherWorkspace := deterministicID("test", "other-workspace")
			addPreferenceConnection(t, source, workspaceConnection)
			addPreferenceConnection(t, source, otherConnection)
			source["aichat_integration_preferences"] = []row{
				{"id": deterministicID("test", "global-preference"), "organization_id": orgID, "account_id": ownerID, "workspace_id": nil, "integration_id": "github", "preferred_connection_id": connID},
				{"id": deterministicID("test", "workspace-preference"), "organization_id": orgID, "account_id": ownerID, "workspace_id": workspaceID, "integration_id": "github", "preferred_connection_id": workspaceConnection},
				{"id": deterministicID("test", "other-preference"), "organization_id": orgID, "account_id": ownerID, "workspace_id": otherWorkspace, "integration_id": "github", "preferred_connection_id": otherConnection},
			}
			if reverse {
				preferences := source["aichat_integration_preferences"]
				preferences[0], preferences[2] = preferences[2], preferences[0]
			}
			want := map[string]string{}
			for _, tc := range []struct {
				name       string
				workspace  any
				connection string
			}{
				{"unscoped", nil, connID},
				{"empty_scope", "", connID},
				{"workspace", workspaceID, workspaceConnection},
				{"other_workspace", otherWorkspace, otherConnection},
				{"no_preference", deterministicID("test", "unselected-workspace"), ""},
			} {
				id := deterministicID("chat", tc.name)
				source["chat_runtime_conversations"] = append(source["chat_runtime_conversations"], row{"id": id, "organization_id": orgID, "account_id": ownerID, "caller_type": "aichat", "workspace_id": tc.workspace})
				if tc.connection != "" {
					want[id] = tc.connection
				}
			}
			for _, override := range []row{
				{"organization_id": deterministicID("test", "other-org")},
				{"account_id": deterministicID("test", "other-owner")},
				{"deleted_at": "2026-09-01T00:00:00Z"},
				{"caller_type": "agent"},
			} {
				chat := row{"id": deterministicID("excluded-chat", hash(override)), "organization_id": orgID, "account_id": ownerID, "caller_type": "aichat", "workspace_id": workspaceID}
				for key, value := range override {
					chat[key] = value
				}
				source["chat_runtime_conversations"] = append(source["chat_runtime_conversations"], chat)
			}
			before := hash(source)
			plan, err := makePlan(source, testKeys())
			if err != nil || len(plan.Report.Issues) != 0 {
				t.Fatalf("plan failed: %v %#v", err, plan.Report.Issues)
			}
			if hash(source) != before {
				t.Fatal("dry-run changed source preferences")
			}
			converted := applyMemory(t, source, plan)
			if len(converted["connector_target_bindings"]) != len(want) {
				t.Fatalf("unexpected binding count: %d", len(converted["connector_target_bindings"]))
			}
			for _, binding := range converted["connector_target_bindings"] {
				if expected := want[text(binding, "target_id")]; expected == "" || text(binding, "connection_id") != expected {
					t.Fatal("preference crossed a workspace, owner or organization boundary")
				}
			}
			if report := verifySnapshot(converted, testKeys()); len(report.Issues) != 0 {
				t.Fatalf("verification failed: %#v", report.Issues)
			}
		})
	}
}

func TestChatPreferencesPreserveCanonicalRestrictionsAndRealConflicts(t *testing.T) {
	for _, conflicting := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_connection", true: "conflicting_connection"}[conflicting], func(t *testing.T) {
			source := chatPreferenceFixture(t)
			preferred := connID
			if conflicting {
				preferred = deterministicID("test", "conflicting-connection")
				addPreferenceConnection(t, source, preferred)
			}
			restriction := ownerConstraint(ownerID, []string{"github.user.get"})
			restriction["expires_at"] = "2099-01-01T00:00:00Z"
			addDeny(restriction, "retained_denial")
			source["connector_target_bindings"] = []row{{"id": bindingID, "organization_id": orgID, "owner_account_id": ownerID, "connection_id": connID, "connector_id": "github", "target_type": "chat", "target_id": agentID, "target_scope": "live", "target_version_id": "", "binding_instance_id": nil, "compatibility": "native", "compatibility_state": row{}, "status": "disabled", "binding_constraint": restriction, "revision": 3}}
			source["chat_runtime_conversations"] = []row{{"id": agentID, "organization_id": orgID, "account_id": ownerID, "workspace_id": workspaceID, "caller_type": "aichat"}}
			source["aichat_integration_preferences"] = []row{{"id": receiptID, "organization_id": orgID, "account_id": ownerID, "workspace_id": workspaceID, "integration_id": "github", "preferred_connection_id": preferred}}
			plan, err := makePlan(source, testKeys())
			if err != nil {
				t.Fatal(err)
			}
			if conflicting {
				if len(plan.Report.Issues) != 1 || plan.Report.Issues[0].Code != "multiple_connections_for_same_target" || plan.Report.Issues[0].ID != receiptID {
					t.Fatalf("real conflict was suppressed: %#v", plan.Report.Issues)
				}
				return
			}
			if len(plan.Report.Issues) != 0 {
				t.Fatalf("unexpected issues: %#v", plan.Report.Issues)
			}
			converted := applyMemory(t, source, plan)
			bindings := converted["connector_target_bindings"]
			if len(bindings) != 1 || text(bindings[0], "id") != bindingID || text(bindings[0], "connection_id") != connID || text(bindings[0], "status") != "disabled" || hash(bindings[0]["binding_constraint"]) != hash(restriction) {
				t.Fatal("canonical binding identity, status or restrictions changed")
			}
		})
	}
}
