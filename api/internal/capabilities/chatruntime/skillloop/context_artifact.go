package skillloop

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const contextArtifactToolName = "read_context_artifact"

func contextArtifactTool() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.Function{
			Name:        contextArtifactToolName,
			Description: "Read the complete original content of an oversized tool result referenced by an agent-context artifact receipt. Use this only when a projected or compacted result omitted evidence needed for the current task.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"artifact_ref": map[string]interface{}{
						"type":        "string",
						"description": "The exact agent-context://tool-results/... reference from a projected or compacted tool result receipt.",
						"minLength":   1,
						"maxLength":   512,
					},
				},
				"required":             []string{"artifact_ref"},
				"additionalProperties": false,
			},
		},
	}
}

func appendContextArtifactTool(tools []adapter.Tool, enabled bool) []adapter.Tool {
	if !enabled {
		return tools
	}
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool.Function.Name), contextArtifactToolName) {
			return tools
		}
	}
	return append(tools, contextArtifactTool())
}

func (r *Runner) handleReadContextArtifactCall(ctx context.Context, callID string, args map[string]interface{}) skillStepResult {
	artifactRef := stringArg(args, "artifact_ref")
	if artifactRef == "" {
		return contextArtifactReadFailure(callID, fmt.Errorf("artifact_ref is required"))
	}
	if r == nil || r.ContextManager == nil {
		return contextArtifactReadFailure(callID, fmt.Errorf("context artifact reader is unavailable"))
	}
	result, err := r.ContextManager.ReadContextArtifact(ctx, artifactRef)
	if err != nil {
		return contextArtifactReadFailure(callID, err)
	}
	trace := skills.SkillTrace{
		Kind:     "context_artifact_read",
		ToolName: contextArtifactToolName,
		Status:   "success",
		Result: map[string]interface{}{
			"returned_tokens": result.ReturnedTokens,
			"total_tokens":    result.TotalTokens,
			"complete":        true,
		},
	}
	return successfulSkillStep(trace, contextArtifactToolResultMessage(callID, result), false, false)
}

func contextArtifactToolResultMessage(callID string, result contextmgr.ContextArtifactResult) adapter.Message {
	content, err := contextmgr.FormatContextArtifactToolResult(result)
	if err != nil {
		return skills.ToolResultMessage(callID, map[string]interface{}{"status": "error", "error": "failed to encode context artifact metadata"})
	}
	return adapter.Message{
		Role:       "tool",
		ToolCallID: callID,
		Content:    content,
	}
}

func contextArtifactReadFailure(callID string, err error) skillStepResult {
	trace := failedSkillTrace("context_artifact_read", contextArtifactToolName, err)
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status":      "error",
		"error":       "context artifact could not be read",
		"next_action": "Use the exact artifact_ref from the tool-result receipt. If the artifact is unavailable, continue from the retained preview and state the limitation.",
	}), false, false)
}
