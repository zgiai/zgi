package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gopkg.in/yaml.v3"
)

const (
	defaultMaxCallsPerTurn = 20
	defaultTimeoutSeconds  = 5
	defaultCatalogDir      = "internal/modules/skills/catalog"
	defaultDisplayIcon     = "sparkles"
	defaultLocale          = "en_US"

	MetaToolLoadSkill          = "load_skill"
	MetaToolReadSkillReference = "read_skill_reference"
	MetaToolCallSkillTool      = "call_skill_tool"
	MetaToolIntermediateAnswer = "submit_intermediate_answer"
	MetaToolRequestUserInput   = "request_user_input"
)

var ErrSkillNotFound = errors.New("skill not found")

type Runtime struct {
	engine       *tools.ToolEngine
	manager      *tools.ToolManager
	catalogDir   string
	scriptRunner SkillScriptRunner
	governance   ToolGovernanceGateway
}

type SkillScriptRunner interface {
	RunSkillScript(ctx context.Context, doc SkillDocument, arguments map[string]interface{}, execCtx ExecutionContext, callID string) (*ToolInvocationResult, error)
	Configured() bool
}

type skillLocation struct {
	ID     string
	Root   string
	Source string
}

type ExecutionContext struct {
	OrganizationID    string
	UserID            string
	ConversationID    string
	AppID             string
	MessageID         string
	InvokeFrom        tools.ToolInvokeFrom
	RuntimeParameters map[string]interface{}
}

type ToolInvocationResult struct {
	ToolMessage llmadapter.Message
	Trace       SkillTrace
	Messages    []tools.ToolInvokeMessage
}

func NewRuntime(engine *tools.ToolEngine, manager *tools.ToolManager) *Runtime {
	return NewRuntimeWithCatalog(engine, manager, defaultSkillCatalogDir())
}

func NewRuntimeWithCatalog(engine *tools.ToolEngine, manager *tools.ToolManager, catalogDir string) *Runtime {
	return &Runtime{
		engine:     engine,
		manager:    manager,
		catalogDir: strings.TrimSpace(catalogDir),
	}
}

func (r *Runtime) WithScriptRunner(scriptRunner SkillScriptRunner) *Runtime {
	if r != nil && scriptRunner != nil && scriptRunner.Configured() {
		r.scriptRunner = scriptRunner
	}
	return r
}

func (r *Runtime) WithToolGovernanceGateway(governance ToolGovernanceGateway) *Runtime {
	if r != nil {
		r.governance = governance
	}
	return r
}

func (r *Runtime) ScriptsSupported() bool {
	return r != nil && r.scriptRunner != nil && r.scriptRunner.Configured()
}

func (r *Runtime) ResolveEnabledSkills(ctx context.Context, skillIDs []string) (*ResolvedSkills, error) {
	return r.ResolveEnabledSkillsWithCustom(ctx, skillIDs, nil)
}

func (r *Runtime) ResolveEnabledSkillsWithCustom(ctx context.Context, skillIDs []string, custom []CustomSkillCatalogEntry) (*ResolvedSkills, error) {
	_ = ctx
	ids := normalizeSkillIDs(skillIDs)
	resolved := &ResolvedSkills{Skills: make([]SkillDocument, 0, len(ids))}
	locations, err := r.skillLocations(custom)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		location, ok := locations[id]
		if !ok {
			return nil, fmt.Errorf("skill %s not found: %w", id, ErrSkillNotFound)
		}
		doc, err := r.loadSkillDocumentFromLocation(location)
		if err != nil {
			return nil, err
		}
		resolved.Skills = append(resolved.Skills, doc)
	}
	return resolved, nil
}

func (r *Runtime) ListSkills(ctx context.Context) ([]SkillDiscoveryMetadata, error) {
	return r.ListSkillsWithCustom(ctx, nil)
}

func (r *Runtime) ListSystemSkillsBestEffort(ctx context.Context) ([]SkillDiscoveryMetadata, error) {
	_ = ctx
	if r == nil {
		return nil, fmt.Errorf("skill runtime is not configured")
	}
	entries, err := os.ReadDir(r.catalogDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill catalog: %w", err)
	}
	metadata := make([]SkillDiscoveryMetadata, 0, len(entries))
	errs := make([]error, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if !isValidSkillName(name) {
			errs = append(errs, fmt.Errorf("invalid skill directory %s: use lowercase letters, numbers, and hyphens only", entry.Name()))
			continue
		}
		id := normalizeSkillID(name)
		doc, err := r.loadSkillDocumentFromLocation(skillLocation{
			ID:     id,
			Root:   filepath.Join(r.catalogDir, name),
			Source: SkillSourceSystem,
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		metadata = append(metadata, skillDiscoveryMetadata(doc))
	}
	sort.Slice(metadata, func(i, j int) bool { return metadata[i].ID < metadata[j].ID })
	return metadata, errors.Join(errs...)
}

func (r *Runtime) ListSkillsWithCustom(ctx context.Context, custom []CustomSkillCatalogEntry) ([]SkillDiscoveryMetadata, error) {
	_ = ctx
	if r == nil {
		return nil, fmt.Errorf("skill runtime is not configured")
	}
	locations, err := r.skillLocations(custom)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(locations))
	for id := range locations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	metadata := make([]SkillDiscoveryMetadata, 0, len(ids))
	for _, id := range ids {
		doc, err := r.loadSkillDocumentFromLocation(locations[id])
		if err != nil {
			return nil, err
		}
		metadata = append(metadata, skillDiscoveryMetadata(doc))
	}
	return metadata, nil
}

func (r *Runtime) SystemSkillExists(skillID string) bool {
	if r == nil {
		return false
	}
	id := normalizeSkillID(skillID)
	if id == "" || !isValidSkillName(id) {
		return false
	}
	info, err := os.Stat(filepath.Join(r.catalogDir, id))
	return err == nil && info.IsDir()
}

func (r *Runtime) GetSkillMetadata(ctx context.Context, skillID string) (*SkillDiscoveryMetadata, error) {
	return r.GetSkillMetadataWithCustom(ctx, skillID, nil)
}

func (r *Runtime) GetSkillMetadataWithCustom(ctx context.Context, skillID string, custom []CustomSkillCatalogEntry) (*SkillDiscoveryMetadata, error) {
	_ = ctx
	if r == nil {
		return nil, fmt.Errorf("skill runtime is not configured")
	}
	id := normalizeSkillID(skillID)
	locations, err := r.skillLocations(custom)
	if err != nil {
		return nil, err
	}
	location, ok := locations[id]
	if !ok {
		return nil, fmt.Errorf("skill %s not found: %w", id, ErrSkillNotFound)
	}
	doc, err := r.loadSkillDocumentFromLocation(location)
	if err != nil {
		return nil, err
	}
	metadata := skillDiscoveryMetadata(doc)
	return &metadata, nil
}

func (r *Runtime) ValidateCatalog(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("skill runtime is not configured")
	}
	entries, err := os.ReadDir(r.catalogDir)
	if err != nil {
		return fmt.Errorf("failed to read skill catalog: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if !isValidSkillName(name) {
			return fmt.Errorf("invalid skill directory %s: use lowercase letters, numbers, and hyphens only", name)
		}
		id := normalizeSkillID(name)
		doc, err := r.loadSkillDocumentFromLocation(skillLocation{
			ID:     id,
			Root:   filepath.Join(r.catalogDir, id),
			Source: SkillSourceSystem,
		})
		if err != nil {
			return err
		}
		if err := r.validateSkillTools(ctx, doc); err != nil {
			return err
		}
		if err := r.validateSkillReferences(doc); err != nil {
			return err
		}
	}
	return nil
}

