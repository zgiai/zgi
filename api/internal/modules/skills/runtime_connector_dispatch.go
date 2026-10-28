package skills

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
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
	runCtx = connectorexecution.WithPresentationLocale(runCtx, execCtx.PresentationLocale)
	if requester, parseErr := uuid.Parse(strings.TrimSpace(execCtx.ConnectorRequesterID)); identity.Interactive && parseErr == nil && requester != uuid.Nil {
		runCtx = connectorexecution.WithPresentRequester(runCtx, requester)
	}
	if !identity.Interactive && identity.Target.Type == core.TargetTypeDigitalHuman {
		runID, runErr := uuid.Parse(strings.TrimSpace(execCtx.ConnectorRunID))
		messageID, messageErr := uuid.Parse(strings.TrimSpace(execCtx.MessageID))
		if runErr != nil || messageErr != nil || runID == uuid.Nil || messageID == uuid.Nil {
			err = core.NewError(core.ErrorCodeAccessDenied, false, nil)
			invocation.Trace.Status = "error"
			invocation.Trace.Error, invocation.Trace.ErrorCode = skillTraceError(err)
			return invocation, err
		}
		runCtx = connectorexecution.WithBackgroundIdentity(runCtx, runID, messageID)
	}
	governanceParams := governanceRuntimeParameters(execCtx.RuntimeParameters)
	sessionGrants := governanceSessionGrants(execCtx.RuntimeParameters, governanceParams)
	result, err := runtime.connectorDispatcher.Dispatch(runCtx, dispatch.Invocation{
		Identity: identity, SessionGrants: sessionGrants, Input: arguments,
	})
	invocation.Trace.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		invocation.Trace.Status = "error"
		invocation.Trace.Error, invocation.Trace.ErrorCode = skillTraceError(err)
	}
	invocation.Messages = connectortools.MessagesForInvocation(result, identity, err)
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
