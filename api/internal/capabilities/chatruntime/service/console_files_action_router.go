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
	consoleAgentsCapabilityPattern      = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])(agent\.(list_visible|open_visible|open|inspect_summary|inspect)|inspect_agent_runtime)([^a-z0-9_.-]|$)`)
	consoleAgentsManagePattern          = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])(agent\.(create|create_from_page|update|update_identity|update_config|delete|delete_visible)|update_agent_runtime_config)([^a-z0-9_.-]|$)`)
)

func isConsoleFilesContext(runtimeContext string, contexts ...map[string]interface{}) bool {
	if consoleNavigationLoadedHrefMatchesTarget(consoleRouteFromRuntimeContext(runtimeContext), "/console/files") {
		return true
	}
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

func isConsoleAgentsContext(runtimeContext string, contexts ...map[string]interface{}) bool {
	if consoleNavigationLoadedHrefMatchesTarget(consoleRouteFromRuntimeContext(runtimeContext), "/console/agents") {
		return true
	}
	for _, ctx := range contexts {
		if contextContainsConsoleAgentsPageResource(ctx) || contextContainsConsoleAgentResource(ctx) {
			return true
		}
	}
	return false
}

func contextContainsConsoleAgentsPageResource(context map[string]interface{}) bool {
	if len(context) == 0 {
		return false
	}
	for _, item := range operationItemsFromValue(context["resources"]) {
		resource, ok := item.(map[string]interface{})
		if !ok || !isConsoleAgentsPageResource(resource) {
			continue
		}
		return true
	}
	return false
}

func isConsoleAgentsPageResource(resource map[string]interface{}) bool {
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
		if isConsoleAgentsPageToken(stringMetadataValue(value)) {
			return true
		}
	}
	return false
}

func isConsoleAgentsPageToken(value string) bool {
	token := strings.ToLower(strings.TrimSpace(value))
	return token == "console.agents" ||
		token == "console_agents" ||
		token == "/console/agents" ||
		strings.HasPrefix(token, "/console/agents/")
}

func contextContainsConsoleAgentResource(context map[string]interface{}) bool {
	if len(context) == 0 {
		return false
	}
	for _, item := range operationItemsFromValue(context["resources"]) {
		resource, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if isConsoleAgentResource(resource) {
			return true
		}
	}
	return false
}

