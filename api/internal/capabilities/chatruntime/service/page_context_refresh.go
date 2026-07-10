package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	currentPageContextKey    = "current_page_context"
	currentPageContextSchema = "zgi.aichat.current_page_context.v2"
	turnStartContextKey      = "turn_start_context"
)

type pageContextProvider struct {
	name     string
	matches  func(string) bool
	skillID  string
	toolName func(string) string
	args     func(string, map[string]interface{}) map[string]interface{}
	itemsKey func(string) string
}

func registeredPageContextProviders() []pageContextProvider {
	return []pageContextProvider{
		{
			name:     "files",
			matches:  func(route string) bool { return consoleNavigationLoadedHrefMatchesTarget(route, "/console/files") },
			skillID:  skills.SkillFileReader,
			toolName: func(string) string { return "list_visible_files" },
			args: func(_ string, query map[string]interface{}) map[string]interface{} {
				args := copyStringAnyMap(query)
				if selected, ok := args["selected_ids"].([]interface{}); ok {
					values := make([]string, 0, len(selected))
					for _, item := range selected {
						if value := strings.TrimSpace(fmt.Sprint(item)); value != "" {
							values = append(values, value)
						}
					}
					args["selected_ids"] = strings.Join(values, ",")
				}
				return args
			},
			itemsKey: func(string) string { return "files" },
		},
		{
			name: "agents",
			matches: func(route string) bool {
				return consoleNavigationLoadedHrefMatchesTarget(route, "/console/agents") || isConsoleAgentDetailRoute(route)
			},
			skillID: skills.SkillAgentManagement,
			toolName: func(route string) string {
				if isConsoleAgentDetailRoute(route) {
					return "get_agent_config"
				}
				return "list_agents"
			},
			args: func(route string, query map[string]interface{}) map[string]interface{} {
				if agentID := agentIDFromConsoleAgentPageRoute(route); agentID != "" {
					return map[string]interface{}{"agent_id": agentID}
				}
				args := copyStringAnyMap(query)
				pageSize := intValueFromAny(args["page_size"])
				loadedCount := intValueFromAny(args["loaded_count"])
				if loadedCount > pageSize {
					pageSize = loadedCount
				}
				if pageSize > 0 {
					args["limit"] = pageSize
					delete(args, "page_size")
				}
				delete(args, "asset_kind")
				delete(args, "loaded_count")
				return args
			},
			itemsKey: func(route string) string {
				if isConsoleAgentDetailRoute(route) {
					return "agent"
				}
				return "agents"
			},
		},
	}
}

func pageContextProviderForRoute(route string) (pageContextProvider, bool) {
	for _, provider := range registeredPageContextProviders() {
		if provider.matches(route) {
			return provider, true
		}
	}
	return pageContextProvider{}, false
}

func (s *service) refreshPageContextAfterClientAction(ctx context.Context, prepared *PreparedChat, event map[string]interface{}, req runtimedto.ClientActionResultRequest) {
	if prepared == nil || prepared.parts == nil || prepared.Message == nil {
		return
	}
	record := clientActionObservationRecord(event, req)
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(record["action_type"])), "route_navigation") &&
		!isConsoleNavigatorNavigateTool(stringFromAny(record["skill_id"]), stringFromAny(record["tool_name"])) {
		return
	}
	started := time.Now()
	result := mapFromOperationContext(record["result"])
	target := normalizeConsoleNavigationGuardHref(firstNonEmptyString(event["href"], result["requested_href"], result["target_href"], result["href"]))
	observed := normalizeConsoleNavigationGuardHref(firstNonEmptyString(result["observed_path"], result["current_href"], result["loaded_href"]))
	query := mapFromOperationContext(result["view_query"])
	if !strings.EqualFold(strings.TrimSpace(req.Status), clientActionStatusSucceeded) {
		return
	}
	if observed == "" {
		errorText := "client did not report the loaded route"
		contextValue := failedCurrentPageContext("", query, "observed_path_missing", errorText)
		contextValue["target_route"] = target
		s.applyCurrentPageContext(prepared, contextValue, record)
		s.recordPageContextRefresh(prepared, contextValue, "route_loaded", time.Since(started), errorText)
		return
	}
	if target != "" && !consoleNavigationLoadedHrefMatchesTarget(observed, target) {
		errorText := fmt.Sprintf("observed path %s does not match target %s", observed, target)
		contextValue := failedCurrentPageContext(observed, query, "observed_path_mismatch", errorText)
		s.applyCurrentPageContext(prepared, contextValue, record)
		s.recordPageContextRefresh(prepared, contextValue, "route_loaded", time.Since(started), errorText)
		return
	}
	s.refreshAuthoritativePageContext(ctx, prepared, observed, query, "route_loaded", record)
}

