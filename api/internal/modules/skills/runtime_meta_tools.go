package skills

import (
	"sort"
	"strings"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func MetaTools() []llmadapter.Tool {
	return metaTools(true)
}

func MetaToolsForSkills(resolved *ResolvedSkills) []llmadapter.Tool {
	return metaTools(resolvedHasToolSkills(resolved))
}

type MetaToolOptions struct {
	RequireFinalPlanSnapshot bool
}

func MetaToolsForSkillState(resolved *ResolvedSkills, loadedSkillIDs map[string]struct{}) []llmadapter.Tool {
	return MetaToolsForSkillStateWithOptions(resolved, loadedSkillIDs, MetaToolOptions{})
}

func MetaToolsForSkillStateWithOptions(resolved *ResolvedSkills, loadedSkillIDs map[string]struct{}, options MetaToolOptions) []llmadapter.Tool {
	loaded := normalizedLoadedSkillIDs(loadedSkillIDs)
	tools := []llmadapter.Tool{
		requestUserInputMetaTool(),
		turnStateMetaTool(),
		updatePlanMetaTool(),
		intermediateAnswerMetaTool(),
		finalAnswerMetaToolWithOptions(options),
	}
	if skillIDs := unloadedSkillIDs(resolved, loaded); len(skillIDs) > 0 {
		tools = append([]llmadapter.Tool{loadSkillMetaTool(skillIDs)}, tools...)
	}
	if referenceSkillIDs, referencePaths := loadedReferenceOptions(resolved, loaded); len(referenceSkillIDs) > 0 && len(referencePaths) > 0 {
		tools = append(tools, readReferenceMetaTool(referenceSkillIDs, referencePaths))
	}
	if toolSkillIDs, toolNames, pairs, contracts, hasUntyped := loadedToolOptions(resolved, loaded); len(toolSkillIDs) > 0 && len(toolNames) > 0 {
		tools = append(tools, callSkillToolMetaTool(toolSkillIDs, toolNames, pairs, contracts, hasUntyped))
	}
	return tools
}

func metaTools(includeToolCaller bool) []llmadapter.Tool {
	tools := []llmadapter.Tool{
		loadSkillMetaTool(nil),
		readReferenceMetaTool(nil, nil),
		requestUserInputMetaTool(),
		turnStateMetaTool(),
		updatePlanMetaTool(),
		intermediateAnswerMetaTool(),
		finalAnswerMetaTool(),
	}
	if includeToolCaller {
		tools = append(tools, callSkillToolMetaTool(nil, nil, nil, nil, true))
	}
	return tools
}

func updatePlanMetaTool() llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolUpdatePlan,
			Description: "Replace the user-visible outcome contract only when the requested result structure changes, a failure invalidates the current route, or the user changes the goal. Ordinary tool success is reconciled automatically and must not trigger this tool. Prefer outcomes; plan is a compatibility projection.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"explanation": map[string]interface{}{"type": "string"},
					"plan": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"id":            map[string]interface{}{"type": "string"},
								"step":          map[string]interface{}{"type": "string"},
								"status":        map[string]interface{}{"type": "string", "enum": []string{"pending", "in_progress", "completed", "skipped"}},
								"evidence_refs": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"note":          map[string]interface{}{"type": "string"},
							},
							"required": []string{"step", "status"},
						},
					},
					"outcomes": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"id":                   map[string]interface{}{"type": "string"},
								"goal":                 map[string]interface{}{"type": "string"},
								"status":               map[string]interface{}{"type": "string", "enum": []string{"pending", "in_progress", "completed", "skipped"}},
								"target_resource_type": map[string]interface{}{"type": "string"},
								"target_resource_id":   map[string]interface{}{"type": "string"},
								"depends_on":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"capabilities":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"constraints":          map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"evidence_refs":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"required":             map[string]interface{}{"type": "boolean"},
							},
							"required": []string{"goal"},
						},
					},
				},
			},
		},
	}
}

func loadSkillMetaTool(skillIDs []string) llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolLoadSkill,
			Description: "Load the full instructions for an enabled skill before using that skill.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"skill_id": stringSchema("The enabled skill ID to load.", skillIDs),
				},
				"required": []string{"skill_id"},
			},
		},
	}
}

func readReferenceMetaTool(skillIDs []string, paths []string) llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolReadSkillReference,
			Description: "Read a reference document from a loaded skill when SKILL.md says it is relevant.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"skill_id": stringSchema("The loaded skill ID that owns the reference.", skillIDs),
					"path":     stringSchema("Reference path relative to the skill references directory.", paths),
				},
				"required": []string{"skill_id", "path"},
			},
		},
	}
}

