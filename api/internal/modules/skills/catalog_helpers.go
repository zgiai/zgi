package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	DefaultSkillMetadataPromptBudgetChars = 8000
	skillMetadataLongFieldBudgetChars     = 600
	skillMetadataShortFieldBudgetChars    = 200
)

func (r *Runtime) validateSkillReferences(doc SkillDocument) error {
	for _, ref := range doc.Metadata.References {
		if _, err := referenceFullPath(doc, ref.Path); err != nil {
			return fmt.Errorf("skill %s reference %s is invalid: %w", doc.Metadata.ID, ref.Path, err)
		}
	}
	return nil
}

func (r *Runtime) skillLocations(custom []CustomSkillCatalogEntry) (map[string]skillLocation, error) {
	if r == nil {
		return nil, fmt.Errorf("skill runtime is not configured")
	}
	locations, err := r.systemSkillLocations()
	if err != nil {
		return nil, err
	}
	for _, entry := range custom {
		id := normalizeSkillID(entry.SkillID)
		if id == "" {
			continue
		}
		if !isValidSkillName(id) {
			return nil, fmt.Errorf("invalid custom skill id %s: use lowercase letters, numbers, and hyphens only", id)
		}
		root := strings.TrimSpace(entry.Root)
		if root == "" {
			return nil, fmt.Errorf("custom skill %s storage path is required", id)
		}
		locations[id] = skillLocation{ID: id, Root: root, Source: SkillSourceCustom}
	}
	return locations, nil
}

func (r *Runtime) systemSkillLocations() (map[string]skillLocation, error) {
	locations, errs, err := r.systemSkillLocationsFromEntries(false)
	if err != nil {
		return nil, err
	}
	if joined := errors.Join(errs...); joined != nil {
		return nil, joined
	}
	return locations, nil
}

func (r *Runtime) systemSkillLocationsBestEffort() (map[string]skillLocation, []error, error) {
	return r.systemSkillLocationsFromEntries(true)
}

func (r *Runtime) systemSkillLocationsFromEntries(bestEffort bool) (map[string]skillLocation, []error, error) {
	entries, embedded, err := r.systemCatalogEntries()
	if err != nil {
		return nil, nil, err
	}
	locations := make(map[string]skillLocation, len(entries))
	errs := make([]error, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := normalizeSkillID(entry.Name())
		if id == "" {
			continue
		}
		if !isValidSkillName(id) {
			err := fmt.Errorf("invalid skill directory %s: use lowercase letters, numbers, and hyphens only", entry.Name())
			if !bestEffort {
				return nil, nil, err
			}
			errs = append(errs, err)
			continue
		}
		location := skillLocation{ID: id, Root: filepath.Join(r.catalogDir, id), Source: SkillSourceSystem}
		if embedded {
			location.Root = path.Join("catalog", id)
			location.Embedded = true
		}
		locations[id] = location
	}
	return locations, errs, nil
}

func (r *Runtime) systemCatalogEntries() ([]fs.DirEntry, bool, error) {
	dir := strings.TrimSpace(r.catalogDir)
	if dir == "" {
		dir = defaultCatalogDir
	}
	entries, err := os.ReadDir(dir)
	if err == nil {
		return entries, false, nil
	}
	if !isDefaultCatalogDir(dir) {
		return nil, false, fmt.Errorf("failed to read skill catalog: %w", err)
	}
	entries, embeddedErr := embeddedSkillCatalog.ReadDir("catalog")
	if embeddedErr != nil {
		return nil, false, fmt.Errorf("failed to read skill catalog: filesystem: %v; embedded: %w", err, embeddedErr)
	}
	return entries, true, nil
}

func isDefaultCatalogDir(dir string) bool {
	return filepath.Clean(strings.TrimSpace(dir)) == filepath.Clean(defaultCatalogDir)
}

func listReferences(root string, source string) []SkillReference {
	if normalizeSkillSource(source) == SkillSourceCustom {
		refs := listRootTextReferences(root)
		sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
		return refs
	}
	refs := listReferencesUnder(filepath.Join(root, "references"), "references", true)
	sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
	return refs
}

