package console

import (
	"context"
	"fmt"
)

var errPaymentProxyUnavailable = fmt.Errorf("payment proxy is not available in self-hosted mode")

// Standalone implements ConsoleProvider for Self-Hosted deployment mode.
// It returns unavailable errors without external dependencies.
type Standalone struct{}

// NewStandalone creates a new Self-Hosted mode Console provider.
func NewStandalone() *Standalone {
	return &Standalone{}
}

// RegisterOrganization does nothing in Self-Hosted mode.
func (s *Standalone) RegisterOrganization(ctx context.Context, req *RegisterOrganizationRequest) error {
	return nil
}

// NotifyOfficialSignup does nothing in Self-Hosted mode.
func (s *Standalone) NotifyOfficialSignup(ctx context.Context, req *NotifyOfficialSignupRequest) (*NotifyOfficialSignupResponse, error) {
	return &NotifyOfficialSignupResponse{}, nil
}

// CheckQuota always allows requests in Self-Hosted mode.
func (s *Standalone) CheckQuota(ctx context.Context, req *CheckQuotaRequest) (*CheckQuotaResponse, error) {
	return &CheckQuotaResponse{
		Allowed:        true,
		QuotaRemaining: 999999999,
	}, nil
}

// ReportUsage does nothing in Self-Hosted mode.
func (s *Standalone) ReportUsage(ctx context.Context, req *ReportUsageRequest) error {
	return nil
}

// ListPlatformChannelModels returns empty in Self-Hosted mode.
func (s *Standalone) ListPlatformChannelModels(ctx context.Context) (*PlatformChannelModelsResponse, error) {
	return &PlatformChannelModelsResponse{}, nil
}

// ListPlatformChannels returns empty in Self-Hosted mode.
func (s *Standalone) ListPlatformChannels(ctx context.Context) (*PlatformChannelsResponse, error) {
	return &PlatformChannelsResponse{Channels: []*PlatformChannelInfo{}}, nil
}

// UpdatePlatformChannel is a no-op in Self-Hosted mode.
func (s *Standalone) UpdatePlatformChannel(ctx context.Context, channelID string, req *UpdatePlatformChannelRequest) error {
	return fmt.Errorf("platform channel updates are not available in self-hosted mode")
}

// ListCreditProducts is unavailable in self-hosted mode.
func (s *Standalone) ListCreditProducts(ctx context.Context) ([]*CreditProductInfo, error) {
	return nil, errPaymentProxyUnavailable
}

// PaymentCheckout is unavailable in self-hosted mode.
func (s *Standalone) PaymentCheckout(ctx context.Context, req *PaymentCheckoutRequest) (*PaymentCheckoutResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// GetPaymentWallet is unavailable in self-hosted mode.
func (s *Standalone) GetPaymentWallet(ctx context.Context, tenantID string, currency string) (*PaymentWalletResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// GetPaymentOrder is unavailable in self-hosted mode.
func (s *Standalone) GetPaymentOrder(ctx context.Context, orderID string, tenantID string) (*PaymentOrderResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// ListPaymentOrders is unavailable in self-hosted mode.
func (s *Standalone) ListPaymentOrders(ctx context.Context, req *ListPaymentOrdersRequest) (*ListPaymentOrdersResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// ListPaymentPurchaseRecords is unavailable in self-hosted mode.
func (s *Standalone) ListPaymentPurchaseRecords(ctx context.Context, req *ListPaymentPurchaseRecordsRequest) (*ListPaymentPurchaseRecordsResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// ExportPaymentPurchaseRecords is unavailable in self-hosted mode.
func (s *Standalone) ExportPaymentPurchaseRecords(ctx context.Context, req *ListPaymentPurchaseRecordsRequest) (*PaymentPurchaseRecordsExportFile, error) {
	return nil, errPaymentProxyUnavailable
}

// CancelPaymentOrder is unavailable in self-hosted mode.
func (s *Standalone) CancelPaymentOrder(ctx context.Context, req *CancelPaymentOrderRequest) error {
	return errPaymentProxyUnavailable
}

// SubmitBankTransfer is unavailable in self-hosted mode.
func (s *Standalone) SubmitBankTransfer(ctx context.Context, req *SubmitBankTransferRequest) (*BankTransferRequestResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// ListBankTransfers is unavailable in self-hosted mode.
func (s *Standalone) ListBankTransfers(ctx context.Context, req *ListBankTransfersRequest) (*ListBankTransfersResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// GetBankTransfer is unavailable in self-hosted mode.
func (s *Standalone) GetBankTransfer(ctx context.Context, req *GetBankTransferRequest) (*BankTransferRequestResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// CancelBankTransfer is unavailable in self-hosted mode.
func (s *Standalone) CancelBankTransfer(ctx context.Context, req *CancelBankTransferRequest) error {
	return errPaymentProxyUnavailable
}

// ReviewBankTransfer is unavailable in self-hosted mode.
func (s *Standalone) ReviewBankTransfer(ctx context.Context, req *ReviewBankTransferRequest) error {
	return errPaymentProxyUnavailable
}

// UploadBankTransferVoucher is unavailable in self-hosted mode.
func (s *Standalone) UploadBankTransferVoucher(ctx context.Context, req *UploadBankTransferVoucherRequest) (*UploadBankTransferVoucherResponse, error) {
	return nil, errPaymentProxyUnavailable
}

// GetBankTransferVoucher is unavailable in self-hosted mode.
func (s *Standalone) GetBankTransferVoucher(ctx context.Context, key string) (*BankTransferVoucherFile, error) {
	return nil, errPaymentProxyUnavailable
}

// GetBaseURL returns empty string for Self-Hosted mode.
func (s *Standalone) GetBaseURL() string {
	return ""
}

// IsAvailable returns false for Self-Hosted mode.
func (s *Standalone) IsAvailable() bool {
	return false
}

// GetMode returns "SELF_HOSTED".
func (s *Standalone) GetMode() string {
	return "SELF_HOSTED"
}
