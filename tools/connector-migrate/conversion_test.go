package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const orgID = "11111111-1111-4111-8111-111111111111"
const ownerID = "22222222-2222-4222-8222-222222222222"
const connID = "33333333-3333-4333-8333-333333333333"
const agentID = "44444444-4444-4444-8444-444444444444"
const bindingID = "55555555-5555-4555-8555-555555555555"
const workspaceID = "66666666-6666-4666-8666-666666666666"
const receiptID = "77777777-7777-4777-8777-777777777777"
const secretMarker = "SYNTHETIC-SECRET-NOT-IN-REPORT"

func testKeys() keyrings {
	return keyrings{Active: "new.key", Native: map[string]string{"new.key": strings.Repeat("n", 32)}, Previous: map[string]string{"old": strings.Repeat("o", 32)}}
}
func oldEnvelope(t *testing.T, version string, s secretSubject, data []byte) string {
	t.Helper()
	keys := testKeys()
	aead, err := newAEAD(keys.Previous["old"], false)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}
	payload := aead.Seal(nonce, nonce, data, previousAAD(s, version, "old"))
	prefix := version + "."
	if version == "v2" {
		prefix += "old."
	}
	return prefix + base64.RawURLEncoding.EncodeToString(payload)
}
func TestPreviousFormatsConvertWithPurposeAndIdentityIsolation(t *testing.T) {
	subject := secretSubject{orgID, connID, "github", "connection-key", 3}
	plaintext := []byte(`{"token":"` + secretMarker + `"}`)
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			envelope := oldEnvelope(t, version, subject, plaintext)
			decoded, err := decryptEnvelope(testKeys(), subject, envelope)
			if err != nil || string(decoded) != string(plaintext) {
				t.Fatalf("old format conversion failed: %v", err)
			}
			native, err := encryptNative(testKeys(), subject, decoded)
			clear(decoded)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(native, secretMarker) {
				t.Fatal("secret leaked into envelope")
			}
			opened, err := decryptEnvelope(testKeys(), subject, native)
			if err != nil || string(opened) != string(plaintext) {
				t.Fatal("native roundtrip failed")
			}
			clear(opened)
			for _, alter := range []func(*secretSubject){func(s *secretSubject) { s.Org = ownerID }, func(s *secretSubject) { s.ID = ownerID }, func(s *secretSubject) { s.Connector = "web-search" }, func(s *secretSubject) { s.Version++ }, func(s *secretSubject) { s.Purpose = "confirmation-continuation" }} {
				wrong := subject
				alter(&wrong)
				if data, err := decryptEnvelope(testKeys(), wrong, native); err == nil {
					clear(data)
					t.Fatal("cross identity or purpose envelope accepted")
				}
			}
		})
	}
}
func fixture(t *testing.T) databaseSnapshot {
	subject := secretSubject{orgID, connID, "github", "connection-key", 1}
	s := databaseSnapshot{
		"connector_schema_readiness": {row{"id": 1, "schema_version": 1, "state": "conversion_required"}},
		"integration_connections":    {row{"id": connID, "organization_id": orgID, "owner_account_id": ownerID, "integration_id": "github", "driver_id": "github-rest", "credential_source": "account", "auth_type": "api_key", "auth_method_id": "personal_access_token", "name": "Personal", "status": "active", "auth_status": "valid", "health_status": "healthy", "scope_status": "verified", "credential_version": 1, "revision": 4, "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z", "config": row{}, "encrypted_credentials": oldEnvelope(t, "v2", subject, []byte(`{"token":"`+secretMarker+`"}`))}},
		"agent_resource_bindings":    {row{"id": bindingID, "agent_id": agentID, "consumer_id": agentID, "consumer_type": "agent", "binding_scope": "draft", "organization_id": orgID, "workspace_id": workspaceID, "binding_type": "integration_connection", "resource_id": connID, "parent_resource_id": "github", "authorized_by": ownerID, "metadata": row{"allowed_action_ids": []string{"github.user.get"}}}},
		"agents":                     {row{"id": agentID, "tenant_id": workspaceID}}, "workspaces": {row{"id": workspaceID, "organization_id": orgID}},
		"agents_configs": {row{"id": "88888888-8888-4888-8888-888888888888", "agents_id": agentID, "created_by": ownerID, "agent_mode": row{"integration_bindings": []any{row{"integration_id": "github", "connection_id": connID, "action_ids": []string{"github.user.get"}}}}}},
	}
	return s
}
func applyMemory(t *testing.T, source databaseSnapshot, plan conversionPlan) databaseSnapshot {
	t.Helper()
	encoded, _ := json.Marshal(source)
	var result databaseSnapshot
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	for _, m := range plan.Mutations {
		if m.Insert {
			result[m.Table] = append(result[m.Table], clone(m.Values))
			continue
		}
		found := false
		for _, record := range result[m.Table] {
			matches := true
			for key, value := range m.Key {
				if hash(record[key]) != hash(value) {
					matches = false
				}
			}
			if matches {
				for key, value := range m.Values {
					record[key] = value
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("update target missing in %s", m.Table)
		}
	}
	return result
}
func TestConversionPreservesConstraintsAndHasNoSecretReportOrReplay(t *testing.T) {
	source := fixture(t)
	before := hash(source)
	plan, err := makePlan(source, testKeys())
	if err != nil || len(plan.Report.Issues) > 0 {
		t.Fatalf("conversion plan failed: %v %#v", err, plan.Report.Issues)
	}
	if hash(source) != before {
		t.Fatal("dry-run changed its source")
	}
	data, _ := json.Marshal(plan.Report)
	if strings.Contains(string(data), secretMarker) || strings.Contains(string(data), "encrypted_credentials") {
		t.Fatal("report leaked credential material")
	}
	converted := applyMemory(t, source, plan)
	r := verifySnapshot(converted, testKeys())
	if len(r.Issues) > 0 {
		t.Fatalf("postconversion verification failed: %#v", r.Issues)
	}
	bindings := converted["connector_target_bindings"]
	if len(bindings) != 1 {
		t.Fatalf("binding count: %d", len(bindings))
	}
	ops := stringsOf(object(bindings[0]["binding_constraint"])["allowed_operation_ids"])
	if len(ops) != 1 || ops[0] != "github.user.get" {
		t.Fatal("binding restriction widened")
	}
	converted["connector_schema_readiness"][0]["state"] = "ready"
	again, err := makePlan(converted, testKeys())
	if err != nil || len(again.Mutations) != 0 || len(again.Report.Issues) > 0 {
		t.Fatalf("conversion not idempotent: %v %#v", err, again.Report.Issues)
	}
	// A subsequent ordinary host binding is independent of the retired archive.
	converted["agent_resource_bindings"] = append(converted["agent_resource_bindings"], row{"binding_type": "skill", "id": "ordinary"})
	if r := verifySnapshot(converted, testKeys()); len(r.Issues) > 0 {
		t.Fatalf("ordinary resource changes affected conversion: %#v", r.Issues)
	}
}
func TestOrganizationAndOAuthReferencesStayUnavailable(t *testing.T) {
	for _, kind := range []string{"organization", "oauth2"} {
		t.Run(kind, func(t *testing.T) {
			source := fixture(t)
			connection := source["integration_connections"][0]
			if kind == "organization" {
				connection["credential_source"] = "organization"
				connection["owner_account_id"] = nil
			} else {
				connection["auth_type"] = "oauth2"
			}
			plan, err := makePlan(source, testKeys())
			if err != nil || len(plan.Report.Issues) > 0 {
				t.Fatalf("plan: %v %#v", err, plan.Report.Issues)
			}
			converted := applyMemory(t, source, plan)
			if len(converted["connector_connections"]) != 0 || len(converted["connector_target_bindings"]) != 0 || len(converted["connector_unavailable_references"]) == 0 {
				t.Fatal("unsupported source became executable")
			}
		})
	}
}
func TestUnknownClaimIsAPermanentBarrier(t *testing.T) {
	source := fixture(t)
	source["connector_execution_route_claims"] = []row{{"receipt_id": receiptID, "organization_id": orgID, "owner_account_id": ownerID, "acting_principal_id": ownerID, "binding_id": bindingID, "connection_id": connID, "connector_id": "github", "target_type": "agent", "target_id": agentID, "operation_id": "github.issue.create", "logical_operation_id": "original-operation", "correlation_id": "original-correlation", "path": "connector_native", "created_at": "2026-09-01T00:00:00Z"}}
	source["connector_operation_outcomes"] = []row{{"organization_id": orgID, "logical_operation_id": "original-operation", "status": "unknown"}}
	plan, err := makePlan(source, testKeys())
	if err != nil {
		t.Fatal(err)
	}
	converted := applyMemory(t, source, plan)
	claim := converted["connector_invocation_claims"][0]
	if text(claim, "receipt_id") != receiptID || text(claim, "state") != "unknown" || text(claim, "logical_operation_id") != "original-operation" {
		t.Fatal("unknown claim lost its identity or replay barrier")
	}
}

type rejectingTx struct{ pgx.Tx }

func (rejectingTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New(secretMarker)
}
func TestWriteFailureDoesNotExposeDriverDetails(t *testing.T) {
	err := applyPlan(context.Background(), rejectingTx{}, conversionPlan{Mutations: []mutation{{Table: "connector_connections", Insert: true, Values: row{"id": connID, "encrypted_credentials": secretMarker}}}})
	if err == nil || strings.Contains(err.Error(), secretMarker) {
		t.Fatal("database failure leaked secrets")
	}
}
