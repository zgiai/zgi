package connectorfinalize

import (
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"gorm.io/gorm"
)

func canonicalUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func sha256Digest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32 && hex.EncodeToString(b) == value
}

func addIssue(report *Report, table, code string) {
	report.Issues = append(report.Issues, Issue{Table: table, Code: code})
}

func validateNativeCatalog(db *gorm.DB, data snapshot, report *Report) {
	versions := map[string]catalog.Definition{}
	for _, version := range data["connector_definition_versions"] {
		var definition catalog.Definition
		encoded, err := json.Marshal(version["definition"])
		if err != nil || json.Unmarshal(encoded, &definition) != nil {
			addIssue(report, "connector_definition_versions", "invalid_definition_snapshot")
			continue
		}
		definition.PluginDigest = text(version, "plugin_digest")
		commitment, err := catalog.DefinitionContractDigest(definition)
		if err != nil || catalog.Validate(definition) != nil || commitment != text(version, "contract_digest") ||
			definition.ID != text(version, "connector_id") || definition.PluginID != text(version, "plugin_id") || definition.PluginVersion != text(version, "plugin_version") {
			addIssue(report, "connector_definition_versions", "accepted_contract_mismatch")
			continue
		}
		var plugin row
		for _, candidate := range data["connector_plugin_versions"] {
			if text(candidate, "plugin_id") == definition.PluginID && text(candidate, "plugin_version") == definition.PluginVersion && text(candidate, "plugin_digest") == definition.PluginDigest {
				plugin = candidate
				break
			}
		}
		if plugin == nil || text(plugin, "source_dialect") != definition.SourceDialect || text(plugin, "schema_version") != definition.SourceSchemaVersion || text(plugin, "status") != text(version, "status") {
			addIssue(report, "connector_plugin_versions", "definition_version_mismatch")
		}
		versions[definition.ID+":"+definition.PluginVersion+":"+definition.PluginDigest] = definition
	}
	state, err := catalog.NewGormDefinitionRepository(db).LoadRuntimeState(db.Statement.Context)
	if err != nil {
		addIssue(report, "connector_definitions", "invalid_current_projection")
		return
	}
	for _, definition := range state.Definitions {
		accepted, exists := versions[definition.ID+":"+definition.PluginVersion+":"+definition.PluginDigest]
		if !exists || digest(accepted) != digest(definition) {
			addIssue(report, "connector_definitions", "current_accepted_snapshot_mismatch")
		}
	}
}

func historicalClaim(data snapshot, claim row) bool {
	switch text(claim, "state") {
	case "successful", "failed", "unknown", "denied", "blocked", "cancelled":
	default:
		return false
	}
	// A receipt alone is insufficient. Source coverage performs the full
	// comparison before apply; repeat verification uses the sealed proof record.
	for _, source := range data["connector_execution_route_claims"] {
		if text(source, "receipt_id") == text(claim, "receipt_id") && text(source, "organization_id") == text(claim, "organization_id") && text(source, "logical_operation_id") == text(claim, "logical_operation_id") {
			return true
		}
	}
	for _, proof := range data["connector_conversion_records"] {
		if text(proof, "source_kind") == "retirement_claim_v2" && text(proof, "source_id") == text(claim, "receipt_id") && text(proof, "result") == "digest_unavailable" && text(proof, "source_digest") == digest(claim) {
			switch text(claim, "state") {
			case "successful", "failed", "unknown", "denied", "blocked", "cancelled":
				return true
			}
		}
	}
	return false
}

func nativeClaim(data snapshot, organization, logical string) row {
	for _, claim := range data["connector_invocation_claims"] {
		if text(claim, "organization_id") == organization && text(claim, "logical_operation_id") == logical {
			return claim
		}
	}
	return nil
}

