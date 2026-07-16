package consolenavigation

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestNavigateToolAllowsWhitelistedConsoleRoute(t *testing.T) {
	tool := NewNavigateTool("").ForkToolRuntime(&tools.ToolRuntime{InvokeFrom: tools.ToolInvokeFromAIChat})
	messages, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"href":   "/console/files",
		"reason": "The user asked to open file management.",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].Type != tools.ToolInvokeMessageTypeJSON {
		t.Fatalf("messages = %#v, want one JSON message", messages)
	}
	result := messages[0].Data
	if result["status"] != navigationStatus ||
		result["event_type"] != navigationEventType ||
		result["href"] != "/console/files" ||
		result["label"] != "Files" {
		t.Fatalf("result = %#v, want navigation request for /console/files", result)
	}
}

func TestNavigateToolRejectsNonAIChatRuntime(t *testing.T) {
	for _, runtime := range []*tools.ToolRuntime{
		nil,
		{InvokeFrom: tools.ToolInvokeFromAgent},
		{InvokeFrom: tools.ToolInvokeFromWorkflow},
		{InvokeFrom: tools.ToolInvokeFromAPI},
	} {
		tool := NewNavigateTool("")
		if runtime != nil {
			tool = tool.ForkToolRuntime(runtime).(*NavigateTool)
		}
		_, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
			"href": "/console/files",
		}, nil, nil, nil)
		if err == nil {
			t.Fatalf("Invoke with runtime %#v returned nil error, want rejection", runtime)
		}
	}
}

func TestNormalizeConsoleRouteAllowsAgentDetailRoutes(t *testing.T) {
	href, label, err := normalizeConsoleRoute("/console/agents/3806ca05-55c0-4380-a07a-e1cbf6fdcdd1/workflow")
	if err != nil {
		t.Fatalf("normalizeConsoleRoute returned error: %v", err)
	}
	if href != "/console/agents/3806ca05-55c0-4380-a07a-e1cbf6fdcdd1/workflow" || label != "Agent Detail" {
		t.Fatalf("href=%q label=%q, want agent detail route", href, label)
	}
}

func TestNormalizeConsoleRouteCanonicalizesBareAgentDetailRoute(t *testing.T) {
	href, label, err := normalizeConsoleRoute("/console/agents/3806ca05-55c0-4380-a07a-e1cbf6fdcdd1")
	if err != nil {
		t.Fatalf("normalizeConsoleRoute returned error: %v", err)
	}
	if href != "/console/agents/3806ca05-55c0-4380-a07a-e1cbf6fdcdd1/agent" || label != "Agent Detail" {
		t.Fatalf("href=%q label=%q, want canonical Agent config route", href, label)
	}
}

func TestNormalizeConsoleRouteRejectsExternalAndUnknownRoutes(t *testing.T) {
	for _, href := range []string{
		"https://example.com/console/files",
		"//example.com/console/files",
		"/console/files/../settings",
		"/admin",
		"/console/unknown",
	} {
		if _, _, err := normalizeConsoleRoute(href); err == nil {
			t.Fatalf("normalizeConsoleRoute(%q) returned nil error, want rejection", href)
		}
	}
}
