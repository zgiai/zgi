package service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	consoleFilesReadIntentPattern       = regexp.MustCompile(`(?i)\b(read|preview|summari[sz]e|summary|analy[sz]e|analysis|inspect|show|translate|translation|abstract|digest|extract)\b`)
	consoleFilesDeleteIntentPattern     = regexp.MustCompile(`(?i)\b(delete|remove|trash|discard)\b`)
	consoleFilesCapabilityPattern       = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])file\.read([^a-z0-9_.-]|$)`)
	consoleFilesDeleteCapabilityPattern = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])file\.delete([^a-z0-9_.-]|$)`)
	consoleFilesCreateCapabilityPattern = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])file\.create([^a-z0-9_.-]|$)`)
)

func isConsoleFilesContext(_ string, contexts ...map[string]interface{}) bool {
	for _, ctx := range contexts {
		if contextContainsConsoleFilesPageResource(ctx) {
			return true
		}
	}
	return false
}

func contextContainsConsoleFilesPageResource(context map[string]interface{}) bool {
	if len(context) == 0 {
		return false
	}
	for _, item := range operationItemsFromValue(context["resources"]) {
		resource, ok := item.(map[string]interface{})
		if !ok || !isConsoleFilesPageResource(resource) {
			continue
		}
		return true
	}
	return false
}

func isConsoleFilesPageResource(resource map[string]interface{}) bool {
	if len(resource) == 0 {
		return false
	}
	resourceType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(resource["resource_type"], resource["type"], resource["kind"])))
	if resourceType != "page" {
		return false
	}
	metadata := mapFromOperationContext(resource["metadata"])
	for _, value := range []interface{}{
		resource["resource_id"],
		resource["id"],
		resource["title"],
		resource["href"],
		metadata["page"],
		metadata["route"],
	} {
		if isConsoleFilesPageToken(stringMetadataValue(value)) {
			return true
		}
	}
	return false
}

func isConsoleFilesPageToken(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "console.files", "console_files", "/console/files":
		return true
	default:
		return false
	}
}

func hasConsoleFilesReadCapability(runtimeContext string, contexts ...map[string]interface{}) bool {
	return hasConsoleFilesCapability(runtimeContext, consoleFilesCapabilityPattern, contexts...)
}

func hasConsoleFilesAssetCapability(runtimeContext string, contexts ...map[string]interface{}) bool {
	if hasConsoleFilesCapability(runtimeContext, consoleFilesCapabilityPattern, contexts...) {
		return true
	}
	return hasConsoleFilesCapability(runtimeContext, consoleFilesDeleteCapabilityPattern, contexts...)
}

func hasConsoleFilesCreateCapability(runtimeContext string, contexts ...map[string]interface{}) bool {
	return hasConsoleFilesCapability(runtimeContext, consoleFilesCreateCapabilityPattern, contexts...)
}

func hasConsoleFilesCapability(runtimeContext string, pattern *regexp.Regexp, contexts ...map[string]interface{}) bool {
	for _, ctx := range contexts {
		if contextContainsStructuredConsoleFilesCapability(ctx, pattern) {
			return true
		}
	}
	return false
}

func contextContainsStructuredConsoleFilesCapability(context map[string]interface{}, pattern *regexp.Regexp) bool {
	if len(context) == 0 || pattern == nil {
		return false
	}
	for _, item := range operationItemsFromValue(context["capabilities"]) {
		if structuredCapabilityItemMatches(item, pattern) {
			return true
		}
	}
	for _, item := range operationItemsFromValue(context["resources"]) {
		resource, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if structuredCapabilityItemMatches(firstMapValue(resource, "capability_id", "tool_id"), pattern) {
			return true
		}
		for _, key := range []string{"capability_ids", "tool_ids", "capabilities", "tools"} {
			for _, capability := range operationItemsFromValue(resource[key]) {
				if structuredCapabilityItemMatches(capability, pattern) {
					return true
				}
			}
		}
	}
	for _, key := range []string{"capability_id", "capability_ids", "tool_id", "tool_ids"} {
		for _, item := range operationItemsFromValue(context[key]) {
			if structuredCapabilityItemMatches(item, pattern) {
				return true
			}
		}
	}
	return false
}

func structuredCapabilityItemMatches(value interface{}, pattern *regexp.Regexp) bool {
	if pattern == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return containsConsoleFilesCapabilityToken(typed, pattern)
	case map[string]interface{}:
		return containsConsoleFilesCapabilityToken(
			firstNonEmptyString(typed["id"], typed["capability_id"], typed["tool_id"]),
			pattern,
		)
	default:
		return false
	}
}

func containsConsoleFilesCapabilityToken(value string, pattern *regexp.Regexp) bool {
	return pattern != nil && pattern.MatchString(strings.TrimSpace(value))
}

func isFileReadIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if consoleFilesReadIntentPattern.MatchString(text) {
		return true
	}
	for _, token := range []string{
		"\u8bfb\u53d6",
		"\u8bfb\u4e00\u4e0b",
		"\u8bfb\u4e0b",
		"\u603b\u7ed3",
		"\u6458\u8981",
		"\u7ffb\u8bd1",
		"\u6982\u62ec",
		"\u63d0\u70bc",
		"\u63d0\u53d6",
		"\u89e3\u91ca",
		"\u5206\u6790",
		"\u67e5\u770b\u5185\u5bb9",
		"\u770b\u770b\u5185\u5bb9",
		"\u770b\u4e00\u4e0b\u5185\u5bb9",
		"\u6587\u4ef6\u5185\u5bb9",
		"\u9884\u89c8",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	if strings.Contains(text, "\u8bfb") && containsAnySubstring(text, []string{
		"\u7b2c",
		"\u6700\u540e",
		"\u5f53\u524d",
		"\u9009\u4e2d",
		"\u8fd9\u4e2a",
		"\u5185\u5bb9",
		"pdf",
		"excel",
		"\u8868\u683c",
		"\u6587\u6863",
	}) {
		return true
	}
	return false
}

func isFileDeleteIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if consoleFilesDeleteIntentPattern.MatchString(text) {
		return true
	}
	for _, token := range []string{
		"\u5220\u9664",
		"\u5220\u6389",
		"\u5220\u4e86",
		"\u79fb\u9664",
		"\u6e05\u7406",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func isManagedFileCreateIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if hasManagedFileCreateNegation(text) {
		return false
	}
	createTerms := []string{
		"create", "generate", "save", "upload", "export", "write",
		"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u4fdd\u5b58", "\u4e0a\u4f20", "\u5bfc\u51fa", "\u5199\u5165",
	}
	targetTerms := []string{
		"file management", "files page", "current files page", "managed file", "workspace file",
		"\u6587\u4ef6\u7ba1\u7406", "\u6587\u4ef6\u9875", "\u5f53\u524d\u6587\u4ef6\u9875", "\u6587\u4ef6\u5217\u8868", "\u5de5\u4f5c\u533a\u6587\u4ef6",
	}
	if !containsAnySubstring(text, createTerms) {
		return false
	}
	if containsAnySubstring(text, targetTerms) {
		return true
	}
	fileTerms := []string{"file", "\u6587\u4ef6"}
	managementTerms := []string{"management", "manage", "\u7ba1\u7406", "\u7ba1\u7406\u9875", "\u7ba1\u7406\u91cc", "\u7ba1\u7406\u91cc\u9762"}
	return containsAnySubstring(text, fileTerms) && containsAnySubstring(text, managementTerms)
}

func hasManagedFileCreateNegation(text string) bool {
	negativePhrases := []string{
		"do not save", "don't save", "dont save", "not save", "without saving", "temporary only",
		"do not add", "don't add", "dont add", "do not upload", "don't upload", "dont upload",
		"\u4e0d\u8981\u4fdd\u5b58", "\u4e0d\u7528\u4fdd\u5b58", "\u4e0d\u4fdd\u5b58", "\u65e0\u9700\u4fdd\u5b58", "\u522b\u4fdd\u5b58",
		"\u4e0d\u8981\u5b58", "\u4e0d\u7528\u5b58", "\u522b\u5b58",
		"\u4e0d\u8981\u6dfb\u52a0", "\u4e0d\u7528\u6dfb\u52a0", "\u522b\u6dfb\u52a0",
		"\u4e0d\u8981\u4e0a\u4f20", "\u4e0d\u7528\u4e0a\u4f20", "\u522b\u4e0a\u4f20",
		"\u4ec5\u4e34\u65f6", "\u53ea\u751f\u6210\u4e34\u65f6", "\u4e34\u65f6\u5373\u53ef",
	}
	return containsAnySubstring(text, negativePhrases)
}

func collectConsoleFilesFileIDs(parts *chatRequestParts) []string {
	collector := newUniqueStringCollector()
	if parts == nil {
		return nil
	}
	if parts.Attachments != nil {
		for _, file := range parts.Attachments.Files {
			collector.add(file.ID)
		}
	}

	selected := newUniqueStringCollector()
	collectSelectedFileIDs(parts.RawOperationContext, 0, selected.add)
	collectSelectedFileIDs(parts.OperationContext, 0, selected.add)
	for _, id := range selected.values() {
		collector.add(id)
	}
	if len(collector.values()) > 0 {
		return collector.values()
	}

	for _, id := range resolveConsoleFileIDsFromQuery(parts) {
		collector.add(id)
	}
	if len(collector.values()) > 0 {
		return collector.values()
	}

	for _, id := range collectNamedVisibleFileIDs(parts.Query, parts.RawOperationContext) {
		collector.add(id)
	}
	if len(collector.values()) > 0 {
		return collector.values()
	}

	visible := visibleFileResources(parts.RawOperationContext)
	if len(visible) == 1 {
		collector.add(visible[0].ID)
	}
	return collector.values()
}

type uniqueStringCollector struct {
	seen map[string]struct{}
	out  []string
}

func newUniqueStringCollector() *uniqueStringCollector {
	return &uniqueStringCollector{seen: map[string]struct{}{}, out: []string{}}
}

func (c *uniqueStringCollector) add(raw string) {
	if c == nil {
		return
	}
	id := strings.TrimSpace(raw)
	if id == "" {
		return
	}
	if _, ok := c.seen[id]; ok {
		return
	}
	c.seen[id] = struct{}{}
	c.out = append(c.out, id)
}

func (c *uniqueStringCollector) values() []string {
	if c == nil || len(c.out) == 0 {
		return nil
	}
	return append([]string(nil), c.out...)
}

func collectSelectedFileIDs(value interface{}, depth int, add func(string)) {
	if depth > 5 || value == nil || add == nil {
		return
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		if isFileResourceMap(typed) && boolMetadataValue(firstMapValue(typed, "selected", "is_selected")) {
			add(fileIDFromResourceMap(typed))
		}
		if metadata := mapFromOperationContext(typed["metadata"]); metadata != nil {
			if boolMetadataValue(firstMapValue(metadata, "selected", "is_selected")) {
				add(fileIDFromResourceMap(typed))
				add(stringMetadataValue(firstMapValue(metadata, "file_id", "upload_file_id")))
			}
			for _, id := range stringMetadataSlice(firstMapValue(metadata, "selected_file_ids", "file_ids")) {
				add(id)
			}
		}
		for key, item := range typed {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "selected_file_id", "selected_upload_file_id":
				add(stringMetadataValue(item))
			case "selected_file_ids", "selected_upload_file_ids":
				for _, id := range stringMetadataSlice(item) {
					add(id)
				}
			default:
				collectSelectedFileIDs(item, depth+1, add)
			}
		}
	case []interface{}:
		for _, item := range typed {
			collectSelectedFileIDs(item, depth+1, add)
		}
	case []map[string]interface{}:
		for _, item := range typed {
			collectSelectedFileIDs(item, depth+1, add)
		}
	}
}

type visibleConsoleFileResource struct {
	ID            string
	Title         string
	Extension     string
	MimeType      string
	FileType      string
	WorkspaceID   string
	VisibleIndex  int
	FileTypeRank  int
	ExtensionRank int
	Selected      bool
}

func visibleFileResources(context map[string]interface{}) []visibleConsoleFileResource {
	if len(context) == 0 {
		return nil
	}
	items := operationItemsFromValue(context["resources"])
	out := make([]visibleConsoleFileResource, 0, len(items))
	fileTypeRanks := map[string]int{}
	extensionRanks := map[string]int{}
	for _, item := range items {
		resource, ok := item.(map[string]interface{})
		if !ok || !isFileResourceMap(resource) {
			continue
		}
		id := fileIDFromResourceMap(resource)
		if id == "" {
			continue
		}
		title := firstNonEmptyString(
			stringMetadataValue(resource["title"]),
			stringMetadataValue(resource["name"]),
			stringMetadataValue(firstMapValue(mapFromOperationContext(resource["metadata"]), "name", "filename")),
		)
		metadata := mapFromOperationContext(resource["metadata"])
		extension := firstNonEmptyString(
			normalizedConsoleFileExtension(stringMetadataValue(resource["extension"])),
			normalizedConsoleFileExtension(stringMetadataValue(firstMapValue(metadata, "extension", "file_extension"))),
			knownConsoleFileExtension(fileNameExtension(title)),
			consoleFileSubtitleExtension(stringMetadataValue(resource["subtitle"])),
		)
		mimeType := firstNonEmptyString(
			stringMetadataValue(resource["mime_type"]),
			stringMetadataValue(resource["mimeType"]),
			stringMetadataValue(firstMapValue(metadata, "mime_type", "mimeType")),
		)
		fileType := firstNonEmptyString(
			stringMetadataValue(resource["file_type"]),
			stringMetadataValue(firstMapValue(metadata, "file_type", "file_type_normalized", "format", "category")),
		)
		workspaceID := firstNonEmptyString(
			stringMetadataValue(resource["workspace_id"]),
			stringMetadataValue(firstMapValue(metadata, "workspace_id", "workspaceId", "team_tenant_id")),
		)
		selected := boolMetadataValue(firstMapValue(resource, "selected", "is_selected")) ||
			boolMetadataValue(firstMapValue(metadata, "selected", "is_selected"))
		visibleIndex := firstPositiveInt(
			intValueFromAny(firstMapValue(resource, "visible_index", "visible_ordinal", "visible_rank")),
			intValueFromAny(firstMapValue(metadata, "visible_index", "visible_ordinal", "visible_rank")),
			len(out)+1,
		)
		rankFileType := firstNonEmptyString(strings.TrimSpace(fileType), extension, "file")
		fileTypeRanks[rankFileType]++
		fileTypeRank := firstPositiveInt(
			intValueFromAny(firstMapValue(resource, "file_type_rank")),
			intValueFromAny(firstMapValue(metadata, "file_type_rank")),
			fileTypeRanks[rankFileType],
		)
		rankExtension := firstNonEmptyString(extension, strings.TrimSpace(fileType), "file")
		extensionRanks[rankExtension]++
		extensionRank := firstPositiveInt(
			intValueFromAny(firstMapValue(resource, "extension_rank")),
			intValueFromAny(firstMapValue(metadata, "extension_rank")),
			extensionRanks[rankExtension],
		)
		out = append(out, visibleConsoleFileResource{
			ID:            id,
			Title:         title,
			Extension:     extension,
			MimeType:      strings.TrimSpace(mimeType),
			FileType:      strings.TrimSpace(fileType),
			WorkspaceID:   strings.TrimSpace(workspaceID),
			VisibleIndex:  visibleIndex,
			FileTypeRank:  fileTypeRank,
			ExtensionRank: extensionRank,
			Selected:      selected,
		})
	}
	return out
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func normalizedConsoleFileExtension(value string) string {
	extension := normalizedResolverFileExtension(value)
	if extension == "" {
		return ""
	}
	for _, r := range extension {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return ""
		}
	}
	if len([]rune(extension)) > 16 {
		return ""
	}
	return extension
}

func consoleFileSubtitleExtension(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		extension := normalizedConsoleFileExtension(token)
		if isKnownConsoleFileExtension(extension) {
			return extension
		}
	}
	return ""
}

func knownConsoleFileExtension(value string) string {
	extension := normalizedConsoleFileExtension(value)
	if !isKnownConsoleFileExtension(extension) {
		return ""
	}
	return extension
}

func isKnownConsoleFileExtension(extension string) bool {
	switch extension {
	case "doc", "docx", "html", "json", "md", "ppt", "pptx", "rtf", "txt", "zip":
		return true
	}
	for _, fileType := range []string{"csv", "document", "excel", "image", "pdf"} {
		if containsString(fileTypeExtensions(fileType), extension) {
			return true
		}
	}
	return false
}

func containsAnySubstring(text string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func collectNamedVisibleFileIDs(query string, context map[string]interface{}) []string {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return nil
	}
	collector := newUniqueStringCollector()
	for _, file := range visibleFileResources(context) {
		name := strings.ToLower(strings.TrimSpace(file.Title))
		if len([]rune(name)) < 3 {
			continue
		}
		if strings.Contains(text, name) {
			collector.add(file.ID)
		}
	}
	return collector.values()
}

func isFileResourceMap(value map[string]interface{}) bool {
	for _, key := range []string{"type", "resource_type", "kind"} {
		if strings.EqualFold(strings.TrimSpace(stringMetadataValue(value[key])), "file") {
			return true
		}
	}
	return false
}

func fileIDFromResourceMap(value map[string]interface{}) string {
	if len(value) == 0 {
		return ""
	}
	if id := stringMetadataValue(firstMapValue(value, "resource_id", "id", "file_id", "upload_file_id")); id != "" {
		return id
	}
	metadata := mapFromOperationContext(value["metadata"])
	return stringMetadataValue(firstMapValue(metadata, "file_id", "upload_file_id"))
}

func firstMapValue(value map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value == nil {
			return nil
		}
		if item, ok := value[key]; ok {
			return item
		}
	}
	return nil
}

func stringMetadataValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func stringMetadataSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringMetadataValue(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		text := stringMetadataValue(value)
		if text == "" {
			return nil
		}
		parts := strings.Split(text, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if id := strings.TrimSpace(part); id != "" {
				out = append(out, id)
			}
		}
		return out
	}
}

func boolMetadataValue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}