func (s *service) refreshInitialPageContext(ctx context.Context, prepared *PreparedChat) {
	if prepared == nil || prepared.parts == nil || prepared.Message == nil {
		return
	}
	route := contextualTurnCurrentPage(prepared.parts)
	if route == "" {
		return
	}
	query := pageViewQueryFromParts(prepared.parts)
	s.refreshAuthoritativePageContext(ctx, prepared, route, query, "turn_start", nil)
}

func (s *service) refreshAuthoritativePageContext(ctx context.Context, prepared *PreparedChat, route string, query map[string]interface{}, reason string, navigation map[string]interface{}) {
	route = normalizeConsoleNavigationGuardHref(route)
	started := time.Now()
	provider, ok := pageContextProviderForRoute(route)
	if !ok {
		contextValue := readyCurrentPageContext(route, "route_observation", "route", query, nil, 0, 0)
		s.applyCurrentPageContext(prepared, contextValue, navigation)
		s.recordPageContextRefresh(prepared, contextValue, reason, time.Since(started), "")
		return
	}
	if s == nil || s.skillRuntime == nil {
		contextValue := failedCurrentPageContext(route, query, "context_refresh_failed", "skill runtime is not configured")
		s.applyCurrentPageContext(prepared, contextValue, navigation)
		s.recordPageContextRefresh(prepared, contextValue, reason, time.Since(started), "skill runtime is not configured")
		return
	}
	resolved, err := s.skillRuntime.ResolveEnabledSkills(ctx, []string{provider.skillID})
	if err == nil {
		invocation, invokeErr := s.skillRuntime.CallSkillTool(
			ctx,
			resolved,
			provider.skillID,
			provider.toolName(route),
			provider.args(route, query),
			s.skillExecutionContext(prepared),
			"page_context_"+uuid.NewString(),
		)
		if invokeErr != nil {
			err = invokeErr
		} else {
			payload := pageContextInvocationPayload(invocation)
			itemsKey := provider.itemsKey(route)
			resources := pageContextResources(route, provider.name, itemsKey, payload)
			itemCount := len(mapSliceFromAny(payload[itemsKey]))
			if itemsKey == "agent" && len(mapFromOperationContext(payload[itemsKey])) > 0 {
				itemCount = 1
			}
			contextValue := readyCurrentPageContext(route, "backend_api", provider.name, query, resources, itemCount, intValueFromAny(payload["total"]))
			contextValue["result_summary"] = compactPageContextResult(payload, itemsKey)
			s.applyCurrentPageContext(prepared, contextValue, navigation)
			s.recordPageContextRefresh(prepared, contextValue, reason, time.Since(started), "")
			return
		}
	}
	contextValue := failedCurrentPageContext(route, query, "context_refresh_failed", err.Error())
	s.applyCurrentPageContext(prepared, contextValue, navigation)
	s.recordPageContextRefresh(prepared, contextValue, reason, time.Since(started), err.Error())
}

func readyCurrentPageContext(route string, source string, provider string, query map[string]interface{}, resources []map[string]interface{}, itemCount int, total int) map[string]interface{} {
	value := map[string]interface{}{
		"schema": currentPageContextSchema, "status": "ready", "route": route, "source": source,
		"data_status": "fresh",
		"provider":    provider, "view_query": copyStringAnyMap(query), "query_fingerprint": pageQueryFingerprint(query),
		"item_count": itemCount, "refreshed_at": time.Now().UTC().Format(time.RFC3339),
	}
	if total > 0 {
		value["total"] = total
	}
	if len(resources) > 0 {
		value["resources"] = mapsToInterfaceSlice(resources)
	}
	return value
}

