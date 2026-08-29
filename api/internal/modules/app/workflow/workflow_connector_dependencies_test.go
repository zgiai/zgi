package workflow

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestExtractWorkflowConnectorDependenciesUsesProviderActionIdentity(t *testing.T) {
	connectionID := "11111111-1111-4111-8111-111111111111"
	graph := map[string]interface{}{"nodes": []interface{}{
		map[string]interface{}{"id": "plain", "data": map[string]interface{}{"type": "llm"}},
		map[string]interface{}{"id": "unconfigured", "data": map[string]interface{}{
			"type": "tools", "provider_type": "connector", "provider_id": "wecom", "tool_name": "get_wecom_user",
		}},
		map[string]interface{}{"id": "send", "data": map[string]interface{}{
			"type": "tools", "title": "Notify owner", "provider_type": "connector", "provider_id": "WeCom",
			"tool_name": "send_wecom_user_message", "connection_id": connectionID,
		}},
	}}

	got, err := extractWorkflowConnectorDependencies(t.Context(), graph, func(_ context.Context, integrationID, toolName string) (workflowConnectorAction, error) {
		if integrationID != "wecom" || toolName != "send_wecom_user_message" {
			t.Fatalf("resolver input = %q/%q", integrationID, toolName)
		}
		return workflowConnectorAction{ActionID: "wecom.message.send_user", AccessMode: "write"}, nil
	})
	if err != nil {
		t.Fatalf("extract dependencies: %v", err)
	}
	want := []workflowConnectorDependency{{
		NodeID: "send", NodeTitle: "Notify owner", IntegrationID: "wecom", ToolName: "send_wecom_user_message",
		ActionID: "wecom.message.send_user", ConnectionID: connectionID, AccessMode: "write",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dependencies = %#v, want %#v", got, want)
	}
}

func TestExtractWorkflowConnectorDependenciesRejectsInvalidConfiguredNode(t *testing.T) {
	for _, test := range []struct {
		name  string
		graph map[string]interface{}
		want  string
	}{
		{
			name: "invalid connection",
			graph: map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"id": "node-1", "data": map[string]interface{}{
				"provider_type": "connector", "provider_id": "wecom", "tool_name": "get_wecom_user", "connection_id": "not-a-uuid",
			}}}},
			want: "invalid connection_id",
		},
		{
			name: "duplicate node",
			graph: map[string]interface{}{"nodes": []interface{}{
				map[string]interface{}{"id": "node-1", "data": map[string]interface{}{"provider_type": "connector"}},
				map[string]interface{}{"id": "node-1", "data": map[string]interface{}{"provider_type": "connector"}},
			}},
			want: "duplicated",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := extractWorkflowConnectorDependencies(t.Context(), test.graph, func(context.Context, string, string) (workflowConnectorAction, error) {
				return workflowConnectorAction{ActionID: "wecom.user.get", AccessMode: "read"}, nil
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestClearWorkflowConnectorConnectionOnlyTouchesSelectedNodes(t *testing.T) {
	target := "11111111-1111-4111-8111-111111111111"
	other := "22222222-2222-4222-8222-222222222222"
	graph := map[string]interface{}{"nodes": []interface{}{
		map[string]interface{}{"id": "selected", "data": map[string]interface{}{"provider_type": "connector", "connection_id": target}},
		map[string]interface{}{"id": "same-unselected", "data": map[string]interface{}{"provider_type": "connector", "connection_id": target}},
		map[string]interface{}{"id": "other", "data": map[string]interface{}{"provider_type": "connector", "connection_id": other}},
	}}
	changed, err := clearWorkflowConnectorConnection(graph, target, map[string]struct{}{"selected": {}})
	if err != nil || !changed {
		t.Fatalf("clear connection changed=%v error=%v", changed, err)
	}
	nodes, _ := workflowConnectorGraphNodes(graph)
	if got := workflowString(workflowNodeData(nodes[0]), "connection_id"); got != "" {
		t.Fatalf("selected connection_id = %q", got)
	}
	if got := workflowString(workflowNodeData(nodes[1]), "connection_id"); got != target {
		t.Fatalf("unselected connection_id = %q", got)
	}
	if got := workflowString(workflowNodeData(nodes[2]), "connection_id"); got != other {
		t.Fatalf("other connection_id = %q", got)
	}
}

func TestValidateWorkflowConnectorNodesConfiguredRejectsDetachedDraft(t *testing.T) {
	graph := map[string]interface{}{"nodes": []interface{}{
		map[string]interface{}{"id": "detached", "data": map[string]interface{}{
			"provider_type": "connector", "provider_id": "wecom", "tool_name": "get_wecom_user",
		}},
	}}
	if err := validateWorkflowConnectorNodesConfigured(graph); err == nil || !strings.Contains(err.Error(), "reconnected before publishing") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestSanitizeGraphRemovesOnlyConnectorConnectionHandle(t *testing.T) {
	graph := map[string]interface{}{"nodes": []interface{}{
		map[string]interface{}{"id": "connector", "data": map[string]interface{}{
			"provider_type": "connector", "connection_id": "organization-local-connection",
		}},
		map[string]interface{}{"id": "unrelated", "data": map[string]interface{}{
			"type": "custom", "connection_id": "domain-data-not-an-auth-handle",
		}},
	}}

	SanitizeGraph(graph)
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		t.Fatalf("read sanitized graph nodes: %v", err)
	}
	if got := workflowString(workflowNodeData(nodes[0]), "connection_id"); got != "" {
		t.Fatalf("exported connector connection_id = %q, want removed", got)
	}
	if got := workflowString(workflowNodeData(nodes[1]), "connection_id"); got != "domain-data-not-an-auth-handle" {
		t.Fatalf("unrelated connection_id = %q, want retained", got)
	}
}

func TestCheckDependenciesExplainsConnectorReconfiguration(t *testing.T) {
	graph := map[string]interface{}{"nodes": []interface{}{
		map[string]interface{}{"id": "send", "data": map[string]interface{}{
			"type": "tools", "provider_type": "connector", "provider_id": "wecom", "provider_name": "企业微信",
		}},
	}}
	warnings := CheckDependencies(graph)
	if len(warnings) != 1 || warnings[0].Type != "connector_requires_config" || warnings[0].NodeID != "send" {
		t.Fatalf("connector import warnings = %#v", warnings)
	}
}
