package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
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

func (r *Runtime) enrichToolGovernanceArguments(ctx context.Context, toolDef SkillToolDefinition, arguments map[string]interface{}, execCtx ExecutionContext) map[string]interface{} {
	if r == nil || r.engine == nil {
		return arguments
	}
	enriched, err := r.engine.EnrichGovernanceArguments(ctx, tools.InvokeRequest{
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
	if err != nil || enriched == nil {
		return arguments
	}
	return enriched
}

func appendGovernanceRewriteObservation(messages []tools.ToolInvokeMessage, decision toolgovernance.Decision, rewrite map[string]interface{}) []tools.ToolInvokeMessage {
	if len(rewrite) == 0 || decision.Status != toolgovernance.DecisionStatusAllowed {
		return messages
	}
	assets := decision.ExpectedAssets
	if len(assets) == 0 {
		assets = decision.Assets
	}
	if len(assets) == 0 {
		return messages
	}
	out := append([]tools.ToolInvokeMessage{}, messages...)
	out = append(out, tools.ToolInvokeMessage{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":                  "completed",
			"kind":                    "resolved_target_observation",
			"resolved_assets":         governanceAssetObservationPayload(assets),
			"resolved_asset_count":    len(assets),
			"resolved_asset_guidance": "The user's request target has been resolved to resolved_assets. Treat these assets as the only target for this turn. Answer with these asset names and the tool content only; do not mention internal resolution, governance, rewrites, redirects, mismatched IDs, or other visible files unless the user asks for debugging or comparison.",
		},
	})
	return out
}

func governanceAssetObservationPayload(assets []toolgovernance.AssetRef) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(assets))
	for _, asset := range assets {
		item := map[string]interface{}{
			"id":   strings.TrimSpace(asset.ID),
			"type": strings.TrimSpace(asset.Type),
			"name": strings.TrimSpace(asset.Name),
		}
		if workspaceID := strings.TrimSpace(asset.WorkspaceID); workspaceID != "" {
			item["workspace_id"] = workspaceID
		}
		if source := strings.TrimSpace(asset.Source); source != "" {
			item["source"] = source
		}
		if len(asset.Metadata) > 0 {
			item["metadata"] = copyStringAnyMap(asset.Metadata)
		}
		out = append(out, item)
	}
	return out
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
	if decision.Status == toolgovernance.DecisionStatusNeedsApproval && decision.RequiresApproval {
		frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
			CorrelationID:  decision.CorrelationID,
			Manifest:       decision.Manifest,
			SkillID:        doc.Metadata.ID,
			ToolName:       toolDef.Name,
			ProviderType:   string(toolDef.ProviderType),
			ProviderID:     toolDef.ProviderID,
			Arguments:      copyStringAnyMap(arguments),
			Assets:         decision.Assets,
			ExpectedAssets: decision.ExpectedAssets,
			Now:            start,
		})
		decision.FrozenInvocation = &frozen
		if decision.ApprovalEvent != nil {
			decision.ApprovalEvent.FrozenInvocation = &frozen
		}
		trace.Governance = &decision
		trace.Result = governanceTraceResult(decision)
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
	feedback["instruction"] = governanceInstruction(decision)
	return map[string]interface{}{
		"governance": feedback,
	}
}

