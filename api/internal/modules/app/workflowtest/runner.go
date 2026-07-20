package workflowtest

import (
	"context"
	"encoding/json"
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
	if err := validateGeneratedAssetBindings(req.CaseSnapshot.Turns); err != nil {
		return nil, fmt.Errorf("测试问题文件校验失败: %w", err)
	}
	draft := r.resolveDraftWorkflow(ctx, req.AgentID)
	startInputs := startInputVariablesFromDraft(draft)
	isChatDraft := draftWorkflowType(draft) == "chat"
	textInputName := primaryTextInputNameFromVariables(startInputs)
	if textInputName == "" {
		textInputName = "input1"
	}
	turns := runnableCaseTurns(req.CaseSnapshot)
	if draftRequiresCurrentTurnFiles(draft) {
		for index, turn := range turns {
			if len(turn.Attachments) == 0 {
				return nil, fmt.Errorf("测试问题第 %d 轮缺少附件：当前工作流每轮都必须提供文件", index+1)
			}
		}
	}
	var lastResult *RunCaseResult
	turnResults := make([]map[string]interface{}, 0, len(turns))
	conversationID := ""
	for index, turn := range turns {
		result, nextConversationID, err := r.runTurn(ctx, req.AgentID, turn, textInputName, startInputs, isChatDraft, conversationID, index+1)
		if nextConversationID != "" {
			conversationID = nextConversationID
		}
		if result != nil {
			lastResult = result
			turnResult := map[string]interface{}{
				"turn_index":      index + 1,
				"content":         turn.Content,
				"workflow_run_id": result.WorkflowRunID,
				"outputs":         result.Outputs,
			}
			if conversationID != "" {
				turnResult["conversation_id"] = conversationID
			}
			turnResults = append(turnResults, turnResult)
		}
		if err != nil {
			if lastResult != nil {
				attachTurnExecutionSummary(lastResult, turnResults, len(turns))
			}
			return lastResult, err
		}
	}
	if lastResult == nil {
		return &RunCaseResult{Outputs: map[string]interface{}{}}, nil
	}
	attachTurnExecutionSummary(lastResult, turnResults, len(turns))
	return lastResult, nil
}

