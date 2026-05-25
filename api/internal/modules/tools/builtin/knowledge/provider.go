package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	dataset_service "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const (
	ProviderID                    = "knowledge"
	ToolListAccessibleKnowledge   = "list_accessible_knowledge_bases"
	ToolRetrieveKnowledge         = "retrieve_knowledge"
	ToolRetrieveAgentKnowledge    = "retrieve_agent_knowledge"
	defaultKnowledgeListToolLimit = 20
	defaultKnowledgeToolTopK      = 5
)

type RetrievalService interface {
	ListAccessibleDatasets(ctx context.Context, scope dataset_service.KnowledgeScope, query string, limit int) ([]dataset_service.KnowledgeDatasetSummary, error)
	Retrieve(ctx context.Context, req dataset_service.KnowledgeRetrieveRequest) (*dataset_service.KnowledgeRetrieveResponse, error)
	RetrieveAgentKnowledge(ctx context.Context, req dataset_service.KnowledgeRetrieveRequest) (*dataset_service.KnowledgeRetrieveResponse, error)
}

type Provider struct {
	*builtin.BuiltinProvider
	service RetrievalService
}

func NewProvider(service RetrievalService) *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Knowledge Tools",
			"zh_Hans": "知识库工具",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for listing and retrieving knowledge bases.",
			"zh_Hans": "用于列出和检索知识库的内置工具。",
		},
		Icon: "library",
		Tags: []string{"knowledge", "system"},
	}
	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
		service:         service,
	}
	provider.RegisterTool(newKnowledgeTool(service, ToolListAccessibleKnowledge))
	provider.RegisterTool(newKnowledgeTool(service, ToolRetrieveKnowledge))
	provider.RegisterTool(newKnowledgeTool(service, ToolRetrieveAgentKnowledge))
	return provider
}

type knowledgeTool struct {
	*builtin.BuiltinTool
	service RetrievalService
	kind    string
}

func newKnowledgeTool(service RetrievalService, kind string) tools.Tool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     kind,
			Author:   "System",
			Provider: ProviderID,
			Label:    tools.I18nText{"en_US": knowledgeToolLabel(kind), "zh_Hans": knowledgeToolLabel(kind)},
			Icon:     "library",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": knowledgeToolDescription(kind), "zh_Hans": knowledgeToolDescription(kind)},
			LLM:   knowledgeToolDescription(kind),
		},
		Parameters: knowledgeToolParameters(kind),
		OutputType: "json",
		Tags:       []string{"knowledge", "system"},
	}
	return &knowledgeTool{
		BuiltinTool: builtin.NewBuiltinTool(entity, ""),
		service:     service,
		kind:        kind,
	}
}

func (t *knowledgeTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = messageID
	if t.service == nil {
		return nil, fmt.Errorf("knowledge retrieval service is not configured")
	}
	scope, err := t.scope(userID, appID)
	if err != nil {
		return nil, err
	}
	if err := t.authorizeRuntime(); err != nil {
		return nil, err
	}
	switch t.kind {
	case ToolListAccessibleKnowledge:
		query := stringValue(params, "query")
		limit := intValue(params, "limit", defaultKnowledgeListToolLimit)
		datasets, err := t.service.ListAccessibleDatasets(ctx, scope, query, limit)
		if err != nil {
			return nil, err
		}
		return jsonMessages(map[string]interface{}{
			"knowledge_bases": datasets,
		})
	case ToolRetrieveKnowledge:
		req, err := knowledgeRetrieveRequest(scope, params)
		if err != nil {
			return nil, err
		}
		response, err := t.service.Retrieve(ctx, req)
		if err != nil {
			return nil, err
		}
		return retrievalMessages(response)
	case ToolRetrieveAgentKnowledge:
		req, err := agentKnowledgeRetrieveRequest(scope, params)
		if err != nil {
			return nil, err
		}
		response, err := t.service.RetrieveAgentKnowledge(ctx, req)
		if err != nil {
			return nil, err
		}
		return retrievalMessages(response)
	default:
		return nil, fmt.Errorf("unknown knowledge tool %s", t.kind)
	}
}

