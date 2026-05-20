package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	platformconsole "github.com/zgiai/ginext/internal/infra/platform/console"
	"github.com/zgiai/ginext/internal/modules/payment/model"
	"github.com/zgiai/ginext/internal/modules/payment/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/middleware"
	"github.com/zgiai/ginext/pkg/response"
)

// TransactionHandler handles transaction-related HTTP requests (unified billing records)
type TransactionHandler struct {
	transactionService *service.TransactionService
	accountService     interfaces.AccountService
	consoleProvider    platformconsole.ConsoleProvider
}

// NewTransactionHandler creates a new transaction handler
func NewTransactionHandler(
	transactionService *service.TransactionService,
	accountService interfaces.AccountService,
	consoleProvider platformconsole.ConsoleProvider,
) *TransactionHandler {
	return &TransactionHandler{
		transactionService: transactionService,
		accountService:     accountService,
		consoleProvider:    consoleProvider,
	}
}

type purchaseRecordsQuery struct {
	Page            int    `form:"page"`
	Limit           int    `form:"limit"`
	TransactionType string `form:"transaction_type"`
	Keyword         string `form:"keyword"`
	StartTime       string `form:"start_time"`
	EndTime         string `form:"end_time"`
}

const (
	transactionsExportContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	transactionsExportSheetName   = "Transactions"
	transactionTypeHeaderLabel    = "\u4ea4\u6613\u7c7b\u578b"
	detailHeaderLabel             = "\u8be6\u60c5"
	descriptionHeaderLabel        = "\u63cf\u8ff0"
	rechargeExportLabel           = "\u5145\u503c"
	rechargePurchaseDisplayLabel  = "\u8d2d\u4e70\u7c7b"
)

func (q purchaseRecordsQuery) normalizedPage() int {
	if q.Page < 1 {
		return 1
	}
	return q.Page
}

func (q purchaseRecordsQuery) normalizedLimit() int {
	if q.Limit < 1 {
		return 20
	}
	if q.Limit > 100 {
		return 100
	}
	return q.Limit
}

func parseRFC3339QueryTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func mapTransactionTypeToPurchaseEvent(transactionType string) (string, bool) {
	switch strings.TrimSpace(transactionType) {
	case "":
		return "", true
	case string(model.TransactionTypeRechargePurchase):
		return "purchase", true
	case string(model.TransactionTypeOther):
		return "refund", true
	default:
		return "", false
	}
}

func mapPurchaseRecordToTransactionResponse(record *platformconsole.PaymentPurchaseRecord) *BillingTransactionResponse {
	if record == nil {
		return nil
	}
	return &BillingTransactionResponse{
		ID:                 record.ID,
		BatchID:            record.BatchID,
		TransactionType:    record.TransactionType,
		DetailText:         record.DetailText,
		RechargeAmount:     record.RechargeAmount,
		WalletChangeAmount: record.WalletChangeAmount,
		BalanceAfter:       record.BalanceAfter,
		CreatedAt:          record.CreatedAt,
	}
}