func requestUserInputMetaTool() llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolRequestUserInput,
			Description: "Ask the user up to five concise questions and pause this turn until they answer. Provide options only when each option is a concrete, directly usable answer. Do not include vague options such as free choice, freestyle, not sure, depends, any, or other; the user can always type freely. Use this only when missing information or ambiguity blocks reliable progress.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "Optional user-visible explanation shown as the assistant message alongside the questions. Use this to briefly explain what has been checked, why user input is needed, and what will happen next. Do not include internal tool names, JSON, IDs, or parameter names.",
						"maxLength":   2000,
					},
					"questions": map[string]interface{}{
						"type":        "array",
						"description": "One to five user-visible questions. Prefer one to three questions, and only ask what blocks reliable progress.",
						"maxItems":    5,
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"id": map[string]interface{}{
									"type":        "string",
									"description": "Optional stable short identifier for the question. This is not shown to the user.",
									"maxLength":   80,
								},
								"question": map[string]interface{}{
									"type":        "string",
									"description": "The natural-language question to show to the user.",
									"maxLength":   1000,
								},
								"options": map[string]interface{}{
									"type":        "array",
									"description": "Optional concrete quick replies for this question. Every option must be a definite answer that can be used directly. Omit options for open-ended or uncertain questions.",
									"maxItems":    5,
									"items": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"label": map[string]interface{}{
												"type":        "string",
												"description": "Short user-visible option label containing a concrete answer, not a vague placeholder such as Other or Freestyle.",
												"maxLength":   80,
											},
											"description": map[string]interface{}{
												"type":        "string",
												"description": "Optional short explanation for this option.",
												"maxLength":   200,
											},
										},
										"required": []string{"label"},
									},
								},
							},
							"required": []string{"question"},
						},
					},
				},
				"required": []string{"message", "questions"},
			},
		},
	}
}

func turnStateMetaTool() llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolTurnState,
			Description: "Record concise structured state for this same AIChat turn. Use this as a state handoff before approvals, page navigation, refresh, or another phase when implicit working memory may become unreliable. Tool-produced files and resource results are recorded automatically; reference their IDs, digest, and concise summary instead of copying full documents or configuration payloads. Use working_fact for model-only derived facts that later steps must reuse exactly. Use user_deliverable only when content should also be visible to the user; submit_intermediate_answer remains a compatibility shortcut for that case.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"items": map[string]interface{}{
						"type":        "array",
						"description": "One to eight structured turn-state items.",
						"minItems":    1,
						"maxItems":    8,
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"kind": map[string]interface{}{
									"type":        "string",
									"description": "The item kind.",
									"enum":        []string{"working_fact", "user_deliverable", "decision", "assumption", "verification"},
								},
								"visibility": map[string]interface{}{
									"type":        "string",
									"description": "Use model_only for internal state; use user_visible only for user-facing deliverables.",
									"enum":        []string{"model_only", "user_visible", "audit"},
								},
								"key": map[string]interface{}{
									"type":        "string",
									"description": "Stable short key for later reuse, for example agent_theme or selected_file_content.",
									"maxLength":   120,
								},
								"value": map[string]interface{}{
									"type":        "string",
									"description": "The concise fact, decision, assumption, verification result, or structured reference serialized as short JSON. Keep exact short user-derived values exact; never copy full documents or complete configuration payloads.",
									"maxLength":   1024,
								},
								"title": map[string]interface{}{
									"type":        "string",
									"description": "Short user-facing title when kind is user_deliverable.",
									"maxLength":   120,
								},
								"content": map[string]interface{}{
									"type":        "string",
									"description": "Concise Markdown content when kind is user_deliverable. Save full documents as artifacts and reference them by file ID instead.",
									"maxLength":   1024,
								},
								"source": map[string]interface{}{
									"type":        "string",
									"description": "Optional source, such as file-reader/read_file or page_context.",
									"maxLength":   200,
								},
								"used_for": map[string]interface{}{
									"type":        "array",
									"description": "Optional later use labels, such as agent.name or agent.prompt.",
									"maxItems":    8,
									"items": map[string]interface{}{
										"type":      "string",
										"maxLength": 120,
									},
								},
								"confidence": map[string]interface{}{
									"type":        "number",
									"description": "Optional confidence from 0 to 1.",
									"minimum":     0,
									"maximum":     1,
								},
							},
							"required": []string{"kind"},
						},
					},
				},
				"required": []string{"items"},
			},
		},
	}
}

func intermediateAnswerMetaTool() llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolIntermediateAnswer,
			Description: "Submit a substantial new intermediate answer or draft that should be visible to the user before continuing with more skill/tool calls. Do not use this to repeat content that was already visible in an earlier assistant answer; for export/save/convert/file-generation requests, pass the existing content directly to the relevant tool instead.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"description": "A short title for the intermediate answer, such as Novel outline or Draft plan.",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "The markdown content of the intermediate answer or draft.",
					},
				},
				"required": []string{"content"},
			},
		},
	}
}

