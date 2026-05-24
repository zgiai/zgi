package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/modules/memory"
	pluginmodel "github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// BuiltinToolsHandler handles builtin tools API requests
type BuiltinToolsHandler struct {
	toolManager                *tools.ToolManager
	accountInstallationService service.AccountInstallationService // For reading from database
	memberSubscriptionService  service.MemberSubscriptionService  // For member-level subscriptions
}

// NewBuiltinToolsHandler creates a new builtin tools handler
func NewBuiltinToolsHandler(toolManager *tools.ToolManager, accountInstallSvc service.AccountInstallationService, memberSubSvc service.MemberSubscriptionService) *BuiltinToolsHandler {
	return &BuiltinToolsHandler{
		toolManager:                toolManager,
		accountInstallationService: accountInstallSvc,
		memberSubscriptionService:  memberSubSvc,
	}
}

// BuiltinProviderResponse represents a builtin provider in API response format
type BuiltinProviderResponse struct {
	ID          string                `json:"id"`
	Author      string                `json:"author,omitempty"`
	Name        string                `json:"name"`
	Description tools.I18nText        `json:"description"`
	Icon        string                `json:"icon,omitempty"`
	Label       tools.I18nText        `json:"label"`
	Type        string                `json:"type"`
	Tags        []string              `json:"tags,omitempty"`
	Tools       []BuiltinToolResponse `json:"tools"`
}

// BuiltinToolResponse represents a tool in API response format
type BuiltinToolResponse struct {
	Name             string                          `json:"name"`
	Author           string                          `json:"author,omitempty"`
	Label            tools.I18nText                  `json:"label"`
	Description      tools.ToolDescription           `json:"description"`
	Parameters       []BuiltinParameterResponse      `json:"parameters"`
	ConfigParameters BuiltinConfigParametersResponse `json:"config_parameters"`
	Tags             []string                        `json:"tags,omitempty"`
	OutputSchema     map[string]interface{}          `json:"output_schema,omitempty"`
}

// BuiltinParameterResponse represents a parameter in API response format
type BuiltinParameterResponse struct {
	Name             string                  `json:"name"`
	Label            tools.I18nText          `json:"label"`
	Placeholder      tools.I18nText          `json:"placeholder,omitempty"`
	Required         bool                    `json:"required"`
	Default          interface{}             `json:"default,omitempty"`
	Min              *float64                `json:"min,omitempty"`
	Max              *float64                `json:"max,omitempty"`
	Options          []BuiltinOptionResponse `json:"options,omitempty"`
	Type             string                  `json:"type"`
	HumanDescription tools.I18nText          `json:"human_description,omitempty"`
	Form             string                  `json:"form"`
	LLMDescription   string                  `json:"llm_description,omitempty"`
	SupportVariable  bool                    `json:"support_variable"` // Controls whether the {x} variable input button is shown
}

// BuiltinConfigParametersResponse represents configuration parameters for a tool
type BuiltinConfigParametersResponse struct {
	Enable     bool                             `json:"enable"`
	Parameters []BuiltinConfigParameterResponse `json:"parameters"`
}

// BuiltinConfigParameterResponse represents a configuration parameter
type BuiltinConfigParameterResponse struct {
	Name             string                  `json:"name"`
	Type             string                  `json:"type"`
	Required         bool                    `json:"required"`
	Label            tools.I18nText          `json:"label"`
	Help             tools.I18nText          `json:"help,omitempty"`
	Placeholder      tools.I18nText          `json:"placeholder,omitempty"`
	Default          interface{}             `json:"default,omitempty"`
	Options          []BuiltinOptionResponse `json:"options,omitempty"`
	HumanDescription tools.I18nText          `json:"human_description,omitempty"`
}

// BuiltinOptionResponse represents an option in API response format
type BuiltinOptionResponse struct {
	Value string         `json:"value"`
	Label tools.I18nText `json:"label"`
}