func LoadCustomSkillDocument(root string) (SkillDocument, error) {
	raw, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return SkillDocument{}, fmt.Errorf("custom skill SKILL.md not found: %w", err)
	}
	frontmatter, body, err := parseSkillMarkdown(raw)
	if err != nil {
		return SkillDocument{}, fmt.Errorf("failed to parse custom skill: %w", err)
	}
	id := normalizeSkillID(frontmatter.Name)
	doc := buildSkillDocument(id, root, SkillSourceCustom, frontmatter, body)
	if err := validateCustomSkillDocument(doc); err != nil {
		return SkillDocument{}, err
	}
	return doc, nil
}

func (r *Runtime) LoadCustomSkillDocument(root string) (SkillDocument, error) {
	doc, err := LoadCustomSkillDocument(root)
	if err != nil {
		return SkillDocument{}, err
	}
	r.applyScriptSupport(&doc)
	return doc, nil
}

func (r *Runtime) LoadSkill(ctx context.Context, resolved *ResolvedSkills, skillID string) (*SkillDocument, SkillTrace, error) {
	_ = ctx
	start := time.Now()
	doc, ok := resolved.Get(skillID)
	trace := SkillTrace{
		Kind:       "skill_load",
		SkillID:    normalizeSkillID(skillID),
		Status:     "success",
		DurationMS: time.Since(start).Milliseconds(),
	}
	if !ok {
		err := fmt.Errorf("skill %s is not enabled", normalizeSkillID(skillID))
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return nil, trace, err
	}
	trace.DurationMS = time.Since(start).Milliseconds()
	return doc, trace, nil
}

func (r *Runtime) ReadReference(ctx context.Context, resolved *ResolvedSkills, skillID string, referencePath string) (string, SkillTrace, error) {
	_ = ctx
	start := time.Now()
	normalizedSkillID := normalizeSkillID(skillID)
	trace := SkillTrace{
		Kind:    "reference_read",
		SkillID: normalizedSkillID,
		Status:  "success",
		Arguments: map[string]interface{}{
			"path": summarizeValue(referencePath),
		},
	}
	doc, ok := resolved.Get(normalizedSkillID)
	if !ok {
		err := fmt.Errorf("skill %s is not enabled", normalizedSkillID)
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return "", trace, err
	}
	path, err := referenceFullPath(*doc, referencePath)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return "", trace, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return "", trace, fmt.Errorf("failed to read skill reference: %w", err)
	}
	trace.DurationMS = time.Since(start).Milliseconds()
	return string(content), trace, nil
}

func (r *Runtime) CallSkillTool(
	ctx context.Context,
	resolved *ResolvedSkills,
	skillID string,
	toolName string,
	arguments map[string]interface{},
	execCtx ExecutionContext,
	callID string,
) (*ToolInvocationResult, error) {
	if r == nil {
		return nil, fmt.Errorf("skill runtime is not configured")
	}
	doc, ok := resolved.Get(skillID)
	if !ok {
		return nil, fmt.Errorf("skill %s is not enabled", normalizeSkillID(skillID))
	}
	if strings.TrimSpace(toolName) == SkillScriptToolRun {
		if !doc.Metadata.ScriptsSupported || r.scriptRunner == nil {
			return nil, fmt.Errorf("skill %s scripts are not supported", doc.Metadata.ID)
		}
		return r.scriptRunner.RunSkillScript(ctx, *doc, arguments, execCtx, callID)
	}
	toolDef, ok := findSkillTool(*doc, toolName)
	if !ok {
		return nil, fmt.Errorf("tool %s is not available in skill %s", strings.TrimSpace(toolName), doc.Metadata.ID)
	}

	governanceDecision, governed, preflight, err := r.preflightToolGovernance(ctx, *doc, toolDef, arguments, execCtx, callID)
	if preflight != nil {
		return preflight, err
	}
	if err != nil {
		return nil, err
	}
	if r.engine == nil {
		return nil, fmt.Errorf("tool engine is not configured")
	}

	timeout := docTimeoutSeconds(*doc)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	start := time.Now()
	result, err := r.engine.Invoke(runCtx, tools.InvokeRequest{
		ProviderType:      toolDef.ProviderType,
		ProviderID:        toolDef.ProviderID,
		ToolName:          toolDef.Name,
		TenantID:          execCtx.OrganizationID,
		UserID:            execCtx.UserID,
		Parameters:        arguments,
		ConversationID:    execCtx.ConversationID,
		AppID:             execCtx.AppID,
		MessageID:         execCtx.MessageID,
		InvokeFrom:        normalizeToolInvokeFrom(execCtx.InvokeFrom),
		RuntimeParameters: copyStringAnyMap(execCtx.RuntimeParameters),
	})
	trace := SkillTrace{
		Kind:       "tool_call",
		SkillID:    doc.Metadata.ID,
		ToolName:   toolDef.Name,
		Status:     "success",
		DurationMS: time.Since(start).Milliseconds(),
		Arguments:  summarizeArguments(arguments),
	}
	if governed {
		trace.Governance = &governanceDecision
	}
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		return &ToolInvocationResult{Trace: trace}, err
	}
	if result == nil || !result.Success {
		message := "tool invocation failed"
		if result != nil && strings.TrimSpace(result.Error) != "" {
			message = result.Error
		}
		trace.Status = "error"
		trace.Error = message
		return &ToolInvocationResult{Trace: trace}, fmt.Errorf("%s", message)
	}

	callID = strings.TrimSpace(callID)
	if callID == "" {
		callID = "call_" + toolDef.Name
	}
	return &ToolInvocationResult{
		Messages: result.Messages,
		Trace:    trace,
		ToolMessage: llmadapter.Message{
			Role:       "tool",
			ToolCallID: callID,
			Content:    toolMessagesContent(result.Messages),
		},
	}, nil
}

func (r *Runtime) preflightToolGovernance(
	ctx context.Context,
	doc SkillDocument,
	toolDef SkillToolDefinition,
	arguments map[string]interface{},
	execCtx ExecutionContext,
	callID string,
) (toolgovernance.Decision, bool, *ToolInvocationResult, error) {
	if r == nil || r.governance == nil || toolDef.Governance == nil {
		return toolgovernance.Decision{}, false, nil, nil
	}
	start := time.Now()
	decision, err := r.governance.DecideSkillTool(ctx, ToolGovernanceRequest{
		Manifest:         *toolDef.Governance,
		SkillID:          doc.Metadata.ID,
		ToolName:         toolDef.Name,
		ProviderType:     toolDef.ProviderType,
		ProviderID:       toolDef.ProviderID,
		Arguments:        copyStringAnyMap(arguments),
		ExecutionContext: execCtx,
	})
	trace := SkillTrace{
		Kind:       "tool_governance",
		SkillID:    doc.Metadata.ID,
		ToolName:   toolDef.Name,
		Status:     string(decision.Status),
		DurationMS: time.Since(start).Milliseconds(),
		Arguments:  summarizeArguments(arguments),
		Governance: &decision,
		Result:     governanceTraceResult(decision),
	}
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		return decision, true, &ToolInvocationResult{Trace: trace}, err
	}
	if decision.Status == toolgovernance.DecisionStatusAllowed {
		return decision, true, nil, nil
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		callID = "call_" + toolDef.Name
	}
	return decision, true, &ToolInvocationResult{
		Trace:       trace,
		ToolMessage: ToolResultMessage(callID, governanceToolFeedback(decision)),
	}, nil
}

