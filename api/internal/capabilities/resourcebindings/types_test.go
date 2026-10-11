package resourcebindings

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

type agentReaderStub struct {
	rows []agentbindings.Binding
}

func (s agentReaderStub) ListScope(context.Context, agentbindings.ScopeRef) ([]agentbindings.Binding, error) {
	return s.rows, nil
}

func (s agentReaderStub) FindBinding(_ context.Context, _ agentbindings.ScopeRef, match agentbindings.Match) (*agentbindings.Binding, error) {
	for i := range s.rows {
		row := &s.rows[i]
		if row.BindingType == match.BindingType && row.ResourceID == match.ResourceID && row.ParentResourceID == match.ParentResourceID && AllowsAccess(AccessMode(row.AccessMode), AccessMode(match.AccessMode)) {
			return row, nil
		}
	}
	return nil, nil
}

func TestAgentAdapterPreservesAgentStoreProjection(t *testing.T) {
	agentID := uuid.New()
	versionID := uuid.New()
	ref := SnapshotRef{Subject: runtimeidentity.SubjectRef{
		Type: runtimeidentity.SubjectTypeAgent, ID: agentID, OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
	}, Phase: PhasePublished, VersionID: &versionID}
	adapter := NewAgentAdapter(agentReaderStub{rows: []agentbindings.Binding{{
		ID: uuid.New(), AgentID: agentID, BindingScope: agentbindings.ScopePublished,
		BindingType: agentbindings.BindingTypeSkill, ResourceID: "skill-1",
		AccessMode: "execute", Metadata: map[string]interface{}{"source": "agent"},
	}}})

	bindings, err := adapter.ListSnapshot(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Snapshot.Subject.Type != runtimeidentity.SubjectTypeAgent || bindings[0].Revision == "" {
		t.Fatalf("bindings = %#v", bindings)
	}
	decision, err := adapter.Verify(t.Context(), ref, Match{
		Type: BindingTypeSkill, ResourceID: "skill-1", AccessMode: AccessModeExecute,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("Verify(allowed) = %#v, %v", decision, err)
	}
	denied, err := adapter.Verify(t.Context(), ref, Match{
		Type: BindingTypeSkill, ResourceID: "skill-2", AccessMode: AccessModeExecute,
	})
	if err != nil || denied.Allowed || denied.Reason != "resource_not_bound" {
		t.Fatalf("Verify(denied) = %#v, %v", denied, err)
	}
}

func TestValidateSnapshotRefRequiresPinnedPublishedVersion(t *testing.T) {
	ref := SnapshotRef{Subject: runtimeidentity.SubjectRef{
		Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
	}, Phase: PhasePublished}
	if _, err := ValidateSnapshotRef(ref); err == nil {
		t.Fatal("ValidateSnapshotRef() error = nil")
	}
}
