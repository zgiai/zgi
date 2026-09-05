package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type failingSettleQuotaClient struct{}

func (failingSettleQuotaClient) PreDeductQuota(context.Context, *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	return nil, errors.New("unexpected pre-deduct")
}

func (failingSettleQuotaClient) SettleQuota(context.Context, *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	return nil, errors.New("quota service unavailable")
}

func (failingSettleQuotaClient) CheckCreditBalance(context.Context, string, int64) (bool, int64, error) {
	return false, 0, errors.New("unexpected balance check")
}

func (failingSettleQuotaClient) Close() error { return nil }

type capturingFailingSettleQuotaClient struct {
	settleRequest *SettleQuotaRequest
}

func (c *capturingFailingSettleQuotaClient) PreDeductQuota(context.Context, *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	return nil, errors.New("unexpected pre-deduct")
}

func (c *capturingFailingSettleQuotaClient) SettleQuota(_ context.Context, req *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	copy := *req
	c.settleRequest = &copy
	return nil, errors.New("quota service unavailable")
}

func (c *capturingFailingSettleQuotaClient) CheckCreditBalance(context.Context, string, int64) (bool, int64, error) {
	return false, 0, errors.New("unexpected balance check")
}

func (c *capturingFailingSettleQuotaClient) Close() error { return nil }

type emptyDeductionIDQuotaClient struct{}

func (emptyDeductionIDQuotaClient) PreDeductQuota(context.Context, *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	return &PreDeductQuotaResponse{Success: true}, nil
}

func (emptyDeductionIDQuotaClient) SettleQuota(context.Context, *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	return nil, errors.New("unexpected settle")
}

func (emptyDeductionIDQuotaClient) CheckCreditBalance(context.Context, string, int64) (bool, int64, error) {
	return false, 0, errors.New("unexpected balance check")
}

func (emptyDeductionIDQuotaClient) Close() error { return nil }

type lateDeductionQuotaClient struct {
	onPreDeduct func(context.Context)
	settleCalls int
	requests    []*SettleQuotaRequest
}

func (c *lateDeductionQuotaClient) PreDeductQuota(ctx context.Context, _ *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	if c.onPreDeduct != nil {
		c.onPreDeduct(ctx)
	}
	return &PreDeductQuotaResponse{Success: true, DeductionID: "remote-deduction-after-timeout"}, nil
}

func (c *lateDeductionQuotaClient) SettleQuota(_ context.Context, req *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	copy := *req
	c.requests = append(c.requests, &copy)
	c.settleCalls++
	if c.settleCalls == 1 {
		return nil, errors.New("temporary compensation outage")
	}
	return &SettleQuotaResponse{Success: true, SettledCredits: 0}, nil
}

func (c *lateDeductionQuotaClient) CheckCreditBalance(context.Context, string, int64) (bool, int64, error) {
	return false, 0, errors.New("unexpected balance check")
}

func (c *lateDeductionQuotaClient) Close() error { return nil }

func openRemoteBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&BillingAttempt{}, &BillingAttemptEntry{}, &UsageBill{}); err != nil {
		t.Fatalf("automigrate billing tables: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX uq_billing_attempt_entry ON billing_attempt_entries (attempt_id, entry_type, ledger_type)`).Error; err != nil {
		t.Fatalf("create billing attempt entry unique index: %v", err)
	}
	return db
}

func seedRemoteBillingPersonalKey(t *testing.T, db *gorm.DB, grant *accessmodel.Grant, bc *BillingContext, status string) apikeymodel.TenantAPIKey {
	t.Helper()
	if err := db.AutoMigrate(&apikeymodel.TenantAPIKey{}); err != nil {
		t.Fatalf("migrate personal api key: %v", err)
	}
	principalType, workspaceID := accessmodel.PrincipalTypeUser, grant.WorkspaceID
	key := apikeymodel.TenantAPIKey{
		OrganizationID:       grant.OrganizationID,
		WorkspaceID:          &workspaceID,
		PrincipalType:        &principalType,
		PrincipalID:          &grant.PrincipalID,
		AccessGrantID:        &grant.ID,
		KeyHash:              uuid.NewString(),
		Name:                 "remote billing personal key",
		Status:               status,
		SecretVersion:        2,
		AuthorizationVersion: grant.AuthorizationVersion,
	}
	if err := db.Omit("Key").Create(&key).Error; err != nil {
		t.Fatalf("create personal api key: %v", err)
	}
	bc.APIKeyID = key.ID
	return key
}

