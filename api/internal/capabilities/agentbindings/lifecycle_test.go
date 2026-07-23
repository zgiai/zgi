package agentbindings

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPruneAgentModeResourceRemovesDatabaseTableReadAndWrite(t *testing.T) {
	raw := `{"database_bindings":[{"data_source_id":"db-1","table_ids":["table-1","table-2"],"writable_table_ids":["table-1","table-2"]}],"home_title":"kept"}`
	updated, changed, err := pruneAgentModeResource(&raw, ResourceRef{
		BindingType:      BindingTypeDatabaseTable,
		ResourceID:       "table-1",
		ParentResourceID: "db-1",
	})
	if err != nil {
		t.Fatalf("pruneAgentModeResource() error = %v", err)
	}
	if !changed {
		t.Fatal("pruneAgentModeResource() changed = false, want true")
	}
	var mode map[string]interface{}
	if err := json.Unmarshal([]byte(updated), &mode); err != nil {
		t.Fatalf("decode updated mode: %v", err)
	}
	bindings := mode["database_bindings"].([]interface{})
	binding := bindings[0].(map[string]interface{})
	if got := binding["table_ids"].([]interface{}); len(got) != 1 || got[0] != "table-2" {
		t.Fatalf("table_ids = %#v, want table-2", got)
	}
	if got := binding["writable_table_ids"].([]interface{}); len(got) != 1 || got[0] != "table-2" {
		t.Fatalf("writable_table_ids = %#v, want table-2", got)
	}
	if got := mode["home_title"]; got != "kept" {
		t.Fatalf("home_title = %#v, want kept", got)
	}
}

func TestPruneAgentModeResourceRemovesEmptyDatabaseBinding(t *testing.T) {
	raw := `{"database_bindings":[{"data_source_id":"db-1","table_ids":["table-1"],"writable_table_ids":["table-1"]}]}`
	updated, changed, err := pruneAgentModeResource(&raw, ResourceRef{
		BindingType:      BindingTypeDatabaseTable,
		ResourceID:       "table-1",
		ParentResourceID: "db-1",
	})
	if err != nil || !changed {
		t.Fatalf("pruneAgentModeResource() changed = %v, error = %v", changed, err)
	}
	var mode map[string]interface{}
	if err := json.Unmarshal([]byte(updated), &mode); err != nil {
		t.Fatalf("decode updated mode: %v", err)
	}
	if got := mode["database_bindings"].([]interface{}); len(got) != 0 {
		t.Fatalf("database_bindings = %#v, want empty", got)
	}
}

func TestPruneAgentModeResourceRemovesWorkflowByTargetAgentID(t *testing.T) {
	raw := `{"workflow_bindings":[{"binding_id":"binding-1","workflow_id":"workflow-1","agent_id":"target-agent-1"},{"binding_id":"binding-2","workflow_id":"workflow-2","agent_id":"target-agent-2"}]}`
	updated, changed, err := pruneAgentModeResource(&raw, ResourceRef{
		BindingType: BindingTypeWorkflow,
		ResourceID:  "target-agent-1",
	})
	if err != nil || !changed {
		t.Fatalf("pruneAgentModeResource() changed = %v, error = %v", changed, err)
	}
	var mode map[string]interface{}
	if err := json.Unmarshal([]byte(updated), &mode); err != nil {
		t.Fatalf("decode updated mode: %v", err)
	}
	bindings := mode["workflow_bindings"].([]interface{})
	if len(bindings) != 1 || bindings[0].(map[string]interface{})["agent_id"] != "target-agent-2" {
		t.Fatalf("workflow_bindings = %#v, want only target-agent-2", bindings)
	}
}

func TestImpactTokenRejectsExpiredAndChangedImpact(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := []byte("shared-test-secret")
	actorID := uuid.New()
	ref := ResourceRef{BindingType: BindingTypeSkill, ResourceID: "calculator"}
	payload := impactTokenPayload{
		ActorID:     actorID.String(),
		Operation:   "delete",
		BindingType: ref.BindingType,
		ResourceID:  ref.ResourceID,
		Revision:    bindingImpactRevision(nil),
		ExpiresAt:   now.Add(ImpactTokenTTL).Unix(),
	}
	token, err := encodeImpactToken(secret, payload)
	if err != nil {
		t.Fatalf("encodeImpactToken() error = %v", err)
	}
	decoded, err := decodeImpactToken(secret, token)
	if err != nil {
		t.Fatalf("decodeImpactToken() error = %v", err)
	}
	if decoded.ActorID != actorID.String() || decoded.Revision != payload.Revision {
		t.Fatalf("decoded payload = %#v, want actor and revision preserved", decoded)
	}
	decoded.ExpiresAt = now.Add(-time.Second).Unix()
	expired, err := encodeImpactToken(secret, decoded)
	if err != nil {
		t.Fatalf("encode expired token: %v", err)
	}
	decodedExpired, err := decodeImpactToken(secret, expired)
	if err != nil {
		t.Fatalf("decode expired token: %v", err)
	}
	if decodedExpired.ExpiresAt >= now.Unix() {
		t.Fatal("expired token unexpectedly valid at test time")
	}
	if _, err := decodeImpactToken(secret, token+"broken"); !errors.Is(err, ErrImpactTokenInvalid) {
		t.Fatalf("decodeImpactToken(tampered) error = %v, want ErrImpactTokenInvalid", err)
	}
	if _, err := decodeImpactToken([]byte("another-secret"), token); !errors.Is(err, ErrImpactTokenInvalid) {
		t.Fatalf("decodeImpactToken(wrong secret) error = %v, want ErrImpactTokenInvalid", err)
	}
}
