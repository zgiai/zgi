package skills

import (
	"context"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func (r *Runtime) enrichToolGovernanceArguments(ctx context.Context, toolDef SkillToolDefinition, arguments map[string]interface{}, execCtx ExecutionContext) map[string]interface{} {
	if r == nil || r.engine == nil {
		return arguments
	}
	enriched, err := r.engine.EnrichGovernanceArguments(ctx, tools.InvokeRequest{
		ProviderType:      toolDef.ProviderType,
		ProviderID:        toolDef.ProviderID,
		ToolName:          toolDef.Name,
		TenantID:          execCtx.OrganizationID,
		UserID:            execCtx.UserID,
		Parameters:        arguments,
		ConversationID:    execCtx.ConversationID,
		AppID:             execCtx.AppID,
		MessageID:         execCtx.MessageID,
		InvokeFrom:        normalizeToolInvokeFrom(execCtx.InvokeFrom),
		RuntimeParameters: copyStringAnyMap(execCtx.RuntimeParameters),
	})
	if err != nil || enriched == nil {
		return arguments
	}
	return enriched
}

func appendGovernanceRewriteObservation(messages []tools.ToolInvokeMessage, decision toolgovernance.Decision, rewrite map[string]interface{}) []tools.ToolInvokeMessage {
	if len(rewrite) == 0 || decision.Status != toolgovernance.DecisionStatusAllowed {
		return messages
	}
	assets := decision.ExpectedAssets
	if len(assets) == 0 {
		assets = decision.Assets
	}
	if len(assets) == 0 {
		return messages
	}
	out := append([]tools.ToolInvokeMessage{}, messages...)
	out = append(out, tools.ToolInvokeMessage{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":                  "completed",
			"kind":                    "resolved_target_observation",
			"resolved_assets":         governanceAssetObservationPayload(assets),
			"resolved_asset_count":    len(assets),
			"resolved_asset_guidance": "The user's request target has been resolved to resolved_assets. Treat these assets as the only target for this turn. Answer with these asset names and the tool content only; do not mention internal resolution, governance, rewrites, redirects, mismatched IDs, or other visible files unless the user asks for debugging or comparison.",
		},
	})
	return out
}

func governanceAssetObservationPayload(assets []toolgovernance.AssetRef) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(assets))
	for _, asset := range assets {
		item := map[string]interface{}{
			"id":   strings.TrimSpace(asset.ID),
			"type": strings.TrimSpace(asset.Type),
			"name": strings.TrimSpace(asset.Name),
		}
		if workspaceID := strings.TrimSpace(asset.WorkspaceID); workspaceID != "" {
			item["workspace_id"] = workspaceID
		}
		if source := strings.TrimSpace(asset.Source); source != "" {
			item["source"] = source
		}
		if len(asset.Metadata) > 0 {
			item["metadata"] = copyStringAnyMap(asset.Metadata)
		}
		out = append(out, item)
	}
	return out
}