func buildEmptyTransactionsExportWorkbook() ([]byte, error) {
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", transactionsExportSheetName); err != nil {
		return nil, err
	}

	headers := []string{
		"\u4ea4\u6613ID",
		"\u65f6\u95f4",
		transactionTypeHeaderLabel,
		detailHeaderLabel,
		"\u5145\u503c\u91d1\u989d",
		"\u94b1\u5305\u53d8\u52a8",
		"\u8d26\u6237\u4f59\u989d",
	}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(transactionsExportSheetName, cell, header); err != nil {
			return nil, err
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func normalizeTransactionsExportWorkbook(data []byte) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return data, nil
	}
	sheetName := sheets[0]

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return data, nil
	}

	transactionTypeColumn := findTransactionsExportColumn(rows[0], transactionTypeHeaderLabel)
	if transactionTypeColumn == 0 {
		return data, nil
	}

	if err := normalizeTransactionsExportHeaders(f, sheetName, rows[0]); err != nil {
		return nil, err
	}

	for rowIndex := 2; rowIndex <= len(rows); rowIndex++ {
		cell, err := excelize.CoordinatesToCellName(transactionTypeColumn, rowIndex)
		if err != nil {
			return nil, err
		}
		value, err := f.GetCellValue(sheetName, cell)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(value) == rechargeExportLabel {
			if err := f.SetCellValue(sheetName, cell, rechargePurchaseDisplayLabel); err != nil {
				return nil, err
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func normalizeTransactionsExportHeaders(f *excelize.File, sheetName string, headers []string) error {
	for i, header := range headers {
		if strings.TrimSpace(header) != descriptionHeaderLabel {
			continue
		}
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheetName, cell, detailHeaderLabel); err != nil {
			return err
		}
	}
	return nil
}

func findTransactionsExportColumn(headers []string, target string) int {
	for i, header := range headers {
		if strings.TrimSpace(header) == target {
			return i + 1
		}
	}
	return 0
}

// BillingTransactionResponse represents the billing list response payload.
type BillingTransactionResponse struct {
	ID                 string    `json:"id"`
	BatchID            string    `json:"batch_id"`
	TransactionType    string    `json:"transaction_type"`
	DetailText         string    `json:"detail_text"`
	RechargeAmount     float64   `json:"recharge_amount"`
	WalletChangeAmount float64   `json:"wallet_change_amount"`
	BalanceAfter       float64   `json:"balance_after"`
	CreatedAt          time.Time `json:"created_at"`
}

// TransactionListResponse represents paginated transaction list
type TransactionListResponse struct {
	Data    []BillingTransactionResponse `json:"data"`
	HasMore bool                         `json:"has_more"`
	Limit   int                          `json:"limit"`
	Total   int64                        `json:"total"`
	Page    int                          `json:"page"`
}

// ListTransactions lists purchase and refund records for the current organization.
// @Summary List transactions
// @Description List purchase and refund records for the current user's organization
// @Tags Transactions
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Param transaction_type query string false "Transaction type: recharge_purchase, other"
// @Param keyword query string false "Search by transaction ID or description"
// @Param start_time query string false "Start time (RFC3339 format, e.g. 2024-01-01T00:00:00+08:00)"
// @Param end_time query string false "End time (RFC3339 format, e.g. 2024-01-31T23:59:59+08:00)"
// @Success 200 {object} TransactionListResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/transactions [get]
func (h *TransactionHandler) ListTransactions(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Account ID not found")
		return
	}

	groupID, err := h.getGroupID(c, accountID)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	var req purchaseRecordsQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	page := req.normalizedPage()
	limit := req.normalizedLimit()

	startTime, err := parseRFC3339QueryTime(req.StartTime)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid start_time")
		return
	}
	endTime, err := parseRFC3339QueryTime(req.EndTime)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid end_time")
		return
	}

	eventKind, ok := mapTransactionTypeToPurchaseEvent(req.TransactionType)
	if !ok {
		response.Success(c, TransactionListResponse{
			Data:    []BillingTransactionResponse{},
			HasMore: false,
			Limit:   limit,
			Total:   0,
			Page:    page,
		})
		return
	}

	resp, err := h.consoleProvider.ListPaymentPurchaseRecords(c.Request.Context(), &platformconsole.ListPaymentPurchaseRecordsRequest{
		OrganizationID: groupID.String(),
		EventKind:      eventKind,
		Keyword:        strings.TrimSpace(req.Keyword),
		StartTime:      startTime,
		EndTime:        endTime,
		Page:           page,
		Size:           limit,
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to list transactions")
		return
	}

	data := make([]BillingTransactionResponse, 0, len(resp.Items))
	for _, record := range resp.Items {
		if mapped := mapPurchaseRecordToTransactionResponse(record); mapped != nil {
			data = append(data, *mapped)
		}
	}

	total := resp.Total
	hasMore := int64((page-1)*limit+len(data)) < total
	response.Success(c, TransactionListResponse{
		Data:    data,
		HasMore: hasMore,
		Limit:   limit,
		Total:   total,
		Page:    page,
	})
}

// getGroupID gets the group ID for the current user
func (h *TransactionHandler) getGroupID(c *gin.Context, accountID uuid.UUID) (uuid.UUID, error) {
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

	return uuid.Parse(tenantID)
}

