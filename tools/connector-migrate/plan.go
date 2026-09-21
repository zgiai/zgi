package main

import (
	"embed"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// These are public, frozen native contracts, not executable provider code.
//
//go:embed data/contracts.json
var contractsFS embed.FS

type converter struct {
	source       databaseSnapshot
	plan         conversionPlan
	keys         keyrings
	definitions  map[string]row
	connections  map[string]row
	bindings     map[string]row
	unavailable  map[string]row
	runWorkflows map[string]row
}

func makePlan(snapshot databaseSnapshot, keys keyrings) (conversionPlan, error) {
	c := &converter{source: snapshot, keys: keys, definitions: map[string]row{}, connections: map[string]row{}, bindings: map[string]row{}, unavailable: map[string]row{}}
	c.plan.Report = inventory(snapshot)
	c.plan.Report.State = "dry_run"
	if len(snapshot["connector_schema_readiness"]) != 1 {
		return c.plan, errors.New("native schema readiness row is missing")
	}
	if integer(snapshot["connector_schema_readiness"][0], "schema_version") != 1 {
		return c.plan, errors.New("historical converter supports schema1 only; use connector-finalize verify for schema2")
	}
	if text(snapshot["connector_schema_readiness"][0], "state") == "ready" {
		c.plan.Report = verifySnapshot(snapshot, keys)
		c.plan.Report.State = "already_ready"
		return c.plan, nil
	}
	if len(snapshot["connector_conversion_records"]) > 0 {
		return c.plan, errors.New("partial conversion ledger requires recovery from a consistent backup")
	}
	if err := c.seedDefinitions(); err != nil {
		return c.plan, err
	}
	c.convertConnections()
	c.convertCanonicalBindings()
	c.convertHostBindings()
	c.convertHostSnapshots()
	c.convertChatPreferences()
	c.convertSkills()
	c.convertClaims()
	sort.Slice(c.plan.Report.Issues, func(i, j int) bool {
		a, b := c.plan.Report.Issues[i], c.plan.Report.Issues[j]
		return a.Table+a.ID+a.Code < b.Table+b.ID+b.Code
	})
	return c.plan, nil
}
func (c *converter) seedDefinitions() error {
	data, err := contractsFS.ReadFile("data/contracts.json")
	if err != nil {
		return err
	}
	var contracts []row
	if json.Unmarshal(data, &contracts) != nil {
		return errors.New("invalid frozen native contracts")
	}
	for _, contract := range contracts {
		d := object(contract["definition"])
		if text(d, "plugin_digest") == "" {
			// Frozen built-ins predate content manifests. Preserve their published
			// identity digest, using the exact field order of the native catalog.
			d["plugin_digest"] = hash(struct {
				PluginID      string `json:"plugin_id"`
				PluginVersion string `json:"plugin_version"`
				Dialect       string `json:"source_dialect"`
				SchemaVersion string `json:"schema_version"`
			}{text(d, "plugin_id"), text(d, "plugin_version"), text(d, "source_dialect"), text(d, "source_schema_version")})
		}
		id := text(d, "id")
		c.definitions[id] = contract
		if existing := findBy(c.source["connector_definitions"], "connector_id", id); existing != nil {
			if text(existing, "provider_id") != text(d, "provider_id") || text(existing, "driver_id") != text(d, "driver_id") {
				c.plan.problem("connector_definitions", id, "native_definition_identity_conflict")
			}
			continue
		}
		plugin := row{"plugin_id": d["plugin_id"], "plugin_version": d["plugin_version"], "plugin_digest": d["plugin_digest"], "source_dialect": d["source_dialect"], "schema_version": d["source_schema_version"], "status": "active"}
		c.plan.add("connector_plugin_versions", nil, plugin, true)
		c.plan.add("connector_definition_versions", nil, row{"connector_id": id, "plugin_id": d["plugin_id"], "plugin_version": d["plugin_version"], "plugin_digest": d["plugin_digest"], "contract_digest": contract["contract_digest"], "definition": d, "status": "active"}, true)
		c.plan.add("connector_definitions", nil, row{"connector_id": id, "plugin_id": d["plugin_id"], "plugin_version": d["plugin_version"], "plugin_digest": d["plugin_digest"], "source_dialect": d["source_dialect"], "schema_version": d["source_schema_version"], "provider_id": d["provider_id"], "driver_id": d["driver_id"], "classification": d["classification"], "contract_digest": contract["contract_digest"], "display": row{"name": d["name"], "description": d["description"], "icon": d["icon"], "name_i18n": d["name_i18n"], "description_i18n": d["description_i18n"]}, "key_schema": row{"setup_method": d["setup_method"], "ownership": d["ownership"], "auth_method_id": d["auth_method_id"], "fields": d["key_fields"]}, "capability_summary": row{"types": d["capability_types"], "surfaces": d["surfaces"], "operations": d["operations"]}, "status": "active"}, true)
	}
	return nil
}
func (c *converter) operations(id string) []string {
	out := []string{}
	for _, raw := range list(object(c.definitions[id]["definition"])["operations"]) {
		out = append(out, text(object(raw), "id"))
	}
	sort.Strings(out)
	return out
}
func (c *converter) operation(id, operation string) row {
	for _, raw := range list(object(c.definitions[id]["definition"])["operations"]) {
		r := object(raw)
		if text(r, "id") == operation || text(r, "tool_name") == operation {
			return r
		}
	}
	return nil
}
func ownerConstraint(owner string, operations []string) row {
	return row{"schema_version": 1, "audience_owner": owner, "allowed_operation_ids": operations, "target_boundaries": []any{row{"type": "chat", "id": "*", "scope": "live", "version_id": "*"}, row{"type": "agent", "id": "*", "scope": "*", "version_id": "*"}, row{"type": "workflow", "id": "*", "scope": "*", "version_id": "*"}}, "deny_reasons": []string{}, "expires_at": nil}
}
func intersection(a, b []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range a {
		for _, w := range b {
			if v == w && !seen[v] {
				out = append(out, v)
				seen[v] = true
			}
		}
	}
	sort.Strings(out)
	return out
}
func addDeny(constraint row, reason string) {
	reasons := stringsOf(constraint["deny_reasons"])
	for _, v := range reasons {
		if v == reason {
			return
		}
	}
	constraint["deny_reasons"] = append(reasons, reason)
}
func (c *converter) connectionConstraint(source row, connector, owner string) row {
	out := ownerConstraint(owner, c.operations(connector))
	out["source_digest"] = hash(source)
	if value := source["expires_at"]; value != nil {
		out["expires_at"] = value
	}
	if len(object(source["config"])) > 0 {
		addDeny(out, "unsupported_source_configuration")
	}
	ops := stringsOf(out["allowed_operation_ids"])
	for _, policy := range c.source["integration_action_policies"] {
		if text(policy, "organization_id") != text(source, "organization_id") || text(policy, "integration_id") != connector {
			continue
		}
		if policy["enabled"] == false || policy["data_egress_allowed"] == false {
			filtered := []string{}
			for _, op := range ops {
				if op != text(policy, "action_id") {
					filtered = append(filtered, op)
				}
			}
			ops = filtered
		}
		if text(policy, "approval_policy") == "always_ask" {
			addDeny(out, "source_confirmation_policy_requires_review")
		}
	}
	out["allowed_operation_ids"] = ops
	for _, grant := range c.source["integration_connection_grants"] {
		if text(grant, "connection_id") == text(source, "id") && len(object(grant["resource_constraints"])) > 0 {
			addDeny(out, "unsupported_source_resource_constraint")
		}
	}
	return out
}
func (c *converter) convertConnections() {
	for _, source := range c.source["connector_connections"] {
		id := text(source, "id")
		native := clone(source)
		constraint := object(source["connection_constraint"])
		if len(constraint) == 0 {
			constraint = ownerConstraint(text(source, "owner_account_id"), intersection(c.operations(text(source, "connector_id")), stringsOf(source["usable_operation_ids"])))
			// A pre-existing migrated connection must retain the original restrictions,
			// even if its old bridge represented the connection as generally ready.
			if old := findBy(c.source["integration_connections"], "id", id); old != nil {
				constraint = c.connectionConstraint(old, text(source, "connector_id"), text(source, "owner_account_id"))
				constraint["allowed_operation_ids"] = intersection(stringsOf(constraint["allowed_operation_ids"]), stringsOf(source["usable_operation_ids"]))
			}
		}
		native["connection_constraint"] = constraint
		if source["deleted_at"] != nil {
			addDeny(constraint, "connection_deleted")
			c.plan.add("connector_connections", row{"id": id}, row{"connection_constraint": constraint}, false)
			c.connections[id] = native
			c.plan.record("native_connection", id, source, id, "tombstone")
			continue
		}
		subject := secretSubject{text(source, "organization_id"), id, text(source, "connector_id"), "connection-key", integer(source, "credential_version")}
		plaintext, err := decryptEnvelope(c.keys, subject, text(source, "encrypted_credentials"))
		if err != nil {
			c.plan.problem("connector_connections", id, "credential_decode_failed")
			continue
		}
		envelope, err := encryptNative(c.keys, subject, plaintext)
		clear(plaintext)
		if err != nil {
			c.plan.problem("connector_connections", id, "credential_reseal_failed")
			continue
		}
		native["encrypted_credentials"] = envelope
		c.plan.add("connector_connections", row{"id": id}, row{"encrypted_credentials": envelope, "connection_constraint": constraint}, false)
		c.connections[id] = native
		c.plan.record("native_connection", id, source, id, "converted")
	}
	for _, source := range c.source["integration_connections"] {
		id := text(source, "id")
		if existing := c.connections[id]; existing != nil {
			if text(existing, "organization_id") != text(source, "organization_id") || text(existing, "owner_account_id") != text(source, "owner_account_id") || text(existing, "connector_id") != text(source, "integration_id") {
				c.plan.problem("integration_connections", id, "connection_identity_collision")
			}
			continue
		}
		connector, owner := text(source, "integration_id"), text(source, "owner_account_id")
		if c.definitions[connector] == nil || source["deleted_at"] != nil || text(source, "credential_source") != "account" || text(source, "auth_type") != "api_key" || !validID(owner) {
			c.plan.record("source_connection", id, source, "", "unavailable")
			continue
		}
		d := object(c.definitions[connector]["definition"])
		if text(source, "driver_id") != text(d, "driver_id") {
			c.plan.record("source_connection", id, source, "", "unavailable")
			continue
		}
		subject := secretSubject{text(source, "organization_id"), id, connector, "connection-key", integer(source, "credential_version")}
		plaintext, err := decryptEnvelope(c.keys, subject, text(source, "encrypted_credentials"))
		if err != nil {
			c.plan.problem("integration_connections", id, "credential_decode_failed")
			continue
		}
		var values map[string]string
		if json.Unmarshal(plaintext, &values) != nil || len(values) == 0 {
			clear(plaintext)
			c.plan.problem("integration_connections", id, "credential_payload_invalid")
			continue
		}
		envelope, err := encryptNative(c.keys, subject, plaintext)
		clear(plaintext)
		clear(values)
		if err != nil {
			c.plan.problem("integration_connections", id, "credential_reseal_failed")
			continue
		}
		constraint := c.connectionConstraint(source, connector, owner)
		state := "reconnect_required"
		if text(source, "status") == "active" && text(source, "auth_status") == "valid" && text(source, "health_status") == "healthy" && text(source, "scope_status") == "verified" {
			state = "ready"
		}
		if text(source, "status") == "disabled" || len(stringsOf(constraint["deny_reasons"])) > 0 || len(stringsOf(constraint["allowed_operation_ids"])) == 0 {
			state = "unavailable"
		}
		verified := source["last_health_checked_at"]
		if verified == nil {
			verified = source["last_tested_at"]
		}
		if verified == nil {
			verified = source["created_at"]
		}
		if verified == nil {
			c.plan.problem("integration_connections", id, "verification_timestamp_missing")
			continue
		}
		native := row{"id": id, "organization_id": source["organization_id"], "owner_account_id": owner, "connector_id": connector, "provider_id": d["provider_id"], "driver_id": d["driver_id"], "auth_method_id": d["auth_method_id"], "name": source["name"], "external_account_id": source["account_id"], "external_display_name": source["display_name"], "encrypted_credentials": envelope, "credential_version": source["credential_version"], "state": state, "usable_operation_ids": stringsOf(constraint["allowed_operation_ids"]), "unavailable_operations": row{}, "revision": source["revision"], "last_verified_at": verified, "created_at": source["created_at"], "updated_at": source["updated_at"], "connection_constraint": constraint}
		c.plan.add("connector_connections", nil, native, true)
		c.connections[id] = native
		c.plan.record("source_connection", id, source, id, "converted")
	}
}

func targetKey(r row) string {
	return strings.Join([]string{text(r, "organization_id"), text(r, "owner_account_id"), text(r, "target_type"), text(r, "target_id"), text(r, "target_scope"), text(r, "target_version_id"), text(r, "binding_instance_id"), text(r, "connector_id")}, "\x00")
}
func (c *converter) convertCanonicalBindings() {
	for _, source := range c.source["connector_target_bindings"] {
		b := clone(source)
		id := text(source, "id")
		if text(b, "target_scope") == "" {
			b["target_scope"] = "draft"
			if text(b, "target_type") == "chat" {
				b["target_scope"] = "live"
			}
		}
		if b["target_version_id"] == nil {
			b["target_version_id"] = ""
		}
		connection := c.connections[text(b, "connection_id")]
		if connection == nil || connection["deleted_at"] != nil || text(source, "compatibility") != "native" {
			c.unavailableReference(b, id, "source_connection_unavailable")
			b["status"] = "needs_replacement"
			b["compatibility"] = "native"
			b["compatibility_state"] = row{}
		}
		constraint := object(b["binding_constraint"])
		if len(constraint) == 0 {
			constraint = ownerConstraint(text(b, "owner_account_id"), c.operations(text(b, "connector_id")))
			// Preserve any frozen allowlist on the preceding canonical projection.
			state := object(source["compatibility_state"])
			if allowed, ok := state["allowed_action_ids"]; ok {
				constraint["allowed_operation_ids"] = intersection(stringsOf(constraint["allowed_operation_ids"]), stringsOf(allowed))
			}
			if len(state) > 0 {
				for key := range state {
					if key != "allowed_action_ids" && key != "legacy_connection_id" {
						addDeny(constraint, "unsupported_projection_restriction")
					}
				}
			}
		}
		b["binding_constraint"] = constraint
		if len(stringsOf(constraint["deny_reasons"])) > 0 && text(b, "status") == "active" {
			b["status"] = "needs_replacement"
			c.unavailableReference(b, id, "binding_restriction_unavailable")
		}
		c.bindings[targetKey(b)] = b
		c.plan.add("connector_target_bindings", row{"id": id}, row{"binding_constraint": constraint, "target_scope": b["target_scope"], "target_version_id": b["target_version_id"], "compatibility": b["compatibility"], "compatibility_state": b["compatibility_state"], "status": b["status"]}, false)
		c.plan.record("native_binding", id, source, id, "converted")
	}
}
func (c *converter) unavailableReference(target row, former, reason string) row {
	if !validID(text(target, "organization_id")) || !validID(text(target, "owner_account_id")) || !validID(text(target, "target_id")) {
		c.plan.problem("connector_unavailable_references", former, "target_owner_unresolved")
		return nil
	}
	key := targetKey(target) + "\x00" + former
	if existing := c.unavailable[key]; existing != nil {
		return existing
	}
	ref := row{"id": deterministicID("unavailable", key), "organization_id": target["organization_id"], "owner_account_id": target["owner_account_id"], "target_type": target["target_type"], "target_id": target["target_id"], "target_scope": target["target_scope"], "target_version_id": text(target, "target_version_id"), "binding_instance_id": text(target, "binding_instance_id"), "former_reference_id": former, "label": "Unavailable external connection", "reason_code": reason, "status": "needs_replacement", "revision": 1}
	c.unavailable[key] = ref
	c.plan.add("connector_unavailable_references", nil, ref, true)
	return ref
}
func (c *converter) binding(target row, connectionID, former string, restriction row) row {
	connection := c.connections[connectionID]
	if connection == nil || connection["deleted_at"] != nil || text(connection, "organization_id") != text(target, "organization_id") || text(connection, "owner_account_id") != text(target, "owner_account_id") {
		c.unavailableReference(target, former, "source_connection_unavailable")
		return nil
	}
	target = clone(target)
	target["connector_id"] = connection["connector_id"]
	key := targetKey(target)
	if existing := c.bindings[key]; existing != nil {
		if restriction != nil {
			c.narrowExistingBinding(existing, restriction)
		}
		if text(existing, "connection_id") != connectionID {
			c.plan.problem("connector_target_bindings", former, "multiple_connections_for_same_target")
		}
		return existing
	}
	constraint := c.restrictionConstraint(text(target, "owner_account_id"), text(connection, "connector_id"), restriction)
	id := deterministicID("binding", key)
	status := "active"
	if len(stringsOf(constraint["deny_reasons"])) > 0 {
		status = "needs_replacement"
		c.unavailableReference(target, former, "binding_restriction_unavailable")
	}
	b := clone(target)
	b["id"] = id
	b["connection_id"] = connectionID
	b["status"] = status
	b["compatibility"] = "native"
	b["compatibility_state"] = row{}
	b["binding_constraint"] = constraint
	b["revision"] = 1
	if text(b, "target_type") != "workflow" {
		b["binding_instance_id"] = nil
	}
	c.bindings[key] = b
	c.plan.add("connector_target_bindings", nil, b, true)
	return b
}
func (c *converter) convertHostBindings() {
	for _, source := range c.source["agent_resource_bindings"] {
		if text(source, "binding_type") != "integration_connection" {
			continue
		}
		kind := text(source, "consumer_type")
		if kind == "" {
			kind = "agent"
		}
		id := text(source, "consumer_id")
		if id == "" {
			id = text(source, "agent_id")
		}
		version := ""
		scope := text(source, "binding_scope")
		if scope == "published" {
			version = text(source, "published_version_uuid")
			if kind == "workflow" {
				id = version
			}
		}
		target := row{"organization_id": source["organization_id"], "owner_account_id": source["authorized_by"], "target_type": kind, "target_id": id, "target_scope": scope, "target_version_id": version, "binding_instance_id": text(source, "binding_instance_id"), "connector_id": source["parent_resource_id"]}
		b := c.binding(target, text(source, "resource_id"), text(source, "id"), object(source["metadata"]))
		if b != nil && (source["revoked_at"] != nil || source["deleted_at"] != nil || text(source, "status") == "revoked" || text(source, "status") == "disabled") {
			b["status"] = "disabled"
			constraint := object(b["binding_constraint"])
			addDeny(constraint, "source_binding_revoked")
			b["binding_constraint"] = constraint
			c.plan.add("connector_target_bindings", row{"id": b["id"]}, row{"status": "disabled", "binding_constraint": constraint}, false)
		}
		dest := ""
		result := "unavailable"
		if b != nil {
			dest = text(b, "id")
			result = "converted"
		}
		c.plan.record("host_binding", text(source, "id"), source, dest, result)
	}
}
func (c *converter) target(kind, id, scope, version, instance, org, owner string) row {
	return row{"target_type": kind, "target_id": id, "target_scope": scope, "target_version_id": version, "binding_instance_id": instance, "organization_id": org, "owner_account_id": owner}
}
func (c *converter) workspaceOrg(id string) string {
	return text(findBy(c.source["workspaces"], "id", id), "organization_id")
}
func (c *converter) hostOwner(source row) string {
	owner := text(source, "updated_by")
	if !validID(owner) {
		owner = text(source, "created_by")
	}
	return owner
}
func constraintValid(r row, owner string) bool {
	if integer(r, "schema_version") != 1 || text(r, "audience_owner") != owner || !validID(owner) || r["allowed_operation_ids"] == nil || r["target_boundaries"] == nil || r["deny_reasons"] == nil {
		return false
	}
	known := map[string]bool{"schema_version": true, "audience_owner": true, "allowed_operation_ids": true, "target_boundaries": true, "deny_reasons": true, "expires_at": true, "source_digest": true}
	for key := range r {
		if !known[key] {
			return false
		}
	}
	if value := text(r, "expires_at"); value != "" {
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return false
		}
	}
	for _, field := range []string{"allowed_operation_ids", "deny_reasons"} {
		values, ok := r[field].([]any)
		if typed, typedOK := r[field].([]string); typedOK {
			values, ok = make([]any, len(typed)), true
			for i, value := range typed {
				values[i] = value
			}
		}
		if !ok {
			return false
		}
		seen := map[string]bool{}
		for _, value := range values {
			id, ok := value.(string)
			if !ok || strings.TrimSpace(id) == "" || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	boundaries, ok := r["target_boundaries"].([]any)
	if !ok {
		return false
	}
	for _, value := range boundaries {
		b := object(value)
		if len(b) != 4 || (text(b, "type") != "chat" && text(b, "type") != "agent" && text(b, "type") != "workflow") || text(b, "id") == "" {
			return false
		}
		if scope := text(b, "scope"); scope != "*" && scope != "live" && scope != "draft" && scope != "published" {
			return false
		}
		if _, ok := b["version_id"].(string); !ok {
			return false
		}
	}
	if digest := text(r, "source_digest"); digest != "" && !digestPattern.MatchString(digest) {
		return false
	}
	return true
}

func (c *converter) narrowExistingBinding(binding row, restriction row) {
	prior := object(binding["binding_constraint"])
	next := intersectConstraints(prior, c.restrictionConstraint(text(binding, "owner_account_id"), text(binding, "connector_id"), restriction))
	if hash(prior) != hash(next) {
		binding["binding_constraint"] = next
		c.plan.add("connector_target_bindings", row{"id": binding["id"]}, row{"binding_constraint": next}, false)
	}
}
