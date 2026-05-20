package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeQuotaClient struct {
	preDeductResp *PreDeductQuotaResponse
	preDeductErr  error
	settleResp    *SettleQuotaResponse
	settleErr     error

	preDeductRequests []*PreDeductQuotaRequest
	settleRequests    []*SettleQuotaRequest

	onPreDeduct func(req *PreDeductQuotaRequest) error
	onSettle    func(req *SettleQuotaRequest) error
}

func (f *fakeQuotaClient) PreDeductQuota(ctx context.Context, req *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	f.preDeductRequests = append(f.preDeductRequests, req)
	if f.onPreDeduct != nil {
		if err := f.onPreDeduct(req); err != nil {
			return nil, err
		}
	}
	if f.preDeductErr != nil {
		return nil, f.preDeductErr
	}
	if f.preDeductResp != nil {
		return f.preDeductResp, nil
	}
	return &PreDeductQuotaResponse{Success: true, DeductionID: "deduction-default"}, nil
}

func (f *fakeQuotaClient) SettleQuota(ctx context.Context, req *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	f.settleRequests = append(f.settleRequests, req)
	if f.onSettle != nil {
		if err := f.onSettle(req); err != nil {
			return nil, err
		}
	}
	if f.settleErr != nil {
		return nil, f.settleErr
	}
	if f.settleResp != nil {
		resp := *f.settleResp
		return &resp, nil
	}
	return &SettleQuotaResponse{Success: true, SettledCredits: req.ActualCredits}, nil
}

func (f *fakeQuotaClient) CheckCreditBalance(ctx context.Context, organizationID string, estimatedCredits int64) (bool, int64, error) {
	return true, 0, nil
}

func (f *fakeQuotaClient) Close() error { return nil }

func TestApplyRemoteSettlementResult_AllowsZeroSettledCreditsForZeroCreditSuccess(t *testing.T) {
	bc := &BillingContext{
		RequestID:     "req-zero-credit",
		AttemptID:     "req-zero-credit-a1",
		Status:        "success",
		ActualCredits: 0,
	}
	resp := &SettleQuotaResponse{
		Success:        true,
		UsedQuota:      0,
		SettledCredits: 0,
	}

	if err := applyRemoteSettlementResult(bc, resp); err != nil {
		t.Fatalf("applyRemoteSettlementResult returned error: %v", err)
	}
	if bc.ActualCredits != 0 {
		t.Fatalf("ActualCredits = %d, want 0", bc.ActualCredits)
	}
	if bc.BillingLane != UsageBillingLanePlatform {
		t.Fatalf("BillingLane = %q, want %q", bc.BillingLane, UsageBillingLanePlatform)
	}
}

func TestApplyRemoteSettlementResult_RejectsMissingSettledCreditsForPositiveCharge(t *testing.T) {
	bc := &BillingContext{
		RequestID:     "req-positive-credit",
		AttemptID:     "req-positive-credit-a1",
		Status:        "success",
		ActualCredits: 7,
	}
	resp := &SettleQuotaResponse{
		Success:        true,
		UsedQuota:      0,
		SettledCredits: 0,
	}

	err := applyRemoteSettlementResult(bc, resp)
	if err == nil {
		t.Fatalf("expected missing settled_credits error")
	}
	if !strings.Contains(err.Error(), "without settled_credits") {
		t.Fatalf("error = %v, want missing settled_credits", err)
	}
	if bc.ActualCredits != 7 {
		t.Fatalf("ActualCredits = %d, want original 7 after rejected response", bc.ActualCredits)
	}
}

func openRemoteBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:remote_billing_test_" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("sqlite driver requires cgo in this environment")
		}
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	return db
}

func migrateRemoteBillingTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&BillingAttempt{}, &BillingAttemptEntry{}, &apikeymodel.TenantAPIKey{}, &WorkspaceQuota{}, &UsageBill{}); err != nil {
		t.Fatalf("failed to migrate billing tables: %v", err)
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_attempt_entry
		ON billing_attempt_entries(attempt_id, entry_type, ledger_type)
	`).Error; err != nil {
		t.Fatalf("failed to create billing_attempt_entries unique index: %v", err)
	}
}

func seedBillingAPIKey(t *testing.T, db *gorm.DB, organizationID, apiKeyID string, remainQuota int64, quotaLimit *int64) {
	t.Helper()
	key := &apikeymodel.TenantAPIKey{
		ID:             apiKeyID,
		OrganizationID: organizationID,
		Key:            "encrypted",
		KeyHash:        "hash-" + apiKeyID,
		Name:           "test-key",
		Status:         "active",
		AllowIPs:       "",
		RemainQuota:    remainQuota,
		QuotaLimit:     quotaLimit,
	}
	if err := db.Create(key).Error; err != nil {
		t.Fatalf("failed to seed api key: %v", err)
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}

func TestRemoteBilling_PreDeductViaGRPC_BindsDeductionID(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	migrateRemoteBillingTables(t, db)
	orgID := uuid.NewString()
	seedBillingAPIKey(t, db, orgID, "key-1", 100, ptrInt64(100))

	client := &fakeQuotaClient{
		preDeductResp: &PreDeductQuotaResponse{
			Success:     true,
			DeductionID: "deduction-1",
		},
	}
	rb := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   client,
	}

	bc := &BillingContext{
		APIKeyID:          "key-1",
		OrganizationID:    orgID,
		AttemptID:         "req-1-a1",
		RequestID:         "req-1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    "key-1",
		EstimatedCredits:  12,
		UseSystemProvider: true,
	}

	if err := rb.preDeductViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("preDeductViaGRPC returned error: %v", err)
	}
	if bc.DeductionID != "deduction-1" {
		t.Fatalf("billingContext deduction_id = %q, want %q", bc.DeductionID, "deduction-1")
	}
	if len(client.preDeductRequests) != 1 {
		t.Fatalf("preDeduct requests = %d, want 1", len(client.preDeductRequests))
	}

	var fundEntry BillingAttemptEntry
	if err := db.Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", bc.AttemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
		First(&fundEntry).Error; err != nil {
		t.Fatalf("failed to load fund entry: %v", err)
	}
	if fundEntry.IdempotencyKey == nil || *fundEntry.IdempotencyKey != "deduction-1" {
		t.Fatalf("fund entry idempotency_key = %v, want deduction-1", fundEntry.IdempotencyKey)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusPre {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusPre)
	}
}

func TestRemoteBilling_SettleViaGRPC_ResponseFail_MarksPartialWithSuccessInvocation(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	migrateRemoteBillingTables(t, db)
	orgID := uuid.NewString()
	seedBillingAPIKey(t, db, orgID, "key-2", 100, ptrInt64(100))

	client := &fakeQuotaClient{
		preDeductResp: &PreDeductQuotaResponse{
			Success:     true,
			DeductionID: "deduction-2",
		},
		settleResp: &SettleQuotaResponse{
			Success:      false,
			ErrorMessage: "remote settle failed",
		},
	}
	rb := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   client,
	}

	bc := &BillingContext{
		APIKeyID:          "key-2",
		OrganizationID:    orgID,
		AttemptID:         "req-2-a1",
		RequestID:         "req-2",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    "key-2",
		EstimatedCredits:  10,
		ActualCredits:     8,
		UseSystemProvider: true,
		Status:            "success",
	}

	if err := rb.preDeductViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("preDeductViaGRPC returned error: %v", err)
	}

	err := rb.settleViaGRPC(context.Background(), bc)
	if err == nil {
		t.Fatalf("expected settleViaGRPC to fail when remote returns success=false")
	}
	if !strings.Contains(err.Error(), "remote settle failed") {
		t.Fatalf("unexpected settle error: %v", err)
	}

	var attempt BillingAttempt
	if loadErr := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; loadErr != nil {
		t.Fatalf("failed to load attempt: %v", loadErr)
	}

	if attempt.Status != billingAttemptStatusPartial {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusPartial)
	}
	if attempt.InvocationResult == nil || *attempt.InvocationResult != "success" {
		t.Fatalf("invocation_result = %v, want success", attempt.InvocationResult)
	}
}

func TestRemoteBilling_SettleViaGRPC_ClientError_MarksPartialWithErrorInvocation(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	migrateRemoteBillingTables(t, db)
	orgID := uuid.NewString()
	seedBillingAPIKey(t, db, orgID, "key-3", 100, ptrInt64(100))

	client := &fakeQuotaClient{
		preDeductResp: &PreDeductQuotaResponse{
			Success:     true,
			DeductionID: "deduction-3",
		},
		settleErr: errors.New("grpc unavailable"),
	}
	rb := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   client,
	}

	bc := &BillingContext{
		APIKeyID:          "key-3",
		OrganizationID:    orgID,
		AttemptID:         "req-3-a1",
		RequestID:         "req-3",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    "key-3",
		EstimatedCredits:  9,
		ActualCredits:     0,
		UseSystemProvider: true,
		Status:            "error",
	}

	if err := rb.preDeductViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("preDeductViaGRPC returned error: %v", err)
	}
	if err := rb.settleViaGRPC(context.Background(), bc); err == nil {
		t.Fatalf("expected settleViaGRPC to fail when grpc client returns error")
	}

	var attempt BillingAttempt
	if loadErr := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; loadErr != nil {
		t.Fatalf("failed to load attempt: %v", loadErr)
	}
	if attempt.Status != billingAttemptStatusPartial {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusPartial)
	}
	if attempt.InvocationResult == nil || *attempt.InvocationResult != "error" {
		t.Fatalf("invocation_result = %v, want error", attempt.InvocationResult)
	}
}

func TestRemoteBilling_PreDeductViaGRPC_RemoteFailure_RollsBackAPIKeyQuota(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	migrateRemoteBillingTables(t, db)

	orgID := uuid.NewString()
	apiKeyID := "key-rollback-1"
	seedBillingAPIKey(t, db, orgID, apiKeyID, 100, ptrInt64(100))

	client := &fakeQuotaClient{
		preDeductResp: &PreDeductQuotaResponse{
			Success:      false,
			ErrorMessage: "remote pre-deduct failed",
		},
	}
	rb := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   client,
	}

	bc := &BillingContext{
		APIKeyID:          apiKeyID,
		OrganizationID:    orgID,
		AttemptID:         "req-rollback-a1",
		RequestID:         "req-rollback",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    apiKeyID,
		EstimatedCredits:  10,
		UseSystemProvider: true,
	}

	if err := rb.preDeductViaGRPC(context.Background(), bc); err == nil {
		t.Fatalf("expected preDeductViaGRPC to fail when remote pre-deduct fails")
	}

	var key apikeymodel.TenantAPIKey
	if err := db.Where("id = ?", apiKeyID).First(&key).Error; err != nil {
		t.Fatalf("failed to load api key: %v", err)
	}
	if key.RemainQuota != 100 {
		t.Fatalf("remain_quota = %d, want 100 after rollback", key.RemainQuota)
	}
}

func TestRemoteBilling_PreDeductViaGRPC_WorkspaceSubject_DeductsWorkspaceQuota(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	migrateRemoteBillingTables(t, db)

	orgID := uuid.New()

	if err := db.Create(&WorkspaceQuota{
		WorkspaceID:    "ws-1",
		OrganizationID: orgID,
		UsedQuota:      0,
		RemainQuota:    100,
		QuotaLimit:     ptrInt64(100),
	}).Error; err != nil {
		t.Fatalf("failed to seed workspace quota: %v", err)
	}

	client := &fakeQuotaClient{
		preDeductResp: &PreDeductQuotaResponse{
			Success:     true,
			DeductionID: "deduction-ws-1",
		},
	}
	rb := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   client,
	}

	bc := &BillingContext{
		OrganizationID:    orgID.String(),
		AttemptID:         "req-ws-a1",
		RequestID:         "req-ws",
		QuotaSubjectType:  quotaSubjectTypeWorkspace,
		QuotaSubjectID:    "ws-1",
		EstimatedCredits:  10,
		ActualCredits:     6,
		UseSystemProvider: true,
		Status:            "success",
	}

	if err := rb.preDeductViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("preDeductViaGRPC returned error: %v", err)
	}

	var quota WorkspaceQuota
	if err := db.Where("workspace_id = ?", "ws-1").First(&quota).Error; err != nil {
		t.Fatalf("failed to load workspace quota after prededuct: %v", err)
	}
	if quota.RemainQuota != 90 {
		t.Fatalf("workspace remain_quota = %d, want 90 after prededuct", quota.RemainQuota)
	}

	if err := rb.settleViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("settleViaGRPC returned error: %v", err)
	}

	if err := db.Where("workspace_id = ?", "ws-1").First(&quota).Error; err != nil {
		t.Fatalf("failed to load workspace quota after settle: %v", err)
	}
	if quota.RemainQuota != 94 {
		t.Fatalf("workspace remain_quota = %d, want 94 after settle", quota.RemainQuota)
	}
	if quota.UsedQuota != 6 {
		t.Fatalf("workspace used_quota = %d, want 6", quota.UsedQuota)
	}
}

func TestRemoteBilling_PreDeductViaGRPC_BindFailure_CompensatesRemoteAndLocal(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	migrateRemoteBillingTables(t, db)

	orgID := uuid.NewString()
	apiKeyID := "key-bind-fail-1"
	seedBillingAPIKey(t, db, orgID, apiKeyID, 100, ptrInt64(100))

	client := &fakeQuotaClient{
		preDeductResp: &PreDeductQuotaResponse{
			Success:     true,
			DeductionID: "deduction-bind-fail-1",
		},
		settleResp: &SettleQuotaResponse{Success: true},
		onPreDeduct: func(req *PreDeductQuotaRequest) error {
			return db.Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", req.AttemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
				Delete(&BillingAttemptEntry{}).Error
		},
	}
	rb := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   client,
	}

	bc := &BillingContext{
		APIKeyID:          apiKeyID,
		OrganizationID:    orgID,
		AttemptID:         "req-bind-fail-a1",
		RequestID:         "req-bind-fail",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    apiKeyID,
		EstimatedCredits:  10,
		UseSystemProvider: true,
	}

	err := rb.preDeductViaGRPC(context.Background(), bc)
	if err == nil {
		t.Fatalf("expected preDeductViaGRPC to fail when deduction bind fails")
	}
	if !strings.Contains(err.Error(), "persist remote deduction binding failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	var key apikeymodel.TenantAPIKey
	if err := db.Where("id = ?", apiKeyID).First(&key).Error; err != nil {
		t.Fatalf("failed to load api key: %v", err)
	}
	if key.RemainQuota != 100 {
		t.Fatalf("remain_quota = %d, want 100 after local rollback", key.RemainQuota)
	}

	if len(client.settleRequests) != 1 {
		t.Fatalf("remote settle compensation calls = %d, want 1", len(client.settleRequests))
	}
	compReq := client.settleRequests[0]
	if compReq.DeductionID != "deduction-bind-fail-1" || compReq.ActualCredits != 0 || compReq.Status != "error" {
		t.Fatalf("unexpected compensation settle request: deduction=%s actual=%d status=%s", compReq.DeductionID, compReq.ActualCredits, compReq.Status)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusPredeductFailed {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusPredeductFailed)
	}
}

func TestRemoteBilling_SettleViaGRPC_PersistsInvocationInSettlePending(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	migrateRemoteBillingTables(t, db)

	orgID := uuid.NewString()
	apiKeyID := "key-pending-invocation-1"
	seedBillingAPIKey(t, db, orgID, apiKeyID, 100, ptrInt64(100))

	called := false
	client := &fakeQuotaClient{
		preDeductResp: &PreDeductQuotaResponse{
			Success:     true,
			DeductionID: "deduction-pending-invocation-1",
		},
		settleResp: &SettleQuotaResponse{Success: true},
		onSettle: func(req *SettleQuotaRequest) error {
			called = true
			var attempt BillingAttempt
			if err := db.Where("attempt_id = ?", req.AttemptID).First(&attempt).Error; err != nil {
				return err
			}
			if attempt.Status != billingAttemptStatusSettlePending {
				return fmt.Errorf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusSettlePending)
			}
			if attempt.InvocationResult == nil || *attempt.InvocationResult != "error" {
				return fmt.Errorf("invocation_result = %v, want error", attempt.InvocationResult)
			}
			return nil
		},
	}
	rb := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   client,
	}

	bc := &BillingContext{
		APIKeyID:          apiKeyID,
		OrganizationID:    orgID,
		AttemptID:         "req-pending-invocation-a1",
		RequestID:         "req-pending-invocation",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    apiKeyID,
		EstimatedCredits:  8,
		ActualCredits:     0,
		UseSystemProvider: true,
		Status:            "error",
	}
	if err := rb.preDeductViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("preDeductViaGRPC returned error: %v", err)
	}
	if err := rb.settleViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("settleViaGRPC returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected onSettle callback to be called")
	}
}

func TestRemoteBilling_SettleViaGRPC_SuccessUpsertsUsageBill(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	migrateRemoteBillingTables(t, db)

	orgID := uuid.NewString()
	apiKeyID := "key-usage-bill-1"
	seedBillingAPIKey(t, db, orgID, apiKeyID, 100, ptrInt64(100))

	client := &fakeQuotaClient{
		preDeductResp: &PreDeductQuotaResponse{
			Success:     true,
			DeductionID: "deduction-usage-bill-1",
		},
		settleResp: &SettleQuotaResponse{Success: true, UsedQuota: 107, SettledCredits: 7},
	}
	rb := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   client,
	}

	startedAt := time.Now().UTC().Add(-2 * time.Minute)
	providerID := uuid.New()
	modelID := uuid.New()
	routeID := uuid.New()

	bc := &BillingContext{
		APIKeyID:          apiKeyID,
		OrganizationID:    orgID,
		AttemptID:         "req-usage-bill-a1",
		RequestID:         "req-usage-bill",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    apiKeyID,
		ModelID:           modelID,
		ModelName:         "gpt-4o",
		ProviderID:        providerID,
		ProviderName:      "openai",
		ChannelID:         &routeID,
		EstimatedCredits:  12,
		ActualCredits:     9,
		PromptTokens:      20,
		CompletionTokens:  5,
		TotalTokens:       25,
		UseSystemProvider: true,
		Status:            "success",
		RequestCreatedAt:  startedAt,
		ResponseTime:      321,
	}

	if err := rb.preDeductViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("preDeductViaGRPC returned error: %v", err)
	}
	if err := rb.settleViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("first settleViaGRPC returned error: %v", err)
	}
	if err := rb.settleViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("second settleViaGRPC returned error: %v", err)
	}

	var bills []UsageBill
	if err := db.Where("attempt_id = ?", bc.AttemptID).Find(&bills).Error; err != nil {
		t.Fatalf("failed to load usage bills: %v", err)
	}
	if len(bills) != 1 {
		t.Fatalf("usage bill rows = %d, want 1", len(bills))
	}

	bill := bills[0]
	if bill.Status != usageBillStatusSuccess {
		t.Fatalf("bill status = %s, want %s", bill.Status, usageBillStatusSuccess)
	}
	if !bill.UseSystemProvider {
		t.Fatalf("bill use_system_provider = false, want true")
	}
	if bill.BillingLane != UsageBillingLanePlatform {
		t.Fatalf("bill billing_lane = %s, want %s", bill.BillingLane, UsageBillingLanePlatform)
	}
	if bill.RemoteDeductionID == nil || *bill.RemoteDeductionID != "deduction-usage-bill-1" {
		t.Fatalf("bill remote_deduction_id = %v, want deduction-usage-bill-1", bill.RemoteDeductionID)
	}
	if bill.OfficialPoints != 7 {
		t.Fatalf("bill official_points = %d, want 7", bill.OfficialPoints)
	}
	if bill.PrivatePoints != 0 {
		t.Fatalf("bill private_points = %d, want 0", bill.PrivatePoints)
	}
	if bill.TotalTokens != 25 || bill.PromptTokens != 20 || bill.CompletionTokens != 5 {
		t.Fatalf("bill tokens = %d/%d/%d, want 25/20/5", bill.TotalTokens, bill.PromptTokens, bill.CompletionTokens)
	}
	if bill.ModelName != "gpt-4o" || bill.ProviderName != "openai" {
		t.Fatalf("bill model/provider = %s/%s, want gpt-4o/openai", bill.ModelName, bill.ProviderName)
	}
	if bill.APIKeyID != apiKeyID {
		t.Fatalf("bill api_key_id = %s, want %s", bill.APIKeyID, apiKeyID)
	}
	if bill.RequestCreatedAt.Sub(startedAt) > time.Second || startedAt.Sub(bill.RequestCreatedAt) > time.Second {
		t.Fatalf("bill request_created_at = %v, want close to %v", bill.RequestCreatedAt, startedAt)
	}
	if bill.SettledAt.IsZero() {
		t.Fatalf("bill settled_at should be set")
	}
}
