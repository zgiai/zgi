package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	paymentMethodWallet       = "wallet"
	orderTypeCreditPurchase   = "credit_purchase"
	orderTypeWalletRecharge   = "wallet_recharge"
	productCodeWalletRecharge = "wallet_recharge"
	currencyCNY               = "CNY"
)

// OrderHandler handles order-related HTTP requests
type OrderHandler struct {
	accountService           interfaces.AccountService
	consoleProvider          platformconsole.ConsoleProvider
	organizationAdminChecker func(ctx context.Context, organizationID, accountID string) (bool, error)
}

// NewOrderHandler creates a new order handler
func NewOrderHandler(
	accountService interfaces.AccountService,
	consoleProvider platformconsole.ConsoleProvider,
) *OrderHandler {
	return &OrderHandler{
		accountService:  accountService,
		consoleProvider: consoleProvider,
	}
}

// CreateOrderRequest represents the request to create an order
type CreateOrderRequest struct {
	OrderType       string                 `json:"order_type" binding:"required"` // wallet_recharge
	ProductCode     string                 `json:"product_code" binding:"required"`
	ProductName     string                 `json:"product_name" binding:"required"`
	Quantity        int                    `json:"quantity" binding:"required"`
	OriginalAmount  float64                `json:"original_amount" binding:"required"`
	DiscountAmount  float64                `json:"discount_amount"`
	FinalAmount     float64                `json:"final_amount" binding:"required"`
	Currency        string                 `json:"currency" binding:"required"`
	ProductSnapshot map[string]interface{} `json:"product_snapshot" binding:"required"`
	SubscriptionID  *string                `json:"subscription_id"`
	CouponID        *string                `json:"coupon_id"`
	CouponCode      *string                `json:"coupon_code"`
}

// CancelOrderRequest represents the request to cancel an order
type CancelOrderRequest struct {
	Reason string `json:"reason"`
}

// CreateSubscriptionOrderRequest represents the request to create a subscription order
type CreateSubscriptionOrderRequest struct {
	PlanCode     string `json:"plan_code" binding:"required"`
	BillingCycle string `json:"billing_cycle" binding:"required"` // monthly, yearly
	CouponCode   string `json:"coupon_code"`                      // Optional coupon code
}

// CreateSubscriptionAndPayRequest represents the request to create a subscription order and initiate payment
type CreateSubscriptionAndPayRequest struct {
	PlanCode              string  `json:"plan_code" binding:"required"`
	BillingCycle          string  `json:"billing_cycle" binding:"required"`  // monthly, yearly
	CouponCode            string  `json:"coupon_code"`                       // Optional coupon code
	PaymentMethod         string  `json:"payment_method" binding:"required"` // alipay, wechat, stripe
	PaymentSubMethod      string  `json:"payment_sub_method"`                // qrcode, native, wap, app
	UseWalletBalance      bool    `json:"use_wallet_balance"`
	WalletDeductionAmount float64 `json:"wallet_deduction_amount"`
	ReturnURL             string  `json:"return_url"` // For web payments
}

// InitiatePaymentRequest represents the request to initiate payment
type InitiatePaymentRequest struct {
	PaymentMethod    string `json:"payment_method" binding:"required"` // alipay, wechat, stripe
	PaymentSubMethod string `json:"payment_sub_method"`                // qrcode, native, wap, app
	ReturnURL        string `json:"return_url"`                        // For web payments
}

// CreateAICreditAndPayRequest represents the request to buy one AI credit package and initiate payment.
type CreateAICreditAndPayRequest struct {
	ProductID             string  `json:"product_id" binding:"required"`
	Quantity              int     `json:"quantity"`
	PaymentMethod         string  `json:"payment_method" binding:"required"`
	PaymentSubMethod      string  `json:"payment_sub_method"`
	UseWalletBalance      bool    `json:"use_wallet_balance"`
	WalletDeductionAmount float64 `json:"wallet_deduction_amount"`
	ReturnURL             string  `json:"return_url"`
}

// CreateWalletRechargeRequest represents the request to create a wallet recharge order
type CreateWalletRechargeRequest struct {
	Amount float64 `json:"amount" binding:"required,gte=1"` // Recharge amount (minimum 1)
}

