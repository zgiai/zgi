package graph_engine

import (
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

func (e *WorkflowEngine) workflowRunSpanAttributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 8)
	attrs = appendWorkflowSystemAttributes(attrs, e.workflowSystemVariables())
	attrs = append(attrs, attribute.Int("zgi.workflow_node_count", len(e.steps)))
	return attrs
}

func (e *WorkflowEngine) workflowNodeSpanAttributes(nodeID string, nodeType shared.NodeType) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 9)
	attrs = appendWorkflowSystemAttributes(attrs, e.workflowSystemVariables())
	attrs = appendIfNotEmpty(attrs, "zgi.node_id", nodeID)
	attrs = appendIfNotEmpty(attrs, "zgi.node_type", string(nodeType))
	return attrs
}

func (e *WorkflowEngine) workflowSystemVariables() *entities.SystemVariable {
	if e == nil || e.runtimeState == nil || e.runtimeState.VariablePool == nil {
		return nil
	}
	return e.runtimeState.VariablePool.SystemVariables
}

func appendWorkflowSystemAttributes(attrs []attribute.KeyValue, sys *entities.SystemVariable) []attribute.KeyValue {
	if sys == nil {
		return attrs
	}
	attrs = appendIfNotEmpty(attrs, "zgi.tenant_id", sys.TenantID)
	attrs = appendIfNotEmpty(attrs, "zgi.workspace_id", sys.WorkspaceID)
	attrs = appendIfNotEmpty(attrs, "zgi.organization_id", sys.OrganizationID)
	attrs = appendIfNotEmpty(attrs, "zgi.app_id", sys.AppID)
	attrs = appendIfNotEmpty(attrs, "zgi.workflow_id", sys.WorkflowID)
	attrs = appendIfNotEmpty(attrs, "zgi.workflow_run_id", sys.WorkflowRunID)
	attrs = appendIfNotEmpty(attrs, "zgi.workflow_type", sys.WorkflowType)
	return attrs
}

func (e *WorkflowEngine) langfuseTraceAttributes() []attribute.KeyValue {
	sys := e.workflowSystemVariables()
	if sys == nil {
		return nil
	}

	attrs := observability.LangfuseRuntimeAttributes()
	attrs = append(attrs, attribute.String("langfuse.trace.name", "workflow.run"))
	attrs = appendIfNotEmpty(attrs, "langfuse.user.id", sys.UserID)
	attrs = appendIfNotEmpty(attrs, "langfuse.session.id", sys.ConversationID)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.tenant_id", sys.TenantID)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.workspace_id", sys.WorkspaceID)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.organization_id", sys.OrganizationID)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.app_id", sys.AppID)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.workflow_id", sys.WorkflowID)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.workflow_run_id", sys.WorkflowRunID)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.workflow_type", sys.WorkflowType)

	tags := []string{"zgi", "workflow"}
	if sys.WorkflowType != "" {
		tags = append(tags, "workflow_type:"+sys.WorkflowType)
	}
	attrs = append(attrs, attribute.StringSlice("langfuse.trace.tags", tags))
	return attrs
}

func (e *WorkflowEngine) langfuseWorkflowNodeTraceAttributes(nodeID string, nodeType shared.NodeType) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.node_id", nodeID)
	attrs = appendIfNotEmpty(attrs, "langfuse.trace.metadata.node_type", string(nodeType))
	if nodeType != "" {
		attrs = append(attrs, attribute.StringSlice("langfuse.trace.tags", []string{"node_type:" + string(nodeType)}))
	}
	return attrs
}

func appendIfNotEmpty(attrs []attribute.KeyValue, key string, value string) []attribute.KeyValue {
	if value == "" {
		return attrs
	}
	return append(attrs, attribute.String(key, value))
}
