package workflowtest

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type RunCaseRequest struct {
	AgentID      string
	BatchID      string
	BatchItemID  string
	CaseSnapshot CaseSnapshot
}

type RunCaseResult struct {
	WorkflowRunID string
	Outputs       map[string]interface{}
}

type Runner interface {
	RunCase(ctx context.Context, req RunCaseRequest) (*RunCaseResult, error)
}

type WorkflowServiceRunner struct {
	WorkflowService interfaces.WorkflowService
	WorkspaceID     string
	AccountID       string
}

func (r *WorkflowServiceRunner) RunCase(ctx context.Context, req RunCaseRequest) (*RunCaseResult, error) {
	if r == nil || r.WorkflowService == nil {
		return nil, fmt.Errorf("workflow service runner is not configured")
	}
	textInputName := r.resolvePrimaryTextInputName(ctx, req.AgentID)
	turns := runnableCaseTurns(req.CaseSnapshot)
	var lastResult *RunCaseResult
	turnResults := make([]map[string]interface{}, 0, len(turns))
	for index, turn := range turns {
		result, err := r.runTurn(ctx, req.AgentID, turn, textInputName)
		if err != nil {
			return nil, err
		}
		lastResult = result
		turnResults = append(turnResults, map[string]interface{}{
			"turn_index":      index + 1,
			"content":         turn.Content,
			"workflow_run_id": result.WorkflowRunID,
			"outputs":         result.Outputs,
		})
	}
	if lastResult == nil {
		return &RunCaseResult{Outputs: map[string]interface{}{}}, nil
	}
	outputs := make(map[string]interface{}, len(lastResult.Outputs)+2)
	for key, value := range lastResult.Outputs {
		outputs[key] = value
	}
	outputs["turn_count"] = len(turns)
	outputs["turn_results"] = turnResults
	lastResult.Outputs = outputs
	return lastResult, nil
}

func runnableCaseTurns(snapshot CaseSnapshot) CaseTurns {
	turns := make(CaseTurns, 0, len(snapshot.Turns))
	for _, turn := range snapshot.Turns {
		content := strings.TrimSpace(turn.Content)
		if content == "" && len(turn.Attachments) == 0 {
			continue
		}
		role := strings.TrimSpace(turn.Role)
		if role == "" {
			role = "user"
		}
		turns = append(turns, CaseTurn{
			Role:        role,
			Content:     content,
			Attachments: turn.Attachments,
			Inputs:      turn.Inputs,
		})
	}
	if len(turns) == 0 && strings.TrimSpace(snapshot.Content) != "" {
		return CaseTurns{{Role: "user", Content: strings.TrimSpace(snapshot.Content)}}
	}
	return turns
}

func (r *WorkflowServiceRunner) resolvePrimaryTextInputName(ctx context.Context, agentID string) string {
	draft, err := r.WorkflowService.GetDraftWorkflow(ctx, agentID, true)
	if err != nil {
		return "input1"
	}
	var graph map[string]interface{}
	switch data := draft.(type) {
	case dto.WorkflowDetail:
		graph = data.Graph
	case *dto.WorkflowDetail:
		if data != nil {
			graph = data.Graph
		}
	case map[string]interface{}:
		if value, ok := data["graph"].(map[string]interface{}); ok {
			graph = value
		}
	}
	name := primaryTextInputNameFromGraph(graph)
	if name == "" {
		return "input1"
	}
	return name
}

func primaryTextInputNameFromGraph(graph map[string]interface{}) string {
	nodes, ok := graph["nodes"].([]interface{})
	if !ok {
		return ""
	}
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		data, ok := nodeMap["data"].(map[string]interface{})
		if !ok || data["type"] != "start" {
			continue
		}
		variables, ok := data["variables"].([]interface{})
		if !ok {
			return ""
		}
		for _, item := range variables {
			variable, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			varType, _ := variable["type"].(string)
			if varType != "" && varType != "text-input" && varType != "paragraph" && varType != "string" {
				continue
			}
			for _, key := range []string{"variable", "name"} {
				if name, ok := variable[key].(string); ok && strings.TrimSpace(name) != "" {
					return strings.TrimSpace(name)
				}
			}
		}
	}
	return ""
}

func (r *WorkflowServiceRunner) runTurn(ctx context.Context, agentID string, turn CaseTurn, textInputName string) (*RunCaseResult, error) {
	inputs := map[string]interface{}{
		"sys.query": turn.Content,
	}
	if textInputName != "" {
		inputs[textInputName] = turn.Content
	}
	for key, value := range turn.Inputs {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		inputs[name] = value
	}
	files := make([]dto.FileInfo, 0)
	fileInputs := make([]interface{}, 0)
	for _, attachment := range turn.Attachments {
		fileInfo := dto.FileInfo{
			Type:           attachment.Type,
			TransferMethod: attachment.TransferMethod,
			URL:            attachment.URL,
			UploadFileID:   attachment.UploadFileID,
		}
		files = append(files, fileInfo)
		fileInputs = append(fileInputs, map[string]interface{}{
			"type":            fileInfo.Type,
			"transfer_method": fileInfo.TransferMethod,
			"url":             fileInfo.URL,
			"upload_file_id":  fileInfo.UploadFileID,
			"name":            attachment.Name,
		})
	}
	if len(fileInputs) > 0 {
		inputs["#files#"] = fileInputs
		inputs["sys.files"] = fileInputs
	}
	runReq := &dto.DraftWorkflowRunRequest{
		Inputs:       inputs,
		UserID:       r.AccountID,
		ResponseMode: "blocking",
		Files:        files,
	}
	result, err := r.WorkflowService.RunDraftWorkflow(ctx, r.WorkspaceID, agentID, runReq, r.AccountID)
	if err != nil {
		return nil, err
	}
	return normalizeWorkflowRunResult(result), nil
}

func normalizeWorkflowRunResult(result interface{}) *RunCaseResult {
	outputs := map[string]interface{}{"raw": result}
	workflowRunID := ""
	switch data := result.(type) {
	case map[string]interface{}:
		outputs = data
		if value, ok := data["workflow_run_id"].(string); ok {
			workflowRunID = value
		}
	case dto.WorkflowRunResponse:
		workflowRunID = data.WorkflowRunID
		outputs = map[string]interface{}{
			"task_id":         data.TaskID,
			"workflow_run_id": data.WorkflowRunID,
		}
	case *dto.WorkflowRunResponse:
		if data != nil {
			workflowRunID = data.WorkflowRunID
			outputs = map[string]interface{}{
				"task_id":         data.TaskID,
				"workflow_run_id": data.WorkflowRunID,
			}
		}
	}
	return &RunCaseResult{
		WorkflowRunID: workflowRunID,
		Outputs:       outputs,
	}
}
