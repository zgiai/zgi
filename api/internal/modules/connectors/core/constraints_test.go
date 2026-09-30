package core

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestConstraintsRequireBothLayersAndExplicitAuthority(t *testing.T) {
	owner := uuid.New()
	target := TargetRef{Type: TargetTypeAgent, ID: uuid.NewString()}
	now := time.Now().UTC()
	connection := OwnerConstraint(owner, []Identifier{"github.issue.get"})
	binding := OwnerConstraint(owner, []Identifier{"github.issue.get", "github.issue.create"})
	if !connection.Allows(owner, "github.issue.get", target, now) || !binding.Allows(owner, "github.issue.get", target, now) {
		t.Fatal("valid read denied")
	}
	if connection.Allows(owner, "github.issue.create", target, now) {
		t.Fatal("binding widened connection restriction")
	}
	if (*AccessConstraint)(nil).Allows(owner, "github.issue.get", target, now) {
		t.Fatal("missing constraint allowed")
	}
	if connection.Allows(uuid.New(), "github.issue.get", target, now) {
		t.Fatal("audience widened")
	}
	connection.ExpiresAt = &now
	if connection.Allows(owner, "github.issue.get", target, now) {
		t.Fatal("expired permission allowed")
	}
	connection.ExpiresAt = nil
	connection.DenyReasons = []string{"source_disabled"}
	if connection.Allows(owner, "github.issue.get", target, now) {
		t.Fatal("deny lost")
	}
}

func TestLegacyConstraintDefaultsToAskAndDirectConstraintIsExplicit(t *testing.T) {
	owner := uuid.New()
	legacy, err := DecodeConstraint([]byte(`{"schema_version":1,"allowed_operation_ids":["github.issue.create"],"target_boundaries":[{"type":"chat","id":"*","scope":"live","version_id":"*"}],"audience_owner":"` + owner.String() + `","expires_at":null,"deny_reasons":[]}`))
	if err != nil || legacy.AuthorizationModeValue() != AuthorizationModeAsk || legacy.DirectlyAuthorized() {
		t.Fatalf("legacy constraint mode = %#v, err=%v", legacy, err)
	}
	direct := OwnerConstraint(owner, []Identifier{"github.issue.create"})
	direct.AuthorizationMode = AuthorizationModeDirect
	if err := direct.Validate(); err != nil || !direct.DirectlyAuthorized() {
		t.Fatalf("direct constraint = %#v, err=%v", direct, err)
	}
}

func TestDraftBoundaryDoesNotAuthorizePublishedVersion(t *testing.T) {
	owner := uuid.New()
	id := uuid.NewString()
	c := OwnerConstraint(owner, []Identifier{"web.search"})
	c.TargetBoundaries = []TargetBoundary{{Type: TargetTypeAgent, ID: id, Scope: "draft", VersionID: ""}}
	if !c.Allows(owner, "web.search", TargetRef{Type: TargetTypeAgent, ID: id}, time.Now()) {
		t.Fatal("draft denied")
	}
	if c.Allows(owner, "web.search", TargetRef{Type: TargetTypeAgent, ID: id, Scope: ScopePublished, VersionID: uuid.NewString()}, time.Now()) {
		t.Fatal("version boundary lost")
	}
	if _, err := DecodeConstraint([]byte(`{"schema_version":1}`)); err == nil {
		t.Fatal("incomplete constraint accepted")
	}
}