func (r *Runtime) preflightToolGovernance(
	ctx context.Context,
	doc SkillDocument,
	toolDef SkillToolDefinition,
	arguments map[string]interface{},
	execCtx ExecutionContext,
	callID string,
) (toolgovernance.Decision, bool, *ToolInvocationResult, error) {
	if r == nil || r.governance == nil || toolDef.Governance == nil {
		return toolgovernance.Decision{}, false, nil, nil
	}
	start := time.Now()
	decision, err := r.governance.DecideSkillTool(ctx, ToolGovernanceRequest{
		Manifest:         *toolDef.Governance,
		SkillID:          doc.Metadata.ID,
		ToolName:         toolDef.Name,
		ProviderType:     toolDef.ProviderType,
		ProviderID:       toolDef.ProviderID,
		Arguments:        copyStringAnyMap(arguments),
		ExecutionContext: execCtx,
	})
	trace := SkillTrace{
		Kind:       "tool_governance",
		SkillID:    doc.Metadata.ID,
		ToolName:   toolDef.Name,
		Status:     string(decision.Status),
		DurationMS: time.Since(start).Milliseconds(),
		Arguments:  summarizeArguments(arguments),
		Governance: &decision,
		Result:     governanceTraceResult(decision),
	}
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		return decision, true, &ToolInvocationResult{Trace: trace}, err
	}
	if decision.Status == toolgovernance.DecisionStatusNeedsApproval && decision.RequiresApproval {
		frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
			CorrelationID:  decision.CorrelationID,
			Manifest:       decision.Manifest,
			SkillID:        doc.Metadata.ID,
			ToolName:       toolDef.Name,
			ProviderType:   string(toolDef.ProviderType),
			ProviderID:     toolDef.ProviderID,
			Arguments:      copyStringAnyMap(arguments),
			Assets:         decision.Assets,
			ExpectedAssets: decision.ExpectedAssets,
			Now:            start,
		})
		decision.FrozenInvocation = &frozen
		if decision.ApprovalEvent != nil {
			decision.ApprovalEvent.FrozenInvocation = &frozen
		}
		trace.Governance = &decision
		trace.Result = governanceTraceResult(decision)
	}
	if decision.Status == toolgovernance.DecisionStatusAllowed {
		return decision, true, nil, nil
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		callID = "call_" + toolDef.Name
	}
	return decision, true, &ToolInvocationResult{
		Trace:       trace,
		ToolMessage: ToolResultMessage(callID, governanceToolFeedback(decision)),
	}, nil
}

func governanceTraceResult(decision toolgovernance.Decision) map[string]interface{} {
	result := governanceToolFeedback(decision)
	if decision.ApprovalEvent != nil {
		result["approval_event"] = decision.ApprovalEvent
	}
	return result
}

func governanceToolFeedback(decision toolgovernance.Decision) map[string]interface{} {
	feedback := copyStringAnyMap(decision.ModelFeedback)
	if feedback == nil {
		feedback = map[string]interface{}{}
	}
	feedback["status"] = string(decision.Status)
	feedback["reason"] = strings.TrimSpace(decision.Reason)
	feedback["correlation_id"] = strings.TrimSpace(decision.CorrelationID)
	feedback["requires_approval"] = decision.RequiresApproval
	feedback["instruction"] = governanceInstruction(decision)
	return map[string]interface{}{
		"governance": feedback,
	}
}

func governanceInstruction(decision toolgovernance.Decision) string {
	switch decision.Status {
	case toolgovernance.DecisionStatusNeedsApproval:
		return "The tool was not executed. Explain that user approval is required and wait for approval before retrying this action."
	case toolgovernance.DecisionStatusNeedsResolution:
		if len(decision.ExpectedAssets) > 0 {
			return "The tool was not executed because the tool arguments did not match the resolved target asset. Retry the same tool with the exact ID from expected_assets; do not ask the user to clarify unless expected_assets is ambiguous or empty."
		}
		return "The tool was not executed. Ask the user to clarify the target asset or resolve the asset reference before retrying."
	case toolgovernance.DecisionStatusDenied:
		code, _ := decision.ModelFeedback["authorization_code"].(string)
		code = strings.TrimSpace(code)
		if code == agentToolNotPreauthorizedCode {
			return "The tool was not executed because this action is not preauthorized in the non-interactive Agent runtime. Do not retry with unchanged arguments; choose an already authorized tool or explain that this action is unavailable."
		}
		if strings.HasPrefix(code, "agent_") {
			return "The tool was not executed because the current Agent configuration does not authorize it. Do not retry with unchanged arguments; choose an authorized bound resource or explain which Agent binding must be updated."
		}
		return "The tool was not executed. Explain the denial and continue with a safe alternative."
	case toolgovernance.DecisionStatusBlocked:
		return "The tool was not executed. Explain why the action is blocked and continue without this tool."
	default:
		return "Continue with the tool result."
	}
}
