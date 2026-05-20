package question_answer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const defaultMaxAnswerCount = 3

func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nodeData, nodeID, err := parseNodeData(config)
	if err != nil {
		return nil, err
	}

	var client llmclient.LLMClient
	for _, dep := range optionalDeps {
		if candidate, ok := dep.(llmclient.LLMClient); ok {
			client = candidate
			break
		}
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.QuestionAnswer,

			TenantID:          graphInitParams.TenantID,
			APPID:             graphInitParams.AppID,
			WorkflowType:      string(graphInitParams.WorkflowType),
			WorkflowID:        graphInitParams.WorkflowID,
			UserFrom:          string(graphInitParams.UserFrom),
			UserID:            graphInitParams.UserID,
			GraphConfig:       graphInitParams.GraphConfig,
			InvokeFrom:        string(graphInitParams.InvokeFrom),
			WorkflowCallDepth: graphInitParams.CallDepth,

			Graph:             graph,
			GraphRuntimeState: graphRuntimeState,
			PreviousNodeID:    previousNodeID,
		},
		NodeData:  nodeData,
		llmClient: client,
	}, nil
}

func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	select {
	case eventChan <- &shared.NodeEventCh{Type: shared.EventTypeRunStarted, NodeID: n.NodeID, Timestamp: time.Now()}:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx)
	if err != nil {
		select {
		case eventChan <- &shared.NodeEventCh{Type: shared.EventTypeRunFailed, NodeID: n.NodeID, Error: err, Timestamp: time.Now()}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return err
	}

	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil, fmt.Errorf("variable pool is not initialized")
	}
	if err := n.validateConfig(); err != nil {
		return nil, err
	}

	previousQuestion := n.previousQuestion()
	question := previousQuestion
	if question == "" {
		renderedQuestion, err := n.renderQuestion(n.NodeData.Question)
		if err != nil {
			return nil, err
		}
		question = renderedQuestion
	}
	if question == "" {
		return nil, fmt.Errorf("question is required")
	}

	previousRounds := n.previousAnswers()
	if previousQuestion == "" {
		return n.pauseForQuestion(question, previousRounds)
	}

	answer := currentAnswer(n.GraphRuntimeState.VariablePool, n.NodeID)
	if answer == "" && n.NodeData.AnswerType == AnswerTypeChoice {
		answer = optionID(n.GraphRuntimeState.VariablePool)
	}
	if answer == "" {
		return nil, fmt.Errorf("question answer response is required")
	}

	rounds := append(previousRounds, AnswerRound{
		Round:    len(previousRounds) + 1,
		Question: question,
		Answer:   answer,
	})

	if n.NodeData.AnswerType == AnswerTypeChoice {
		return n.executeChoice(ctx, question, answer, rounds)
	}
	return n.executeText(ctx, question, answer, rounds)
}

func (n *Node) pauseForQuestion(question string, rounds []AnswerRound) (*shared.NodeRunResult, error) {
	if n.NodeData.AnswerType != AnswerTypeChoice {
		return n.pauseResult(question, rounds, nil), nil
	}
	choices, err := n.resolveChoices()
	if err != nil {
		return nil, err
	}
	return n.pauseResult(question, rounds, choices), nil
}

func (n *Node) executeChoice(ctx context.Context, question string, answer string, rounds []AnswerRound) (*shared.NodeRunResult, error) {
	_ = ctx

	choices, err := n.resolveChoices()
	if err != nil {
		return nil, err
	}
	selected, ok := matchChoice(choices, optionID(n.GraphRuntimeState.VariablePool), answer)
	if ok {
		outputs := n.outputs(question, answer, rounds, choices, true)
		outputs["choice_id"] = selected.ID
		outputs["choice_label"] = selected.Label
		outputs["choice_value"] = selected.Value
		edgeHandle := selected.ID
		if n.isDynamicChoiceMode() {
			edgeHandle = DynamicChoiceHandle
		}
		return &shared.NodeRunResult{
			Status:           shared.SUCCEEDED,
			Inputs:           map[string]any{"question": question, "answer": answer, "answer_type": AnswerTypeChoice},
			Outputs:          outputs,
			ProcessData:      map[string]any{"answers": rounds},
			EdgeSourceHandle: edgeHandle,
		}, nil
	}

	if len(rounds) >= n.maxAnswerCount() {
		return nil, fmt.Errorf("question answer choice not matched after %d rounds", len(rounds))
	}

	followUp := question
	return n.pauseResult(followUp, rounds, choices), nil
}

