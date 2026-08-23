package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const maxNativeToolNameLength = 64

var nativeToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type NativeToolBinding struct {
	SkillID          string                 `json:"skill_id"`
	ToolName         string                 `json:"tool_name"`
	ArgumentEnvelope string                 `json:"argument_envelope,omitempty"`
	FixedArguments   map[string]interface{} `json:"fixed_arguments,omitempty"`
}

// NativeToolProjection adds a provider-native function while preserving the
// existing Skill execution boundary described by Binding. It is used for
// request-scoped capabilities whose public schema is narrower than the bound
// Skill tool schema.
type NativeToolProjection struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Binding     NativeToolBinding
}

type NativeToolProjectionOptions struct {
	MaxTools       int
	EstimateTokens func([]llmadapter.Message, []llmadapter.Tool) int
}

type NativeToolSkip struct {
	ToolName       string `json:"tool_name"`
	Reason         string `json:"reason"`
	Detail         string `json:"detail,omitempty"`
	Budget         int    `json:"budget,omitempty"`
	Required       int    `json:"required,omitempty"`
	BudgetTokens   int    `json:"budget_tokens,omitempty"`
	RequiredTokens int    `json:"required_tokens,omitempty"`
}

type NativeSkillSkip struct {
	SkillID        string `json:"skill_id"`
	Reason         string `json:"reason"`
	Detail         string `json:"detail,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	Budget         int    `json:"budget,omitempty"`
	Required       int    `json:"required,omitempty"`
	BudgetTokens   int    `json:"budget_tokens,omitempty"`
	RequiredTokens int    `json:"required_tokens,omitempty"`
}

type NativeToolSet struct {
	ActiveSkillIDs      []string                     `json:"active_skill_ids"`
	InstructionMessages []llmadapter.Message         `json:"-"`
	ProviderTools       []llmadapter.Tool            `json:"-"`
	ToolBindings        map[string]NativeToolBinding `json:"tool_bindings"`
	SkippedSkills       []NativeSkillSkip            `json:"skipped_skills"`
	SkippedTools        []NativeToolSkip             `json:"skipped_tools,omitempty"`
	InstructionChars    int                          `json:"instruction_chars"`
	SchemaChars         int                          `json:"schema_chars"`
	BudgetChars         int                          `json:"budget_chars"`
	InstructionTokens   int                          `json:"instruction_tokens"`
	SchemaTokens        int                          `json:"schema_tokens"`
	BudgetTokens        int                          `json:"budget_tokens"`
}

const DefaultMaxNativeToolProjections = 24

type NativeToolSetOptions struct {
	TenantID              string
	BudgetChars           int
	BudgetTokens          int
	PrioritySkillIDs      []string
	SelectedSkillIDs      []string
	AlreadyActiveSkillIDs []string
	ReservedToolNames     []string
	AuthorizeSkill        func(context.Context, string) (bool, error)
	EstimateTokens        func([]llmadapter.Message, []llmadapter.Tool) int
}

type nativeSkillCandidate struct {
	tools             []llmadapter.Tool
	bindings          map[string]NativeToolBinding
	instruction       llmadapter.Message
	instructionChars  int
	schemaChars       int
	instructionTokens int
	schemaTokens      int
	totalTokens       int
}

// BuildNativeToolSet projects complete skill documents into one provider-native
// function set. A skill is activated atomically: all of its instructions and
// callable tools must be safe to expose in the same request.
func (r *Runtime) BuildNativeToolSet(ctx context.Context, resolved *ResolvedSkills, options NativeToolSetOptions) NativeToolSet {
	result := NativeToolSet{
		ToolBindings: make(map[string]NativeToolBinding),
		BudgetChars:  options.BudgetChars,
		BudgetTokens: options.BudgetTokens,
	}
	if resolved == nil {
		return result
	}

	docs := orderedNativeSkillDocuments(resolved.Skills, options.PrioritySkillIDs)
	allDocs := docs
	if len(options.SelectedSkillIDs) > 0 {
		docs = selectedNativeSkillDocuments(docs, options.SelectedSkillIDs)
	}
	reservedNames := nativeControlToolNames()
	for name, count := range nativeToolNameCounts(allDocs) {
		if count > 1 && validNativeToolName(name) {
			reservedNames[name] = struct{}{}
		}
	}
	for _, name := range options.ReservedToolNames {
		if name = strings.TrimSpace(name); name != "" {
			reservedNames[name] = struct{}{}
		}
	}
	active := make(map[string]struct{}, len(docs))
	for _, skillID := range options.AlreadyActiveSkillIDs {
		if skillID = normalizeSkillID(skillID); skillID != "" {
			active[skillID] = struct{}{}
		}
	}
	usedChars := 0
	usedTokens := 0
	for _, doc := range docs {
		skillID := normalizeSkillID(doc.Metadata.ID)
		if skillID == "" {
			continue
		}
		if options.AuthorizeSkill != nil {
			allowed, err := options.AuthorizeSkill(ctx, skillID)
			if err != nil || !allowed {
				detail := "skill is not authorized for this request"
				if err != nil {
					detail = err.Error()
				}
				result.SkippedSkills = append(result.SkippedSkills, NativeSkillSkip{SkillID: skillID, Reason: "unauthorized", Detail: detail})
				continue
			}
		}
		if nativeSkillRequiresPromptProfessionalizer(doc) {
			if _, ok := active[SkillPromptProfessionalizer]; !ok {
				result.SkippedSkills = append(result.SkippedSkills, NativeSkillSkip{SkillID: skillID, Reason: "missing_dependency", Detail: SkillPromptProfessionalizer})
				continue
			}
		}

		candidate, skip := r.nativeSkillCandidate(ctx, doc, options.TenantID, reservedNames, options.EstimateTokens)
		if skip != nil {
			result.SkippedSkills = append(result.SkippedSkills, *skip)
			continue
		}
		requiredChars := candidate.instructionChars + candidate.schemaChars
		if options.BudgetTokens > 0 && options.EstimateTokens != nil && usedTokens+candidate.totalTokens > options.BudgetTokens {
			result.SkippedSkills = append(result.SkippedSkills, NativeSkillSkip{
				SkillID:        skillID,
				Reason:         "context_budget_exceeded",
				BudgetTokens:   options.BudgetTokens - usedTokens,
				RequiredTokens: candidate.totalTokens,
			})
			continue
		}
		if options.BudgetChars > 0 && usedChars+requiredChars > options.BudgetChars {
			result.SkippedSkills = append(result.SkippedSkills, NativeSkillSkip{
				SkillID:  skillID,
				Reason:   "context_budget_exceeded",
				Budget:   options.BudgetChars - usedChars,
				Required: requiredChars,
			})
			continue
		}

		result.ActiveSkillIDs = append(result.ActiveSkillIDs, skillID)
		result.InstructionMessages = append(result.InstructionMessages, candidate.instruction)
		result.ProviderTools = append(result.ProviderTools, candidate.tools...)
		for name, binding := range candidate.bindings {
			result.ToolBindings[name] = binding
			reservedNames[name] = struct{}{}
		}
		result.InstructionChars += candidate.instructionChars
		result.SchemaChars += candidate.schemaChars
		result.InstructionTokens += candidate.instructionTokens
		result.SchemaTokens += candidate.schemaTokens
		usedChars += requiredChars
		usedTokens += candidate.totalTokens
		active[skillID] = struct{}{}
	}
	return result
}

// AppendNativeToolProjections adds bounded request-scoped functions to an
// already built native set. Existing Skill instructions and tools retain
// priority; projections consume only the remaining schema budget.
func AppendNativeToolProjections(
	toolSet *NativeToolSet,
	projections []NativeToolProjection,
	options NativeToolProjectionOptions,
) int {
	if toolSet == nil || len(projections) == 0 {
		return 0
	}
	if toolSet.ToolBindings == nil {
		toolSet.ToolBindings = make(map[string]NativeToolBinding)
	}
	maxTools := options.MaxTools
	if maxTools <= 0 || maxTools > DefaultMaxNativeToolProjections {
		maxTools = DefaultMaxNativeToolProjections
	}
	reservedNames := nativeControlToolNames()
	for _, tool := range toolSet.ProviderTools {
		if name := strings.TrimSpace(tool.Function.Name); name != "" {
			reservedNames[name] = struct{}{}
		}
	}
	localNames := map[string]struct{}{}
	added := 0
	for _, projection := range projections {
		requestedName := strings.TrimSpace(projection.Name)
		binding := cloneNativeToolBinding(projection.Binding)
		binding.SkillID = normalizeSkillID(binding.SkillID)
		binding.ToolName = strings.TrimSpace(binding.ToolName)
		binding.ArgumentEnvelope = strings.TrimSpace(binding.ArgumentEnvelope)
		if requestedName == "" || binding.SkillID == "" || binding.ToolName == "" {
			toolSet.SkippedTools = append(toolSet.SkippedTools, NativeToolSkip{
				ToolName: requestedName, Reason: "binding_invalid",
			})
			continue
		}
		if added >= maxTools {
			toolSet.SkippedTools = append(toolSet.SkippedTools, NativeToolSkip{
				ToolName: requestedName, Reason: "tool_limit_exceeded",
			})
			continue
		}
		schema := tools.ModelVisibleJSONSchema(projection.InputSchema)
		if !validNativeObjectSchema(schema) {
			toolSet.SkippedTools = append(toolSet.SkippedTools, NativeToolSkip{
				ToolName: requestedName, Reason: "schema_unavailable",
			})
			continue
		}
		providerName := nativeProviderToolName(binding.SkillID, requestedName, reservedNames, localNames)
		if providerName == "" {
			toolSet.SkippedTools = append(toolSet.SkippedTools, NativeToolSkip{
				ToolName: requestedName, Reason: "tool_name_invalid",
			})
			continue
		}
		description := strings.TrimSpace(projection.Description)
		if description == "" {
			description = "Use " + requestedName + " through the " + binding.SkillID + " runtime."
		}
		tool := llmadapter.Tool{Type: "function", Function: llmadapter.Function{
			Name: providerName, Description: description, Parameters: schema,
		}}
		encoded, err := json.Marshal(tool)
		if err != nil {
			toolSet.SkippedTools = append(toolSet.SkippedTools, NativeToolSkip{
				ToolName: requestedName, Reason: "schema_unavailable", Detail: err.Error(),
			})
			continue
		}
		requiredChars := len(encoded)
		if toolSet.BudgetChars > 0 && toolSet.InstructionChars+toolSet.SchemaChars+requiredChars > toolSet.BudgetChars {
			toolSet.SkippedTools = append(toolSet.SkippedTools, NativeToolSkip{
				ToolName: requestedName, Reason: "context_budget_exceeded",
				Budget: max(0, toolSet.BudgetChars-toolSet.InstructionChars-toolSet.SchemaChars), Required: requiredChars,
			})
			continue
		}
		requiredTokens := 0
		if options.EstimateTokens != nil {
			requiredTokens = options.EstimateTokens(nil, []llmadapter.Tool{tool})
		}
		if toolSet.BudgetTokens > 0 && toolSet.InstructionTokens+toolSet.SchemaTokens+requiredTokens > toolSet.BudgetTokens {
			toolSet.SkippedTools = append(toolSet.SkippedTools, NativeToolSkip{
				ToolName: requestedName, Reason: "context_budget_exceeded",
				BudgetTokens: max(0, toolSet.BudgetTokens-toolSet.InstructionTokens-toolSet.SchemaTokens), RequiredTokens: requiredTokens,
			})
			continue
		}
		toolSet.ProviderTools = append(toolSet.ProviderTools, tool)
		toolSet.ToolBindings[providerName] = binding
		toolSet.SchemaChars += requiredChars
		toolSet.SchemaTokens += requiredTokens
		reservedNames[providerName] = struct{}{}
		localNames[providerName] = struct{}{}
		added++
	}
	return added
}

func cloneNativeToolBinding(input NativeToolBinding) NativeToolBinding {
	out := input
	out.FixedArguments = copyStringAnyMap(input.FixedArguments)
	return out
}

func nativeToolNameCounts(docs []SkillDocument) map[string]int {
	counts := make(map[string]int)
	for _, doc := range docs {
		for _, definition := range doc.Tools {
			if name := strings.TrimSpace(definition.Name); name != "" {
				counts[name]++
			}
		}
	}
	return counts
}

func (r *Runtime) nativeSkillCandidate(ctx context.Context, doc SkillDocument, tenantID string, reservedNames map[string]struct{}, estimateTokens func([]llmadapter.Message, []llmadapter.Tool) int) (nativeSkillCandidate, *NativeSkillSkip) {
	skillID := normalizeSkillID(doc.Metadata.ID)
	candidate := nativeSkillCandidate{
		bindings: make(map[string]NativeToolBinding, len(doc.Tools)),
		instruction: llmadapter.Message{
			Role: "system",
			Content: strings.Join([]string{
				"Active skill: " + skillID,
				"Description: " + strings.TrimSpace(doc.Metadata.Description),
				"Instructions:",
				strings.TrimSpace(doc.Instructions),
			}, "\n"),
		},
	}
	candidate.instructionChars = len([]rune(fmt.Sprint(candidate.instruction.Content)))

	localNames := make(map[string]struct{}, len(doc.Tools))
	for _, definition := range doc.Tools {
		toolName := strings.TrimSpace(definition.Name)
		if toolName == "" {
			return nativeSkillCandidate{}, &NativeSkillSkip{SkillID: skillID, Reason: "tool_name_invalid"}
		}
		schema, description, ok := r.nativeToolSchema(ctx, skillID, definition, tenantID)
		if !ok {
			return nativeSkillCandidate{}, &NativeSkillSkip{SkillID: skillID, Reason: "schema_unavailable", ToolName: toolName}
		}
		providerName := nativeProviderToolName(skillID, toolName, reservedNames, localNames)
		if providerName == "" {
			return nativeSkillCandidate{}, &NativeSkillSkip{SkillID: skillID, Reason: "tool_name_invalid", ToolName: toolName}
		}
		tool := llmadapter.Tool{Type: "function", Function: llmadapter.Function{
			Name:        providerName,
			Description: description,
			Parameters:  schema,
		}}
		encoded, err := json.Marshal(tool)
		if err != nil {
			return nativeSkillCandidate{}, &NativeSkillSkip{SkillID: skillID, Reason: "schema_unavailable", ToolName: toolName, Detail: err.Error()}
		}
		candidate.schemaChars += len(encoded)
		candidate.tools = append(candidate.tools, tool)
		candidate.bindings[providerName] = NativeToolBinding{SkillID: skillID, ToolName: toolName}
		localNames[providerName] = struct{}{}
	}
	if estimateTokens != nil {
		candidate.instructionTokens = estimateTokens([]llmadapter.Message{candidate.instruction}, nil)
		candidate.schemaTokens = estimateTokens(nil, candidate.tools)
		candidate.totalTokens = estimateTokens([]llmadapter.Message{candidate.instruction}, candidate.tools)
	}
	return candidate, nil
}

func (r *Runtime) nativeToolSchema(ctx context.Context, skillID string, definition SkillToolDefinition, tenantID string) (map[string]interface{}, string, bool) {
	if contract, ok := SkillToolArgumentContractFor(skillID, definition.Name); ok && validNativeObjectSchema(contract.Schema) {
		description := strings.TrimSpace(contract.Description)
		if description == "" {
			description = "Use " + definition.Name + " from the " + skillID + " skill."
		}
		return copyStringAnyMap(contract.Schema), description, true
	}
	if r == nil || r.manager == nil {
		return nil, "", false
	}
	provider, err := r.manager.GetProvider(ctx, definition.ProviderType, definition.ProviderID, tenantID)
	if err != nil {
		return nil, "", false
	}
	tool, err := provider.GetTool(definition.Name)
	if err != nil {
		return nil, "", false
	}
	entity := tool.GetEntity()
	parameters := entity.Parameters
	if r.engine != nil {
		parameters, err = r.engine.GetToolParameters(ctx, definition.ProviderType, definition.ProviderID, definition.Name, tenantID)
	} else {
		parameters, err = tool.GetRuntimeParameters(ctx, nil, nil, nil)
	}
	if err != nil {
		parameters = entity.Parameters
	}
	schema, ok := nativeSchemaFromToolParameters(parameters)
	if !ok {
		return nil, "", false
	}
	description := strings.TrimSpace(entity.Description.LLM)
	if description == "" {
		description = strings.TrimSpace(entity.Description.Human.Get("en_US"))
	}
	if description == "" {
		description = "Use " + definition.Name + " from the " + skillID + " skill."
	}
	return schema, description, true
}

func nativeSchemaFromToolParameters(parameters []tools.ToolParameter) (map[string]interface{}, bool) {
	properties := make(map[string]interface{})
	required := make([]string, 0)
	for _, parameter := range parameters {
		if parameter.Form != "" && parameter.Form != tools.ToolParameterFormLLM {
			continue
		}
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			return nil, false
		}
		property := map[string]interface{}{}
		switch parameter.Type {
		case tools.ToolParameterTypeString, tools.ToolParameterTypeFile, "":
			property["type"] = "string"
		case tools.ToolParameterTypeNumber:
			property["type"] = "number"
		case tools.ToolParameterTypeBoolean:
			property["type"] = "boolean"
		case tools.ToolParameterTypeSelect:
			property["type"] = "string"
			values := make([]string, 0, len(parameter.Options))
			for _, option := range parameter.Options {
				if value := strings.TrimSpace(option.Value); value != "" {
					values = append(values, value)
				}
			}
			if len(values) > 0 {
				property["enum"] = values
			}
		default:
			return nil, false
		}
		if description := strings.TrimSpace(parameter.LLMDescription); description != "" {
			property["description"] = description
		}
		if parameter.Default != nil {
			property["default"] = parameter.Default
		}
		if parameter.MinValue != nil {
			property["minimum"] = *parameter.MinValue
		}
		if parameter.MaxValue != nil {
			property["maximum"] = *parameter.MaxValue
		}
		properties[name] = property
		if parameter.Required {
			required = append(required, name)
		}
	}
	schema := map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema, true
}

func validNativeObjectSchema(schema map[string]interface{}) bool {
	if len(schema) == 0 || !strings.EqualFold(strings.TrimSpace(fmt.Sprint(schema["type"])), "object") {
		return false
	}
	_, ok := schema["properties"].(map[string]interface{})
	return ok
}

func orderedNativeSkillDocuments(input []SkillDocument, priority []string) []SkillDocument {
	order := make(map[string]int, len(priority))
	for index, skillID := range priority {
		skillID = normalizeSkillID(skillID)
		if skillID == "" {
			continue
		}
		if _, exists := order[skillID]; !exists {
			order[skillID] = index
		}
	}
	out := append([]SkillDocument(nil), input...)
	sort.SliceStable(out, func(i, j int) bool {
		leftID := normalizeSkillID(out[i].Metadata.ID)
		rightID := normalizeSkillID(out[j].Metadata.ID)
		if leftID == SkillPromptProfessionalizer && rightID != SkillPromptProfessionalizer {
			return true
		}
		if rightID == SkillPromptProfessionalizer && leftID != SkillPromptProfessionalizer {
			return false
		}
		left, leftOK := order[leftID]
		right, rightOK := order[rightID]
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return false
		}
		return left < right
	})
	return out
}

func selectedNativeSkillDocuments(input []SkillDocument, selected []string) []SkillDocument {
	allowed := make(map[string]struct{}, len(selected))
	for _, skillID := range selected {
		if skillID = normalizeSkillID(skillID); skillID != "" {
			allowed[skillID] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	out := make([]SkillDocument, 0, len(allowed))
	for _, doc := range input {
		if _, ok := allowed[normalizeSkillID(doc.Metadata.ID)]; ok {
			out = append(out, doc)
		}
	}
	return out
}

func nativeSkillRequiresPromptProfessionalizer(doc SkillDocument) bool {
	for _, tool := range doc.Tools {
		if RequiresPromptProfessionalizerPreflight(doc.Metadata.ID, tool.Name) {
			return true
		}
	}
	return false
}

func nativeControlToolNames() map[string]struct{} {
	return map[string]struct{}{
		MetaToolRequestUserInput:   {},
		MetaToolTurnState:          {},
		MetaToolUpdatePlan:         {},
		MetaToolReadSkillReference: {},
		MetaToolActivateSkills:     {},
		MetaToolSearchSkills:       {},
	}
}

func nativeProviderToolName(skillID string, toolName string, reserved map[string]struct{}, local map[string]struct{}) string {
	original := strings.TrimSpace(toolName)
	if validNativeToolName(original) {
		if _, exists := reserved[original]; !exists {
			if _, duplicate := local[original]; !duplicate {
				return original
			}
		}
	}
	base := sanitizeNativeToolName(skillID) + "__" + sanitizeNativeToolName(original)
	if base == "__" {
		return ""
	}
	digest := sha256.Sum256([]byte(normalizeSkillID(skillID) + "/" + original))
	suffix := "_" + hex.EncodeToString(digest[:4])
	limit := maxNativeToolNameLength - len(suffix)
	if len(base) > limit {
		base = base[:limit]
	}
	name := strings.Trim(base, "_-") + suffix
	if !validNativeToolName(name) {
		return ""
	}
	if _, exists := reserved[name]; exists {
		return ""
	}
	if _, exists := local[name]; exists {
		return ""
	}
	return name
}

func sanitizeNativeToolName(value string) string {
	var builder strings.Builder
	for _, current := range strings.TrimSpace(value) {
		if unicode.IsLetter(current) || unicode.IsDigit(current) || current == '_' || current == '-' {
			if current <= unicode.MaxASCII {
				builder.WriteRune(current)
				continue
			}
		}
		builder.WriteByte('_')
	}
	return strings.Trim(builder.String(), "_-")
}

func validNativeToolName(name string) bool {
	return len(name) > 0 && len(name) <= maxNativeToolNameLength && nativeToolNamePattern.MatchString(name)
}
