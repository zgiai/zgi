package skills

func agentManagementListAgentsContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    "list_agents",
		Description: "List Agents visible to the current AIChat user in the current workspace.",
		Schema: objectSchema(
			map[string]interface{}{
				"workspace_id": stringValueSchema("Optional workspace ID. Usually omit so current AIChat workspace context is used."),
				"keyword":      stringValueSchema("Optional search keyword for Agent name or description."),
				"limit":        numberSchema("Optional maximum result count."),
			},
			nil,
		),
		Example: map[string]interface{}{"limit": 20},
	}
}

func agentManagementAgentIDContract(toolName string, description string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    toolName,
		Description: description,
		Schema: objectSchema(
			map[string]interface{}{
				"agent_id": stringValueSchema("Required Agent ID from page context, list_agents, create_agent, get_agent_config, or governed asset resolution. Do not invent IDs."),
			},
			[]string{"agent_id"},
		),
		Example: map[string]interface{}{"agent_id": "agent-id"},
	}
}

func agentManagementCreateAgentContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    "create_agent",
		Description: "Create one draft AGENT asset in the current workspace. This does not publish the Agent or configure model, prompt, upload, memory, skills, knowledge, databases, or workflows.",
		Schema: objectSchema(
			map[string]interface{}{
				"name":            stringValueSchema("Required Agent name shown in the Agent list."),
				"description":     stringValueSchema("Optional Agent description."),
				"icon_type":       enumStringSchema("Optional icon type.", []string{"text", "image"}),
				"icon":            stringValueSchema("Optional icon value. For text icons pass visible text such as AI, BOT, or an emoji."),
				"icon_background": stringValueSchema("Optional text icon background color such as #0f766e."),
				"workspace_id":    stringValueSchema("Optional target workspace ID. Usually omit so current AIChat workspace context is used."),
			},
			[]string{"name"},
		),
		Example: map[string]interface{}{"name": "小说创作大师", "description": "帮助用户创作小说的草稿智能体"},
	}
}

func agentManagementUpdateIdentityContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    "update_agent_identity",
		Description: "Update one resolved Agent's name, description, or icon. This does not publish the Agent.",
		Schema: objectSchema(
			map[string]interface{}{
				"agent_id":        stringValueSchema("Required Agent ID from page context, list_agents, create_agent, get_agent_config, or governed asset resolution. Do not invent IDs."),
				"name":            stringValueSchema("Optional new Agent name."),
				"description":     stringValueSchema("Optional new Agent description."),
				"icon_type":       enumStringSchema("Optional icon type.", []string{"text", "image"}),
				"icon":            stringValueSchema("Optional new icon value. For text icons pass visible text such as AI, BOT, or an emoji."),
				"icon_background": stringValueSchema("Optional text icon background color such as #0f766e."),
			},
			[]string{"agent_id"},
		),
		Example: map[string]interface{}{"agent_id": "agent-id", "name": "客服智能体"},
	}
}

func agentManagementDeleteAgentsContract() SkillToolArgumentContract {
	agentItem := objectSchema(
		map[string]interface{}{
			"agent_id":     stringValueSchema("Resolved Agent ID."),
			"id":           stringValueSchema("Optional resolved Agent ID alias."),
			"name":         stringValueSchema("Visible Agent name."),
			"agent_name":   stringValueSchema("Optional visible Agent name alias."),
			"workspace_id": stringValueSchema("Optional workspace ID."),
		},
		[]string{"agent_id"},
	)
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    "delete_agents",
		Description: "Delete multiple resolved Agent assets as one governed frozen batch.",
		Schema: objectSchema(
			map[string]interface{}{
				"agents":    arraySchema("Required frozen target Agents. Each item should include agent_id and visible name.", agentItem),
				"agent_ids": stringArrayOrCSVSchema("Optional fallback ID list when agents is unavailable. Prefer agents so approval cards show names."),
			},
			[]string{"agents"},
		),
		Example: map[string]interface{}{
			"agents": []map[string]interface{}{
				{"agent_id": "agent-1", "name": "Agent A"},
				{"agent_id": "agent-2", "name": "Agent B"},
			},
		},
	}
}

func agentManagementUpdateConfigContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    "update_agent_config",
		Description: "Patch selected draft runtime configuration fields for one resolved AGENT asset. Omitted fields are preserved. One call may update model, prompt, file upload, suggested questions, and add/remove bindings. System-prompt changes must send the complete final prompt after preserving unrelated current content and applying the user's requested transformation.",
		Schema: objectSchema(
			map[string]interface{}{
				"agent_id":                     stringValueSchema("Required Agent ID from page context, create_agent result, get_agent_config, or governed asset resolution. Do not invent IDs."),
				"system_prompt":                stringValueSchema("Optional complete replacement system prompt. Read the current config first, preserve every unrelated part, apply only the requested change, and send the full final prompt. Treat source material as input rather than content to copy by default; match the user's requested scope and level of detail, and reproduce it verbatim only when explicitly requested."),
				"model_provider":               stringValueSchema("Required whenever model is provided. Use the provider returned by list_available_models."),
				"model":                        stringValueSchema("Optional replacement model ID. Provide model_provider from the same list_available_models item."),
				"model_parameters":             objectSchema(map[string]interface{}{}, nil),
				"enabled_skill_ids":            stringArrayOrCSVSchema("Optional full list of enabled user-selectable skill IDs. Use [] to clear all user-selectable skills."),
				"add_enabled_skill_ids":        stringArrayOrCSVSchema("Optional skill IDs to add while preserving current enabled skills."),
				"remove_enabled_skill_ids":     stringArrayOrCSVSchema("Optional skill IDs to remove while preserving other enabled skills."),
				"agent_memory_enabled":         booleanSchema("Optional Agent memory switch."),
				"file_upload_enabled":          booleanSchema("Optional file upload switch."),
				"home_title":                   stringValueSchema("Optional Agent home title."),
				"opening_statement":            stringValueSchema("Optional Markdown landing guide shown before the first message."),
				"input_placeholder":            stringValueSchema("Optional chat input placeholder."),
				"theme_color":                  enumStringSchema("Optional theme color.", []string{"default", "blue", "emerald", "violet", "rose", "amber", "slate"}),
				"suggested_questions":          stringArrayOrCSVSchema("Optional full list of suggested questions."),
				"knowledge_dataset_ids":        stringArrayOrCSVSchema("Optional full replacement list of knowledge dataset IDs. Use [] to clear knowledge bindings."),
				"add_knowledge_dataset_ids":    stringArrayOrCSVSchema("Optional knowledge dataset IDs to add while preserving existing knowledge bindings."),
				"remove_knowledge_dataset_ids": stringArrayOrCSVSchema("Optional knowledge dataset IDs to unbind while preserving other knowledge bindings."),
				"knowledge_retrieval_config":   objectSchema(map[string]interface{}{}, nil),
				"database_bindings":            stringValueSchema("Optional JSON array replacing database bindings. Use [] to clear."),
				"add_database_bindings":        stringValueSchema("Optional JSON array of database table bindings to add."),
				"remove_database_bindings":     stringValueSchema("Optional JSON array of database table bindings to remove."),
				"workflow_bindings":            stringValueSchema("Optional JSON array replacing workflow bindings. Use [] to clear."),
				"add_workflow_bindings":        stringValueSchema("Optional JSON array of workflow bindings to add."),
				"remove_workflow_bindings":     stringValueSchema("Optional JSON array of workflow bindings to remove."),
				"display_names":                objectSchema(map[string]interface{}{}, nil),
			},
			[]string{"agent_id"},
		),
		Example: map[string]interface{}{
			"agent_id":              "agent-id",
			"model_provider":        "deepseek",
			"model":                 "deepseek-v4-flash",
			"file_upload_enabled":   true,
			"add_enabled_skill_ids": []string{"file-generator"},
		},
	}
}

func agentManagementReplaceMemorySlotsContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    "replace_agent_memory_slots",
		Description: "Replace the complete draft Agent memory slot list for one resolved AGENT asset.",
		Schema: objectSchema(
			map[string]interface{}{
				"agent_id":           stringValueSchema("Required Agent ID from page context, create_agent result, get_agent_config, or governed asset resolution. Do not invent IDs."),
				"agent_memory_slots": stringValueSchema("Required JSON array replacing all memory slots. Use [] to clear slots."),
			},
			[]string{"agent_id", "agent_memory_slots"},
		),
		Example: map[string]interface{}{"agent_id": "agent-id", "agent_memory_slots": "[]"},
	}
}

func agentManagementBindingCandidateContract(toolName string, description string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    toolName,
		Description: description,
		Schema: objectSchema(
			map[string]interface{}{
				"agent_id":         stringValueSchema("Required Agent ID from page context, create_agent result, get_agent_config, or governed asset resolution. Do not invent IDs."),
				"query":            stringValueSchema("Optional search query for narrowing candidates."),
				"limit":            numberSchema("Optional maximum result count."),
				"include_selected": booleanSchema("Optional. Defaults to true. Set false to exclude already selected resources."),
			},
			[]string{"agent_id"},
		),
		Example: map[string]interface{}{"agent_id": "agent-id", "query": "file generation"},
	}
}

func agentManagementListAvailableModelsContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentManagement,
		ToolName:    "list_available_models",
		Description: "List Agent runtime model candidates available to the current organization. Use this before changing an Agent model, then pass one returned item's provider and model together to update_agent_config.",
		Schema: objectSchema(
			map[string]interface{}{
				"use_case": enumStringSchema("Optional model use case. Defaults to agent for normal Agent runtime replacement. Use all only when the user asks to inspect every model.", []string{"agent", "workflow", "text-chat", "reasoning", "vision", "function-calling", "all"}),
				"provider": stringValueSchema("Optional provider slug filter, such as openai or deepseek."),
				"limit":    numberSchema("Optional maximum number of model candidates. Defaults to 20 and is capped by the backend."),
			},
			nil,
		),
		Example: map[string]interface{}{"use_case": "agent", "limit": 20},
	}
}