// CreateWalletRechargeAndPayRequest represents the request to create a wallet recharge order and initiate payment
type CreateWalletRechargeAndPayRequest struct {
	Amount           float64 `json:"amount" binding:"required,gte=1"`   // Recharge amount (minimum 1)
	PaymentMethod    string  `json:"payment_method" binding:"required"` // alipay, wechat, stripe
	PaymentSubMethod string  `json:"payment_sub_method"`                // qrcode, native, wap, app
	ReturnURL        string  `json:"return_url"`                        // For web payments
}

// CreateOrder creates a new order
// @Summary Create order
// @Description Create a new order for subscription or credit purchase
// @Tags Orders
// @Accept json
// @Produce json
// @Param request body CreateOrderRequest true "Create order request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/orders [post]
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	errorResponse(c, http.StatusGone, "This endpoint is deprecated. Use /orders/recharge/pay.")
}

// GetOrder retrieves an order by ID
// @Summary Get order
// @Description Get order details
// @Tags Orders
// @Produce json
// @Param id path string true "Order ID"
// @Success 200 {object} platformconsole.PaymentOrderResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/orders/{id} [get]
func (h *OrderHandler) GetOrder(c *gin.Context) {
	id := c.Param("id")
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
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	order, err := h.consoleProvider.GetPaymentOrder(c.Request.Context(), id, groupID.String())
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusNotFound, "Order not found")
		return
	}
	if order.AccountID != accountID.String() {
		errorResponse(c, http.StatusForbidden, "Access denied")
		return
	}
	response.Success(c, order)
}

// GetOrderByOrderNo retrieves an order by order number
// @Summary Get order by order number
// @Description Get order details by order number
// @Tags Orders
// @Produce json
// @Param order_no path string true "Order Number"
// @Success 200 {object} platformconsole.PaymentOrderResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/orders/no/{order_no} [get]
func (h *OrderHandler) GetOrderByOrderNo(c *gin.Context) {
	errorResponse(c, http.StatusGone, "This endpoint is deprecated in API->console mode.")
}

// OrderListResponse represents paginated order list
type OrderListResponse struct {
	Data    []*platformconsole.PaymentOrderResponse `json:"data"`
	HasMore bool                                    `json:"has_more"`
	Limit   int                                     `json:"limit"`
	Total   int64                                   `json:"total"`
	Page    int                                     `json:"page"`
}

// ListOrders lists orders for current user
// @Summary List orders
// @Description Get all orders for current user
// @Tags Orders
// @Produce json
// @Param status query string false "Order status: pending, paid, completed, failed, canceled, refunded"
// @Param order_type query string false "Order type: wallet_recharge"
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} OrderListResponse
// @Router /api/v1/orders [get]
func (h *OrderHandler) ListOrders(c *gin.Context) {
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

	status := c.Query("status")
	orderType := c.Query("order_type")

	// Parse page and limit
	page := getIntQuery(c, "page", 1)
	limit := getIntQuery(c, "limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	listResp, err := h.consoleProvider.ListPaymentOrders(c.Request.Context(), &platformconsole.ListPaymentOrdersRequest{
		TenantID: groupID.String(),
		Status:   status,
		Page:     page,
		Size:     limit,
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to list orders")
		return
	}

	filtered := make([]*platformconsole.PaymentOrderResponse, 0, len(listResp.Items))
	for _, item := range listResp.Items {
		if item.AccountID != accountID.String() {
			continue
		}
		if orderType != "" && item.OrderType != orderType {
			continue
		}
		filtered = append(filtered, item)
	}

	resp := OrderListResponse{
		Data:    filtered,
		HasMore: false,
		Limit:   limit,
		Total:   int64(len(filtered)),
		Page:    page,
	}

	response.Success(c, resp)
}

// CreateSubscriptionOrder keeps the deprecated subscription order endpoint explicit.
// CreateSubscriptionOrder is kept only for backward-compatible handler shape.
func (h *OrderHandler) CreateSubscriptionOrder(c *gin.Context) {
	errorResponse(c, http.StatusGone, "Subscription orders have been removed.")
}

// CreateSubscriptionAndPay is kept only for backward-compatible handler shape.
func (h *OrderHandler) CreateSubscriptionAndPay(c *gin.Context) {
	errorResponse(c, http.StatusGone, "Subscription orders have been removed.")
}