func attachTurnExecutionSummary(result *RunCaseResult, turnResults []map[string]interface{}, plannedTurnCount int) {
	if result == nil {
		return
	}
	outputs := make(map[string]interface{}, len(result.Outputs)+3)
	for key, value := range result.Outputs {
		outputs[key] = value
	}
	outputs["turn_count"] = len(turnResults)
	outputs["planned_turn_count"] = plannedTurnCount
	outputs["turn_results"] = turnResults
	result.Outputs = outputs
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
		if strings.ToLower(role) != "user" {
			continue
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

type startInputVariable struct {
	Name string
	Type string
}

func (r *WorkflowServiceRunner) resolvePrimaryTextInputName(ctx context.Context, agentID string) string {
	name := primaryTextInputNameFromVariables(r.resolveStartInputVariables(ctx, agentID))
	if name == "" {
		return "input1"
	}
	return name
}

func (r *WorkflowServiceRunner) resolveStartInputVariables(ctx context.Context, agentID string) []startInputVariable {
	return startInputVariablesFromDraft(r.resolveDraftWorkflow(ctx, agentID))
}

func (r *WorkflowServiceRunner) resolveDraftWorkflow(ctx context.Context, agentID string) interface{} {
	draft, err := r.WorkflowService.GetDraftWorkflow(ctx, agentID, true)
	if err != nil {
		return nil
	}
	return draft
}

func startInputVariablesFromDraft(draft interface{}) []startInputVariable {
	return startInputVariablesFromGraph(workflowGraphFromDraft(draft))
}

func workflowGraphFromDraft(draft interface{}) map[string]interface{} {
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
	return graph
}

func draftWorkflowType(draft interface{}) string {
	switch data := draft.(type) {
	case map[string]interface{}:
		if value, ok := data["type"].(string); ok {
			return strings.TrimSpace(value)
		}
	case dto.WorkflowDetail:
		return strings.TrimSpace(string(data.Type))
	case *dto.WorkflowDetail:
		if data != nil {
			return strings.TrimSpace(string(data.Type))
		}
	}
	return ""
}

func primaryTextInputNameFromGraph(graph map[string]interface{}) string {
	return primaryTextInputNameFromVariables(startInputVariablesFromGraph(graph))
}

func primaryTextInputNameFromVariables(variables []startInputVariable) string {
	for _, variable := range variables {
		if variable.Type != "" && variable.Type != "text-input" && variable.Type != "paragraph" && variable.Type != "string" {
			continue
		}
		if variable.Name != "" {
			return variable.Name
		}
	}
	return ""
}

func startInputVariablesFromGraph(graph map[string]interface{}) []startInputVariable {
	nodes, ok := graph["nodes"].([]interface{})
	if !ok {
		return nil
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
			return nil
		}
		result := make([]startInputVariable, 0, len(variables))
		for _, item := range variables {
			variable, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			varType, _ := variable["type"].(string)
			for _, key := range []string{"variable", "name"} {
				if name, ok := variable[key].(string); ok && strings.TrimSpace(name) != "" {
					result = append(result, startInputVariable{
						Name: strings.TrimSpace(name),
						Type: strings.TrimSpace(varType),
					})
					break
				}
			}
		}
		return result
	}
	return nil
}

func (r *WorkflowServiceRunner) runTurn(ctx context.Context, agentID string, turn CaseTurn, textInputName string, startInputs []startInputVariable, isChatDraft bool, conversationID string, dialogueCount int) (*RunCaseResult, string, error) {
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
		if isWorkflowTestReservedInputKey(name) {
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
		assignAttachmentsToStartFileInputs(inputs, startInputs, fileInputs)
	}
	if isChatDraft {
		inputs["query"] = turn.Content
		inputs["sys.workflow_type"] = "chat"
		if dialogueCount <= 0 {
			dialogueCount = 1
		}
		inputs["sys.dialogue_count"] = dialogueCount
		if strings.TrimSpace(conversationID) != "" {
			inputs["sys.conversation_id"] = strings.TrimSpace(conversationID)
		} else {
			inputs["sys.parent_message_id"] = ""
		}
		inputs["conversation_params"] = map[string]interface{}{
			"from_source": "account",
			"invoke_from": "debugger",
		}
	}
	runReq := &dto.DraftWorkflowRunRequest{
		Inputs:       inputs,
		UserID:       r.AccountID,
		ResponseMode: "blocking",
		Files:        files,
	}
	result, err := r.WorkflowService.RunDraftWorkflow(ctx, r.WorkspaceID, agentID, runReq, r.AccountID)
	if err != nil {
		return nil, "", err
	}
	normalized := normalizeWorkflowRunResult(result)
	r.attachWorkflowTrace(ctx, agentID, normalized)
	promoteWorkflowAnswerFromTrace(normalized.Outputs)
	nextConversationID := conversationIDFromRunInputs(runReq.Inputs)
	if nextConversationID == "" {
		nextConversationID = conversationIDFromOutputs(normalized.Outputs)
	}
	if failure := workflowRunFailure(normalized.Outputs); failure != nil {
		return normalized, nextConversationID, failure
	}
	return normalized, nextConversationID, nil
}

func workflowRunFailure(outputs map[string]interface{}) error {
	if outputs == nil {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(outputs["status"])))
	switch status {
	case "failed", "error", "exception":
	default:
		return nil
	}
	message := strings.TrimSpace(workflowRunErrorText(outputs["error"]))
	if message == "" {
		message = strings.TrimSpace(workflowRunErrorText(outputs["node_errors"]))
	}
	if message == "" {
		message = status
	}
	return fmt.Errorf("工作流执行失败：%s", message)
}

