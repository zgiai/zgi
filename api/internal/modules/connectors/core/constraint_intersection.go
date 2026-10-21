package core

import (
	"encoding/json"
	"sort"
)

// IntersectConstraints can only narrow authority. Missing or unsupported
// restrictions are errors, including during copy, restore and reconnect.
func IntersectConstraints(a, b *AccessConstraint) (*AccessConstraint, error) {
	if a.Validate() != nil || b.Validate() != nil || a.AudienceOwner != b.AudienceOwner {
		return nil, invalidModel()
	}
	out := &AccessConstraint{SchemaVersion: 1, AudienceOwner: a.AudienceOwner, AllowedOperationIDs: []Identifier{}, TargetBoundaries: []TargetBoundary{}, DenyReasons: []string{}, AuthorizationMode: AuthorizationModeAsk}
	// Authorization is a binding-level opt-in. The left-hand constraint is the
	// current binding being narrowed; a connection/source constraint on the
	// right may restrict it, but must never grant direct execution by itself.
	if a.AuthorizationMode == AuthorizationModeDirect {
		out.AuthorizationMode = AuthorizationModeDirect
		out.AuthorizationConnectionRevision = a.AuthorizationConnectionRevision
		out.AuthorizationCredentialVersion = a.AuthorizationCredentialVersion
		out.AuthorizationContractDigest = a.AuthorizationContractDigest
	}
	for _, id := range a.AllowedOperationIDs {
		for _, other := range b.AllowedOperationIDs {
			if id == other {
				out.AllowedOperationIDs = append(out.AllowedOperationIDs, id)
				break
			}
		}
	}
	sort.Slice(out.AllowedOperationIDs, func(i, j int) bool { return out.AllowedOperationIDs[i] < out.AllowedOperationIDs[j] })
	seen := map[TargetBoundary]bool{}
	for _, left := range a.TargetBoundaries {
		for _, right := range b.TargetBoundaries {
			if left.Type != right.Type {
				continue
			}
			id, ok := intersectBoundary(left.ID, right.ID)
			if !ok {
				continue
			}
			scope, ok := intersectBoundary(left.Scope, right.Scope)
			if !ok {
				continue
			}
			version, ok := intersectBoundary(left.VersionID, right.VersionID)
			if !ok {
				continue
			}
			bound := TargetBoundary{left.Type, id, scope, version}
			if !seen[bound] {
				seen[bound] = true
				out.TargetBoundaries = append(out.TargetBoundaries, bound)
			}
		}
	}
	sort.Slice(out.TargetBoundaries, func(i, j int) bool {
		l, _ := json.Marshal(out.TargetBoundaries[i])
		r, _ := json.Marshal(out.TargetBoundaries[j])
		return string(l) < string(r)
	})
	denied := map[string]bool{}
	for _, set := range [][]string{a.DenyReasons, b.DenyReasons} {
		for _, reason := range set {
			if !denied[reason] {
				denied[reason] = true
				out.DenyReasons = append(out.DenyReasons, reason)
			}
		}
	}
	sort.Strings(out.DenyReasons)
	out.ExpiresAt = a.ExpiresAt
	if b.ExpiresAt != nil && (out.ExpiresAt == nil || b.ExpiresAt.Before(*out.ExpiresAt)) {
		out.ExpiresAt = b.ExpiresAt
	}
	out.SourceDigest = a.SourceDigest
	if out.SourceDigest == "" {
		out.SourceDigest = b.SourceDigest
	}
	return out, nil
}
func intersectBoundary(a, b string) (string, bool) {
	if a == b || b == "*" {
		return a, true
	}
	if a == "*" {
		return b, true
	}
	return "", false
}

func EquivalentConstraints(a, b *AccessConstraint) bool {
	left, e := IntersectConstraints(a, a)
	if e != nil {
		return false
	}
	right, e := IntersectConstraints(b, b)
	if e != nil {
		return false
	}
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	return string(l) == string(r)
}
