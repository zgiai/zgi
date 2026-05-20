package console

import (
	"context"
	"time"
)

// ConsoleProvider defines the interface for Console-API integration.
// LLM proxy methods (ChatCompletions, Embeddings) have been removed —
// official channels now go through the standard adapter with APIBaseURL pointing to console-api.
type ConsoleProvider interface {
	// IsAvailable returns true if Console-API integration is available
	IsAvailable() bool

	// GetMode returns the deployment mode ("CLOUD" or "SELF_HOSTED")
	GetMode() string

	// GetBaseURL returns the Console-API base URL (e.g. "http://localhost:2625")
	GetBaseURL() string

	// RegisterOrganization registers a new organization to Console-API (async)
	RegisterOrganization(ctx context.Context, req *RegisterOrganizationRequest) error

	// NotifyOfficialSignup tells Console-API that a cloud signup completed.
	NotifyOfficialSignup(ctx context.Context, req *NotifyOfficialSignupRequest) (*NotifyOfficialSignupResponse, error)

	// CheckQuota checks if organization has sufficient quota (sync)
	CheckQuota(ctx context.Context, req *CheckQuotaRequest) (*CheckQuotaResponse, error)

	// ReportUsage reports LLM usage to Console-API (async)
	ReportUsage(ctx context.Context, req *ReportUsageRequest) error

	// ListPlatformChannelModels returns deduplicated models across all active platform channels
	ListPlatformChannelModels(ctx context.Context) (*PlatformChannelModelsResponse, error)

	// ListPlatformChannels returns all active platform channels (full info for management display)
	ListPlatformChannels(ctx context.Context) (*PlatformChannelsResponse, error)

	// UpdatePlatformChannel updates routing-related fields of a platform channel
	UpdatePlatformChannel(ctx context.Context, channelID string, req *UpdatePlatformChannelRequest) error

	// ListCreditProducts returns active credit packages owned by Console-API.
	ListCreditProducts(ctx context.Context) ([]*CreditProductInfo, error)

	// PaymentCheckout creates/initiates payment order on console payment core.
	PaymentCheckout(ctx context.Context, req *PaymentCheckoutRequest) (*PaymentCheckoutResponse, error)

	// GetPaymentWallet returns tenant wallet information from console payment core.
	GetPaymentWallet(ctx context.Context, tenantID string, currency string) (*PaymentWalletResponse, error)

	// GetPaymentOrder returns payment order detail from console payment core.
	GetPaymentOrder(ctx context.Context, orderID string, tenantID string) (*PaymentOrderResponse, error)

	// ListPaymentOrders returns paginated payment orders.
	ListPaymentOrders(ctx context.Context, req *ListPaymentOrdersRequest) (*ListPaymentOrdersResponse, error)

	// ListPaymentPurchaseRecords returns paginated purchase records for the current organization.
	ListPaymentPurchaseRecords(ctx context.Context, req *ListPaymentPurchaseRecordsRequest) (*ListPaymentPurchaseRecordsResponse, error)

	// ExportPaymentPurchaseRecords exports purchase records as an Excel file.
	ExportPaymentPurchaseRecords(ctx context.Context, req *ListPaymentPurchaseRecordsRequest) (*PaymentPurchaseRecordsExportFile, error)

	// CancelPaymentOrder cancels a pending order in console payment core.
	CancelPaymentOrder(ctx context.Context, req *CancelPaymentOrderRequest) error

	// SubmitBankTransfer creates bank transfer request in console payment core.
	SubmitBankTransfer(ctx context.Context, req *SubmitBankTransferRequest) (*BankTransferRequestResponse, error)

	// ListBankTransfers returns paginated bank transfer requests by tenant.
	ListBankTransfers(ctx context.Context, req *ListBankTransfersRequest) (*ListBankTransfersResponse, error)

	// GetBankTransfer returns a bank transfer request by ID.
	GetBankTransfer(ctx context.Context, req *GetBankTransferRequest) (*BankTransferRequestResponse, error)

	// CancelBankTransfer cancels a pending bank transfer request.
	CancelBankTransfer(ctx context.Context, req *CancelBankTransferRequest) error

	// ReviewBankTransfer reviews a bank transfer request (approve/reject).
	ReviewBankTransfer(ctx context.Context, req *ReviewBankTransferRequest) error

	// UploadBankTransferVoucher uploads voucher file to console and returns metadata.
	UploadBankTransferVoucher(ctx context.Context, req *UploadBankTransferVoucherRequest) (*UploadBankTransferVoucherResponse, error)

	// GetBankTransferVoucher loads voucher binary from console.
	GetBankTransferVoucher(ctx context.Context, key string) (*BankTransferVoucherFile, error)
}

