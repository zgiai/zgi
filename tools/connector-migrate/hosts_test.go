package main

import (
	"encoding/json"
	"testing"
)

func TestAgentSnapshotsCarryCompleteRestrictions(t *testing.T) {
	source := fixture(t)
	restriction := ownerConstraint(ownerID, []string{"github.user.get"})
	restriction["target_boundaries"] = []any{row{"type": "agent", "id": agentID, "scope": "*", "version_id": "*"}}
	restriction["expires_at"] = "2099-01-01T00:00:00Z"
	source["connector_target_bindings"] = []row{{"id": bindingID, "organization_id": orgID, "owner_account_id": ownerID, "connection_id": connID, "connector_id": "github", "target_type": "agent", "target_id": agentID, "target_scope": "draft", "target_version_id": "", "binding_instance_id": nil, "compatibility": "native", "compatibility_state": row{}, "status": "active", "binding_constraint": restriction, "revision": 3}}
	source["agent_published_versions"] = []row{{"id": receiptID, "agent_id": agentID, "workspace_id": workspaceID, "version_uuid": receiptID, "created_by": ownerID, "config_snapshot": row{"connector_bindings": []any{row{"binding_id": bindingID, "connection_id": connID, "connector_id": "github"}}}}}
	plan, err := makePlan(source, testKeys())
	if err != nil || len(plan.Report.Issues) > 0 {
		t.Fatalf("plan: %v %#v", err, plan.Report.Issues)
	}
	converted := applyMemory(t, source, plan)
	config := object(converted["agent_published_versions"][0]["config_snapshot"])
	ref := object(list(config["connector_bindings"])[0])
	b := findBy(converted["connector_target_bindings"], "id", text(ref, "binding_id"))
	if text(b, "id") == bindingID || text(b, "target_scope") != "published" || text(b, "target_version_id") != receiptID {
		t.Fatal("published identity was not isolated")
	}
	got := object(b["binding_constraint"])
	for _, key := range []string{"allowed_operation_ids", "target_boundaries", "expires_at", "deny_reasons", "audience_owner"} {
		if hash(got[key]) != hash(restriction[key]) {
			t.Fatalf("restriction lost: %s", key)
		}
	}
}

func TestPausedWorkflowKeepsFrozenInputReceiptAndItsOwnVersion(t *testing.T) {
	source := fixture(t)
	source["agent_resource_bindings"] = nil
	source["agents_configs"] = nil
	source["connector_target_bindings"] = []row{{"id": bindingID, "organization_id": orgID, "owner_account_id": ownerID, "connection_id": connID, "connector_id": "github", "target_type": "workflow", "target_id": agentID, "target_scope": "draft", "target_version_id": "", "binding_instance_id": "node", "compatibility": "native", "compatibility_state": row{}, "status": "active", "binding_constraint": ownerConstraint(ownerID, []string{"github.user.get"}), "revision": 3}}
	c := &converter{source: source, definitions: map[string]row{}, plan: conversionPlan{Report: inventory(source)}}
	if err := c.seedDefinitions(); err != nil {
		t.Fatal(err)
	}
	definition := object(c.definitions["github"]["definition"])
	operation := c.operation("github", "github.user.get")
	graph := row{"nodes": []any{row{"id": "node", "data": row{"provider_type": "connector", "provider_id": "github", "tool_name": operation["tool_name"], "connector": row{"binding_id": bindingID, "connection_id": connID, "connector_id": "github", "operation_id": "github.user.get"}}}}}
	source["workflows"] = []row{{"id": agentID, "agent_id": agentID, "tenant_id": workspaceID, "version": "draft", "created_by": ownerID, "graph": row{"nodes": []any{}}}}
	source["workflow_runs"] = []row{{"id": receiptID, "workflow_id": agentID, "created_by": ownerID, "graph": graph}}
	source["workflow_run_pauses"] = []row{{"id": bindingID, "workflow_run_id": receiptID, "state_json": row{"workflow_id": agentID, "graph": graph}}}
	source["connector_execution_route_claims"] = []row{{"receipt_id": receiptID, "organization_id": orgID, "owner_account_id": ownerID, "acting_principal_id": ownerID, "binding_id": bindingID, "connection_id": connID, "connector_id": "github", "target_type": "workflow", "target_id": agentID, "binding_instance_id": "node", "operation_id": "github.user.get", "logical_operation_id": "frozen-operation", "correlation_id": "frozen-correlation", "path": "connector_native"}}
	accepted := row{"connector_id": "github", "plugin_id": definition["plugin_id"], "plugin_version": definition["plugin_version"], "plugin_digest": definition["plugin_digest"], "contract_digest": c.definitions["github"]["contract_digest"], "definition": definition, "operation": operation}
	frozen := row{"version": 2, "route_receipt": receiptID, "request": row{"organization_id": orgID, "owner_account_id": ownerID, "acting_principal_id": ownerID, "binding_id": bindingID, "connection_id": connID, "connector_id": "github", "target": row{"type": "workflow", "id": agentID, "binding_instance_id": "node"}, "operation_id": "github.user.get", "logical_operation_id": "frozen-operation", "correlation_id": "frozen-correlation", "interactive": true}, "origin": row{"surface": "workflow", "workflow_id": agentID, "node_id": "node", "run_id": receiptID}, "accepted_operation": accepted, "input": row{"private": "frozen-input"}}
	encoded, _ := json.Marshal(frozen)
	source["connector_confirmation_continuations"] = []row{{"route_receipt": receiptID, "organization_id": orgID, "owner_account_id": ownerID, "state": "pending", "protected_payload_envelope": oldEnvelope(t, "v2", secretSubject{orgID, receiptID, "connector-confirmation-continuation", "confirmation-continuation", 2}, encoded)}}
	plan, err := makePlan(source, testKeys())
	if err != nil || len(plan.Report.Issues) > 0 {
		t.Fatalf("plan: %v %#v", err, plan.Report.Issues)
	}
	converted := applyMemory(t, source, plan)
	claim := converted["connector_invocation_claims"][0]
	if text(claim, "state") != "awaiting_confirmation" || text(claim, "logical_operation_id") != "frozen-operation" || text(claim, "receipt_id") != receiptID || text(claim, "target_scope") != "published" || text(claim, "target_id") == agentID {
		t.Fatalf("bad continuation identity: %#v", claim)
	}
	continuation := converted["connector_confirmation_continuations"][0]
	opened, err := decryptEnvelope(testKeys(), secretSubject{orgID, receiptID, "connector-confirmation-continuation", "confirmation-continuation", 2}, text(continuation, "protected_payload_envelope"))
	if err != nil {
		t.Fatal(err)
	}
	var got row
	json.Unmarshal(opened, &got)
	clear(opened)
	if hash(got["input"]) != hash(frozen["input"]) || hash(got["accepted_operation"]) != hash(accepted) {
		t.Fatal("accepted operation or input changed")
	}
	request := object(got["request"])
	if text(object(request["target"]), "id") != text(claim, "target_id") || text(object(got["origin"]), "workflow_id") != text(claim, "target_id") || text(request, "binding_id") != text(claim, "binding_id") {
		t.Fatal("host and frozen identities disagree")
	}
	if result := verifySnapshot(converted, testKeys()); len(result.Issues) > 0 {
		t.Fatalf("verify: %#v", result.Issues)
	}
}
