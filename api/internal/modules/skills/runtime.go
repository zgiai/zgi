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

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gopkg.in/yaml.v3"
)

const (
	defaultMaxCallsPerTurn = 3
	defaultTimeoutSeconds  = 5
	defaultCatalogDir      = "internal/modules/skills/catalog"
	defaultDisplayIcon     = "sparkles"
	defaultLocale          = "en_US"

	MetaToolLoadSkill          = "load_skill"
	MetaToolReadSkillReference = "read_skill_reference"
	MetaToolCallSkillTool      = "call_skill_tool"
)

var ErrSkillNotFound = errors.New("skill not found")

type Runtime struct {
	engine     *tools.ToolEngine
	manager    *tools.ToolManager
	catalogDir string
}

type skillLocation struct {
	ID     string
	Root   string
	Source string
}

type ExecutionContext struct {
	TenantID       string
	UserID         string
	ConversationID string
	AppID          string
	MessageID      string
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
	if r == nil || r.engine == nil {
		return nil, fmt.Errorf("tool engine is not configured")
	}
	doc, ok := resolved.Get(skillID)
	if !ok {
		return nil, fmt.Errorf("skill %s is not enabled", normalizeSkillID(skillID))
	}
	toolDef, ok := findSkillTool(*doc, toolName)
	if !ok {
		return nil, fmt.Errorf("tool %s is not available in skill %s", strings.TrimSpace(toolName), doc.Metadata.ID)
	}

	timeout := docTimeoutSeconds(*doc)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	start := time.Now()
	result, err := r.engine.Invoke(runCtx, tools.InvokeRequest{
		ProviderType:   toolDef.ProviderType,
		ProviderID:     toolDef.ProviderID,
		ToolName:       toolDef.Name,
		TenantID:       execCtx.TenantID,
		UserID:         execCtx.UserID,
		Parameters:     arguments,
		ConversationID: execCtx.ConversationID,
		AppID:          execCtx.AppID,
		MessageID:      execCtx.MessageID,
		InvokeFrom:     tools.ToolInvokeFromAIChat,
	})
	trace := SkillTrace{
		Kind:       "tool_call",
		SkillID:    doc.Metadata.ID,
		ToolName:   toolDef.Name,
		Status:     "success",
		DurationMS: time.Since(start).Milliseconds(),
		Arguments:  summarizeArguments(arguments),
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

func MetaTools() []llmadapter.Tool {
	return metaTools(true)
}

func MetaToolsForSkills(resolved *ResolvedSkills) []llmadapter.Tool {
	return metaTools(resolvedHasToolSkills(resolved))
}

func metaTools(includeToolCaller bool) []llmadapter.Tool {
	tools := []llmadapter.Tool{
		{
			Type: "function",
			Function: llmadapter.Function{
				Name:        MetaToolLoadSkill,
				Description: "Load the full instructions for an enabled skill before using that skill.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"skill_id": map[string]interface{}{
							"type":        "string",
							"description": "The enabled skill ID to load.",
						},
					},
					"required": []string{"skill_id"},
				},
			},
		},
		{
			Type: "function",
			Function: llmadapter.Function{
				Name:        MetaToolReadSkillReference,
				Description: "Read a reference document from a loaded skill when SKILL.md says it is relevant.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"skill_id": map[string]interface{}{
							"type":        "string",
							"description": "The enabled skill ID that owns the reference.",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Reference path relative to the skill references directory.",
						},
					},
					"required": []string{"skill_id", "path"},
				},
			},
		},
	}
	if includeToolCaller {
		tools = append(tools, llmadapter.Tool{
			Type: "function",
			Function: llmadapter.Function{
				Name:        MetaToolCallSkillTool,
				Description: "Call a tool allowed by a loaded skill after reading its instructions.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"skill_id": map[string]interface{}{
							"type":        "string",
							"description": "The enabled skill ID that allows the tool.",
						},
						"tool_name": map[string]interface{}{
							"type":        "string",
							"description": "The allowed tool name to call.",
						},
						"arguments": map[string]interface{}{
							"type":        "object",
							"description": "Arguments for the skill tool.",
						},
					},
					"required": []string{"skill_id", "tool_name", "arguments"},
				},
			},
		})
	}
	return tools
}

func SkillMetadataSystemMessage(metadata []SkillPromptMetadata) llmadapter.Message {
	return llmadapter.Message{
		Role:    "system",
		Content: skillMetadataPrompt(metadata),
	}
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
			TimeoutSeconds:   normalizePositive(frontmatter.TimeoutSeconds, defaultTimeoutSeconds),
			References:       listReferences(root, source),
			HasScripts:       hasScripts(root),
			ScriptsSupported: false,
			RootPath:         root,
		},
		Instructions: strings.TrimSpace(body),
		Tools:        buildSkillToolDefinitions(frontmatter),
	}
}

func parseSkillMarkdown(raw []byte) (SkillFrontmatter, string, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
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
	if len(doc.Metadata.Tools) > 0 || len(doc.Tools) > 0 {
		return fmt.Errorf("custom skill %s must not declare tools", doc.Metadata.ID)
	}
	if err := validateSkillDocument(doc); err != nil {
		return err
	}
	return nil
}

func buildSkillToolDefinitions(frontmatter SkillFrontmatter) []SkillToolDefinition {
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
		defs = append(defs, SkillToolDefinition{
			Name:         name,
			ProviderType: providerType,
			ProviderID:   strings.TrimSpace(frontmatter.ProviderID),
		})
	}
	return defs
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
