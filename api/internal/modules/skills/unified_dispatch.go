package skills

import (
	"encoding/json"
	"sort"
	"strings"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// UnifiedSkillToolName is the one tool through which a run in unified mode
// calls the business tools of activatable skills (ADR 0044).
const UnifiedSkillToolName = "zgi"

// UnifiedSkillTool is the definition of the unified tool. It names no skill,
// tool or schema, so it is the same in every request.
func UnifiedSkillTool() llmadapter.Tool {
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        UnifiedSkillToolName,
			Description: "Run a business tool of a skill. The skill's instructions list its tools and their input; activate a candidate skill first to see them.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The skill ID and tool name, separated by a space, such as \"file-generator generate_file\".",
					},
					"input": map[string]interface{}{
						"type":        "object",
						"description": "The tool's arguments, as its input schema describes.",
					},
				},
				"required":             []string{"command", "input"},
				"additionalProperties": false,
			},
		},
	}
}

// UnifiedDispatchExposesTool reports whether a projected tool stays a native
// function in unified mode: resident tools and connector operations do; the
// tools of activatable skills go through the unified tool (ADR 0044).
func UnifiedDispatchExposesTool(binding NativeToolBinding) bool {
	return binding.ConnectorOperation != nil || IsResidentSkill(binding.SkillID) || normalizeSkillID(binding.SkillID) == SkillConnectorRuntime
}

// withUnifiedToolUsage adds a skill's tools, with their input schemas, to its
// instructions, which are the only place a unified-mode model learns them.
func withUnifiedToolUsage(skillID string, candidate nativeSkillCandidate, estimateTokens func([]llmadapter.Message, []llmadapter.Tool) int) nativeSkillCandidate {
	if candidate.instructionChars == 0 || len(candidate.tools) == 0 || normalizeSkillID(skillID) == SkillConnectorRuntime {
		return candidate
	}
	names := make([]string, 0, len(candidate.tools))
	tools := make(map[string]llmadapter.Tool, len(candidate.tools))
	for _, tool := range candidate.tools {
		binding, ok := candidate.bindings[tool.Function.Name]
		if !ok || binding.ConnectorOperation != nil {
			continue
		}
		names = append(names, binding.ToolName)
		tools[binding.ToolName] = tool
	}
	if len(names) == 0 {
		return candidate
	}
	sort.Strings(names)
	var builder strings.Builder
	builder.WriteString("Tools of this skill. Call each with zgi, command \"" + skillID + " <tool>\" and input matching its schema:")
	for _, name := range names {
		tool := tools[name]
		schema, err := json.Marshal(tool.Function.Parameters)
		if err != nil {
			schema = []byte("{}")
		}
		builder.WriteString("\n- " + name + ": " + strings.TrimSpace(tool.Function.Description) + "\n  input schema: " + string(schema))
	}
	content, _ := candidate.instruction.Content.(string)
	candidate.instruction.Content = strings.TrimSpace(content) + "\n\n" + builder.String()
	candidate.instructionChars = len([]rune(candidate.instruction.Content.(string)))
	if estimateTokens != nil {
		candidate.instructionTokens = estimateTokens([]llmadapter.Message{candidate.instruction}, nil)
		candidate.totalTokens = estimateTokens([]llmadapter.Message{candidate.instruction}, candidate.tools)
	}
	return candidate
}
