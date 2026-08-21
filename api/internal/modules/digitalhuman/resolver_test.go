package digitalhuman

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/datatypes"
)

type resolverStoreStub struct {
	definition  *Definition
	version     *Version
	assignments []*RoleAssignment
	role        *RoleTemplateVersion
	bindings    []resourcebindings.Binding
}

func (s *resolverStoreStub) RuntimePublishedVersion(context.Context, runtimeidentity.SubjectRef, *uuid.UUID) (*Definition, *Version, error) {
	return s.definition, s.version, nil
}

func (s *resolverStoreStub) ListRoleAssignments(context.Context, Scope, uuid.UUID, uuid.UUID) ([]*RoleAssignment, error) {
	return s.assignments, nil
}

func (s *resolverStoreStub) PublishedRoleTemplateVersion(context.Context, Scope, uuid.UUID) (*RoleTemplateVersion, error) {
	return s.role, nil
}

func (s *resolverStoreStub) ListSnapshot(context.Context, resourcebindings.SnapshotRef) ([]resourcebindings.Binding, error) {
	return s.bindings, nil
}

func (s *resolverStoreStub) Verify(context.Context, resourcebindings.SnapshotRef, resourcebindings.Match) (resourcebindings.Decision, error) {
	return resourcebindings.Decision{}, nil
}

func TestResolverSealsPinnedMemoryFreeExecution(t *testing.T) {
	subject := runtimeidentity.SubjectRef{
		Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
	}
	versionID, roleTemplateID, roleVersionID := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	store := &resolverStoreStub{
		definition: &Definition{ID: subject.ID, Name: "Atlas", Description: "Research partner", Status: StatusActive},
		version: &Version{
			ID: versionID, DigitalHumanID: subject.ID, State: VersionStatePublished, VersionNumber: 3,
			Persona: "Be evidence-led", Rules: datatypes.JSON(`["Cite sources"]`), DefaultModel: "gpt-5",
			TriggerPolicy: datatypes.JSON(`{"channel_response":{"intermediate_messages_enabled":false,"acknowledgement_enabled":true}}`),
			Revision:      "sha256:identity",
		},
		assignments: []*RoleAssignment{{
			ID: uuid.New(), DigitalHumanID: subject.ID, DigitalHumanVersionID: versionID,
			RoleTemplateID: roleTemplateID, RoleTemplateVersionID: roleVersionID, Revision: "sha256:assignment",
		}},
		role: &RoleTemplateVersion{
			ID: roleVersionID, RoleTemplateID: roleTemplateID, State: VersionStatePublished, VersionNumber: 2,
			Revision: "sha256:role", Capabilities: datatypes.JSON(`[{"kind":"capability","id":"web-search","version":"1"}]`),
			SOPRefs: datatypes.JSON(`[{"kind":"sop","id":"research","revision":"sop-r1"}]`), EvaluationRefs: datatypes.JSON(`[]`),
		},
		bindings: []resourcebindings.Binding{{
			ID: uuid.New(), Type: resourcebindings.BindingTypeIntegrationConnection,
			ResourceID: "connection-1", ParentResourceID: "feishu", AccessMode: resourcebindings.AccessModeExecute,
			Revision: "sha256:binding", CredentialRef: "credential/feishu-primary",
		}},
	}
	resolver := NewResolver(store)
	resolver.now = func() time.Time { return now }
	event := invocation.Event{
		ID: "manual-1", Type: invocation.EventTypeManual, Origin: invocation.Origin{Kind: invocation.OriginManual},
		Target: subject, Sender: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: uuid.NewString()}, OccurredAt: now,
		Context: map[string]any{"message": "research current risks"},
	}

	resolved, err := resolver.Resolve(t.Context(), event, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Spec.Subject != subject || resolved.Spec.Identity.ID != versionID.String() || resolved.Spec.Model.ID != "gpt-5" {
		t.Fatalf("RunSpec = %#v", resolved.Spec)
	}
	if len(resolved.Spec.Roles) != 1 || len(resolved.Spec.Capabilities) != 1 || len(resolved.Spec.SOPs) != 1 || len(resolved.Spec.Bindings) != 1 {
		t.Fatalf("RunSpec references = %#v", resolved.Spec)
	}
	if resolved.Snapshot.UseUserMemory || resolved.Snapshot.UseAgentMemory || resolved.Snapshot.Revision == "" {
		t.Fatalf("execution snapshot memory/revision = %#v", resolved.Snapshot)
	}
	if resolved.Snapshot.ChannelResponsePolicy == nil || resolved.Snapshot.ChannelResponsePolicy.IntermediateMessagesEnabled ||
		!resolved.Snapshot.ChannelResponsePolicy.AcknowledgementEnabled {
		t.Fatalf("execution snapshot channel policy = %#v", resolved.Snapshot.ChannelResponsePolicy)
	}
	if _, err := runspec.Seal(resolved.Spec); err != nil {
		t.Fatalf("sealed RunSpec cannot be verified: %v", err)
	}
	encoded, err := json.Marshal(resolved)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(encoded)
	if !strings.Contains(serialized, "credential/feishu-primary") {
		t.Fatalf("credential reference missing: %s", serialized)
	}
	for _, forbidden := range []string{`"password"`, `"access_token"`, `"api_key"`, `"agent_id"`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("resolved execution contains forbidden field %s: %s", forbidden, serialized)
		}
	}

	store.role.Capabilities[0] = 'x'
	if string(resolved.Snapshot.Roles[0].Version.Capabilities)[0] == 'x' {
		t.Fatal("resolved snapshot aliases mutable role storage")
	}
}

func TestResolverRejectsNonDigitalHumanTarget(t *testing.T) {
	resolver := NewResolver(&resolverStoreStub{})
	now := time.Now().UTC()
	event := invocation.Event{
		ID: "manual-agent", Type: invocation.EventTypeManual, Origin: invocation.Origin{Kind: invocation.OriginManual},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeAgent, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: uuid.NewString()}, OccurredAt: now,
	}
	if _, err := resolver.Resolve(t.Context(), event, nil); err == nil {
		t.Fatal("Resolve(Agent) error = nil")
	}
}
