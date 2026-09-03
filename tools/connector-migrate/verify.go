package main

import (
	"encoding/json"
	"strings"
	"time"
)

func verifySnapshot(snapshot databaseSnapshot, keys keyrings) report {
	result := inventory(snapshot)
	result.State = "verified"
	c := &converter{source: snapshot, definitions: map[string]row{}, plan: conversionPlan{Report: inventory(snapshot)}}
	if c.seedDefinitions() != nil {
		result.Issues = append(result.Issues, issue{"connector_definitions", "", "native_contract_unreadable"})
		return result
	}
	problem := func(table, id, code string) { result.Issues = append(result.Issues, issue{table, id, code}) }
	if len(snapshot["connector_schema_readiness"]) != 1 || integer(snapshot["connector_schema_readiness"][0], "schema_version") != 1 {
		problem("connector_schema_readiness", "1", "native_schema_required")
	}
	connections := map[string]row{}
	for _, r := range snapshot["connector_connections"] {
		id := text(r, "id")
		connections[id] = r
		if !constraintValid(object(r["connection_constraint"]), text(r, "owner_account_id")) {
			problem("connector_connections", id, "connection_constraint_invalid")
		}
		if r["deleted_at"] != nil {
			continue
		}
		if !strings.HasPrefix(text(r, "encrypted_credentials"), "c1.") {
			problem("connector_connections", id, "non_native_secret")
			continue
		}
		data, err := decryptEnvelope(keys, secretSubject{text(r, "organization_id"), id, text(r, "connector_id"), "connection-key", integer(r, "credential_version")}, text(r, "encrypted_credentials"))
		if err != nil {
			problem("connector_connections", id, "native_secret_unreadable")
		} else {
			var values map[string]string
			if json.Unmarshal(data, &values) != nil || len(values) == 0 {
				problem("connector_connections", id, "native_secret_invalid")
			}
			clear(values)
			clear(data)
		}
	}
	bindings := map[string]row{}
	logical := map[string]bool{}
	for _, r := range snapshot["connector_target_bindings"] {
		id := text(r, "id")
		bindings[id] = r
		if text(r, "compatibility") != "native" || len(object(r["compatibility_state"])) != 0 {
			problem("connector_target_bindings", id, "non_native_binding")
		}
		if !constraintValid(object(r["binding_constraint"]), text(r, "owner_account_id")) {
			problem("connector_target_bindings", id, "binding_constraint_invalid")
		}
		key := targetKey(r)
		if logical[key] {
			problem("connector_target_bindings", id, "duplicate_target_identity")
		}
		logical[key] = true
		if !validTarget(r) {
			problem("connector_target_bindings", id, "target_scope_invalid")
		}
		conn := connections[text(r, "connection_id")]
		if conn == nil || text(conn, "organization_id") != text(r, "organization_id") || text(conn, "owner_account_id") != text(r, "owner_account_id") || text(conn, "connector_id") != text(r, "connector_id") {
			problem("connector_target_bindings", id, "connection_identity_mismatch")
		}
	}
	refs := map[string]row{}
	for _, r := range snapshot["connector_unavailable_references"] {
		refs[text(r, "id")] = r
		if !validTarget(r) || !validID(text(r, "owner_account_id")) {
			problem("connector_unavailable_references", text(r, "id"), "unavailable_target_invalid")
		}
	}
	claims := map[string]row{}
	for _, r := range snapshot["connector_invocation_claims"] {
		claims[text(r, "receipt_id")] = r
		if !digestPattern.MatchString(text(r, "input_digest")) || !digestPattern.MatchString(text(r, "contract_digest")) {
			problem("connector_invocation_claims", text(r, "receipt_id"), "claim_digest_invalid")
		}
	}
	for _, source := range snapshot["connector_execution_route_claims"] {
		claim := claims[text(source, "receipt_id")]
		if claim == nil || text(claim, "organization_id") != text(source, "organization_id") || text(claim, "owner_account_id") != text(source, "owner_account_id") || text(claim, "logical_operation_id") != text(source, "logical_operation_id") {
			problem("connector_execution_route_claims", text(source, "receipt_id"), "historic_claim_not_preserved")
		}
	}
	for _, r := range snapshot["connector_confirmation_continuations"] {
		id := text(r, "route_receipt")
		claim := claims[id]
		if claim == nil || text(claim, "organization_id") != text(r, "organization_id") || text(claim, "owner_account_id") != text(r, "owner_account_id") {
			problem("connector_confirmation_continuations", id, "claim_ownership_mismatch")
		}
		if !strings.HasPrefix(text(r, "protected_payload_envelope"), "c1.") {
			problem("connector_confirmation_continuations", id, "non_native_continuation")
			continue
		}
		data, err := decryptEnvelope(keys, secretSubject{text(r, "organization_id"), id, "connector-confirmation-continuation", "confirmation-continuation", 2}, text(r, "protected_payload_envelope"))
		if err != nil {
			problem("connector_confirmation_continuations", id, "native_continuation_unreadable")
			continue
		}
		var frozen row
		err = json.Unmarshal(data, &frozen)
		clear(data)
		if err != nil {
			problem("connector_confirmation_continuations", id, "native_continuation_invalid")
			continue
		}
		if text(r, "state") == "pending" {
			request := object(frozen["request"])
			accepted := object(frozen["accepted_operation"])
			binding := bindings[text(request, "binding_id")]
			if !pendingSnapshotMatches(frozen, claim, binding, connections[text(binding, "connection_id")]) || !c.validAcceptedSnapshot(accepted, text(request, "connector_id"), text(request, "operation_id")) {
				problem("connector_confirmation_continuations", id, "pending_snapshot_mismatch")
			}
		}
	}
	for _, table := range []string{"agents_configs", "agent_published_versions"} {
		for _, r := range snapshot[table] {
			field := "agent_mode"
			if table == "agent_published_versions" {
				field = "config_snapshot"
			}
			config := object(r[field])
			var target row
			if table == "agents_configs" {
				agent := findBy(snapshot["agents"], "id", text(r, "agents_id"))
				target = c.target("agent", text(r, "agents_id"), "draft", "", "", c.workspaceOrg(text(agent, "tenant_id")), "")
			} else {
				target = c.target("agent", text(r, "agent_id"), "published", text(r, "version_uuid"), "", c.workspaceOrg(text(r, "workspace_id")), "")
			}
			if len(list(config["integration_bindings"])) > 0 {
				problem(table, text(r, "id"), "legacy_agent_snapshot")
			}
			for _, raw := range list(config["connector_bindings"]) {
				b := object(raw)
				binding := bindings[text(b, "binding_id")]
				if !hostBindingMatches(binding, target, b) {
					problem(table, text(r, "id"), "agent_snapshot_binding_mismatch")
				}
			}
		}
	}
	for _, table := range []string{"workflows", "workflow_runs"} {
		for _, r := range snapshot[table] {
			graph := object(r["graph"])
			host := r
			if table == "workflow_runs" {
				host = findBy(snapshot["workflows"], "id", text(r, "workflow_id"))
			}
			target := c.workflowTarget(host, "")
			for _, raw := range list(graph["nodes"]) {
				node := object(raw)
				data := object(node["data"])
				target["binding_instance_id"] = text(node, "id")
				if text(data, "provider_type") != "connector" {
					continue
				}
				if text(data, "connection_id") != "" {
					problem(table, text(r, "id"), "legacy_workflow_snapshot")
				}
				if id := text(data, "connector_unavailable_ref"); id != "" {
					if !sameHostTarget(refs[id], target) {
						problem(table, text(r, "id"), "unavailable_reference_missing")
					}
					continue
				}
				ref := object(data["connector"])
				binding := bindings[text(ref, "binding_id")]
				if len(ref) > 0 && (!hostBindingMatches(binding, target, ref) || c.operation(text(ref, "connector_id"), text(ref, "operation_id")) == nil) {
					problem(table, text(r, "id"), "workflow_snapshot_binding_mismatch")
				}
			}
		}
	}
	// Source tables are archival evidence. If retained, any subsequent mutation
	// makes verification fail; removing the archive is outside this program.
	for _, skill := range snapshot["chat_runtime_custom_skills"] {
		file := findBy(snapshot["connector_skill_file_inventory"], "id", text(skill, "id"))
		if text(skill, "storage_path") != "" && file == nil {
			problem("chat_runtime_custom_skills", text(skill, "id"), "skill_file_inventory_required")
		}
		if file != nil && text(file, "state") != "native" && text(skill, "status") != "invalid" {
			problem("chat_runtime_custom_skills", text(skill, "id"), "retired_skill_still_active")
		}
	}
	for _, ledger := range snapshot["connector_conversion_records"] {
		if text(ledger, "source_kind") == "source_inventory" && snapshot["integration_connections"] != nil && text(ledger, "source_digest") != hash(sourceInventory(snapshot)) {
			problem("connector_conversion_records", "v1", "archived_source_changed")
		}
	}
	return result
}