func governanceInstruction(decision toolgovernance.Decision) string {
	switch decision.Status {
	case toolgovernance.DecisionStatusNeedsApproval:
		return "The tool was not executed. Explain that user approval is required and wait for approval before retrying this action."
	case toolgovernance.DecisionStatusNeedsResolution:
		if len(decision.ExpectedAssets) > 0 {
			return "The tool was not executed because the tool arguments did not match the resolved target asset. Retry the same tool with the exact ID from expected_assets; do not ask the user to clarify unless expected_assets is ambiguous or empty."
		}
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
		requestUserInputMetaTool(),
		turnStateMetaTool(),
		updatePlanMetaTool(),
		intermediateAnswerMetaTool(),
		finalAnswerMetaTool(),
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
			Description: "Replace the current execution plan snapshot after progress changes. Keep stable phase IDs and allow at most one in_progress phase. Evidence refs are optional audit links: include exact refs when available, but do not delay work or revise a plan only to clear an unresolved ref. You may call update_plan together with the next business tool in the same response.",
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
				},
				"required": []string{"plan"},
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
			Description: "Record structured state for this same AIChat turn. Use this as a state handoff before approvals, page navigation, refresh, or another phase when implicit working memory may become unreliable. Use working_fact for model-only derived facts that later steps must reuse exactly, such as a file theme, target agent name, chosen model, selected asset, decision, assumption, or verification result. Use user_deliverable only when content should also be visible to the user; submit_intermediate_answer remains a compatibility shortcut for that case.",
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
									"description": "The concise fact, decision, assumption, or verification result. Keep exact user-derived values exact.",
									"maxLength":   4000,
								},
								"title": map[string]interface{}{
									"type":        "string",
									"description": "Short user-facing title when kind is user_deliverable.",
									"maxLength":   120,
								},
								"content": map[string]interface{}{
									"type":        "string",
									"description": "Markdown content when kind is user_deliverable.",
									"maxLength":   16000,
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
	return llmadapter.Tool{
		Type: "function",
		Function: llmadapter.Function{
			Name:        MetaToolFinalAnswer,
			Description: "Submit the final user-facing answer and end the current skill loop when you judge the task complete or have honestly reached a terminal outcome. This call is terminal: do not combine it with business tools or request_user_input. Put the complete final response in answer; ordinary assistant content is progress, not the final answer. A plan snapshot is optional audit metadata and never determines whether the answer is accepted.",
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
				"required": []string{"answer"},
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
		SkillConsoleNavigator + "/navigate": {
			SkillID:     SkillConsoleNavigator,
			ToolName:    "navigate",
			Description: "Request navigation to a whitelisted internal ZGI console page. This only changes the visible page and does not mutate assets.",
			Schema: objectSchema(
				map[string]interface{}{
					"href":   stringValueSchema("Required whitelisted internal /console route, such as /console/files, /console/agents, /console/workflows, /console/dataset, /console/db, /console/work/task, /console/prompts, /console/work/chat, /console/work/image, /console/work/app, /console/workspace, or /console/settings. Do not use external URLs."),
					"reason": stringValueSchema("Optional short user-facing reason for why the route is relevant."),
				},
				[]string{"href"},
			),
			Example: map[string]interface{}{"href": "/console/files", "reason": "The user asked to open file management."},
		},
		SkillFileGenerator + "/generate_file": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_file",
			Description: "Generate a downloadable temporary file artifact from provided content. This does not write to File Management; use file-manager/save_file_to_management after generation when the user explicitly asks to save into File Management.",
			Schema: objectSchema(
				map[string]interface{}{
					"content":   stringValueSchema("Text content to write into the generated file. Use valid CSV content for xlsx, runnable HTML content for html, and a complete self-contained <svg> document for svg."),
					"format":    enumStringSchema("Output format.", []string{"txt", "md", "html", "json", "csv", "svg", "docx", "xlsx", "pdf"}),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional document title used by generated HTML, XLSX, and PDF files. For XLSX, this becomes a merged title row above the table."),
					"lifecycle": enumStringSchema("Temporary artifact lifecycle. Defaults to temporary.", []string{"persistent", "temporary"}),
				},
				[]string{"content", "format"},
			),
			Example: map[string]interface{}{"content": "# Report\n\nSummary...", "format": "md", "filename": "report"},
		},
		SkillFileReader + "/read_file": {
			SkillID:     SkillFileReader,
			ToolName:    "read_file",
			Description: "Read extracted text content from one file available in the current AIChat context.",
			Schema: objectSchema(
				map[string]interface{}{
					"file_id":   stringValueSchema("Required file ID from the current page context, attachment context, or governed asset resolution. Do not invent IDs."),
					"max_chars": numberSchema("Optional maximum returned content characters. Defaults to 4000 and is capped at 12000."),
				},
				[]string{"file_id"},
			),
			Example: map[string]interface{}{"file_id": "file_123", "max_chars": 4000},
		},
		SkillFileReader + "/list_visible_files": {
			SkillID:     SkillFileReader,
			ToolName:    "list_visible_files",
			Description: "List files visible in the current Console Files page context without reading file contents.",
			Schema: objectSchema(
				map[string]interface{}{},
				nil,
			),
			Example: map[string]interface{}{},
		},
		SkillFileManager + "/delete_file": {
			SkillID:     SkillFileManager,
			ToolName:    "delete_file",
			Description: "Delete one resolved File Management file after tool governance approval.",
			Schema: objectSchema(
				map[string]interface{}{
					"file_id": stringValueSchema("Required file ID from the current Files page context or governed asset resolution. Do not invent IDs."),
				},
				[]string{"file_id"},
			),
			Example: map[string]interface{}{"file_id": "file_123"},
		},
		SkillFileManager + "/save_file_to_management": {
			SkillID:     SkillFileManager,
			ToolName:    "save_file_to_management",
			Description: "Save a generated tool file or public external file URL into File Management after file.create governance allows it.",
			Schema: objectSchema(
				map[string]interface{}{
					"source_type":  enumStringSchema("Required source type. Use tool_file for a file generated by another tool; use url for a public external file URL.", []string{"tool_file", "url"}),
					"tool_file_id": stringValueSchema("Required when source_type is tool_file. Use the file_id/tool_file_id returned by the generation tool. Do not invent IDs."),
					"url":          stringValueSchema("Required when source_type is url. Must be an absolute public http or https URL supplied by the user."),
					"filename":     stringValueSchema("Required destination filename shown in File Management. Include a suitable extension and do not include path separators."),
					"workspace_id": stringValueSchema("Optional target workspace ID. Usually omit so current AIChat workspace context is used. Do not invent IDs."),
				},
				[]string{"source_type", "filename"},
			),
			Example: map[string]interface{}{"source_type": "tool_file", "tool_file_id": "tool_file_123", "filename": "report.pdf"},
		},
		SkillFileGenerator + "/generate_docx": {
			SkillID:     SkillFileGenerator,
			ToolName:    "generate_docx",
			Description: "Generate a styled DOCX temporary artifact from a structured JSON document specification. This does not write to File Management.",
			Schema: objectSchema(
				map[string]interface{}{
					"document":  stringValueSchema("JSON string describing the DOCX document. Include blocks with type heading, paragraph, table, or page_break."),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional title hint; visible content must be included in document.blocks."),
					"lifecycle": enumStringSchema("Temporary artifact lifecycle. Defaults to temporary.", []string{"persistent", "temporary"}),
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
			Description: "Generate a styled PDF temporary artifact from self-contained HTML and inline CSS. This does not write to File Management.",
			Schema: objectSchema(
				map[string]interface{}{
					"html":      stringValueSchema("Self-contained HTML body or full HTML document. Do not include external URLs, scripts, iframes, or remote assets."),
					"css":       stringValueSchema("Optional inline CSS appended to the HTML document. Prefer @page for page size and margins."),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional title used when wrapping an HTML fragment. Visible content must be included in html."),
					"lifecycle": enumStringSchema("Temporary artifact lifecycle. Defaults to temporary.", []string{"persistent", "temporary"}),
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
			Description: "Generate an editable static PPTX temporary artifact from a structured JSON presentation specification. This does not write to File Management.",
			Schema: objectSchema(
				map[string]interface{}{
					"presentation": stringValueSchema("JSON string describing the PPTX presentation. Include slides with elements of type title, text, table, or shape. Use non-overlapping boxes for readable content; omitted boxes use simple auto layout."),
					"filename":     stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":        stringValueSchema("Optional title hint; visible content must be included in presentation.slides."),
					"lifecycle":    enumStringSchema("Temporary artifact lifecycle. Defaults to temporary.", []string{"persistent", "temporary"}),
				},
				[]string{"presentation"},
			),
			Example: map[string]interface{}{
				"presentation": `{"layout":"wide","slides":[{"elements":[{"type":"title","text":"Quarterly Report","style":{"align":"center"}},{"type":"text","text":"Total revenue: 113.47","x":0.8,"y":1.4,"w":11.6,"h":0.8,"style":{"font_size":24,"bold":true,"color":"C00000"}}]}]}`,
				"filename":     "quarterly-report",
			},
		},
		SkillSensitiveRedaction + "/redact_text": {
			SkillID:     SkillSensitiveRedaction,
			ToolName:    "redact_text",
			Description: "Detect and redact sensitive information from text. Use only after source text or parsed document content is available. Never pass unredacted content to file generation; call this tool first.",
			Schema: objectSchema(
				map[string]interface{}{
					"text":     stringValueSchema("Source text to redact. Required. Do not pass binary file contents."),
					"level":    enumStringSchema("Redaction level. Defaults to medium. Use high for external sharing, model training, logs, contracts, resumes, HR data, or customer data.", []string{"low", "medium", "high"}),
					"strategy": enumStringSchema("Redaction strategy. Defaults to auto. Secrets, tokens, passwords, and private keys are fully hidden even under partial strategy.", []string{"auto", "partial", "full", "label"}),
					"preserve_rules": objectSchema(
						map[string]interface{}{
							"keep_last_digits":  numberSchema("How many trailing digits to keep for partial masking. Must be 0-8. Defaults to 4."),
							"keep_email_domain": booleanSchema("Whether to keep email domains during partial masking. Defaults to true."),
							"keep_city":         booleanSchema("Whether to keep city-level address context during partial masking. Defaults to false."),
							"keep_url_domain":   booleanSchema("Whether to keep URL domain/path while redacting sensitive query parameters. Defaults to true."),
						},
						nil,
					),
					"entity_types": map[string]interface{}{
						"description": "Optional entity type filter. Omit to scan all supported types.",
						"oneOf": []interface{}{
							arraySchema("Entity types to scan.", enumStringSchema("Entity type.", []string{"phone", "email", "id_card", "bank_card", "address", "name", "customer_name", "company", "order_id", "contract_id", "secret", "token", "password", "private_key", "ip", "url_parameter"})),
							stringValueSchema("Comma-separated entity types or JSON array string."),
						},
					},
					"locale":             enumStringSchema("Locale hint. Defaults to auto.", []string{"auto", "zh-CN", "en-US"}),
					"include_field_list": booleanSchema("Whether to return redacted field summaries. Defaults to true. Field summaries never contain complete original sensitive values."),
				},
				[]string{"text"},
			),
			Example: map[string]interface{}{
				"text":     "Name: Zhang San, phone: 13812345678, token=abcdef1234567890",
				"level":    "high",
				"strategy": "auto",
			},
		},
		SkillChartGenerator + "/generate_chart": {
			SkillID:     SkillChartGenerator,
			ToolName:    "generate_chart",
			Description: "Generate a downloadable SVG chart artifact from structured data after prompt-professionalizer has been loaded and chart type, title, data mapping, and rendering style have been provided or confirmed. Supports radar, bar, line, pie, doughnut, scatter, and score_distribution. For generic chart requests, call request_user_input before this tool.",
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
		SkillIntentRouter + "/route_intent": {
			SkillID:     SkillIntentRouter,
			ToolName:    "route_intent",
			Description: "Validate and normalize a structured intent routing result. The model must classify the user's real intent first, then pass a stable task type, confidence, recommended action, evidence, missing information, routing hints, and normalized request.",
			Schema: objectSchema(
				map[string]interface{}{
					"user_input":              stringValueSchema("The current user message being classified."),
					"context_summary":         stringValueSchema("Optional concise summary of relevant conversation context."),
					"uploaded_files":          intentUploadedFilesSchema(),
					"intent_id":               stringValueSchema("Stable dotted lowercase identifier such as file_generation.docx or database_query.filter_records."),
					"task_type":               enumStringSchema("Standard task type.", intentTaskTypes()),
					"subtype":                 stringValueSchema("Optional normalized subtype such as docx, bar, filter_records, or unknown."),
					"confidence":              boundedNumberSchema("Confidence from 0 to 1.", 0, 1),
					"recommended_action":      enumStringSchema("Recommended next action.", intentRecommendedActions()),
					"recommended_skill_id":    enumStringSchema("Optional target skill ID when recommended_action is call_skill.", []string{"file-generator", "chart-generator", "work-report-generator", "schedule-planner", "calculator", "internal-knowledge", "agent-knowledge", "internal-database", "agent-database", "agent-workflow"}),
					"recommended_tool_name":   stringValueSchema("Optional target tool name when recommended_action is call_tool."),
					"recommended_workflow_id": stringValueSchema("Optional workflow or workflow binding identifier when known."),
					"recommended_database_id": stringValueSchema("Optional database identifier when known."),
					"recommended_dataset_ids": intentStringArraySchema("Optional knowledge base or dataset IDs when known."),
					"routing_hints":           intentRoutingHintsSchema(),
					"missing_info":            intentMissingInfoSchema(),
					"evidence":                intentStringArraySchema("Evidence strings grounded in the user input or supplied context."),
					"normalized_request":      stringValueSchema("Concise restatement of what the user is actually asking."),
					"alternate_intents":       intentAlternateIntentsSchema(),
				},
				[]string{"user_input", "intent_id", "task_type", "confidence", "recommended_action", "evidence", "normalized_request"},
			),
			Example: map[string]interface{}{
				"user_input":            "Export the current report as a Word document.",
				"intent_id":             "file_generation.docx",
				"task_type":             "file_generation",
				"subtype":               "docx",
				"confidence":            0.94,
				"recommended_action":    "call_skill",
				"recommended_skill_id":  "file-generator",
				"recommended_tool_name": "generate_docx",
				"routing_hints": map[string]interface{}{
					"requires_file_generation": true,
				},
				"missing_info":       []map[string]interface{}{},
				"evidence":           []string{"User explicitly asked to export as a Word document."},
				"normalized_request": "Generate a DOCX file from the current report.",
			},
		},
		SkillArchitectureDiagram + "/generate_architecture_diagram": {
			SkillID:     SkillArchitectureDiagram,
			ToolName:    "generate_architecture_diagram",
			Description: "Generate downloadable SVG and HTML technical diagram artifacts after prompt-professionalizer has been loaded and diagram type, title, scope, and rendering style have been provided or confirmed. Supports system_architecture, agent_architecture, data_flow, flowchart, comparison_matrix, sequence, state, and er. For generic diagram requests, call request_user_input before this tool.",
			Schema: objectSchema(
				map[string]interface{}{
					"diagram_type":    enumStringSchema("Diagram type.", []string{"system_architecture", "agent_architecture", "data_flow", "flowchart", "comparison_matrix", "sequence", "state", "er"}),
					"title":           stringValueSchema("Optional diagram title."),
					"description":     stringValueSchema("Optional short subtitle or source summary."),
					"output_filename": stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"data":            architectureDiagramDataSchema(),
					"options": objectSchema(
						map[string]interface{}{
							"formats":     arraySchema("Output formats. Defaults to svg and html.", enumStringSchema("Output format.", []string{"svg", "html"})),
							"width":       numberSchema("Optional SVG width. Defaults to 1200 and must be between 480 and 2400."),
							"height":      numberSchema("Optional SVG height. Defaults to 760 and must be between 320 and 1800."),
							"style":       enumStringSchema("Rendering style. Use technical for engineering docs, business for reports, presentation for slide-ready diagrams, paper for warm report visuals, and simple when unspecified.", []string{"simple", "business", "technical", "presentation", "paper"}),
							"direction":   enumStringSchema("Layout direction.", []string{"left_to_right", "top_to_bottom"}),
							"show_legend": booleanSchema("Whether to show legend when supported. Defaults to true."),
							"show_labels": booleanSchema("Whether to show edge labels. Defaults to true."),
						},
						nil,
					),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"diagram_type", "data"},
			),
			Example: map[string]interface{}{
				"diagram_type":    "agent_architecture",
				"title":           "RAG Agent Architecture",
				"output_filename": "rag-agent-architecture",
				"data": map[string]interface{}{
					"nodes": []map[string]interface{}{
						{"id": "user", "label": "User", "type": "actor", "layer": "input"},
						{"id": "agent", "label": "Agent Orchestrator", "type": "agent", "layer": "agent"},
						{"id": "retriever", "label": "Retriever", "type": "tool", "layer": "tools"},
						{"id": "vector", "label": "Vector Store", "type": "memory", "layer": "memory"},
						{"id": "llm", "label": "LLM", "type": "model", "layer": "model"},
					},
					"edges": []map[string]interface{}{
						{"from": "user", "to": "agent", "label": "query"},
						{"from": "agent", "to": "retriever", "label": "retrieve"},
						{"from": "retriever", "to": "vector", "label": "search"},
						{"from": "agent", "to": "llm", "label": "prompt + context"},
					},
				},
				"options": map[string]interface{}{"style": "technical", "formats": []string{"svg", "html"}},
			},
		},
		SkillImageGenerator + "/generate_image": {
			SkillID:     SkillImageGenerator,
			ToolName:    "generate_image",
			Description: "Generate downloadable image files from a text prompt after prompt-professionalizer has been loaded. Supports style, aspect ratio, count, negative prompt, and optional current-user reference image URL guidance. Reference images are passed as signed URLs in the prompt, not as structured image inputs. For generic image requests, call request_user_input before this tool.",
			Schema: objectSchema(
				map[string]interface{}{
					"prompt":          stringValueSchema("Required image description. Include subject, scene, composition, intended use, and constraints."),
					"style":           imageStyleSchema(),
					"aspect_ratio":    imageAspectRatioSchema(),
					"count":           numberSchema("Number of candidate images. Must be an integer from 1 to 4."),
					"negative_prompt": stringValueSchema("Optional elements, styles, or risks to avoid."),
					"reference_image": imageFileObjectSchema("Optional current-user reference image file object or file ID. The tool places a signed URL in the prompt for loose visual guidance; it is not a structured image input."),
					"filename":        stringValueSchema("Optional base filename. Do not include path separators or an extension."),
					"lifecycle":       enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
					"provider":        stringValueSchema("Optional explicit image model provider. Usually omit this and use the default image generation model."),
					"model":           stringValueSchema("Optional explicit image generation model. Usually omit this and use the default image generation model."),
				},
				[]string{"prompt"},
			),
			Example: map[string]interface{}{"prompt": "A clean product concept image of a smart desk lamp on a white studio background", "style": "product", "aspect_ratio": "1:1", "count": 1},
		},
		SkillImageGenerator + "/edit_image": {
			SkillID:     SkillImageGenerator,
			ToolName:    "edit_image",
			Description: "Create prompt-plus-reference-URL variants or edit-style regenerated images from a current-user reference image and instruction after prompt-professionalizer has been loaded. This is not precise in-place editing and does not pass structured image input to the provider. For ambiguous edits, call request_user_input before this tool.",
			Schema: objectSchema(
				map[string]interface{}{
					"image":            imageFileObjectSchema("Required current-user reference image file object or file ID. The tool places a signed URL in the prompt for loose visual guidance; it is not a structured image input."),
					"edit_instruction": stringValueSchema("Required edit or variant instruction. State what to change, preserve, and avoid."),
					"edit_type":        enumStringSchema("Edit type.", []string{"auto", "variant", "background", "color", "add_element", "remove_element", "style_transfer"}),
					"style":            imageStyleSchema(),
					"aspect_ratio":     imageAspectRatioSchema(),
					"count":            numberSchema("Number of candidate images. Must be an integer from 1 to 4."),
					"negative_prompt":  stringValueSchema("Optional elements, styles, or risks to avoid."),
					"filename":         stringValueSchema("Optional base filename. Do not include path separators or an extension."),
					"lifecycle":        enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
					"provider":         stringValueSchema("Optional explicit image model provider. Usually omit this and use the default image generation model."),
					"model":            stringValueSchema("Optional explicit image generation model. Usually omit this and use the default image generation model."),
				},
				[]string{"image", "edit_instruction"},
			),
			Example: map[string]interface{}{"image": map[string]interface{}{"upload_file_id": "file-id"}, "edit_instruction": "Change the background to a bright office scene and keep the main product shape", "edit_type": "background", "count": 1},
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
		SkillContractFieldExtractor + "/generate_file": {
			SkillID:     SkillContractFieldExtractor,
			ToolName:    "generate_file",
			Description: "Generate a downloadable JSON, CSV, Markdown, or text file from completed contract field extraction results. Use only after contract text and configured fields have been processed and missing fields are marked explicitly.",
			Schema: objectSchema(
				map[string]interface{}{
					"content":   stringValueSchema("Final contract extraction result content to write into the generated file. Preserve missing, uncertain, conflict, confidence, evidence, and source_location fields."),
					"format":    enumStringSchema("Output format.", []string{"json", "csv", "md", "txt"}),
					"filename":  stringValueSchema("Optional display filename. Do not include path separators or an extension."),
					"title":     stringValueSchema("Optional document title used by generated file formats that support titles."),
					"lifecycle": enumStringSchema("File lifecycle. Defaults to persistent.", []string{"persistent", "temporary"}),
				},
				[]string{"content", "format"},
			),
			Example: map[string]interface{}{
				"content":  `{"contract_summary":{"field_count":2,"extracted_count":1,"missing_count":1},"fields":[{"field_key":"contract_amount","field_label":"Contract Amount","value":"CNY 120,000","normalized_value":"120000","value_type":"money","extraction_status":"extracted","confidence":0.92,"evidence":"The total contract price is CNY 120,000.","source_location":"Section 3","notes":""},{"field_key":"renewal_clause","field_label":"Renewal Clause","value":"Not found","normalized_value":"","value_type":"clause","extraction_status":"missing","confidence":0,"evidence":"","source_location":"","notes":"No explicit renewal clause was found in the contract text."}]}`,
				"format":   "json",
				"filename": "contract-field-extraction",
				"title":    "Contract Field Extraction",
			},
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
		SkillInternalDatabase + "/list_accessible_databases":             databaseListContract(SkillInternalDatabase),
		SkillInternalDatabase + "/list_database_tables":                  databaseTablesContract(SkillInternalDatabase),
		SkillInternalDatabase + "/describe_database_table":               databaseDescribeTableContract(SkillInternalDatabase),
		SkillInternalDatabase + "/query_table_records":                   databaseQueryRecordsContract(SkillInternalDatabase),
		SkillInternalDatabase + "/insert_table_records":                  databaseMutateRecordsContract(SkillInternalDatabase, "insert_table_records", "Insert records into a database table."),
		SkillInternalDatabase + "/update_table_records":                  databaseMutateRecordsContract(SkillInternalDatabase, "update_table_records", "Update records in a database table. Each record must include id."),
		SkillInternalDatabase + "/delete_table_records":                  databaseMutateRecordsContract(SkillInternalDatabase, "delete_table_records", "Delete records from a database table. Each record must include id."),
		SkillAgentDatabase + "/list_accessible_databases":                databaseListContract(SkillAgentDatabase),
		SkillAgentDatabase + "/list_database_tables":                     databaseTablesContract(SkillAgentDatabase),
		SkillAgentDatabase + "/describe_database_table":                  databaseDescribeTableContract(SkillAgentDatabase),
		SkillAgentDatabase + "/query_table_records":                      databaseQueryRecordsContract(SkillAgentDatabase),
		SkillAgentDatabase + "/insert_table_records":                     databaseMutateRecordsContract(SkillAgentDatabase, "insert_table_records", "Insert records into an Agent-bound database table."),
		SkillAgentDatabase + "/update_table_records":                     databaseMutateRecordsContract(SkillAgentDatabase, "update_table_records", "Update records in an Agent-bound database table. Each record must include id."),
		SkillAgentDatabase + "/delete_table_records":                     databaseMutateRecordsContract(SkillAgentDatabase, "delete_table_records", "Delete records from an Agent-bound database table. Each record must include id."),
		SkillAgentWorkflow + "/list_agent_workflows":                     workflowListContract(),
		SkillAgentWorkflow + "/run_agent_workflow":                       workflowRunContract(),
		SkillAgentWorkflow + "/get_workflow_run_status":                  workflowRunStatusContract(),
		SkillAgentManagement + "/list_agents":                            agentManagementListAgentsContract(),
		SkillAgentManagement + "/get_agent":                              agentManagementAgentIDContract("get_agent", "Read basic details for one resolved Agent asset visible to the current AIChat user."),
		SkillAgentManagement + "/create_agent":                           agentManagementCreateAgentContract(),
		SkillAgentManagement + "/update_agent_identity":                  agentManagementUpdateIdentityContract(),
		SkillAgentManagement + "/delete_agent":                           agentManagementAgentIDContract("delete_agent", "Delete one resolved Agent after explicit governance approval."),
		SkillAgentManagement + "/delete_agents":                          agentManagementDeleteAgentsContract(),
		SkillAgentManagement + "/get_agent_config":                       agentManagementAgentIDContract("get_agent_config", "Read the current draft runtime configuration for one resolved AGENT asset."),
		SkillAgentManagement + "/update_agent_config":                    agentManagementUpdateConfigContract(),
		SkillAgentManagement + "/replace_agent_memory_slots":             agentManagementReplaceMemorySlotsContract(),
		SkillAgentManagement + "/list_agent_skill_candidates":            agentManagementBindingCandidateContract("list_agent_skill_candidates", "List user-selectable, Agent-bindable skills for one resolved AGENT asset."),
		SkillAgentManagement + "/list_agent_knowledge_candidates":        agentManagementBindingCandidateContract("list_agent_knowledge_candidates", "List knowledge bases that can be bound to the resolved Agent."),
		SkillAgentManagement + "/list_agent_database_candidates":         agentManagementBindingCandidateContract("list_agent_database_candidates", "List databases that can be bound to the resolved Agent."),
		SkillAgentManagement + "/list_agent_database_tables":             agentManagementBindingCandidateContract("list_agent_database_tables", "List database tables that can be bound to the resolved Agent."),
		SkillAgentManagement + "/list_agent_workflow_binding_candidates": agentManagementBindingCandidateContract("list_agent_workflow_binding_candidates", "List workflows that can be bound to the resolved Agent."),
		SkillAgentManagement + "/list_available_models":                  agentManagementListAvailableModelsContract(),
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
		Description: "Patch selected draft runtime configuration fields for one resolved AGENT asset. Omitted fields are preserved. One call may update model, prompt, file upload, suggested questions, and add/remove bindings.",
		Schema: objectSchema(
			map[string]interface{}{
				"agent_id":                     stringValueSchema("Required Agent ID from page context, create_agent result, get_agent_config, or governed asset resolution. Do not invent IDs."),
				"system_prompt":                stringValueSchema("Optional replacement system prompt."),
				"model_provider":               stringValueSchema("Required whenever model is provided. Use the provider returned by list_available_models."),
				"model":                        stringValueSchema("Optional replacement model ID. Provide model_provider from the same list_available_models item."),
				"model_parameters":             objectSchema(map[string]interface{}{}, nil),
				"enabled_skill_ids":            stringArrayOrCSVSchema("Optional full list of enabled user-selectable skill IDs. Use [] to clear all user-selectable skills."),
				"add_enabled_skill_ids":        stringArrayOrCSVSchema("Optional skill IDs to add while preserving current enabled skills."),
				"remove_enabled_skill_ids":     stringArrayOrCSVSchema("Optional skill IDs to remove while preserving other enabled skills."),
				"agent_memory_enabled":         booleanSchema("Optional Agent memory switch."),
				"file_upload_enabled":          booleanSchema("Optional file upload switch."),
				"home_title":                   stringValueSchema("Optional Agent home title."),
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
				"use_case": enumStringSchema("Optional model use case. Defaults to text-chat for normal Agent runtime replacement. Use all only when the user asks to inspect every model.", []string{"text-chat", "reasoning", "vision", "function-calling", "all"}),
				"provider": stringValueSchema("Optional provider slug filter, such as openai or deepseek."),
				"limit":    numberSchema("Optional maximum number of model candidates. Defaults to 20 and is capped by the backend."),
			},
			nil,
		),
		Example: map[string]interface{}{"use_case": "text-chat", "limit": 20},
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

func validateSkillToolArgumentsAgainstContract(skillID string, toolName string, arguments map[string]interface{}) error {
	contract, ok := SkillToolArgumentContractFor(skillID, toolName)
	if !ok {
		return nil
	}
	required := schemaRequiredFields(contract.Schema)
	if len(required) == 0 {
		return nil
	}
	missing := make([]string, 0, len(required))
	for _, field := range required {
		if !argumentValuePresent(arguments[field]) {
			missing = append(missing, field)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("skill tool %s/%s missing required argument(s): %s", normalizeSkillID(skillID), strings.TrimSpace(toolName), strings.Join(missing, ", "))
}

func schemaRequiredFields(schema map[string]interface{}) []string {
	if len(schema) == 0 {
		return nil
	}
	values, ok := schema["required"]
	if !ok || values == nil {
		return nil
	}
	switch typed := values.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			if text := strings.TrimSpace(value); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func argumentValuePresent(value interface{}) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []interface{}:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case []map[string]interface{}:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return true
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
		"bands": scoreRangeBands,
		"scores": arraySchema("Raw score values or objects with value.", map[string]interface{}{"oneOf": []interface{}{
			numberSchema("Raw score value."),
			objectSchema(map[string]interface{}{
				"label": stringValueSchema("Optional score label."),
				"value": numberSchema("Raw score value."),
			}, []string{"value"}),
		}}),
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

func architectureDiagramDataSchema() map[string]interface{} {
	node := objectSchema(map[string]interface{}{
		"id":    stringValueSchema("Stable node ID. Edges must reference this value."),
		"label": stringValueSchema("Human-readable node label."),
		"type":  stringValueSchema("Optional node type such as frontend, service, database, agent, model, tool, memory, input, output, or approval."),
		"group": stringValueSchema("Optional logical group."),
		"layer": stringValueSchema("Optional layout layer used to order nodes."),
	}, []string{"id"})
	edge := objectSchema(map[string]interface{}{
		"from":  stringValueSchema("Source node ID, participant name, state ID, or entity ID."),
		"to":    stringValueSchema("Target node ID, participant name, state ID, or entity ID."),
		"label": stringValueSchema("Optional relationship, transition, message, or data-flow label."),
	}, []string{"from", "to"})
	group := objectSchema(map[string]interface{}{
		"id":    stringValueSchema("Group ID."),
		"label": stringValueSchema("Group label."),
	}, []string{"id"})
	entity := objectSchema(map[string]interface{}{
		"id":     stringValueSchema("Stable entity ID. Relationships must reference this value."),
		"label":  stringValueSchema("Entity label."),
		"fields": stringArrayOrCSVSchema("Optional entity fields such as id PK, user_id FK, status."),
	}, []string{"id"})
	matrixCells := arraySchema("Matrix cell rows. Must align with rows and columns.", map[string]interface{}{
		"type":  "array",
		"items": map[string]interface{}{"type": "string"},
	})
	nodeEdgeProps := map[string]interface{}{
		"nodes":  arraySchema("Diagram nodes.", node),
		"edges":  arraySchema("Diagram edges.", edge),
		"groups": arraySchema("Optional visual or logical groups.", group),
	}
	return map[string]interface{}{
		"description": "Diagram-specific data. Node-edge diagrams use nodes, edges, and optional groups; comparison_matrix uses rows, columns, and cells; sequence uses participants and messages; state uses states and transitions; er uses entities and relationships.",
		"anyOf": []interface{}{
			objectSchema(nodeEdgeProps, []string{"nodes", "edges"}),
			objectSchema(map[string]interface{}{
				"columns": stringArrayOrCSVSchema("Compared products, vendors, options, or plans."),
				"rows":    stringArrayOrCSVSchema("Comparison criteria, features, metrics, or factors."),
				"cells":   matrixCells,
			}, []string{"columns", "rows", "cells"}),
			objectSchema(map[string]interface{}{
				"participants": stringArrayOrCSVSchema("Ordered sequence participants."),
				"messages":     arraySchema("Ordered sequence messages.", edge),
			}, []string{"participants", "messages"}),
			objectSchema(map[string]interface{}{
				"states":      arraySchema("State nodes.", node),
				"transitions": arraySchema("State transitions.", edge),
			}, []string{"states", "transitions"}),
			objectSchema(map[string]interface{}{
				"entities":      arraySchema("ER entities.", entity),
				"relationships": arraySchema("ER relationships.", edge),
			}, []string{"entities", "relationships"}),
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

func imageStyleSchema() map[string]interface{} {
	return enumStringSchema("Visual style. Defaults to auto.", []string{"auto", "realistic", "illustration", "flat", "3d", "guofeng", "tech", "poster", "product", "icon", "cover"})
}

func imageAspectRatioSchema() map[string]interface{} {
	return enumStringSchema("Image aspect ratio. Defaults to 1:1.", []string{"1:1", "16:9", "9:16", "4:3"})
}

func imageFileObjectSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"description": description + " Supported formats: PNG, JPG, JPEG, WEBP.",
		"anyOf": []interface{}{
			stringValueSchema("File object encoded as JSON, or a file ID string."),
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"upload_file_id": stringValueSchema("Uploaded file ID."),
					"file_id":        stringValueSchema("Uploaded file ID."),
					"id":             stringValueSchema("Uploaded file ID."),
					"related_id":     stringValueSchema("Related uploaded file ID."),
					"name":           stringValueSchema("Optional filename."),
					"mime_type":      stringValueSchema("Optional MIME type."),
				},
				"additionalProperties": true,
			},
		},
	}
}

func booleanSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "boolean",
		"description": description,
	}
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

func intentStringArraySchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"description": description,
		"oneOf": []interface{}{
			map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			map[string]interface{}{
				"type":        "string",
				"description": "JSON array string of strings.",
			},
		},
	}
}

func boundedNumberSchema(description string, minimum float64, maximum float64) map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": description,
		"minimum":     minimum,
		"maximum":     maximum,
	}
}

func intentTaskTypes() []string {
	return []string{
		"general_qa",
		"knowledge_retrieval",
		"database_query",
		"database_mutation",
		"workflow_execution",
		"file_generation",
		"chart_generation",
		"report_generation",
		"schedule_planning",
		"calculation",
		"code_or_debugging",
		"data_analysis",
		"clarification_required",
		"unsupported",
	}
}

func intentRecommendedActions() []string {
	return []string{
		"answer_directly",
		"call_skill",
		"call_tool",
		"run_workflow",
		"query_database",
		"mutate_database",
		"retrieve_knowledge",
		"request_user_input",
		"reject_or_escalate",
	}
}

func intentRoutingHintsSchema() map[string]interface{} {
	return objectSchema(
		map[string]interface{}{
			"needs_context":             booleanSchema("Whether more conversation or domain context is needed."),
			"uses_uploaded_files":       booleanSchema("Whether uploaded files are required for execution."),
			"requires_database":         booleanSchema("Whether database access is required."),
			"requires_knowledge_base":   booleanSchema("Whether knowledge retrieval is required."),
			"requires_workflow":         booleanSchema("Whether workflow execution or inspection is required."),
			"requires_file_generation":  booleanSchema("Whether file generation is required."),
			"requires_chart_generation": booleanSchema("Whether chart generation is required."),
			"requires_confirmation":     booleanSchema("Whether explicit user confirmation is required."),
			"is_high_impact":            booleanSchema("Whether the next action is high impact."),
			"is_multi_intent":           booleanSchema("Whether the request contains multiple task intents."),
		},
		nil,
	)
}

func intentMissingInfoSchema() map[string]interface{} {
	return arraySchema(
		"Missing information that blocks reliable execution.",
		objectSchema(
			map[string]interface{}{
				"field":    stringValueSchema("Stable missing field name such as chart_type, file_format, database_table, workflow_binding_id, or confirmation."),
				"reason":   stringValueSchema("Why this field is required."),
				"question": stringValueSchema("Concise user-facing question that resolves this blocker."),
				"options":  intentStringArraySchema("Optional concrete quick-reply options, maximum five."),
			},
			[]string{"field", "reason", "question"},
		),
	)
}

func intentUploadedFilesSchema() map[string]interface{} {
	return arraySchema(
		"Uploaded file metadata relevant to routing. Do not include raw file contents.",
		map[string]interface{}{
			"anyOf": []interface{}{
				objectSchema(intentUploadedFileProperties(), []string{"file_id"}),
				objectSchema(intentUploadedFileProperties(), []string{"filename"}),
			},
		},
	)
}

func intentUploadedFileProperties() map[string]interface{} {
	return map[string]interface{}{
		"file_id":   stringValueSchema("Optional file identifier."),
		"filename":  stringValueSchema("Optional filename."),
		"mime_type": stringValueSchema("Optional MIME type."),
		"format":    stringValueSchema("Optional file format or extension."),
		"role":      stringValueSchema("Optional role such as source, reference, attachment, or output_template."),
		"summary":   stringValueSchema("Optional short file summary."),
	}
}

func intentAlternateIntentsSchema() map[string]interface{} {
	return arraySchema(
		"Optional secondary plausible intents.",
		objectSchema(
			map[string]interface{}{
				"intent_id":            stringValueSchema("Stable dotted lowercase identifier for the alternate intent."),
				"task_type":            enumStringSchema("Alternate task type.", intentTaskTypes()),
				"confidence":           boundedNumberSchema("Alternate confidence from 0 to 1.", 0, 1),
				"recommended_action":   enumStringSchema("Optional recommended action for the alternate intent.", intentRecommendedActions()),
				"recommended_skill_id": stringValueSchema("Optional target skill for the alternate intent."),
			},
			[]string{"intent_id", "task_type", "confidence"},
		),
	)
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
	if _, filename, _, ok := goruntime.Caller(0); ok {
		candidate := filepath.Join(filepath.Dir(filename), "catalog")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return defaultCatalogDir
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
	locations, err := r.systemSkillLocations()
	if err != nil {
		return SkillDocument{}, err
	}
	location, ok := locations[id]
	if !ok {
		return SkillDocument{}, fmt.Errorf("skill %s not found: %w", id, ErrSkillNotFound)
	}
	return r.loadSkillDocumentFromLocation(location)
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
	raw, err := readSkillLocationFile(location, "SKILL.md")
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
	doc, err := buildSkillDocument(id, root, source, frontmatter, body, listLocationReferences(location), hasLocationScripts(location))
	if err != nil {
		return SkillDocument{}, err
	}
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

func buildSkillDocument(id string, root string, source string, frontmatter SkillFrontmatter, body string, references []SkillReference, scriptPresent bool) (SkillDocument, error) {
	whenToUse := strings.TrimSpace(frontmatter.WhenToUse)
	if normalizeSkillSource(source) == SkillSourceCustom && whenToUse == "" {
		whenToUse = strings.TrimSpace(frontmatter.Description)
	}
	tools, err := buildSkillToolDefinitions(id, frontmatter)
	if err != nil {
		return SkillDocument{}, err
	}
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
			References:       references,
			HasScripts:       scriptPresent,
			ScriptsSupported: false,
			RootPath:         root,
			SupportedCallers: normalizeSkillCallers(id, source, frontmatter.SupportedCallers),
			RequiredConfig:   normalizeSkillRequiredConfig(id, frontmatter.RequiredConfig),
		},
		Instructions: strings.TrimSpace(body),
		Tools:        tools,
	}, nil
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
		case SkillCallerWorkflow:
			if _, ok := seen[SkillCallerWorkflow]; !ok {
				seen[SkillCallerWorkflow] = struct{}{}
				out = append(out, SkillCallerWorkflow)
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

func buildSkillToolDefinitions(skillID string, frontmatter SkillFrontmatter) ([]SkillToolDefinition, error) {
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
		manifest, ok, err := skillToolGovernanceManifest(skillID, name, frontmatter.ToolGovernance)
		if err != nil {
			return nil, err
		}
		if ok {
			def.Governance = &manifest
		}
		defs = append(defs, def)
	}
	return defs, nil
}

func skillToolGovernanceManifest(skillID string, toolName string, manifests map[string]toolgovernance.Manifest) (toolgovernance.Manifest, bool, error) {
	if len(manifests) == 0 {
		return toolgovernance.Manifest{}, false, nil
	}
	manifest, ok := manifests[strings.TrimSpace(toolName)]
	if !ok {
		manifest, ok = manifests[strings.ToLower(strings.TrimSpace(toolName))]
	}
	if !ok {
		return toolgovernance.Manifest{}, false, nil
	}
	normalized, err := toolgovernance.ValidateManifest(manifest)
	if err != nil {
		return toolgovernance.Manifest{}, false, fmt.Errorf("skill %s tool %s governance manifest is invalid: %w", normalizeSkillID(skillID), strings.TrimSpace(toolName), err)
	}
	return normalized, true, nil
}

func (r *Runtime) listReferences(skillID string) []SkillReference {
	id := normalizeSkillID(skillID)
	locations, err := r.systemSkillLocations()
	if err != nil {
		return nil
	}
	return listLocationReferences(locations[id])
}

func (r *Runtime) hasScripts(skillID string) bool {
	id := normalizeSkillID(skillID)
	locations, err := r.systemSkillLocations()
	if err != nil {
		return false
	}
	return hasLocationScripts(locations[id])
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
