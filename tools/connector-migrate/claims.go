package main

import (
	"encoding/json"
	"strings"
	"time"
)

func (c *converter) convertClaims() {
	continuations := map[string]row{}
	for _, source := range c.source["connector_confirmation_continuations"] {
		continuations[text(source, "route_receipt")] = source
	}
	for _, source := range c.source["connector_execution_route_claims"] {
		receipt := text(source, "receipt_id")
		id := text(source, "logical_operation_id")
		org := text(source, "organization_id")
		owner := text(source, "owner_account_id")
		if existing := findBy(c.source["connector_invocation_claims"], "receipt_id", receipt); existing != nil {
			if text(existing, "logical_operation_id") != id || text(existing, "organization_id") != org || text(existing, "owner_account_id") != owner {
				c.plan.problem("connector_invocation_claims", receipt, "receipt_identity_collision")
			}
			continue
		}
		claim := row{}
		for _, key := range []string{"receipt_id", "organization_id", "owner_account_id", "acting_principal_id", "binding_id", "connection_id", "connector_id", "target_type", "target_id", "operation_id", "logical_operation_id", "correlation_id", "created_at"} {
			claim[key] = source[key]
		}
		claim["binding_instance_id"] = text(source, "binding_instance_id")
		claim["target_scope"] = "draft"
		claim["target_version_id"] = ""
		if text(source, "target_type") == "chat" {
			claim["target_scope"] = "live"
		}
		claim["binding_revision"] = 1
		claim["credential_version"] = 1
		claim["input_digest"] = hash(nil)
		claim["contract_digest"] = hash(nil)
		claim["state"] = "unknown"
		for _, binding := range c.bindings {
			if text(binding, "id") == text(source, "binding_id") {
				claim["binding_revision"] = binding["revision"]
				claim["target_scope"] = binding["target_scope"]
				claim["target_version_id"] = text(binding, "target_version_id")
				break
			}
		}
		if connection := c.connections[text(source, "connection_id")]; connection != nil {
			claim["credential_version"] = connection["credential_version"]
		}
		submitted := false
		for _, decision := range c.source["connector_lifecycle_decisions"] {
			if text(decision, "organization_id") == org && text(decision, "logical_operation_id") == id {
				switch text(decision, "status") {
				case "submitted":
					submitted = true
					claim["state"] = "unknown"
				case "denied", "cancelled", "blocked":
					if !submitted {
						claim["state"] = decision["status"]
					}
				}
			}
		}
		for _, outcome := range c.source["connector_operation_outcomes"] {
			if text(outcome, "organization_id") == org && text(outcome, "logical_operation_id") == id {
				claim["state"] = outcome["status"]
				submitted = true
			}
		}
		continuation := continuations[receipt]
		if continuation != nil {
			subject := secretSubject{org, receipt, "connector-confirmation-continuation", "confirmation-continuation", 2}
			plaintext, err := decryptEnvelope(c.keys, subject, text(continuation, "protected_payload_envelope"))
			if err != nil {
				c.plan.problem("connector_confirmation_continuations", receipt, "continuation_decode_failed")
				continue
			}
			payload, err := decodeContinuation(plaintext)
			clear(plaintext)
			if err != nil {
				c.plan.problem("connector_confirmation_continuations", receipt, "continuation_payload_invalid")
				continue
			}
			var frozen row
			if json.Unmarshal(payload, &frozen) != nil {
				clear(payload)
				c.plan.problem("connector_confirmation_continuations", receipt, "continuation_payload_invalid")
				continue
			}
			clear(payload)
			request := object(frozen["request"])
			target := object(request["target"])
			accepted := object(frozen["accepted_operation"])
			valid := text(frozen, "route_receipt") == receipt && text(request, "organization_id") == org && text(request, "owner_account_id") == owner && text(request, "acting_principal_id") == owner && text(request, "logical_operation_id") == id && text(request, "connection_id") == text(source, "connection_id") && text(request, "operation_id") == text(source, "operation_id") && digestPattern.MatchString(text(accepted, "contract_digest"))
			origin := object(frozen["origin"])
			valid = valid && text(request, "connector_id") == text(source, "connector_id") && text(request, "binding_id") == text(source, "binding_id") && text(request, "correlation_id") == text(source, "correlation_id") && text(target, "type") == text(source, "target_type") && text(target, "id") == text(source, "target_id") && text(target, "binding_instance_id") == text(source, "binding_instance_id")
			if text(target, "type") == "workflow" {
				run := findBy(c.source["workflow_runs"], "id", text(origin, "run_id"))
				workflow := findBy(c.source["workflows"], "id", text(run, "workflow_id"))
				valid = valid && run != nil && workflow != nil && text(origin, "surface") == "workflow" && text(origin, "node_id") == text(target, "binding_instance_id") && text(origin, "workflow_id") == text(target, "id") && text(c.workflowTarget(workflow, owner), "target_id") == text(target, "id")
			}
			// Paused runs may have received their own immutable host snapshot.
			bindingID := ""
			if text(target, "type") == "workflow" {
				if host := c.runWorkflows[text(origin, "run_id")]; host != nil {
					mapped := c.workflowTarget(host, owner)
					target["id"], target["scope"], target["version_id"] = mapped["target_id"], mapped["target_scope"], mapped["target_version_id"]
					for _, raw := range list(object(host["graph"])["nodes"]) {
						node := object(raw)
						if text(node, "id") == text(target, "binding_instance_id") {
							bindingID = text(object(object(node["data"])["connector"]), "binding_id")
						}
					}
					origin["workflow_id"] = target["id"]
				}
			}
			valid = valid && c.validAcceptedSnapshot(accepted, text(request, "connector_id"), text(request, "operation_id"))
			var binding row
			for _, candidate := range c.bindings {
				if text(candidate, "organization_id") == org && text(candidate, "owner_account_id") == owner && text(candidate, "target_type") == text(target, "type") && text(candidate, "target_id") == text(target, "id") && text(candidate, "binding_instance_id") == text(target, "binding_instance_id") && text(candidate, "connection_id") == text(request, "connection_id") && (bindingID == "" || text(candidate, "id") == bindingID) && (text(target, "scope") == "" || text(target, "scope") == text(candidate, "target_scope")) && (text(target, "version_id") == "" || text(target, "version_id") == text(candidate, "target_version_id")) {
					if binding != nil && text(binding, "id") != text(candidate, "id") {
						valid = false
						break
					}
					binding = candidate
				}
			}
			if binding != nil {
				request["binding_id"] = binding["id"]
				target["id"], target["scope"], target["version_id"] = binding["target_id"], binding["target_scope"], text(binding, "target_version_id")
				request["target"] = target
				frozen["request"] = request
				frozen["origin"] = origin
				claim["binding_id"], claim["binding_revision"] = binding["id"], binding["revision"]
				claim["target_id"], claim["target_scope"], claim["target_version_id"] = binding["target_id"], binding["target_scope"], text(binding, "target_version_id")
				connection := c.connections[text(binding, "connection_id")]
				valid = valid && constraintAllows(object(binding["binding_constraint"]), binding, text(request, "operation_id"), time.Now()) && constraintAllows(object(connection["connection_constraint"]), binding, text(request, "operation_id"), time.Now()) && text(connection, "state") == "ready"
			}

			claim["input_digest"] = hash(frozen["input"])
			if digestPattern.MatchString(text(accepted, "contract_digest")) {
				claim["contract_digest"] = accepted["contract_digest"]
			}
			update := row{}
			if text(continuation, "state") == "pending" {
				if valid && binding != nil && text(binding, "status") == "active" && text(source, "path") == "connector_native" && !submitted && claim["state"] == "unknown" {
					claim["state"] = "awaiting_confirmation"
				} else {
					update["state"] = "failed"
					update["diagnostic_code"] = "connector_continuation_retired"
					update["terminal_at"] = time.Now().UTC().Format(time.RFC3339Nano)
				}
			}
			encoded, err := json.Marshal(frozen)
			if err != nil {
				c.plan.problem("connector_confirmation_continuations", receipt, "continuation_payload_invalid")
				continue
			}
			envelope, err := encryptNative(c.keys, subject, encoded)
			clear(encoded)
			if err != nil {
				c.plan.problem("connector_confirmation_continuations", receipt, "continuation_reseal_failed")
				continue
			}
			update["protected_payload_envelope"] = envelope
			// Insert the claim before rewriting its continuation because the new FK is
			// enforced on every updated row, even before historical validation.
			c.plan.add("connector_invocation_claims", nil, claim, true)
			c.plan.add("connector_confirmation_continuations", row{"route_receipt": receipt}, update, false)
			c.plan.record("continuation", receipt, continuation, receipt, "converted")
		} else {
			c.plan.add("connector_invocation_claims", nil, claim, true)
		}
		c.plan.record("route_claim", receipt, source, receipt, "converted")
	}
	for receipt := range continuations {
		if findBy(c.source["connector_execution_route_claims"], "receipt_id", receipt) == nil && findBy(c.source["connector_invocation_claims"], "receipt_id", receipt) == nil {
			c.plan.problem("connector_confirmation_continuations", receipt, "claim_missing")
		}
	}
	c.plan.record("source_inventory", "v1", sourceInventory(c.source), "", "converted")
}
func sourceInventory(snapshot databaseSnapshot) databaseSnapshot {
	selected := databaseSnapshot{}
	for _, name := range snapshotTables {
		if strings.HasPrefix(name, "integration_") || name == "agent_resource_bindings" || name == "aichat_integration_preferences" || name == "connector_execution_route_claims" {
			if rows, ok := snapshot[name]; ok {
				if name == "agent_resource_bindings" {
					filtered := []row{}
					for _, r := range rows {
						if text(r, "binding_type") == "integration_connection" {
							filtered = append(filtered, r)
						}
					}
					selected[name] = filtered
				} else {
					selected[name] = rows
				}
			}
		}
	}
	return selected
}

func (c *converter) validAcceptedSnapshot(accepted row, connector, operation string) bool {
	definition := object(accepted["definition"])
	op := object(accepted["operation"])
	expected := c.operation(connector, operation)
	return expected != nil && text(accepted, "connector_id") == connector && text(definition, "id") == connector && text(op, "id") == operation && hash(op) == hash(expected) && text(accepted, "contract_digest") == text(c.definitions[connector], "contract_digest") && text(accepted, "plugin_id") == text(definition, "plugin_id") && text(accepted, "plugin_version") == text(definition, "plugin_version")
}