func sameHostTarget(binding, target row) bool {
	if binding == nil {
		return false
	}
	for _, field := range []string{"organization_id", "target_type", "target_id", "target_scope", "target_version_id", "binding_instance_id"} {
		if text(binding, field) != text(target, field) {
			return false
		}
	}
	return true
}

func hostBindingMatches(binding, target, ref row) bool {
	return sameHostTarget(binding, target) && text(binding, "connection_id") == text(ref, "connection_id") && text(binding, "connector_id") == text(ref, "connector_id") && text(binding, "status") == "active"
}

func pendingSnapshotMatches(frozen, claim, binding, connection row) bool {
	if claim == nil || binding == nil || connection == nil || (text(claim, "state") != "awaiting_confirmation" && text(claim, "state") != "claimed") || text(binding, "status") != "active" || text(connection, "state") != "ready" {
		return false
	}
	request, target, accepted := object(frozen["request"]), object(object(frozen["request"])["target"]), object(frozen["accepted_operation"])
	for _, field := range []string{"organization_id", "owner_account_id", "acting_principal_id", "binding_id", "connection_id", "connector_id", "operation_id", "logical_operation_id", "correlation_id"} {
		if text(request, field) != text(claim, field) {
			return false
		}
	}
	for _, pair := range [][2]string{{"type", "target_type"}, {"id", "target_id"}, {"scope", "target_scope"}, {"version_id", "target_version_id"}, {"binding_instance_id", "binding_instance_id"}} {
		if text(target, pair[0]) != text(claim, pair[1]) || text(binding, pair[1]) != text(claim, pair[1]) {
			return false
		}
	}
	for _, field := range []string{"organization_id", "owner_account_id", "connection_id", "connector_id"} {
		if text(binding, field) != text(claim, field) {
			return false
		}
	}
	return text(frozen, "route_receipt") == text(claim, "receipt_id") && hash(frozen["input"]) == text(claim, "input_digest") && text(accepted, "contract_digest") == text(claim, "contract_digest") && integer(binding, "revision") == integer(claim, "binding_revision") && integer(connection, "credential_version") == integer(claim, "credential_version") && constraintAllows(object(binding["binding_constraint"]), binding, text(claim, "operation_id"), time.Now()) && constraintAllows(object(connection["connection_constraint"]), binding, text(claim, "operation_id"), time.Now())
}
func validTarget(r row) bool {
	if !validID(text(r, "target_id")) {
		return false
	}
	kind, scope, version := text(r, "target_type"), text(r, "target_scope"), text(r, "target_version_id")
	if kind == "chat" {
		return scope == "live" && version == "" && text(r, "binding_instance_id") == ""
	}
	if kind != "agent" && kind != "workflow" {
		return false
	}
	if kind == "workflow" && text(r, "binding_instance_id") == "" {
		return false
	}
	return scope == "draft" && version == "" || scope == "published" && validID(version)
}
