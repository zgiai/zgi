package container

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

func TestHostReferenceRequiresExactPersistedSelection(t *testing.T) {
	for _, surface := range []core.TargetType{core.TargetTypeAgent, core.TargetTypeWorkflow} {
		for _, scope := range []core.TargetScope{core.ScopeDraft, core.ScopePublished} {
			for _, change := range []string{"unchanged", "binding", "connection", "connector", "removed", "duplicate", "invalid-json", "operation", "unavailable", "provider"} {
				t.Run(string(surface)+"/"+string(scope)+"/"+change, func(t *testing.T) {
					b := core.TargetBinding{ID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github", Target: core.TargetRef{Type: surface, ID: uuid.NewString(), Scope: scope, BindingInstanceID: "node"}}
					ref := map[string]any{"binding_id": b.ID.String(), "connection_id": b.ConnectionID.String(), "connector_id": "github", "operation_id": "github.user.get"}
					request := execution.TargetAuthorizationRequest{Binding: b, OperationID: "github.user.get"}
					switch change {
					case "binding":
						ref["binding_id"] = uuid.NewString()
					case "connection":
						ref["connection_id"] = uuid.NewString()
					case "connector":
						ref["connector_id"] = "exa"
					case "operation":
						ref["operation_id"] = "github.repo.list"
					}
					entries := []any{ref}
					data := map[string]any{"provider_type": "connector", "provider_id": "github", "connector": ref}
					if change == "unavailable" {
						data["connector_unavailable_ref"] = uuid.NewString()
					}
					if change == "provider" {
						data["provider_type"] = "builtin"
					}
					if surface == core.TargetTypeWorkflow {
						entries = []any{map[string]any{"id": "node", "data": data}}
					}
					if change == "removed" {
						entries = nil
					}
					if change == "duplicate" {
						entries = append(entries, entries[0])
					}
					key := "connector_bindings"
					if surface == core.TargetTypeWorkflow {
						key = "nodes"
					}
					raw, _ := json.Marshal(map[string]any{key: entries})
					if change == "invalid-json" {
						raw = []byte("{")
					}
					want := change == "unchanged" || surface == core.TargetTypeAgent && (change == "operation" || change == "unavailable" || change == "provider")
					if got := hostDocumentSelectsBinding(raw, request); got != want {
						t.Fatalf("accepted=%t want=%t", got, want)
					}
				})
			}
		}
	}
}
