// Package connectorfinalize owns the bounded, offline retirement of the former
// Connector schema. It is never a dependency of a request handler or provider.
package connectorfinalize

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

const ReportVersion = 2

// RetiredTables returns a copy: callers cannot enlarge the retirement capability.
func RetiredTables() []string {
	return []string{
		"aichat_integration_preferences",
		"connector_execution_route_claims", "connector_legacy_identity_mappings",
		"connector_migration_inventory", "connector_mutation_journal", "connector_provider_rollouts",
		"connector_reversal_baselines", "connector_rollout_evidence", "connector_shadow_observations",
		"integration_action_policies", "integration_connection_grants", "integration_connection_health_events",
		"integration_connections", "integration_executions", "integration_oauth_client_configs",
		"integration_oauth_flows", "integration_oauth_recovery_operations", "integration_oauth_states",
		"integration_operation_receipts", "integration_provider_authorizations",
		"integration_provider_install_flows", "integration_provider_suite_states",
	}
}

func NativeTables() []string {
	return []string{
		"connector_chat_preference_mutations", "connector_chat_preferences",
		"connector_confirmation_continuations", "connector_connection_provider_evidence",
		"connector_connections", "connector_conversion_records", "connector_definition_versions",
		"connector_definitions", "connector_invocation_claims", "connector_key_setup_attempts",
		"connector_lifecycle_decisions", "connector_messaging_contexts", "connector_messaging_states",
		"connector_onboarding_flows", "connector_onboarding_secrets", "connector_operation_outcomes",
		"connector_platform_lifecycle_events", "connector_plugin_versions", "connector_schema_readiness",
		"connector_target_bindings", "connector_unavailable_references",
	}
}

type TableProof struct {
	Table  string `json:"table"`
	Rows   int    `json:"rows"`
	Digest string `json:"digest"`
}

type Issue struct {
	Table string `json:"table"`
	Code  string `json:"code"`
}

// Report contains counts and commitments, never rows, credentials or user text.
type Report struct {
	Version                 int          `json:"version"`
	DatabaseFingerprint     string       `json:"database_fingerprint"`
	SchemaVersion           int          `json:"schema_version"`
	State                   string       `json:"state"`
	Checkpoint              string       `json:"checkpoint"`
	SchemaDigest            string       `json:"schema_digest"`
	Source                  []TableProof `json:"source"`
	Native                  []TableProof `json:"native"`
	Hosts                   []TableProof `json:"hosts"`
	Issues                  []Issue      `json:"issues"`
	DigestUnavailableClaims int          `json:"digest_unavailable_claims"`
	VerifiedEnvelopes       int          `json:"verified_envelopes"`
	CatalogTransitions      int          `json:"catalog_transitions"`
	HostTransitions         int          `json:"host_transitions"`
	Digest                  string       `json:"digest"`
}

func (r *Report) seal() {
	r.Digest = ""
	sort.Slice(r.Issues, func(i, j int) bool {
		if r.Issues[i].Table == r.Issues[j].Table {
			return r.Issues[i].Code < r.Issues[j].Code
		}
		return r.Issues[i].Table < r.Issues[j].Table
	})
	r.Digest = digest(r)
}

func digest(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic("retirement commitment requires JSON data")
	}
	value := sha256.Sum256(encoded)
	return hex.EncodeToString(value[:])
}

type row map[string]any
type snapshot map[string][]row

func text(r row, key string) string { value, _ := r[key].(string); return value }
func number(r row, key string) int {
	switch value := r[key].(type) {
	case json.Number:
		number, err := strconv.Atoi(string(value))
		if err == nil {
			return number
		}
	case float64:
		return int(value)
	case int:
		return value
	}
	return 0
}
func find(rows []row, key, value string) row {
	for _, r := range rows {
		if text(r, key) == value {
			return r
		}
	}
	return nil
}
