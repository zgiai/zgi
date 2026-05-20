package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zgiai/ginext/internal/modules/tools"
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
	entries, err := os.ReadDir(r.catalogDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill catalog: %w", err)
	}
	locations := make(map[string]skillLocation, len(entries)+len(custom))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := normalizeSkillID(entry.Name())
		if id == "" {
			continue
		}
		if !isValidSkillName(id) {
			return nil, fmt.Errorf("invalid skill directory %s: use lowercase letters, numbers, and hyphens only", entry.Name())
		}
		locations[id] = skillLocation{ID: id, Root: filepath.Join(r.catalogDir, id), Source: SkillSourceSystem}
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

func referenceFullPath(doc SkillDocument, referencePath string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(referencePath)))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid skill reference path")
	}
	for _, ref := range doc.Metadata.References {
		if filepath.ToSlash(ref.Path) == clean {
			if strings.TrimSpace(ref.FullPath) == "" {
				return "", fmt.Errorf("invalid skill reference path")
			}
			return ref.FullPath, nil
		}
	}
	return "", fmt.Errorf("skill reference %s is not available", clean)
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
	payload, err := json.Marshal(metadata)
	if err != nil {
		payload = []byte("[]")
	}
	return "The following skills are enabled for this AIChat turn. Only lightweight skill metadata is shown. If a skill is useful, call load_skill first to read its SKILL.md instructions. Do not call business tools before loading the skill. Enabled skills JSON: " + string(payload)
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