func TestRemoteBillingMarkAttemptSettleFailedWritesPartialUsageBill(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	remote := &RemoteBilling{localService: &BillingService{db: db}}
	bc := testUsageBillContext(time.Now().Add(-time.Second), time.Now())
	bc.BillingLane = UsageBillingLanePlatform
	bc.UseSystemProvider = true
	bc.ActualCredits = 9

	err := remote.markAttemptSettleFailed(context.Background(), bc, "SETTLE_FAILED", "grpc down")
	if err != nil {
		t.Fatalf("markAttemptSettleFailed returned error: %v", err)
	}

	var bill UsageBill
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
		t.Fatalf("load usage bill: %v", err)
	}
	if bill.Status != usageBillStatusPartial {
		t.Fatalf("usage bill status = %q, want %q", bill.Status, usageBillStatusPartial)
	}
	if bill.OfficialPoints != 9 || bill.TotalPoints != 9 {
		t.Fatalf("usage bill points = official %d total %d, want 9/9", bill.OfficialPoints, bill.TotalPoints)
	}
	if bill.ErrorCode == nil || *bill.ErrorCode != "SETTLE_FAILED" {
		t.Fatalf("usage bill error code = %v, want SETTLE_FAILED", bill.ErrorCode)
	}
}

func TestRemoteBillingEmptyDeductionIDRollsBackLocalGrantReservation(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatalf("migrate developer grant: %v", err)
	}
	quota := int64(100)
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
		AttemptID: uuid.NewString(), RequestID: uuid.NewString(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		InvocationSource: InvocationSourceAPI, AuthMethod: "personal_api_key",
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: grant.PrincipalID,
		AccessGrantID: grant.ID, GrantAuthorizationVersion: &version,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: 100,
	}
	seedRemoteBillingPersonalKey(t, db, &grant, bc, "active")
	remote := &RemoteBilling{localService: &BillingService{db: db}, grpcClient: emptyDeductionIDQuotaClient{}}
	if err := remote.preDeductViaGRPC(context.Background(), bc); err == nil {
		t.Fatal("empty remote deduction ID unexpectedly succeeded")
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != 0 || grant.RemainQuota != quota {
		t.Fatalf("invalid remote response leaked grant reservation: used/remain=%d/%d", grant.UsedQuota, grant.RemainQuota)
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusPredeductFailed || attempt.ErrorCode == nil || *attempt.ErrorCode != "PREDEDUCT_INVALID_RESPONSE" {
		t.Fatalf("invalid response attempt state = %#v", attempt)
	}
}

func TestRemoteBillingRejectsInactivePersonalKeyAndRollsBackGrantReservation(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatalf("migrate developer grant: %v", err)
	}
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 1,
		AllowedModels: []string{}, AuthorizationVersion: 2,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	version := grant.AuthorizationVersion
	bc := &BillingContext{
		OrganizationID: grant.OrganizationID, WorkspaceID: grant.WorkspaceID,
		AttemptID: uuid.NewString(), RequestID: uuid.NewString(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		InvocationSource: InvocationSourceAPI, AuthMethod: "personal_api_key",
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: grant.PrincipalID,
		AccessGrantID: grant.ID, GrantAuthorizationVersion: &version,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: 25,
	}
	seedRemoteBillingPersonalKey(t, db, &grant, bc, "inactive")
	remote := &RemoteBilling{localService: &BillingService{db: db}}
	if err := remote.preDeductLocalSubjectQuota(context.Background(), bc); !errors.Is(err, ErrAPIKeyInactive) {
		t.Fatalf("inactive remote personal key pre-deduct error = %v, want %v", err, ErrAPIKeyInactive)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != 0 || grant.RemainQuota != quota {
		t.Fatalf("inactive remote personal key left a grant reservation: used/remain=%d/%d", grant.UsedQuota, grant.RemainQuota)
	}
}

func TestRemoteBillingRecoversStaleInitWithoutBoundDeduction(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatalf("migrate developer grant: %v", err)
	}
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 1,
		AllowedModels: []string{}, AuthorizationVersion: 3,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	version := grant.AuthorizationVersion
	bc := &BillingContext{
		OrganizationID: grant.OrganizationID, WorkspaceID: grant.WorkspaceID,
		AttemptID: uuid.NewString(), RequestID: uuid.NewString(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		InvocationSource: InvocationSourceAPI, AuthMethod: "personal_api_key",
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: grant.PrincipalID,
		AccessGrantID: grant.ID, GrantAuthorizationVersion: &version,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: quota,
	}
	seedRemoteBillingPersonalKey(t, db, &grant, bc, "active")
	remote := &RemoteBilling{localService: &BillingService{db: db}}
	if err := remote.preDeductLocalSubjectQuota(context.Background(), bc); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.RemainQuota != 0 {
		t.Fatalf("pre-deduct grant remain = %d, want 0", grant.RemainQuota)
	}
	staleAt := time.Now().Add(-defaultRemoteInitTimeout - time.Minute)
	if err := db.Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Update("updated_at", staleAt).Error; err != nil {
		t.Fatal(err)
	}
	if err := remote.recoverStaleRemoteInitAttempts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := remote.rollbackLocalSubjectQuotaAndMarkFailed(context.Background(), bc, "LATE_GRPC_FAILURE", "late response"); err != nil {
		t.Fatalf("late gRPC failure handling must be idempotent: %v", err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != 0 || grant.RemainQuota != quota {
		t.Fatalf("stale INIT recovery grant used/remain = %d/%d, want 0/%d", grant.UsedQuota, grant.RemainQuota, quota)
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusPredeductFailed || attempt.ErrorCode == nil || *attempt.ErrorCode != "REMOTE_PREDEDUCT_TIMEOUT_NO_DEDUCTION_ID" {
		t.Fatalf("stale INIT recovery attempt = %#v", attempt)
	}
	var subjectEntry, fundEntry BillingAttemptEntry
	if err := db.Where("attempt_id = ? AND entry_type = ?", bc.AttemptID, billingEntryTypeSubject).First(&subjectEntry).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("attempt_id = ? AND entry_type = ?", bc.AttemptID, billingEntryTypeFund).First(&fundEntry).Error; err != nil {
		t.Fatal(err)
	}
	if subjectEntry.Status != billingEntryStatusRolled || subjectEntry.ActualAmount != 0 || subjectEntry.RefundedAmount != quota {
		t.Fatalf("stale INIT subject entry = %#v", subjectEntry)
	}
	if fundEntry.Status != billingEntryStatusFailed || fundEntry.RefundedAmount != 0 {
		t.Fatalf("stale INIT unbound fund entry = %#v", fundEntry)
	}
	// Recovery is idempotent and must not credit the Grant twice.
	if err := remote.recoverStaleRemoteInitAttempts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.RemainQuota != quota {
		t.Fatalf("second stale INIT recovery changed grant remain to %d", grant.RemainQuota)
	}
}

func TestRemoteBillingPersistsAndRetriesFailedLateDeductionCompensation(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatalf("migrate developer grant: %v", err)
	}
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 1,
		AllowedModels: []string{}, AuthorizationVersion: 5,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	version := grant.AuthorizationVersion
	bc := &BillingContext{
		OrganizationID: grant.OrganizationID, WorkspaceID: grant.WorkspaceID,
		AttemptID: uuid.NewString(), RequestID: uuid.NewString(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		InvocationSource: InvocationSourceAPI, AuthMethod: "personal_api_key",
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: grant.PrincipalID,
		AccessGrantID: grant.ID, GrantAuthorizationVersion: &version,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: quota, SubjectReservedCredits: quota,
	}
	seedRemoteBillingPersonalKey(t, db, &grant, bc, "active")
	client := &lateDeductionQuotaClient{}
	remote := &RemoteBilling{localService: &BillingService{db: db}, grpcClient: client}
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	client.onPreDeduct = func(ctx context.Context) {
		staleAt := time.Now().Add(-defaultRemoteInitTimeout - time.Minute)
		if err := db.Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Update("updated_at", staleAt).Error; err != nil {
			t.Fatalf("mark attempt stale: %v", err)
		}
		if err := remote.recoverStaleRemoteInitAttempt(ctx, bc.AttemptID, time.Now()); err != nil {
			t.Fatalf("recover stale attempt during remote call: %v", err)
		}
		cancelRequest()
	}

	if err := remote.preDeductViaGRPC(requestCtx, bc); err == nil {
		t.Fatal("late remote deduction unexpectedly succeeded")
	}
	if client.settleCalls != 1 {
		t.Fatalf("immediate compensation calls = %d, want 1", client.settleCalls)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != 0 || grant.RemainQuota != quota {
		t.Fatalf("late deduction changed grant after local timeout: used/remain=%d/%d", grant.UsedQuota, grant.RemainQuota)
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusCompensationPending || attempt.NextReconcileAt == nil {
		t.Fatalf("failed remote compensation was not queued: %#v", attempt)
	}
	var fundEntry BillingAttemptEntry
	if err := db.Where("attempt_id = ? AND entry_type = ?", bc.AttemptID, billingEntryTypeFund).First(&fundEntry).Error; err != nil {
		t.Fatal(err)
	}
	if fundEntry.IdempotencyKey == nil || *fundEntry.IdempotencyKey != bc.DeductionID || fundEntry.Status != billingEntryStatusPending {
		t.Fatalf("remote deduction identity was not persisted: %#v", fundEntry)
	}

	if err := remote.reconcilePendingRemoteCompensations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.settleCalls != 2 {
		t.Fatalf("compensation calls after reconcile = %d, want 2", client.settleCalls)
	}
	if got := client.requests[1]; got.DeductionID != bc.DeductionID || got.ActualCredits != 0 || got.Status != "error" {
		t.Fatalf("retry compensation request = %#v", got)
	}
	attempt = BillingAttempt{}
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusCompensated || attempt.NextReconcileAt != nil || attempt.ReconcileAttempts != 1 {
		t.Fatalf("reconciled compensation attempt = %#v", attempt)
	}
	fundEntryID := fundEntry.ID
	fundEntry = BillingAttemptEntry{}
	if err := db.First(&fundEntry, "id = ?", fundEntryID).Error; err != nil {
		t.Fatal(err)
	}
	if fundEntry.Status != billingEntryStatusRolled || fundEntry.ActualAmount != 0 || fundEntry.RefundedAmount != quota {
		t.Fatalf("reconciled compensation fund entry = %#v", fundEntry)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != 0 || grant.RemainQuota != quota {
		t.Fatalf("compensation retry changed grant twice: used/remain=%d/%d", grant.UsedQuota, grant.RemainQuota)
	}
}

func TestRemoteBillingFinalizationIsIdempotentUnderAttemptLock(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatalf("migrate developer grant: %v", err)
	}
	organizationID := uuid.New()
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: organizationID.String(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, UsedQuota: 20, RemainQuota: 70, MaxKeys: 2,
		AllowedModels: []string{}, AuthorizationVersion: 4,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	attemptID, requestID := uuid.NewString(), uuid.NewString()
	version := grant.AuthorizationVersion
	attempt := BillingAttempt{
		AttemptID: attemptID, RequestID: requestID, OrganizationID: organizationID,
		Lane: billingAttemptLaneRemote, InvocationSource: InvocationSourceAPI,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		GrantAuthorizationVersion: &version, AuthMethod: "personal_api_key",
		Status: billingAttemptStatusSettlePending, CreatedAt: now, UpdatedAt: now,
	}
	entries := []BillingAttemptEntry{
		{AttemptID: attemptID, EntryType: billingEntryTypeSubject, LedgerType: billingLedgerTypeGrantQuota, LedgerRefID: grant.ID, ReservedAmount: 10, Status: billingEntryStatusPending, CreatedAt: now, UpdatedAt: now},
		{AttemptID: attemptID, EntryType: billingEntryTypeFund, LedgerType: billingLedgerTypeOrgFunds, LedgerRefID: organizationID.String(), ReservedAmount: 10, Status: billingEntryStatusPending, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}

	bc := &BillingContext{
		OrganizationID: organizationID.String(), AttemptID: attemptID, RequestID: requestID,
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID, AccessGrantID: grant.ID,
		GrantAuthorizationVersion: &version, AuthMethod: "personal_api_key",
		EstimatedCredits: 10, SubjectReservedCredits: 10, ActualCredits: 3, Status: "success",
	}
	remote := &RemoteBilling{localService: &BillingService{db: db}}
	if err := remote.finalizeRemoteSettlement(context.Background(), bc); err != nil {
		t.Fatalf("first finalization: %v", err)
	}
	if err := remote.finalizeRemoteSettlement(context.Background(), bc); err != nil {
		t.Fatalf("duplicate finalization: %v", err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != 23 || grant.RemainQuota != 77 {
		t.Fatalf("duplicate finalization changed grant twice: used/remain = %d/%d, want 23/77", grant.UsedQuota, grant.RemainQuota)
	}

	alreadyFinalized, err := remote.prepareRemoteSettlement(context.Background(), bc)
	if err != nil {
		t.Fatalf("prepare after terminal settlement: %v", err)
	}
	if !alreadyFinalized {
		t.Fatal("terminal attempt was reopened by a later finalizer")
	}
	if err := remote.markAttemptSettleFailed(context.Background(), bc, "LATE_FAILURE", "stale finalizer"); err != nil {
		t.Fatalf("late failure marker: %v", err)
	}
	var storedAttempt BillingAttempt
	if err := db.First(&storedAttempt, "attempt_id = ?", attemptID).Error; err != nil {
		t.Fatal(err)
	}
	if storedAttempt.Status != billingAttemptStatusSettled {
		t.Fatalf("attempt status = %q, want settled", storedAttempt.Status)
	}
	var subjectEntry BillingAttemptEntry
	if err := db.Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeSubject).First(&subjectEntry).Error; err != nil {
		t.Fatal(err)
	}
	if subjectEntry.Status != billingEntryStatusSettled || subjectEntry.ActualAmount != 3 || subjectEntry.RefundedAmount != 7 {
		t.Fatalf("terminal subject ledger was reopened: %#v", subjectEntry)
	}
}

func TestBillingAttemptRecoveryPreservesInvocationSource(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	service := &BillingService{db: db}
	bc := testUsageBillContext(time.Now().Add(-time.Second), time.Now())
	bc.InvocationSource = InvocationSourceAPI
	bc.EstimatedCredits = 11
	apiKeyID, workspaceID, accountID, grantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	bc.APIKeyID = apiKeyID.String()
	bc.WorkspaceID = workspaceID.String()
	bc.AccountID = &accountID
	bc.PrincipalType = "user"
	bc.PrincipalID = accountID.String()
	bc.AccessGrantID = grantID.String()
	grantVersion := int64(12)
	bc.GrantAuthorizationVersion = &grantVersion
	bc.AuthMethod = "personal_api_key"
	bc.QuotaSubjectType = quotaSubjectTypeAccessGrant
	bc.QuotaSubjectID = grantID.String()

	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.upsertAttemptInit(context.Background(), tx, bc)
	}); err != nil {
		t.Fatalf("upsert billing attempt: %v", err)
	}
	// A retry may only carry the stable attempt identity. It must not erase the
	// authorization version needed to protect a renewed grant during settlement.
	retry := *bc
	retry.GrantAuthorizationVersion = nil
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.upsertAttemptInit(context.Background(), tx, &retry)
	}); err != nil {
		t.Fatalf("retry billing attempt upsert: %v", err)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("load billing attempt: %v", err)
	}
	if attempt.InvocationSource != InvocationSourceAPI {
		t.Fatalf("persisted invocation source = %q, want %q", attempt.InvocationSource, InvocationSourceAPI)
	}
	if attempt.APIKeyID == nil || *attempt.APIKeyID != apiKeyID || attempt.WorkspaceID == nil || *attempt.WorkspaceID != workspaceID ||
		attempt.AccountID == nil || *attempt.AccountID != accountID || attempt.AccessGrantID == nil || *attempt.AccessGrantID != grantID ||
		attempt.PrincipalType == nil || *attempt.PrincipalType != "user" || attempt.PrincipalID == nil || *attempt.PrincipalID != accountID.String() ||
		attempt.AuthMethod != "personal_api_key" {
		t.Fatalf("persisted principal attribution is incomplete: %#v", attempt)
	}
	if attempt.GrantAuthorizationVersion == nil || *attempt.GrantAuthorizationVersion != grantVersion {
		t.Fatalf("persisted grant authorization version = %v, want %d", attempt.GrantAuthorizationVersion, grantVersion)
	}

	recovered, err := service.buildLocalRecoveryBillingContext(context.Background(), bc.AttemptID)
	if err != nil {
		t.Fatalf("build recovery billing context: %v", err)
	}
	if recovered.InvocationSource != InvocationSourceAPI {
		t.Fatalf("recovered invocation source = %q, want %q", recovered.InvocationSource, InvocationSourceAPI)
	}
	if recovered.APIKeyID != apiKeyID.String() || recovered.WorkspaceID != workspaceID.String() || recovered.AccountID == nil || *recovered.AccountID != accountID ||
		recovered.PrincipalType != "user" || recovered.PrincipalID != accountID.String() || recovered.AccessGrantID != grantID.String() || recovered.AuthMethod != "personal_api_key" {
		t.Fatalf("recovered principal attribution is incomplete: %#v", recovered)
	}
	if recovered.GrantAuthorizationVersion == nil || *recovered.GrantAuthorizationVersion != grantVersion {
		t.Fatalf("recovered grant authorization version = %v, want %d", recovered.GrantAuthorizationVersion, grantVersion)
	}
}

func TestRemoteBillingRecoveryPreservesInvocationSourceInPartialUsageBill(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	organizationID := uuid.New()
	attemptID := uuid.NewString()
	deductionID := uuid.NewString()
	invocationResult := "success"
	now := time.Now().UTC()
	apiKeyID, workspaceID, accountID, grantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	principalType, principalID := "user", accountID.String()
	attempt := BillingAttempt{
		AttemptID:        attemptID,
		RequestID:        uuid.NewString(),
		OrganizationID:   organizationID,
		Lane:             billingAttemptLaneRemote,
		InvocationSource: InvocationSourceAPI,
		QuotaSubjectType: quotaSubjectTypeAccessGrant,
		QuotaSubjectID:   grantID.String(),
		APIKeyID:         &apiKeyID,
		WorkspaceID:      &workspaceID,
		AccountID:        &accountID,
		PrincipalType:    &principalType,
		PrincipalID:      &principalID,
		AccessGrantID:    &grantID,
		AuthMethod:       "personal_api_key",
		Status:           billingAttemptStatusSettlePending,
		InvocationResult: &invocationResult,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	entries := []BillingAttemptEntry{
		{
			AttemptID: attemptID, EntryType: billingEntryTypeSubject,
			LedgerType: billingLedgerTypeGrantQuota, LedgerRefID: grantID.String(),
			ReservedAmount: 7, ActualAmount: 7, Status: billingEntryStatusPending,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			AttemptID: attemptID, EntryType: billingEntryTypeFund,
			LedgerType: billingLedgerTypeOrgFunds, LedgerRefID: organizationID.String(),
			ReservedAmount: 7, ActualAmount: 7, Status: billingEntryStatusPending,
			IdempotencyKey: &deductionID, CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := db.Create(&attempt).Error; err != nil {
		t.Fatalf("create billing attempt: %v", err)
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatalf("create billing attempt entries: %v", err)
	}

	remote := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   failingSettleQuotaClient{},
	}
	if err := remote.reconcileAttempt(context.Background(), attemptID); err == nil {
		t.Fatal("expected failed quota settlement")
	}

	var bill UsageBill
	if err := db.Where("attempt_id = ?", attemptID).First(&bill).Error; err != nil {
		t.Fatalf("load partial usage bill: %v", err)
	}
	if bill.InvocationSource != InvocationSourceAPI {
		t.Fatalf("partial usage bill invocation source = %q, want %q", bill.InvocationSource, InvocationSourceAPI)
	}
	if bill.APIKeyID != apiKeyID.String() || bill.WorkspaceID == nil || *bill.WorkspaceID != workspaceID.String() ||
		bill.AccountID == nil || *bill.AccountID != accountID || bill.PrincipalType == nil || *bill.PrincipalType != principalType ||
		bill.PrincipalID == nil || *bill.PrincipalID != principalID || bill.AccessGrantID == nil || *bill.AccessGrantID != grantID ||
		bill.AuthMethod != "personal_api_key" {
		t.Fatalf("partial usage bill lost principal attribution: %#v", bill)
	}
}

func TestRemoteBillingReconcileUsesFundActualInsteadOfCappedSubjectCharge(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	organizationID := uuid.New()
	attemptID := uuid.NewString()
	deductionID := uuid.NewString()
	invocationResult := "success"
	now := time.Now().UTC()
	attempt := BillingAttempt{
		AttemptID: attemptID, RequestID: uuid.NewString(), OrganizationID: organizationID,
		Lane: billingAttemptLaneRemote, InvocationSource: InvocationSourceAPI,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: uuid.NewString(),
		AuthMethod: "personal_api_key", Status: billingAttemptStatusPartial,
		InvocationResult: &invocationResult, CreatedAt: now, UpdatedAt: now,
	}
	entries := []BillingAttemptEntry{
		{
			AttemptID: attemptID, EntryType: billingEntryTypeSubject,
			LedgerType: billingLedgerTypeGrantQuota, LedgerRefID: attempt.QuotaSubjectID,
			ReservedAmount: 10, ActualAmount: 3, Status: billingEntryStatusPending,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			AttemptID: attemptID, EntryType: billingEntryTypeFund,
			LedgerType: billingLedgerTypeOrgFunds, LedgerRefID: organizationID.String(),
			ReservedAmount: 0, ActualAmount: 7, Status: billingEntryStatusPending,
			IdempotencyKey: &deductionID, CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := db.Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	client := &capturingFailingSettleQuotaClient{}
	remote := &RemoteBilling{localService: &BillingService{db: db}, grpcClient: client}
	if err := remote.reconcileAttempt(context.Background(), attemptID); err == nil {
		t.Fatal("expected failed quota settlement")
	}
	if client.settleRequest == nil {
		t.Fatal("expected remote settle request")
	}
	if client.settleRequest.ActualCredits != 7 {
		t.Fatalf("remote fund actual credits = %d, want 7", client.settleRequest.ActualCredits)
	}
	if client.settleRequest.EstimatedCredits != 0 {
		t.Fatalf("remote fund reservation = %d, want 0", client.settleRequest.EstimatedCredits)
	}
}
