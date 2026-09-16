package core

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TargetScope string

const (
	ScopeLive      TargetScope = "live"
	ScopeDraft     TargetScope = "draft"
	ScopePublished TargetScope = "published"
)

func (target TargetRef) EffectiveScope() TargetScope {
	if target.Scope != "" {
		return target.Scope
	}
	if target.Type == TargetTypeChat {
		return ScopeLive
	}
	return ScopeDraft
}

func (target TargetRef) SameIdentity(other TargetRef) bool {
	return target.Type == other.Type && target.ID == other.ID && target.EffectiveScope() == other.EffectiveScope() && target.VersionID == other.VersionID && target.BindingInstanceID == other.BindingInstanceID
}

// TargetBoundary is server-owned. A wildcard must be explicit; missing fields
// never grant access. Published versions remain separate from mutable drafts.
type TargetBoundary struct {
	Type      TargetType `json:"type"`
	ID        string     `json:"id"`
	Scope     string     `json:"scope"`
	VersionID string     `json:"version_id"`
}

type AccessConstraint struct {
	SchemaVersion       int              `json:"schema_version"`
	AllowedOperationIDs []Identifier     `json:"allowed_operation_ids"`
	TargetBoundaries    []TargetBoundary `json:"target_boundaries"`
	AudienceOwner       uuid.UUID        `json:"audience_owner"`
	ExpiresAt           *time.Time       `json:"expires_at"`
	DenyReasons         []string         `json:"deny_reasons"`
	SourceDigest        string           `json:"source_digest,omitempty"`
}

func OwnerConstraint(owner uuid.UUID, operations []Identifier) *AccessConstraint {
	return &AccessConstraint{SchemaVersion: 1, AudienceOwner: owner,
		AllowedOperationIDs: append([]Identifier{}, operations...), DenyReasons: []string{},
		TargetBoundaries: []TargetBoundary{{TargetTypeChat, "*", "live", "*"}, {TargetTypeAgent, "*", "*", "*"}, {TargetTypeWorkflow, "*", "*", "*"}},
	}
}

// ConstraintsAllow preserves the historical Connection boundary while allowing
// an owner-created, exact DigitalHuman Target Binding to supply the newly added
// host boundary. All operation, expiry, deny and owner restrictions still come
// from both layers; a persisted binding is therefore required before V4 access.
func ConstraintsAllow(connection, binding *AccessConstraint, owner uuid.UUID, operation Identifier, target TargetRef, now time.Time) bool {
	if connection == nil || binding == nil || !binding.Allows(owner, operation, target, now) {
		return false
	}
	if connection.Allows(owner, operation, target, now) {
		return true
	}
	if target.Type != TargetTypeDigitalHuman || connection.Validate() != nil {
		return false
	}
	projected := *connection
	projected.TargetBoundaries = append(append([]TargetBoundary{}, connection.TargetBoundaries...), TargetBoundary{
		Type: target.Type, ID: target.ID, Scope: string(target.EffectiveScope()), VersionID: target.VersionID,
	})
	return projected.Allows(owner, operation, target, now)
}

func (constraint *AccessConstraint) Validate() error {
	if constraint == nil || constraint.SchemaVersion != 1 || constraint.AudienceOwner == uuid.Nil ||
		constraint.AllowedOperationIDs == nil || constraint.TargetBoundaries == nil || constraint.DenyReasons == nil {
		return invalidModel()
	}
	seen := map[Identifier]bool{}
	for _, id := range constraint.AllowedOperationIDs {
		if !id.Valid() || seen[id] {
			return invalidModel()
		}
		seen[id] = true
	}
	for _, boundary := range constraint.TargetBoundaries {
		if !boundary.Type.Valid() || strings.TrimSpace(boundary.ID) == "" || (boundary.Scope != "*" && boundary.Scope != "live" && boundary.Scope != "draft" && boundary.Scope != "published") {
			return invalidModel()
		}
	}
	if constraint.SourceDigest != "" && !hmacSHA256HexPattern.MatchString(constraint.SourceDigest) {
		return invalidModel()
	}
	return nil
}

func (constraint *AccessConstraint) Allows(owner uuid.UUID, operation Identifier, target TargetRef, now time.Time) bool {
	if constraint.Validate() != nil || target.Validate() != nil || now.IsZero() || constraint.AudienceOwner != owner || len(constraint.DenyReasons) != 0 ||
		(constraint.ExpiresAt != nil && !now.Before(*constraint.ExpiresAt)) {
		return false
	}
	allowed := false
	for _, id := range constraint.AllowedOperationIDs {
		if id == operation {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	for _, boundary := range constraint.TargetBoundaries {
		if boundary.Type == target.Type && matchesBoundary(boundary.ID, target.ID) && matchesBoundary(boundary.Scope, string(target.EffectiveScope())) && matchesBoundary(boundary.VersionID, target.VersionID) {
			return true
		}
	}
	return false
}

func matchesBoundary(allowed, actual string) bool { return allowed == "*" || allowed == actual }

// DecodeConstraint rejects unknown fields so a newer/unsupported restriction
// cannot be silently discarded by an older server.
func DecodeConstraint(data []byte) (*AccessConstraint, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var constraint AccessConstraint
	if decoder.Decode(&constraint) != nil || constraint.Validate() != nil {
		return nil, invalidModel()
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, invalidModel()
	}
	return &constraint, nil
}

type UnavailableReference struct {
	ID                uuid.UUID `json:"id"`
	OrganizationID    uuid.UUID `json:"-"`
	OwnerAccountID    uuid.UUID `json:"-"`
	Target            TargetRef `json:"target"`
	FormerReferenceID string    `json:"former_reference_id"`
	Label             string    `json:"label"`
	ReasonCode        string    `json:"reason_code"`
	Status            string    `json:"status"`
	Revision          int       `json:"revision"`
}

type ReferenceCommand struct {
	ReferenceID      uuid.UUID  `json:"reference_id"`
	ExpectedRevision int        `json:"expected_revision"`
	Action           string     `json:"action"`
	ConnectionID     uuid.UUID  `json:"connection_id,omitempty"`
	OperationID      Identifier `json:"operation_id,omitempty"`
}
