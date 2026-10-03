package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"gorm.io/gorm"
)

// countingSettleQuotaClient pre-deducts successfully and counts settle calls,
// so a test can prove the stale recovery never re-settled a hold on Console.
type countingSettleQuotaClient struct{ settleCalls int }

func (c *countingSettleQuotaClient) PreDeductQuota(context.Context, *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	return &PreDeductQuotaResponse{Success: true, DeductionID: uuid.NewString()}, nil
}

func (c *countingSettleQuotaClient) PreDeductMeteredUsage(context.Context, *PreDeductMeteredUsageRequest) (*PreDeductMeteredUsageResponse, error) {
	return nil, errors.New("unexpected metered pre-deduct")
}

func (c *countingSettleQuotaClient) SettleQuota(context.Context, *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	c.settleCalls++
	return &SettleQuotaResponse{Success: true}, nil
}

func (c *countingSettleQuotaClient) CheckCreditBalance(context.Context, string, int64) (bool, int64, error) {
	return false, 0, errors.New("unexpected balance check")
}

func (c *countingSettleQuotaClient) Close() error { return nil }

// seedStaleRemotePrededuct books a remote hold and ages it past the stale
// prededuct timeout, the state an async video reaches when its client never
// polls the task.
func seedStaleRemotePrededuct(t *testing.T, db *gorm.DB, client *countingSettleQuotaClient, consoleURL string) (*RemoteBilling, *BillingContext) {
	t.Helper()
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatalf("migrate developer grant: %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS llm_models (id text, name text, provider text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS llm_providers (id text, provider text)`).Error; err != nil {
		t.Fatal(err)
	}
	quota := millipoints.MilliPoints(1000)
	grant := accessmodel.Grant{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 1,
		AllowedModels: []string{}, AuthorizationVersion: 1,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	version := grant.AuthorizationVersion
	bc := &BillingContext{
		OrganizationID: grant.OrganizationID, WorkspaceID: grant.WorkspaceID,
		AttemptID: uuid.NewString() + "-a1", RequestID: uuid.NewString(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		InvocationSource: InvocationSourceAPI, AuthMethod: "personal_api_key",
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: grant.PrincipalID,
		AccessGrantID: grant.ID, GrantAuthorizationVersion: &version,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		ModelID: uuid.New(), ModelName: "seedance-fixture", ProviderID: uuid.New(), ProviderName: "doubao",
		EstimatedCredits: 100, SubjectReservedCredits: 100,
	}
	if err := db.Exec(`INSERT INTO llm_models (id, name, provider) VALUES (?, ?, ?)`, bc.ModelID.String(), bc.ModelName, bc.ProviderName).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_providers (id, provider) VALUES (?, ?)`, bc.ProviderID.String(), bc.ProviderName).Error; err != nil {
		t.Fatal(err)
	}
	seedRemoteBillingPersonalKey(t, db, &grant, bc, "active")
	remote := &RemoteBilling{localService: &BillingService{db: db}, grpcClient: client, consoleAPIBaseURL: consoleURL}
	if err := remote.preDeductViaGRPC(context.Background(), bc); err != nil {
		t.Fatalf("pre-deduct: %v", err)
	}
	staleAt := time.Now().Add(-defaultRemotePredeductTimeout - time.Minute)
	if err := db.Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Update("updated_at", staleAt).Error; err != nil {
		t.Fatal(err)
	}
	return remote, bc
}

func consoleSettlementServer(t *testing.T, body func(bc *BillingContext) string, bc **BillingContext) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body(*bc)))
	}))
	t.Cleanup(server.Close)
	return server
}

// TestStalePredeductRecoveryMirrorsConsoleSettlement pins E1: a hold Console
// already settled must end SETTLED with a usage bill for exactly the credits
// Console charged, and Console's own status, instead of a ROLLED_BACK attempt
// with no bill.
func TestStalePredeductRecoveryMirrorsConsoleSettlement(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	var bc *BillingContext
	server := consoleSettlementServer(t, func(bc *BillingContext) string {
		return fmt.Sprintf(`{"found":true,"settlement_id":%q,"deduction_id":%q,"attempt_id":%q,"organization_id":%q,"request_id":%q,"settlement_status":"SETTLED","reserved_credits":100,"settled_credits":60,"refunded_credits":40,"retryable":false,"pricing_snapshot":{"currency":"USD","cny_per_usd":"7","points_per_usd":"1000"}}`,
			bc.DeductionID, bc.DeductionID, bc.AttemptID, bc.OrganizationID, bc.RequestID)
	}, &bc)
	client := &countingSettleQuotaClient{}
	remote, seeded := seedStaleRemotePrededuct(t, db, client, server.URL)
	bc = seeded

	if err := remote.recoverStalePredeductedRemoteAttempts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.settleCalls != 0 {
		t.Fatalf("settle calls = %d, want 0: a Console-settled hold must not be re-settled", client.settleCalls)
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusSettled || attempt.ConsoleSettlementStatus == nil || *attempt.ConsoleSettlementStatus != "SETTLED" {
		t.Fatalf("attempt status=%s console_settlement_status=%v, want SETTLED/SETTLED", attempt.Status, attempt.ConsoleSettlementStatus)
	}
	var bill UsageBill
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
		t.Fatalf("usage bill missing: %v", err)
	}
	if bill.TotalPoints != 60 || bill.OfficialPoints != 60 || bill.ModelName != "seedance-fixture" || bill.ProviderName != "doubao" {
		t.Fatalf("usage bill = %#v, want 60 official points for seedance-fixture/doubao", bill)
	}
}

// TestStalePredeductRecoveryWaitsForAnOpenConsoleHold covers a video still
// running past the timeout: the hold stays open and nothing is settled.
func TestStalePredeductRecoveryWaitsForAnOpenConsoleHold(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	var bc *BillingContext
	server := consoleSettlementServer(t, func(bc *BillingContext) string {
		return fmt.Sprintf(`{"found":true,"settlement_id":%q,"deduction_id":%q,"attempt_id":%q,"organization_id":%q,"settlement_status":"PENDING","retryable":true}`,
			bc.DeductionID, bc.DeductionID, bc.AttemptID, bc.OrganizationID)
	}, &bc)
	client := &countingSettleQuotaClient{}
	remote, seeded := seedStaleRemotePrededuct(t, db, client, server.URL)
	bc = seeded

	if err := remote.recoverStalePredeductedRemoteAttempts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.settleCalls != 0 {
		t.Fatalf("settle calls = %d, want 0: an open hold must not be released", client.settleCalls)
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusPre {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusPre)
	}
}

// TestStalePredeductRecoveryMirrorsConsoleRelease covers a hold Console already
// released (expired): the attempt is rolled back locally without a settle call
// Console would refuse.
func TestStalePredeductRecoveryMirrorsConsoleRelease(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	var bc *BillingContext
	server := consoleSettlementServer(t, func(bc *BillingContext) string {
		return fmt.Sprintf(`{"found":true,"settlement_id":%q,"deduction_id":%q,"attempt_id":%q,"organization_id":%q,"settlement_status":"ROLLED_BACK","retryable":false}`,
			bc.DeductionID, bc.DeductionID, bc.AttemptID, bc.OrganizationID)
	}, &bc)
	client := &countingSettleQuotaClient{}
	remote, seeded := seedStaleRemotePrededuct(t, db, client, server.URL)
	bc = seeded

	if err := remote.recoverStalePredeductedRemoteAttempts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.settleCalls != 0 {
		t.Fatalf("settle calls = %d, want 0", client.settleCalls)
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusRolledBack || attempt.ConsoleSettlementStatus == nil || *attempt.ConsoleSettlementStatus != billingAttemptStatusRolledBack {
		t.Fatalf("attempt status=%s console_settlement_status=%v, want ROLLED_BACK", attempt.Status, attempt.ConsoleSettlementStatus)
	}
}
