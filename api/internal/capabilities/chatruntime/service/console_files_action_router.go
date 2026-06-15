package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	actionservice "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	consoleFilesActionCapabilityID = "file.read"
	consoleFilesActionIntent       = "console.files.file_read"
	consoleFilesNoFileAnswer       = "Please select a file on the Files page, or mention the exact visible file name, then ask me to read it."
	consoleFilesReadMaxChars       = 4000
)

var (
	consoleFilesReadIntentPattern       = regexp.MustCompile(`(?i)\b(read|preview|summari[sz]e|summary|analy[sz]e|analysis|inspect|show)\b`)
	consoleFilesDeleteIntentPattern     = regexp.MustCompile(`(?i)\b(delete|remove|trash|discard)\b`)
	consoleFilesCapabilityPattern       = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])file\.read([^a-z0-9_.-]|$)`)
	consoleFilesDeleteCapabilityPattern = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])file\.delete([^a-z0-9_.-]|$)`)
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

func isConsoleFilesContext(runtimeContext string, contexts ...map[string]interface{}) bool {
	if containsConsoleFilesToken(runtimeContext) {
		return true
	}
	for _, ctx := range contexts {
		if contextContainsConsoleFiles(ctx, 0) {
			return true
		}
	}
	if parsed := runtimeContextJSON(runtimeContext); parsed != nil {
		return contextContainsConsoleFiles(parsed, 0)
	}
	return false
}

func contextContainsConsoleFiles(value interface{}, depth int) bool {
	if depth > 4 || value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return containsConsoleFilesToken(typed)
	case map[string]interface{}:
		for key, item := range typed {
			if containsConsoleFilesToken(key) || contextContainsConsoleFiles(item, depth+1) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if contextContainsConsoleFiles(item, depth+1) {
				return true
			}
		}
	case []map[string]interface{}:
		for _, item := range typed {
			if contextContainsConsoleFiles(item, depth+1) {
				return true
			}
		}
	}
	return false
}

func containsConsoleFilesToken(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(text, "/console/files") || strings.Contains(text, "console.files") || strings.Contains(text, "console_files")
}

func runtimeContextJSON(value string) map[string]interface{} {
	text := strings.TrimSpace(value)
	if text == "" || !strings.HasPrefix(text, "{") {
		return nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil
	}
	return parsed
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

func hasConsoleFilesCapability(runtimeContext string, pattern *regexp.Regexp, contexts ...map[string]interface{}) bool {
	if containsConsoleFilesCapabilityToken(runtimeContext, pattern) {
		return true
	}
	for _, ctx := range contexts {
		if contextContainsConsoleFilesCapability(ctx, pattern, 0) {
			return true
		}
	}
	if parsed := runtimeContextJSON(runtimeContext); parsed != nil {
		return contextContainsConsoleFilesCapability(parsed, pattern, 0)
	}
	return false
}

func contextContainsConsoleFilesCapability(value interface{}, pattern *regexp.Regexp, depth int) bool {
	if depth > 5 || value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return containsConsoleFilesCapabilityToken(typed, pattern)
	case map[string]interface{}:
		for key, item := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			switch normalizedKey {
			case "id", "capability_id", "tool_id":
				if containsConsoleFilesCapabilityToken(stringMetadataValue(item), pattern) {
					return true
				}
			case "capability_ids", "capabilities", "tool_ids", "tools":
				if contextContainsConsoleFilesCapability(item, pattern, depth+1) {
					return true
				}
			default:
				if contextContainsConsoleFilesCapability(item, pattern, depth+1) {
					return true
				}
			}
		}
	case []interface{}:
		for _, item := range typed {
			if contextContainsConsoleFilesCapability(item, pattern, depth+1) {
				return true
			}
		}
	case []map[string]interface{}:
		for _, item := range typed {
			if contextContainsConsoleFilesCapability(item, pattern, depth+1) {
				return true
			}
		}
	}
	return false
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
	ID        string
	Title     string
	Extension string
	MimeType  string
	Selected  bool
}

func visibleFileResources(context map[string]interface{}) []visibleConsoleFileResource {
	if len(context) == 0 {
		return nil
	}
	items := operationItemsFromValue(context["resources"])
	out := make([]visibleConsoleFileResource, 0, len(items))
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
			stringMetadataValue(resource["extension"]),
			stringMetadataValue(resource["subtitle"]),
			stringMetadataValue(firstMapValue(metadata, "extension", "file_extension")),
		)
		mimeType := firstNonEmptyString(
			stringMetadataValue(resource["mime_type"]),
			stringMetadataValue(resource["mimeType"]),
			stringMetadataValue(firstMapValue(metadata, "mime_type", "mimeType")),
		)
		selected := boolMetadataValue(firstMapValue(resource, "selected", "is_selected")) ||
			boolMetadataValue(firstMapValue(metadata, "selected", "is_selected"))
		out = append(out, visibleConsoleFileResource{
			ID:        id,
			Title:     title,
			Extension: strings.TrimPrefix(strings.TrimSpace(extension), "."),
			MimeType:  strings.TrimSpace(mimeType),
			Selected:  selected,
		})
	}
	return out
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
			Output:               nonNilActionMap(step.Output),
			Metadata:             nonNilActionMap(step.Metadata),
			CreatedAt:            step.CreatedAt.Unix(),
			UpdatedAt:            step.UpdatedAt.Unix(),
		})
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
