package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"

	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/pkg/response"
)

type WorkflowRunPrecheckStatus string

type WorkflowRunPrecheckWarning struct {
	Code   int            `json:"code"`
	Params map[string]any `json:"params"`
}

type WorkflowRunPrecheckResponse struct {
	ContainsAICreditNodes bool                         `json:"contains_ai_credit_nodes"`
	Status                WorkflowRunPrecheckStatus    `json:"status"`
	Warnings              []WorkflowRunPrecheckWarning `json:"warnings"`
}

const (
	WorkflowRunPrecheckStatusOK      WorkflowRunPrecheckStatus = "ok"
	WorkflowRunPrecheckStatusWarning WorkflowRunPrecheckStatus = "warning"
	WorkflowRunPrecheckStatusUnknown WorkflowRunPrecheckStatus = "unknown"
)

func (s *WorkflowService) PrecheckWorkflowRun(ctx context.Context, workflow any, appCtx *llmclient.AppContext, userInputs map[string]any) (*WorkflowRunPrecheckResponse, error) {
	graph, err := workflowGraphForPrecheck(workflow)
	if err != nil {
		return nil, err
	}

	if !graphContainsAICreditNodes(graph) {
		return &WorkflowRunPrecheckResponse{
			ContainsAICreditNodes: false,
			Status:                WorkflowRunPrecheckStatusOK,
			Warnings:              []WorkflowRunPrecheckWarning{},
		}, nil
	}

	models, _, err := collectWorkflowAICreditModels(graph, userInputs)
	if err != nil {
		return &WorkflowRunPrecheckResponse{
			ContainsAICreditNodes: true,
			Status:                WorkflowRunPrecheckStatusUnknown,
			Warnings:              []WorkflowRunPrecheckWarning{},
		}, nil
	}

	prechecker := s.getAppModelPrechecker()
	if prechecker == nil {
		return &WorkflowRunPrecheckResponse{
			ContainsAICreditNodes: true,
			Status:                WorkflowRunPrecheckStatusUnknown,
			Warnings:              []WorkflowRunPrecheckWarning{},
		}, nil
	}

	result, err := prechecker.PrecheckAppModels(ctx, appCtx, models)
	if err != nil || result == nil {
		return &WorkflowRunPrecheckResponse{
			ContainsAICreditNodes: true,
			Status:                WorkflowRunPrecheckStatusUnknown,
			Warnings:              []WorkflowRunPrecheckWarning{},
		}, nil
	}

	return buildWorkflowRunPrecheckResponse(true, result), nil
}

func (s *WorkflowService) getAppModelPrechecker() llmclient.AppModelPrechecker {
	if s == nil || s.executor == nil {
		return nil
	}
	prechecker, _ := s.executor.GetLLMClient().(llmclient.AppModelPrechecker)
	return prechecker
}

func buildWorkflowRunPrecheckResponse(containsAICreditNodes bool, result *llmclient.AppModelPrecheckResult) *WorkflowRunPrecheckResponse {
	if result == nil {
		return &WorkflowRunPrecheckResponse{
			ContainsAICreditNodes: containsAICreditNodes,
			Status:                WorkflowRunPrecheckStatusUnknown,
			Warnings:              []WorkflowRunPrecheckWarning{},
		}
	}

	warnings := make([]WorkflowRunPrecheckWarning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		code := 0
		switch warning.Kind {
		case llmclient.AppModelPrecheckWarningOrganizationBalanceLow:
			code = response.ErrWorkflowOrganizationBalanceLow.Code
		case llmclient.AppModelPrecheckWarningWorkspaceQuotaLow:
			code = response.ErrWorkflowWorkspaceQuotaLow.Code
		case llmclient.AppModelPrecheckWarningPrivateChannelBalanceLow:
			code = response.ErrWorkflowPrivateChannelBalanceLow.Code
		case llmclient.AppModelPrecheckWarningPrivateChannelUpstreamUnavailable:
			code = response.ErrWorkflowPrivateChannelUpstreamUnavailable.Code
		default:
			continue
		}
		warnings = append(warnings, WorkflowRunPrecheckWarning{
			Code: code,
			Params: map[string]any{
				"current_value": warning.CurrentValue,
				"threshold":     warning.Threshold,
				"reason":        warning.Reason,
			},
		})
	}

	status := WorkflowRunPrecheckStatusUnknown
	switch result.Status {
	case llmclient.AppModelPrecheckStatusOK:
		status = WorkflowRunPrecheckStatusOK
	case llmclient.AppModelPrecheckStatusWarning:
		status = WorkflowRunPrecheckStatusWarning
	case llmclient.AppModelPrecheckStatusUnknown:
		status = WorkflowRunPrecheckStatusUnknown
	}

	return &WorkflowRunPrecheckResponse{
		ContainsAICreditNodes: containsAICreditNodes,
		Status:                status,
		Warnings:              warnings,
	}
}

