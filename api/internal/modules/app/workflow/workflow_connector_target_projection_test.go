package workflow

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func TestWorkflowPublishedProjectionUsesVersionUUID(t *testing.T) {
	org, id, version := uuid.New(), uuid.New(), uuid.New()
	draft := &Workflow{ID: id.String(), Version: "draft"}
	scope, err := workflowConnectorProjectionScope(draft, org)
	if err != nil || scope.TargetID != id || scope.Scope != core.ScopeDraft {
		t.Fatal(scope, err)
	}
	published := &Workflow{ID: uuid.NewString(), Version: "v1", VersionUUID: uuid.NullUUID{UUID: version, Valid: true}}
	scope, err = workflowConnectorProjectionScope(published, org)
	if err != nil || scope.TargetID != version || scope.Scope != core.ScopePublished {
		t.Fatal(scope, err)
	}
	published.VersionUUID.Valid = false
	if _, err := workflowConnectorProjectionScope(published, org); err == nil {
		t.Fatal("versionless published graph accepted")
	}
}
func TestWorkflowPublishedSnapshotRejectsAnyDifferentTuple(t *testing.T) {
	owner, connection := uuid.New(), uuid.New()
	selection := workflowConnectorSelection{NodeID: "n", OwnerID: owner, ConnectionID: connection, ConnectorID: "github", OperationID: "github.user.get"}
	version := uuid.NewString()
	binding := core.TargetBinding{OwnerAccountID: owner, ConnectionID: connection, ConnectorID: "github", Target: core.TargetRef{Type: core.TargetTypeWorkflow, ID: version, VersionID: version, Scope: core.ScopePublished, BindingInstanceID: "n"}, Constraint: core.OwnerConstraint(owner, []core.Identifier{"github.user.get"})}
	if !workflowPublishedBindingsMatch([]workflowConnectorSelection{selection}, []core.TargetBinding{binding}) {
		t.Fatal("valid tuple rejected")
	}
	for _, kind := range []string{"owner", "connection", "connector", "node", "operation"} {
		t.Run(kind, func(t *testing.T) {
			bad := binding
			switch kind {
			case "owner":
				bad.OwnerAccountID = uuid.New()
			case "connection":
				bad.ConnectionID = uuid.New()
			case "connector":
				bad.ConnectorID = "web-search"
			case "node":
				bad.Target.BindingInstanceID = "other"
			case "operation":
				bad.Constraint = core.OwnerConstraint(owner, []core.Identifier{})
			}
			if workflowPublishedBindingsMatch([]workflowConnectorSelection{selection}, []core.TargetBinding{bad}) {
				t.Fatal("mismatched tuple accepted")
			}
		})
	}
}
