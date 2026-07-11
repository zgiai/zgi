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

	modelNames, _, err := collectWorkflowAICreditModelNames(graph, userInputs)
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

	result, err := prechecker.PrecheckAppModels(ctx, appCtx, modelNames)
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

func collectWorkflowAICreditModelNames(graph map[string]any, userInputs map[string]any) ([]string, bool, error) {
	rawNodes, ok := toAnySlice(graph["nodes"])
	if !ok {
		return nil, false, nil
	}

	seen := map[string]struct{}{}
	modelNames := make([]string, 0)
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

		names, err := resolveAICreditNodeModelNames(nodeType, data, userInputs)
		if err != nil {
			return nil, true, err
		}
		for _, name := range names {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			modelNames = append(modelNames, name)
		}
	}

	sort.Strings(modelNames)
	return modelNames, containsAICreditNodes, nil
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

func resolveAICreditNodeModelNames(nodeType string, data map[string]any, userInputs map[string]any) ([]string, error) {
	switch workflowshared.NodeType(nodeType) {
	case workflowshared.LLM, workflowshared.ParameterExtractor, workflowshared.SQLGenerator, workflowshared.ImageGen:
		name, err := resolveNodeModelNameWithRuntimeOverride(data, userInputs)
		if err != nil {
			return nil, err
		}
		return []string{name}, nil
	case workflowshared.KnowledgeRetrieval:
		return resolveKnowledgeRetrievalModelNames(data)
	default:
		return nil, nil
	}
}

func resolveNodeModelNameWithRuntimeOverride(data map[string]any, userInputs map[string]any) (string, error) {
	if modelConfig, exists := userInputs["model_config"]; exists {
		return extractModelNameFromValue(modelConfig)
	}

	modelValue, exists := data["model"]
	if !exists {
		return "", fmt.Errorf("model config missing")
	}
	return extractModelNameFromValue(modelValue)
}

func resolveKnowledgeRetrievalModelNames(data map[string]any) ([]string, error) {
	names := make([]string, 0, 4)
	appendName := func(value any) error {
		name, err := extractModelNameFromValue(value)
		if err != nil {
			return err
		}
		names = append(names, name)
		return nil
	}

	if single, ok := toStringAnyMap(data["single_retrieval_config"]); ok {
		if _, exists := single["provider"]; exists {
			if err := appendName(single); err != nil {
				return nil, err
			}
		}
	}

	if metadataModel, exists := data["metadata_model_config"]; exists && metadataModel != nil {
		if err := appendName(metadataModel); err != nil {
			return nil, err
		}
	}

	if multiple, ok := toStringAnyMap(data["multiple_retrieval_config"]); ok {
		if rerankingModel, exists := multiple["reranking_model"]; exists && rerankingModel != nil {
			if err := appendName(rerankingModel); err != nil {
				return nil, err
			}
		}
		if vectorSetting, ok := nestedMap(multiple, "weights", "vector_setting"); ok {
			embeddingName, _ := vectorSetting["embedding_model_name"].(string)
			embeddingName = strings.TrimSpace(embeddingName)
			if embeddingName == "" {
				return nil, fmt.Errorf("knowledge retrieval embedding model missing")
			}
			names = append(names, embeddingName)
		}
	}

	if len(names) == 0 {
		return nil, fmt.Errorf("knowledge retrieval model config missing")
	}

	return names, nil
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

func extractModelNameFromValue(value any) (string, error) {
	config, ok := toStringAnyMap(value)
	if !ok {
		return "", fmt.Errorf("model config must be an object")
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
		return "", fmt.Errorf("model name missing")
	}
	return name, nil
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
