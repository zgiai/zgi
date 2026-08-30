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

// NativeExternalActionPreparationHint is server-owned projection metadata. It
// allows the runtime to attest which successful read result paths may supply a
// projected mutation target without trusting model-authored identifiers.
type NativeExternalActionPreparationHint struct {
	ActionID        string   `json:"action_id"`
	Relation        string   `json:"relation"`
	TargetArguments []string `json:"target_arguments"`
	ResultPaths     []string `json:"result_paths"`
	ResultTransform string   `json:"result_transform,omitempty"`
}

// NativeExternalActionOptionalTargetArgument is derived only from the
// server-owned Action input schema. It records a conditional branch where one
// success-deduplication target is not required. Model-authored schemas or plan
// fields can never create or weaken this condition.
type NativeExternalActionOptionalTargetArgument struct {
	Path               string      `json:"path"`
	WhenArgument       string      `json:"when_argument"`
	WhenEquals         interface{} `json:"when_equals"`
	DiscardWhenMatched bool        `json:"discard_when_matched,omitempty"`
}

type NativeToolBinding struct {
	SkillID              string                                       `json:"skill_id"`
	ToolName             string                                       `json:"tool_name"`
	Effect               string                                       `json:"effect,omitempty"`
	IntentMatched        bool                                         `json:"intent_matched,omitempty"`
	IntentGroup          string                                       `json:"intent_group,omitempty"`
	IntentTokens         []string                                     `json:"intent_tokens,omitempty"`
	BindingFingerprint   string                                       `json:"binding_fingerprint,omitempty"`
	ConnectionBinding    string                                       `json:"-"`
	Pinned               bool                                         `json:"pinned,omitempty"`
	ProjectionPriority   int                                          `json:"-"`
	ArgumentEnvelope     string                                       `json:"argument_envelope,omitempty"`
	FixedArguments       map[string]interface{}                       `json:"fixed_arguments,omitempty"`
	DefaultArguments     map[string]interface{}                       `json:"default_arguments,omitempty"`
	TargetArgumentPaths  []string                                     `json:"target_argument_paths,omitempty"`
	OptionalTargets      []NativeExternalActionOptionalTargetArgument `json:"optional_targets,omitempty"`
	PreparationActionIDs []string                                     `json:"preparation_action_ids,omitempty"`
	PreparationHints     []NativeExternalActionPreparationHint        `json:"preparation_hints,omitempty"`
	PlanPhaseArgument    string                                       `json:"plan_phase_argument,omitempty"`
}

// NativeExternalActionCandidate is a request-scoped, server-authorized Action
// binding. It is retained even when schema budget prevents exposing the direct
// alias, so persisted plan contracts can be revalidated and safely fall back to
// the external-apps meta tools.
type NativeExternalActionCandidate struct {
	IntegrationID        string                                       `json:"integration_id"`
	ActionID             string                                       `json:"action_id"`
	IntentGroup          string                                       `json:"intent_group,omitempty"`
	IntentTokens         []string                                     `json:"intent_tokens,omitempty"`
	IntentMatched        bool                                         `json:"intent_matched,omitempty"`
	BindingFingerprint   string                                       `json:"binding_fingerprint"`
	ConnectionBinding    string                                       `json:"-"`
	Effect               string                                       `json:"effect,omitempty"`
	DefaultArguments     map[string]interface{}                       `json:"default_arguments,omitempty"`
	TargetArgumentPaths  []string                                     `json:"target_argument_paths,omitempty"`
	OptionalTargets      []NativeExternalActionOptionalTargetArgument `json:"optional_targets,omitempty"`
	PreparationActionIDs []string                                     `json:"preparation_action_ids,omitempty"`
	PreparationHints     []NativeExternalActionPreparationHint        `json:"preparation_hints,omitempty"`
	ResultLimitArgument  string                                       `json:"result_limit_argument,omitempty"`
	ResultLimitDefault   int                                          `json:"result_limit_default,omitempty"`
	Pinned               bool                                         `json:"pinned,omitempty"`
}

