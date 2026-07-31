package consolenavigation

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func authorizedAIChatRuntime(authorizer RouteAuthorizer) *tools.ToolRuntime {
	return &tools.ToolRuntime{
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: WithRouteAuthorizer(
			map[string]interface{}{"workspace_id": "workspace-1"},
			authorizer,
		),
	}
}

func TestNavigateToolAllowsWhitelistedConsoleRoute(t *testing.T) {
	tests := []struct {
		name  string
		href  string
		label string
	}{
		{name: "files", href: "/console/files", label: "Files"},
		{name: "workflows", href: "/console/workflows", label: "Workflows"},
		{name: "skills", href: "/console/skills", label: "Skills"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewNavigateTool("").ForkToolRuntime(authorizedAIChatRuntime(
				func(context.Context, RouteAuthorizationRequest) error { return nil },
			))
			messages, err := tool.Invoke(t.Context(), "user-1", map[string]interface{}{
				"href":   tt.href,
				"reason": "The user asked to open the page.",
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
				result["href"] != tt.href ||
				result["label"] != tt.label {
				t.Fatalf("result = %#v, want navigation request for %s labeled %q", result, tt.href, tt.label)
			}
		})
	}
}

func TestNavigateToolUsesTrustedWorkspacePermissionAuthorizer(t *testing.T) {
	var received RouteAuthorizationRequest
	runtime := authorizedAIChatRuntime(func(_ context.Context, request RouteAuthorizationRequest) error {
		received = request
		return nil
	})
	tool := NewNavigateTool("").ForkToolRuntime(runtime)
	_, err := tool.Invoke(t.Context(), "user-1", map[string]interface{}{
		"href": "/console/workflows",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if received.WorkspaceID != "workspace-1" || received.Href != "/console/workflows" {
		t.Fatalf("authorizer request = %#v, want trusted workspace workflow request", received)
	}
	if !slices.Contains(received.PermissionCodes, workspacemodel.WorkspacePermissionWorkflowView) {
		t.Fatalf("permission codes = %#v, want workflow.view", received.PermissionCodes)
	}
}

func TestNavigateToolRejectsUnauthorizedWorkspaceRoute(t *testing.T) {
	denied := errors.New("not allowed")
	tool := NewNavigateTool("").ForkToolRuntime(authorizedAIChatRuntime(
		func(context.Context, RouteAuthorizationRequest) error { return denied },
	))
	_, err := tool.Invoke(t.Context(), "user-1", map[string]interface{}{
		"href": "/console/files",
	}, nil, nil, nil)
	if !errors.Is(err, denied) {
		t.Fatalf("Invoke error = %v, want wrapped permission error", err)
	}
}

func TestNavigateToolRequiresTrustedWorkspaceAuthorization(t *testing.T) {
	tests := []struct {
		name    string
		runtime *tools.ToolRuntime
	}{
		{
			name:    "missing workspace",
			runtime: &tools.ToolRuntime{InvokeFrom: tools.ToolInvokeFromAIChat},
		},
		{
			name: "missing authorizer",
			runtime: &tools.ToolRuntime{
				InvokeFrom:        tools.ToolInvokeFromAIChat,
				RuntimeParameters: map[string]interface{}{"workspace_id": "workspace-1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewNavigateTool("").ForkToolRuntime(tt.runtime)
			_, err := tool.Invoke(t.Context(), "user-1", map[string]interface{}{
				"href": "/console/workflows",
			}, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), "permission denied") {
				t.Fatalf("Invoke error = %v, want permission denial", err)
			}
		})
	}
}

func TestNavigateToolAllowsOrganizationRouteWithoutWorkspace(t *testing.T) {
	tool := NewNavigateTool("").ForkToolRuntime(&tools.ToolRuntime{
		InvokeFrom: tools.ToolInvokeFromAIChat,
	})
	if _, err := tool.Invoke(t.Context(), "user-1", map[string]interface{}{
		"href": "/console/skills",
	}, nil, nil, nil); err != nil {
		t.Fatalf("Invoke returned error: %v", err)
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
	if href != "/console/agents/3806ca05-55c0-4380-a07a-e1cbf6fdcdd1/workflow" || label != "Workflow Detail" {
		t.Fatalf("href=%q label=%q, want workflow detail route", href, label)
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

func TestNormalizeConsoleRouteCanonicalizesAPIIndexRoutes(t *testing.T) {
	for _, tt := range []struct {
		href  string
		label string
	}{
		{href: "/console/agents/agent-1/api", label: "Agent API"},
		{href: "/console/workflows/workflow-1/api", label: "Workflow API"},
	} {
		normalized, label, err := normalizeConsoleRoute(tt.href)
		if err != nil {
			t.Fatalf("normalizeConsoleRoute(%q) returned error: %v", tt.href, err)
		}
		want := tt.href + "/keys"
		if normalized != want || label != tt.label {
			t.Fatalf(
				"normalizeConsoleRoute(%q) = (%q, %q), want (%q, %q)",
				tt.href,
				normalized,
				label,
				want,
				tt.label,
			)
		}
	}
}

func TestNormalizeConsoleRouteAllowsWorkflowDetailRoutes(t *testing.T) {
	for _, tt := range []struct {
		href  string
		label string
	}{
		{href: "/console/workflows/workflow-1", label: "Workflow Detail"},
		{href: "/console/workflows/workflow-1/logs", label: "Workflow Logs"},
		{href: "/console/workflows/workflow-1/api/keys", label: "Workflow API"},
		{href: "/console/workflows/workflow-1/api/docs", label: "Workflow API"},
		{href: "/console/workflows/workflow-1/batch-test", label: "Workflow Batch Test"},
	} {
		normalized, label, err := normalizeConsoleRoute(tt.href)
		if err != nil {
			t.Fatalf("normalizeConsoleRoute(%q) returned error: %v", tt.href, err)
		}
		if normalized != tt.href || label != tt.label {
			t.Fatalf(
				"normalizeConsoleRoute(%q) = (%q, %q), want (%q, %q)",
				tt.href,
				normalized,
				label,
				tt.href,
				tt.label,
			)
		}
	}
}

func TestNormalizeConsoleRouteRejectsExternalAndUnknownRoutes(t *testing.T) {
	for _, href := range []string{
		"https://example.com/console/files",
		"//example.com/console/files",
		"/console/files/../settings",
		"/admin",
		"/console/unknown",
		"/console/settings",
		"/console/db/database-1/table",
		"/console/files/file-1",
		"/console/integrations",
		"/console/workflows/workflow-1/api/unknown",
	} {
		if _, _, err := normalizeConsoleRoute(href); err == nil {
			t.Fatalf("normalizeConsoleRoute(%q) returned nil error, want rejection", href)
		}
	}
}