func sameClaimCoordinates(claim, record row, historical bool) bool {
	for _, field := range []string{"organization_id", "owner_account_id", "acting_principal_id", "connection_id", "connector_id", "operation_id", "logical_operation_id", "target_type", "target_id", "target_scope", "target_version_id", "binding_instance_id"} {
		observed := text(record, field)
		if historical && field == "target_scope" && observed == "" && text(record, "target_type") == "chat" {
			observed = "live"
		}
		if observed != text(claim, field) {
			return false
		}
	}
	return true
}

func validateNativeClaims(data snapshot, report *Report) {
	for _, claim := range data["connector_invocation_claims"] {
		valid := true
		for _, field := range []string{"receipt_id", "organization_id", "owner_account_id", "acting_principal_id", "binding_id", "connection_id"} {
			valid = valid && canonicalUUID(text(claim, field))
		}
		target := core.TargetRef{Type: core.TargetType(text(claim, "target_type")), ID: text(claim, "target_id"), Scope: core.TargetScope(text(claim, "target_scope")), VersionID: text(claim, "target_version_id"), BindingInstanceID: text(claim, "binding_instance_id")}
		valid = valid && text(claim, "owner_account_id") == text(claim, "acting_principal_id") && target.Validate() == nil &&
			core.Identifier(text(claim, "connector_id")).Valid() && core.Identifier(text(claim, "operation_id")).Valid() && strings.TrimSpace(text(claim, "logical_operation_id")) != "" && strings.TrimSpace(text(claim, "correlation_id")) != "" &&
			sha256Digest(text(claim, "input_digest")) && sha256Digest(text(claim, "contract_digest")) && number(claim, "binding_revision") > 0 && number(claim, "credential_version") > 0
		if !valid {
			addIssue(report, "connector_invocation_claims", "invalid_immutable_identity")
		}
		historical := historicalClaim(data, claim)
		if !historical {
			accepted := false
			for _, version := range data["connector_definition_versions"] {
				if text(version, "connector_id") == text(claim, "connector_id") && text(version, "contract_digest") == text(claim, "contract_digest") {
					accepted = true
					break
				}
			}
			if !accepted {
				addIssue(report, "connector_invocation_claims", "accepted_contract_missing")
			}
		}
		outcomes := 0
		for _, outcome := range data["connector_operation_outcomes"] {
			if text(outcome, "organization_id") != text(claim, "organization_id") || text(outcome, "logical_operation_id") != text(claim, "logical_operation_id") {
				continue
			}
			outcomes++
			if !sameClaimCoordinates(claim, outcome, historical) || text(claim, "state") != text(outcome, "status") {
				addIssue(report, "connector_operation_outcomes", "claim_identity_or_state_mismatch")
			}
		}
		if outcomes > 1 || (outcomes != 1 && (text(claim, "state") == "successful" || text(claim, "state") == "failed")) {
			addIssue(report, "connector_invocation_claims", "terminal_outcome_cardinality")
		}
	}
	for _, outcome := range data["connector_operation_outcomes"] {
		claim := nativeClaim(data, text(outcome, "organization_id"), text(outcome, "logical_operation_id"))
		decision := find(data["connector_lifecycle_decisions"], "id", text(outcome, "decision_id"))
		if claim == nil || decision == nil || text(decision, "status") != "submitted" || !core.OutcomeStatus(text(outcome, "status")).Valid() ||
			!sameClaimCoordinates(claim, decision, historicalClaim(data, claim)) || !canonicalUUID(text(outcome, "id")) {
			addIssue(report, "connector_operation_outcomes", "submitted_decision_proof_missing")
		}
	}
}

