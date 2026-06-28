package service

import (
	"fmt"
	"strings"
	"testing"

	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestInjectClientActionContinuationContextPromotesLoadedRouteContext(t *testing.T) {
	parts := &chatRequestParts{
		Query:   "open the files page and tell me what is visible",
		Surface: aiChatSurfaceContextualSidebar,
	}
	event := map[string]interface{}{
		"action_id":   "route_navigation:files",
		"action_type": "route_navigation",
		"status":      clientActionStatusWaiting,
		"skill_id":    "console-navigator",
		"tool_name":   "navigate",
	}
	req := runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{
			"event_type":         "route_loaded",
			"loaded_href":        "/console/files",
			"page_context_ready": true,
			"context_items": []interface{}{
				map[string]interface{}{
					"id":                 "console.files",
					"type":               "page",
					"title":              "Files",
					"href":               "/console/files",
					"context_ready":      true,
					"files_query_status": "ready",
					"total_file_count":   7,
					"visible_file_count": 7,
				},
			},
		},
	}

	injectClientActionContinuationContext(parts, event, req, nil)

	if got := contextualTurnCurrentPage(parts); got != "/console/files" {
		t.Fatalf("contextualTurnCurrentPage() = %q, want /console/files", got)
	}
	if !clientActionContinuationLoadedRoute(parts, "/console/files") {
		t.Fatal("clientActionContinuationLoadedRoute() = false, want true for loaded_href")
	}
	if !consoleNavigationRouteAlreadyAvailable(parts, "/console/files") {
		t.Fatal("consoleNavigationRouteAlreadyAvailable() = false, want true")
	}

	evidence := skillLoopCompletionPageContextEvidence(parts)
	resources := operationItemsFromValue(evidence["resources"])
	if len(resources) == 0 {
		t.Fatalf("page context resources = %#v, want promoted route resources", evidence["resources"])
	}
	var filesPage map[string]interface{}
	for _, item := range resources {
		resource := mapFromOperationContext(item)
		if stringFromAny(resource["href"]) == "/console/files" {
			filesPage = resource
			break
		}
	}
	if len(filesPage) == 0 {
		t.Fatalf("page context resources = %#v, want files page resource", resources)
	}
	if filesPage["context_ready"] != true {
		t.Fatalf("files page context_ready = %#v, want true", filesPage["context_ready"])
	}
	if filesPage["total_file_count"] != 7 {
		t.Fatalf("files page total_file_count = %#v, want 7", filesPage["total_file_count"])
	}
	if filesPage["visible_file_count"] != 7 {
		t.Fatalf("files page visible_file_count = %#v, want 7", filesPage["visible_file_count"])
	}
}

func TestClientActionContinuationMessageFramesToolResultAsCurrentTurnEvidence(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "请删除刚刚创建的文件 aichat-plan-smoke.md",
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillFileManager,
					"tool_name": "delete_file",
					"status":    "success",
					"result": map[string]interface{}{
						"status":        "completed",
						"deleted_count": 1,
						"file_name":     "aichat-plan-smoke.md",
					},
				},
				map[string]interface{}{
					"kind":        "client_action",
					"action_id":   "asset_observation:delete-file",
					"action_type": "asset_observation",
					"status":      clientActionStatusWaiting,
					"skill_id":    skills.SkillFileManager,
					"tool_name":   "delete_file",
				},
			},
		},
	}
	event := map[string]interface{}{
		"action_id":   "asset_observation:delete-file",
		"action_type": "asset_observation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillFileManager,
		"tool_name":   "delete_file",
	}

	msg := clientActionContinuationMessage(message, event, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"visible_count": 0},
	})
	content := strings.TrimSpace(fmt.Sprint(msg.Content))
	for _, want := range []string{
		"Current-turn tool result immediately before this frontend action JSON",
		"authoritative evidence for the current user request",
		"do not describe it as a previous round",
		"上一轮",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Previously completed tool call") {
		t.Fatalf("continuation message still uses previous-turn wording:\n%s", content)
	}
}
