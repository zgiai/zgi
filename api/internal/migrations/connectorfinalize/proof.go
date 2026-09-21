package connectorfinalize

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func validateSource(data snapshot, report *Report) {
	problem := func(table, code string) { report.Issues = append(report.Issues, Issue{Table: table, Code: code}) }
	validateSourceCoverage(data, report)
	for _, binding := range data["connector_target_bindings"] {
		state, ok := binding["compatibility_state"].(map[string]any)
		if text(binding, "compatibility") != "native" || !ok || len(state) != 0 {
			problem("connector_target_bindings", "non_native_binding")
		}
	}
	for _, binding := range data["agent_resource_bindings"] {
		if text(binding, "binding_type") == "integration_connection" && !legacyBindingCovered(data, binding) {
			problem("agent_resource_bindings", "legacy_binding_conversion_required")
		}
	}
	for _, source := range data["connector_execution_route_claims"] {
		claim := find(data["connector_invocation_claims"], "receipt_id", text(source, "receipt_id"))
		if claim == nil {
			problem("connector_execution_route_claims", "permanent_claim_missing")
			continue
		}
		valid := true
		for _, field := range []string{"receipt_id", "organization_id", "owner_account_id", "acting_principal_id", "logical_operation_id", "binding_id", "connection_id", "connector_id", "operation_id", "target_type", "target_id", "binding_instance_id", "correlation_id"} {
			if text(source, field) != text(claim, field) {
				valid = false
			}
		}
		if !valid {
			problem("connector_execution_route_claims", "permanent_claim_identity_mismatch")
			continue
		}
		if text(claim, "target_type") == "chat" && (text(claim, "target_scope") != "live" || text(claim, "target_version_id") != "") {
			problem("connector_execution_route_claims", "claim_target_scope_mismatch")
		}
		// Old route rows contain neither original input nor accepted contract.
		// Their terminal permanent barrier is retained exactly; no digest is invented.
		switch text(claim, "state") {
		case "successful", "failed", "unknown", "denied", "blocked", "cancelled":
			report.DigestUnavailableClaims++
		case "claimed", "awaiting_confirmation":
			continuation := find(data["connector_confirmation_continuations"], "route_receipt", text(claim, "receipt_id"))
			if continuation == nil || text(continuation, "organization_id") != text(claim, "organization_id") || text(continuation, "owner_account_id") != text(claim, "owner_account_id") {
				problem("connector_execution_route_claims", "pending_frozen_proof_required")
			}
		default:
			problem("connector_execution_route_claims", "pending_frozen_proof_required")
		}
		matchedOutcome := false
		for _, outcome := range data["connector_operation_outcomes"] {
			if text(outcome, "organization_id") != text(claim, "organization_id") || text(outcome, "logical_operation_id") != text(claim, "logical_operation_id") {
				continue
			}
			matchedOutcome = true
			for _, field := range []string{"organization_id", "owner_account_id", "acting_principal_id", "connection_id", "connector_id", "operation_id", "target_type", "target_id", "target_scope", "target_version_id", "binding_instance_id"} {
				observed := text(outcome, field)
				// Before scope columns existed, Chat outcomes had an empty scope.
				// Chat has exactly one live target scope. Infer only this provable
				// historical fact, without changing either immutable row.
				if field == "target_scope" && observed == "" && text(source, "target_type") == "chat" && text(outcome, "target_type") == "chat" {
					observed = string(core.ScopeLive)
				}
				if observed != text(claim, field) {
					problem("connector_execution_route_claims", "outcome_identity_mismatch_"+field)
					break
				}
			}
			if text(outcome, "status") != text(claim, "state") {
				problem("connector_execution_route_claims", "outcome_state_mismatch")
			}
		}
		if !matchedOutcome && (text(claim, "state") == "successful" || text(claim, "state") == "failed") {
			problem("connector_execution_route_claims", "terminal_outcome_missing")
		}
	}
	validateNative(data, report)
}

func validateNative(data snapshot, report *Report) {
	problem := func(table, code string) { report.Issues = append(report.Issues, Issue{Table: table, Code: code}) }
	for _, connection := range data["connector_connections"] {
		if !validConstraint(connection["connection_constraint"], text(connection, "owner_account_id")) {
			problem("connector_connections", "invalid_native_constraint")
		}
		if connection["deleted_at"] == nil && !strings.HasPrefix(text(connection, "encrypted_credentials"), "c1.") {
			problem("connector_connections", "non_native_credential")
		}
	}
	for _, binding := range data["connector_target_bindings"] {
		connection := find(data["connector_connections"], "id", text(binding, "connection_id"))
		if connection == nil {
			problem("connector_target_bindings", "connection_missing")
			continue
		}
		for _, field := range []string{"organization_id", "owner_account_id", "connector_id"} {
			if text(connection, field) != text(binding, field) {
				problem("connector_target_bindings", "connection_identity_mismatch")
				break
			}
		}
		if !validConstraint(binding["binding_constraint"], text(binding, "owner_account_id")) {
			problem("connector_target_bindings", "invalid_native_constraint")
		}
		target := core.TargetRef{Type: core.TargetType(text(binding, "target_type")), ID: text(binding, "target_id"), Scope: core.TargetScope(text(binding, "target_scope")), VersionID: text(binding, "target_version_id"), BindingInstanceID: text(binding, "binding_instance_id")}
		if target.Validate() != nil || number(binding, "revision") < 1 {
			problem("connector_target_bindings", "invalid_target_or_revision")
		}
	}
	for _, continuation := range data["connector_confirmation_continuations"] {
		claim := find(data["connector_invocation_claims"], "receipt_id", text(continuation, "route_receipt"))
		if claim == nil || text(claim, "organization_id") != text(continuation, "organization_id") || text(claim, "owner_account_id") != text(continuation, "owner_account_id") {
			problem("connector_confirmation_continuations", "claim_owner_mismatch")
		}
		if !strings.HasPrefix(text(continuation, "protected_payload_envelope"), "c1.") {
			problem("connector_confirmation_continuations", "non_native_envelope")
		}
	}
}

func validConstraint(value any, owner string) bool {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	constraint, err := core.DecodeConstraint(encoded)
	ownerID, ownerErr := uuid.Parse(owner)
	return err == nil && ownerErr == nil && constraint != nil && constraint.AudienceOwner == ownerID
}
