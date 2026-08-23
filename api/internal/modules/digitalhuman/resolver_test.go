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
	definition *Definition
	version    *Version
	bindings   []resourcebindings.Binding
}

func (s *resolverStoreStub) RuntimePublishedVersion(context.Context, runtimeidentity.SubjectRef, *uuid.UUID) (*Definition, *Version, error) {
	return s.definition, s.version, nil
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
	versionID := uuid.New()
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	store := &resolverStoreStub{
		definition: &Definition{ID: subject.ID, Name: "Live draft name", Description: "Live draft description", Status: StatusActive},
		version: &Version{
			ID: versionID, DigitalHumanID: subject.ID, State: VersionStatePublished, VersionNumber: 3,
			IdentityName: "Atlas Runtime", Persona: "Be evidence-led", Goals: datatypes.JSON(`["Track research risks"]`), Rules: datatypes.JSON(`["Cite sources"]`), DefaultModel: "gpt-5",
			IdentitySnapshot: datatypes.JSON(`{"name":"Atlas","avatar":"avatar.png","description":"Published research partner","autonomy_level":"supervised","visibility":"workspace"}`),
			TriggerPolicy:    datatypes.JSON(`{"channel_response":{"intermediate_messages_enabled":false,"acknowledgement_enabled":true}}`),
			Revision:         "sha256:identity",
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
	if len(resolved.Spec.Roles) != 0 || len(resolved.Spec.Capabilities) != 0 || len(resolved.Spec.SOPs) != 0 || len(resolved.Spec.Bindings) != 1 {
		t.Fatalf("RunSpec references = %#v", resolved.Spec)
	}
	if resolved.Snapshot.UseUserMemory || resolved.Snapshot.UseAgentMemory || resolved.Snapshot.Revision == "" {
		t.Fatalf("execution snapshot memory/revision = %#v", resolved.Snapshot)
	}
	if resolved.Snapshot.IdentityName != "Atlas Runtime" || len(resolved.Snapshot.Goals) != 1 || resolved.Snapshot.Goals[0] != "Track research risks" {
		t.Fatalf("execution snapshot runtime identity = %#v", resolved.Snapshot)
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

func TestResolverAllowsIdentityWithoutRoleAssignment(t *testing.T) {
	subject := runtimeidentity.SubjectRef{
		Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
	}
	versionID := uuid.New()
	store := &resolverStoreStub{
		definition: &Definition{ID: subject.ID, Name: "Atlas", Status: StatusActive},
		version: &Version{
			ID: versionID, DigitalHumanID: subject.ID, State: VersionStatePublished, VersionNumber: 1,
			Persona: "Be direct", Rules: datatypes.JSON(`["Stay within scope"]`),
			DefaultModel: "gpt-5", IdentitySnapshot: datatypes.JSON(`{"name":"Atlas"}`), Revision: "sha256:version",
		},
	}
	resolver := NewResolver(store)
	event := invocation.Event{
		ID: "manual-identity", Type: invocation.EventTypeManual, Origin: invocation.Origin{Kind: invocation.OriginManual},
		Target: subject, Sender: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: uuid.NewString()}, OccurredAt: time.Now().UTC(),
	}
	resolved, err := resolver.Resolve(t.Context(), event, nil)
	if err != nil {
		t.Fatalf("Resolve(identity without role assignment) error = %v", err)
	}
	if len(resolved.Spec.Roles) != 0 {
		t.Fatalf("resolved roles = %#v", resolved.Spec.Roles)
	}
}