func workflowRunErrorText(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case error:
		return typed.Error()
	case map[string]string:
		parts := make([]string, 0, len(typed))
		for key, message := range typed {
			parts = append(parts, fmt.Sprintf("%s: %s", key, message))
		}
		return strings.Join(parts, "; ")
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(encoded)
	}
}

func isWorkflowTestReservedInputKey(key string) bool {
	switch strings.TrimSpace(key) {
	case caseModeInputKey,
		expectedChecksInputKey,
		turnExpectationInputKey,
		turnChecksInputKey,
		conversationChecksInputKey,
		"__fixture_spec",
		"__asset_source",
		"__tags":
		return true
	default:
		return false
	}
}

func (r *WorkflowServiceRunner) attachWorkflowTrace(ctx context.Context, agentID string, result *RunCaseResult) {
	if r == nil || r.WorkflowService == nil || result == nil || strings.TrimSpace(result.WorkflowRunID) == "" {
		return
	}
	raw, err := r.WorkflowService.GetWorkflowRunNodeExecutions(ctx, r.WorkspaceID, agentID, result.WorkflowRunID)
	if err != nil {
		return
	}
	nodes := normalizeWorkflowTraceNodes(raw)
	if len(nodes) == 0 {
		return
	}
	if result.Outputs == nil {
		result.Outputs = map[string]interface{}{}
	}
	result.Outputs["workflow_trace"] = map[string]interface{}{"nodes": nodes}
}

func normalizeWorkflowTraceNodes(raw interface{}) []map[string]interface{} {
	switch typed := raw.(type) {
	case *dto.WorkflowRunNodeExecutionListResponse:
		if typed == nil {
			return nil
		}
		return workflowTraceNodesFromResponses(typed.Data)
	case dto.WorkflowRunNodeExecutionListResponse:
		return workflowTraceNodesFromResponses(typed.Data)
	case map[string]interface{}:
		return workflowTraceNodesFromInterfaceSlice(typed["data"])
	default:
		return nil
	}
}

func workflowTraceNodesFromResponses(responses []dto.WorkflowRunNodeExecutionResponse) []map[string]interface{} {
	nodes := make([]map[string]interface{}, 0, len(responses))
	for _, item := range responses {
		nodes = append(nodes, map[string]interface{}{
			"node_id":        item.NodeID,
			"node_name":      item.Title,
			"node_type":      item.NodeType,
			"status":         item.Status,
			"duration_ms":    item.ElapsedTime,
			"input":          rawJSONMap(item.Inputs),
			"output":         rawJSONMap(item.Outputs),
			"error":          item.Error,
			"started_at":     item.CreatedAt,
			"finished_at":    item.FinishedAt,
			"execution_id":   item.ID,
			"predecessor":    item.PredecessorNodeID,
			"process_data":   rawJSONMap(item.ProcessData),
			"metadata":       rawJSONMap(item.ExecutionMetadata),
			"triggered_from": item.TriggeredFrom,
		})
	}
	return nodes
}

func rawJSONMap(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}
	}
	var mapped map[string]interface{}
	if err := json.Unmarshal(raw, &mapped); err != nil {
		var value interface{}
		if valueErr := json.Unmarshal(raw, &value); valueErr != nil {
			return map[string]interface{}{}
		}
		if value == nil {
			return map[string]interface{}{}
		}
		return map[string]interface{}{"value": value}
	}
	return mapped
}

func workflowTraceNodesFromInterfaceSlice(value interface{}) []map[string]interface{} {
	values, ok := value.([]interface{})
	if !ok {
		return nil
	}
	nodes := make([]map[string]interface{}, 0, len(values))
	for _, item := range values {
		mapped, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		nodes = append(nodes, mapped)
	}
	return nodes
}

