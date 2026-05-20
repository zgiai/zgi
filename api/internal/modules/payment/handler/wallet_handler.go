package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	platformconsole "github.com/zgiai/ginext/internal/infra/platform/console"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/middleware"
	"github.com/zgiai/ginext/pkg/response"
)

// WalletHandler handles wallet-related HTTP requests
type WalletHandler struct {
	accountService  interfaces.AccountService
	consoleProvider platformconsole.ConsoleProvider
}

// NewWalletHandler creates a new wallet handler
func NewWalletHandler(
	accountService interfaces.AccountService,
	consoleProvider platformconsole.ConsoleProvider,
) *WalletHandler {
	return &WalletHandler{
		accountService:  accountService,
		consoleProvider: consoleProvider,
	}
}

// WalletResponse represents the wallet information response
type WalletResponse struct {
	ID               string    `json:"id"`
	GroupID          string    `json:"group_id"`
	Currency         string    `json:"currency"`
	Balance          float64   `json:"balance"`
	FrozenBalance    float64   `json:"frozen_balance"`
	AvailableBalance float64   `json:"available_balance"`
	TotalRecharged   float64   `json:"total_recharged"`
	TotalConsumed    float64   `json:"total_consumed"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// GetWallet retrieves the wallet information for current user's group
// @Summary Get my wallet
// @Description Get wallet information for the current user's group (balance, frozen balance, etc.)
// @Tags Wallets
// @Produce json
// @Security BearerAuth
// @Success 200 {object} WalletResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/wallets/me [get]
func (h *WalletHandler) GetWallet(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Account ID not found")
		return
	}

	// Get user's group ID
	groupID, err := h.getWalletGroupID(c, accountID)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	wallet, err := h.consoleProvider.GetPaymentWallet(c.Request.Context(), groupID.String(), "CNY")
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to get wallet")
		return
	}

	balance, err := strconv.ParseFloat(wallet.Balance, 64)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Invalid wallet balance")
		return
	}
	frozen, err := strconv.ParseFloat(wallet.FrozenBalance, 64)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Invalid wallet frozen balance")
		return
	}
	totalRecharged, err := strconv.ParseFloat(wallet.TotalRecharged, 64)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Invalid wallet total_recharged")
		return
	}
	totalConsumed, err := strconv.ParseFloat(wallet.TotalConsumed, 64)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Invalid wallet total_consumed")
		return
	}

	resp := WalletResponse{
		ID:               wallet.ID,
		GroupID:          wallet.TenantID,
		Currency:         wallet.Currency,
		Balance:          balance,
		FrozenBalance:    frozen,
		AvailableBalance: balance - frozen,
		TotalRecharged:   totalRecharged,
		TotalConsumed:    totalConsumed,
		CreatedAt:        time.Time{},
		UpdatedAt:        time.Time{},
	}

	response.Success(c, resp)
}

// getWalletGroupID gets the group ID for the current user's wallet
// It tries to get from context first (set by auth middleware), then falls back to EnsureCurrentOrganizationID
func (h *WalletHandler) getWalletGroupID(c *gin.Context, accountID uuid.UUID) (uuid.UUID, error) {
	// First try to get from context (set by middleware)
	tenantIDStr, exists := c.Get("tenant_id")
	if exists {
		switch v := tenantIDStr.(type) {
		case string:
			return uuid.Parse(v)
		case uuid.UUID:
			return v, nil
		}
	}

	// Fallback: get user's current tenant ID via account service
	tenantID, err := h.accountService.EnsureCurrentOrganizationID(c.Request.Context(), accountID.String())
	if err != nil {
		return uuid.Nil, err
	}

	parsed, err := uuid.Parse(tenantID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid tenant_id from account context: %w", err)
	}
	return parsed, nil
}

// RegisterRoutes registers wallet-related routes
func (h *WalletHandler) RegisterRoutes(router *gin.RouterGroup) {
	// Wallet routes - /wallets/me for current user's wallet info
	wallets := router.Group("/wallets", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		wallets.GET("/me", h.GetWallet)
	}
}
