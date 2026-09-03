package main

import (
	"sort"
	"time"
)

// Restriction intersections preserve the complete frozen authority, including
// audience, target boundaries, expiry and explicit denials.
func intersectConstraints(a, b row) row {
	owner := text(a, "audience_owner")
	out := ownerConstraint(owner, intersection(stringsOf(a["allowed_operation_ids"]), stringsOf(b["allowed_operation_ids"])))
	out["target_boundaries"] = []any{}
	if !constraintValid(a, owner) || !constraintValid(b, owner) {
		addDeny(out, "source_constraint_invalid")
		return out
	}
	boundaries := []any{}
	seen := map[string]bool{}
	for _, left := range list(a["target_boundaries"]) {
		for _, right := range list(b["target_boundaries"]) {
			l, r := object(left), object(right)
			if text(l, "type") != text(r, "type") {
				continue
			}
			bound := row{"type": l["type"]}
			compatible := true
			for _, field := range []string{"id", "scope", "version_id"} {
				x, y := text(l, field), text(r, field)
				switch {
				case x == y:
					bound[field] = x
				case x == "*":
					bound[field] = y
				case y == "*":
					bound[field] = x
				default:
					compatible = false
				}
			}
			if compatible && !seen[hash(bound)] {
				boundaries = append(boundaries, bound)
				seen[hash(bound)] = true
			}
		}
	}
	sort.Slice(boundaries, func(i, j int) bool { return hash(boundaries[i]) < hash(boundaries[j]) })
	out["target_boundaries"] = boundaries
	for _, source := range []row{a, b} {
		for _, reason := range stringsOf(source["deny_reasons"]) {
			addDeny(out, reason)
		}
		if expiry := text(source, "expires_at"); expiry != "" {
			value, err := time.Parse(time.RFC3339Nano, expiry)
			previous, previousErr := time.Parse(time.RFC3339Nano, text(out, "expires_at"))
			if err != nil {
				addDeny(out, "source_expiry_invalid")
			} else if previousErr != nil || value.Before(previous) {
				out["expires_at"] = expiry
			}
		}
	}
	out["source_digest"] = hash([]any{a, b})
	return out
}

func constraintAllows(constraint, target row, operation string, now time.Time) bool {
	if !constraintValid(constraint, text(target, "owner_account_id")) || len(stringsOf(constraint["deny_reasons"])) > 0 {
		return false
	}
	if expiry := text(constraint, "expires_at"); expiry != "" {
		value, err := time.Parse(time.RFC3339Nano, expiry)
		if err != nil || !now.Before(value) {
			return false
		}
	}
	if len(intersection([]string{operation}, stringsOf(constraint["allowed_operation_ids"]))) != 1 {
		return false
	}
	for _, raw := range list(constraint["target_boundaries"]) {
		boundary := object(raw)
		if text(boundary, "type") != text(target, "target_type") {
			continue
		}
		matches := true
		for _, pair := range [][2]string{{"id", "target_id"}, {"scope", "target_scope"}, {"version_id", "target_version_id"}} {
			value := text(boundary, pair[0])
			if value != "*" && value != text(target, pair[1]) {
				matches = false
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func (c *converter) restrictionConstraint(owner, connector string, restriction row) row {
	constraint := ownerConstraint(owner, c.operations(connector))
	if restriction == nil {
		return constraint
	}
	if native := object(restriction["native_constraint"]); len(native) > 0 {
		constraint = intersectConstraints(constraint, native)
	}
	if allowed, ok := restriction["allowed_action_ids"]; ok {
		operations := []string{}
		for _, id := range stringsOf(allowed) {
			if op := c.operation(connector, id); op != nil {
				operations = append(operations, text(op, "id"))
			}
		}
		constraint["allowed_operation_ids"] = intersection(stringsOf(constraint["allowed_operation_ids"]), operations)
	} else if restriction["native_constraint"] == nil && text(restriction, "connector_projection_compatibility") != "native" {
		addDeny(constraint, "source_allowlist_missing")
	}
	if expiry := text(restriction, "expires_at"); expiry != "" {
		extra := ownerConstraint(owner, c.operations(connector))
		extra["expires_at"] = expiry
		constraint = intersectConstraints(constraint, extra)
	}
	if tool := text(restriction, "tool_name"); tool != "" {
		if op := c.operation(connector, tool); op != nil {
			constraint["allowed_operation_ids"] = intersection(stringsOf(constraint["allowed_operation_ids"]), []string{text(op, "id")})
		} else {
			addDeny(constraint, "source_operation_unknown")
		}
	}
	for key, value := range restriction {
		switch key {
		case "native_constraint", "allowed_action_ids", "expires_at", "connector_projection_compatibility", "connector_id", "tool_name":
			continue
		}
		if value != nil {
			addDeny(constraint, "unsupported_source_binding_restriction")
		}
	}
	constraint["source_digest"] = hash(restriction)
	return constraint
}
