package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	modelsProtocolResponses = "openai_responses"
	modelsProtocolAnthropic = "anthropic_messages"
	defaultContextWindow    = 128000
	defaultMaxTokens        = 8192
	codexBaseInstructions   = "You are Codex, a coding agent. Follow the user's instructions, inspect the workspace as needed, use the available tools, and report the verified result concisely."
)

type anthropicCatalogModel struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`
	DisplayName    string   `json:"display_name"`
	CreatedAt      string   `json:"created_at"`
	Capabilities   []string `json:"capabilities"`
	MaxInputTokens int      `json:"max_input_tokens"`
	MaxTokens      int      `json:"max_tokens"`
}

type anthropicCatalogResponse struct {
	Data    []anthropicCatalogModel `json:"data"`
	FirstID *string                 `json:"first_id"`
	LastID  *string                 `json:"last_id"`
	HasMore bool                    `json:"has_more"`
}

func writeModelsCatalog(c *gin.Context, models []adapter.Model) {
	models = mergeCatalogLogicalModels(models)
	if strings.TrimSpace(c.Query("client_version")) != "" {
		writeCodexModels(c, filterCatalogModels(models, modelsProtocolResponses))
		return
	}
	if strings.TrimSpace(c.GetHeader("anthropic-version")) != "" {
		writeAnthropicModels(c, filterCatalogModels(models, modelsProtocolAnthropic))
		return
	}
	modelList := make([]gin.H, 0, len(models))
	for _, model := range models {
		modelList = append(modelList, gin.H{"id": model.ID, "object": "model", "created": model.Created, "owned_by": "zgi", "supported_protocols": model.SupportedProtocols})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": modelList})
}

func writeModelsDetail(c *gin.Context, models []adapter.Model) {
	models = mergeCatalogLogicalModels(models)
	protocol := ""
	if strings.TrimSpace(c.Query("client_version")) != "" {
		protocol = modelsProtocolResponses
	} else if strings.TrimSpace(c.GetHeader("anthropic-version")) != "" {
		protocol = modelsProtocolAnthropic
	}
	modelID := strings.TrimSpace(c.Param("model_id"))
	for _, model := range filterCatalogModels(models, protocol) {
		if model.ID != modelID {
			continue
		}
		switch protocol {
		case modelsProtocolResponses:
			c.JSON(http.StatusOK, codexCatalogModel(model, 0))
		case modelsProtocolAnthropic:
			c.JSON(http.StatusOK, anthropicCatalogModelInfo(model))
		default:
			c.JSON(http.StatusOK, gin.H{"id": model.ID, "object": "model", "created": model.Created, "owned_by": "zgi", "supported_protocols": model.SupportedProtocols})
		}
		return
	}
	if protocol == modelsProtocolAnthropic {
		writeAnthropicCatalogError(c, http.StatusNotFound, "not_found_error", "Model not found")
		return
	}
	writeOpenAIProtocolError(c, protocolError{openAIStatus: http.StatusNotFound, openAIType: "invalid_request_error", openAICode: "model_not_found", message: "Model not found"})
}

func mergeCatalogLogicalModels(models []adapter.Model) []adapter.Model {
	merged := make([]adapter.Model, 0, len(models))
	indexes := make(map[string]int, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		model.OwnedBy = "zgi"
		index, found := indexes[model.ID]
		if !found {
			model.SupportedProtocols = appendUniqueCatalogStrings(nil, model.SupportedProtocols...)
			model.Capabilities = appendUniqueCatalogStrings(nil, model.Capabilities...)
			model.InputModalities = appendUniqueCatalogStrings(nil, model.InputModalities...)
			model.Endpoints = appendUniqueCatalogStrings(nil, model.Endpoints...)
			indexes[model.ID] = len(merged)
			merged = append(merged, model)
			continue
		}

		current := &merged[index]
		current.SupportedProtocols = appendUniqueCatalogStrings(current.SupportedProtocols, model.SupportedProtocols...)
		current.Capabilities = appendUniqueCatalogStrings(current.Capabilities, model.Capabilities...)
		current.InputModalities = appendUniqueCatalogStrings(current.InputModalities, model.InputModalities...)
		current.Endpoints = appendUniqueCatalogStrings(current.Endpoints, model.Endpoints...)
		current.ParallelToolCalls = current.ParallelToolCalls || model.ParallelToolCalls
		if current.Name == "" {
			current.Name = model.Name
		}
		if current.Description == "" {
			current.Description = model.Description
		}
		if current.Created <= 0 {
			current.Created = model.Created
		}
		if model.ContextLength > current.ContextLength {
			current.ContextLength = model.ContextLength
		}
		if model.MaxInputTokens > current.MaxInputTokens {
			current.MaxInputTokens = model.MaxInputTokens
		}
		if model.MaxOutputTokens > current.MaxOutputTokens {
			current.MaxOutputTokens = model.MaxOutputTokens
		}
	}
	return merged
}

func appendUniqueCatalogStrings(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func filterCatalogModels(models []adapter.Model, protocol string) []adapter.Model {
	if protocol == "" {
		return models
	}
	filtered := make([]adapter.Model, 0, len(models))
	for _, model := range models {
		for _, supported := range model.SupportedProtocols {
			if supported == protocol {
				filtered = append(filtered, model)
				break
			}
		}
	}
	return filtered
}

func writeCodexModels(c *gin.Context, models []adapter.Model) {
	items := make([]gin.H, 0, len(models))
	for index, model := range models {
		items = append(items, codexCatalogModel(model, index+1))
	}
	c.JSON(http.StatusOK, gin.H{"models": items})
}

func codexCatalogModel(model adapter.Model, priority int) gin.H {
	contextWindow := positiveCatalogValue(model.ContextLength, defaultContextWindow)
	modalities := append([]string(nil), model.InputModalities...)
	if len(modalities) == 0 {
		modalities = []string{"text"}
	}
	toolMode := any(nil)
	if catalogModelHasCapability(model, "codex_code_mode") {
		toolMode = "code_mode_only"
	}
	multiAgentVersion := any(nil)
	if catalogModelHasCapability(model, "codex_multi_agent_v2") {
		multiAgentVersion = "v2"
	} else if catalogModelHasCapability(model, "codex_multi_agent_v1") {
		multiAgentVersion = "v1"
	}
	return gin.H{
		"slug": model.ID, "display_name": catalogDisplayName(model), "description": catalogDescription(model),
		"base_instructions":       codexBaseInstructions,
		"default_reasoning_level": "medium", "supported_reasoning_levels": []gin.H{
			{"effort": "low", "description": "Fast responses with lighter reasoning"},
			{"effort": "medium", "description": "Balances speed and reasoning depth"},
			{"effort": "high", "description": "Greater reasoning depth for complex tasks"},
		},
		"shell_type": "shell_command", "visibility": "list", "supported_in_api": true, "priority": priority,
		"additional_speed_tiers": []string{}, "service_tiers": []any{}, "availability_nux": nil, "upgrade": nil,
		"include_skills_usage_instructions": catalogModelHasCapability(model, "codex_skills"),
		"include_plugin_usage_instructions": catalogModelHasCapability(model, "codex_plugins"),
		"include_apps_usage_instructions":   catalogModelHasCapability(model, "codex_apps"),
		"default_reasoning_summary":         "none", "support_verbosity": false, "default_verbosity": "medium",
		"apply_patch_tool_type": "freeform", "web_search_tool_type": "text_and_image", "truncation_policy": gin.H{"mode": "bytes", "limit": 10000},
		"supports_parallel_tool_calls": model.ParallelToolCalls, "supports_image_detail_original": false,
		"context_window": contextWindow, "max_context_window": contextWindow, "effective_context_window_percent": 95,
		"experimental_supported_tools": []string{}, "input_modalities": modalities, "supports_search_tool": false,
		"use_responses_lite": false, "node_repl_auto_review_required": false, "node_repl_disabled": false,
		"tool_mode": toolMode, "multi_agent_version": multiAgentVersion, "comp_hash": "",
	}
}

func catalogModelHasCapability(model adapter.Model, capability string) bool {
	for _, supported := range model.Capabilities {
		if supported == capability {
			return true
		}
	}
	return false
}

func writeAnthropicModels(c *gin.Context, models []adapter.Model) {
	limit, err := parseCatalogLimit(c.Query("limit"))
	if err != nil {
		writeAnthropicCatalogError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	afterID, beforeID := strings.TrimSpace(c.Query("after_id")), strings.TrimSpace(c.Query("before_id"))
	if afterID != "" && beforeID != "" {
		writeAnthropicCatalogError(c, http.StatusBadRequest, "invalid_request_error", "after_id and before_id cannot be used together")
		return
	}
	start, end := 0, len(models)
	if afterID != "" {
		index := catalogModelIndex(models, afterID)
		if index < 0 {
			writeAnthropicCatalogError(c, http.StatusBadRequest, "invalid_request_error", "Invalid after_id")
			return
		}
		start = index + 1
	}
	if beforeID != "" {
		index := catalogModelIndex(models, beforeID)
		if index < 0 {
			writeAnthropicCatalogError(c, http.StatusBadRequest, "invalid_request_error", "Invalid before_id")
			return
		}
		end = index
	}
	if start > end {
		start = end
	}
	hasMore := end-start > limit
	if start+limit < end {
		end = start + limit
	}
	data := make([]anthropicCatalogModel, 0, end-start)
	for _, model := range models[start:end] {
		data = append(data, anthropicCatalogModelInfo(model))
	}
	response := anthropicCatalogResponse{Data: data, HasMore: hasMore}
	if len(data) > 0 {
		response.FirstID = &data[0].ID
		response.LastID = &data[len(data)-1].ID
	}
	c.JSON(http.StatusOK, response)
}

func anthropicCatalogModelInfo(model adapter.Model) anthropicCatalogModel {
	created := model.Created
	if created <= 0 {
		created = time.Now().Unix()
	}
	capabilities := append([]string(nil), model.Capabilities...)
	if len(capabilities) == 0 {
		capabilities = []string{"messages"}
	}
	maxInput := model.MaxInputTokens
	if maxInput <= 0 {
		maxInput = model.ContextLength
	}
	return anthropicCatalogModel{ID: model.ID, Type: "model", DisplayName: catalogDisplayName(model), CreatedAt: time.Unix(created, 0).UTC().Format(time.RFC3339), Capabilities: capabilities, MaxInputTokens: positiveCatalogValue(maxInput, defaultContextWindow), MaxTokens: positiveCatalogValue(model.MaxOutputTokens, defaultMaxTokens)}
}

func writeAnthropicCatalogError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{"type": "error", "error": gin.H{"type": errType, "message": message}})
}

func parseCatalogLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 20, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 1000 {
		return 0, &catalogLimitError{}
	}
	return value, nil
}

type catalogLimitError struct{}

func (*catalogLimitError) Error() string { return "limit must be an integer between 1 and 1000" }

func catalogModelIndex(models []adapter.Model, id string) int {
	for index, model := range models {
		if model.ID == id {
			return index
		}
	}
	return -1
}

func catalogDisplayName(model adapter.Model) string {
	if name := strings.TrimSpace(model.Name); name != "" {
		return name
	}
	return model.ID
}

func catalogDescription(model adapter.Model) string {
	if description := strings.TrimSpace(model.Description); description != "" {
		return description
	}
	return catalogDisplayName(model)
}

func positiveCatalogValue(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
