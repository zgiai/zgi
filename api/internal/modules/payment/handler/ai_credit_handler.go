package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/payment/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

// AICreditHandler handles AI credit-related HTTP requests
type AICreditHandler struct {
	service         *service.AICreditService
	accountService  interfaces.AccountService
	consoleProvider platformconsole.ConsoleProvider
}

// NewAICreditHandler creates a new AI credit handler
func NewAICreditHandler(
	service *service.AICreditService,
	accountService interfaces.AccountService,
	consoleProvider platformconsole.ConsoleProvider,
) *AICreditHandler {
	return &AICreditHandler{
		service:         service,
		accountService:  accountService,
		consoleProvider: consoleProvider,
	}
}

// ConsumeCreditsRequest represents the request to consume credits
type ConsumeCreditsRequest struct {
	Amount      int64                  `json:"amount" binding:"required"`
	RelatedID   *string                `json:"related_id"`
	Description string                 `json:"description" binding:"required"`
	UsageDetail map[string]interface{} `json:"usage_detail"`
}

// ListProducts returns active AI credit packages from console-api.
func (h *AICreditHandler) ListProducts(c *gin.Context) {
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	products, err := h.consoleProvider.ListCreditProducts(c.Request.Context())
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to list AI credit products")
		return
	}
	response.Success(c, products)
}

// GetMyAccount retrieves current user's AI credit account
// @Summary Get my AI credit account
// @Description Get current user's official AI credit and private channel funds overview
// @Tags AI Credits
// @Produce json
// @Success 200 {object} service.AICreditAccountOverview
// @Failure 503 {object} response.ErrorResponse
// @Router /api/v1/ai-credits/me [get]
func (h *AICreditHandler) GetMyAccount(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Account ID not found")
		return
	}

	groupID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	account, err := h.service.GetMyAccountOverview(c.Request.Context(), accountID, groupID)
	if err != nil {
		if errors.Is(err, service.ErrOfficialCreditUnavailable) {
			errorResponse(c, http.StatusServiceUnavailable, "Official AI credit service unavailable")
			return
		}
		errorResponse(c, http.StatusInternalServerError, "Failed to get AI credit account overview")
		return
	}

	response.Success(c, account)
}

// ConsumeCredits consumes credits from user's account
// @Summary Consume credits
// @Description Consume AI credits (Internal API, called by AI service)
// @Tags AI Credits
// @Accept json
// @Produce json
// @Param request body ConsumeCreditsRequest true "Consume request"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/ai-credits/consume [post]
func (h *AICreditHandler) ConsumeCredits(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Account ID not found")
		return
	}

	groupID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	var req ConsumeCreditsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if err := h.service.ConsumeCredits(
		c.Request.Context(),
		accountID,
		groupID,
		req.Amount,
		req.RelatedID,
		req.Description,
		req.UsageDetail,
	); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Credits consumed successfully"})
}

// GetTransactionHistory retrieves user's transaction history
// @Summary Get transaction history
// @Description Get AI credit transaction history
// @Tags AI Credits
// @Produce json
// @Param currency_type query string false "Currency type: purchased_credits (empty for all credits)"
// @Param limit query int false "Limit" default(20)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} model.Transaction
// @Router /api/v1/ai-credits/transactions [get]
func (h *AICreditHandler) GetTransactionHistory(c *gin.Context) {
	groupID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	currencyType := c.Query("currency_type")

	limit := getIntQuery(c, "limit", 20)
	offset := getIntQuery(c, "offset", 0)

	transactions, err := h.service.GetTransactionHistory(
		c.Request.Context(),
		groupID,
		currencyType,
		limit,
		offset,
	)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to get transactions")
		return
	}

	response.Success(c, transactions)
}

// GetTransaction retrieves a specific transaction
// @Summary Get transaction
// @Description Get AI credit transaction details
// @Tags AI Credits
// @Produce json
// @Param id path string true "Transaction ID"
// @Success 200 {object} model.Transaction
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/ai-credits/transactions/{id} [get]
func (h *AICreditHandler) GetTransaction(c *gin.Context) {
	id := c.Param("id")

	transaction, err := h.service.GetTransactionByID(c.Request.Context(), id)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "Transaction not found")
		return
	}

	response.Success(c, transaction)
}

// RegisterRoutes registers AI credit-related routes
func (h *AICreditHandler) RegisterRoutes(router *gin.RouterGroup) {
	creditProducts := router.Group("/ai-credit-products")
	{
		creditProducts.GET("", h.ListProducts)
	}

	// AI Credits (User - requires authentication)
	aiCredits := router.Group("/ai-credits", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		aiCredits.GET("/me", h.GetMyAccount)
		// aiCredits.POST("/consume", h.ConsumeCredits)
		aiCredits.GET("/transactions", h.GetTransactionHistory)
		aiCredits.GET("/transactions/:id", h.GetTransaction)
	}
}