func workflowGraphForPrecheck(workflow any) (map[string]any, error) {
	switch value := workflow.(type) {
	case map[string]any:
		if _, ok := toAnySlice(value["nodes"]); ok {
			return value, nil
		}
		return mergeRootVariablesIntoGraph(value)
	case *Workflow:
		graph := value.GetGraphDict()
		graph["environment_variables"] = value.GetEnvironmentVariablesDict()
		graph["conversation_variables"] = value.GetConversationVariablesDict()
		return graph, nil
	default:
		return nil, fmt.Errorf("unsupported workflow type %T", workflow)
	}
}

func graphContainsAICreditNodes(graph map[string]any) bool {
	rawNodes, ok := toAnySlice(graph["nodes"])
	if !ok {
		return false
	}

	for _, rawNode := range rawNodes {
		nodeMap, ok := toStringAnyMap(rawNode)
		if !ok {
			continue
		}
		data, ok := toStringAnyMap(nodeMap["data"])
		if !ok {
			continue
		}

		nodeType, _ := data["type"].(string)
		if isAICreditNodeType(nodeType) {
			return true
		}
	}

	return false
}

func collectWorkflowAICreditModels(graph map[string]any, userInputs map[string]any) ([]llmclient.AppModelRef, bool, error) {
	rawNodes, ok := toAnySlice(graph["nodes"])
	if !ok {
		return nil, false, nil
	}

	seen := map[llmclient.AppModelRef]struct{}{}
	models := make([]llmclient.AppModelRef, 0)
	containsAICreditNodes := false

	for _, rawNode := range rawNodes {
		nodeMap, ok := toStringAnyMap(rawNode)
		if !ok {
			continue
		}
		data, ok := toStringAnyMap(nodeMap["data"])
		if !ok {
			continue
		}

		nodeType, _ := data["type"].(string)
		if !isAICreditNodeType(nodeType) {
			continue
		}
		containsAICreditNodes = true

		resolvedModels, err := resolveAICreditNodeModels(nodeType, data, userInputs)
		if err != nil {
			return nil, true, err
		}
		for _, model := range resolvedModels {
			if _, exists := seen[model]; exists {
				continue
			}
			seen[model] = struct{}{}
			models = append(models, model)
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider == models[j].Provider {
			return models[i].Model < models[j].Model
		}
		return models[i].Provider < models[j].Provider
	})
	return models, containsAICreditNodes, nil
}

func isAICreditNodeType(nodeType string) bool {
	switch workflowshared.NodeType(nodeType) {
	case workflowshared.LLM,
		workflowshared.KnowledgeRetrieval,
		workflowshared.ParameterExtractor,
		workflowshared.SQLGenerator,
		workflowshared.ImageGen:
		return true
	default:
		return false
	}
}

func resolveAICreditNodeModels(nodeType string, data map[string]any, userInputs map[string]any) ([]llmclient.AppModelRef, error) {
	switch workflowshared.NodeType(nodeType) {
	case workflowshared.LLM, workflowshared.ParameterExtractor, workflowshared.SQLGenerator, workflowshared.ImageGen:
		model, err := resolveNodeModelWithRuntimeOverride(data, userInputs)
		if err != nil {
			return nil, err
		}
		return []llmclient.AppModelRef{model}, nil
	case workflowshared.KnowledgeRetrieval:
		return resolveKnowledgeRetrievalModels(data)
	default:
		return nil, nil
	}
}

