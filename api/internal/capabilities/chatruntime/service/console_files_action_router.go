package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	actionservice "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	consoleFilesActionCapabilityID  = "file.read"
	consoleFilesActionIntent        = "console.files.file_read"
	consoleFilesNoFileAnswer        = "Please select a file on the Files page, or mention the exact visible file name, then ask me to read it."
	consoleFilesSkillRequiredAnswer = "Reading, summarizing, or translating file content from the Files page requires the file-reader skill. Enable file-reader for AIChat, then try again."
	consoleFilesReadMaxChars        = 4000
)

var (
	consoleFilesReadIntentPattern       = regexp.MustCompile(`(?i)\b(read|preview|summari[sz]e|summary|analy[sz]e|analysis|inspect|show|translate|translation|abstract|digest|extract)\b`)
	consoleFilesDeleteIntentPattern     = regexp.MustCompile(`(?i)\b(delete|remove|trash|discard)\b`)
	consoleFilesCapabilityPattern       = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])file\.read([^a-z0-9_.-]|$)`)
	consoleFilesDeleteCapabilityPattern = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])file\.delete([^a-z0-9_.-]|$)`)
	consoleFilesCreateCapabilityPattern = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])file\.create([^a-z0-9_.-]|$)`)
)

type consoleFilesActionDecision struct {
	Matched bool
	FileIDs []string
}

func (s *service) runConsoleFilesActionIfMatched(ctx context.Context, prepared *PreparedChat, onChunk func(string) error) (*ChatResult, bool, error) {
	if s == nil || s.actionRuntime == nil || prepared == nil || prepared.Message == nil || prepared.Conversation == nil || prepared.parts == nil {
		return nil, false, nil
	}
	if !isConsoleFilesContext(prepared.parts.RuntimeContext, prepared.parts.RawOperationContext, prepared.parts.OperationContext) ||
		!hasConsoleFilesReadCapability(prepared.parts.RuntimeContext, prepared.parts.RawOperationContext, prepared.parts.OperationContext) {
		return nil, false, nil
	}
	if shouldRouteConsoleFilesReadThroughSkillRuntime(prepared) {
		return nil, false, nil
	}
	if shouldBlockConsoleFilesActionRuntimeFallback(prepared) {
		metadata := preparedResultMetadata(prepared.Message.Metadata, nil)
		metadata["console_files"] = map[string]interface{}{
			"blocked_action_runtime_fallback": true,
			"required_skill_id":               skills.SkillFileReader,
			"capability_id":                   consoleFilesActionCapabilityID,
		}
		if err := s.completePreparedChat(ctx, prepared, consoleFilesSkillRequiredAnswer, metadata); err != nil {
			return nil, true, err
		}
		s.appendConsoleFilesMessageEvent(ctx, prepared, consoleFilesSkillRequiredAnswer, onChunk)
		s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessageEnd, messageEndPayload(prepared, metadata))
		return &ChatResult{Answer: consoleFilesSkillRequiredAnswer, Metadata: metadata}, true, nil
	}
	decision := s.planAIChatActionDecision(ctx, prepared)
	if !decision.Matched || !strings.EqualFold(decision.CapabilityID, consoleFilesActionCapabilityID) {
		return nil, false, nil
	}
	fileIDs := resolveConsoleFileIDsFromActionDecision(prepared.parts, decision)
	if len(fileIDs) == 0 {
		metadata := preparedResultMetadata(prepared.Message.Metadata, nil)
		if err := s.completePreparedChat(ctx, prepared, consoleFilesNoFileAnswer, metadata); err != nil {
			return nil, true, err
		}
		s.appendConsoleFilesMessageEvent(ctx, prepared, consoleFilesNoFileAnswer, onChunk)
		s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessageEnd, messageEndPayload(prepared, metadata))
		return &ChatResult{Answer: consoleFilesNoFileAnswer, Metadata: metadata}, true, nil
	}

	actionScope := actionservice.Scope{
		OrganizationID:  prepared.Scope.OrganizationID,
		AccountID:       prepared.Scope.AccountID,
		WorkspaceID:     prepared.Scope.WorkspaceID,
		SkipAccessCheck: prepared.Scope.SkipAccessCheck,
	}
	plan, err := s.actionRuntime.PlanAction(ctx, actionScope, consoleFilesActionPlanRequest(prepared, fileIDs, decision))
	if err != nil {
		return nil, true, err
	}
	run := plan
	if plan != nil && plan.Run != nil {
		run, err = s.actionRuntime.ExecuteAction(ctx, actionScope, plan.Run.ID, actiondto.ExecuteActionRequest{
			Metadata: map[string]interface{}{
				"source": "aichat.console.files",
			},
		})
		if err != nil {
			return nil, true, err
		}
	}

	answer := consoleFilesAnswerFromRun(run)
	postprocessAnswer, postprocessUsage, postprocessErr := s.postprocessConsoleFilesActionAnswer(ctx, prepared, run, decision, answer)
	if strings.TrimSpace(postprocessAnswer) != "" {
		answer = postprocessAnswer
	}
	metadata := preparedResultMetadata(prepared.Message.Metadata, postprocessUsage)
	actionRuntimeMetadata := map[string]interface{}{
		"plan": actionRunResponseForMetadata(plan),
		"run":  actionRunResponseForMetadata(run),
	}
	if postprocessErr != nil {
		actionRuntimeMetadata["postprocess_error"] = postprocessErr.Error()
	}
	if len(decision.Postprocess) > 0 {
		actionRuntimeMetadata["postprocess"] = decision.Postprocess
	}
	metadata["action_runtime"] = actionRuntimeMetadata
	if err := s.completePreparedChat(ctx, prepared, answer, metadata); err != nil {
		return nil, true, err
	}
	s.appendConsoleFilesMessageEvent(ctx, prepared, answer, onChunk)
	s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessageEnd, messageEndPayload(prepared, metadata))
	return &ChatResult{Answer: answer, Metadata: metadata}, true, nil
}

func shouldRouteConsoleFilesReadThroughSkillRuntime(prepared *PreparedChat) bool {
	return prepared != nil &&
		prepared.skillsEnabled() &&
		skillIDEnabled(prepared.parts.SkillIDs, skills.SkillFileReader)
}

func shouldBlockConsoleFilesActionRuntimeFallback(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil {
		return false
	}
	if shouldRouteConsoleFilesReadThroughSkillRuntime(prepared) {
		return false
	}
	return isFileReadIntent(prepared.parts.Query)
}

func (s *service) appendConsoleFilesMessageEvent(ctx context.Context, prepared *PreparedChat, answer string, onChunk func(string) error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil || strings.TrimSpace(answer) == "" {
		return
	}
	s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessage, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer":          answer,
	})
	if onChunk != nil {
		_ = onChunk(answer)
	}
}

func consoleFilesActionDecisionForParts(parts *chatRequestParts) consoleFilesActionDecision {
	if parts == nil {
		return consoleFilesActionDecision{}
	}
	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return consoleFilesActionDecision{}
	}
	if !hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return consoleFilesActionDecision{}
	}
	if !isFileReadIntent(parts.Query) {
		return consoleFilesActionDecision{}
	}
	return consoleFilesActionDecision{
		Matched: true,
		FileIDs: collectConsoleFilesFileIDs(parts),
	}
}

func consoleFilesActionPlanRequest(prepared *PreparedChat, fileIDs []string, decision AIChatActionDecision) actiondto.ActionPlanRequest {
	resources := make([]actiondto.ResourceRef, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		resources = append(resources, actiondto.ResourceRef{Type: "file", ID: fileID, Source: "console.files"})
	}
	intent := strings.TrimSpace(decision.Intent)
	if intent == "" {
		intent = consoleFilesActionIntent
	}
	arguments := map[string]interface{}{"file_ids": fileIDs, "include_content": true, "max_chars": consoleFilesReadMaxChars}
	if len(decision.Postprocess) > 0 {
		arguments["postprocess"] = decision.Postprocess
	}
	metadata := map[string]interface{}{
		"source": "aichat.console.files",
		"planner": map[string]interface{}{
			"confidence":  decision.Confidence,
			"intent":      intent,
			"reason":      decision.Reason,
			"postprocess": decision.Postprocess,
		},
	}
	return actiondto.ActionPlanRequest{
		ConversationID:   prepared.Conversation.ID.String(),
		MessageID:        prepared.Message.ID.String(),
		Intent:           intent,
		CapabilityID:     consoleFilesActionCapabilityID,
		Title:            "Read selected file",
		Summary:          "Read selected console file content for this AIChat turn",
		Resources:        resources,
		Arguments:        arguments,
		OperationContext: copyStringAnyMap(prepared.parts.RawOperationContext),
		Metadata:         metadata,
	}
}

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
	createTerms := []string{
		"create", "generate", "save", "upload", "export", "write",
		"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u4fdd\u5b58", "\u4e0a\u4f20", "\u5bfc\u51fa", "\u5199\u5165",
	}
	targetTerms := []string{
		"file management", "files page", "current files page", "managed file", "workspace file",
		"\u6587\u4ef6\u7ba1\u7406", "\u6587\u4ef6\u9875", "\u5f53\u524d\u6587\u4ef6\u9875", "\u6587\u4ef6\u5217\u8868", "\u5de5\u4f5c\u533a\u6587\u4ef6",
	}
	return containsAnySubstring(text, createTerms) && containsAnySubstring(text, targetTerms)
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

func consoleFilesAnswerFromRun(view *actionservice.ActionRunView) string {
	if view == nil || view.Run == nil {
		return "I could not read the file result. Please try again."
	}
	if view.Run.Status == actionmodel.ActionRunStatusFailed || view.Run.Status == actionmodel.ActionRunStatusBlocked {
		if view.Run.Error != nil && strings.TrimSpace(*view.Run.Error) != "" {
			return "File read failed: " + strings.TrimSpace(*view.Run.Error)
		}
		return "File read failed. Please try again."
	}
	files := actionRunOutputFiles(view)
	if len(files) == 0 {
		return "I read the file, but no extracted text was returned."
	}
	var builder strings.Builder
	builder.WriteString("I read ")
	builder.WriteString(fmt.Sprintf("%d file", len(files)))
	if len(files) != 1 {
		builder.WriteString("s")
	}
	builder.WriteString(".")
	for _, file := range files {
		name := strings.TrimSpace(stringMetadataValue(file["name"]))
		if name == "" {
			name = strings.TrimSpace(stringMetadataValue(file["id"]))
		}
		if name != "" {
			builder.WriteString("\n\n")
			builder.WriteString(name)
			builder.WriteString(":\n")
		}
		preview := strings.TrimSpace(stringMetadataValue(file["content_preview"]))
		if preview == "" {
			status := strings.TrimSpace(stringMetadataValue(file["content_status"]))
			if status == "" {
				status = "metadata_only"
			}
			builder.WriteString("No extracted text is available (")
			builder.WriteString(status)
			builder.WriteString(").")
			continue
		}
		builder.WriteString(truncateRunes(preview, 700))
		if truncated, ok := file["content_truncated"].(bool); ok && truncated {
			builder.WriteString("...")
		}
	}
	return strings.TrimSpace(builder.String())
}

func actionRunOutputFiles(view *actionservice.ActionRunView) []map[string]interface{} {
	if view == nil {
		return nil
	}
	for _, step := range view.Steps {
		if step == nil || len(step.Output) == 0 {
			continue
		}
		files := mapsFromAny(step.Output["files"])
		if len(files) > 0 {
			return files
		}
	}
	return nil
}

func mapsFromAny(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func actionRunResponseForMetadata(view *actionservice.ActionRunView) actiondto.ActionRunResponse {
	if view == nil || view.Run == nil {
		return actiondto.ActionRunResponse{Steps: []actiondto.ActionStepResponse{}}
	}
	run := view.Run
	resp := actiondto.ActionRunResponse{
		ID:                   run.ID.String(),
		OrganizationID:       run.OrganizationID.String(),
		AccountID:            run.AccountID.String(),
		IdempotencyKey:       run.IdempotencyKey,
		Intent:               run.Intent,
		CapabilityID:         run.CapabilityID,
		Title:                run.Title,
		Summary:              run.Summary,
		Status:               run.Status,
		RiskLevel:            run.RiskLevel,
		RequiresConfirmation: run.RequiresConfirmation,
		ConfirmationStatus:   actionRunConfirmationStatus(run.RequiresConfirmation, run.Status, run.ConfirmedAt),
		ConfirmedAt:          unixTimePtr(run.ConfirmedAt),
		CanceledAt:           unixTimePtr(run.CanceledAt),
		Error:                run.Error,
		Resources:            nonNilActionMap(run.Resources),
		Arguments:            nonNilActionMap(run.Arguments),
		Ledger:               nonNilActionMap(run.Ledger),
		Metadata:             nonNilActionMap(run.Metadata),
		Steps:                actionStepResponsesForMetadata(view.Steps),
		CreatedAt:            run.CreatedAt.Unix(),
		UpdatedAt:            run.UpdatedAt.Unix(),
	}
	if run.WorkspaceID != nil {
		resp.WorkspaceID = actionStringPtr(run.WorkspaceID.String())
	}
	if run.ConversationID != nil {
		resp.ConversationID = actionStringPtr(run.ConversationID.String())
	}
	if run.MessageID != nil {
		resp.MessageID = actionStringPtr(run.MessageID.String())
	}
	if run.ConfirmedBy != nil {
		resp.ConfirmedBy = actionStringPtr(run.ConfirmedBy.String())
	}
	if view.Capability != nil && view.Capability.ID != "" {
		capability := actiondto.ActionCapabilityResponse{
			ID:                   view.Capability.ID,
			Domain:               view.Capability.Domain,
			Action:               view.Capability.Action,
			Name:                 view.Capability.Name,
			Description:          view.Capability.Description,
			Runtime:              view.Capability.Runtime,
			AuthMode:             view.Capability.AuthMode,
			RiskLevel:            view.Capability.RiskLevel,
			RequiresConfirmation: view.Capability.RequiresConfirmation,
			IdempotencyRequired:  view.Capability.IdempotencyRequired,
			TokenTTLSeconds:      view.Capability.TokenTTLSeconds,
			AllowedResources:     append([]string(nil), view.Capability.AllowedResources...),
			Scopes:               append([]string(nil), view.Capability.Scopes...),
		}
		resp.Capability = &capability
	}
	return resp
}

func actionStepResponsesForMetadata(steps []*actionmodel.ActionStep) []actiondto.ActionStepResponse {
	out := make([]actiondto.ActionStepResponse, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		out = append(out, actiondto.ActionStepResponse{
			ID:                   step.ID.String(),
			RunID:                step.RunID.String(),
			StepIndex:            step.StepIndex,
			StepKey:              step.StepKey,
			CapabilityID:         step.CapabilityID,
			Title:                step.Title,
			Status:               step.Status,
			RiskLevel:            step.RiskLevel,
			RequiresConfirmation: step.RequiresConfirmation,
			StartedAt:            unixTimePtr(step.StartedAt),
			CompletedAt:          unixTimePtr(step.CompletedAt),
			Error:                step.Error,
			Input:                nonNilActionMap(step.Input),
			Output:               actionStepOutputForMetadata(step.Output),
			Metadata:             nonNilActionMap(step.Metadata),
			CreatedAt:            step.CreatedAt.Unix(),
			UpdatedAt:            step.UpdatedAt.Unix(),
		})
	}
	return out
}

func actionStepOutputForMetadata(output map[string]interface{}) map[string]interface{} {
	out := copyStringAnyMap(output)
	if out == nil {
		out = map[string]interface{}{}
	}
	if len(out) == 0 {
		return out
	}
	if files := modelInvocationFileItemsFromAny(out["files"]); len(files) > 0 {
		fileSummaries, redacted := modelInvocationPayloadFilesSummary(files)
		if len(fileSummaries) > 0 {
			out["files"] = fileSummaries
		}
		if redacted {
			out["files_content_redacted"] = true
		}
	}
	return out
}

func actionRunConfirmationStatus(requiresConfirmation bool, status string, confirmedAt *time.Time) string {
	if status == actionmodel.ActionRunStatusCanceled {
		return "canceled"
	}
	if confirmedAt != nil {
		return "confirmed"
	}
	if requiresConfirmation {
		return "pending"
	}
	return "not_required"
}

func unixTimePtr(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	out := value.Unix()
	return &out
}

func nonNilActionMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	return input
}

func actionStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
