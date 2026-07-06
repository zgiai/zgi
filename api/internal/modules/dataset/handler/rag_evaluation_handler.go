package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	defaultRAGEvaluationTopK     = 10
	maxRAGEvaluationBatchSize    = 100
	defaultRAGEvaluationMaxToken = 1024
)

type RAGEvaluationHandler struct {
	knowledgeRetrieval *datasetservice.KnowledgeRetrievalService
	llmClient          client.LLMClient
	defaultModel       llmdefaultservice.DefaultModelService
	organization       interfaces.OrganizationService
}

type RAGEvaluationRequest struct {
	KnowledgeBaseName string   `json:"knowledge_base_name"`
	UserInputs        []string `json:"user_inputs"`
	TopK              int      `json:"top_k,omitempty"`
	ScoreThreshold    *float64 `json:"score_threshold,omitempty"`
	RetrievalMode     string   `json:"retrieval_mode,omitempty"`
	Model             string   `json:"model,omitempty"`
}

type RAGEvaluationBatchResponse struct {
	Data []RAGEvaluationItemResponse `json:"data"`
}

type RAGEvaluationItemResponse struct {
	UserInput          string                                      `json:"user_input"`
	Response           string                                      `json:"response"`
	RetrievedContexts  []string                                    `json:"retrieved_contexts"`
	RetrieverResources []datasetservice.KnowledgeRetrieverResource `json:"retriever_resources"`
	Status             string                                      `json:"status"`
	Error              string                                      `json:"error,omitempty"`
}

func NewRAGEvaluationHandler(
	knowledgeRetrieval *datasetservice.KnowledgeRetrievalService,
	llmClient client.LLMClient,
	defaultModel llmdefaultservice.DefaultModelService,
	organization interfaces.OrganizationService,
) *RAGEvaluationHandler {
	return &RAGEvaluationHandler{
		knowledgeRetrieval: knowledgeRetrieval,
		llmClient:          llmClient,
		defaultModel:       defaultModel,
		organization:       organization,
	}
}

func (h *RAGEvaluationHandler) BatchEvaluate(c *gin.Context) {
	if h == nil || h.knowledgeRetrieval == nil || h.llmClient == nil || h.defaultModel == nil || h.organization == nil {
		response.FailWithMessage(c, response.ErrSystemError, "rag evaluation service is not configured")
		return
	}

	var req RAGEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	req.KnowledgeBaseName = strings.TrimSpace(req.KnowledgeBaseName)
	if req.KnowledgeBaseName == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "knowledge_base_name is required")
		return
	}

	inputs := normalizeRAGUserInputs(req.UserInputs)
	if len(inputs) == 0 {
		response.FailWithMessage(c, response.ErrInvalidParam, "user_inputs is required")
		return
	}
	if len(inputs) > maxRAGEvaluationBatchSize {
		response.FailWithMessage(c, response.ErrInvalidParam, "user_inputs cannot exceed "+strconv.Itoa(maxRAGEvaluationBatchSize))
		return
	}

	workspaceID := util.GetWorkspaceID(c)
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		organizationID = workspaceID
	}
	scope := datasetservice.KnowledgeScope{
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		AccountID:      c.GetString("account_id"),
	}
	dataset, err := h.resolveDataset(c.Request.Context(), scope, req.KnowledgeBaseName)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	ok, err := h.checkRetrievalPermission(c.Request.Context(), scope, dataset.WorkspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !ok {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	topK := req.TopK
	if topK <= 0 {
		topK = defaultRAGEvaluationTopK
	}
	if req.ScoreThreshold != nil && (*req.ScoreThreshold < 0 || *req.ScoreThreshold > 1) {
		response.FailWithMessage(c, response.ErrInvalidParam, "score_threshold must be between 0 and 1")
		return
	}
	model, err := h.resolveModel(c.Request.Context(), scope.OrganizationID, req.Model)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	out := RAGEvaluationBatchResponse{Data: make([]RAGEvaluationItemResponse, 0, len(inputs))}
	for _, userInput := range inputs {
		item := h.evaluateOne(c.Request.Context(), scope, dataset.DatasetID, req.KnowledgeBaseName, userInput, topK, req.ScoreThreshold, req.RetrievalMode, model)
		out.Data = append(out.Data, item)
	}

	response.Success(c, out)
}

func (h *RAGEvaluationHandler) resolveDataset(ctx context.Context, scope datasetservice.KnowledgeScope, name string) (datasetservice.KnowledgeDatasetSummary, error) {
	list, err := h.knowledgeRetrieval.ListAccessibleDatasets(ctx, scope, name, 100)
	if err != nil {
		return datasetservice.KnowledgeDatasetSummary{}, fmt.Errorf("failed to list accessible knowledge bases: %w", err)
	}

	matches := make([]datasetservice.KnowledgeDatasetSummary, 0)
	for _, dataset := range list.KnowledgeBases {
		if strings.EqualFold(strings.TrimSpace(dataset.Name), name) {
			matches = append(matches, dataset)
		}
	}
	if len(matches) == 0 {
		return datasetservice.KnowledgeDatasetSummary{}, fmt.Errorf("knowledge base %q was not found or is not accessible", name)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.DatasetID)
		}
		return datasetservice.KnowledgeDatasetSummary{}, fmt.Errorf("knowledge base name %q matched multiple datasets: %s", name, strings.Join(ids, ", "))
	}
	return matches[0], nil
}