func validateNativeEnvelopes(db *gorm.DB, data snapshot, report *Report, secrets *credentials.Store) {
	open := func(table string, subject credentials.Subject, envelope string) {
		plain, err := secrets.Open(subject, envelope)
		defer clear(plain)
		if err != nil || !json.Valid(plain) {
			addIssue(report, table, "authenticated_envelope_unavailable")
			return
		}
		report.VerifiedEnvelopes++
	}
	for _, connection := range data["connector_connections"] {
		if connection["deleted_at"] != nil && text(connection, "encrypted_credentials") == "removed" && text(connection, "state") == "unavailable" && text(connection, "diagnostic_code") == "connection_removed" {
			continue
		}
		open("connector_connections", credentials.Subject{Purpose: credentials.ConnectionKey, OrganizationID: uuidValue(connection, "organization_id"), ID: uuidValue(connection, "id"), ConnectorID: text(connection, "connector_id"), Version: number(connection, "credential_version")}, text(connection, "encrypted_credentials"))
	}
	for _, device := range data["connector_onboarding_secrets"] {
		flow := find(data["connector_onboarding_flows"], "id", text(device, "flow_id"))
		if flow == nil || text(flow, "organization_id") != text(device, "organization_id") || text(flow, "owner_account_id") != text(device, "owner_account_id") || number(flow, "generation") != number(device, "generation") {
			addIssue(report, "connector_onboarding_secrets", "flow_identity_mismatch")
			continue
		}
		open("connector_onboarding_secrets", credentials.Subject{Purpose: credentials.OnboardingDevice, OrganizationID: uuidValue(device, "organization_id"), ID: uuidValue(device, "flow_id"), ConnectorID: text(flow, "connector_id"), Version: number(device, "generation")}, text(device, "encrypted_device"))
	}
	for _, table := range []string{"connector_messaging_states", "connector_messaging_contexts"} {
		for _, record := range data[table] {
			purpose, version, column := credentials.MessagingState, 1, "encrypted_state"
			if table == "connector_messaging_contexts" {
				purpose, version, column = credentials.MessagingContext, number(record, "credential_version"), "encrypted_context"
			}
			open(table, credentials.Subject{Purpose: purpose, OrganizationID: uuidValue(record, "organization_id"), ID: uuidValue(record, "id"), ConnectorID: text(record, "connector_id"), Version: version}, text(record, column))
		}
	}
	if len(data["connector_confirmation_continuations"]) == 0 {
		return
	}
	protector, err := credentials.NewContinuationPayloadProtector(secrets)
	if err != nil {
		addIssue(report, "connector_confirmation_continuations", "protector_unavailable")
		return
	}
	store, err := execution.NewGormConfirmationContinuationStore(db, protector)
	if err != nil {
		addIssue(report, "connector_confirmation_continuations", "store_unavailable")
		return
	}
	for _, record := range data["connector_confirmation_continuations"] {
		continuation, err := store.Load(db.Statement.Context, uuidValue(record, "organization_id"), uuidValue(record, "owner_account_id"), execution.RouteClaimReceipt(text(record, "route_receipt")))
		if err != nil {
			addIssue(report, "connector_confirmation_continuations", "invalid_frozen_payload")
			continue
		}
		claim := find(data["connector_invocation_claims"], "receipt_id", text(record, "route_receipt"))
		request := continuation.Frozen.Request
		coordinates := row{"organization_id": request.OrganizationID.String(), "owner_account_id": request.OwnerAccountID.String(), "acting_principal_id": request.ActingPrincipalID.String(), "connection_id": request.ConnectionID.String(), "connector_id": request.ConnectorID.String(), "operation_id": request.OperationID.String(), "logical_operation_id": request.LogicalOperationID, "target_type": string(request.Target.Type), "target_id": request.Target.ID, "target_scope": string(request.Target.EffectiveScope()), "target_version_id": request.Target.VersionID, "binding_instance_id": request.Target.BindingInstanceID}
		if claim == nil || !sameClaimCoordinates(claim, coordinates, false) || request.BindingID.String() != text(claim, "binding_id") || request.CorrelationID != text(claim, "correlation_id") || digest(request.Input) != text(claim, "input_digest") || continuation.Frozen.AcceptedOperation.ContractDigest != text(claim, "contract_digest") {
			addIssue(report, "connector_confirmation_continuations", "frozen_claim_mismatch")
		} else {
			report.VerifiedEnvelopes++
		}
		continuation.Frozen.Request.Input = nil
	}
}

func uuidValue(record row, field string) uuid.UUID {
	id, _ := uuid.Parse(text(record, field))
	return id
}
