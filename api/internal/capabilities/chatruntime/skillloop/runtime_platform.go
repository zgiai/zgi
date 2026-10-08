package skillloop

import (
	"context"
	"encoding/json"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func runtimePlatformToolBinding(resolved *skills.ResolvedSkills, loaded map[string]struct{}, toolSet *skills.NativeToolSet, name string) (skills.NativeToolBinding, bool) {
	if name == "" || isSkillMetaToolName(name) || nativeForbiddenProtocolTool(name) {
		return skills.NativeToolBinding{}, false
	}
	var binding skills.NativeToolBinding
	var found bool
	if toolSet != nil {
		binding, found = toolSet.ToolBindings[name]
	} else if skillID, ok := uniqueResolvedSkillForToolName(resolved, name); ok {
		binding, found = skills.NativeToolBinding{SkillID: skillID, ToolName: name}, true
	}
	if !found || binding.ToolName == skills.SkillScriptToolRun || isSkillMetaToolName(binding.ToolName) {
		return skills.NativeToolBinding{}, false
	}
	_, active := loaded[strings.ToLower(strings.TrimSpace(binding.SkillID))]
	return binding, active
}

// This bridge reenters the same synchronous tool pipeline as the parent loop.
// It cannot activate Skills, recurse into Shell, select arbitrary tools, or
// replace the live authorizer. Results remain ephemeral, traces stay governed.
func (r *Runner) runtimePlatformServices(req RunRequest, prepared *PreparedChat, resolved *skills.ResolvedSkills, loaded map[string]struct{}, currentToolSet func() *skills.NativeToolSet, toolCalls *int, skillCounts map[string]int, successful *[]SkillToolCallRef, traces *[]skills.SkillTrace) RuntimePlatformServices {
	execution := req.ExecutionContext
	// Copy the context so this capability never leaks into ordinary parent
	// tools. RuntimeTools are server-owned, not model-selectable policy data.
	execution.PlatformInteractiveApproval = false
	for _, tool := range req.RuntimeTools {
		if tool.BindPlatform != nil && tool.PlatformInteractiveApproval {
			execution.PlatformInteractiveApproval = true
			break
		}
	}
	return RuntimePlatformServices{
		AuthorizeSkill: func(ctx context.Context, skillID string) (bool, error) {
			if _, active := loaded[strings.ToLower(strings.TrimSpace(skillID))]; !active || req.AuthorizeSkillStep == nil {
				return false, nil
			}
			return req.AuthorizeSkillStep(ctx, skillID)
		},
		Model: req.PlatformModel,
		Tool: func(ctx context.Context, requestID, name string, arguments json.RawMessage) (RuntimePlatformToolResult, error) {
			denied := func() (RuntimePlatformToolResult, error) {
				return RuntimePlatformToolResult{}, apperror.New(catalog.CodePermissionDenied, apperror.WithOperation("runtime_platform.tool_scope"))
			}
			binding, ok := runtimePlatformToolBinding(resolved, loaded, currentToolSet(), name)
			if !ok || req.AuthorizeSkillStep == nil || ctx.Err() != nil {
				return denied()
			}
			allowed, err := req.AuthorizeSkillStep(ctx, binding.SkillID)
			if err != nil || !allowed {
				return denied()
			}
			var args map[string]interface{}
			if json.Unmarshal(arguments, &args) != nil || args == nil {
				return RuntimePlatformToolResult{}, apperror.New(catalog.CodeRequestInvalid, apperror.WithOperation("runtime_platform.tool_arguments"))
			}
			encoded, _ := json.Marshal(map[string]interface{}{"skill_id": binding.SkillID, "tool_name": binding.ToolName, "arguments": args})
			evidence := runtimeStateWithSuccessfulToolCalls(req, *successful)
			evidence[runtimeStateNativeAgentLoopKey] = req.NativeAgentLoop
			evidence[runtimeStateAllowPlanUpdateKey] = false
			evidence[runtimeStateAllowIntermediateAnswerKey] = false
			evidence[runtimeStateKnowledgeRetrievalHistoryKey] = knowledgeRetrievalHistoryForToolCalls(*successful)
			step := r.handleProgressiveSkillCall(ctx, prepared, resolved, adapter.ToolCall{ID: requestID, Function: adapter.FunctionCall{Name: skills.MetaToolCallSkillTool, Arguments: string(encoded)}}, execution, *toolCalls, skillCounts, loaded, evidence, 1, nil)
			*traces = append(*traces, step.trace)
			r.recordTrace(*traces, step.trace)
			r.logSkillTrace(ctx, prepared, step.trace)
			if step.usedTool {
				*toolCalls++
				incrementSkillToolCallCount(skillCounts, binding.SkillID)
				if step.trace.Status == "success" {
					*successful = append(*successful, SkillToolCallRef{SkillID: binding.SkillID, ToolName: binding.ToolName, ConnectorNative: step.trace.ConnectorNative, Arguments: copyStringAnyMap(step.trace.Arguments), Result: copyStringAnyMap(step.toolResult)})
				}
			}
			if err := prepared.presentationEventError(); err != nil {
				return RuntimePlatformToolResult{}, err
			}
			pending := RuntimePlatformToolResult{PendingGovernance: step.pendingGovernance, PendingConnector: step.pendingConnector, PendingApproval: step.pendingApproval, PendingQuestion: step.pendingQuestion, PendingClientAction: step.pendingClientAction, PendingUserInput: step.pendingUserInput}
			if pending.NeedsContinuation() {
				return pending, nil
			}
			// Only completed tool effects can become successful CLI receipts.
			if step.fatalErr != nil || step.recoverable || step.trace.Status != "success" {
				return RuntimePlatformToolResult{}, apperror.New(catalog.CodeResourceConflict, apperror.WithOperation("runtime_platform.tool_incomplete"))
			}
			data, err := json.Marshal(step.toolMessage.Content)
			if text, ok := step.toolMessage.Content.(string); ok && json.Valid([]byte(text)) {
				data = []byte(text)
			}
			if err != nil || len(data) > 32<<10 {
				return RuntimePlatformToolResult{}, apperror.New(catalog.CodeRequestInvalid, apperror.WithOperation("runtime_platform.tool_output"))
			}
			return RuntimePlatformToolResult{Data: data}, nil
		},
	}
}
