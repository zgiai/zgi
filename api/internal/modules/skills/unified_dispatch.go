package skills

import (
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
	builder.WriteString("Tools of this skill, as signatures (\"?\" marks an optional field). Call each with zgi, command \"" + skillID + " <tool>\". If the input is rejected, the error gives the tool's full schema.")
	for _, name := range names {
		tool := tools[name]
		schema, _ := tool.Function.Parameters.(map[string]interface{})
		builder.WriteString("\n- " + compactSignature(name, schema) + "\n  " + strings.TrimSpace(tool.Function.Description))
		for _, line := range compactParameterNotes(schema) {
			builder.WriteString("\n  " + line)
		}
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

// compactSignature renders a tool's input as a short signature, such as
// generate_file(format: txt|md|csv, content: string, filename?: string). A
// unified-mode model reads it instead of the JSON schema, which the server
// still validates against (ADR 0044). Objects deeper than two levels are
// shown as {...}.
func compactSignature(name string, schema map[string]interface{}) string {
	return name + "(" + compactFields(schema, 0) + ")"
}

const compactSignatureDepth = 2

func compactFields(schema map[string]interface{}, depth int) string {
	properties, _ := schema["properties"].(map[string]interface{})
	if len(properties) == 0 {
		return ""
	}
	required := map[string]bool{}
	for _, name := range compactStrings(schema["required"]) {
		required[name] = true
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		if required[names[i]] != required[names[j]] {
			return required[names[i]]
		}
		return names[i] < names[j]
	})
	fields := make([]string, 0, len(names))
	for _, name := range names {
		field, _ := properties[name].(map[string]interface{})
		marker := "?"
		if required[name] {
			marker = ""
		}
		fields = append(fields, name+marker+": "+compactType(field, depth))
	}
	return strings.Join(fields, ", ")
}

func compactType(schema map[string]interface{}, depth int) string {
	if values := compactStrings(schema["enum"]); len(values) > 0 {
		if len(values) > 12 {
			values = append(values[:12], "...")
		}
		return strings.Join(values, "|")
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		if options, ok := schema[key].([]interface{}); ok && len(options) > 0 {
			kinds := make([]string, 0, len(options))
			for _, option := range options {
				branch, _ := option.(map[string]interface{})
				kinds = append(kinds, compactType(branch, depth))
			}
			return strings.Join(kinds, " | ")
		}
	}
	switch kind, _ := schema["type"].(string); kind {
	case "array":
		items, _ := schema["items"].(map[string]interface{})
		return compactType(items, depth) + "[]"
	case "object":
		if depth >= compactSignatureDepth {
			return "{...}"
		}
		if fields := compactFields(schema, depth+1); fields != "" {
			return "{" + fields + "}"
		}
		return "object"
	case "":
		return "any"
	default:
		return kind
	}
}

// compactParameterNotes keeps the descriptions of a tool's top-level fields,
// which carry most of its usage rules.
func compactParameterNotes(schema map[string]interface{}) []string {
	properties, _ := schema["properties"].(map[string]interface{})
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	notes := make([]string, 0, len(names))
	for _, name := range names {
		field, _ := properties[name].(map[string]interface{})
		if description, _ := field["description"].(string); strings.TrimSpace(description) != "" {
			notes = append(notes, name+": "+strings.TrimSpace(description))
		}
	}
	return notes
}

func compactStrings(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