func listLocationReferences(location skillLocation) []SkillReference {
	if normalizeSkillSource(location.Source) == SkillSourceCustom {
		return listReferences(location.Root, SkillSourceCustom)
	}
	if location.Embedded {
		refs := listEmbeddedReferencesUnder(path.Join(location.Root, "references"), "references", true)
		sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
		return refs
	}
	return listReferences(location.Root, SkillSourceSystem)
}

func listEmbeddedReferencesUnder(root string, pathPrefix string, stripPrefix bool) []SkillReference {
	entries, err := embeddedSkillCatalog.ReadDir(filepath.ToSlash(root))
	if err != nil {
		return nil
	}
	refs := make([]SkillReference, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isReadableReferenceFile(name) {
			continue
		}
		refPath := name
		if !stripPrefix {
			refPath = filepath.ToSlash(filepath.Join(pathPrefix, name))
		}
		refs = append(refs, SkillReference{Path: refPath, Name: name, FullPath: path.Join(root, name), Embedded: true})
	}
	return refs
}

func listReferencesUnder(root string, pathPrefix string, stripPrefix bool) []SkillReference {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	refs := make([]SkillReference, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isReadableReferenceFile(name) {
			continue
		}
		refPath := name
		if !stripPrefix {
			refPath = filepath.ToSlash(filepath.Join(pathPrefix, name))
		}
		refs = append(refs, SkillReference{Path: refPath, Name: name, FullPath: filepath.Join(root, name)})
	}
	return refs
}

func listRootTextReferences(root string) []SkillReference {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	refs := make([]SkillReference, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.EqualFold(name, "SKILL.md") || !isReadableReferenceFile(name) {
			continue
		}
		refs = append(refs, SkillReference{Path: name, Name: name, FullPath: filepath.Join(root, name)})
	}
	return append(refs, listReferencesUnder(filepath.Join(root, "references"), "references", false)...)
}

func hasScripts(root string) bool {
	entries, err := os.ReadDir(filepath.Join(root, "scripts"))
	return err == nil && len(entries) > 0
}

func hasLocationScripts(location skillLocation) bool {
	if location.Embedded {
		entries, err := embeddedSkillCatalog.ReadDir(path.Join(location.Root, "scripts"))
		return err == nil && len(entries) > 0
	}
	return hasScripts(location.Root)
}

func readSkillLocationFile(location skillLocation, relativePath string) ([]byte, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relativePath)))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(clean) {
		return nil, fmt.Errorf("invalid skill path")
	}
	if location.Embedded {
		return embeddedSkillCatalog.ReadFile(path.Join(location.Root, clean))
	}
	return os.ReadFile(filepath.Join(location.Root, filepath.FromSlash(clean)))
}

func referenceFullPath(doc SkillDocument, referencePath string) (string, error) {
	ref, err := skillReference(doc, referencePath)
	if err != nil {
		return "", err
	}
	return ref.FullPath, nil
}

func skillReference(doc SkillDocument, referencePath string) (SkillReference, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(referencePath)))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(clean) {
		return SkillReference{}, fmt.Errorf("invalid skill reference path")
	}
	for _, ref := range doc.Metadata.References {
		if filepath.ToSlash(ref.Path) == clean {
			if strings.TrimSpace(ref.FullPath) == "" {
				return SkillReference{}, fmt.Errorf("invalid skill reference path")
			}
			return ref, nil
		}
	}
	return SkillReference{}, fmt.Errorf("skill reference %s is not available", clean)
}

func readSkillReference(ref SkillReference) ([]byte, error) {
	if ref.Embedded {
		return embeddedSkillCatalog.ReadFile(filepath.ToSlash(ref.FullPath))
	}
	return os.ReadFile(ref.FullPath)
}

func isReadableReferenceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".txt", ".json", ".csv":
		return true
	default:
		return false
	}
}

