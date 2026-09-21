package connectorfinalize

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestRetirementCapabilityIsFixedAndDisjoint(t *testing.T) {
	retired := RetiredTables()
	native := NativeTables()
	if len(retired) != 22 || len(native) != 22 {
		t.Fatal("retirement manifest changed")
	}
	seen := map[string]bool{}
	for _, table := range append(retired, native...) {
		if seen[table] {
			t.Fatal("duplicate or overlapping table", table)
		}
		seen[table] = true
	}
	retired[0] = "connector_connections"
	if RetiredTables()[0] == "connector_connections" {
		t.Fatal("caller mutated retirement capability")
	}
}

func TestSourceProofRejectsUncoveredOrConflictingClaim(t *testing.T) {
	org, owner, receipt := uuid.NewString(), uuid.NewString(), uuid.NewString()
	source := row{"receipt_id": receipt, "organization_id": org, "owner_account_id": owner, "acting_principal_id": owner, "target_type": "chat", "target_id": "chat-1", "connector_id": "github", "operation_id": "github.user.get", "binding_id": uuid.NewString(), "connection_id": uuid.NewString(), "logical_operation_id": "operation-1", "correlation_id": "turn-1"}
	claim := row{}
	for k, v := range source {
		claim[k] = v
	}
	claim["state"] = "unknown"
	claim["target_scope"] = "live"
	data := snapshot{"connector_execution_route_claims": {source}, "connector_invocation_claims": {claim}}
	r := Report{}
	validateSource(data, &r)
	if len(r.Issues) != 0 || r.DigestUnavailableClaims != 1 {
		t.Fatalf("terminal barrier was not preserved: %#v", r)
	}
	claim["connection_id"] = uuid.NewString()
	r = Report{}
	validateSource(data, &r)
	if len(r.Issues) == 0 {
		t.Fatal("matching receipt alone authorized a different connection")
	}
	delete(data, "connector_invocation_claims")
	r = Report{}
	validateSource(data, &r)
	if len(r.Issues) == 0 {
		t.Fatal("missing permanent barrier authorized retirement")
	}
}

func TestSourceProofDoesNotAcceptDigestUnavailablePending(t *testing.T) {
	source := row{"receipt_id": "receipt", "target_type": "chat"}
	claim := row{"receipt_id": "receipt", "target_type": "chat", "target_scope": "live", "state": "awaiting_confirmation"}
	r := Report{}
	validateSource(snapshot{"connector_execution_route_claims": {source}, "connector_invocation_claims": {claim}}, &r)
	if len(r.Issues) == 0 {
		t.Fatal("pending operation accepted without a frozen input proof")
	}
}

func TestConstraintProofPreservesAudience(t *testing.T) {
	owner := uuid.New()
	constraint := core.OwnerConstraint(owner, []core.Identifier{"github.user.get"})
	encoded, _ := json.Marshal(constraint)
	var value any
	_ = json.Unmarshal(encoded, &value)
	if !validConstraint(value, owner.String()) || validConstraint(value, uuid.NewString()) || validConstraint(nil, owner.String()) {
		t.Fatal("constraint audience proof invalid")
	}
}