func (h *RAGEvaluationHandler) checkRetrievalPermission(ctx context.Context, scope datasetservice.KnowledgeScope, workspaceID string) (bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(scope.WorkspaceID)
	}
	return h.organization.CheckWorkspaceOrganizationAnyPermission(
		ctx,
		strings.TrimSpace(scope.OrganizationID),
		workspaceID,
		strings.TrimSpace(scope.AccountID),
		workspace_model.WorkspacePermissionKnowledgeBaseRetrievalTest,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
	)
}

func (h *RAGEvaluationHandler) evaluateOne(
	ctx context.Context,
	scope datasetservice.KnowledgeScope,
	datasetID string,
	datasetName string,
	userInput string,
	topK int,
	scoreThreshold *float64,
	retrievalMode string,
	model string,
) RAGEvaluationItemResponse {
	item := RAGEvaluationItemResponse{
		UserInput:         userInput,
		RetrievedContexts: []string{},
		Status:            datasetservice.KnowledgeRetrieveStatusSuccess,
	}

	retrievalResp, err := h.knowledgeRetrieval.Retrieve(ctx, datasetservice.KnowledgeRetrieveRequest{
		Scope:           scope,
		Query:           userInput,
		DatasetIDs:      []string{datasetID},
		TopK:            topK,
		RetrievalMode:   retrievalMode,
		RetrievalConfig: ragEvaluationRetrievalConfig(scoreThreshold),
	})
	if err != nil {
		item.Status = "error"
		item.Error = err.Error()
		return item
	}

	item.RetrieverResources = retrievalResp.Resources
	for _, resource := range retrievalResp.Resources {
		if content := strings.TrimSpace(resource.Content); content != "" {
			item.RetrievedContexts = append(item.RetrievedContexts, content)
		}
	}
	if len(item.RetrievedContexts) == 0 {
		item.Status = datasetservice.KnowledgeRetrieveStatusNoResults
		item.Response = "暂时没有相关信息"
		return item
	}

	answer, err := h.generateAnswer(ctx, scope.OrganizationID, model, datasetName, userInput, retrievalResp.Context)
	if err != nil {
		item.Status = "error"
		item.Error = err.Error()
		return item
	}
	item.Response = answer
	return item
}

func ragEvaluationRetrievalConfig(scoreThreshold *float64) map[string]interface{} {
	if scoreThreshold == nil {
		return nil
	}
	return map[string]interface{}{
		"score_threshold_enabled": true,
		"score_threshold":         *scoreThreshold,
	}
}

func (h *RAGEvaluationHandler) resolveModel(ctx context.Context, organizationID string, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested, nil
	}
	resolved, err := llmruntime.NewModelResolver(h.defaultModel).ResolveDefault(ctx, organizationID, sharedmodel.ModelTypeLLM)
	if err != nil {
		return "", fmt.Errorf("failed to resolve default LLM model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return "", fmt.Errorf("default LLM model is not configured")
	}
	return resolved.Model, nil
}

func (h *RAGEvaluationHandler) generateAnswer(ctx context.Context, organizationID string, model string, datasetName string, question string, contextText string) (string, error) {
	temperature := 0.0
	maxTokens := defaultRAGEvaluationMaxToken
	req := &adapter.ChatRequest{
		Model:       model,
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: "你是一个严谨的RAG问答助手。请只依据给定知识库上下文回答用户问题。若上下文没有答案，请回答“暂时没有相关信息”。不要编造上下文外的信息。",
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("知识库：%s\n\n上下文：\n%s\n\n用户问题：%s\n\n请给出简洁、准确的中文回答。", datasetName, contextText, question),
			},
		},
	}

	resp, err := h.llmClient.Chat(ctx, organizationID, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate answer: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}
	answer := strings.TrimSpace(messageContentToString(resp.Choices[0].Message.Content))
	if answer == "" {
		return "", fmt.Errorf("LLM returned empty answer")
	}
	return answer, nil
}

func normalizeRAGUserInputs(inputs []string) []string {
	out := make([]string, 0, len(inputs))
	for _, input := range inputs {
		trimmed := strings.TrimSpace(input)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func messageContentToString(content interface{}) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	case []adapter.MessageContentPart:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
		return strings.Join(parts, "")
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			if text := strings.TrimSpace(fmt.Sprint(part)); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return fmt.Sprint(value)
	}
}