// InitiatePayment initiates payment for an order
// @Summary Initiate payment
// @Description Create payment transaction and get payment parameters (QR code, redirect URL, etc.)
// @Tags Orders
// @Accept json
// @Produce json
// @Param id path string true "Order ID"
// @Param request body InitiatePaymentRequest true "Initiate payment request"
// @Success 200 {object} map[string]interface{} "Payment parameters (qrcode_url, payment_url, etc.)"
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/orders/{id}/payment [post]
func (h *OrderHandler) InitiatePayment(c *gin.Context) {
	errorResponse(c, http.StatusGone, "This endpoint is deprecated. Please use /orders/recharge/pay.")
}

// GetOrderPaymentStatus retrieves order payment status for polling
// @Summary Get order payment status
// @Description Get current payment status of an order (for frontend polling)
// @Tags Orders
// @Produce json
// @Param id path string true "Order ID"
// @Success 200 {object} map[string]interface{} "Payment status details"
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/orders/{id}/payment-status [get]
func (h *OrderHandler) GetOrderPaymentStatus(c *gin.Context) {
	orderID := c.Param("id")
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
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	order, err := h.consoleProvider.GetPaymentOrder(c.Request.Context(), orderID, groupID.String())
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusNotFound, "Order not found")
		return
	}
	if order.AccountID != accountID.String() {
		errorResponse(c, http.StatusForbidden, "Access denied")
		return
	}
	response.Success(c, map[string]interface{}{
		"order_id": order.ID,
		"order_no": order.OrderNo,
		"status":   order.Status,
	})
}

// CancelOrder cancels an order
// @Summary Cancel order
// @Description Cancel a pending order
// @Tags Orders
// @Accept json
// @Produce json
// @Param id path string true "Order ID"
// @Param request body CancelOrderRequest true "Cancel request"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/orders/{id}/cancel [post]
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	id := c.Param("id")
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
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	order, err := h.consoleProvider.GetPaymentOrder(c.Request.Context(), id, groupID.String())
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusNotFound, "Order not found")
		return
	}
	if order.AccountID != accountID.String() {
		errorResponse(c, http.StatusForbidden, "Access denied")
		return
	}

	if err := h.consoleProvider.CancelPaymentOrder(c.Request.Context(), &platformconsole.CancelPaymentOrderRequest{
		OrderID:  id,
		TenantID: groupID.String(),
	}); err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to cancel order")
		return
	}
	response.Success(c, gin.H{"message": "Order canceled successfully"})
}

// CreateWalletRechargeOrder creates a wallet recharge order
// @Summary Create wallet recharge order
// @Description Create a new order for wallet balance recharge with custom amount
// @Tags Orders
// @Accept json
// @Produce json
// @Param request body CreateWalletRechargeRequest true "Create wallet recharge order request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse "Too many pending orders"
// @Router /api/v1/orders/recharge [post]
func (h *OrderHandler) CreateWalletRechargeOrder(c *gin.Context) {
	errorResponse(c, http.StatusGone, "This endpoint is deprecated. Use /orders/recharge/pay.")
}

// CreateAICreditAndPay creates a credit package order and initiates payment.
func (h *OrderHandler) CreateAICreditAndPay(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Account ID not found")
		return
	}

	organizationID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	if !h.requireOrganizationAdmin(c, accountID.String(), organizationID.String(), "Only group admins can purchase AI credits") {
		return
	}

	var req CreateAICreditAndPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if req.Quantity == 0 {
		req.Quantity = 1
	}
	if req.Quantity != 1 {
		errorResponse(c, http.StatusBadRequest, "quantity must be 1")
		return
	}

	productID := strings.TrimSpace(req.ProductID)
	if productID == "" {
		errorResponse(c, http.StatusBadRequest, "product_id is required")
		return
	}
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	checkoutResp, err := h.consoleProvider.PaymentCheckout(c.Request.Context(), &platformconsole.PaymentCheckoutRequest{
		// TenantID is a historical field name in Console API; here it carries organization scope ID.
		TenantID:              organizationID.String(),
		AccountID:             accountID.String(),
		OrderType:             orderTypeCreditPurchase,
		ProductID:             productID,
		PaymentMethod:         req.PaymentMethod,
		PaymentSubMethod:      req.PaymentSubMethod,
		UseWalletBalance:      req.UseWalletBalance,
		WalletDeductionAmount: req.WalletDeductionAmount,
		ReturnURL:             req.ReturnURL,
		ClientIP:              c.ClientIP(),
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to create payment order")
		return
	}

	successWithStatus(c, http.StatusCreated, map[string]any{
		"order": map[string]any{
			"id":       checkoutResp.OrderID,
			"order_no": checkoutResp.OrderNo,
			"status":   checkoutResp.Status,
		},
		"payment": checkoutResp,
	})
}

