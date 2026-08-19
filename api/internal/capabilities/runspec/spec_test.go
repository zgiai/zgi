package runspec

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

func TestSealIsDeterministicAndOwnsSlices(t *testing.T) {
	t.Parallel()

	first := validSpec()
	second := validSpec()
	second.Roles[0], second.Roles[1] = second.Roles[1], second.Roles[0]
	second.Bindings[0], second.Bindings[1] = second.Bindings[1], second.Bindings[0]

	sealedFirst, err := Seal(first)
	if err != nil {
		t.Fatalf("Seal(first) error = %v", err)
	}
	sealedSecond, err := Seal(second)
	if err != nil {
		t.Fatalf("Seal(second) error = %v", err)
	}
	if sealedFirst.Revision != sealedSecond.Revision {
		t.Fatalf("revisions differ: %q != %q", sealedFirst.Revision, sealedSecond.Revision)
	}

	first.Roles[0].ID = "mutated"
	if sealedFirst.Roles[0].ID == "mutated" {
		t.Fatal("sealed spec aliases caller-owned role slice")
	}
}

func TestSealJSONRoundTrip(t *testing.T) {
	t.Parallel()

	want, err := Seal(validSpec())
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded Spec
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	got, err := Seal(decoded)
	if err != nil {
		t.Fatalf("Seal(decoded) error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestSpecSerializesCredentialReferencesWithoutSecretFields(t *testing.T) {
	t.Parallel()

	sealed, err := Seal(validSpec())
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	encoded, err := json.Marshal(sealed)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	serialized := string(encoded)
	if !strings.Contains(serialized, `"credential_ref":"credential/mail-primary"`) {
		t.Fatalf("snapshot does not contain the credential reference: %s", serialized)
	}
	for _, forbidden := range []string{`"secret"`, `"api_key"`, `"password"`, `"access_token"`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("snapshot contains forbidden credential material field %s: %s", forbidden, serialized)
		}
	}
}

func TestSealRejectsInvalidSpecs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Spec)
	}{
		{name: "schema", mutate: func(spec *Spec) { spec.SchemaVersion = "run_spec.v2" }},
		{name: "invocation", mutate: func(spec *Spec) { spec.InvocationID = " " }},
		{name: "subject", mutate: func(spec *Spec) { spec.Subject.ID = uuid.Nil }},
		{name: "identity kind", mutate: func(spec *Spec) { spec.Identity.Kind = "role" }},
		{name: "model kind", mutate: func(spec *Spec) { spec.Model.Kind = "provider" }},
		{name: "version ref", mutate: func(spec *Spec) { spec.Roles[0].Version = "" }},
		{name: "duplicate ref", mutate: func(spec *Spec) { spec.Roles[1] = spec.Roles[0] }},
		{name: "binding", mutate: func(spec *Spec) { spec.Bindings[0].Revision = "" }},
		{name: "access mode", mutate: func(spec *Spec) { spec.Bindings[0].AccessMode = "admin" }},
		{name: "negative budget", mutate: func(spec *Spec) { spec.Budget.MaxToolCalls = -1 }},
		{name: "resolved time", mutate: func(spec *Spec) { spec.ResolvedAt = time.Time{} }},
		{name: "revision mismatch", mutate: func(spec *Spec) { spec.Revision = "sha256:not-the-snapshot" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := validSpec()
			tt.mutate(&spec)
			if _, err := Seal(spec); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("Seal() error = %v, want ErrInvalidSpec", err)
			}
		})
	}
}

func validSpec() Spec {
	return Spec{
		InvocationID: "event-123",
		Subject: runtimeidentity.SubjectRef{
			Type:           runtimeidentity.SubjectTypeDigitalHuman,
			ID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			OrganizationID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			WorkspaceID:    uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		},
		Identity: VersionRef{Kind: "identity", ID: "persona-1", Revision: "identity-r1"},
		Model:    VersionRef{Kind: "model", ID: "gpt-5", Revision: "catalog-r1"},
		Roles: []VersionRef{
			{Kind: "role", ID: "researcher", Version: "1"},
			{Kind: "role", ID: "writer", Version: "2"},
		},
		Capabilities: []VersionRef{{Kind: "capability", ID: "web-search", Version: "1"}},
		SOPs:         []VersionRef{{Kind: "sop", ID: "weekly-review", Revision: "sop-r3"}},
		Bindings: []BindingRef{
			{Type: "dataset", ResourceID: "knowledge", Revision: "7", AccessMode: AccessModeRead},
			{Type: "tool", ResourceID: "mail", Revision: "4", AccessMode: AccessModeExecute, CredentialRef: "credential/mail-primary"},
		},
		Policies:    []VersionRef{{Kind: "policy", ID: "external-action", Revision: "policy-r5"}},
		Evaluations: []VersionRef{{Kind: "evaluation", ID: "quality", Version: "1"}},
		Budget: Budget{
			MaxDurationSeconds: 300,
			MaxModelTokens:     12_000,
			MaxToolCalls:       20,
			MaxExternalActions: 5,
		},
		ResolvedAt: time.Date(2026, time.August, 18, 3, 4, 5, 0, time.FixedZone("CST", 8*60*60)),
	}
}
