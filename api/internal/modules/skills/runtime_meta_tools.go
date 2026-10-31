package skills

import (
	"sort"
	"strings"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type MetaToolOptions struct {
	RequireFinalPlanSnapshot bool
}

// NativeControlToolsForSession returns state, reference, and progressive
// discovery controls for one native skill session.
func NativeControlToolsForSession(resolved *ResolvedSkills, activeSkillIDs []string, exposedCandidateSkillIDs []string, searchAvailable bool) []llmadapter.Tool {
	return NativeControlToolsForRun(resolved, activeSkillIDs, exposedCandidateSkillIDs, searchAvailable, true)
}

// NativeControlToolsForRun is NativeControlToolsForSession for a run that may
// have no business action to plan. Without one, update_plan and
// submit_turn_state are left out (ADR 0043).
func NativeControlToolsForRun(resolved *ResolvedSkills, activeSkillIDs []string, exposedCandidateSkillIDs []string, searchAvailable bool, planning bool) []llmadapter.Tool {
	active := make(map[string]struct{}, len(activeSkillIDs))
	for _, skillID := range activeSkillIDs {
		if normalized := normalizeSkillID(skillID); normalized != "" {
			active[normalized] = struct{}{}
		}
	}
	result := []llmadapter.Tool{requestUserInputMetaTool()}
	if planning {
		result = append(result, turnStateMetaTool(), updatePlanMetaTool())
	}
	if len(exposedCandidateSkillIDs) > 0 {
		result = append(result, activateSkillsMetaTool(exposedCandidateSkillIDs))
	}
	if searchAvailable {
		result = append(result, searchSkillsMetaTool())
	}
	if skillIDs, paths := loadedReferenceOptions(resolved, active); len(skillIDs) > 0 && len(paths) > 0 {
		result = append(result, readReferenceMetaTool(skillIDs, paths))
	}
	return result
}

func activateSkillsMetaTool(skillIDs []string) llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolActivateSkills,
			Description: "Activate the smallest relevant set of candidate skills before using their instructions or business tools. Do not narrate this internal activation to the user.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"skill_ids": map[string]interface{}{
						"type":        "array",
						"description": "One to three candidate skill IDs to activate for the current task.",
						"items":       stringSchema("A candidate skill ID.", normalizeSkillIDs(skillIDs)),
						"minItems":    1,
						"maxItems":    MaxNativeSkillActivationBatch,
						"uniqueItems": true,
					},
				},
				"required":             []string{"skill_ids"},
				"additionalProperties": false,
			},
		},
	}
}

func searchSkillsMetaTool() llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolSearchSkills,
			Description: "Search omitted candidate skills only when the needed capability is not present in the compact candidate directory. Search does not activate a skill.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "A concise capability description, not the full user request.",
						"minLength":   2,
						"maxLength":   300,
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum candidate count. Defaults to 5 and is capped at 10.",
						"minimum":     1,
						"maximum":     MaxNativeSkillSearchLimit,
						"default":     DefaultNativeSkillSearchLimit,
					},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
	}
}

// updatePlanMetaTool is the model's own checklist for multi-step work: steps
// and their status, replaced whole on each call (ADR 0049).
func updatePlanMetaTool() llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolUpdatePlan,
			Description: "Keep a short checklist for work with three or more steps or several deliverables. Write it before starting, and update it as each step is done. Each call replaces the whole list; keep at most one step in_progress. Skip it for a single step or a direct answer.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"explanation": map[string]interface{}{"type": "string", "description": "Optional short reason for a change to the plan.", "maxLength": 500},
					"plan": map[string]interface{}{
						"type":     "array",
						"minItems": 1,
						"maxItems": 16,
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"step":   map[string]interface{}{"type": "string", "description": "What this step does, in a few words.", "maxLength": 200},
								"status": map[string]interface{}{"type": "string", "enum": []string{"pending", "in_progress", "completed", "skipped"}},
							},
							"required":             []string{"step", "status"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"plan"},
				"additionalProperties": false,
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
			Description: "Ask the user up to five short questions and pause the turn until they answer. Use it only when missing information blocks reliable progress.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "Shown with the questions: what you checked, why you need the answer, and what happens next.",
						"maxLength":   2000,
					},
					"questions": map[string]interface{}{
						"type":        "array",
						"description": "One to five questions; prefer one to three.",
						"maxItems":    5,
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"id": map[string]interface{}{
									"type":        "string",
									"description": "Optional short ID, not shown.",
									"maxLength":   80,
								},
								"question": map[string]interface{}{
									"type":        "string",
									"description": "The question.",
									"maxLength":   1000,
								},
								"options": map[string]interface{}{
									"type":        "array",
									"description": "Optional quick replies, each a definite answer. Never add vague ones such as Other or Not sure; the user can always type.",
									"maxItems":    5,
									"items": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"label": map[string]interface{}{
												"type":        "string",
												"description": "The answer.",
												"maxLength":   80,
											},
											"description": map[string]interface{}{
												"type":        "string",
												"description": "Optional short explanation.",
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
			Description: "Keep short structured state for this turn so it survives an approval, a question to the user, page navigation, or another pause. Files and tool results are recorded automatically: refer to their IDs and a short summary instead of copying them. Use working_fact for derived facts later steps must reuse exactly, and user_deliverable only for content the user should also see.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"items": map[string]interface{}{
						"type":        "array",
						"description": "One to eight items.",
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
									"description": "model_only for internal state, user_visible for deliverables.",
									"enum":        []string{"model_only", "user_visible", "audit"},
								},
								"key": map[string]interface{}{
									"type":        "string",
									"description": "Short key for later reuse.",
									"maxLength":   120,
								},
								"value": map[string]interface{}{
									"type":        "string",
									"description": "The fact, decision or reference, as short text or JSON. Keep user-given values exact; never copy whole documents.",
									"maxLength":   1024,
								},
								"title": map[string]interface{}{
									"type":        "string",
									"description": "Title of a user_deliverable.",
									"maxLength":   120,
								},
								"content": map[string]interface{}{
									"type":        "string",
									"description": "Short Markdown of a user_deliverable; refer to files by ID.",
									"maxLength":   1024,
								},
								"source": map[string]interface{}{
									"type":        "string",
									"description": "Optional source.",
									"maxLength":   200,
								},
								"used_for": map[string]interface{}{
									"type":        "array",
									"description": "Optional labels for where it is used.",
									"maxItems":    8,
									"items": map[string]interface{}{
										"type":      "string",
										"maxLength": 120,
									},
								},
								"confidence": map[string]interface{}{
									"type":        "number",
									"description": "Optional, 0 to 1.",
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