func governanceTraceResult(decision toolgovernance.Decision) map[string]interface{} {
	result := governanceToolFeedback(decision)
	if decision.ApprovalEvent != nil {
		result["approval_event"] = decision.ApprovalEvent
	}
	return result
}

func governanceToolFeedback(decision toolgovernance.Decision) map[string]interface{} {
	feedback := copyStringAnyMap(decision.ModelFeedback)
	if feedback == nil {
		feedback = map[string]interface{}{}
	}
	feedback["status"] = string(decision.Status)
	feedback["reason"] = strings.TrimSpace(decision.Reason)
	feedback["correlation_id"] = strings.TrimSpace(decision.CorrelationID)
	feedback["requires_approval"] = decision.RequiresApproval
	feedback["instruction"] = governanceInstruction(decision.Status)
	return map[string]interface{}{
		"governance": feedback,
	}
}

func governanceInstruction(status toolgovernance.DecisionStatus) string {
	switch status {
	case toolgovernance.DecisionStatusNeedsApproval:
		return "The tool was not executed. Explain that user approval is required and wait for approval before retrying this action."
	case toolgovernance.DecisionStatusNeedsResolution:
		return "The tool was not executed. Ask the user to clarify the target asset or resolve the asset reference before retrying."
	case toolgovernance.DecisionStatusDenied:
		return "The tool was not executed. Explain the denial and continue with a safe alternative."
	case toolgovernance.DecisionStatusBlocked:
		return "The tool was not executed. Explain why the action is blocked and continue without this tool."
	default:
		return "Continue with the tool result."
	}
}

func MetaTools() []llmadapter.Tool {
	return metaTools(true)
}

func MetaToolsForSkills(resolved *ResolvedSkills) []llmadapter.Tool {
	return metaTools(resolvedHasToolSkills(resolved))
}