func normalizeSkillSource(source string) string {
	if strings.ToLower(strings.TrimSpace(source)) == SkillSourceCustom {
		return SkillSourceCustom
	}
	return SkillSourceSystem
}

func resolvedHasToolSkills(resolved *ResolvedSkills) bool {
	if resolved == nil {
		return false
	}
	for _, doc := range resolved.Skills {
		if len(doc.Tools) > 0 {
			return true
		}
	}
	return false
}

func docTimeoutSeconds(doc SkillDocument) int {
	if doc.Metadata.TimeoutSeconds <= 0 {
		return defaultTimeoutSeconds
	}
	return doc.Metadata.TimeoutSeconds
}

func normalizePositive(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func normalizeSkillRuntimeType(raw string, toolNames []string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case SkillRuntimeTypeTool, SkillRuntimeTypePrompt, SkillRuntimeTypeHybrid:
		return value
	}
	if len(toolNames) > 0 {
		return SkillRuntimeTypeTool
	}
	return SkillRuntimeTypePrompt
}

func normalizeSkillDisplay(frontmatter SkillFrontmatter) SkillDisplayMetadata {
	return normalizeSkillDisplayWithFallback(frontmatter, strings.TrimSpace(frontmatter.WhenToUse))
}

func normalizeSkillDisplayWithFallback(frontmatter SkillFrontmatter, whenToUse string) SkillDisplayMetadata {
	display := frontmatter.Display
	display.Icon = strings.TrimSpace(display.Icon)
	display.Category = strings.TrimSpace(display.Category)
	display.Scenarios = normalizeSkillIDs(display.Scenarios)
	display.Label = normalizeLocalizedText(display.Label, strings.TrimSpace(frontmatter.Name))
	display.Description = normalizeLocalizedText(display.Description, strings.TrimSpace(frontmatter.Description))
	display.WhenToUse = normalizeLocalizedText(display.WhenToUse, strings.TrimSpace(whenToUse))
	display.Tags = normalizeLocalizedTags(display.Tags)
	if display.Icon == "" {
		display.Icon = defaultDisplayIcon
	}
	return display
}

func normalizeLocalizedText(values map[string]string, fallback string) map[string]string {
	out := make(map[string]string, len(values)+1)
	for locale, value := range values {
		key := strings.TrimSpace(locale)
		text := strings.TrimSpace(value)
		if key == "" || text == "" {
			continue
		}
		out[key] = text
	}
	if _, ok := out[defaultLocale]; !ok && fallback != "" {
		out[defaultLocale] = fallback
	}
	return out
}