func (n *Node) executeText(ctx context.Context, question string, answer string, rounds []AnswerRound) (*shared.NodeRunResult, error) {
	if !n.NodeData.ExtractFromAnswer {
		return &shared.NodeRunResult{
			Status:      shared.SUCCEEDED,
			Inputs:      map[string]any{"question": question, "answer": answer, "answer_type": AnswerTypeText},
			Outputs:     n.outputs(question, answer, rounds, nil, true),
			ProcessData: map[string]any{"answers": rounds},
		}, nil
	}

	if n.llmClient == nil {
		return nil, fmt.Errorf("question answer text extraction requires llm client")
	}

	decision, usage, err := n.evaluateTextExtraction(ctx, question, answer, rounds)
	if err != nil {
		return nil, err
	}
	extractedFields, missingFields, err := n.validateExtractedFields(decision.Fields)
	if err != nil {
		return nil, err
	}
	if len(missingFields) == 0 {
		outputs := n.outputs(question, answer, rounds, nil, true)
		outputs["extracted_fields"] = extractedFields
		for key, value := range extractedFields {
			outputs[key] = value
		}
		return &shared.NodeRunResult{
			Status:      shared.SUCCEEDED,
			Inputs:      map[string]any{"question": question, "answer": answer, "answer_type": AnswerTypeText},
			Outputs:     outputs,
			ProcessData: map[string]any{"answers": rounds, "extraction_reason": decision.Reason},
			LLMUsage:    usage,
		}, nil
	}

	if len(rounds) >= n.maxAnswerCount() {
		return nil, fmt.Errorf("missing required extracted fields: %s", strings.Join(missingFields, ", "))
	}
	followUp := strings.TrimSpace(decision.FollowUpQuestion)
	if followUp == "" {
		return nil, fmt.Errorf("question answer extraction decision missing follow-up question")
	}

	result := n.pauseResult(followUp, rounds, nil)
	result.ProcessData["missing_fields"] = missingFields
	result.ProcessData["extraction_reason"] = decision.Reason
	result.LLMUsage = usage
	return result, nil
}

func (n *Node) pauseResult(question string, rounds []AnswerRound, choices []Choice) *shared.NodeRunResult {
	outputs := n.outputs(question, "", rounds, choices, false)
	return &shared.NodeRunResult{
		Status:      shared.PAUSED,
		Inputs:      map[string]any{"question": question, "answer_type": n.NodeData.AnswerType},
		Outputs:     outputs,
		ProcessData: map[string]any{"answers": rounds},
	}
}

func (n *Node) outputs(question, answer string, rounds []AnswerRound, choices []Choice, complete bool) map[string]any {
	outputs := map[string]any{
		"question": question,
		"answer":   answer,
		"answers":  rounds,
		"round":    len(rounds),
		"complete": complete,
	}
	if n.NodeData.AnswerType == AnswerTypeChoice {
		outputs["choices"] = choices
	}
	return outputs
}

func (n *Node) evaluateTextExtraction(ctx context.Context, question string, answer string, rounds []AnswerRound) (extractionDecision, *shared.LLMUsage, error) {
	model := n.effectiveModel()
	modelName := strings.TrimSpace(model.Name)
	if modelName == "" {
		modelName = strings.TrimSpace(model.Model)
	}
	if strings.TrimSpace(model.Provider) == "" || modelName == "" {
		return extractionDecision{}, nil, fmt.Errorf("question answer text extraction requires model config")
	}

	req := &llmadapter.ChatRequest{
		Provider: strings.TrimSpace(model.Provider),
		Model:    modelName,
		Messages: []llmadapter.Message{
			{Role: "system", Content: textExtractionSystemPrompt()},
			{Role: "user", Content: textExtractionUserPrompt(n.NodeData.ExtractionInstruction, question, answer, rounds, n.NodeData.ExtractionFields)},
		},
		ResponseFormat: &llmadapter.ResponseFormat{Type: "json_object"},
		Stream:         false,
		User:           n.UserID,
	}
	applyCompletionParams(req, model.CompletionParams)

	appCtx := &llmclient.AppContext{
		AppID:              n.APPID,
		AppType:            "workflow",
		AccountID:          n.UserID,
		WorkspaceID:        n.TenantID,
		BillingSubjectType: n.billingSubjectType(),
		SessionID:          n.conversationID(),
		ConversationID:     n.conversationID(),
		WorkflowID:         n.WorkflowID,
		WorkflowRunID:      n.workflowRunID(),
		NodeID:             n.NodeID,
		NodeType:           string(shared.QuestionAnswer),
	}
	resp, err := n.llmClient.AppChat(ctx, appCtx, req)
	if err != nil {
		return extractionDecision{}, nil, fmt.Errorf("question answer extraction failed: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return extractionDecision{}, nil, fmt.Errorf("question answer extraction returned empty response")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(content) == "" {
		return extractionDecision{}, nil, fmt.Errorf("question answer extraction returned empty content")
	}

	var decision extractionDecision
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return extractionDecision{}, nil, fmt.Errorf("decode question answer extraction decision: %w", err)
	}
	if decision.Fields == nil {
		decision.Fields = map[string]any{}
	}
	usage := adapterUsage(resp.Usage)
	return decision, usage, nil
}

func parseNodeData(config map[string]any) (NodeData, string, error) {
	nodeID, ok := config["id"].(string)
	if !ok || strings.TrimSpace(nodeID) == "" {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	rawData, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	payload, err := json.Marshal(rawData)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("marshal question answer node data: %w", err)
	}
	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("unmarshal question answer node data: %w", err)
	}
	normalizeNodeData(&nodeData)
	return nodeData, nodeID, nil
}