func MetaToolsForSkillState(resolved *ResolvedSkills, loadedSkillIDs map[string]struct{}) []llmadapter.Tool {
	loaded := normalizedLoadedSkillIDs(loadedSkillIDs)
	tools := []llmadapter.Tool{
		loadSkillMetaTool(resolvedSkillIDs(resolved)),
		requestUserInputMetaTool(),
		intermediateAnswerMetaTool(),
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
		intermediateAnswerMetaTool(),
	}
	if includeToolCaller {
		tools = append(tools, callSkillToolMetaTool(nil, nil, nil, nil, true))
	}
	return tools
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
					"skill_id":  stringSchema("The loaded skill ID that allows the tool.", skillIDs),
					"tool_name": stringSchema("The allowed tool name to call.", toolNames),
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

func SkillToolArgumentContractFor(skillID string, toolName string) (SkillToolArgumentContract, bool) {
	skillID = normalizeSkillID(skillID)
	toolName = strings.TrimSpace(toolName)
	key := skillID + "/" + toolName
	contract, ok := skillToolArgumentContracts()[key]
	return contract, ok
}

func skillToolArgumentContracts() map[string]SkillToolArgumentContract {
	return map[string]SkillToolArgumentContract{
		SkillCalculator + "/evaluate_expression": {
			SkillID:     SkillCalculator,
			ToolName:    "evaluate_expression",
			Description: "Evaluate one deterministic arithmetic expression.",
			Schema: objectSchema(
				map[string]interface{}{
					"expression": map[string]interface{}{
						"type":        "string",
						"description": "Arithmetic expression to evaluate, such as 23*17+9. Only numbers, parentheses, +, -, *, /, %, and ^ are allowed.",
					},
					"precision": precisionSchema(),
				},
				[]string{"expression"},
			),
			Example: map[string]interface{}{"expression": "23*17+9"},
		},
		SkillCalculator + "/calculate": {
			SkillID:     SkillCalculator,
			ToolName:    "calculate",
			Description: "Perform deterministic binary arithmetic between two numbers.",
			Schema: objectSchema(
				map[string]interface{}{
					"operation": enumStringSchema("Arithmetic operation.", []string{"add", "subtract", "multiply", "divide", "power", "mod"}),
					"left":      numberSchema("Left operand."),
					"right":     numberSchema("Right operand."),
					"precision": precisionSchema(),
				},
				[]string{"operation", "left", "right"},
			),
			Example: map[string]interface{}{"operation": "multiply", "left": 23, "right": 17},
		},
		SkillCalculator + "/percentage": {
			SkillID:     SkillCalculator,
			ToolName:    "percentage",
			Description: "Calculate percent-of, percentage change, or apply a percentage increase/decrease.",
			Schema: objectSchema(
				map[string]interface{}{
					"operation": enumStringSchema("Percentage operation. percent_of/apply_* require value and percent; change requires from and to.", []string{"percent_of", "change", "apply_increase", "apply_decrease"}),
					"value":     numberSchema("Base value for percent_of, apply_increase, and apply_decrease."),
					"percent":   numberSchema("Percentage value, such as 15 for 15 percent."),
					"from":      numberSchema("Original value for change."),
					"to":        numberSchema("New value for change."),
					"precision": precisionSchema(),
				},
				[]string{"operation"},
			),
			Example: map[string]interface{}{"operation": "percent_of", "value": 200, "percent": 15},
		},
		SkillFileGenerator + "/generate_file": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_file",
			Description: "Generate a downloadable file artifact from provided content.",
			Schema: objectSchema(
				map[string]interface{}{
					"content":   stringValueSchema("Text content to write into the generated file. Use valid CSV content for xlsx and runnable HTML content for html."),
					"format":    enumStringSchema("Output format.", []string{"txt", "md", "html", "json", "csv", "docx", "xlsx", "pdf"}),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional document title used by generated HTML and PDF files."),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"content", "format"},
			),
			Example: map[string]interface{}{"content": "# Report\n\nSummary...", "format": "md", "filename": "report"},
		},
		SkillFileGenerator + "/generate_docx": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_docx",
			Description: "Generate a styled DOCX file from a structured JSON document specification.",
			Schema: objectSchema(
				map[string]interface{}{
					"document":  stringValueSchema("JSON string describing the DOCX document. Include blocks with type heading, paragraph, table, or page_break."),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional title hint; visible content must be included in document.blocks."),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"document"},
			),
			Example: map[string]interface{}{
				"document": `{"blocks":[{"type":"heading","level":1,"text":"Report","style":{"alignment":"center","font_size":18,"bold":true}},{"type":"paragraph","runs":[{"text":"Total: "},{"text":"113.47","bold":true,"color":"C00000"}]}]}`,
				"filename": "styled-report",
			},
		},
		SkillFileGenerator + "/generate_pdf": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_pdf",
			Description: "Generate a styled PDF file from self-contained HTML and inline CSS.",
			Schema: objectSchema(
				map[string]interface{}{
					"html":      stringValueSchema("Self-contained HTML body or full HTML document. Do not include external URLs, scripts, iframes, or remote assets."),
					"css":       stringValueSchema("Optional inline CSS appended to the HTML document. Prefer @page for page size and margins."),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional title used when wrapping an HTML fragment. Visible content must be included in html."),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"html"},
			),
			Example: map[string]interface{}{
				"html":     `<main><h1>Report</h1><p>Total: <strong class="amount">113.47</strong></p></main>`,
				"css":      `@page { size: A4; margin: 18mm; } h1 { text-align: center; } .amount { color: #c00000; }`,
				"filename": "styled-report",
			},
		},
		SkillFileGenerator + "/generate_pptx": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_pptx",
			Description: "Generate an editable static PPTX presentation from a structured JSON presentation specification.",
			Schema: objectSchema(
				map[string]interface{}{
					"presentation": stringValueSchema("JSON string describing the PPTX presentation. Include slides with elements of type title, text, table, or shape. Use non-overlapping boxes for readable content; omitted boxes use simple auto layout."),
					"filename":     stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":        stringValueSchema("Optional title hint; visible content must be included in presentation.slides."),
					"lifecycle":    enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"presentation"},
			),
			Example: map[string]interface{}{
				"presentation": `{"layout":"wide","slides":[{"elements":[{"type":"title","text":"Quarterly Report","style":{"align":"center"}},{"type":"text","text":"Total revenue: 113.47","x":0.8,"y":1.4,"w":11.6,"h":0.8,"style":{"font_size":24,"bold":true,"color":"C00000"}}]}]}`,
				"filename":     "quarterly-report",
			},
		},
		SkillChartGenerator + "/generate_chart": {
			SkillID:     SkillChartGenerator,
			ToolName:    "generate_chart",
			Description: "Generate a downloadable SVG chart artifact from structured data after chart type, title, data mapping, and rendering style have been provided or confirmed. Supports radar, bar, line, pie, doughnut, scatter, and score_distribution. For generic chart requests, call request_user_input before this tool.",
			Schema: objectSchema(
				map[string]interface{}{
					"chart_type":      enumStringSchema("Chart type.", []string{"radar", "bar", "line", "pie", "doughnut", "scatter", "score_distribution"}),
					"title":           stringValueSchema("Optional chart title."),
					"output_filename": stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"data":            chartDataSchema(),
					"options": objectSchema(
						map[string]interface{}{
							"width":       numberSchema("Optional SVG width. Defaults to 900."),
							"height":      numberSchema("Optional SVG height. Defaults to 700 for radar and 620 for bar/line."),
							"style":       enumStringSchema("Rendering style.", []string{"simple", "business", "teaching", "comparison"}),
							"show_values": booleanSchema("Whether to show point values. Defaults to true."),
							"show_labels": booleanSchema("Whether to show scatter point labels. Defaults to true."),
							"legend":      booleanSchema("Whether to show legend. Defaults to true."),
							"grid":        booleanSchema("Whether to show grid lines. Defaults to true for bar/line."),
						},
						nil,
					),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"chart_type", "data"},
			),
			Example: map[string]interface{}{
				"chart_type":      "radar",
				"title":           "Score Comparison",
				"output_filename": "score-radar",
				"data": map[string]interface{}{
					"dimensions": []string{"Chinese", "Math", "English", "Physics", "Chemistry", "Biology"},
					"max_value":  100,
					"series": []map[string]interface{}{
						{"name": "Class Average", "values": []int{78, 82, 80, 75, 73, 76}},
						{"name": "Student", "values": []int{88, 92, 84, 81, 77, 86}},
					},
				},
			},
		},
		SkillWorkReport + "/generate_file": {
			SkillID:     SkillWorkReport,
			ToolName:    "generate_file",
			Description: "Generate a downloadable weekly, monthly, or work report artifact from prepared report content.",
			Schema: objectSchema(
				map[string]interface{}{
					"content":   stringValueSchema("Final weekly, monthly, or work report content to write into the generated file."),
					"format":    enumStringSchema("Output format.", []string{"txt", "md", "docx", "pdf"}),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional document title used by generated PDF files."),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"content", "format"},
			),
			Example: map[string]interface{}{"content": "# Weekly Work Report\n\n## Summary\n\n...", "format": "md", "filename": "weekly-work-report"},
		},
		SkillInternalKnowledge + "/list_accessible_knowledge_bases": {
			SkillID:     SkillInternalKnowledge,
			ToolName:    "list_accessible_knowledge_bases",
			Description: "List knowledge bases accessible to the current AIChat user. Inspect status and fallback_used before selecting dataset IDs.",
			Schema: objectSchema(
				map[string]interface{}{
					"query": stringValueSchema("Optional search text for narrowing candidate knowledge bases."),
					"limit": numberSchema("Maximum number of knowledge bases to list. Defaults to 20 and is capped at 100."),
				},
				nil,
			),
			Example: map[string]interface{}{"query": "expense policy", "limit": 10},
		},
		SkillInternalKnowledge + "/retrieve_knowledge": {
			SkillID:     SkillInternalKnowledge,
			ToolName:    "retrieve_knowledge",
			Description: "Retrieve relevant context from selected accessible knowledge base IDs returned by list_accessible_knowledge_bases.",
			Schema: objectSchema(
				map[string]interface{}{
					"query":          stringValueSchema("The user question or refined search query."),
					"dataset_ids":    stringArrayOrCSVSchema("Knowledge base IDs selected from list_accessible_knowledge_bases. Pass a JSON array of IDs when possible."),
					"top_k":          numberSchema("Maximum number of retrieved chunks. Defaults to 5 and is capped at 20."),
					"retrieval_mode": enumStringSchema("Optional retrieval mode. Omit for default hybrid mode; use graph only for relationship/entity questions.", []string{"hybrid", "vector", "graph"}),
				},
				[]string{"query", "dataset_ids"},
			),
			Example: map[string]interface{}{"query": "What is the reimbursement policy?", "dataset_ids": []string{"dataset-id"}},
		},
		SkillAgentKnowledge + "/retrieve_agent_knowledge": {
			SkillID:     SkillAgentKnowledge,
			ToolName:    "retrieve_agent_knowledge",
			Description: "Retrieve relevant context from knowledge bases bound to the current Agent. Do not pass dataset IDs.",
			Schema: objectSchema(
				map[string]interface{}{
					"query":          stringValueSchema("The user question or refined search query."),
					"top_k":          numberSchema("Maximum number of retrieved chunks. Defaults to 5 and is capped at 20."),
					"retrieval_mode": enumStringSchema("Optional retrieval mode. Omit for default hybrid mode; use graph only for relationship/entity questions.", []string{"hybrid", "vector", "graph"}),
				},
				[]string{"query"},
			),
			Example: map[string]interface{}{"query": "Summarize the configured product FAQ."},
		},
		SkillInternalDatabase + "/list_accessible_databases": databaseListContract(SkillInternalDatabase),
		SkillInternalDatabase + "/list_database_tables":      databaseTablesContract(SkillInternalDatabase),
		SkillInternalDatabase + "/describe_database_table":   databaseDescribeTableContract(SkillInternalDatabase),
		SkillInternalDatabase + "/query_table_records":       databaseQueryRecordsContract(SkillInternalDatabase),
		SkillInternalDatabase + "/insert_table_records":      databaseMutateRecordsContract(SkillInternalDatabase, "insert_table_records", "Insert records into a database table."),
		SkillInternalDatabase + "/update_table_records":      databaseMutateRecordsContract(SkillInternalDatabase, "update_table_records", "Update records in a database table. Each record must include id."),
		SkillInternalDatabase + "/delete_table_records":      databaseMutateRecordsContract(SkillInternalDatabase, "delete_table_records", "Delete records from a database table. Each record must include id."),
		SkillAgentDatabase + "/list_accessible_databases":    databaseListContract(SkillAgentDatabase),
		SkillAgentDatabase + "/list_database_tables":         databaseTablesContract(SkillAgentDatabase),
		SkillAgentDatabase + "/describe_database_table":      databaseDescribeTableContract(SkillAgentDatabase),
		SkillAgentDatabase + "/query_table_records":          databaseQueryRecordsContract(SkillAgentDatabase),
		SkillAgentDatabase + "/insert_table_records":         databaseMutateRecordsContract(SkillAgentDatabase, "insert_table_records", "Insert records into an Agent-bound database table."),
		SkillAgentDatabase + "/update_table_records":         databaseMutateRecordsContract(SkillAgentDatabase, "update_table_records", "Update records in an Agent-bound database table. Each record must include id."),
		SkillAgentDatabase + "/delete_table_records":         databaseMutateRecordsContract(SkillAgentDatabase, "delete_table_records", "Delete records from an Agent-bound database table. Each record must include id."),
		SkillAgentWorkflow + "/list_agent_workflows":         workflowListContract(),
		SkillAgentWorkflow + "/run_agent_workflow":           workflowRunContract(),
		SkillAgentWorkflow + "/get_workflow_run_status":      workflowRunStatusContract(),
		SkillTime + "/current_time": {
			SkillID:     SkillTime,
			ToolName:    "current_time",
			Description: "Get the current system time with optional timezone and format.",
			Schema: objectSchema(
				map[string]interface{}{
					"format":   stringValueSchema("Optional strftime-style output format. Defaults to %Y-%m-%d %H:%M:%S."),
					"timezone": stringValueSchema("Optional IANA timezone such as Asia/Shanghai. Defaults to UTC."),
				},
				nil,
			),
			Example: map[string]interface{}{"timezone": "Asia/Shanghai", "format": "%Y-%m-%d %H:%M:%S"},
		},
		SkillTime + "/date_calculate": {
			SkillID:     SkillTime,
			ToolName:    "date_calculate",
			Description: "Add or subtract date intervals, or calculate the day interval between two dates.",
			Schema: objectSchema(
				map[string]interface{}{
					"operation":   enumStringSchema("Operation to perform. diff requires target_date.", []string{"add", "subtract", "diff"}),
					"base_date":   stringValueSchema("Base date in YYYY-MM-DD format. Use today or omit to use the current date."),
					"amount":      numberSchema("Interval amount for add or subtract. Defaults to 1."),
					"unit":        enumStringSchema("Interval unit for add or subtract.", []string{"day", "week", "month", "year"}),
					"target_date": stringValueSchema("Target date in YYYY-MM-DD format. Required when operation is diff."),
					"timezone":    stringValueSchema("IANA timezone used when base_date is omitted. Defaults to UTC."),
				},
				[]string{"operation"},
			),
			Example: map[string]interface{}{"operation": "add", "base_date": "today", "amount": 3, "unit": "day", "timezone": "Asia/Shanghai"},
		},
	}
}

func ExpectedSkillToolArguments(skillID string, toolName string) map[string]interface{} {
	contract, ok := SkillToolArgumentContractFor(skillID, toolName)
	if !ok {
		return nil
	}
	return map[string]interface{}{
		"skill_id":    contract.SkillID,
		"tool_name":   contract.ToolName,
		"description": contract.Description,
		"schema":      contract.Schema,
		"example":     contract.Example,
	}
}

func databaseListContract(skillID string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    "list_accessible_databases",
		Description: "List databases accessible to the current user or bound to the current Agent.",
		Schema: objectSchema(
			map[string]interface{}{
				"query": stringValueSchema("Optional search text for narrowing candidate databases."),
				"limit": numberSchema("Maximum number of databases to list. Defaults to 20."),
			},
			nil,
		),
		Example: map[string]interface{}{"query": "customers", "limit": 10},
	}
}

