package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gopkg.in/yaml.v3"
)

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
