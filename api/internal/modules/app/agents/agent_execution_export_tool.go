package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func newAgentExecutionExportTool(gateway agentExecutionGateway, persister agentExecutionArtifactPersister, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend string) skillloop.RuntimeTool {
	return skillloop.RuntimeTool{
		SkillID: "agent_execution",
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{
			Name:        "workspace_export_file",
			Description: "Persist a completed file from the isolated Linux workspace and make it available as a downloadable file card. Call this after creating the result file; never present a workspace path as a download URL.",
			Parameters: map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{"path": map[string]any{"type": "string", "maxLength": 256, "description": "Relative path of the completed workspace file"}},
				"required":   []string{"path"},
			},
		}},
		Handler: func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
			return exportAgentExecutionFile(ctx, gateway, persister, scope, agentID, conversationID, messageID, profileID, backend, call)
		},
	}
}

func exportAgentExecutionFile(ctx context.Context, gateway agentExecutionGateway, persister agentExecutionArtifactPersister, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend string, call adapter.ToolCall) skillloop.RuntimeToolResult {
	failed := func(code string, err error) skillloop.RuntimeToolResult {
		return skillloop.RuntimeToolResult{
			Status: "failed", Result: map[string]interface{}{"error_code": code},
			ModelResult: map[string]interface{}{"error_code": code}, Error: err,
		}
	}
	var args struct {
		Path string `json:"path"`
	}
	decoder := json.NewDecoder(strings.NewReader(call.Function.Arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return failed("invalid_arguments", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return failed("invalid_arguments", fmt.Errorf("unexpected trailing export arguments"))
	}
	filePath := strings.TrimSpace(args.Path)
	if filePath == "" || len(filePath) > 256 || strings.ContainsAny(filePath, "\\\x00") || path.IsAbs(filePath) || path.Clean(filePath) != filePath || filePath == "." || filePath == ".." || strings.HasPrefix(filePath, "../") {
		return failed("invalid_path", fmt.Errorf("agent execution export path is invalid"))
	}
	if persister == nil {
		return failed("artifact_store_unavailable", fmt.Errorf("agent execution artifact store is unavailable"))
	}
	operationCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	binding, err := readyAgentExecutionBinding(operationCtx, gateway, scope, agentID, conversationID, profileID, backend)
	if err != nil {
		return failed("binding_unavailable", err)
	}
	if binding.Recreated {
		return failed("workspace_recreated", fmt.Errorf("agent execution workspace was recreated and previous files are unavailable"))
	}
	capabilities, err := gateway.Capabilities(operationCtx, scope, binding.ID)
	if err != nil || !capabilities.FileTransfer || capabilities.NetworkMode != "disabled" || capabilities.MaxDownloadBytes <= 0 {
		return failed("capability_denied", fmt.Errorf("offline agent execution download is unavailable"))
	}
	data, err := gateway.DownloadFile(operationCtx, scope, binding.ID, filePath)
	if err != nil || int64(len(data)) > capabilities.MaxDownloadBytes || len(data) > agentExecutionFileLimit {
		return failed("download_failed", fmt.Errorf("agent execution result file is missing or exceeds the limit"))
	}
	artifact, err := persister.Persist(operationCtx, scope, conversationID, filePath, data)
	if err != nil {
		return failed("persist_failed", err)
	}
	if len(artifact) == 0 {
		return failed("persist_failed", fmt.Errorf("agent execution artifact metadata is missing"))
	}
	artifact["conversation_id"] = conversationID
	artifact["message_id"] = messageID
	artifact["skill_id"] = "agent_execution"
	artifact["tool_name"] = "workspace_export_file"
	artifact["invocation_id"] = call.ID
	return skillloop.RuntimeToolResult{
		Status:      "success",
		Arguments:   map[string]interface{}{"path_bytes": len(filePath)},
		Result:      map[string]interface{}{"file_id": artifact["file_id"], "size": artifact["size"]},
		ModelResult: map[string]interface{}{"file": artifact},
		Artifacts:   []map[string]interface{}{artifact},
	}
}
