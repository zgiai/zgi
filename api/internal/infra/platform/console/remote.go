package console

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/observability"
)

const (
	apiPrefix = "/v1/internal"

	pathOrgRegister   = "/organizations/register"
	pathSignupGift    = "/credits/official-signup-grants"
	pathQuotaCheck    = "/quota/check"
	pathUsageReport   = "/usage/report"
	pathChannelModels = "/channels/models"

	pathCreditProducts  = "/credit-products"
	pathPaymentCheckout = "/payment/checkout"
	pathPurchaseRecords = "/payment/purchase-records"
)

// ConsoleAPIError represents a non-2xx HTTP result from console internal API.
type ConsoleAPIError struct {
	StatusCode int
	Message    string
}

func (e *ConsoleAPIError) Error() string {
	if e == nil {
		return "console api error"
	}
	if e.Message == "" {
		return fmt.Sprintf("console api returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("console api returned status %d: %s", e.StatusCode, e.Message)
}

// Remote implements ConsoleProvider for Cloud deployment mode.
// It communicates with Console-API via HTTP.
type Remote struct {
	baseURL        string
	internalAPIKey string
	httpClient     *http.Client
}

// NewRemote creates a new Cloud mode Console provider.
func NewRemote(baseURL string, internalAPIKey string) *Remote {
	return &Remote{
		baseURL:        baseURL,
		internalAPIKey: internalAPIKey,
		httpClient: observability.HTTPClient(&http.Client{
			Timeout: 30 * time.Second,
		}),
	}
}

// RegisterOrganization registers organization to Console-API (async).
func (r *Remote) RegisterOrganization(ctx context.Context, req *RegisterOrganizationRequest) error {
	go func() {
		syncCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		reqBody, err := json.Marshal(req)
		if err != nil {
			return
		}

		url := r.baseURL + apiPrefix + pathOrgRegister
		httpReq, err := http.NewRequestWithContext(syncCtx, "POST", url, bytes.NewReader(reqBody))
		if err != nil {
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		r.signRequest(httpReq)
		resp, err := r.httpClient.Do(httpReq)
		if err != nil {
			return
		}
		defer resp.Body.Close()
	}()

	return nil
}

// NotifyOfficialSignup tells Console-API a cloud signup completed.
func (r *Remote) NotifyOfficialSignup(ctx context.Context, req *NotifyOfficialSignupRequest) (*NotifyOfficialSignupResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var out NotifyOfficialSignupResponse
	if err := r.doJSON(ctx, http.MethodPost, pathSignupGift, nil, req, "application/json", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CheckQuota checks organization quota (sync with fallback).
func (r *Remote) CheckQuota(ctx context.Context, req *CheckQuotaRequest) (*CheckQuotaResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	reqBody, err := json.Marshal(req)
	if err != nil {
		return &CheckQuotaResponse{Allowed: true}, nil
	}

	url := r.baseURL + apiPrefix + pathQuotaCheck
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return &CheckQuotaResponse{Allowed: true}, nil
	}

	httpReq.Header.Set("Content-Type", "application/json")
	r.signRequest(httpReq)
	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return &CheckQuotaResponse{Allowed: true}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &CheckQuotaResponse{Allowed: true}, nil
	}

	var consoleResp struct {
		Code    int                `json:"code"`
		Message string             `json:"message"`
		Data    CheckQuotaResponse `json:"data"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &consoleResp); err != nil {
		return &CheckQuotaResponse{Allowed: true}, nil
	}

	return &consoleResp.Data, nil
}

// ReportUsage reports usage to Console-API (async).
func (r *Remote) ReportUsage(ctx context.Context, req *ReportUsageRequest) error {
	go func() {
		reportCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		reqBody, _ := json.Marshal(req)
		url := r.baseURL + apiPrefix + pathUsageReport

		httpReq, _ := http.NewRequestWithContext(reportCtx, "POST", url, bytes.NewReader(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		r.signRequest(httpReq)

		r.httpClient.Do(httpReq)
	}()

	return nil
}

// ListPlatformChannelModels fetches deduplicated models across all active platform channels.
func (r *Remote) ListPlatformChannelModels(ctx context.Context) (*PlatformChannelModelsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := r.baseURL + apiPrefix + pathChannelModels
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	r.signRequest(httpReq)

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call Console-API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Console-API returned status %d", resp.StatusCode)
	}

	var consoleResp struct {
		Code    int                           `json:"code"`
		Message string                        `json:"message"`
		Data    PlatformChannelModelsResponse `json:"data"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &consoleResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &consoleResp.Data, nil
}

// ListPlatformChannels fetches all active platform channels for management display.
func (r *Remote) ListPlatformChannels(ctx context.Context) (*PlatformChannelsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := r.baseURL + apiPrefix + "/channels"
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	r.signRequest(httpReq)

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call Console-API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Console-API returned status %d: %s", resp.StatusCode, string(body))
	}

	var consoleResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Channels []*PlatformChannelInfo `json:"channels"`
		} `json:"data"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &consoleResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &PlatformChannelsResponse{Channels: consoleResp.Data.Channels}, nil
}

// UpdatePlatformChannel updates routing-related fields of a platform channel via Console-API.
func (r *Remote) UpdatePlatformChannel(ctx context.Context, channelID string, req *UpdatePlatformChannelRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := r.baseURL + apiPrefix + "/channels/" + channelID
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	r.signRequest(httpReq)

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to call Console-API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Console-API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ListCreditProducts fetches active credit packages from Console-API.
func (r *Remote) ListCreditProducts(ctx context.Context) ([]*CreditProductInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var out []*CreditProductInfo
	if err := r.doJSON(ctx, http.MethodGet, pathCreditProducts, nil, nil, "", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PaymentCheckout calls console internal payment checkout API.
func (r *Remote) PaymentCheckout(ctx context.Context, req *PaymentCheckoutRequest) (*PaymentCheckoutResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var out PaymentCheckoutResponse
	if err := r.doJSON(
		ctx,
		http.MethodPost,
		pathPaymentCheckout,
		nil,
		req,
		"application/json",
		&out,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPaymentWallet fetches wallet by tenant ID.
func (r *Remote) GetPaymentWallet(ctx context.Context, tenantID string, currency string) (*PaymentWalletResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := url.Values{}
	if strings.TrimSpace(currency) != "" {
		query.Set("currency", currency)
	}

	var out PaymentWalletResponse
	path := "/payment/wallets/" + tenantID
	if err := r.doJSON(ctx, http.MethodGet, path, query, nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPaymentOrder fetches payment order detail.
func (r *Remote) GetPaymentOrder(ctx context.Context, orderID string, tenantID string) (*PaymentOrderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := url.Values{}
	if strings.TrimSpace(tenantID) != "" {
		query.Set("tenant_id", tenantID)
	}

	var out PaymentOrderResponse
	path := "/payment/orders/" + orderID
	if err := r.doJSON(ctx, http.MethodGet, path, query, nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPaymentOrders lists payment orders by tenant.
func (r *Remote) ListPaymentOrders(ctx context.Context, req *ListPaymentOrdersRequest) (*ListPaymentOrdersResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := url.Values{}
	query.Set("tenant_id", req.TenantID)
	if strings.TrimSpace(req.Status) != "" {
		query.Set("status", req.Status)
	}
	if req.Page > 0 {
		query.Set("page", strconv.Itoa(req.Page))
	}
	if req.Size > 0 {
		query.Set("size", strconv.Itoa(req.Size))
	}

	var out ListPaymentOrdersResponse
	if err := r.doJSON(ctx, http.MethodGet, "/payment/orders", query, nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPaymentPurchaseRecords lists purchase records for an organization.
func (r *Remote) ListPaymentPurchaseRecords(ctx context.Context, req *ListPaymentPurchaseRecordsRequest) (*ListPaymentPurchaseRecordsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := url.Values{}
	query.Set("organization_id", req.OrganizationID)
	if strings.TrimSpace(req.PurchaseKind) != "" {
		query.Set("purchase_kind", req.PurchaseKind)
	}
	if strings.TrimSpace(req.EventKind) != "" {
		query.Set("event_kind", req.EventKind)
	}
	if strings.TrimSpace(req.PaymentMethod) != "" {
		query.Set("payment_method", req.PaymentMethod)
	}
	if strings.TrimSpace(req.Keyword) != "" {
		query.Set("keyword", req.Keyword)
	}
	if req.StartTime != nil && !req.StartTime.IsZero() {
		query.Set("start_time", req.StartTime.Format(time.RFC3339))
	}
	if req.EndTime != nil && !req.EndTime.IsZero() {
		query.Set("end_time", req.EndTime.Format(time.RFC3339))
	}
	if req.Page > 0 {
		query.Set("page", strconv.Itoa(req.Page))
	}
	if req.Size > 0 {
		query.Set("size", strconv.Itoa(req.Size))
	}

	var out ListPaymentPurchaseRecordsResponse
	if err := r.doJSON(ctx, http.MethodGet, pathPurchaseRecords, query, nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExportPaymentPurchaseRecords downloads purchase records as an Excel file.
func (r *Remote) ExportPaymentPurchaseRecords(ctx context.Context, req *ListPaymentPurchaseRecordsRequest) (*PaymentPurchaseRecordsExportFile, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := url.Values{}
	query.Set("organization_id", req.OrganizationID)
	if strings.TrimSpace(req.PurchaseKind) != "" {
		query.Set("purchase_kind", req.PurchaseKind)
	}
	if strings.TrimSpace(req.EventKind) != "" {
		query.Set("event_kind", req.EventKind)
	}
	if strings.TrimSpace(req.PaymentMethod) != "" {
		query.Set("payment_method", req.PaymentMethod)
	}
	if strings.TrimSpace(req.Keyword) != "" {
		query.Set("keyword", req.Keyword)
	}
	if req.StartTime != nil && !req.StartTime.IsZero() {
		query.Set("start_time", req.StartTime.Format(time.RFC3339))
	}
	if req.EndTime != nil && !req.EndTime.IsZero() {
		query.Set("end_time", req.EndTime.Format(time.RFC3339))
	}

	file, err := r.doBinaryWithQuery(ctx, http.MethodGet, pathPurchaseRecords+"/export", query)
	if err != nil {
		return nil, err
	}
	return &PaymentPurchaseRecordsExportFile{
		ContentType: file.ContentType,
		Data:        file.Data,
	}, nil
}

// CancelPaymentOrder cancels pending order.
func (r *Remote) CancelPaymentOrder(ctx context.Context, req *CancelPaymentOrderRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := url.Values{}
	query.Set("tenant_id", req.TenantID)
	path := "/payment/orders/" + req.OrderID + "/cancel"
	return r.doJSON(ctx, http.MethodPost, path, query, nil, "", nil)
}

// SubmitBankTransfer submits bank transfer request.
func (r *Remote) SubmitBankTransfer(ctx context.Context, req *SubmitBankTransferRequest) (*BankTransferRequestResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var out BankTransferRequestResponse
	if err := r.doJSON(ctx, http.MethodPost, "/payment/bank-transfer/requests", nil, req, "application/json", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListBankTransfers lists bank transfer requests by tenant.
func (r *Remote) ListBankTransfers(ctx context.Context, req *ListBankTransfersRequest) (*ListBankTransfersResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := url.Values{}
	query.Set("tenant_id", req.TenantID)
	if strings.TrimSpace(req.Status) != "" {
		query.Set("status", req.Status)
	}
	if req.Page > 0 {
		query.Set("page", strconv.Itoa(req.Page))
	}
	if req.Size > 0 {
		query.Set("size", strconv.Itoa(req.Size))
	}

	var out ListBankTransfersResponse
	if err := r.doJSON(ctx, http.MethodGet, "/payment/bank-transfer/requests", query, nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetBankTransfer fetches bank transfer request detail.
func (r *Remote) GetBankTransfer(ctx context.Context, req *GetBankTransferRequest) (*BankTransferRequestResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := url.Values{}
	if strings.TrimSpace(req.TenantID) != "" {
		query.Set("tenant_id", req.TenantID)
	}
	path := "/payment/bank-transfer/requests/" + req.ID
	var out BankTransferRequestResponse
	if err := r.doJSON(ctx, http.MethodGet, path, query, nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelBankTransfer cancels pending bank transfer request.
func (r *Remote) CancelBankTransfer(ctx context.Context, req *CancelBankTransferRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	payload := map[string]interface{}{
		"tenant_id":  req.TenantID,
		"account_id": req.AccountID,
	}
	if strings.TrimSpace(req.Reason) != "" {
		payload["reason"] = req.Reason
	}
	path := "/payment/bank-transfer/requests/" + req.ID + "/cancel"
	return r.doJSON(ctx, http.MethodPost, path, nil, payload, "application/json", nil)
}

// ReviewBankTransfer reviews bank transfer request.
func (r *Remote) ReviewBankTransfer(ctx context.Context, req *ReviewBankTransferRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	payload := map[string]interface{}{
		"reviewer_id": req.ReviewerID,
		"action":      req.Action,
	}
	if strings.TrimSpace(req.Reason) != "" {
		payload["reason"] = req.Reason
	}
	path := "/payment/bank-transfer/requests/" + req.ID + "/review"
	return r.doJSON(ctx, http.MethodPost, path, nil, payload, "application/json", nil)
}

// UploadBankTransferVoucher uploads voucher content to console.
func (r *Remote) UploadBankTransferVoucher(ctx context.Context, req *UploadBankTransferVoucherRequest) (*UploadBankTransferVoucherResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", req.FileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := fileWriter.Write(req.Data); err != nil {
		return nil, fmt.Errorf("failed to write file payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close form writer: %w", err)
	}

	fullURL := r.baseURL + apiPrefix + "/payment/bank-transfer/upload-voucher"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	r.signRequest(httpReq)

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call Console-API: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &ConsoleAPIError{
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(string(rawBody)),
		}
	}

	var envelope struct {
		Code    interface{}                       `json:"code"`
		Message string                            `json:"message"`
		Data    UploadBankTransferVoucherResponse `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &envelope.Data, nil
}

// GetBankTransferVoucher downloads voucher binary from console.
func (r *Remote) GetBankTransferVoucher(ctx context.Context, key string) (*BankTransferVoucherFile, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	path := "/payment/bank-transfer/vouchers/" + strings.TrimPrefix(key, "/")
	file, err := r.doBinaryWithQuery(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return &BankTransferVoucherFile{
		ContentType: file.ContentType,
		Data:        file.Data,
	}, nil
}

// signRequest adds HMAC-SHA256 authentication headers to an outgoing request.
// It sets X-Internal-Timestamp and X-Internal-Signature headers.
func (r *Remote) signRequest(req *http.Request) {
	if r.internalAPIKey == "" {
		return
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	message := ts + "|" + req.URL.Path
	mac := hmac.New(sha256.New, []byte(r.internalAPIKey))
	mac.Write([]byte(message))
	sig := hex.EncodeToString(mac.Sum(nil))
	req.Header.Set("X-Internal-Timestamp", ts)
	req.Header.Set("X-Internal-Signature", sig)
}

func (r *Remote) doJSON(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	payload interface{},
	contentType string,
	out interface{},
) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	fullURL := r.baseURL + apiPrefix + path
	if query != nil && len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	r.signRequest(req)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call Console-API: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg := strings.TrimSpace(string(rawBody))
		var apiErr struct {
			Code    interface{} `json:"code"`
			Message string      `json:"message"`
		}
		if json.Unmarshal(rawBody, &apiErr) == nil && apiErr.Message != "" {
			msg = apiErr.Message
		}
		return &ConsoleAPIError{
			StatusCode: resp.StatusCode,
			Message:    msg,
		}
	}

	var envelope struct {
		Code    interface{}     `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return fmt.Errorf("failed to decode response envelope: %w", err)
	}

	if out == nil {
		return nil
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("failed to decode response data: %w", err)
	}
	return nil
}

type downloadedBinary struct {
	ContentType string
	Data        []byte
}

func (r *Remote) doBinaryWithQuery(ctx context.Context, method string, path string, query url.Values) (*downloadedBinary, error) {
	fullURL := r.baseURL + apiPrefix + path
	if query != nil && len(query) > 0 {
		fullURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	r.signRequest(req)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Console-API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &ConsoleAPIError{
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(string(body)),
		}
	}

	return &downloadedBinary{
		ContentType: resp.Header.Get("Content-Type"),
		Data:        body,
	}, nil
}

// GetBaseURL returns the Console-API base URL.
func (r *Remote) GetBaseURL() string {
	return r.baseURL
}

// IsAvailable returns true for Cloud mode.
func (r *Remote) IsAvailable() bool {
	return true
}

// GetMode returns "CLOUD".
func (r *Remote) GetMode() string {
	return "CLOUD"
}
