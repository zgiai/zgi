package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
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
	MetaToolTurnState          = "submit_turn_state"
	MetaToolUpdatePlan         = "update_plan"
	MetaToolRequestUserInput   = "request_user_input"
	MetaToolFinalAnswer        = "submit_final_answer"
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
	ID       string
	Root     string
	Source   string
	Embedded bool
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
	ids := withRequiredPreflightSkills(normalizeSkillIDs(skillIDs))
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

func withRequiredPreflightSkills(ids []string) []string {
	if len(ids) == 0 {
		return ids
	}
	hasPromptProfessionalizer := false
	needsPromptProfessionalizer := false
	for _, id := range ids {
		switch normalizeSkillID(id) {
		case SkillPromptProfessionalizer:
			hasPromptProfessionalizer = true
		case SkillImageGenerator, SkillArchitectureDiagram, SkillChartGenerator:
			needsPromptProfessionalizer = true
		}
	}
	if !needsPromptProfessionalizer || hasPromptProfessionalizer {
		return ids
	}
	out := append([]string{}, ids...)
	out = append(out, SkillPromptProfessionalizer)
	return out
}

func RequiresPromptProfessionalizerPreflight(skillID string, toolName string) bool {
	switch normalizeSkillID(skillID) {
	case SkillImageGenerator:
		switch strings.TrimSpace(toolName) {
		case "generate_image", "edit_image":
			return true
		}
	case SkillArchitectureDiagram:
		return strings.TrimSpace(toolName) == "generate_architecture_diagram"
	case SkillChartGenerator:
		return strings.TrimSpace(toolName) == "generate_chart"
	}
	return false
}

func (r *Runtime) ListSkills(ctx context.Context) ([]SkillDiscoveryMetadata, error) {
	return r.ListSkillsWithCustom(ctx, nil)
}

func (r *Runtime) ListSystemSkillsBestEffort(ctx context.Context) ([]SkillDiscoveryMetadata, error) {
	_ = ctx
	if r == nil {
		return nil, fmt.Errorf("skill runtime is not configured")
	}
	locations, errs, err := r.systemSkillLocationsBestEffort()
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

func (r *Runtime) ListSystemSkillDocuments(ctx context.Context) ([]SkillDocument, error) {
	_ = ctx
	if r == nil {
		return nil, fmt.Errorf("skill runtime is not configured")
	}
	locations, err := r.skillLocations(nil)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(locations))
	for id := range locations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	docs := make([]SkillDocument, 0, len(ids))
	for _, id := range ids {
		doc, err := r.loadSkillDocumentFromLocation(locations[id])
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func (r *Runtime) SystemSkillExists(skillID string) bool {
	if r == nil {
		return false
	}
	id := normalizeSkillID(skillID)
	if id == "" || !isValidSkillName(id) {
		return false
	}
	locations, err := r.systemSkillLocations()
	if err != nil {
		return false
	}
	_, ok := locations[id]
	return ok
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
	locations, err := r.systemSkillLocations()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(locations))
	for id := range locations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		doc, err := r.loadSkillDocumentFromLocation(locations[id])
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
	doc, err := buildSkillDocument(id, root, SkillSourceCustom, frontmatter, body, listReferences(root, SkillSourceCustom), hasScripts(root))
	if err != nil {
		return SkillDocument{}, err
	}
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
	ref, err := skillReference(*doc, referencePath)
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		trace.DurationMS = time.Since(start).Milliseconds()
		return "", trace, err
	}
	content, err := readSkillReference(ref)
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
		toolDef, ok := findSkillTool(*doc, toolName)
		if !ok {
			return nil, fmt.Errorf("tool %s is not available in skill %s", strings.TrimSpace(toolName), doc.Metadata.ID)
		}
		executionArguments := copyStringAnyMap(arguments)
		if executionArguments == nil {
			executionArguments = map[string]interface{}{}
		}
		if _, _, preflight, err := r.preflightToolGovernance(ctx, *doc, toolDef, executionArguments, execCtx, callID); preflight != nil {
			return preflight, err
		} else if err != nil {
			return nil, err
		}
		if !doc.Metadata.ScriptsSupported || r.scriptRunner == nil {
			return nil, fmt.Errorf("skill %s scripts are not supported", doc.Metadata.ID)
		}
		return r.scriptRunner.RunSkillScript(ctx, *doc, executionArguments, execCtx, callID)
	}
	toolDef, ok := findSkillTool(*doc, toolName)
	if !ok {
		return nil, fmt.Errorf("tool %s is not available in skill %s", strings.TrimSpace(toolName), doc.Metadata.ID)
	}

	executionArguments := copyStringAnyMap(arguments)
	if executionArguments == nil {
		executionArguments = map[string]interface{}{}
	}
	var governanceArgumentRewrite map[string]interface{}
	if rewritten, rewriteSummary, ok := rewriteReadToolArgumentsFromResolvedAsset(toolDef.Governance, executionArguments, execCtx); ok {
		executionArguments = rewritten
		governanceArgumentRewrite = rewriteSummary
	}
	executionArguments = r.enrichToolGovernanceArguments(ctx, toolDef, executionArguments, execCtx)
	if err := validateSkillToolArgumentsAgainstContract(doc.Metadata.ID, toolDef.Name, executionArguments); err != nil {
		trace := SkillTrace{
			Kind:      "tool_call",
			SkillID:   doc.Metadata.ID,
			ToolName:  toolDef.Name,
			Status:    "error",
			Arguments: summarizeArguments(executionArguments),
			Error:     err.Error(),
		}
		return &ToolInvocationResult{Trace: trace}, err
	}

	governanceDecision, governed, preflight, err := r.preflightToolGovernance(ctx, *doc, toolDef, executionArguments, execCtx, callID)
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
		Parameters:        executionArguments,
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
		Arguments:  summarizeArguments(executionArguments),
	}
	if len(governanceArgumentRewrite) > 0 {
		trace.Arguments["governance_argument_rewrite"] = governanceArgumentRewrite
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
	messages := appendGovernanceRewriteObservation(result.Messages, governanceDecision, governanceArgumentRewrite)
	return &ToolInvocationResult{
		Messages: messages,
		Trace:    trace,
		ToolMessage: llmadapter.Message{
			Role:       "tool",
			ToolCallID: callID,
			Content:    toolMessagesContent(messages),
		},
	}, nil
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
