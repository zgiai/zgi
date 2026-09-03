package skills

import (
	"context"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func (runtime *Runtime) WithConnectorDispatcher(dispatcher dispatch.Dispatcher) *Runtime {
	runtime.connectorDispatcher = dispatcher
	return runtime
}

func (runtime *Runtime) callConnectorTool(ctx context.Context, doc SkillDocument, tool SkillToolDefinition, arguments map[string]any, execCtx ExecutionContext, callID string, identity dispatch.Identity) (*ToolInvocationResult, error) {
	arguments = dispatch.InputWithDefaults(tool.InputSchema, arguments)
	trace := SkillTrace{Kind: "tool_call", SkillID: doc.Metadata.ID, ToolName: tool.Name, Status: "success", ConnectorNative: true, Arguments: summarizeToolArguments(tool, arguments)}
	invocation := &ToolInvocationResult{Trace: trace}
	if runtime.connectorDispatcher == nil || execCtx.ConnectorActionResolver == nil {
		err := core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
		invocation.Trace.Status = "error"
		invocation.Trace.Error, invocation.Trace.ErrorCode = skillTraceError(err)
		return invocation, err
	}
	if err := validateSkillToolArguments(doc.Metadata.ID, tool, arguments); err != nil {
		invocation.Trace.Status = "error"
		invocation.Trace.Error, invocation.Trace.ErrorCode = skillTraceError(err)
		return invocation, err
	}
	persisted, err := execCtx.ConnectorActionResolver(ctx, identity, arguments)
	if err != nil {
		invocation.Trace.Status = "error"
		invocation.Trace.Error, invocation.Trace.ErrorCode = skillTraceError(err)
		return invocation, err
	}
	identity = persisted
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(docTimeoutSeconds(doc))*time.Second)
	defer cancel()
	start := time.Now()
	result, err := runtime.connectorDispatcher.Dispatch(runCtx, dispatch.Invocation{Identity: identity, Input: arguments})
	invocation.Trace.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		invocation.Trace.Status = "error"
		invocation.Trace.Error, invocation.Trace.ErrorCode = skillTraceError(err)
	}
	invocation.Messages = connectortools.MessagesForInvocation(result, identity)
	if err == nil {
		pending, handled, freezeErr := freezeConnectorConfirmation(ctx, tool, &identity, arguments, execCtx, invocation.Messages)
		if freezeErr != nil {
			invocation.Messages = nil
			invocation.Trace.Status = "error"
			invocation.Trace.Error, invocation.Trace.ErrorCode = skillTraceError(freezeErr)
			return invocation, freezeErr
		}
		if handled {
			invocation.ConnectorConfirmation = pending
			invocation.Trace.Status = "pending_confirmation"
		}
	}
	if strings.TrimSpace(callID) == "" {
		callID = "call_" + tool.Name
	}
	invocation.ToolMessage = llmadapter.Message{Role: "tool", ToolCallID: callID, Content: toolMessagesContent(invocation.Messages)}
	return invocation, err
}