// ListBuiltinProviders returns all builtin tool providers
// @Summary List builtin tool providers
// @Description Retrieves all system-provided builtin tool providers with their tools and parameters
// @Tags Builtin Tools
// @Produce json
// @Success 200 {object} response.Response{data=[]BuiltinProviderResponse} "List of builtin providers"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/tools/builtin [get]
func (h *BuiltinToolsHandler) ListBuiltinProviders(c *gin.Context) {
	logger.Info("API: Getting all builtin tool providers")

	var responses []BuiltinProviderResponse

	// 1. Get hardcoded builtin providers from tool manager (e.g., time)
	providers := h.toolManager.ListProviders(tools.ToolProviderTypeBuiltin)
	for _, provider := range providers {
		entity := provider.GetEntity()
		if isHiddenBuiltinProvider(entity) {
			continue
		}
		providerResp := h.convertProviderEntityToResponse(entity)
		responses = append(responses, providerResp)
	}

	// 2. Get installed plugins from database (e.g., regex, email)
	if h.accountInstallationService != nil {
		organizationID := getOrganizationIDFromContext(c)
		if organizationID == "" {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}

		accountID := c.GetString("account_id")
		if accountID == "" {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		var declarations []pluginmodel.PluginDeclaration
		var err error

		// Admin/Owner are treated as subscribed to all organization installations.
		if middleware.IsOrganizationAdminOrOwner(c) {
			declarations, err = h.accountInstallationService.ListDeclarationsByTenant(c.Request.Context(), organizationID)
		} else {
			if h.memberSubscriptionService != nil {
				declarations, err = h.memberSubscriptionService.ListSubscribedDeclarations(c.Request.Context(), organizationID, accountID)
			}
		}

		if err != nil {
			logger.Warn("Failed to get plugin declarations", "error", err)
		} else {
			for _, decl := range declarations {
				responses = append(responses, h.convertDeclarationToResponse(decl))
			}
		}
	} else {
		logger.Warn("accountInstallationService is nil, skipping plugin providers")
	}

	logger.Info("Successfully retrieved builtin providers", "count", len(responses))
	response.Success(c, responses)
}

// GetBuiltinProvider returns a specific builtin provider
// @Summary Get builtin provider by name
// @Description Retrieves a specific builtin tool provider by its name (supports both builtin and plugin_runner types)
// @Tags Builtin Tools
// @Param provider path string true "Provider name"
// @Produce json
// @Success 200 {object} response.Response{data=BuiltinProviderResponse} "Provider details"
// @Failure 404 {object} response.Response "Provider not found"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/tools/builtin/{provider} [get]
func (h *BuiltinToolsHandler) GetBuiltinProvider(c *gin.Context) {
	providerName := c.Param("provider")
	if providerName == "" {
		response.Fail(c, response.ErrInvalidParams)
		return
	}

	logger.Info("API: Getting builtin provider", "provider", providerName)

	// 1. First try to get from builtin providers (toolManager)
	provider, err := h.toolManager.GetProvider(c.Request.Context(), tools.ToolProviderTypeBuiltin, providerName, "")
	if err == nil {
		entity := provider.GetEntity()
		if isHiddenBuiltinProvider(entity) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		providerResp := h.convertProviderEntityToResponse(entity)
		logger.Info("Successfully retrieved builtin provider", "provider", providerName, "type", "builtin")
		response.Success(c, providerResp)
		return
	}

	// 2. If not found in builtin, try to get from installed plugins (database)
	if h.accountInstallationService != nil {
		organizationID := getOrganizationIDFromContext(c)
		if organizationID == "" {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}

		accountID := c.GetString("account_id")
		if accountID == "" {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		if middleware.IsOrganizationAdminOrOwner(c) {
			declaration, err := h.accountInstallationService.GetDeclarationByProviderName(c.Request.Context(), organizationID, providerName)
			if err == nil && declaration != nil {
				providerResp := h.convertDeclarationToResponse(*declaration)
				logger.Info("Successfully retrieved plugin provider", "provider", providerName, "type", "plugin_runner")
				response.Success(c, providerResp)
				return
			}
		} else if h.memberSubscriptionService != nil {
			declarations, err := h.memberSubscriptionService.ListSubscribedDeclarations(c.Request.Context(), organizationID, accountID)
			if err == nil {
				for _, decl := range declarations {
					if decl.Provider.Name == providerName {
						providerResp := h.convertDeclarationToResponse(decl)
						logger.Info("Successfully retrieved subscribed plugin provider", "provider", providerName, "type", "plugin_runner")
						response.Success(c, providerResp)
						return
					}
				}
			}
		}
	}

	// 3. Not found in either source
	logger.Warn("Provider not found", "provider", providerName)
	response.Fail(c, response.ErrNotFound)
}

func isHiddenBuiltinProvider(entity tools.ToolProviderEntity) bool {
	for _, tag := range entity.Identity.Tags {
		if tag == memory.HiddenProviderTag {
			return true
		}
	}
	return false
}

// convertProviderEntityToResponse converts ToolProviderEntity to API response format
func (h *BuiltinToolsHandler) convertProviderEntityToResponse(entity tools.ToolProviderEntity) BuiltinProviderResponse {
	var toolResponses []BuiltinToolResponse
	for _, tool := range entity.Tools {
		toolResp := h.convertToolEntityToResponse(tool)
		toolResponses = append(toolResponses, toolResp)
	}

	return BuiltinProviderResponse{
		ID:          entity.Identity.Name,
		Author:      entity.Identity.Author,
		Name:        entity.Identity.Name,
		Description: entity.Identity.Description,
		Icon:        entity.Identity.Icon,
		Label:       entity.Identity.Label,
		Type:        "builtin",
		Tags:        entity.Identity.Tags,
		Tools:       toolResponses,
	}
}

// convertToolEntityToResponse converts ToolEntity to API response format
func (h *BuiltinToolsHandler) convertToolEntityToResponse(entity tools.ToolEntity) BuiltinToolResponse {
	var params []BuiltinParameterResponse
	for _, param := range entity.Parameters {
		paramResp := h.convertParameterToResponse(param)
		params = append(params, paramResp)
	}

	configParameters := BuiltinConfigParametersResponse{
		Enable:     false,
		Parameters: []BuiltinConfigParameterResponse{},
	}

	return BuiltinToolResponse{
		Name:             entity.Identity.Name,
		Author:           entity.Identity.Author,
		Label:            entity.Identity.Label,
		Description:      entity.Description,
		Parameters:       params,
		ConfigParameters: configParameters,
		Tags:             entity.Tags,
	}
}

// convertParameterToResponse converts ToolParameter to API response format
func (h *BuiltinToolsHandler) convertParameterToResponse(param tools.ToolParameter) BuiltinParameterResponse {
	var options []BuiltinOptionResponse
	for _, opt := range param.Options {
		options = append(options, BuiltinOptionResponse{
			Value: opt.Value,
			Label: opt.Label,
		})
	}

	return BuiltinParameterResponse{
		Name:             param.Name,
		Label:            param.Label,
		Placeholder:      param.Placeholder,
		Required:         param.Required,
		Default:          param.Default,
		Min:              param.MinValue,
		Max:              param.MaxValue,
		Options:          options,
		Type:             string(param.Type),
		HumanDescription: param.HumanDescription,
		Form:             string(param.Form),
		LLMDescription:   param.LLMDescription,
		SupportVariable:  param.SupportVariable,
	}
}

// convertDeclarationToResponse converts database PluginDeclaration to API response format
func (h *BuiltinToolsHandler) convertDeclarationToResponse(decl pluginmodel.PluginDeclaration) BuiltinProviderResponse {
	var toolResponses []BuiltinToolResponse
	for _, tool := range decl.Tools {
		var params []BuiltinParameterResponse
		for _, param := range tool.Parameters {
			// Convert options from database format to API format
			var options []BuiltinOptionResponse
			for _, opt := range param.Options {
				options = append(options, BuiltinOptionResponse{
					Value: formatOptionValue(opt.Value),
					Label: opt.Label,
				})
			}
			params = append(params, BuiltinParameterResponse{
				Name:             param.Name,
				Label:            param.Label,
				Required:         param.Required,
				Type:             param.Type,
				HumanDescription: param.HumanDescription,
				Form:             param.Form,
				LLMDescription:   param.LLMDescription,
				Default:          param.Default,
				Options:          options,
			})
		}
		configParams := make([]BuiltinConfigParameterResponse, 0, len(tool.Configurations))
		for _, config := range tool.Configurations {
			var options []BuiltinOptionResponse
			for _, opt := range config.Options {
				options = append(options, BuiltinOptionResponse{
					Value: formatOptionValue(opt.Value),
					Label: opt.Label,
				})
			}
			configParams = append(configParams, BuiltinConfigParameterResponse{
				Name:             config.Name,
				Type:             config.Type,
				Required:         config.Required,
				Label:            config.Label,
				Help:             config.Help,
				Placeholder:      config.Placeholder,
				Default:          config.Default,
				Options:          options,
				HumanDescription: config.HumanDescription,
			})
		}

		configParameters := BuiltinConfigParametersResponse{
			Enable:     len(configParams) > 0,
			Parameters: configParams,
		}

		toolResponses = append(toolResponses, BuiltinToolResponse{
			Name:             tool.Name,
			Label:            tool.Label,
			Description:      tools.ToolDescription{Human: tool.Description.Human, LLM: tool.Description.LLM},
			Parameters:       params,
			ConfigParameters: configParameters,
		})
	}

	return BuiltinProviderResponse{
		ID:          decl.Provider.Name,
		Author:      decl.Provider.Author,
		Name:        decl.Provider.Name,
		Description: decl.Provider.Description,
		Icon:        decl.Provider.Icon,
		Label:       decl.Provider.Label,
		Type:        "plugin_runner",
		Tags:        decl.Provider.Tags,
		Tools:       toolResponses,
	}
}

func formatOptionValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprint(value)
}

func getOrganizationIDFromContext(c *gin.Context) string {
	organizationID := c.GetString("organization_id")
	if organizationID != "" {
		return organizationID
	}
	return c.GetString("tenant_id")
}