func finalAnswerMetaTool() llmadapter.Tool {
	return finalAnswerMetaToolWithOptions(MetaToolOptions{})
}

func finalAnswerMetaToolWithOptions(options MetaToolOptions) llmadapter.Tool {
	description := "Submit the final user-facing answer and end the current skill loop when you judge the task complete or have honestly reached a terminal outcome. This call is terminal: do not combine it with business tools or request_user_input. Put the complete final response in answer; ordinary assistant content is progress, not the final answer. A plan snapshot is optional audit metadata and never determines whether the answer is accepted."
	required := []string{"answer"}
	if options.RequireFinalPlanSnapshot {
		description = "Submit the final user-facing answer and the latest execution plan snapshot in the same call when you judge the task complete or have honestly reached a terminal outcome. This call is terminal: do not combine it with business tools or request_user_input. Put the complete final response in answer; ordinary assistant content is progress, not the final answer. The plan is required for synchronization but remains advisory and never determines whether the answer is accepted."
		required = append(required, "plan")
	}
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolFinalAnswer,
			Description: description,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"answer": map[string]interface{}{
						"type":        "string",
						"description": "The complete final answer shown to the user, in the same language as the latest user request.",
					},
					"explanation": map[string]interface{}{
						"type":        "string",
						"description": "Optional concise explanation for the final plan update. This is audit metadata and is not shown as the answer.",
						"maxLength":   500,
					},
					"plan": planSnapshotSchema(),
				},
				"required": required,
			},
		},
	}
}

func planSnapshotSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Optional execution plan snapshot for audit. It does not determine whether the final answer is accepted.",
		"maxItems":    16,
		"items": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":            map[string]interface{}{"type": "string"},
				"step":          map[string]interface{}{"type": "string"},
				"status":        map[string]interface{}{"type": "string", "enum": []string{"pending", "in_progress", "completed", "skipped"}},
				"evidence_refs": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"note":          map[string]interface{}{"type": "string"},
				"expected_action": map[string]interface{}{
					"type":        "object",
					"description": "Optional structured action expected to complete this phase. Use exact loaded skill/tool IDs and stable target resource IDs when known.",
					"properties": map[string]interface{}{
						"skill_id":  map[string]interface{}{"type": "string"},
						"tool_name": map[string]interface{}{"type": "string"},
						"target": map[string]interface{}{
							"type":                 "object",
							"additionalProperties": map[string]interface{}{"type": "string"},
						},
					},
					"required": []string{"skill_id", "tool_name"},
				},
			},
			"required": []string{"step", "status"},
		},
	}
}

func callSkillToolMetaTool(skillIDs []string, toolNames []string, pairs []string, contracts []SkillToolArgumentContract, hasUntypedTools bool) llmadapter.Tool {
	description := "Call a tool allowed by a loaded skill after reading its instructions."
	if len(pairs) > 0 {
		description += " Allowed skill/tool pairs: " + strings.Join(pairs, "; ") + "."
	}
	argumentsSchema := callSkillToolArgumentsSchema(contracts, hasUntypedTools)
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolCallSkillTool,
			Description: description,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"skill_id":      stringSchema("The loaded skill ID that allows the tool.", skillIDs),
					"tool_name":     stringSchema("The allowed tool name to call.", toolNames),
					"plan_phase_id": map[string]interface{}{"type": "string", "description": "Optional outcome-phase ID. Include it only when this tool's successful result is sufficient to complete that phase; omit it for prerequisite reads, inspections, and helper calls."},
					"completion_intent": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"continue", "finalize_if_success"},
						"description": "Use finalize_if_success only when this exact tool call is the final remaining user-requested effect and all prerequisites are already complete. It never bypasses approval or proves success by itself.",
					},
					"arguments": argumentsSchema,
				},
				"required": []string{"skill_id", "tool_name", "arguments"},
			},
		},
	}
}

func callSkillToolArgumentsSchema(contracts []SkillToolArgumentContract, hasUntypedTools bool) map[string]interface{} {
	schema := map[string]interface{}{
		"type":        "object",
		"description": "Arguments for the selected skill tool. Pass a non-empty object that satisfies the selected tool's required parameters.",
	}
	if len(contracts) == 0 {
		return schema
	}
	options := make([]interface{}, 0, len(contracts)+1)
	for _, contract := range contracts {
		if len(contract.Schema) == 0 {
			continue
		}
		options = append(options, contract.Schema)
	}
	if hasUntypedTools {
		options = append(options, map[string]interface{}{
			"type":        "object",
			"description": "Arguments for a skill tool that does not expose a structured argument schema.",
		})
	}
	if len(options) == 0 {
		return schema
	}
	if hasUntypedTools || hasOptionalOnlyContract(contracts) {
		schema["anyOf"] = options
	} else {
		schema["oneOf"] = options
	}
	return schema
}