// CreateWalletRechargeAndPay creates a wallet recharge order and initiates payment
// @Summary Create wallet recharge order and initiate payment
// @Description Create a new order for wallet balance recharge and get payment parameters immediately
// @Tags Orders
// @Accept json
// @Produce json
// @Param request body CreateWalletRechargeAndPayRequest true "Create wallet recharge and pay request"
// @Success 201 {object} map[string]interface{} "Order and payment parameters"
// @Failure 400 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Router /api/v1/orders/recharge/pay [post]
func (h *OrderHandler) CreateWalletRechargeAndPay(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Account ID not found")
		return
	}

	organizationID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	if !h.requireOrganizationAdmin(c, accountID.String(), organizationID.String(), "Only group admins can recharge wallet") {
		return
	}

	var req CreateWalletRechargeAndPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if strings.EqualFold(strings.TrimSpace(req.PaymentMethod), paymentMethodWallet) {
		errorResponse(c, http.StatusBadRequest, "wallet payment is not allowed for wallet recharge")
		return
	}
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	checkoutResp, err := h.consoleProvider.PaymentCheckout(c.Request.Context(), &platformconsole.PaymentCheckoutRequest{
		// TenantID is a historical field name in Console API; here it carries organization scope ID.
		TenantID:         organizationID.String(),
		AccountID:        accountID.String(),
		OrderType:        orderTypeWalletRecharge,
		ProductCode:      productCodeWalletRecharge,
		PaymentMethod:    req.PaymentMethod,
		PaymentSubMethod: req.PaymentSubMethod,
		ReturnURL:        req.ReturnURL,
		ClientIP:         c.ClientIP(),
		Amount:           req.Amount,
		Currency:         currencyCNY,
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to create payment order")
		return
	}

	successWithStatus(c, http.StatusCreated, map[string]any{
		"order": map[string]any{
			"id":       checkoutResp.OrderID,
			"order_no": checkoutResp.OrderNo,
			"status":   checkoutResp.Status,
		},
		"payment": checkoutResp,
	})
}

func (h *OrderHandler) requireOrganizationAdmin(c *gin.Context, accountID, organizationID, denyMessage string) bool {
	checker := h.organizationAdminChecker
	if checker == nil {
		if h.accountService == nil {
			errorResponse(c, http.StatusInternalServerError, "Failed to verify permission")
			return false
		}
		checker = func(ctx context.Context, organizationID, accountID string) (bool, error) {
			return h.accountService.IsOrganizationAdminOrOwner(ctx, organizationID, accountID)
		}
	}

	isAdmin, err := checker(c.Request.Context(), organizationID, accountID)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to verify permission")
		return false
	}
	if !isAdmin {
		errorResponse(c, http.StatusForbidden, denyMessage)
		return false
	}
	return true
}

// RegisterRoutes registers order-related routes
func (h *OrderHandler) RegisterRoutes(router *gin.RouterGroup) {
	// Orders (User - requires authentication with tenant)
	authWithTenant := router.Group("/orders", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		// General order operations
		authWithTenant.POST("", h.CreateOrder)
		authWithTenant.GET("", h.ListOrders)
		authWithTenant.GET("/:id", h.GetOrder)
		authWithTenant.GET("/no/:order_no", h.GetOrderByOrderNo)
		authWithTenant.POST("/:id/cancel", h.CancelOrder)

		// AI credit order creation and payment initiation.
		authWithTenant.POST("/ai-credits/pay", h.CreateAICreditAndPay)

		// Wallet recharge order creation (with deduplication)
		authWithTenant.POST("/recharge", h.CreateWalletRechargeOrder)
		// Wallet recharge order creation and payment initiation (Combined)
		authWithTenant.POST("/recharge/pay", h.CreateWalletRechargeAndPay)

		// Payment operations
		authWithTenant.POST("/:id/payment", h.InitiatePayment)
		authWithTenant.GET("/:id/payment-status", h.GetOrderPaymentStatus)
	}
}