func isConsoleAgentResource(resource map[string]interface{}) bool {
	if len(resource) == 0 {
		return false
	}
	metadata := mapFromOperationContext(resource["metadata"])
	for _, value := range []interface{}{
		resource["resource_type"],
		resource["type"],
		resource["kind"],
		metadata["resource_kind"],
		metadata["asset_type"],
	} {
		token := strings.ToLower(strings.TrimSpace(stringMetadataValue(value)))
		if token == "agent" || token == "agents" {
			return true
		}
	}
	if firstNonEmptyString(resource["agent_id"], metadata["agent_id"]) != "" {
		return true
	}
	return false
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

func hasConsoleAgentsReadCapability(runtimeContext string, contexts ...map[string]interface{}) bool {
	return hasConsoleFilesCapability(runtimeContext, consoleAgentsCapabilityPattern, contexts...) ||
		hasConsoleFilesCapability(runtimeContext, consoleAgentsManagePattern, contexts...)
}

func hasConsoleAgentsManageCapability(runtimeContext string, contexts ...map[string]interface{}) bool {
	return hasConsoleFilesCapability(runtimeContext, consoleAgentsManagePattern, contexts...)
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
	if isConsoleFilesPageSummaryQuestion(text) {
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

func isConsoleFilesPageSummaryQuestion(text string) bool {
	if text == "" {
		return false
	}
	if !containsAnySubstring(text, []string{
		"how many",
		"count",
		"total",
		"table total",
		"file count",
		"files count",
		"items",
		"file list",
		"list files",
		"\u5171\u591a\u5c11",
		"\u5171\u51e0",
		"\u6709\u51e0",
		"\u591a\u5c11\u4e2a",
		"\u603b\u6570",
		"\u603b\u5171",
		"\u6587\u4ef6\u6570",
		"\u6587\u4ef6\u5217\u8868",
		"\u54ea\u4e9b\u6587\u4ef6",
		"\u6709\u54ea\u4e9b\u6587\u4ef6",
	}) {
		return false
	}
	return !containsAnySubstring(text, []string{
		"content",
		"contents",
		"inside",
		"body",
		"read file",
		"file content",
		"\u5185\u5bb9",
		"\u6587\u4ef6\u5185\u5bb9",
		"\u8bfb\u53d6",
		"\u7ffb\u8bd1",
		"\u6458\u8981",
		"\u603b\u7ed3",
		"\u5206\u6790",
	})
}

func isFileDeleteIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if hasFileDeleteNegation(text) {
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

func isTemporaryFileGenerateIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" ||
		isManagedFileCreateIntent(query) ||
		isFileReadIntent(query) ||
		isFileDeleteIntent(query) {
		return false
	}
	if hasTemporaryFileGenerateNegation(text) {
		return false
	}
	createTerms := []string{
		"create", "generate", "write", "export", "make", "produce",
		"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u5199", "\u5199\u4e00\u4e2a", "\u5bfc\u51fa", "\u505a\u4e00\u4e2a",
	}
	artifactTerms := []string{
		"file", ".txt", ".md", ".markdown", ".json", ".csv", ".tsv", ".xlsx", ".docx", ".pptx", ".pdf", ".html", ".svg",
		"txt", "markdown", "json", "csv", "tsv", "xlsx", "docx", "pptx", "pdf", "html", "svg",
		" txt", " md", " json", " csv", " xlsx", " docx", " pptx", " pdf", " html", " svg",
		"\u6587\u4ef6", "\u4e34\u65f6\u6587\u4ef6", "\u6587\u6863", "\u8868\u683c", "\u56fe\u7247",
	}
	return containsAnySubstring(text, createTerms) && containsAnySubstring(text, artifactTerms)
}

func hasTemporaryFileGenerateNegation(text string) bool {
	negativePhrases := []string{
		"do not create", "don't create", "dont create", "not create", "without creating",
		"do not generate", "don't generate", "dont generate", "not generate", "without generating",
		"do not write", "don't write", "dont write", "not write", "without writing",
		"do not export", "don't export", "dont export", "not export", "without exporting",
		"do not make", "don't make", "dont make", "not make", "without making",
		"do not produce", "don't produce", "dont produce", "not produce", "without producing",
		"read only", "answer only",
		"\u4e0d\u8981\u521b\u5efa", "\u4e0d\u7528\u521b\u5efa", "\u4e0d\u521b\u5efa", "\u65e0\u9700\u521b\u5efa", "\u522b\u521b\u5efa",
		"\u4e0d\u8981\u65b0\u5efa", "\u4e0d\u7528\u65b0\u5efa", "\u4e0d\u65b0\u5efa", "\u65e0\u9700\u65b0\u5efa", "\u522b\u65b0\u5efa",
		"\u4e0d\u8981\u751f\u6210", "\u4e0d\u7528\u751f\u6210", "\u4e0d\u751f\u6210", "\u65e0\u9700\u751f\u6210", "\u522b\u751f\u6210",
		"\u4e0d\u8981\u5199", "\u4e0d\u7528\u5199", "\u4e0d\u5199", "\u65e0\u9700\u5199", "\u522b\u5199",
		"\u4e0d\u8981\u5bfc\u51fa", "\u4e0d\u7528\u5bfc\u51fa", "\u4e0d\u5bfc\u51fa", "\u65e0\u9700\u5bfc\u51fa", "\u522b\u5bfc\u51fa",
		"\u4ec5\u56de\u7b54", "\u53ea\u56de\u7b54", "\u4ec5\u8bfb", "\u53ea\u8bfb",
	}
	return containsAnySubstring(text, negativePhrases)
}

func hasManagedFileCreateNegation(text string) bool {
	negativePhrases := []string{
		"do not create", "don't create", "dont create", "not create", "without creating",
		"do not generate", "don't generate", "dont generate", "not generate", "without generating",
		"do not write", "don't write", "dont write", "not write", "without writing",
		"do not export", "don't export", "dont export", "not export", "without exporting",
		"do not save", "don't save", "dont save", "not save", "without saving", "temporary only",
		"do not add", "don't add", "dont add", "do not upload", "don't upload", "dont upload",
		"read only", "answer only",
		"\u4e0d\u8981\u521b\u5efa", "\u4e0d\u7528\u521b\u5efa", "\u4e0d\u521b\u5efa", "\u65e0\u9700\u521b\u5efa", "\u522b\u521b\u5efa",
		"\u4e0d\u8981\u65b0\u5efa", "\u4e0d\u7528\u65b0\u5efa", "\u4e0d\u65b0\u5efa", "\u65e0\u9700\u65b0\u5efa", "\u522b\u65b0\u5efa",
		"\u4e0d\u8981\u751f\u6210", "\u4e0d\u7528\u751f\u6210", "\u4e0d\u751f\u6210", "\u65e0\u9700\u751f\u6210", "\u522b\u751f\u6210",
		"\u4e0d\u8981\u5199\u5165", "\u4e0d\u7528\u5199\u5165", "\u4e0d\u5199\u5165", "\u65e0\u9700\u5199\u5165", "\u522b\u5199\u5165",
		"\u4e0d\u8981\u5bfc\u51fa", "\u4e0d\u7528\u5bfc\u51fa", "\u4e0d\u5bfc\u51fa", "\u65e0\u9700\u5bfc\u51fa", "\u522b\u5bfc\u51fa",
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

type visibleConsoleAgentResource struct {
	ID           string
	Title        string
	Description  string
	AgentType    string
	WorkspaceID  string
	Href         string
	VisibleIndex int
	Selected     bool
	CanEdit      bool
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

func visibleAgentResources(context map[string]interface{}) []visibleConsoleAgentResource {
	if len(context) == 0 {
		return nil
	}
	items := operationItemsFromValue(context["resources"])
	out := make([]visibleConsoleAgentResource, 0, len(items))
	for _, item := range items {
		resource, ok := item.(map[string]interface{})
		if !ok || !isConsoleAgentResource(resource) {
			continue
		}
		metadata := mapFromOperationContext(resource["metadata"])
		id := firstNonEmptyString(
			stringMetadataValue(resource["agent_id"]),
			stringMetadataValue(resource["id"]),
			stringMetadataValue(resource["resource_id"]),
			stringMetadataValue(metadata["agent_id"]),
		)
		if id == "" {
			continue
		}
		title := firstNonEmptyString(
			stringMetadataValue(resource["title"]),
			stringMetadataValue(resource["name"]),
			stringMetadataValue(firstMapValue(metadata, "name", "agent_name", "title")),
		)
		description := firstNonEmptyString(
			stringMetadataValue(resource["description"]),
			stringMetadataValue(resource["subtitle"]),
			stringMetadataValue(firstMapValue(metadata, "description", "summary")),
		)
		agentType := firstNonEmptyString(
			stringMetadataValue(resource["agent_type"]),
			stringMetadataValue(firstMapValue(metadata, "agent_type", "type")),
		)
		workspaceID := firstNonEmptyString(
			stringMetadataValue(resource["workspace_id"]),
			stringMetadataValue(firstMapValue(metadata, "workspace_id", "workspaceId", "tenant_id")),
		)
		href := firstNonEmptyString(
			stringMetadataValue(resource["href"]),
			stringMetadataValue(metadata["href"]),
			"/console/agents/"+id+"/agent",
		)
		selected := boolMetadataValue(firstMapValue(resource, "selected", "is_selected")) ||
			boolMetadataValue(firstMapValue(metadata, "selected", "is_selected"))
		canEdit := boolMetadataValue(firstMapValue(resource, "can_edit", "canEdit")) ||
			boolMetadataValue(firstMapValue(metadata, "can_edit", "canEdit"))
		visibleIndex := firstPositiveInt(
			intValueFromAny(firstMapValue(resource, "visible_index", "visible_ordinal", "visible_rank")),
			intValueFromAny(firstMapValue(metadata, "visible_index", "visible_ordinal", "visible_rank")),
			len(out)+1,
		)
		out = append(out, visibleConsoleAgentResource{
			ID:           strings.TrimSpace(id),
			Title:        strings.TrimSpace(title),
			Description:  strings.TrimSpace(description),
			AgentType:    strings.TrimSpace(agentType),
			WorkspaceID:  strings.TrimSpace(workspaceID),
			Href:         strings.TrimSpace(href),
			VisibleIndex: visibleIndex,
			Selected:     selected,
			CanEdit:      canEdit,
		})
	}
	return out
}

func isAgentManagementIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if consoleAgentsManagePattern.MatchString(text) || containsAgentManagementToolMention(text) {
		return true
	}
	agentTerms := []string{"agent", "\u667a\u80fd\u4f53"}
	if !containsAnySubstring(text, agentTerms) {
		return false
	}
	operationTerms := []string{
		"create", "new", "add", "edit", "update", "rename", "delete", "remove", "config", "configure", "prompt", "model", "icon", "description",
		"bind", "unbind", "enable", "disable", "detach", "clear",
		"\u521b\u5efa", "\u65b0\u5efa", "\u6dfb\u52a0", "\u7f16\u8f91", "\u4fee\u6539", "\u66f4\u65b0", "\u6539\u540d", "\u5220\u9664", "\u5220\u6389",
		"\u914d\u7f6e", "\u63d0\u793a\u8bcd", "\u6a21\u578b", "\u56fe\u6807", "\u63cf\u8ff0",
		"\u7ed1\u5b9a", "\u89e3\u7ed1", "\u542f\u7528", "\u7981\u7528", "\u505c\u7528", "\u79fb\u9664", "\u6e05\u7a7a",
	}
	return agentManagementOperationNearAgent(text, agentTerms, operationTerms)
}

func containsAgentManagementToolMention(text string) bool {
	for _, marker := range []string{
		"list_agents",
		"get_agent",
		"create_agent",
		"update_agent_identity",
		"delete_agent",
		"delete_agents",
		"get_agent_config",
		"update_agent_config",
		"replace_agent_memory_slots",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"replace_agent_skill_bindings",
		"replace_agent_knowledge_bindings",
		"replace_agent_database_bindings",
		"replace_agent_workflow_bindings",
		"list_available_models",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func agentManagementOperationNearAgent(text string, agentTerms []string, operationTerms []string) bool {
	const maxAgentOperationDistance = 48
	for _, agentTerm := range agentTerms {
		for _, agentPos := range allStringIndexes(text, agentTerm) {
			agentEnd := agentPos + len(agentTerm)
			for _, operationTerm := range operationTerms {
				for _, operationPos := range allStringIndexes(text, operationTerm) {
					distance := agentPos - operationPos
					if distance < 0 {
						distance = -distance
					}
					if distance <= maxAgentOperationDistance {
						if operationPos > agentPos && agentReferenceAttributeCrossesClauseBoundary(text, agentEnd, operationPos) {
							continue
						}
						return true
					}
				}
			}
		}
	}
	return false
}

func agentReferenceAttributeCrossesClauseBoundary(text string, agentEnd int, operationPos int) bool {
	if text == "" || agentEnd < 0 || operationPos <= agentEnd || agentEnd > len(text) || operationPos > len(text) {
		return false
	}
	suffix := text[agentEnd:]
	if !agentReferenceHasAttributeSuffix(suffix) {
		return false
	}
	between := text[agentEnd:operationPos]
	return containsAnySubstring(between, []string{";", ",", ".", "\uff1b", "\uff0c", "\u3002", "\u3001"})
}

func agentReferenceHasAttributeSuffix(suffix string) bool {
	for _, marker := range []string{
		"\u540d\u79f0",
		"\u540d\u5b57",
		"id",
		"\u7f16\u53f7",
		"\u914d\u7f6e",
		"\u5185\u5bb9",
		"\u63cf\u8ff0",
		"\u7ed3\u679c",
		"\u8f93\u51fa",
		"\u7ed1\u5b9a",
		"\u6570\u91cf",
		"\u72b6\u6001",
	} {
		if strings.HasPrefix(suffix, marker) {
			return true
		}
	}
	return false
}

func hasFileMutationNegation(text string) bool {
	if text == "" || !containsAnySubstring(text, []string{"file", "\u6587\u4ef6", "asset", "resource", "\u8d44\u4ea7", "\u8d44\u6e90"}) {
		return false
	}
	englishNegative := []string{
		"do not create", "don't create", "dont create", "not create",
		"do not save", "don't save", "dont save", "not save",
		"do not delete", "don't delete", "dont delete", "not delete",
		"do not remove", "don't remove", "dont remove", "not remove",
	}
	if containsAnySubstring(text, englishNegative) {
		return true
	}
	if containsAnySubstring(text, []string{
		"\u522b\u521b\u5efa", "\u522b\u65b0\u5efa", "\u522b\u751f\u6210", "\u522b\u4fdd\u5b58", "\u522b\u5b58", "\u522b\u4e0a\u4f20", "\u522b\u6dfb\u52a0",
		"\u522b\u5220\u9664", "\u522b\u5220\u6389", "\u522b\u79fb\u9664", "\u522b\u6e05\u7406",
	}) {
		return true
	}
	return containsAnySubstring(text, []string{"\u4e0d\u8981", "\u4e0d\u7528", "\u65e0\u9700"}) &&
		containsAnySubstring(text, []string{
			"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u4fdd\u5b58", "\u5b58", "\u4e0a\u4f20", "\u6dfb\u52a0",
			"\u5220\u9664", "\u5220\u6389", "\u5220\u4e86", "\u79fb\u9664", "\u6e05\u7406",
		})
}

func hasFileDeleteNegation(text string) bool {
	if text == "" || !containsAnySubstring(text, []string{"file", "\u6587\u4ef6", "asset", "resource", "\u8d44\u4ea7", "\u8d44\u6e90"}) {
		return false
	}
	if containsAnySubstring(text, []string{
		"do not delete", "don't delete", "dont delete", "not delete",
		"do not remove", "don't remove", "dont remove", "not remove",
		"do not create or delete", "don't create or delete", "dont create or delete",
		"\u522b\u5220\u9664", "\u522b\u5220\u6389", "\u522b\u79fb\u9664", "\u522b\u6e05\u7406",
		"\u4e0d\u8981\u5220\u9664", "\u4e0d\u8981\u5220\u6389", "\u4e0d\u8981\u79fb\u9664", "\u4e0d\u8981\u6e05\u7406",
		"\u4e0d\u7528\u5220\u9664", "\u4e0d\u7528\u5220\u6389", "\u4e0d\u7528\u79fb\u9664", "\u4e0d\u7528\u6e05\u7406",
		"\u65e0\u9700\u5220\u9664", "\u65e0\u9700\u5220\u6389", "\u65e0\u9700\u79fb\u9664", "\u65e0\u9700\u6e05\u7406",
		"\u4e0d\u8981\u521b\u5efa\u6216\u5220\u9664", "\u4e0d\u8981\u521b\u5efa\u548c\u5220\u9664", "\u4e0d\u8981\u521b\u5efa\u3001\u5220\u9664",
		"\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664", "\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u6216\u5220\u9664",
		"\u4e0d\u7528\u521b\u5efa\u6216\u5220\u9664", "\u65e0\u9700\u521b\u5efa\u6216\u5220\u9664",
	}) {
		return true
	}
	return hasNegatedFileDeleteClause(text)
}

func hasNegatedFileDeleteClause(text string) bool {
	for _, clause := range strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '.', ',', ';', ':', '\uff0c', '\u3002', '\uff1b', '\uff1a':
			return true
		default:
			return false
		}
	}) {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		if !containsAnySubstring(clause, []string{"file", "\u6587\u4ef6", "asset", "resource", "\u8d44\u4ea7", "\u8d44\u6e90"}) {
			continue
		}
		negativeAt := firstSubstringIndex(clause, []string{"do not", "don't", "dont", "not ", "without", "never", "\u4e0d\u8981", "\u4e0d\u7528", "\u65e0\u9700", "\u522b"})
		if negativeAt < 0 {
			continue
		}
		if containsAnySubstring(clause[negativeAt:], []string{"delete", "remove", "trash", "discard", "\u5220\u9664", "\u5220\u6389", "\u5220\u4e86", "\u79fb\u9664", "\u6e05\u7406"}) {
			return true
		}
	}
	return false
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

func firstSubstringIndex(text string, needles []string) int {
	first := -1
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		idx := strings.Index(text, needle)
		if idx >= 0 && (first < 0 || idx < first) {
			first = idx
		}
	}
	return first
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
