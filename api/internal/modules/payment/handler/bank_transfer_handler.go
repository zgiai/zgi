package handler

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	platformconsole "github.com/zgiai/ginext/internal/infra/platform/console"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/middleware"
)

// BankTransferHandler handles bank transfer related HTTP requests
type BankTransferHandler struct {
	accountService  interfaces.AccountService
	consoleProvider platformconsole.ConsoleProvider
}

// NewBankTransferHandler creates a new bank transfer handler
func NewBankTransferHandler(
	accountService interfaces.AccountService,
	consoleProvider platformconsole.ConsoleProvider,
) *BankTransferHandler {
	return &BankTransferHandler{
		accountService:  accountService,
		consoleProvider: consoleProvider,
	}
}

// SubmitBankTransferRequest represents the request to submit a bank transfer
type SubmitBankTransferRequest struct {
	Amount     float64 `json:"amount" binding:"required,gt=0"` // Transfer amount (CNY)
	VoucherKey string  `json:"voucher_key" binding:"required"` // Voucher key (obtained after upload)
	Remark     string  `json:"remark"`                         // Remark (company name + contact person + phone number)
}

// CancelBankTransferRequest represents the request to cancel a bank transfer
type CancelBankTransferRequest struct {
	Reason string `json:"reason"`
}

// ReviewBankTransferRequest represents the request to review a bank transfer
type ReviewBankTransferRequest struct {
	Action string `json:"action" binding:"required,oneof=approve reject"` // approve or reject
	Reason string `json:"reason"`                                         // Required when rejecting
}