func databaseTablesContract(skillID string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    "list_database_tables",
		Description: "List tables in an accessible database.",
		Schema: objectSchema(
			map[string]interface{}{
				"data_source_id": stringValueSchema("Database ID returned by list_accessible_databases."),
				"query":          stringValueSchema("Optional search text for narrowing tables by name or description."),
				"limit":          numberSchema("Maximum number of tables to list. Defaults to 50."),
			},
			[]string{"data_source_id"},
		),
		Example: map[string]interface{}{"data_source_id": "database-id", "query": "orders"},
	}
}

func databaseDescribeTableContract(skillID string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    "describe_database_table",
		Description: "Describe a database table and its columns.",
		Schema: objectSchema(
			map[string]interface{}{
				"data_source_id":        stringValueSchema("Database ID returned by list_accessible_databases."),
				"table_id":              stringValueSchema("Table metadata ID returned by list_database_tables."),
				"include_system_fields": booleanSchema("Whether to include system fields such as id and timestamps. Defaults to false."),
			},
			[]string{"data_source_id", "table_id"},
		),
		Example: map[string]interface{}{"data_source_id": "database-id", "table_id": "table-id"},
	}
}

func databaseQueryRecordsContract(skillID string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    "query_table_records",
		Description: "Query table records with pagination and a safe order clause.",
		Schema: objectSchema(
			map[string]interface{}{
				"data_source_id": stringValueSchema("Database ID returned by list_accessible_databases."),
				"table_id":       stringValueSchema("Table metadata ID returned by list_database_tables."),
				"limit":          numberSchema("Maximum number of records. Defaults to 20 and is capped by the backend."),
				"offset":         numberSchema("Pagination offset. Defaults to 0."),
				"order":          stringValueSchema("Optional safe order clause such as id DESC."),
			},
			[]string{"data_source_id", "table_id"},
		),
		Example: map[string]interface{}{"data_source_id": "database-id", "table_id": "table-id", "limit": 20},
	}
}

func databaseMutateRecordsContract(skillID string, toolName string, description string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    toolName,
		Description: description,
		Schema: objectSchema(
			map[string]interface{}{
				"data_source_id": stringValueSchema("Database ID returned by list_accessible_databases."),
				"table_id":       stringValueSchema("Table metadata ID returned by list_database_tables."),
				"records": map[string]interface{}{
					"type":        "array",
					"description": "Records to mutate. For update and delete, each record must include id.",
					"items": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": true,
					},
				},
			},
			[]string{"data_source_id", "table_id", "records"},
		),
		Example: map[string]interface{}{"data_source_id": "database-id", "table_id": "table-id", "records": []map[string]interface{}{{"id": "record-id"}}},
	}
}

func workflowListContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentWorkflow,
		ToolName:    "list_agent_workflows",
		Description: "Fallback/debug list of workflows bound to the current Agent. Prefer the injected available_workflows context when it is present.",
		Schema:      objectSchema(map[string]interface{}{}, nil),
		Example:     map[string]interface{}{},
	}
}

func workflowRunContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentWorkflow,
		ToolName:    "run_agent_workflow",
		Description: "Run an Agent-bound workflow by binding_id. Do not pass workflow_id directly. Set inputs.query to the user's current request. After a succeeded run, final answers must use primary_output or outputs and must not invent workflow output.",
		Schema: objectSchema(
			map[string]interface{}{
				"binding_id": stringValueSchema("Workflow binding ID from injected available_workflows, or from list_agent_workflows if the injected list is missing or ambiguous."),
				"inputs": map[string]interface{}{
					"type":                 "object",
					"description":          "Workflow input object. Include query with the user's current request unless the binding's input_schema, required_inputs, or default_input_key says otherwise; the runtime also forwards query as sys.query.",
					"additionalProperties": true,
					"properties": map[string]interface{}{
						"query": stringValueSchema("The user's current request or instruction to pass into the workflow."),
					},
					"required": []string{"query"},
				},
			},
			[]string{"binding_id", "inputs"},
		),
		Example: map[string]interface{}{"binding_id": "approval-flow", "inputs": map[string]interface{}{"query": "Approve refund request #123"}},
	}
}

func workflowRunStatusContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentWorkflow,
		ToolName:    "get_workflow_run_status",
		Description: "Query the status and available outputs for an Agent-bound workflow run.",
		Schema: objectSchema(
			map[string]interface{}{
				"workflow_run_id": stringValueSchema("Workflow run ID returned by run_agent_workflow."),
			},
			[]string{"workflow_run_id"},
		),
		Example: map[string]interface{}{"workflow_run_id": "workflow-run-id"},
	}
}

func objectSchema(properties map[string]interface{}, required []string) map[string]interface{} {
	if required == nil {
		required = []string{}
	}
	return map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func numberSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": description,
	}
}

func stringValueSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": description,
	}
}

func enumStringSchema(description string, values []string) map[string]interface{} {
	schema := stringValueSchema(description)
	if len(values) > 0 {
		schema["enum"] = values
	}
	return schema
}

func booleanSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "boolean",
		"description": description,
	}
}

func arraySchema(description string, items map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": description,
		"items":       items,
	}
}

func chartDataSchema() map[string]interface{} {
	series := arraySchema(
		"Chart data series. Radar supports 1-2 series; bar and line support 1-8 series.",
		objectSchema(
			map[string]interface{}{
				"name":   stringValueSchema("Series label."),
				"values": arraySchema("Numeric values matching the selected chart labels length.", numberSchema("Score or metric value.")),
				"color":  stringValueSchema("Optional #RRGGBB color."),
			},
			[]string{"name", "values"},
		),
	)
	pieItems := arraySchema(
		"Pie or doughnut chart items.",
		objectSchema(
			map[string]interface{}{
				"label": stringValueSchema("Slice label."),
				"value": numberSchema("Slice value."),
				"color": stringValueSchema("Optional #RRGGBB color."),
			},
			[]string{"label", "value"},
		),
	)
	scatterPoints := arraySchema(
		"Scatter chart points.",
		objectSchema(
			map[string]interface{}{
				"x":     numberSchema("X-axis value."),
				"y":     numberSchema("Y-axis value."),
				"label": stringValueSchema("Optional point label."),
				"color": stringValueSchema("Optional #RRGGBB color."),
			},
			[]string{"x", "y"},
		),
	)
	scoreCountBands := arraySchema(
		"Precomputed score distribution bands.",
		objectSchema(
			map[string]interface{}{
				"label": stringValueSchema("Band label such as 90-100."),
				"count": numberSchema("Precomputed count for this band."),
			},
			[]string{"label", "count"},
		),
	)
	scoreRangeBands := arraySchema(
		"Score distribution bands used to count raw scores.",
		objectSchema(
			map[string]interface{}{
				"label": stringValueSchema("Band label such as 90-100."),
				"min":   numberSchema("Inclusive minimum score when calculating from raw scores."),
				"max":   numberSchema("Inclusive maximum score when calculating from raw scores."),
			},
			[]string{"label", "min", "max"},
		),
	)
	common := map[string]interface{}{
		"max_value": numberSchema("Optional shared maximum value. Radar defaults to 100; bar and line auto-scale when omitted."),
		"series":    series,
	}
	radarProps := copySchemaProperties(common)
	radarProps["dimensions"] = stringArrayOrCSVSchema("Radar axis labels, such as subject names. Required for radar charts.")
	barProps := copySchemaProperties(common)
	barProps["categories"] = stringArrayOrCSVSchema("Bar chart category labels.")
	lineProps := copySchemaProperties(common)
	lineProps["x_axis"] = stringArrayOrCSVSchema("Line chart x-axis labels.")
	lineProps["categories"] = stringArrayOrCSVSchema("Line chart x-axis labels alias.")
	pieProps := map[string]interface{}{
		"items": pieItems,
	}
	scatterProps := map[string]interface{}{
		"x_label": stringValueSchema("Optional x-axis label."),
		"y_label": stringValueSchema("Optional y-axis label."),
		"x_min":   numberSchema("Optional x-axis minimum."),
		"x_max":   numberSchema("Optional x-axis maximum."),
		"y_min":   numberSchema("Optional y-axis minimum."),
		"y_max":   numberSchema("Optional y-axis maximum."),
		"points":  scatterPoints,
	}
	distributionCountProps := map[string]interface{}{
		"bands":     scoreCountBands,
		"max_value": numberSchema("Optional y-axis maximum for distribution counts."),
	}
	distributionRangeProps := map[string]interface{}{
		"bands":     scoreRangeBands,
		"scores":    arraySchema("Raw score values or objects with value.", map[string]interface{}{"oneOf": []interface{}{numberSchema("Raw score value."), objectSchema(map[string]interface{}{"label": stringValueSchema("Optional score label."), "value": numberSchema("Raw score value.")}, []string{"value"})}}),
		"max_value": numberSchema("Optional y-axis maximum for distribution counts."),
	}

	return map[string]interface{}{
		"description": "Chart-specific data. Use dimensions for radar, categories for bar, x_axis or categories for line, items for pie/doughnut, points for scatter, and bands for score_distribution.",
		"anyOf": []interface{}{
			objectSchema(radarProps, []string{"dimensions", "series"}),
			objectSchema(barProps, []string{"categories", "series"}),
			objectSchema(lineProps, []string{"x_axis", "series"}),
			objectSchema(lineProps, []string{"categories", "series"}),
			objectSchema(pieProps, []string{"items"}),
			objectSchema(scatterProps, []string{"points"}),
			objectSchema(distributionCountProps, []string{"bands"}),
			objectSchema(distributionRangeProps, []string{"bands", "scores"}),
		},
	}
}

func copySchemaProperties(input map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func stringArrayOrCSVSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"description": description,
		"oneOf": []interface{}{
			map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			map[string]interface{}{
				"type": "string",
			},
		},
	}
}

func precisionSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": "Optional decimal places to round the result to. Defaults to 6 and must be between 0 and 12.",
		"minimum":     0,
		"maximum":     12,
	}
}