// RegisterOrganizationRequest represents organization registration request.
type RegisterOrganizationRequest struct {
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	OwnerEmail     string    `json:"owner_email"`
	CreatedAt      time.Time `json:"created_at"`
}

type NotifyOfficialSignupRequest struct {
	OrganizationID string `json:"organization_id"`
	AccountID      string `json:"account_id"`
}

type NotifyOfficialSignupResponse struct {
	OrganizationID string `json:"organization_id"`
	AccountID      string `json:"account_id"`
	Amount         int64  `json:"amount"`
	NewBalance     int64  `json:"new_balance"`
	AlreadyGranted bool   `json:"already_granted"`
}

// CheckQuotaRequest represents quota check request.
type CheckQuotaRequest struct {
	OrganizationID  string `json:"organization_id"`
	EstimatedTokens int64  `json:"estimated_tokens"`
}

// CheckQuotaResponse represents quota check response.
type CheckQuotaResponse struct {
	Allowed        bool   `json:"allowed"`
	QuotaRemaining int64  `json:"quota_remaining"`
	Reason         string `json:"reason,omitempty"`
}

// ReportUsageRequest represents usage reporting request.
type ReportUsageRequest struct {
	OrganizationID string  `json:"organization_id"`
	RequestID      string  `json:"request_id"`
	Model          string  `json:"model"`
	TotalTokens    int64   `json:"total_tokens"`
	Cost           float64 `json:"cost"`
}

// PlatformChannelModelsResponse represents the response for listing platform channel models.
type PlatformChannelModelsResponse struct {
	Models []string `json:"models"`
}