func normalizeLocalizedTags(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string][]string, len(values))
	for locale, tags := range values {
		key := strings.TrimSpace(locale)
		if key == "" {
			continue
		}
		normalized := make([]string, 0, len(tags))
		seen := make(map[string]struct{}, len(tags))
		for _, tag := range tags {
			text := strings.TrimSpace(tag)
			if text == "" {
				continue
			}
			if _, ok := seen[text]; ok {
				continue
			}
			seen[text] = struct{}{}
			normalized = append(normalized, text)
		}
		if len(normalized) > 0 {
			out[key] = normalized
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeSkillIDs(skillIDs []string) []string {
	seen := make(map[string]struct{}, len(skillIDs))
	out := make([]string, 0, len(skillIDs))
	for _, raw := range skillIDs {
		id := normalizeSkillID(raw)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func isValidSkillName(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func skillMetadataPrompt(metadata []SkillPromptMetadata) string {
	content, _ := skillMetadataPromptWithBudget(metadata, DefaultSkillMetadataPromptBudgetChars)
	return content
}

func skillMetadataPromptWithBudget(metadata []SkillPromptMetadata, budgetChars int) (string, SkillMetadataPromptStats) {
	if budgetChars <= 0 {
		budgetChars = DefaultSkillMetadataPromptBudgetChars
	}
	stats := SkillMetadataPromptStats{EnabledCount: len(metadata)}
	exposed := make([]SkillPromptMetadata, 0, len(metadata))
	for _, item := range metadata {
		candidate, truncated := skillPromptMetadataWithFieldBudget(item, skillMetadataLongFieldBudgetChars)
		if promptFitsBudget(exposed, candidate, stats, truncated, budgetChars) {
			exposed = append(exposed, candidate)
			stats.ExposedCount = len(exposed)
			stats.Truncated = stats.Truncated || truncated
			continue
		}
		candidate, truncated = skillPromptMetadataWithFieldBudget(item, skillMetadataShortFieldBudgetChars)
		if promptFitsBudget(exposed, candidate, stats, truncated, budgetChars) {
			exposed = append(exposed, candidate)
			stats.ExposedCount = len(exposed)
			stats.Truncated = true
			continue
		}
		candidate.Description = ""
		candidate.WhenToUse = ""
		if promptFitsBudget(exposed, candidate, stats, true, budgetChars) {
			exposed = append(exposed, candidate)
			stats.ExposedCount = len(exposed)
			stats.Truncated = true
			continue
		}
		break
	}
	stats.ExposedCount = len(exposed)
	stats.OmittedCount = len(metadata) - len(exposed)
	if stats.OmittedCount > 0 {
		stats.Truncated = true
	}
	return skillMetadataPromptContent(exposed, stats), stats
}

func skillMetadataPromptContent(metadata []SkillPromptMetadata, stats SkillMetadataPromptStats) string {
	payload, err := json.Marshal(metadata)
	if err != nil {
		payload = []byte("[]")
	}
	note := ""
	if stats.Truncated {
		note = fmt.Sprintf(" Metadata was shortened or omitted to fit the discovery budget. enabled_count=%d exposed_count=%d omitted_count=%d.", stats.EnabledCount, stats.ExposedCount, stats.OmittedCount)
	}
	return "The following skills are enabled for this AIChat turn. Only lightweight skill metadata is shown. If a skill is useful, call load_skill first to read its SKILL.md instructions. Do not call business tools before loading the skill." + note + " Enabled skills JSON: " + string(payload)
}

func promptFitsBudget(existing []SkillPromptMetadata, candidate SkillPromptMetadata, stats SkillMetadataPromptStats, truncated bool, budgetChars int) bool {
	next := append(append([]SkillPromptMetadata{}, existing...), candidate)
	stats.ExposedCount = len(next)
	stats.OmittedCount = stats.EnabledCount - stats.ExposedCount
	stats.Truncated = stats.Truncated || truncated || stats.OmittedCount > 0
	return utf8.RuneCountInString(skillMetadataPromptContent(next, stats)) <= budgetChars
}

func skillPromptMetadataWithFieldBudget(item SkillPromptMetadata, budget int) (SkillPromptMetadata, bool) {
	var truncated bool
	item.Description, truncated = truncatePromptMetadataField(item.Description, budget)
	whenToUse, whenTruncated := truncatePromptMetadataField(item.WhenToUse, budget)
	item.WhenToUse = whenToUse
	return item, truncated || whenTruncated
}

func truncatePromptMetadataField(value string, budget int) (string, bool) {
	text := strings.TrimSpace(value)
	if budget <= 0 || utf8.RuneCountInString(text) <= budget {
		return text, false
	}
	runes := []rune(text)
	return string(runes[:budget]), true
}

func toolMessagesContent(messages []tools.ToolInvokeMessage) string {
	if len(messages) == 1 && messages[0].Type == tools.ToolInvokeMessageTypeText {
		return messages[0].Text
	}
	payload, err := json.Marshal(messages)
	if err != nil {
		return fmt.Sprintf("%v", messages)
	}
	return string(payload)
}

func summarizeArguments(args map[string]interface{}) map[string]interface{} {
	if len(args) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(args))
	for key, value := range args {
		out[key] = summarizeValue(value)
	}
	return out
}

func summarizeValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case string:
		if len(typed) > 256 {
			return typed[:256]
		}
		return typed
	case float64, float32, int, int64, int32, bool, nil:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		text := string(data)
		if len(text) > 256 {
			return text[:256]
		}
		return text
	}
}