func resolveNodeModelWithRuntimeOverride(data map[string]any, userInputs map[string]any) (llmclient.AppModelRef, error) {
	if modelConfig, exists := userInputs["model_config"]; exists {
		return extractModelRefFromValue(modelConfig)
	}

	modelValue, exists := data["model"]
	if !exists {
		return llmclient.AppModelRef{}, fmt.Errorf("model config missing")
	}
	return extractModelRefFromValue(modelValue)
}

func resolveKnowledgeRetrievalModels(data map[string]any) ([]llmclient.AppModelRef, error) {
	models := make([]llmclient.AppModelRef, 0, 4)
	appendModel := func(value any) error {
		model, err := extractModelRefFromValue(value)
		if err != nil {
			return err
		}
		models = append(models, model)
		return nil
	}

	if single, ok := toStringAnyMap(data["single_retrieval_config"]); ok {
		if _, exists := single["provider"]; exists {
			if err := appendModel(single); err != nil {
				return nil, err
			}
		}
	}

	if metadataModel, exists := data["metadata_model_config"]; exists && metadataModel != nil {
		if err := appendModel(metadataModel); err != nil {
			return nil, err
		}
	}

	if multiple, ok := toStringAnyMap(data["multiple_retrieval_config"]); ok {
		if rerankingModel, exists := multiple["reranking_model"]; exists && rerankingModel != nil {
			if err := appendModel(rerankingModel); err != nil {
				return nil, err
			}
		}
		if vectorSetting, ok := nestedMap(multiple, "weights", "vector_setting"); ok {
			embeddingProvider, _ := vectorSetting["embedding_provider_name"].(string)
			embeddingProvider = strings.TrimSpace(embeddingProvider)
			embeddingName, _ := vectorSetting["embedding_model_name"].(string)
			embeddingName = strings.TrimSpace(embeddingName)
			if embeddingProvider == "" {
				return nil, fmt.Errorf("knowledge retrieval embedding provider missing")
			}
			if embeddingName == "" {
				return nil, fmt.Errorf("knowledge retrieval embedding model missing")
			}
			models = append(models, llmclient.AppModelRef{Provider: embeddingProvider, Model: embeddingName})
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("knowledge retrieval model config missing")
	}

	return models, nil
}

func nestedMap(root map[string]any, keys ...string) (map[string]any, bool) {
	current := root
	for idx, key := range keys {
		value, exists := current[key]
		if !exists {
			return nil, false
		}
		if idx == len(keys)-1 {
			return toStringAnyMap(value)
		}
		next, ok := toStringAnyMap(value)
		if !ok {
			return nil, false
		}
		current = next
	}
	return nil, false
}

func extractModelRefFromValue(value any) (llmclient.AppModelRef, error) {
	config, ok := toStringAnyMap(value)
	if !ok {
		return llmclient.AppModelRef{}, fmt.Errorf("model config must be an object")
	}

	provider, _ := config["provider"].(string)
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return llmclient.AppModelRef{}, fmt.Errorf("model provider missing")
	}

	name := ""
	if rawName, ok := config["name"].(string); ok {
		name = strings.TrimSpace(rawName)
	}
	if name == "" {
		if rawName, ok := config["model"].(string); ok {
			name = strings.TrimSpace(rawName)
		}
	}
	if name == "" {
		return llmclient.AppModelRef{}, fmt.Errorf("model name missing")
	}
	return llmclient.AppModelRef{Provider: provider, Model: name}, nil
}

func toAnySlice(value any) ([]any, bool) {
	typed, ok := value.([]interface{})
	if !ok {
		return nil, false
	}
	result := make([]any, len(typed))
	for i, item := range typed {
		result[i] = item
	}
	return result, true
}

func toStringAnyMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]interface{})
	if !ok {
		return nil, false
	}
	result := make(map[string]any, len(typed))
	for key, item := range typed {
		result[key] = item
	}
	return result, true
}
