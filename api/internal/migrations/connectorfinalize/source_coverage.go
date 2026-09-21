package connectorfinalize

import (
	"slices"
	"time"
)

func sourceProof(data snapshot, kind, id string, source row) row {
	for _, proof := range data["connector_conversion_records"] {
		if text(proof, "source_kind") == kind && text(proof, "source_id") == id && text(proof, "source_digest") == digest(source) {
			return proof
		}
	}
	return nil
}

func stringValues(value any) []string {
	result := []string{}
	if values, ok := value.([]any); ok {
		for _, value := range values {
			if value, ok := value.(string); ok {
				result = append(result, value)
			}
		}
	}
	return result
}

func validateSourceCoverage(data snapshot, report *Report) {
	for _, source := range data["integration_connections"] {
		id := text(source, "id")
		native := find(data["connector_connections"], "id", id)
		proof := sourceProof(data, "source_connection", id, source)
		if native == nil {
			if proof == nil || text(proof, "result") != "unavailable" {
				addIssue(report, "integration_connections", "source_coverage_required")
			}
			continue
		}
		constraint := object(native["connection_constraint"])
		if text(native, "organization_id") != text(source, "organization_id") || text(native, "owner_account_id") != text(source, "owner_account_id") || text(native, "connector_id") != text(source, "integration_id") || !validConstraint(constraint, text(source, "owner_account_id")) {
			addIssue(report, "integration_connections", "native_identity_or_constraint_mismatch")
			continue
		}
		if text(constraint, "source_digest") != digest(source) && (proof == nil || text(proof, "result") != "converted") {
			addIssue(report, "integration_connections", "source_constraint_proof_required")
		}
		denied := len(stringValues(constraint["deny_reasons"])) > 0 || text(native, "state") == "unavailable" || native["deleted_at"] != nil
		if !denied {
			if len(object(source["config"])) > 0 || (text(source, "status") != "active" && text(source, "status") != "") {
				addIssue(report, "integration_connections", "source_restriction_widened")
			}
			if expiry := text(source, "expires_at"); expiry != "" {
				old, oldErr := time.Parse(time.RFC3339Nano, expiry)
				current, currentErr := time.Parse(time.RFC3339Nano, text(constraint, "expires_at"))
				if oldErr != nil || currentErr != nil || current.After(old) {
					addIssue(report, "integration_connections", "source_expiry_widened")
				}
			}
			for _, policy := range data["integration_action_policies"] {
				if text(policy, "organization_id") != text(source, "organization_id") || text(policy, "integration_id") != text(source, "integration_id") {
					continue
				}
				if text(policy, "approval_policy") == "always_ask" || ((policy["enabled"] == false || policy["data_egress_allowed"] == false) && slices.Contains(stringValues(constraint["allowed_operation_ids"]), text(policy, "action_id"))) {
					addIssue(report, "integration_action_policies", "policy_restriction_widened")
				}
			}
			for _, grant := range data["integration_connection_grants"] {
				if text(grant, "connection_id") == id && len(object(grant["resource_constraints"])) > 0 {
					addIssue(report, "integration_connection_grants", "grant_restriction_requires_review")
				}
			}
		}
	}
	for _, source := range data["aichat_integration_preferences"] {
		if sourceProof(data, "chat_preference", text(source, "id"), source) == nil {
			addIssue(report, "aichat_integration_preferences", "preference_conversion_required")
		}
	}
	for _, receipt := range data["integration_operation_receipts"] {
		if !canonicalUUID(text(receipt, "id")) || !canonicalUUID(text(receipt, "organization_id")) || !canonicalUUID(text(receipt, "message_id")) || !slices.Contains([]string{"executing", "succeeded", "outcome_unknown"}, text(receipt, "status")) {
			addIssue(report, "integration_operation_receipts", "invalid_historical_receipt")
		}
	}
	// All other fixed source tables contain obsolete control state or audit
	// evidence. The complete authenticated archive covers every row, including
	// abandoned onboarding state. None is interpreted as executable authority.
}

func legacyBindingCovered(data snapshot, source row) bool {
	proof := sourceProof(data, "host_binding", text(source, "id"), source)
	if proof == nil {
		return false
	}
	kind, id := text(source, "consumer_type"), text(source, "consumer_id")
	if kind == "" {
		kind = "agent"
	}
	if id == "" {
		id = text(source, "agent_id")
	}
	scope, version := text(source, "binding_scope"), ""
	if scope == "published" {
		version = text(source, "published_version_uuid")
		if kind == "workflow" {
			id = version
		}
	}
	matches := func(target row) bool {
		return text(target, "organization_id") == text(source, "organization_id") && text(target, "owner_account_id") == text(source, "authorized_by") && text(target, "target_type") == kind && text(target, "target_id") == id && text(target, "target_scope") == scope && text(target, "target_version_id") == version && text(target, "binding_instance_id") == text(source, "binding_instance_id")
	}
	if text(proof, "result") == "unavailable" {
		for _, ref := range data["connector_unavailable_references"] {
			if text(ref, "former_reference_id") == text(source, "id") && matches(ref) {
				return true
			}
		}
		return false
	}
	if text(proof, "result") != "converted" {
		return false
	}
	binding := find(data["connector_target_bindings"], "id", text(proof, "destination_id"))
	if binding == nil || !matches(binding) || text(binding, "connection_id") != text(source, "resource_id") || text(binding, "connector_id") != text(source, "parent_resource_id") {
		return false
	}
	constraint := object(binding["binding_constraint"])
	if !validConstraint(constraint, text(source, "authorized_by")) {
		return false
	}
	if source["revoked_at"] != nil || source["deleted_at"] != nil || text(source, "status") == "revoked" || text(source, "status") == "disabled" {
		if text(binding, "status") == "active" {
			return false
		}
	}
	if len(stringValues(constraint["deny_reasons"])) == 0 && text(binding, "status") == "active" {
		metadata := object(source["metadata"])
		if allowed, exists := metadata["allowed_action_ids"]; exists {
			for _, operation := range stringValues(constraint["allowed_operation_ids"]) {
				if !slices.Contains(stringValues(allowed), operation) {
					return false
				}
			}
		}
	}
	return true
}