func failedCurrentPageContext(route string, query map[string]interface{}, status string, message string) map[string]interface{} {
	return map[string]interface{}{
		"schema": currentPageContextSchema, "status": status, "route": route, "source": "backend_api",
		"view_query": copyStringAnyMap(query), "query_fingerprint": pageQueryFingerprint(query),
		"error": compactForPrompt(message, 500), "refreshed_at": time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *service) applyCurrentPageContext(prepared *PreparedChat, current map[string]interface{}, navigation map[string]interface{}) {
	if prepared == nil || prepared.parts == nil || prepared.Message == nil || len(current) == 0 {
		return
	}
	resources := mapSliceFromAny(current["resources"])
	if len(resources) == 0 {
		resources = []map[string]interface{}{pageContextRouteResource(current)}
	} else {
		resources = append([]map[string]interface{}{pageContextRouteResource(current)}, resources...)
	}
	current["resources"] = mapsToInterfaceSlice(resources)
	for _, target := range []map[string]interface{}{prepared.parts.RawOperationContext, prepared.parts.OperationContext} {
		if target == nil {
			continue
		}
		target[currentPageContextKey] = copyStringAnyMap(current)
		target["resources"] = mapsToInterfaceSlice(resources)
		if start := mapFromOperationContext(prepared.Message.Metadata[turnStartContextKey]); len(start) > 0 {
			target[turnStartContextKey] = start
		}
	}
	metadata := prepared.Message.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata[currentPageContextKey] = copyStringAnyMap(current)
	state := copyStringAnyMap(mapFromOperationContext(metadata["turn_state"]))
	if state == nil {
		state = map[string]interface{}{}
	}
	if len(navigation) > 0 {
		navigations := mapSliceFromAny(state["navigations"])
		navigations = append(navigations, map[string]interface{}{
			"action_id": stringFromAny(navigation["action_id"]), "status": current["status"], "route": current["route"],
			"query_fingerprint": current["query_fingerprint"], "observed_at": current["refreshed_at"],
		})
		if len(navigations) > 16 {
			navigations = navigations[len(navigations)-16:]
		}
		state["navigations"] = mapsToInterfaceSlice(navigations)
	}
	metadata["turn_state"] = state
	applyOperationPlanPageEvidence(metadata, map[string]interface{}{
		"current_page":   current["route"],
		"runtime_route":  current["route"],
		"route_evidence": current["status"],
		"resources":      current["resources"],
	})
	appendPageContextEvidence(metadata, current)
	if snapshot := consoleFilesContextSnapshot(prepared.parts); len(snapshot) > 0 {
		metadata[consoleFilesContextSnapshotKey] = snapshot
	}
	if snapshot := consoleAgentsContextSnapshot(prepared.parts); len(snapshot) > 0 {
		metadata[consoleAgentsContextSnapshotKey] = snapshot
	}
	prepared.Message.Metadata = metadata
}

func pageContextRouteResource(current map[string]interface{}) map[string]interface{} {
	route := strings.TrimSpace(stringFromAny(current["route"]))
	return map[string]interface{}{
		"resource_type": "page", "resource_id": route, "title": route, "href": route,
		"metadata": map[string]interface{}{
			"route": route, "context_status": current["status"], "source": current["source"],
			"query_fingerprint": current["query_fingerprint"],
		},
	}
}

func appendPageContextEvidence(metadata map[string]interface{}, current map[string]interface{}) {
	if metadata == nil || current == nil {
		return
	}
	status := "completed"
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(current["status"])), "ready") {
		status = "failed"
	}
	ref := "page_context_refresh:" + strings.TrimSpace(stringFromAny(current["query_fingerprint"])) + ":" + fmt.Sprint(time.Now().UnixNano())
	entry := map[string]interface{}{
		"kind": "page_context_refresh", "status": status, "invocation_id": ref,
		"keys":           []interface{}{"page.route", "page.context"},
		"target":         map[string]interface{}{"route": current["route"]},
		"result_summary": map[string]interface{}{"route": current["route"], "source": current["source"], "item_count": current["item_count"]},
		"result_facts":   map[string]interface{}{"observed_path": current["route"], "context_status": current["status"], "query_fingerprint": current["query_fingerprint"]},
	}
	ledger := mapSliceFromAny(metadata[operationPlanEvidenceLedgerKey])
	ledger = append(ledger, entry)
	if len(ledger) > 50 {
		ledger = ledger[len(ledger)-50:]
	}
	metadata[operationPlanEvidenceLedgerKey] = mapsToInterfaceSlice(ledger)
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) > 0 {
		plan[operationPlanEvidenceLedgerKey] = mapsToInterfaceSlice(ledger)
		metadata["operation_plan"] = plan
	}
}

