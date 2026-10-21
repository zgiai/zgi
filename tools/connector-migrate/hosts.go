package main

import (
	"encoding/json"
	"time"
)

func (c *converter) convertHostSnapshots() {
	for _, source := range c.source["agents_configs"] {
		agent := findBy(c.source["agents"], "id", text(source, "agents_id"))
		mode := object(source["agent_mode"])
		if len(list(mode["integration_bindings"])) == 0 && len(list(mode["connector_bindings"])) == 0 {
			continue
		}
		target := c.target("agent", text(source, "agents_id"), "draft", "", "", c.workspaceOrg(text(agent, "tenant_id")), c.hostOwner(source))
		converted := c.agentConfig(mode, target)
		encoded, _ := json.Marshal(converted)
		c.plan.add("agents_configs", row{"id": source["id"]}, row{"agent_mode": string(encoded)}, false)
		c.plan.record("agent_draft", text(source, "id"), source, "", "converted")
	}
	for _, source := range c.source["agent_published_versions"] {
		config := object(source["config_snapshot"])
		if len(list(config["integration_bindings"])) == 0 && len(list(config["connector_bindings"])) == 0 {
			continue
		}
		target := c.target("agent", text(source, "agent_id"), "published", text(source, "version_uuid"), "", c.workspaceOrg(text(source, "workspace_id")), text(source, "created_by"))
		converted := c.agentConfig(config, target)
		converted["connector_target_scope"] = "published"
		converted["connector_target_version_id"] = source["version_uuid"]
		c.plan.add("agent_published_versions", row{"id": source["id"]}, row{"config_snapshot": converted}, false)
		c.plan.record("agent_published", text(source, "id"), source, "", "converted")
	}
	convertedWorkflows := map[string]row{}
	for _, source := range c.source["workflows"] {
		graph := object(source["graph"])
		if !hasConnectorNodes(graph) {
			convertedWorkflows[text(source, "id")] = source
			continue
		}
		w := clone(source)
		target := c.workflowTarget(w, c.hostOwner(w))
		converted := c.workflowGraph(graph, target)
		encoded, _ := json.Marshal(converted)
		w["graph"] = string(encoded)
		c.plan.add("workflows", row{"id": source["id"]}, row{"graph": w["graph"]}, false)
		c.plan.record("workflow", text(source, "id"), source, "", "converted")
		convertedWorkflows[text(w, "id")] = w
	}
	runWorkflows := map[string]row{}
	c.runWorkflows = runWorkflows
	for _, run := range c.source["workflow_runs"] {
		graph := object(run["graph"])
		if !hasConnectorNodes(graph) {
			continue
		}
		source := findBy(c.source["workflows"], "id", text(run, "workflow_id"))
		if source == nil {
			c.plan.problem("workflow_runs", text(run, "id"), "workflow_snapshot_missing")
			continue
		}
		w := convertedWorkflows[text(source, "id")]
		if hash(graph) != hash(object(source["graph"])) {
			// A historical draft run gets an immutable retained host snapshot. It may
			// never acquire today's edited draft bindings when a pause resumes.
			w = clone(source)
			w["id"] = deterministicID("workflow-run-host", text(run, "id"))
			w["version_uuid"] = deterministicID("workflow-run-version", text(run, "id"))
			w["version"] = "run-snapshot-" + text(run, "id")
			w["created_by"] = run["created_by"]
			w["updated_by"] = run["created_by"]
			converted := c.workflowGraph(graph, c.workflowTarget(w, text(run, "created_by")), c.workflowTarget(source, text(run, "created_by")))
			encoded, _ := json.Marshal(converted)
			w["graph"] = string(encoded)
			c.plan.add("workflows", nil, w, true)
		}
		runWorkflows[text(run, "id")] = w
		c.plan.add("workflow_runs", row{"id": run["id"]}, row{"workflow_id": w["id"], "version": w["version"], "graph": w["graph"]}, false)
		c.plan.record("workflow_run", text(run, "id"), run, "", "converted")
	}
	for _, pause := range c.source["workflow_run_pauses"] {
		w := runWorkflows[text(pause, "workflow_run_id")]
		if w == nil {
			continue
		}
		state := object(pause["state_json"])
		if len(state) == 0 {
			c.plan.problem("workflow_run_pauses", text(pause, "id"), "pause_snapshot_invalid")
			continue
		}
		var rewrite func(any) any
		rewrite = func(value any) any {
			switch typed := value.(type) {
			case map[string]any:
				out := row{}
				for key, v := range typed {
					if key == "workflow_id" {
						out[key] = w["id"]
					} else if key == "graph" {
						if _, ok := v.(string); ok {
							out[key] = w["graph"]
						} else {
							out[key] = object(w["graph"])
						}
					} else {
						out[key] = rewrite(v)
					}
				}
				return out
			case []any:
				out := []any{}
				for _, v := range typed {
					out = append(out, rewrite(v))
				}
				return out
			default:
				return value
			}
		}
		encoded, _ := json.Marshal(rewrite(map[string]any(state)))
		c.plan.add("workflow_run_pauses", row{"id": pause["id"]}, row{"state_json": string(encoded)}, false)
		c.plan.record("workflow_pause", text(pause, "id"), pause, "", "converted")
	}
}
func (c *converter) agentConfig(config row, target row) row {
	out := clone(config)
	result := []any{}
	seen := map[string]bool{}
	candidates := append(append([]any{}, list(config["connector_bindings"])...), list(config["integration_bindings"])...)
	for _, raw := range candidates {
		item := object(raw)
		connectionID := text(item, "connection_id")
		connector := text(item, "connector_id")
		if connector == "" {
			connector = text(item, "integration_id")
		}
		t := clone(target)
		t["connector_id"] = connector
		restriction := row{}
		for _, b := range c.bindings {
			if (text(b, "id") == text(item, "binding_id") || text(item, "binding_id") == "" && text(b, "target_type") == "agent" && text(b, "target_id") == text(t, "target_id") && text(b, "target_scope") == text(t, "target_scope") && text(b, "target_version_id") == text(t, "target_version_id")) && text(b, "connection_id") == connectionID && text(b, "organization_id") == text(t, "organization_id") {
				t["owner_account_id"] = b["owner_account_id"]
				constraint := object(b["binding_constraint"])
				restriction["native_constraint"] = clone(constraint)
				break
			}
		}
		if actions, ok := item["action_ids"]; ok {
			restriction["allowed_action_ids"] = actions
		}
		if actions, ok := item["allowed_action_ids"]; ok {
			restriction["allowed_action_ids"] = actions
		}
		b := c.binding(t, connectionID, connectionID, restriction)
		if b == nil {
			continue
		}
		if text(b, "status") != "active" {
			c.unavailableReference(t, connectionID, "binding_restriction_unavailable")
			continue
		}
		key := targetKey(b)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, row{"binding_id": b["id"], "revision": b["revision"], "connection_id": b["connection_id"], "connector_id": b["connector_id"]})
	}
	delete(out, "integration_bindings")
	out["connector_bindings"] = result
	if nested := object(out["runtime_config"]); len(nested) > 0 {
		delete(nested, "integration_bindings")
		nested["connector_bindings"] = result
		out["runtime_config"] = nested
	}
	return out
}
func hasConnectorNodes(graph row) bool {
	for _, raw := range list(graph["nodes"]) {
		data := object(object(raw)["data"])
		if text(data, "provider_type") == "connector" || data["connector"] != nil || data["connector_unavailable_ref"] != nil {
			return true
		}
	}
	return false
}
func (c *converter) workflowTarget(w row, owner string) row {
	scope, id, version := "draft", text(w, "id"), ""
	if text(w, "version") != "draft" {
		scope = "published"
		id = text(w, "version_uuid")
		version = id
	}
	return c.target("workflow", id, scope, version, "", c.workspaceOrg(text(w, "tenant_id")), owner)
}
func (c *converter) workflowGraph(graph row, target row, sources ...row) row {
	sourceTarget := target
	if len(sources) > 0 {
		sourceTarget = sources[0]
	}
	result := clone(graph)
	for _, raw := range list(result["nodes"]) {
		node := object(raw)
		data := object(node["data"])
		if text(data, "provider_type") != "connector" && data["connector"] == nil {
			continue
		}
		ref := object(data["connector"])
		connectionID := text(ref, "connection_id")
		if connectionID == "" {
			connectionID = text(data, "connection_id")
		}
		connector := text(ref, "connector_id")
		if connector == "" {
			connector = text(data, "provider_id")
		}
		operation := text(ref, "operation_id")
		if operation == "" {
			operation = text(data, "tool_name")
		}
		t := clone(target)
		t["binding_instance_id"] = node["id"]
		t["connector_id"] = connector
		// Prefer the exact persisted placement, including its original owner.
		restriction := row{}
		for _, candidate := range c.bindings {
			if text(candidate, "target_type") == "workflow" && text(candidate, "target_id") == text(sourceTarget, "target_id") && text(candidate, "target_scope") == text(sourceTarget, "target_scope") && text(candidate, "binding_instance_id") == text(t, "binding_instance_id") && text(candidate, "connection_id") == connectionID {
				t["owner_account_id"] = candidate["owner_account_id"]
				constraint := object(candidate["binding_constraint"])
				restriction["native_constraint"] = clone(constraint)
				break
			}
		}
		if refID := text(ref, "binding_id"); refID != "" {
			for _, old := range c.bindings {
				if text(old, "id") == refID && text(old, "connection_id") == connectionID && text(old, "target_type") == "workflow" && text(old, "binding_instance_id") == text(t, "binding_instance_id") && text(old, "organization_id") == text(t, "organization_id") {
					t["owner_account_id"] = old["owner_account_id"]
					restriction["native_constraint"] = clone(object(old["binding_constraint"]))
					break
				}
			}
		}

		op := c.operation(connector, operation)
		if connectionID == "" {
			delete(data, "connection_id")
			delete(data, "connector")
			node["data"] = data
			continue
		}
		var b row
		if op != nil {
			b = c.binding(t, connectionID, connectionID, restriction)
		}
		if b == nil || text(b, "status") != "active" || !constraintAllows(object(b["binding_constraint"]), b, text(op, "id"), time.Now()) || !constraintAllows(object(c.connections[connectionID]["connection_constraint"]), b, text(op, "id"), time.Now()) {
			unavailable := c.unavailableReference(t, connectionID, "operation_or_connection_unavailable")
			delete(data, "connector")
			data["connector_unavailable_ref"] = unavailable["id"]
		} else {
			data["connector"] = row{"binding_id": b["id"], "connection_id": b["connection_id"], "connector_id": b["connector_id"], "operation_id": op["id"]}
			data["provider_id"] = b["connector_id"]
			data["tool_name"] = op["tool_name"]
			delete(data, "connector_unavailable_ref")
		}
		delete(data, "connection_id")
		node["data"] = data
	}
	return result
}
func (c *converter) convertChatPreferences() {
	for _, preference := range c.source["aichat_integration_preferences"] {
		owner, org, connector := text(preference, "account_id"), text(preference, "organization_id"), text(preference, "integration_id")
		preferred := text(preference, "preferred_connection_id")
		for _, chat := range c.source["chat_runtime_conversations"] {
			if text(chat, "organization_id") != org || text(chat, "account_id") != owner || chat["deleted_at"] != nil || text(chat, "caller_type") != "aichat" {
				continue
			}
			// Legacy preferences use an exact workspace scope. An unscoped
			// preference is not a fallback for workspace-scoped conversations;
			// applying it there can both widen selection and collide with the
			// workspace's independently selected connection.
			if text(preference, "workspace_id") != text(chat, "workspace_id") {
				continue
			}
			target := c.target("chat", text(chat, "id"), "live", "", "", org, owner)
			target["connector_id"] = connector
			if preferred != "" {
				c.binding(target, preferred, text(preference, "id"), nil)
			}
		}
		c.plan.record("chat_preference", text(preference, "id"), preference, "", "converted")
	}
}
func (c *converter) convertSkills() {
	for _, source := range c.source["chat_runtime_custom_skills"] {
		manifest := object(source["manifest"])
		encoded, _ := json.Marshal(manifest)
		file := findBy(c.source["connector_skill_file_inventory"], "id", text(source, "id"))
		if text(source, "storage_path") != "" && file == nil {
			c.plan.problem("chat_runtime_custom_skills", text(source, "id"), "skill_file_inventory_required")
			continue
		}
		if !retiredSkillContract.Match(encoded) && (file == nil || text(file, "state") == "native") {
			c.plan.record("skill_manifest", text(source, "id"), source, "", "unchanged")
			continue
		}
		// Unrecognized executable metatool requirements cannot become ambient
		// Connector authority. Keep the skill visible with a clear repair reason.
		c.plan.add("chat_runtime_custom_skills", row{"id": source["id"]}, row{"status": "invalid", "validation_error": "connector_requirements_need_reimport"}, false)
		c.plan.record("skill_manifest", text(source, "id"), source, "", "unavailable")
	}
}