// ExportTransactions exports purchase and refund records to Excel.
// @Summary Export transactions to Excel
// @Description Export purchase and refund records to Excel file
// @Tags Transactions
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security BearerAuth
// @Param transaction_type query string false "Transaction type: recharge_purchase, other"
// @Param keyword query string false "Search by transaction ID or description"
// @Param start_time query string false "Start time (RFC3339 format, e.g. 2024-01-01T00:00:00+08:00)"
// @Param end_time query string false "End time (RFC3339 format, e.g. 2024-01-31T23:59:59+08:00)"
// @Success 200 {file} file "Excel file"
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/transactions/export [get]
func (h *TransactionHandler) ExportTransactions(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Account ID not found")
		return
	}

	groupID, err := h.getGroupID(c, accountID)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	var req purchaseRecordsQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	startTime, err := parseRFC3339QueryTime(req.StartTime)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid start_time")
		return
	}
	endTime, err := parseRFC3339QueryTime(req.EndTime)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid end_time")
		return
	}

	eventKind, ok := mapTransactionTypeToPurchaseEvent(req.TransactionType)
	if !ok {
		data, buildErr := buildEmptyTransactionsExportWorkbook()
		if buildErr != nil {
			errorResponse(c, http.StatusInternalServerError, "Failed to build empty export file: "+buildErr.Error())
			return
		}
		c.Header("Content-Type", transactionsExportContentType)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=transactions_%s.xlsx", time.Now().Format("20060102_150405")))
		c.Data(http.StatusOK, transactionsExportContentType, data)
		return
	}

	if !requireConsolePaymentProxy(c, h.consoleProvider) {
		return
	}

	resp, err := h.consoleProvider.ExportPaymentPurchaseRecords(c.Request.Context(), &platformconsole.ListPaymentPurchaseRecordsRequest{
		OrganizationID: groupID.String(),
		EventKind:      eventKind,
		Keyword:        strings.TrimSpace(req.Keyword),
		StartTime:      startTime,
		EndTime:        endTime,
	})
	if err != nil {
		writeConsoleProxyError(c, err, http.StatusBadGateway, "Failed to export transactions")
		return
	}

	contentType := resp.ContentType
	if strings.TrimSpace(contentType) == "" {
		contentType = transactionsExportContentType
	}
	data, err := normalizeTransactionsExportWorkbook(resp.Data)
	if err != nil {
		errorResponse(c, http.StatusBadGateway, "Failed to normalize exported transactions: "+err.Error())
		return
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=transactions_%s.xlsx", time.Now().Format("20060102_150405")))
	c.Data(http.StatusOK, contentType, data)
}

// MonthlyConsumptionStatsResponse represents the response for monthly consumption statistics
type MonthlyConsumptionStatsResponse struct {
	Period struct {
		Year  int    `json:"year"`
		Month int    `json:"month"`
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"period"`
	Cash struct {
		TotalConsumed float64 `json:"total_consumed"`
		Currency      string  `json:"currency"`
	} `json:"cash"`
	Credits struct {
		SubscriptionCreditsConsumed int64 `json:"subscription_credits_consumed"`
		PurchasedCreditsConsumed    int64 `json:"purchased_credits_consumed"`
		TotalCreditsConsumed        int64 `json:"total_credits_consumed"`
	} `json:"credits"`
}

// GetMonthlyConsumptionStats returns monthly consumption statistics for the current user's group
// @Summary Get monthly consumption statistics
// @Description Get monthly consumption statistics including cash spent and credits consumed
// @Tags Transactions
// @Produce json
// @Security BearerAuth
// @Param year query int false "Year (default: current year)"
// @Param month query int false "Month (1-12, default: current month)"
// @Success 200 {object} MonthlyConsumptionStatsResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/transactions/monthly-stats [get]
func (h *TransactionHandler) GetMonthlyConsumptionStats(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "Account ID not found")
		return
	}

	// Get user's group ID
	groupID, err := h.getGroupID(c, accountID)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Parse query parameters
	var year, month int
	if yearStr := c.Query("year"); yearStr != "" {
		fmt.Sscanf(yearStr, "%d", &year)
	}
	if monthStr := c.Query("month"); monthStr != "" {
		fmt.Sscanf(monthStr, "%d", &month)
		if month < 1 || month > 12 {
			errorResponse(c, http.StatusBadRequest, "Invalid month, must be 1-12")
			return
		}
	}

	// Get monthly consumption stats
	stats, err := h.transactionService.GetMonthlyConsumptionStats(c.Request.Context(), groupID, year, month)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, fmt.Sprintf("Failed to get monthly consumption stats: %v", err))
		return
	}

	// Build response
	resp := MonthlyConsumptionStatsResponse{}
	resp.Period.Year = stats.Period.Year
	resp.Period.Month = stats.Period.Month
	resp.Period.Start = stats.Period.Start.Format(time.RFC3339)
	resp.Period.End = stats.Period.End.Format(time.RFC3339)
	resp.Cash.TotalConsumed = stats.Cash.TotalConsumed
	resp.Cash.Currency = stats.Cash.Currency
	resp.Credits.SubscriptionCreditsConsumed = stats.Credits.SubscriptionCreditsConsumed
	resp.Credits.PurchasedCreditsConsumed = stats.Credits.PurchasedCreditsConsumed
	resp.Credits.TotalCreditsConsumed = stats.Credits.TotalCreditsConsumed

	response.Success(c, resp)
}

// RegisterRoutes registers transaction-related routes
func (h *TransactionHandler) RegisterRoutes(router *gin.RouterGroup) {
	// Transaction routes - /transactions for unified billing records
	transactions := router.Group("/transactions", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		transactions.GET("", h.ListTransactions)
		transactions.GET("/monthly-stats", h.GetMonthlyConsumptionStats) // Monthly consumption statistics
		transactions.GET("/export", h.ExportTransactions)
	}
}