// PlatformChannelInfo represents a platform channel for management display.
type PlatformChannelInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	Models      []string `json:"models"`
	Priority    int      `json:"priority"`
	Weight      int      `json:"weight"`
	IsActive    bool     `json:"is_active"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description,omitempty"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// UpdatePlatformChannelRequest represents the request to update a platform channel.
type UpdatePlatformChannelRequest struct {
	Priority *int  `json:"priority"`
	Weight   *int  `json:"weight"`
	IsActive *bool `json:"is_active"`
}

// PlatformChannelsResponse represents the response for listing platform channels.
type PlatformChannelsResponse struct {
	Channels []*PlatformChannelInfo `json:"channels"`
}

// CreditProductInfo represents an AI credit package returned by Console-API.
type CreditProductInfo struct {
	ID           string   `json:"id"`
	ProductCode  string   `json:"product_code"`
	ProductName  string   `json:"product_name"`
	CreditAmount int64    `json:"credit_amount"`
	Price        float64  `json:"price"`
	Currency     string   `json:"currency"`
	ValidityDays *int     `json:"validity_days,omitempty"`
	Description  *string  `json:"description,omitempty"`
	DisplayOrder int      `json:"display_order"`
	Tags         []string `json:"tags,omitempty"`
	IsActive     bool     `json:"is_active"`
}

// PaymentCheckoutRequest represents POST /v1/internal/payment/checkout payload.
type PaymentCheckoutRequest struct {
	TenantID              string  `json:"tenant_id"`
	AccountID             string  `json:"account_id"`
	OrderType             string  `json:"order_type"`
	ProductID             string  `json:"product_id,omitempty"`
	ProductCode           string  `json:"product_code,omitempty"`
	ProductName           string  `json:"product_name,omitempty"`
	BillingCycle          string  `json:"billing_cycle,omitempty"`
	PaymentMethod         string  `json:"payment_method"`
	PaymentSubMethod      string  `json:"payment_sub_method,omitempty"`
	UseWalletBalance      bool    `json:"use_wallet_balance,omitempty"`
	WalletDeductionAmount float64 `json:"wallet_deduction_amount,omitempty"`
	ReturnURL             string  `json:"return_url,omitempty"`
	ClientIP              string  `json:"client_ip,omitempty"`
	Amount                float64 `json:"amount,omitempty"`
	CreditAmount          int64   `json:"credit_amount,omitempty"`
	Currency              string  `json:"currency,omitempty"`
}

// PaymentCheckoutResponse represents payment checkout response from console.
type PaymentCheckoutResponse struct {
	OrderID               string            `json:"order_id"`
	OrderNo               string            `json:"order_no"`
	TransactionNo         string            `json:"transaction_no,omitempty"`
	OrderAmount           string            `json:"order_amount,omitempty"`
	WalletDeductedAmount  string            `json:"wallet_deducted_amount,omitempty"`
	ExternalPayableAmount string            `json:"external_payable_amount,omitempty"`
	PaymentURL            string            `json:"payment_url,omitempty"`
	QRCodeContent         string            `json:"qr_code_content,omitempty"`
	PaymentForm           string            `json:"payment_form,omitempty"`
	AppPayParams          map[string]string `json:"app_pay_params,omitempty"`
	Status                string            `json:"status"`
}

// PaymentWalletResponse represents wallet data from console.
type PaymentWalletResponse struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenant_id"`
	Currency       string `json:"currency"`
	Balance        string `json:"balance"`
	FrozenBalance  string `json:"frozen_balance"`
	TotalRecharged string `json:"total_recharged"`
	TotalConsumed  string `json:"total_consumed"`
}

// PaymentOrderResponse represents payment order from console.
type PaymentOrderResponse struct {
	ID              string                 `json:"id"`
	OrderNo         string                 `json:"order_no"`
	TenantID        string                 `json:"tenant_id"`
	AccountID       string                 `json:"account_id"`
	OrderType       string                 `json:"order_type"`
	ProductCode     string                 `json:"product_code"`
	ProductType     string                 `json:"product_type"`
	ProductSnapshot map[string]interface{} `json:"product_snapshot,omitempty"`
	OriginalAmount  string                 `json:"original_amount"`
	DiscountAmount  string                 `json:"discount_amount"`
	FinalAmount     string                 `json:"final_amount"`
	Currency        string                 `json:"currency"`
	Status          string                 `json:"status"`
	PaymentMethod   *string                `json:"payment_method,omitempty"`
	PaidAt          *string                `json:"paid_at,omitempty"`
	CompletedAt     *string                `json:"completed_at,omitempty"`
	CreatedAt       string                 `json:"created_at"`
}

// ListPaymentOrdersRequest represents list filter.
type ListPaymentOrdersRequest struct {
	TenantID string
	Status   string
	Page     int
	Size     int
}

// ListPaymentOrdersResponse represents paginated payment orders.
type ListPaymentOrdersResponse struct {
	Items []*PaymentOrderResponse `json:"items"`
	Total int64                   `json:"total"`
	Page  int                     `json:"page"`
	Size  int                     `json:"size"`
}

// ListPaymentPurchaseRecordsRequest represents a purchase record list filter.
type ListPaymentPurchaseRecordsRequest struct {
	OrganizationID string     `json:"organization_id"`
	PurchaseKind   string     `json:"purchase_kind,omitempty"`
	EventKind      string     `json:"event_kind,omitempty"`
	PaymentMethod  string     `json:"payment_method,omitempty"`
	Keyword        string     `json:"keyword,omitempty"`
	StartTime      *time.Time `json:"start_time,omitempty"`
	EndTime        *time.Time `json:"end_time,omitempty"`
	Page           int        `json:"page,omitempty"`
	Size           int        `json:"size,omitempty"`
}

