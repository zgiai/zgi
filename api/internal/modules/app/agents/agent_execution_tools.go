package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/dto"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

const (
	agentExecutionMaxCommandBytes = 8192
	agentExecutionMaxOutputBytes  = 32 << 10
	agentExecutionMaxTimeout      = 60 * time.Second
)

func (h *AgentsHandler) attachConsoleAgentExecutionTools(runConfig *runtimeservice.RunConfig, scope runtimeservice.Scope, agentID uuid.UUID, cfg dto.AgentConfigResponse) error {
	if runConfig == nil || cfg.ExecutionRuntime == nil || !cfg.ExecutionRuntime.Enabled {
		return nil
	}
	if err := h.executionApprovals.validate(scope.OrganizationID.String(), cfg.ExecutionRuntime); err != nil {
		return err
	}
	if h.executionGateway == nil {
		return apperror.New(AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation("agent_execution.gateway_unavailable"))
	}
	profileID := strings.TrimSpace(cfg.ExecutionRuntime.ProfileID)
	backend := h.executionApprovals.Profiles[profileID].Backend
	gateway := h.executionGateway
	runConfig.RuntimeToolFactory = func(prepared *runtimeservice.PreparedChat) []skillloop.RuntimeTool {
		if prepared == nil || prepared.Conversation == nil || prepared.Message == nil ||
			prepared.Caller.Source != runtimemodel.ConversationSourceConsole ||
			prepared.Caller.Type != runtimemodel.ConversationCallerAgent ||
			prepared.Caller.ID == nil || *prepared.Caller.ID != agentID ||
			prepared.Scope.OrganizationID != scope.OrganizationID ||
			prepared.Scope.AccountID != scope.AccountID {
			return nil
		}
		return []skillloop.RuntimeTool{newAgentExecutionShellTool(gateway, agentExecutionScope{
			OrganizationID: scope.OrganizationID.String(), SubjectID: scope.AccountID.String(),
		}, agentID.String(), prepared.Conversation.ID.String(), prepared.Message.ID.String(), profileID, backend)}
	}
	return nil
}

func newAgentExecutionShellTool(gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend string) skillloop.RuntimeTool {
	return skillloop.RuntimeTool{
		SkillID: "agent_execution",
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{
			Name:        "shell_exec",
			Description: "Run a command in the approved, isolated Linux workspace for this console conversation. The workspace may be released after inactivity. Network access is disabled unless its approved profile explicitly allows it.",
			Parameters: map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"command":         map[string]any{"type": "string", "description": "Shell command to run in the workspace", "maxLength": agentExecutionMaxCommandBytes},
					"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 60},
				},
				"required": []string{"command"},
			},
		}},
		Handler: func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
			return executeAgentShellTool(ctx, gateway, scope, agentID, conversationID, messageID, profileID, backend, call)
		},
	}
}

func executeAgentShellTool(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend string, call adapter.ToolCall) skillloop.RuntimeToolResult {
	failed := func(code string, err error) skillloop.RuntimeToolResult {
		return skillloop.RuntimeToolResult{
			Status: "failed", Result: map[string]interface{}{"error_code": code},
			ModelResult: map[string]interface{}{"error_code": code}, Error: err,
		}
	}
	var args struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	decoder := json.NewDecoder(strings.NewReader(call.Function.Arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return failed("invalid_arguments", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return failed("invalid_arguments", fmt.Errorf("unexpected trailing shell arguments"))
	}
	if strings.TrimSpace(args.Command) == "" || len(args.Command) > agentExecutionMaxCommandBytes || strings.ContainsRune(args.Command, '\x00') {
		return failed("invalid_command", fmt.Errorf("shell command is empty or exceeds its limit"))
	}
	if args.TimeoutSeconds == 0 {
		args.TimeoutSeconds = 30
	}
	if args.TimeoutSeconds < 1 || args.TimeoutSeconds > 60 {
		return failed("invalid_timeout", fmt.Errorf("shell timeout is out of range"))
	}
	timeout := time.Duration(args.TimeoutSeconds) * time.Second
	operationCtx, cancel := context.WithTimeout(ctx, timeout+15*time.Second)
	defer cancel()
	binding, err := gateway.CreateBinding(operationCtx, scope, agentID, conversationID, profileID, backend)
	if err != nil {
		return failed("binding_failed", err)
	}
	if binding.Status != "active" {
		return failed("binding_inactive", fmt.Errorf("agent execution binding is %s", binding.Status))
	}
	capabilities, err := gateway.Capabilities(operationCtx, scope, binding.ID)
	if err != nil {
		return failed("capability_check_failed", err)
	}
	if !capabilities.Shell || capabilities.NetworkMode != "disabled" {
		return failed("capability_denied", fmt.Errorf("agent execution profile does not provide an offline shell"))
	}
	callID := strings.TrimSpace(call.ID)
	if callID == "" {
		callID = uuid.NewString()
	}
	job, err := gateway.SubmitJob(operationCtx, scope, binding.ID, "tool:"+messageID+":"+callID, "/bin/sh", []string{"-lc", args.Command}, timeout)
	if err != nil {
		return failed("job_submission_failed", err)
	}
	for job.Status == "queued" || job.Status == "running" {
		select {
		case <-operationCtx.Done():
			cancelCtx, cancelJob := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_ = gateway.CancelJob(cancelCtx, scope, job.ID, job.Generation)
			cancelJob()
			return failed("job_interrupted", operationCtx.Err())
		case <-time.After(350 * time.Millisecond):
		}
		job, err = gateway.GetJob(operationCtx, scope, job.ID)
		if err != nil {
			return failed("job_status_failed", err)
		}
	}
	stdout, stderr, truncated, err := readAgentExecutionOutput(operationCtx, gateway, scope, job.ID)
	if err != nil {
		return failed("job_output_failed", err)
	}
	result := map[string]interface{}{"job_id": job.ID, "status": job.Status, "exit_code": job.ExitCode, "truncated": truncated}
	return skillloop.RuntimeToolResult{
		Status: "success", Arguments: map[string]interface{}{"command_bytes": len(args.Command)}, Result: result,
		ModelResult: map[string]interface{}{"job_id": job.ID, "status": job.Status, "exit_code": job.ExitCode, "stdout": stdout, "stderr": stderr, "truncated": truncated},
	}
}

func readAgentExecutionOutput(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, jobID string) (string, string, bool, error) {
	var stdout, stderr strings.Builder
	after := int64(-1)
	truncated := false
	for {
		chunks, err := gateway.JobOutput(ctx, scope, jobID, after, 32)
		if err != nil {
			return "", "", false, err
		}
		if len(chunks) == 0 {
			break
		}
		for _, chunk := range chunks {
			if chunk.Sequence <= after {
				return "", "", false, fmt.Errorf("agent execution output sequence did not advance")
			}
			after = chunk.Sequence
			remaining := agentExecutionMaxOutputBytes - stdout.Len() - stderr.Len()
			if remaining <= 0 {
				truncated = true
				break
			}
			data := chunk.Data
			if len(data) > remaining {
				data = data[:remaining]
				truncated = true
			}
			if chunk.Stream == "stderr" {
				_, _ = stderr.Write(data)
			} else if chunk.Stream == "stdout" {
				_, _ = stdout.Write(data)
			}
		}
		if truncated || len(chunks) < 32 {
			break
		}
	}
	return strings.ToValidUTF8(stdout.String(), "�"), strings.ToValidUTF8(stderr.String(), "�"), truncated, nil
}