// BankTransferResponse represents a bank transfer in API response
type BankTransferResponse struct {
	ID           string  `json:"id"`
	RequestNo    string  `json:"request_no"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	VoucherKey   string  `json:"voucher_key"`
	Remark       *string `json:"remark,omitempty"`
	Status       string  `json:"status"`
	RejectReason *string `json:"reject_reason,omitempty"`
	CreatedAt    string  `json:"created_at"`
	ReviewedAt   *string `json:"reviewed_at,omitempty"`
	CompletedAt  *string `json:"completed_at,omitempty"`
}

func toBankTransferResponseFromConsole(req *platformconsole.BankTransferRequestResponse) *BankTransferResponse {
	if req == nil {
		return nil
	}
	amount, _ := strconv.ParseFloat(req.Amount, 64)
	resp := &BankTransferResponse{
		ID:           req.ID,
		RequestNo:    req.RequestNo,
		Amount:       amount,
		Currency:     req.Currency,
		VoucherKey:   req.VoucherKey,
		Remark:       req.Remark,
		Status:       req.Status,
		RejectReason: req.RejectReason,
		CreatedAt:    req.CreatedAt,
		ReviewedAt:   req.ReviewedAt,
		CompletedAt:  req.CompletedAt,
	}
	return resp
}

// Submit creates a new bank transfer request
// @Summary Submit bank transfer request
// @Description Submit a new bank transfer recharge request
// @Tags BankTransfer
// @Accept json
// @Produce json
// @Param request body SubmitBankTransferRequest true "Submit request"
// @Success 201 {object} BankTransferResponse
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/bank-transfer/requests [post]
func (h *BankTransferHandler) Submit(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	groupID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	var req SubmitBankTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	item, err := h.consoleProvider.SubmitBankTransfer(c.Request.Context(), &platformconsole.SubmitBankTransferRequest{
		TenantID:   groupID.String(),
		AccountID:  accountID.String(),
		Amount:     req.Amount,
		VoucherKey: req.VoucherKey,
		Remark:     req.Remark,
		ClientIP:   c.ClientIP(),
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to submit bank transfer request")
		return
	}

	successWithStatus(c, http.StatusCreated, toBankTransferResponseFromConsole(item))
}

// GetByID gets a bank transfer request by ID
// @Summary Get bank transfer request
// @Description Get a bank transfer request by ID
// @Tags BankTransfer
// @Produce json
// @Param id path string true "Request ID"
// @Success 200 {object} BankTransferResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/bank-transfer/requests/{id} [get]
func (h *BankTransferHandler) GetByID(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	groupID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "Request ID is required")
		return
	}
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	item, err := h.consoleProvider.GetBankTransfer(c.Request.Context(), &platformconsole.GetBankTransferRequest{
		ID:       id,
		TenantID: groupID.String(),
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to get bank transfer request")
		return
	}
	if item.AccountID != accountID.String() {
		errorResponse(c, http.StatusNotFound, "Request not found")
		return
	}
	successWithStatus(c, http.StatusOK, toBankTransferResponseFromConsole(item))
}

// List lists bank transfer requests
// @Summary List bank transfer requests
// @Description List bank transfer requests for current user
// @Tags BankTransfer
// @Produce json
// @Param status query string false "Filter by status"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Page size" default(20)
// @Success 200 {object} response.Response
// @Router /api/v1/bank-transfer/requests [get]
func (h *BankTransferHandler) List(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	groupID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	status := c.Query("status")
	page := getIntQuery(c, "page", 1)
	limit := getIntQuery(c, "limit", 20)
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	listResp, err := h.consoleProvider.ListBankTransfers(c.Request.Context(), &platformconsole.ListBankTransfersRequest{
		TenantID: groupID.String(),
		Status:   status,
		Page:     page,
		Size:     limit,
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to list bank transfer requests")
		return
	}
	items := make([]*BankTransferResponse, 0, len(listResp.Items))
	for _, item := range listResp.Items {
		if item.AccountID != accountID.String() {
			continue
		}
		items = append(items, toBankTransferResponseFromConsole(item))
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    "0",
		"message": "success",
		"data": gin.H{
			"items": items,
			"total": len(items),
			"page":  page,
			"limit": limit,
		},
	})
}

// Cancel cancels a bank transfer request
// @Summary Cancel bank transfer request
// @Description Cancel a pending bank transfer request
// @Tags BankTransfer
// @Accept json
// @Produce json
// @Param id path string true "Request ID"
// @Param request body CancelBankTransferRequest true "Cancel request"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/bank-transfer/requests/{id}/cancel [post]
func (h *BankTransferHandler) Cancel(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	groupID, err := getGroupID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Group ID not found")
		return
	}

	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "Request ID is required")
		return
	}

	var req CancelBankTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
		req.Reason = ""
	}
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	if err := h.consoleProvider.CancelBankTransfer(c.Request.Context(), &platformconsole.CancelBankTransferRequest{
		ID:        id,
		TenantID:  groupID.String(),
		AccountID: accountID.String(),
		Reason:    req.Reason,
	}); err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to cancel bank transfer request")
		return
	}

	successWithStatus(c, http.StatusOK, gin.H{"message": "Canceled successfully"})
}

// UploadVoucherResponse represents the response of voucher upload
type UploadVoucherResponse struct {
	VoucherKey string `json:"voucher_key"`
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	MimeType   string `json:"mime_type"`
}

// UploadVoucher uploads a bank transfer voucher
// @Summary Upload bank transfer voucher
// @Description Upload a voucher image for bank transfer request
// @Tags BankTransfer
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Voucher image (JPG/PNG, max 5MB)"
// @Success 200 {object} UploadVoucherResponse
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/bank-transfer/upload-voucher [post]
func (h *BankTransferHandler) UploadVoucher(c *gin.Context) {
	// Get file from form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Please select a file to upload")
		return
	}
	defer file.Close()
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Failed to read upload file")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	result, err := h.consoleProvider.UploadBankTransferVoucher(c.Request.Context(), &platformconsole.UploadBankTransferVoucherRequest{
		FileName:    header.Filename,
		ContentType: contentType,
		Data:        data,
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Upload failed")
		return
	}

	resp := &UploadVoucherResponse{
		VoucherKey: result.VoucherKey,
		FileName:   result.FileName,
		FileSize:   result.FileSize,
		MimeType:   result.MimeType,
	}
	successWithStatus(c, http.StatusOK, resp)
}

// PreviewVoucher returns the voucher image for preview
// @Summary Preview voucher
// @Description Get voucher image by key for preview
// @Tags BankTransfer
// @Produce image/jpeg,image/png
// @Param key path string true "Voucher key"
// @Success 200 {file} binary
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/bank-transfer/vouchers/{key} [get]
func (h *BankTransferHandler) PreviewVoucher(c *gin.Context) {
	key := c.Param("key")
	// Remove leading slash from wildcard param
	if len(key) > 0 && key[0] == '/' {
		key = key[1:]
	}
	if key == "" {
		errorResponse(c, http.StatusBadRequest, "Voucher key is required")
		return
	}
	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	file, err := h.consoleProvider.GetBankTransferVoucher(c.Request.Context(), key)
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusNotFound, "Voucher not found")
		return
	}
	contentType := file.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Data(http.StatusOK, contentType, file.Data)
}

// RegisterRoutes registers bank transfer routes
func (h *BankTransferHandler) RegisterRoutes(router *gin.RouterGroup) {
	// User routes (requires authentication with tenant)
	userRoutes := router.Group("/bank-transfer", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		userRoutes.POST("/upload-voucher", h.UploadVoucher)
		userRoutes.GET("/vouchers/*key", h.PreviewVoucher)
		userRoutes.POST("/requests", h.Submit)
		userRoutes.GET("/requests", h.List)
		userRoutes.GET("/requests/:id", h.GetByID)
		userRoutes.POST("/requests/:id/cancel", h.Cancel)
	}
}