func conversationIDFromRunInputs(inputs map[string]interface{}) string {
	if inputs == nil {
		return ""
	}
	if value, ok := inputs["sys.conversation_id"].(string); ok {
		return strings.TrimSpace(value)
	}
	if value, ok := inputs["conversation_id"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func conversationIDFromOutputs(outputs map[string]interface{}) string {
	if outputs == nil {
		return ""
	}
	for _, key := range []string{"sys.conversation_id", "conversation_id"} {
		if value, ok := outputs[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if nested, ok := outputs["outputs"].(map[string]interface{}); ok {
		return conversationIDFromOutputs(nested)
	}
	return ""
}

func assignAttachmentsToStartFileInputs(inputs map[string]interface{}, variables []startInputVariable, fileInputs []interface{}) {
	if len(fileInputs) == 0 {
		return
	}
	for _, variable := range variables {
		if !isFileListInputType(variable.Type) {
			continue
		}
		if _, exists := inputs[variable.Name]; exists {
			continue
		}
		inputs[variable.Name] = fileInputs
		return
	}
	fileIndex := 0
	for _, variable := range variables {
		if !isFileInputType(variable.Type) {
			continue
		}
		if _, exists := inputs[variable.Name]; exists {
			continue
		}
		if fileIndex >= len(fileInputs) {
			return
		}
		inputs[variable.Name] = fileInputs[fileIndex]
		fileIndex++
	}
}

func isFileInputType(value string) bool {
	switch value {
	case "file", "file-input":
		return true
	default:
		return false
	}
}

func isFileListInputType(value string) bool {
	switch value {
	case "file-list", "array[file]":
		return true
	default:
		return false
	}
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

func promoteWorkflowAnswerFromTrace(outputs map[string]interface{}) {
	if outputs == nil {
		return
	}
	if text := workflowAnswerTextFromMap(outputs); text != "" {
		outputs["answer"] = text
		return
	}
	if text := workflowAnswerTextFromTrace(outputs); text != "" {
		outputs["answer"] = text
	}
}

func workflowAnswerTextFromTrace(outputs map[string]interface{}) string {
	nodes := workflowTraceFromOutputs(outputs)
	if len(nodes) == 0 {
		return ""
	}
	for i := len(nodes) - 1; i >= 0; i-- {
		if !isAnswerCandidateTraceNode(nodes[i], true) {
			continue
		}
		if text := workflowAnswerTextFromMap(nodes[i].Output); text != "" {
			return text
		}
	}
	for i := len(nodes) - 1; i >= 0; i-- {
		if !isAnswerCandidateTraceNode(nodes[i], false) {
			continue
		}
		if text := workflowAnswerTextFromMap(nodes[i].Output); text != "" {
			return text
		}
	}
	return ""
}

func isAnswerCandidateTraceNode(node WorkflowTestTraceNode, strict bool) bool {
	if status := strings.ToLower(strings.TrimSpace(node.Status)); status != "" && status != "succeeded" && status != "success" {
		return false
	}
	identity := strings.ToLower(strings.Join([]string{node.NodeID, node.NodeName, node.NodeType}, " "))
	if strings.Contains(identity, "answer") || strings.Contains(identity, "reply") || strings.Contains(identity, "end") {
		return true
	}
	if strict {
		return false
	}
	return strings.Contains(identity, "llm") || strings.Contains(identity, "question-answer") || strings.Contains(identity, "question_answer")
}

func workflowAnswerTextFromMap(outputs map[string]interface{}) string {
	if len(outputs) == 0 {
		return ""
	}
	for _, key := range []string{"answer", "text", "summary", "result", "output", "content", "message", "value"} {
		if text := workflowAnswerTextFromValue(outputs[key]); text != "" {
			return text
		}
	}
	for _, key := range []string{"outputs", "data"} {
		if nested, ok := outputs[key].(map[string]interface{}); ok {
			if text := workflowAnswerTextFromMap(nested); text != "" {
				return text
			}
		}
	}
	return ""
}

func workflowAnswerTextFromValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	default:
		return ""
	}
}
