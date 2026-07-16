package service

import (
	"testing"

	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestContextualTurnCurrentPagePrefersCurrentPageContext(t *testing.T) {
	parts := &chatRequestParts{
		RawOperationContext: map[string]interface{}{
			currentPageContextKey: map[string]interface{}{"route": "/console/files", "status": "ready"},
			"resources":           []interface{}{map[string]interface{}{"resource_type": "page", "href": "/console/agents/agent-1/agent"}},
		},
	}
	if got := contextualTurnCurrentPage(parts); got != "/console/files" {
		t.Fatalf("contextualTurnCurrentPage() = %q, want current page context route", got)
	}
}

func TestContextualTurnCurrentPageDoesNotFallBackAfterRefreshFailure(t *testing.T) {
	parts := &chatRequestParts{
		RawOperationContext: map[string]interface{}{
			currentPageContextKey: map[string]interface{}{"route": "", "status": "observed_path_missing"},
			"resources":           []interface{}{map[string]interface{}{"resource_type": "page", "href": "/console/agents/agent-1/agent"}},
		},
		RuntimeContext: "route=/console/agents/agent-1/agent",
	}
	if got := contextualTurnCurrentPage(parts); got != "" {
		t.Fatalf("contextualTurnCurrentPage() = %q, want no stale route fallback", got)
	}
}

func TestNavigationRefreshRequiresObservedPath(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{}},
		parts: &chatRequestParts{
			RawOperationContext: map[string]interface{}{},
			OperationContext:    map[string]interface{}{},
		},
	}
	(&service{}).refreshPageContextAfterClientAction(t.Context(), prepared, map[string]interface{}{
		"action_id": "action-1", "action_type": "route_navigation", "href": "/console/files",
	}, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"view_query": map[string]interface{}{"page": 1}},
	})

	current := mapFromOperationContext(prepared.Message.Metadata[currentPageContextKey])
	if current["status"] != "observed_path_missing" || current["route"] != "" || current["target_route"] != "/console/files" {
		t.Fatalf("current page context = %#v, want missing observed path without target substitution", current)
	}
	refreshes := mapSliceFromAny(prepared.Message.Metadata["page_context_refreshes"])
	if len(refreshes) != 1 || refreshes[0]["status"] != "observed_path_missing" {
		t.Fatalf("page context refreshes = %#v, want observed_path_missing record", refreshes)
	}
}

func TestApplyCurrentPageContextReplacesOldPageResources(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{"current_page": "/console/agents/old-agent/agent"},
		}},
		parts: &chatRequestParts{
			RawOperationContext: map[string]interface{}{"resources": []interface{}{map[string]interface{}{"resource_type": "agent", "resource_id": "old-agent"}}},
			OperationContext:    map[string]interface{}{"resources": []interface{}{map[string]interface{}{"resource_type": "agent", "resource_id": "old-agent"}}},
		},
	}
	current := readyCurrentPageContext("/console/files", "backend_api", "files", map[string]interface{}{"page": 1}, []map[string]interface{}{
		{"resource_type": "file", "resource_id": "file-1", "title": "one.md"},
	}, 1, 1)
	(&service{}).applyCurrentPageContext(prepared, current, map[string]interface{}{"action_id": "action-1"})

	resources := mapSliceFromAny(prepared.parts.OperationContext["resources"])
	if len(resources) != 2 || resources[0]["resource_type"] != "page" || resources[1]["resource_id"] != "file-1" {
		t.Fatalf("current resources = %#v, want route plus backend file", resources)
	}
	for _, resource := range resources {
		if resource["resource_id"] == "old-agent" {
			t.Fatalf("old page resource survived refresh: %#v", resources)
		}
	}
	state := mapFromOperationContext(prepared.Message.Metadata["turn_state"])
	if navigations := mapSliceFromAny(state["navigations"]); len(navigations) != 1 || navigations[0]["route"] != "/console/files" {
		t.Fatalf("turn_state.navigations = %#v, want refreshed route", navigations)
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if plan["current_page"] != "/console/files" {
		t.Fatalf("operation_plan.current_page = %#v, want refreshed route", plan["current_page"])
	}
	pageEvidence := mapFromOperationContext(plan["current_page_evidence"])
	if pageEvidence["route_evidence"] != "ready" {
		t.Fatalf("operation_plan.current_page_evidence = %#v, want ready route evidence", pageEvidence)
	}
}

func TestPageViewQueryFromPartsNormalizesFilesMetadata(t *testing.T) {
	parts := &chatRequestParts{OperationContext: map[string]interface{}{
		"resources": []interface{}{map[string]interface{}{
			"resource_type": "page",
			"metadata": map[string]interface{}{
				"current_page": 2, "page_size": 20, "search": "report", "extension_filter": "pdf", "selected_file_ids": "file-1,file-2",
			},
		}},
	}}
	query := pageViewQueryFromParts(parts)
	if query["page"] != 2 || query["keyword"] != "report" || query["extension"] != "pdf" || query["selected_ids"] != "file-1,file-2" {
		t.Fatalf("pageViewQueryFromParts() = %#v", query)
	}
}

func TestSuccessfulMutationMarksCurrentPageContextStaleAndHidesOrdinalList(t *testing.T) {
	parts := &chatRequestParts{
		RawOperationContext: map[string]interface{}{
			currentPageContextKey: map[string]interface{}{"route": "/console/files", "status": "ready"},
			"resources":           []interface{}{map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "one.md", "metadata": map[string]interface{}{"visible_index": 1, "file_id": "file-1"}}},
		},
		OperationContext: map[string]interface{}{},
	}
	prepared := &PreparedChat{parts: parts, Message: &runtimemodel.Message{Metadata: map[string]interface{}{currentPageContextKey: map[string]interface{}{"route": "/console/files", "status": "ready"}}}}
	markPreparedCurrentPageContextStale(prepared, skills.SkillTrace{SkillID: skills.SkillFileManager, ToolName: "delete_file", Status: "success"})
	if !currentPageContextIsStale(parts) {
		t.Fatal("current page context was not marked stale")
	}
	current := mapFromOperationContext(parts.RawOperationContext[currentPageContextKey])
	if current["status"] != "ready" || current["data_status"] != "stale" {
		t.Fatalf("current page state = %#v, want route ready with stale data", current)
	}
	if files := consoleFilesPromptVisibleFiles(parts); len(files) != 0 {
		t.Fatalf("consoleFilesPromptVisibleFiles() = %#v, want stale list hidden", files)
	}
}