// NativeToolProjection adds a provider-native function while preserving the
// existing Skill execution boundary described by Binding. It is used for
// request-scoped capabilities whose public schema is narrower than the bound
// Skill tool schema.
type NativeToolProjection struct {
	Name        string
	NameScope   string
	Description string
	InputSchema map[string]interface{}
	Binding     NativeToolBinding
}

type NativeToolProjectionOptions struct {
	MaxTools          int
	ReservedToolNames []string
	EstimateTokens    func([]llmadapter.Message, []llmadapter.Tool) int
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
	ActiveSkillIDs              []string                        `json:"active_skill_ids"`
	InstructionMessages         []llmadapter.Message            `json:"-"`
	ProviderTools               []llmadapter.Tool               `json:"-"`
	ToolBindings                map[string]NativeToolBinding    `json:"tool_bindings"`
	SkippedSkills               []NativeSkillSkip               `json:"skipped_skills"`
	SkippedTools                []NativeToolSkip                `json:"skipped_tools,omitempty"`
	InstructionChars            int                             `json:"instruction_chars"`
	SchemaChars                 int                             `json:"schema_chars"`
	BudgetChars                 int                             `json:"budget_chars"`
	InstructionTokens           int                             `json:"instruction_tokens"`
	SchemaTokens                int                             `json:"schema_tokens"`
	BudgetTokens                int                             `json:"budget_tokens"`
	ExternalActionIntentMatched bool                            `json:"external_action_intent_matched,omitempty"`
	ExternalActionIntentKeys    []string                        `json:"external_action_intent_keys,omitempty"`
	ExternalActionCandidates    []NativeExternalActionCandidate `json:"external_action_candidates,omitempty"`
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
	candidatesByFingerprint := make(map[string]NativeExternalActionCandidate, len(toolSet.ExternalActionCandidates)+len(projections))
	for _, candidate := range toolSet.ExternalActionCandidates {
		if fingerprint := strings.TrimSpace(candidate.BindingFingerprint); fingerprint != "" {
			candidatesByFingerprint[fingerprint] = cloneNativeExternalActionCandidate(candidate)
		}
	}
	for _, projection := range projections {
		binding := projection.Binding
		binding.DefaultArguments = nativeExternalActionSchemaDefaults(projection.InputSchema)
		binding.OptionalTargets = nativeExternalActionOptionalTargets(projection.InputSchema, binding.TargetArgumentPaths)
		integrationID := strings.ToLower(strings.TrimSpace(fmt.Sprint(binding.FixedArguments["integration_id"])))
		actionID := strings.ToLower(strings.TrimSpace(fmt.Sprint(binding.FixedArguments["action_id"])))
		fingerprint := strings.TrimSpace(binding.BindingFingerprint)
		if integrationID == "" || actionID == "" || fingerprint == "" {
			continue
		}
		connectionBinding := strings.TrimSpace(binding.ConnectionBinding)
		if connectionBinding == "" {
			connectionBinding = NativeExternalActionConnectionBindingHash(fmt.Sprint(binding.FixedArguments["connection_id"]))
		}
		candidate := NativeExternalActionCandidate{
			IntegrationID: integrationID, ActionID: actionID,
			IntentGroup:   strings.ToLower(strings.TrimSpace(binding.IntentGroup)),
			IntentTokens:  append([]string(nil), binding.IntentTokens...),
			IntentMatched: binding.IntentMatched, BindingFingerprint: fingerprint,
			ConnectionBinding:    connectionBinding,
			Effect:               strings.ToLower(strings.TrimSpace(binding.Effect)),
			DefaultArguments:     copyStringAnyMap(binding.DefaultArguments),
			TargetArgumentPaths:  append([]string(nil), binding.TargetArgumentPaths...),
			OptionalTargets:      cloneNativeExternalActionOptionalTargets(binding.OptionalTargets),
			PreparationActionIDs: append([]string(nil), binding.PreparationActionIDs...),
			PreparationHints:     cloneNativeExternalActionPreparationHints(binding.PreparationHints),
			Pinned:               binding.Pinned,
		}
		candidate.ResultLimitArgument, candidate.ResultLimitDefault = nativeExternalActionResultLimitDefault(projection.InputSchema)
		candidatesByFingerprint[fingerprint] = candidate
		if binding.IntentMatched {
			toolSet.ExternalActionIntentMatched = true
		}
	}
	toolSet.ExternalActionIntentKeys = nil
	toolSet.ExternalActionCandidates = toolSet.ExternalActionCandidates[:0]
	for _, candidate := range candidatesByFingerprint {
		toolSet.ExternalActionCandidates = append(toolSet.ExternalActionCandidates, candidate)
	}
	sort.SliceStable(toolSet.ExternalActionCandidates, func(i, j int) bool {
		left := toolSet.ExternalActionCandidates[i]
		right := toolSet.ExternalActionCandidates[j]
		if left.Pinned != right.Pinned {
			return left.Pinned
		}
		if left.IntentMatched != right.IntentMatched {
			return left.IntentMatched
		}
		return left.IntegrationID+":"+left.ActionID < right.IntegrationID+":"+right.ActionID
	})
	maxTools := options.MaxTools
	if maxTools <= 0 || maxTools > DefaultMaxNativeToolProjections {
		maxTools = DefaultMaxNativeToolProjections
	}
	projections = dependencyOrderedNativeToolProjections(projections, maxTools)
	reservedNames := nativeControlToolNames()
	for _, name := range options.ReservedToolNames {
		if name = strings.TrimSpace(name); name != "" {
			reservedNames[name] = struct{}{}
		}
	}
	for _, tool := range toolSet.ProviderTools {
		if name := strings.TrimSpace(tool.Function.Name); name != "" {
			reservedNames[name] = struct{}{}
		}
	}
	localNames := map[string]struct{}{}
	projectionNameCounts := map[string]int{}
	projectionOriginalNames := map[string]string{}
	for _, projection := range projections {
		name := strings.TrimSpace(projection.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		projectionNameCounts[key]++
		projectionOriginalNames[key] = name
	}
	for key, count := range projectionNameCounts {
		if count > 1 {
			reservedNames[projectionOriginalNames[key]] = struct{}{}
		}
	}
	added := 0
	for _, projection := range projections {
		requestedName := strings.TrimSpace(projection.Name)
		binding := cloneNativeToolBinding(projection.Binding)
		binding.DefaultArguments = nativeExternalActionSchemaDefaults(projection.InputSchema)
		binding.OptionalTargets = nativeExternalActionOptionalTargets(projection.InputSchema, binding.TargetArgumentPaths)
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
		nameScope := strings.TrimSpace(projection.NameScope)
		if nameScope == "" {
			nameScope = binding.SkillID
		}
		providerName := nativeProviderToolName(nameScope, requestedName, reservedNames, localNames)
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

// dependencyOrderedNativeToolProjections emits every eligible preparation
// Action before the target Action that declares it. Therefore any prefix cut by
// the direct-alias limit is dependency-closed: a target can never consume one
// of the 24 slots while its authorized same-connection preparation Action is
// displaced by an unrelated projection.
func dependencyOrderedNativeToolProjections(projections []NativeToolProjection, limit int) []NativeToolProjection {
	if len(projections) < 2 {
		return append([]NativeToolProjection(nil), projections...)
	}
	if limit <= 0 || limit > len(projections) {
		limit = len(projections)
	}
	type identity struct {
		integration string
		action      string
		connection  string
	}
	identities := make([]identity, len(projections))
	byAction := make(map[string]int, len(projections))
	for index, projection := range projections {
		binding := projection.Binding
		integrationID := strings.ToLower(strings.TrimSpace(fmt.Sprint(binding.FixedArguments["integration_id"])))
		actionID := strings.ToLower(strings.TrimSpace(fmt.Sprint(binding.FixedArguments["action_id"])))
		connectionBinding := strings.TrimSpace(binding.ConnectionBinding)
		if connectionBinding == "" {
			connectionBinding = NativeExternalActionConnectionBindingHash(fmt.Sprint(binding.FixedArguments["connection_id"]))
		}
		identities[index] = identity{integration: integrationID, action: actionID, connection: connectionBinding}
		if integrationID != "" && actionID != "" {
			byAction[integrationID+":"+actionID] = index
		}
	}

	dependencyGroup := func(rootIndex int) []int {
		state := make([]uint8, len(projections))
		group := make([]int, 0, 1+len(projections[rootIndex].Binding.PreparationActionIDs))
		var visit func(int)
		visit = func(index int) {
			if index < 0 || index >= len(projections) || state[index] == 2 {
				return
			}
			if state[index] == 1 {
				// Cyclic preparation declarations cannot establish a dependency
				// edge. The server order remains deterministic and bounded.
				return
			}
			state[index] = 1
			current := identities[index]
			for _, dependencyActionID := range projections[index].Binding.PreparationActionIDs {
				dependencyActionID = strings.ToLower(strings.TrimSpace(dependencyActionID))
				dependencyIndex, exists := byAction[current.integration+":"+dependencyActionID]
				if !exists {
					continue
				}
				dependency := identities[dependencyIndex]
				if dependency.connection == "" || dependency.connection != current.connection {
					continue
				}
				visit(dependencyIndex)
			}
			state[index] = 2
			group = append(group, index)
		}
		visit(rootIndex)
		return group
	}

	priority := func(projection NativeToolProjection) int {
		value := projection.Binding.ProjectionPriority
		if projection.Binding.IntentMatched && value < 1 {
			value = 1
		}
		if projection.Binding.Pinned && value < 2 {
			value = 2
		}
		if value < 0 {
			return 0
		}
		if value > 2 {
			return 2
		}
		return value
	}
	selected := make(map[int]struct{}, limit)
	selectedOrder := make([]int, 0, limit)
	for currentPriority := 2; currentPriority >= 0; currentPriority-- {
		for rootIndex, projection := range projections {
			if priority(projection) != currentPriority {
				continue
			}
			group := dependencyGroup(rootIndex)
			newCount := 0
			for _, index := range group {
				if _, exists := selected[index]; !exists {
					newCount++
				}
			}
			if len(selectedOrder)+newCount > limit {
				continue
			}
			for _, index := range group {
				if _, exists := selected[index]; exists {
					continue
				}
				selected[index] = struct{}{}
				selectedOrder = append(selectedOrder, index)
			}
			if len(selectedOrder) == limit {
				break
			}
		}
		if len(selectedOrder) == limit {
			break
		}
	}

	ordered := make([]NativeToolProjection, 0, len(projections))
	for _, index := range selectedOrder {
		ordered = append(ordered, projections[index])
	}
	// Preserve every unselected projection after the closed prefix so the
	// existing append loop records its usual tool_limit_exceeded diagnostic.
	for index, projection := range projections {
		if _, exists := selected[index]; exists {
			continue
		}
		ordered = append(ordered, projection)
	}
	return ordered
}

func nativeExternalActionResultLimitDefault(schema map[string]interface{}) (string, int) {
	properties, _ := schema["properties"].(map[string]interface{})
	for _, key := range []string{"limit", "max_results", "page_size", "per_page"} {
		property, _ := properties[key].(map[string]interface{})
		value, exists := property["default"]
		if !exists {
			continue
		}
		if limit := nativeExternalActionPositiveSchemaInteger(value); limit > 0 {
			return key, limit
		}
	}
	return "", 0
}

func nativeExternalActionSchemaDefaults(schema map[string]interface{}) map[string]interface{} {
	properties, _ := schema["properties"].(map[string]interface{})
	required := make(map[string]struct{})
	for _, name := range nativeExternalActionSchemaStringList(schema["required"]) {
		required[strings.TrimSpace(name)] = struct{}{}
	}
	defaults := map[string]interface{}{}
	for name, rawProperty := range properties {
		// A default on a required property is model guidance, not permission for
		// the runtime to invent a security-sensitive choice. Required values must
		// remain explicit so the authoritative Action schema can fail closed.
		if _, isRequired := required[name]; isRequired {
			continue
		}
		property, _ := rawProperty.(map[string]interface{})
		value, exists := property["default"]
		if !exists || !nativeExternalActionScalarSchemaValue(value) {
			continue
		}
		defaults[name] = value
	}
	if len(defaults) == 0 {
		return nil
	}
	return defaults
}

func nativeExternalActionScalarSchemaValue(value interface{}) bool {
	switch value.(type) {
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func nativeExternalActionOptionalTargets(
	schema map[string]interface{},
	targetPaths []string,
) []NativeExternalActionOptionalTargetArgument {
	targetSet := make(map[string]struct{}, len(targetPaths))
	for _, path := range targetPaths {
		if path = strings.TrimSpace(path); path != "" {
			targetSet[path] = struct{}{}
		}
	}
	if len(targetSet) == 0 {
		return nil
	}
	properties, _ := schema["properties"].(map[string]interface{})
	alwaysRequired := make(map[string]struct{})
	for _, path := range nativeExternalActionSchemaStringList(schema["required"]) {
		alwaysRequired[path] = struct{}{}
	}
	allOf, _ := schema["allOf"].([]interface{})
	out := make([]NativeExternalActionOptionalTargetArgument, 0)
	seen := map[string]struct{}{}
	for _, rawClause := range allOf {
		clause, _ := rawClause.(map[string]interface{})
		condition, _ := clause["if"].(map[string]interface{})
		otherwise, _ := clause["else"].(map[string]interface{})
		conditionProperties, _ := condition["properties"].(map[string]interface{})
		conditionRequired := nativeExternalActionSchemaStringList(condition["required"])
		if len(conditionRequired) != 1 {
			continue
		}
		whenArgument := strings.TrimSpace(conditionRequired[0])
		whenProperty, _ := conditionProperties[whenArgument].(map[string]interface{})
		whenEquals, hasConst := whenProperty["const"]
		if whenArgument == "" || !hasConst || !nativeExternalActionScalarSchemaValue(whenEquals) {
			continue
		}
		for _, path := range nativeExternalActionSchemaStringList(otherwise["required"]) {
			if _, target := targetSet[path]; !target {
				continue
			}
			if _, required := alwaysRequired[path]; required {
				continue
			}
			key := path + "\x00" + whenArgument + "\x00" + fmt.Sprint(whenEquals)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			property, _ := properties[path].(map[string]interface{})
			out = append(out, NativeExternalActionOptionalTargetArgument{
				Path: path, WhenArgument: whenArgument, WhenEquals: whenEquals,
				DiscardWhenMatched: nativeExternalActionDiscardRuleMatches(property, whenArgument, whenEquals),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].WhenArgument < out[j].WhenArgument
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func nativeExternalActionDiscardRuleMatches(property map[string]interface{}, whenArgument string, whenEquals interface{}) bool {
	rule, _ := property["x-zgi-discard-when"].(map[string]interface{})
	argument, _ := rule["argument"].(string)
	argument = strings.TrimSpace(argument)
	equals, exists := rule["equals"]
	if argument == "" || !exists || argument != strings.TrimSpace(whenArgument) {
		return false
	}
	left, leftErr := json.Marshal(equals)
	right, rightErr := json.Marshal(whenEquals)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func nativeExternalActionSchemaStringList(value interface{}) []string {
	var raw []interface{}
	switch typed := value.(type) {
	case []interface{}:
		raw = typed
	case []string:
		raw = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func nativeExternalActionPositiveSchemaInteger(value interface{}) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int32:
		if typed > 0 {
			return int(typed)
		}
	case int64:
		if typed > 0 && typed <= int64(^uint(0)>>1) {
			return int(typed)
		}
	case float32:
		value64 := float64(typed)
		if value64 > 0 && value64 == float64(int(value64)) {
			return int(value64)
		}
	case float64:
		if typed > 0 && typed == float64(int(typed)) {
			return int(typed)
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 && parsed <= int64(^uint(0)>>1) {
			return int(parsed)
		}
	}
	return 0
}

// NativeExternalActionConnectionBindingHash is an opaque proof that two
// projected Actions resolved to the same selected connection. The raw
// connection identifier is never copied into model-visible candidate evidence.
func NativeExternalActionConnectionBindingHash(connectionID string) string {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(connectionID))
	return hex.EncodeToString(digest[:])
}

func cloneNativeToolBinding(input NativeToolBinding) NativeToolBinding {
	out := input
	out.FixedArguments = copyStringAnyMap(input.FixedArguments)
	out.DefaultArguments = copyStringAnyMap(input.DefaultArguments)
	out.TargetArgumentPaths = append([]string(nil), input.TargetArgumentPaths...)
	out.OptionalTargets = cloneNativeExternalActionOptionalTargets(input.OptionalTargets)
	out.PreparationActionIDs = append([]string(nil), input.PreparationActionIDs...)
	out.PreparationHints = cloneNativeExternalActionPreparationHints(input.PreparationHints)
	out.IntentTokens = append([]string(nil), input.IntentTokens...)
	return out
}

func cloneNativeExternalActionCandidate(input NativeExternalActionCandidate) NativeExternalActionCandidate {
	out := input
	out.DefaultArguments = copyStringAnyMap(input.DefaultArguments)
	out.IntentTokens = append([]string(nil), input.IntentTokens...)
	out.TargetArgumentPaths = append([]string(nil), input.TargetArgumentPaths...)
	out.OptionalTargets = cloneNativeExternalActionOptionalTargets(input.OptionalTargets)
	out.PreparationActionIDs = append([]string(nil), input.PreparationActionIDs...)
	out.PreparationHints = cloneNativeExternalActionPreparationHints(input.PreparationHints)
	return out
}

func cloneNativeExternalActionOptionalTargets(input []NativeExternalActionOptionalTargetArgument) []NativeExternalActionOptionalTargetArgument {
	if len(input) == 0 {
		return nil
	}
	return append([]NativeExternalActionOptionalTargetArgument(nil), input...)
}

func cloneNativeExternalActionPreparationHints(input []NativeExternalActionPreparationHint) []NativeExternalActionPreparationHint {
	out := make([]NativeExternalActionPreparationHint, 0, len(input))
	for _, hint := range input {
		cloned := hint
		cloned.TargetArguments = append([]string(nil), hint.TargetArguments...)
		cloned.ResultPaths = append([]string(nil), hint.ResultPaths...)
		out = append(out, cloned)
	}
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
		MetaToolLoadSkill:          {},
		MetaToolCallSkillTool:      {},
		MetaToolIntermediateAnswer: {},
		MetaToolFinalAnswer:        {},
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
		if !nativeToolNameTaken(original, reserved) {
			if !nativeToolNameTaken(original, local) {
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
	if nativeToolNameTaken(name, reserved) {
		return ""
	}
	if nativeToolNameTaken(name, local) {
		return ""
	}
	return name
}

func nativeToolNameTaken(name string, names map[string]struct{}) bool {
	name = strings.TrimSpace(name)
	for existing := range names {
		if strings.EqualFold(strings.TrimSpace(existing), name) {
			return true
		}
	}
	return false
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