func SkillMetadataSystemMessage(metadata []SkillPromptMetadata) llmadapter.Message {
	message, _ := SkillMetadataSystemMessageWithBudget(metadata, DefaultSkillMetadataPromptBudgetChars)
	return message
}

func SkillMetadataSystemMessageWithBudget(metadata []SkillPromptMetadata, budgetChars int) (llmadapter.Message, SkillMetadataPromptStats) {
	content, stats := skillMetadataPromptWithBudget(metadata, budgetChars)
	return llmadapter.Message{
		Role:    "system",
		Content: content,
	}, stats
}

func ToolResultMessage(callID string, payload interface{}) llmadapter.Message {
	content, err := json.Marshal(payload)
	if err != nil {
		content = []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return llmadapter.Message{
		Role:       "tool",
		ToolCallID: callID,
		Content:    string(content),
	}
}

func ParseArguments(raw string) (map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]interface{}{}, nil
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	return args, nil
}

func defaultSkillCatalogDir() string {
	if _, err := os.Stat(defaultCatalogDir); err == nil {
		return defaultCatalogDir
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return defaultCatalogDir
	}
	return filepath.Join(filepath.Dir(file), "catalog")
}

func (r *Runtime) loadSkillDocument(skillID string) (SkillDocument, error) {
	if r == nil {
		return SkillDocument{}, fmt.Errorf("skill runtime is not configured")
	}
	id := normalizeSkillID(skillID)
	if id == "" {
		return SkillDocument{}, fmt.Errorf("skill id is required")
	}
	if !isValidSkillName(id) {
		return SkillDocument{}, fmt.Errorf("invalid skill id %s: use lowercase letters, numbers, and hyphens only", id)
	}
	return r.loadSkillDocumentFromLocation(skillLocation{
		ID:     id,
		Root:   filepath.Join(r.catalogDir, id),
		Source: SkillSourceSystem,
	})
}

func (r *Runtime) loadSkillDocumentFromLocation(location skillLocation) (SkillDocument, error) {
	id := normalizeSkillID(location.ID)
	if id == "" {
		return SkillDocument{}, fmt.Errorf("skill id is required")
	}
	if !isValidSkillName(id) {
		return SkillDocument{}, fmt.Errorf("invalid skill id %s: use lowercase letters, numbers, and hyphens only", id)
	}
	root := strings.TrimSpace(location.Root)
	if root == "" {
		return SkillDocument{}, fmt.Errorf("skill %s storage path is required", id)
	}
	source := normalizeSkillSource(location.Source)
	path := filepath.Join(root, "SKILL.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SkillDocument{}, fmt.Errorf("skill %s not found: %w", id, ErrSkillNotFound)
		}
		return SkillDocument{}, fmt.Errorf("skill %s not found: %w", id, err)
	}
	frontmatter, body, err := parseSkillMarkdown(raw)
	if err != nil {
		return SkillDocument{}, fmt.Errorf("failed to parse skill %s: %w", id, err)
	}
	doc := buildSkillDocument(id, root, source, frontmatter, body)
	r.applyScriptSupport(&doc)
	if source == SkillSourceCustom {
		if err := validateCustomSkillDocument(doc); err != nil {
			return SkillDocument{}, err
		}
		return doc, nil
	}
	if err := validateSkillDocument(doc); err != nil {
		return SkillDocument{}, err
	}
	return doc, nil
}

func buildSkillDocument(id string, root string, source string, frontmatter SkillFrontmatter, body string) SkillDocument {
	whenToUse := strings.TrimSpace(frontmatter.WhenToUse)
	if normalizeSkillSource(source) == SkillSourceCustom && whenToUse == "" {
		whenToUse = strings.TrimSpace(frontmatter.Description)
	}
	scriptPresent := hasScripts(root)
	return SkillDocument{
		Metadata: SkillMetadata{
			ID:               normalizeSkillID(id),
			Source:           normalizeSkillSource(source),
			Name:             strings.TrimSpace(frontmatter.Name),
			Description:      strings.TrimSpace(frontmatter.Description),
			WhenToUse:        whenToUse,
			Display:          normalizeSkillDisplayWithFallback(frontmatter, whenToUse),
			Tools:            append([]string{}, frontmatter.Tools...),
			RuntimeType:      normalizeSkillRuntimeType(frontmatter.RuntimeType, frontmatter.Tools),
			MaxCallsPerTurn:  normalizePositive(frontmatter.MaxCallsPerTurn, defaultMaxCallsPerTurn),
			TimeoutSeconds:   normalizeSkillTimeout(frontmatter.TimeoutSeconds, scriptPresent),
			References:       listReferences(root, source),
			HasScripts:       scriptPresent,
			ScriptsSupported: false,
			RootPath:         root,
			SupportedCallers: normalizeSkillCallers(id, source, frontmatter.SupportedCallers),
			RequiredConfig:   normalizeSkillRequiredConfig(id, frontmatter.RequiredConfig),
		},
		Instructions: strings.TrimSpace(body),
		Tools:        buildSkillToolDefinitions(id, frontmatter),
	}
}

func (r *Runtime) applyScriptSupport(doc *SkillDocument) {
	if r == nil || doc == nil || !doc.Metadata.HasScripts || !r.ScriptsSupported() {
		return
	}
	doc.Metadata.ScriptsSupported = true
	ensureScriptTool(doc)
}

func normalizeToolInvokeFrom(value tools.ToolInvokeFrom) tools.ToolInvokeFrom {
	switch value {
	case tools.ToolInvokeFromAgent:
		return tools.ToolInvokeFromAgent
	default:
		return tools.ToolInvokeFromAIChat
	}
}

func normalizeSkillTimeout(value int, hasScriptFiles bool) int {
	if value > 0 {
		return value
	}
	if hasScriptFiles {
		return defaultSkillScriptTimeoutSeconds
	}
	return defaultTimeoutSeconds
}

