package service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
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
	route := consoleRouteFromRuntimeContext(runtimeContext)
	if consoleNavigationLoadedHrefMatchesTarget(route, "/console/agents") ||
		isConsoleAgentDetailRoute(route) {
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
			consoleAgentDetailHref(id),
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