func hasOptionalOnlyContract(contracts []SkillToolArgumentContract) bool {
	for _, contract := range contracts {
		required, _ := contract.Schema["required"].([]string)
		if len(required) == 0 {
			return true
		}
	}
	return false
}

func stringSchema(description string, values []string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":        "string",
		"description": description,
	}
	if len(values) > 0 {
		schema["enum"] = values
	}
	return schema
}

func resolvedSkillIDs(resolved *ResolvedSkills) []string {
	if resolved == nil {
		return nil
	}
	ids := make([]string, 0, len(resolved.Skills))
	for _, doc := range resolved.Skills {
		if id := normalizeSkillID(doc.Metadata.ID); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func normalizedLoadedSkillIDs(loadedSkillIDs map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(loadedSkillIDs))
	for raw := range loadedSkillIDs {
		id := normalizeSkillID(raw)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func unloadedSkillIDs(resolved *ResolvedSkills, loaded map[string]struct{}) []string {
	ids := resolvedSkillIDs(resolved)
	if len(ids) == 0 || len(loaded) == 0 {
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := loaded[normalizeSkillID(id)]; ok {
			continue
		}
		out = append(out, id)
	}
	return out
}

func loadedReferenceOptions(resolved *ResolvedSkills, loaded map[string]struct{}) ([]string, []string) {
	if resolved == nil || len(loaded) == 0 {
		return nil, nil
	}
	skillSeen := map[string]struct{}{}
	pathSeen := map[string]struct{}{}
	skillIDs := []string{}
	paths := []string{}
	for _, doc := range resolved.Skills {
		skillID := normalizeSkillID(doc.Metadata.ID)
		if _, ok := loaded[skillID]; !ok || len(doc.Metadata.References) == 0 {
			continue
		}
		if _, exists := skillSeen[skillID]; !exists {
			skillSeen[skillID] = struct{}{}
			skillIDs = append(skillIDs, skillID)
		}
		for _, ref := range doc.Metadata.References {
			path := strings.TrimSpace(ref.Path)
			if path == "" {
				continue
			}
			if _, exists := pathSeen[path]; exists {
				continue
			}
			pathSeen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	sort.Strings(skillIDs)
	sort.Strings(paths)
	return skillIDs, paths
}

func loadedToolOptions(resolved *ResolvedSkills, loaded map[string]struct{}) ([]string, []string, []string, []SkillToolArgumentContract, bool) {
	if resolved == nil || len(loaded) == 0 {
		return nil, nil, nil, nil, false
	}
	skillSeen := map[string]struct{}{}
	toolSeen := map[string]struct{}{}
	skillIDs := []string{}
	toolNames := []string{}
	pairs := []string{}
	contracts := []SkillToolArgumentContract{}
	hasUntyped := false
	for _, doc := range resolved.Skills {
		skillID := normalizeSkillID(doc.Metadata.ID)
		if _, ok := loaded[skillID]; !ok || len(doc.Tools) == 0 {
			continue
		}
		if _, exists := skillSeen[skillID]; !exists {
			skillSeen[skillID] = struct{}{}
			skillIDs = append(skillIDs, skillID)
		}
		docToolNames := make([]string, 0, len(doc.Tools))
		for _, tool := range doc.Tools {
			name := strings.TrimSpace(tool.Name)
			if name == "" {
				continue
			}
			docToolNames = append(docToolNames, name)
			if _, exists := toolSeen[name]; !exists {
				toolSeen[name] = struct{}{}
				toolNames = append(toolNames, name)
			}
			if contract, ok := SkillToolArgumentContractFor(skillID, name); ok {
				contracts = append(contracts, contract)
			} else {
				hasUntyped = true
			}
		}
		sort.Strings(docToolNames)
		if len(docToolNames) > 0 {
			pairs = append(pairs, skillID+": "+strings.Join(docToolNames, ", "))
		}
	}
	sort.Strings(skillIDs)
	sort.Strings(toolNames)
	sort.Strings(pairs)
	sort.Slice(contracts, func(i, j int) bool {
		left := contracts[i].SkillID + "/" + contracts[i].ToolName
		right := contracts[j].SkillID + "/" + contracts[j].ToolName
		return left < right
	})
	return skillIDs, toolNames, pairs, contracts, hasUntyped
}