func (s *service) recordPageContextRefresh(prepared *PreparedChat, current map[string]interface{}, reason string, duration time.Duration, errorText string) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	metadata := prepared.Message.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	records := mapSliceFromAny(metadata["page_context_refreshes"])
	record := map[string]interface{}{
		"path": current["route"], "status": current["status"], "source": current["source"], "reason": reason,
		"query_fingerprint": current["query_fingerprint"], "item_count": current["item_count"], "duration_ms": duration.Milliseconds(),
	}
	if errorText != "" {
		record["error"] = compactForPrompt(errorText, 500)
	}
	records = append(records, record)
	if len(records) > 20 {
		records = records[len(records)-20:]
	}
	metadata["page_context_refreshes"] = mapsToInterfaceSlice(records)
	prepared.Message.Metadata = metadata
}

func pageContextInvocationPayload(invocation *skills.ToolInvocationResult) map[string]interface{} {
	if invocation == nil {
		return map[string]interface{}{}
	}
	for _, message := range invocation.Messages {
		if len(message.Data) > 0 {
			return copyStringAnyMap(message.Data)
		}
	}
	content := strings.TrimSpace(messageContentText(invocation.ToolMessage.Content))
	if content == "" {
		return map[string]interface{}{}
	}
	payload := map[string]interface{}{}
	_ = json.Unmarshal([]byte(content), &payload)
	return payload
}

func pageContextResources(route string, provider string, itemsKey string, payload map[string]interface{}) []map[string]interface{} {
	if itemsKey == "agent" {
		item := mapFromOperationContext(payload[itemsKey])
		if len(item) == 0 {
			return nil
		}
		return []map[string]interface{}{pageContextItemResource(route, "agent", item)}
	}
	items := mapSliceFromAny(payload[itemsKey])
	resources := make([]map[string]interface{}, 0, len(items))
	for idx, item := range items {
		resource := pageContextItemResource(route, strings.TrimSuffix(itemsKey, "s"), item)
		metadata := mapFromOperationContext(resource["metadata"])
		metadata["visible_index"] = idx + 1
		metadata["source"] = "backend_api"
		resource["metadata"] = metadata
		resources = append(resources, resource)
	}
	_ = provider
	return resources
}

func pageContextItemResource(route string, resourceType string, item map[string]interface{}) map[string]interface{} {
	id := strings.TrimSpace(firstNonEmptyString(item["id"], item["file_id"], item["agent_id"]))
	name := strings.TrimSpace(firstNonEmptyString(item["name"], item["filename"], item["agent_name"]))
	href := strings.TrimSpace(stringFromAny(item["href"]))
	if href == "" && resourceType == "agent" && id != "" {
		href = "/console/agents/" + id + "/agent"
	}
	metadata := copyStringAnyMap(item)
	metadata["source"] = "backend_api"
	metadata["route"] = route
	return map[string]interface{}{"resource_type": resourceType, "resource_id": id, "title": name, "href": href, "metadata": metadata}
}

func compactPageContextResult(payload map[string]interface{}, itemsKey string) map[string]interface{} {
	return map[string]interface{}{
		"status": payload["status"], "source": payload["source"], "count": payload["count"], "total": payload["total"],
		"items_key": itemsKey,
	}
}

func pageViewQueryFromParts(parts *chatRequestParts) map[string]interface{} {
	if parts == nil {
		return map[string]interface{}{}
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		if current := mapFromOperationContext(source[currentPageContextKey]); len(current) > 0 {
			return copyStringAnyMap(mapFromOperationContext(current["view_query"]))
		}
		for _, resource := range mapSliceFromAny(source["resources"]) {
			if !strings.EqualFold(strings.TrimSpace(stringFromAny(resource["resource_type"])), "page") {
				continue
			}
			metadata := mapFromOperationContext(resource["metadata"])
			query := map[string]interface{}{}
			copyPageQueryFields(query, metadata)
			copyPageQueryFields(query, resource)
			if len(query) > 0 {
				return query
			}
		}
	}
	return map[string]interface{}{}
}

func copyPageQueryFields(target map[string]interface{}, source map[string]interface{}) {
	for _, key := range []string{"page", "page_size", "keyword", "sort", "extension", "category", "folder_id", "processing_status", "workspace_id", "selected_ids", "loaded_count", "asset_kind"} {
		if value, ok := source[key]; ok && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			target[key] = value
		}
	}
	if _, ok := target["page"]; !ok {
		if value, exists := source["current_page"]; exists {
			target["page"] = value
		}
	}
	if _, ok := target["keyword"]; !ok {
		if value, exists := source["search"]; exists {
			target["keyword"] = value
		}
	}
	if _, ok := target["extension"]; !ok {
		if value, exists := source["extension_filter"]; exists {
			target["extension"] = value
		}
	}
	if _, ok := target["selected_ids"]; !ok {
		if value, exists := source["selected_file_ids"]; exists {
			target["selected_ids"] = value
		}
	}
}

