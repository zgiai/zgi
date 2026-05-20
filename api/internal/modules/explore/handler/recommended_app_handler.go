package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/explore/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

// RecommendedAppHandler handles HTTP requests for recommended apps
type RecommendedAppHandler struct {
	service        service.RecommendedAppService
	accountService interfaces.AccountService
}

// NewRecommendedAppHandler creates a new recommended app handler
func NewRecommendedAppHandler(service service.RecommendedAppService, accountService interfaces.AccountService) *RecommendedAppHandler {
	return &RecommendedAppHandler{
		service:        service,
		accountService: accountService,
	}
}

// GetRecommendedAppList handles GET /explore/apps
// @Summary Get recommended apps list
// @Description Get recommended apps and categories based on language preference
// @Tags Explore
// @Accept json
// @Produce json
// @Param language query string false "Language code (e.g., en-US, zh-CN)"
// @Success 200 {object} model.RecommendedAppListResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /console/api/explore/apps [get]
func (h *RecommendedAppHandler) GetRecommendedAppList(c *gin.Context) {
	// Get language from query parameter, default to en-US
	language := c.DefaultQuery("language", "")

	// If no language specified in query, try to get user's interface language preference
	if language == "" {
		accountID := c.GetString("account_id")
		if accountID != "" {
			// Get user's profile to check interface language setting
			profile, err := h.accountService.GetAccountProfile(c.Request.Context(), accountID)
			if err == nil && profile != nil && profile.InterfaceLanguage != "" {
				language = profile.InterfaceLanguage
			}
		}
	}

	// Final fallback to en-US if still no language determined
	if language == "" {
		language = "en-US"
	}

	// Get recommended apps and categories
	result, err := h.service.GetRecommendedAppsAndCategories(language)
	if err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        300001,
			Message:     "Failed to get recommended apps: " + err.Error(),
			UserVisible: true,
		})
		return
	}

	response.Success(c, result)
}

// GetRecommendedAppDetail handles GET /explore/apps/:app_id
// @Summary Get recommended app detail
// @Description Get detailed information for a specific recommended app
// @Tags Explore
// @Accept json
// @Produce json
// @Param app_id path string true "App ID (UUID)"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /console/api/explore/apps/{app_id} [get]
func (h *RecommendedAppHandler) GetRecommendedAppDetail(c *gin.Context) {
	appID := c.Param("app_id")

	if appID == "" {
		response.Fail(c, response.ErrorCode{
			Code:        100001,
			Message:     "app_id is required",
			UserVisible: true,
		})
		return
	}

	// Get app detail
	result, err := h.service.GetRecommendAppDetail(appID)
	if err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        404001,
			Message:     "App not found: " + err.Error(),
			UserVisible: true,
		})
		return
	}

	response.Success(c, result)
}

// RegisterRoutes registers all explore routes
func (h *RecommendedAppHandler) RegisterRoutes(router *gin.RouterGroup) {
	exploreGroup := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))

	// Register routes
	exploreGroup.GET("/explore/apps", h.GetRecommendedAppList)
	exploreGroup.GET("/explore/apps/:app_id", h.GetRecommendedAppDetail)
}
