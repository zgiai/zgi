package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestHostDelegationRequiresOwnerAndExactPublishedTarget(t *testing.T) {
	owner := uuid.New()
	target := TargetRef{Type: TargetTypeDigitalHuman, ID: uuid.NewString(), Scope: ScopePublished, VersionID: uuid.NewString(), BindingInstanceID: uuid.NewString()}
	constraint := OwnerConstraint(owner, []Identifier{"web.search"})
	constraint.TargetBoundaries = []TargetBoundary{{Type: target.Type, ID: target.ID, Scope: string(target.Scope), VersionID: target.VersionID}}
	constraint.HostDelegation = &HostDelegation{GrantedBy: owner, Audiences: []HostAudience{HostAudienceWorkspace, HostAudienceAPI, HostAudienceChannel}}
	if err := constraint.Validate(); err != nil {
		t.Fatalf("owner-approved published grant rejected: %v", err)
	}
	encoded, err := json.Marshal(constraint)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeConstraint(encoded)
	if err != nil || restored.HostDelegation == nil || restored.HostDelegation.GrantedBy != owner || len(restored.HostDelegation.Audiences) != 3 {
		t.Fatalf("persisted grant = %#v, error = %v", restored, err)
	}
	now := time.Now().UTC()
	if constraint.Allows(uuid.New(), "web.search", target, now) {
		t.Fatal("host grant silently widened the personal owner constraint")
	}
	if !constraint.AllowsHostAudience(owner, "web.search", target, HostAudienceAPI, now) {
		t.Fatal("explicit published grant did not permit its API audience")
	}
	withoutGrant := *constraint
	withoutGrant.HostDelegation = nil
	otherVersion := target
	otherVersion.VersionID = uuid.NewString()
	for _, testCase := range []struct {
		constraint *AccessConstraint
		owner      uuid.UUID
		operation  Identifier
		target     TargetRef
		audience   HostAudience
	}{
		{&withoutGrant, owner, "web.search", target, HostAudienceAPI},
		{constraint, uuid.New(), "web.search", target, HostAudienceAPI},
		{constraint, owner, "web.write", target, HostAudienceAPI},
		{constraint, owner, "web.search", otherVersion, HostAudienceAPI},
		{constraint, owner, "web.search", target, "unknown"},
	} {
		if testCase.constraint.AllowsHostAudience(testCase.owner, testCase.operation, testCase.target, testCase.audience, now) {
			t.Fatal("missing or mismatched published grant permitted a delegated operation")
		}
	}
	for _, change := range []struct {
		name   string
		change func(*AccessConstraint)
	}{
		{"wrong approver", func(c *AccessConstraint) { c.HostDelegation.GrantedBy = uuid.New() }},
		{"unknown audience", func(c *AccessConstraint) { c.HostDelegation.Audiences = []HostAudience{"everyone"} }},
		{"duplicate audience", func(c *AccessConstraint) {
			c.HostDelegation.Audiences = []HostAudience{HostAudienceAPI, HostAudienceAPI}
		}},
		{"draft grant", func(c *AccessConstraint) { c.TargetBoundaries[0].Scope = "draft" }},
		{"wildcard version", func(c *AccessConstraint) { c.TargetBoundaries[0].VersionID = "*" }},
		{"multiple hosts", func(c *AccessConstraint) {
			c.TargetBoundaries = append(c.TargetBoundaries, TargetBoundary{Type: TargetTypeAgent, ID: uuid.NewString(), Scope: "published", VersionID: uuid.NewString()})
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			copy := *constraint
			grant := *constraint.HostDelegation
			grant.Audiences = append([]HostAudience(nil), grant.Audiences...)
			copy.HostDelegation = &grant
			copy.TargetBoundaries = append([]TargetBoundary(nil), constraint.TargetBoundaries...)
			change.change(&copy)
			if err := copy.Validate(); err == nil {
				t.Fatal("invalid host delegation accepted")
			}
		})
	}
}

func TestDigitalHumanRequiresExactBindingWithoutRewritingConnectionConstraint(t *testing.T) {
	owner := uuid.New()
	now := time.Now().UTC()
	target := TargetRef{
		Type: TargetTypeDigitalHuman, ID: uuid.NewString(), Scope: ScopePublished,
		VersionID: uuid.NewString(), BindingInstanceID: uuid.NewString(),
	}
	connection := OwnerConstraint(owner, []Identifier{"web.search"})
	if connection.Allows(owner, "web.search", target, now) {
		t.Fatal("historical connection constraint silently granted DigitalHuman access")
	}
	binding := OwnerConstraint(owner, []Identifier{"web.search"})
	binding.TargetBoundaries = []TargetBoundary{{
		Type: target.Type, ID: target.ID, Scope: string(target.Scope), VersionID: target.VersionID,
	}}
	if !ConstraintsAllow(connection, binding, owner, "web.search", target, now) {
		t.Fatal("exact owner-created DigitalHuman binding did not supply the V4 host boundary")
	}
	other := target
	other.ID = uuid.NewString()
	if ConstraintsAllow(connection, binding, owner, "web.search", other, now) {
		t.Fatal("DigitalHuman binding authorized another target")
	}
}