func pageQueryFingerprint(query map[string]interface{}) string {
	encoded, _ := json.Marshal(query)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:8])
}

func restoreCurrentPageContextFromMetadata(parts *chatRequestParts, metadata map[string]interface{}) {
	if parts == nil || len(metadata) == 0 {
		return
	}
	current := mapFromOperationContext(metadata[currentPageContextKey])
	if len(current) == 0 {
		return
	}
	resources := mapSliceFromAny(current["resources"])
	for _, target := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		if target == nil {
			continue
		}
		target[currentPageContextKey] = copyStringAnyMap(current)
		if len(resources) > 0 {
			target["resources"] = mapsToInterfaceSlice(resources)
		}
		if start := mapFromOperationContext(metadata[turnStartContextKey]); len(start) > 0 {
			target[turnStartContextKey] = start
		}
	}
}

func markPreparedCurrentPageContextStale(prepared *PreparedChat, trace skills.SkillTrace) {
	if prepared == nil || prepared.parts == nil || !pageContextTraceInvalidatesCurrentPage(trace) {
		return
	}
	for _, target := range []map[string]interface{}{prepared.parts.RawOperationContext, prepared.parts.OperationContext} {
		markContextMapCurrentPageStale(target, trace.SkillID, trace.ToolName)
	}
	if prepared.Message != nil {
		markMetadataCurrentPageContextStale(prepared.Message.Metadata, trace)
	}
}

func markMetadataCurrentPageContextStale(metadata map[string]interface{}, trace skills.SkillTrace) {
	if !pageContextTraceInvalidatesCurrentPage(trace) {
		return
	}
	markContextMapCurrentPageStale(metadata, trace.SkillID, trace.ToolName)
}

func markMetadataCurrentPageContextStaleFromInvocation(metadata map[string]interface{}, invocation map[string]interface{}) {
	trace := skills.SkillTrace{
		SkillID: strings.TrimSpace(stringFromAny(invocation["skill_id"])), ToolName: strings.TrimSpace(stringFromAny(invocation["tool_name"])),
		Status: strings.TrimSpace(stringFromAny(invocation["status"])),
	}
	markMetadataCurrentPageContextStale(metadata, trace)
}

func pageContextTraceInvalidatesCurrentPage(trace skills.SkillTrace) bool {
	status := strings.ToLower(strings.TrimSpace(trace.Status))
	if status != "success" && status != "succeeded" && status != "completed" {
		return false
	}
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	if toolName == "" || strings.HasPrefix(toolName, "get_") || strings.HasPrefix(toolName, "list_") || strings.HasPrefix(toolName, "read_") || strings.HasPrefix(toolName, "search_") {
		return false
	}
	skillID := strings.ToLower(strings.TrimSpace(trace.SkillID))
	return skillID == skills.SkillAgentManagement || skillID == skills.SkillFileManager || skillID == skills.SkillFileReader
}

func markContextMapCurrentPageStale(target map[string]interface{}, skillID string, toolName string) {
	if len(target) == 0 {
		return
	}
	current := copyStringAnyMap(mapFromOperationContext(target[currentPageContextKey]))
	if len(current) == 0 || !strings.EqualFold(strings.TrimSpace(stringFromAny(current["status"])), "ready") {
		return
	}
	route := strings.TrimSpace(stringFromAny(current["route"]))
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if skillID == skills.SkillAgentManagement && !strings.HasPrefix(route, "/console/agents") {
		return
	}
	if (skillID == skills.SkillFileManager || skillID == skills.SkillFileReader) && !consoleNavigationLoadedHrefMatchesTarget(route, "/console/files") {
		return
	}
	current["data_status"] = "stale"
	current["stale_at"] = time.Now().UTC().Format(time.RFC3339)
	current["stale_reason"] = strings.TrimSpace(skillID + "/" + toolName)
	target[currentPageContextKey] = current
}

func currentPageContextIsStale(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		current := mapFromOperationContext(source[currentPageContextKey])
		if strings.EqualFold(strings.TrimSpace(stringFromAny(current["data_status"])), "stale") {
			return true
		}
	}
	return false
}