func normalizeSkillCallers(id string, source string, callers []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(callers))
	for _, raw := range callers {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case SkillCallerAIChat:
			if _, ok := seen[SkillCallerAIChat]; !ok {
				seen[SkillCallerAIChat] = struct{}{}
				out = append(out, SkillCallerAIChat)
			}
		case SkillCallerAgent:
			if _, ok := seen[SkillCallerAgent]; !ok {
				seen[SkillCallerAgent] = struct{}{}
				out = append(out, SkillCallerAgent)
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	switch normalizeSkillID(id) {
	case SkillInternalKnowledge, SkillInternalDatabase:
		return []string{SkillCallerAIChat}
	case SkillAgentKnowledge, SkillAgentDatabase, SkillAgentWorkflow:
		return []string{SkillCallerAgent}
	default:
		return []string{SkillCallerAIChat, SkillCallerAgent}
	}
}

func normalizeSkillRequiredConfig(id string, required []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(required))
	for _, raw := range required {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 && normalizeSkillID(id) == SkillAgentKnowledge {
		out = append(out, SkillRequiredConfigAgentKnowledge)
	}
	if len(out) == 0 && normalizeSkillID(id) == SkillAgentDatabase {
		out = append(out, SkillRequiredConfigAgentDatabase)
	}
	if len(out) == 0 && normalizeSkillID(id) == SkillAgentWorkflow {
		out = append(out, SkillRequiredConfigAgentWorkflow)
	}
	sort.Strings(out)
	return out
}

func parseSkillMarkdown(raw []byte) (SkillFrontmatter, string, error) {
	text := strings.TrimPrefix(string(raw), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return SkillFrontmatter{}, "", fmt.Errorf("missing yaml frontmatter")
	}
	remaining := text[len("---\n"):]
	end := strings.Index(remaining, "\n---")
	if end < 0 {
		return SkillFrontmatter{}, "", fmt.Errorf("unterminated yaml frontmatter")
	}
	var frontmatter SkillFrontmatter
	if err := yaml.Unmarshal([]byte(remaining[:end]), &frontmatter); err != nil {
		return SkillFrontmatter{}, "", err
	}
	body := strings.TrimPrefix(remaining[end:], "\n---")
	body = strings.TrimPrefix(body, "\r\n")
	body = strings.TrimPrefix(body, "\n")
	return frontmatter, body, nil
}

func validateSkillDocument(doc SkillDocument) error {
	if doc.Metadata.ID == "" {
		return fmt.Errorf("skill id is required")
	}
	if !isValidSkillName(doc.Metadata.ID) {
		return fmt.Errorf("invalid skill id %s: use lowercase letters, numbers, and hyphens only", doc.Metadata.ID)
	}
	if doc.Metadata.Name == "" {
		return fmt.Errorf("skill %s name is required", doc.Metadata.ID)
	}
	if !isValidSkillName(doc.Metadata.Name) {
		return fmt.Errorf("invalid skill name %s: use lowercase letters, numbers, and hyphens only", doc.Metadata.Name)
	}
	if doc.Metadata.Description == "" {
		return fmt.Errorf("skill %s description is required", doc.Metadata.ID)
	}
	if doc.Metadata.WhenToUse == "" {
		return fmt.Errorf("skill %s when_to_use is required", doc.Metadata.ID)
	}
	if strings.TrimSpace(doc.Instructions) == "" {
		return fmt.Errorf("skill %s instructions are required", doc.Metadata.ID)
	}
	if doc.Metadata.RuntimeType == "" {
		return fmt.Errorf("skill %s runtime_type is required", doc.Metadata.ID)
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeTool && doc.Metadata.RuntimeType != SkillRuntimeTypePrompt && doc.Metadata.RuntimeType != SkillRuntimeTypeHybrid {
		return fmt.Errorf("skill %s has invalid runtime_type", doc.Metadata.ID)
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt && len(doc.Tools) == 0 {
		return fmt.Errorf("skill %s tools are required", doc.Metadata.ID)
	}
	for _, tool := range doc.Tools {
		if tool.Name == "" || tool.ProviderID == "" || tool.ProviderType == "" {
			return fmt.Errorf("skill %s has incomplete tool definition", doc.Metadata.ID)
		}
	}
	return nil
}

func validateCustomSkillDocument(doc SkillDocument) error {
	if doc.Metadata.Source != SkillSourceCustom {
		return fmt.Errorf("custom skill source is required")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		return fmt.Errorf("custom skill %s must use prompt runtime_type", doc.Metadata.ID)
	}
	if len(doc.Metadata.Tools) > 0 || hasNonScriptTools(doc.Tools) {
		return fmt.Errorf("custom skill %s must not declare tools", doc.Metadata.ID)
	}
	if err := validateSkillDocument(doc); err != nil {
		return err
	}
	return nil
}

func buildSkillToolDefinitions(skillID string, frontmatter SkillFrontmatter) []SkillToolDefinition {
	providerType := frontmatter.ProviderType
	if providerType == "" {
		providerType = tools.ToolProviderTypeBuiltin
	}
	defs := make([]SkillToolDefinition, 0, len(frontmatter.Tools))
	for _, raw := range frontmatter.Tools {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		def := SkillToolDefinition{
			Name:         name,
			ProviderType: providerType,
			ProviderID:   strings.TrimSpace(frontmatter.ProviderID),
		}
		if manifest, ok := skillToolGovernanceManifest(skillID, name, frontmatter.ToolGovernance); ok {
			def.Governance = &manifest
		}
		defs = append(defs, def)
	}
	return defs
}

func skillToolGovernanceManifest(skillID string, toolName string, manifests map[string]toolgovernance.Manifest) (toolgovernance.Manifest, bool) {
	if len(manifests) == 0 {
		return toolgovernance.Manifest{}, false
	}
	manifest, ok := manifests[strings.TrimSpace(toolName)]
	if !ok {
		manifest, ok = manifests[strings.ToLower(strings.TrimSpace(toolName))]
	}
	if !ok {
		return toolgovernance.Manifest{}, false
	}
	manifest = toolgovernance.NormalizeManifest(manifest)
	if manifest.ToolID == "" {
		manifest.ToolID = strings.TrimSpace(toolName)
	}
	if manifest.SkillID == "" {
		manifest.SkillID = normalizeSkillID(skillID)
	}
	return toolgovernance.NormalizeManifest(manifest), true
}

func (r *Runtime) listReferences(skillID string) []SkillReference {
	return listReferences(filepath.Join(r.catalogDir, skillID), SkillSourceSystem)
}

func (r *Runtime) hasScripts(skillID string) bool {
	return hasScripts(filepath.Join(r.catalogDir, skillID))
}

func (r *Runtime) safeReferencePath(skillID string, referencePath string) (string, error) {
	doc, err := r.loadSkillDocument(skillID)
	if err != nil {
		return "", err
	}
	return referenceFullPath(doc, referencePath)
}

func findSkillTool(doc SkillDocument, toolName string) (SkillToolDefinition, bool) {
	name := strings.TrimSpace(toolName)
	for _, tool := range doc.Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return SkillToolDefinition{}, false
}

func hasNonScriptTools(tools []SkillToolDefinition) bool {
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) != SkillScriptToolRun {
			return true
		}
	}
	return false
}

func ensureScriptTool(doc *SkillDocument) {
	if doc == nil {
		return
	}
	for _, tool := range doc.Tools {
		if strings.TrimSpace(tool.Name) == SkillScriptToolRun {
			return
		}
	}
	doc.Tools = append(doc.Tools, scriptToolDefinition())
}

func scriptToolDefinition() SkillToolDefinition {
	return SkillToolDefinition{
		Name:         SkillScriptToolRun,
		ProviderType: tools.ToolProviderTypeBuiltin,
		ProviderID:   "skill-script",
	}
}

func (r *Runtime) validateSkillTools(ctx context.Context, doc SkillDocument) error {
	if r == nil || r.manager == nil {
		return fmt.Errorf("tool manager is not configured")
	}
	checked := make(map[string]struct{}, len(doc.Tools))
	for _, tool := range doc.Tools {
		key := string(tool.ProviderType) + ":" + tool.ProviderID + ":" + tool.Name
		if _, ok := checked[key]; ok {
			continue
		}
		checked[key] = struct{}{}
		provider, err := r.manager.GetProvider(ctx, tool.ProviderType, tool.ProviderID, "")
		if err != nil {
			return fmt.Errorf("skill %s provider %s not found: %w", doc.Metadata.ID, tool.ProviderID, err)
		}
		if _, err := provider.GetTool(tool.Name); err != nil {
			return fmt.Errorf("skill %s tool %s not found: %w", doc.Metadata.ID, tool.Name, err)
		}
	}
	return nil
}