func (t *knowledgeTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &knowledgeTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
		service:     t.service,
		kind:        t.kind,
	}
}

func (t *knowledgeTool) scope(userID string, appID *string) (dataset_service.KnowledgeScope, error) {
	runtime := t.Runtime()
	workspaceID := t.GetTenantID()
	if runtime != nil && strings.TrimSpace(runtime.TenantID) != "" {
		workspaceID = runtime.TenantID
	}
	if strings.TrimSpace(workspaceID) == "" {
		return dataset_service.KnowledgeScope{}, fmt.Errorf("workspace_id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return dataset_service.KnowledgeScope{}, fmt.Errorf("account_id is required")
	}
	scope := dataset_service.KnowledgeScope{
		WorkspaceID: strings.TrimSpace(workspaceID),
		AccountID:   strings.TrimSpace(userID),
	}
	if appID != nil {
		scope.AppID = strings.TrimSpace(*appID)
	}
	return scope, nil
}

func (t *knowledgeTool) authorizeRuntime() error {
	runtime := t.Runtime()
	if runtime == nil {
		return nil
	}
	switch t.kind {
	case ToolListAccessibleKnowledge, ToolRetrieveKnowledge:
		if runtime.InvokeFrom != "" && runtime.InvokeFrom != tools.ToolInvokeFromAIChat {
			return fmt.Errorf("%s is only available to internal AIChat skills", t.kind)
		}
	case ToolRetrieveAgentKnowledge:
		if runtime.InvokeFrom != "" && runtime.InvokeFrom != tools.ToolInvokeFromAIChat && runtime.InvokeFrom != tools.ToolInvokeFromAgent {
			return fmt.Errorf("%s is only available to AIChat or Agent skill runtimes", t.kind)
		}
	}
	return nil
}

func knowledgeRetrieveRequest(scope dataset_service.KnowledgeScope, params map[string]interface{}) (dataset_service.KnowledgeRetrieveRequest, error) {
	query := strings.TrimSpace(stringValue(params, "query"))
	if query == "" {
		return dataset_service.KnowledgeRetrieveRequest{}, fmt.Errorf("query is required")
	}
	datasetIDs := stringListValue(params, "dataset_ids")
	if len(datasetIDs) == 0 {
		return dataset_service.KnowledgeRetrieveRequest{}, fmt.Errorf("dataset_ids are required")
	}
	return dataset_service.KnowledgeRetrieveRequest{
		Scope:         scope,
		Query:         query,
		DatasetIDs:    datasetIDs,
		TopK:          intValue(params, "top_k", defaultKnowledgeToolTopK),
		RetrievalMode: stringValue(params, "retrieval_mode"),
	}, nil
}

func agentKnowledgeRetrieveRequest(scope dataset_service.KnowledgeScope, params map[string]interface{}) (dataset_service.KnowledgeRetrieveRequest, error) {
	query := strings.TrimSpace(stringValue(params, "query"))
	if query == "" {
		return dataset_service.KnowledgeRetrieveRequest{}, fmt.Errorf("query is required")
	}
	return dataset_service.KnowledgeRetrieveRequest{
		Scope:         scope,
		Query:         query,
		TopK:          intValue(params, "top_k", defaultKnowledgeToolTopK),
		RetrievalMode: stringValue(params, "retrieval_mode"),
	}, nil
}

func retrievalMessages(response *dataset_service.KnowledgeRetrieveResponse) ([]tools.ToolInvokeMessage, error) {
	if response == nil {
		response = &dataset_service.KnowledgeRetrieveResponse{}
	}
	return []tools.ToolInvokeMessage{
		builtin.CreateJSONMessage(map[string]interface{}{
			"query":               response.Query,
			"context":             response.Context,
			"retriever_resources": response.Resources,
			"graph_executions":    response.GraphExecutions,
		}),
		{
			Type: tools.ToolInvokeMessageTypeRetrieverResources,
			Data: map[string]interface{}{
				"retriever_resources": response.Resources,
			},
		},
	}, nil
}

func jsonMessages(payload map[string]interface{}) ([]tools.ToolInvokeMessage, error) {
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func knowledgeToolLabel(kind string) string {
	switch kind {
	case ToolListAccessibleKnowledge:
		return "List Accessible Knowledge Bases"
	case ToolRetrieveKnowledge:
		return "Retrieve Knowledge"
	case ToolRetrieveAgentKnowledge:
		return "Retrieve Agent Knowledge"
	default:
		return kind
	}
}

func knowledgeToolDescription(kind string) string {
	switch kind {
	case ToolListAccessibleKnowledge:
		return "List knowledge bases the current user can access in the current workspace. Use before retrieve_knowledge."
	case ToolRetrieveKnowledge:
		return "Retrieve relevant context from explicitly selected knowledge base IDs. dataset_ids is required and must come from list_accessible_knowledge_bases."
	case ToolRetrieveAgentKnowledge:
		return "Retrieve relevant context only from knowledge bases configured on the current Agent. Ignores any dataset IDs supplied by the model."
	default:
		return kind
	}
}

func knowledgeToolParameters(kind string) []tools.ToolParameter {
	query := stringParam("query", "Query", "The user question or search query.", kind != ToolListAccessibleKnowledge)
	limit := numberParam("limit", "Limit", "Maximum number of knowledge bases to list. Defaults to 20.")
	topK := numberParam("top_k", "Top K", "Maximum number of retrieved chunks. Defaults to 5.")
	retrievalMode := selectParam("retrieval_mode", "Retrieval mode", "Optional retrieval mode: hybrid, vector, or graph.", []string{"hybrid", "vector", "graph"})
	datasetIDs := stringParam("dataset_ids", "Dataset IDs", "Required knowledge base IDs selected after listing. Pass a JSON array or comma-separated string.", true)

	switch kind {
	case ToolListAccessibleKnowledge:
		return []tools.ToolParameter{query, limit}
	case ToolRetrieveKnowledge:
		return []tools.ToolParameter{query, datasetIDs, topK, retrievalMode}
	case ToolRetrieveAgentKnowledge:
		return []tools.ToolParameter{query, topK, retrievalMode}
	default:
		return nil
	}
}

func stringParam(name, label, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeString,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func numberParam(name, label, description string) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeNumber,
		Form:            tools.ToolParameterFormLLM,
		Required:        false,
		SupportVariable: true,
	}
}

func selectParam(name, label, description string, values []string) tools.ToolParameter {
	options := make([]tools.ToolParameterOption, 0, len(values))
	for _, value := range values {
		options = append(options, tools.ToolParameterOption{
			Value: value,
			Label: tools.I18nText{"en_US": value, "zh_Hans": value},
		})
	}
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeSelect,
		Form:            tools.ToolParameterFormLLM,
		Required:        false,
		Options:         options,
		SupportVariable: true,
	}
}

func stringValue(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func intValue(params map[string]interface{}, key string, defaultValue int) int {
	if params == nil {
		return defaultValue
	}
	value, ok := params[key]
	if !ok || value == nil {
		return defaultValue
	}
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case json.Number:
		if parsed, err := strconv.Atoi(typed.String()); err == nil && parsed > 0 {
			return parsed
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultValue
}

func stringListValue(params map[string]interface{}, key string) []string {
	value, ok := params[key]
	if !ok || value == nil {
		return nil
	}
	var raw []string
	switch typed := value.(type) {
	case []string:
		raw = typed
	case []interface{}:
		for _, item := range typed {
			if str := strings.TrimSpace(fmt.Sprint(item)); str != "" {
				raw = append(raw, str)
			}
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		if strings.HasPrefix(trimmed, "[") {
			var parsed []string
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				raw = parsed
				break
			}
		}
		raw = strings.Split(trimmed, ",")
	default:
		raw = []string{fmt.Sprint(value)}
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

var _ tools.ToolProvider = (*Provider)(nil)
var _ tools.Tool = (*knowledgeTool)(nil)
