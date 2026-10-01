package dispatch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestActionInstanceSurvivesRecoveryAndCallRename(t *testing.T) {
	owner := uuid.New()
	identity := Identity{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github", OperationID: "github.issue.create", Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, LogicalOperationID: "lookup", CorrelationID: "turn"}
	ledger := ActionLedger{}
	first, created, err := ResolveActionInstance(&ledger, "server-step", "user-turn", identity, map[string]any{"body": "secret-body", "n": 1})
	if err != nil || !created {
		t.Fatalf("first allocation: created=%v err=%v", created, err)
	}
	encoded, _ := json.Marshal(ledger)
	if strings.Contains(string(encoded), "secret-body") {
		t.Fatal("host ledger retained operation input")
	}
	var restored ActionLedger
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	identity.LogicalOperationID = "another-model-call"
	retry, created, err := ResolveActionInstance(&restored, "server-step", "user-turn", identity, map[string]any{"n": 1.0, "body": "secret-body"})
	if err != nil || created || retry.LogicalOperationID != first.LogicalOperationID || retry.CorrelationID != first.CorrelationID {
		t.Fatalf("retry allocated a different action: created=%v err=%v", created, err)
	}
	if string(encoded) == "" || len(restored.Instances) != 1 {
		t.Fatal("missing durable instance")
	}
	changedBinding := identity
	changedBinding.BindingID = uuid.New()
	if _, _, err := ResolveActionInstance(&restored, "server-step", "user-turn", changedBinding, map[string]any{"body": "secret-body", "n": 1}); err == nil {
		t.Fatal("changed binding reused the action")
	}
	next, created, err := ResolveActionInstance(&restored, "different-server-step", "user-turn", identity, map[string]any{"body": "secret-body", "n": 1})
	if err != nil || !created || next.LogicalOperationID == first.LogicalOperationID {
		t.Fatal("distinct accepted server step could not allocate an action")
	}
	restored.Blocked = true
	if _, _, err := ResolveActionInstance(&restored, "new-step", "user-turn", identity, nil); err == nil {
		t.Fatal("unrecoverable host ledger authorized dispatch")
	}
}

func TestActionInstanceRejectsCorruptionOutsideRequestedEntry(t *testing.T) {
	owner := uuid.New()
	identity := Identity{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github", OperationID: "github.issue.create", Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, LogicalOperationID: "lookup", CorrelationID: "turn"}
	for _, corruption := range []string{"key", "owner", "logical", "digest", "duplicate", "version"} {
		t.Run(corruption, func(t *testing.T) {
			ledger := ActionLedger{}
			if _, _, err := ResolveActionInstance(&ledger, "first", "turn", identity, nil); err != nil {
				t.Fatal(err)
			}
			for key, entry := range ledger.Instances {
				switch corruption {
				case "key":
					delete(ledger.Instances, key)
					key = "corrupt"
				case "owner":
					entry.OwnerAccountID = uuid.New()
				case "logical":
					entry.LogicalOperationID = "replacement"
				case "digest":
					entry.InputDigest = strings.Repeat("z", 64)
				case "duplicate":
					duplicate := entry
					duplicate.StepID = "duplicate"
					ledger.Instances[actionInstanceKey(duplicate.StepID, duplicate.ConnectorID, duplicate.OperationID, duplicate.Target, duplicate.InputDigest)] = duplicate
				case "version":
					ledger.Version = 0
				}
				ledger.Instances[key] = entry
				break
			}
			if _, _, err := ResolveActionInstance(&ledger, "new-step", "turn", identity, nil); err == nil {
				t.Fatal("corrupted ledger allocated another operation")
			}
		})
	}
}
