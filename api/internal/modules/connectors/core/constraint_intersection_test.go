package core

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestConstraintIntersectionNeverWidensRestoredAuthority(t *testing.T) {
	owner := uuid.New()
	version := uuid.NewString()
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	current := OwnerConstraint(owner, []Identifier{"github.user.get", "github.issue.create"})
	historical := OwnerConstraint(owner, []Identifier{"github.user.get"})
	historical.ExpiresAt = &expires
	historical.TargetBoundaries = []TargetBoundary{{Type: TargetTypeAgent, ID: "one-agent", Scope: "published", VersionID: version}}
	narrowed, err := IntersectConstraints(current, historical)
	if err != nil {
		t.Fatal(err)
	}
	target := TargetRef{Type: TargetTypeAgent, ID: "one-agent", Scope: ScopePublished, VersionID: version}
	if !narrowed.Allows(owner, "github.user.get", target, now) {
		t.Fatal("common authorized operation lost")
	}
	for _, scenario := range []struct {
		actor     uuid.UUID
		operation Identifier
		target    TargetRef
		now       time.Time
	}{
		{owner, "github.issue.create", target, now}, {uuid.New(), "github.user.get", target, now}, {owner, "github.user.get", TargetRef{Type: TargetTypeAgent, ID: "one-agent", Scope: ScopeDraft}, now}, {owner, "github.user.get", target, expires},
	} {
		if narrowed.Allows(scenario.actor, scenario.operation, scenario.target, scenario.now) {
			t.Fatal("constraint intersection widened authority")
		}
	}
	if _, err = IntersectConstraints(current, nil); err == nil {
		t.Fatal("missing historical constraint must fail closed")
	}
}
