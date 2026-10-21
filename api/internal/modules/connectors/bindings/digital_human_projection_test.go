package bindings

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestDigitalHumanExplicitEditRejectsOtherSurfacesAndNewBindings(t *testing.T) {
	for _, test := range []struct {
		name     string
		target   core.TargetRef
		revision int
	}{
		{"chat", core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, 1},
		{"agent", core.TargetRef{Type: core.TargetTypeAgent, ID: uuid.NewString()}, 1},
		{"published", core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: uuid.NewString(), Scope: core.ScopePublished, VersionID: uuid.NewString(), BindingInstanceID: uuid.NewString()}, 1},
		{"new", core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: uuid.NewString(), Scope: core.ScopeDraft, BindingInstanceID: uuid.NewString()}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, org := uuid.New(), uuid.New()
			binding := core.TargetBinding{ID: uuid.New(), OrganizationID: org, OwnerAccountID: owner, Target: test.target, Constraint: core.OwnerConstraint(owner, []core.Identifier{"search"})}
			_, err := NewGormRepository(nil).ReplaceDigitalHumanBindingWithLifecycle(t.Context(), org, owner, binding, test.revision, BindingLifecycleMutation{ID: uuid.New(), MutationType: MutationBindingUpserted, CorrelationID: "test-host-only-edit", CreatedAt: time.Now()})
			if !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
				t.Fatalf("host-only edit admitted invalid context: %v", err)
			}
		})
	}
}