// PaymentPurchaseRecord represents a purchase/refund billing view row.
type PaymentPurchaseRecord struct {
	ID                 string    `json:"id"`
	BatchID            string    `json:"batch_id"`
	TransactionType    string    `json:"transaction_type"`
	DetailText         string    `json:"detail_text"`
	RechargeAmount     float64   `json:"recharge_amount"`
	WalletChangeAmount float64   `json:"wallet_change_amount"`
	BalanceAfter       float64   `json:"balance_after"`
	CreatedAt          time.Time `json:"created_at"`
}

// ListPaymentPurchaseRecordsResponse represents paginated purchase records.
type ListPaymentPurchaseRecordsResponse struct {
	Items []*PaymentPurchaseRecord `json:"items"`
	Total int64                    `json:"total"`
	Page  int                      `json:"page"`
	Size  int                      `json:"size"`
}

// PaymentPurchaseRecordsExportFile represents an exported purchase record file.
type PaymentPurchaseRecordsExportFile struct {
	ContentType string
	Data        []byte
}

// CancelPaymentOrderRequest represents order cancel request.
type CancelPaymentOrderRequest struct {
	OrderID  string `json:"order_id"`
	TenantID string `json:"tenant_id"`
}

// SubmitBankTransferRequest represents submit request for console internal API.
type SubmitBankTransferRequest struct {
	TenantID   string  `json:"tenant_id"`
	AccountID  string  `json:"account_id"`
	Amount     float64 `json:"amount"`
	VoucherKey string  `json:"voucher_key"`
	Remark     string  `json:"remark,omitempty"`
	ClientIP   string  `json:"client_ip,omitempty"`
}

// BankTransferRequestResponse represents bank transfer request payload from console.
type BankTransferRequestResponse struct {
	ID           string  `json:"id"`
	RequestNo    string  `json:"request_no"`
	TenantID     string  `json:"tenant_id"`
	AccountID    string  `json:"account_id"`
	Amount       string  `json:"amount"`
	Currency     string  `json:"currency"`
	VoucherKey   string  `json:"voucher_key"`
	Remark       *string `json:"remark,omitempty"`
	Status       string  `json:"status"`
	CancelReason *string `json:"cancel_reason,omitempty"`
	RejectReason *string `json:"reject_reason,omitempty"`
	ReviewedBy   *string `json:"reviewed_by,omitempty"`
	ReviewedAt   *string `json:"reviewed_at,omitempty"`
	CompletedAt  *string `json:"completed_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

// ListBankTransfersRequest represents list filter for bank transfer requests.
type ListBankTransfersRequest struct {
	TenantID string
	Status   string
	Page     int
	Size     int
}

// ListBankTransfersResponse represents paginated bank transfer requests.
type ListBankTransfersResponse struct {
	Items []*BankTransferRequestResponse `json:"items"`
	Total int64                          `json:"total"`
	Page  int                            `json:"page"`
	Size  int                            `json:"size"`
}

// GetBankTransferRequest represents get detail request.
type GetBankTransferRequest struct {
	ID       string
	TenantID string
}

// CancelBankTransferRequest represents cancel request.
type CancelBankTransferRequest struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	AccountID string `json:"account_id"`
	Reason    string `json:"reason,omitempty"`
}

// ReviewBankTransferRequest represents admin review request.
type ReviewBankTransferRequest struct {
	ID         string `json:"id"`
	ReviewerID string `json:"reviewer_id"`
	Action     string `json:"action"`
	Reason     string `json:"reason,omitempty"`
}

// UploadBankTransferVoucherRequest represents voucher upload payload.
type UploadBankTransferVoucherRequest struct {
	FileName    string
	ContentType string
	Data        []byte
}

// UploadBankTransferVoucherResponse represents upload response.
type UploadBankTransferVoucherResponse struct {
	VoucherKey string `json:"voucher_key"`
	VoucherURL string `json:"voucher_url"`
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	MimeType   string `json:"mime_type"`
}

// BankTransferVoucherFile represents downloaded voucher content.
type BankTransferVoucherFile struct {
	ContentType string
	Data        []byte
}
