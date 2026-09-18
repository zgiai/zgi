package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/gorm"
)

func TestMeteredPredeductEnabledForSwitchAndWhitelist(t *testing.T) {
	fact := shared.MeteredUsageFact{Operation: string(PricingOperationImage), Meter: string(PricingMeterImage), BaseUnit: "per_image", Quantity: 1}
	tokenFact := shared.MeteredUsageFact{Operation: operationChatCompletions, Meter: string(PricingMeterInputToken), BaseUnit: "per_1m_tokens", Quantity: 1}

	t.Run("off", func(t *testing.T) {
		t.Setenv(meteredPredeductEnv, "off")
		if meteredPredeductEnabledFor(fact, true) {
			t.Fatal("off must reject the metered branch")
		}
		t.Setenv(meteredPredeductEnv, "")
		if meteredPredeductEnabledFor(fact, true) {
			t.Fatal("empty must fail closed to off")
		}
	})

	t.Run("image", func(t *testing.T) {
		t.Setenv(meteredPredeductEnv, "image")
		if !meteredPredeductEnabledFor(fact, true) {
			t.Fatal("image must allow images.generations")
		}
		if !meteredPredeductEnabledFor(shared.MeteredUsageFact{Operation: string(PricingOperationImage)}, true) {
			t.Fatal("image edits share the image_generation billing operation")
		}
		if meteredPredeductEnabledFor(tokenFact, true) {
			t.Fatal("token lane must never enter the metered branch")
		}
	})

	t.Run("all", func(t *testing.T) {
		t.Setenv(meteredPredeductEnv, "all")
		if !meteredPredeductEnabledFor(fact, true) {
			t.Fatal("all must allow images")
		}
		if meteredPredeductEnabledFor(tokenFact, true) {
			t.Fatal("all must still reject token operations")
		}
	})

	t.Run("no fact", func(t *testing.T) {
		t.Setenv(meteredPredeductEnv, "all")
		if meteredPredeductEnabledFor(shared.MeteredUsageFact{}, false) {
			t.Fatal("a missing metered fact must not open the branch")
		}
	})
}

func TestWithImageMeteredUsageMapsRequestIdentity(t *testing.T) {
	n := 3
	ctx := withImageMeteredUsage(context.Background(), &adapter.ImageRequest{
		N: &n, Size: "1792x1024", Quality: "hd", Style: "vivid",
	})
	fact, ok := shared.MeteredUsageFromContext(ctx)
	if !ok {
		t.Fatal("expected a metered fact in ctx")
	}
	if fact.Operation != "image_generation" || fact.Meter != "image" || fact.BaseUnit != "per_image" {
		t.Fatalf("identity = %s/%s/%s", fact.Operation, fact.Meter, fact.BaseUnit)
	}
	if fact.Quantity != 3 {
		t.Fatalf("quantity = %d, want 3", fact.Quantity)
	}
	if fact.Dimensions["size"] != "1792x1024" || fact.Dimensions["quality"] != "hd" || fact.Dimensions["style"] != "vivid" {
		t.Fatalf("dimensions = %v", fact.Dimensions)
	}
}

func TestWithImageMeteredUsageDefaultsAndCaps(t *testing.T) {
	body := withImageMeteredUsage(context.Background(), &adapter.ImageRequest{})
	fact, _ := shared.MeteredUsageFromContext(body)
	if fact.Quantity != 1 || fact.Dimensions["size"] != "1024x1024" || fact.Dimensions["quality"] != "standard" {
		t.Fatalf("defaults = %#v", fact)
	}

	large := maxImageMeteredQuantity + 25
	cappedCtx := withImageMeteredUsage(context.Background(), &adapter.ImageRequest{N: &large})
	capped, _ := shared.MeteredUsageFromContext(cappedCtx)
	if capped.Quantity != maxImageMeteredQuantity {
		t.Fatalf("capped quantity = %d, want %d", capped.Quantity, maxImageMeteredQuantity)
	}
}

func TestMeteredUsageFromContextRejectsMissing(t *testing.T) {
	if _, ok := shared.MeteredUsageFromContext(context.Background()); ok {
		t.Fatal("empty ctx must not report a metered fact")
	}
}

func TestMapMeteredPreDeductErrorPreservesSentinels(t *testing.T) {
	tests := []struct {
		message string
		target  error
	}{
		{"insufficient credits for reservation", ErrInsufficientBalance},
		{"organization not found", ErrBalanceNotFound},
		{"invalid request", ErrInvalidRequest},
	}
	for _, tc := range tests {
		if err := mapMeteredPreDeductError(tc.message); !errors.Is(err, tc.target) {
			t.Fatalf("mapMeteredPreDeductError(%q) = %v, want %v", tc.message, err, tc.target)
		}
	}
	if err := mapMeteredPreDeductError("metered settlement model not found"); err == nil || errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("unknown message = %v, want a generic error", err)
	}
}

// capturingMeteredQuotaClient records the metered request and simulates a
// successful remote reservation; the legacy path must not be used.
type capturingMeteredQuotaClient struct {
	meteredCalls int
	meteredReq   *PreDeductMeteredUsageRequest
	legacyCalls  int
}

func (c *capturingMeteredQuotaClient) PreDeductQuota(_ context.Context, _ *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	c.legacyCalls++
	return nil, errors.New("legacy path must not run for a metered image")
}

func (c *capturingMeteredQuotaClient) PreDeductMeteredUsage(_ context.Context, req *PreDeductMeteredUsageRequest) (*PreDeductMeteredUsageResponse, error) {
	c.meteredCalls++
	c.meteredReq = req
	return &PreDeductMeteredUsageResponse{Success: true, DeductionID: uuid.NewString(), EstimatedCredits: 100000, ReservedCredits: 100000}, nil
}

func (c *capturingMeteredQuotaClient) SettleQuota(context.Context, *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	return nil, errors.New("unexpected settle")
}

func (c *capturingMeteredQuotaClient) CheckCreditBalance(context.Context, string, int64) (bool, int64, error) {
	return false, 0, errors.New("unexpected balance check")
}

func (c *capturingMeteredQuotaClient) Close() error { return nil }

// seedMeteredAccessGrant mirrors the personal-key fixture used by the other
// remote-billing tests so the local subject pre-deduct can run.
func seedMeteredAccessGrant(t *testing.T, db *gorm.DB) (accessmodel.Grant, int64) {
	t.Helper()
	// A platform-lane reservation with no local quote bounds itself by the whole
	// remaining grant, so the fixture needs headroom beyond one reservation.
	quota := int64(1_000_000)
	grant := accessmodel.Grant{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 1,
		AllowedModels: []string{}, AuthorizationVersion: 1,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("create access grant: %v", err)
	}
	return grant, grant.AuthorizationVersion
}

func TestPreDeductUsesMeteredBranchForImagePlatformLane(t *testing.T) {
	t.Setenv(meteredPredeductEnv, "image")
	db := openRemoteBillingTestDB(t)
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatalf("migrate developer grant: %v", err)
	}
	grant, version := seedMeteredAccessGrant(t, db)
	attemptID := uuid.NewString()
	foreignModelID := uuid.New()
	bc := &BillingContext{
		OrganizationID: grant.OrganizationID, WorkspaceID: grant.WorkspaceID,
		AttemptID: attemptID, RequestID: uuid.NewString(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		InvocationSource: InvocationSourceAPI, AuthMethod: "personal_api_key",
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: grant.PrincipalID,
		AccessGrantID: grant.ID, GrantAuthorizationVersion: &version,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		ModelID: foreignModelID, ModelName: "gpt-image-2", ProviderName: "openai",
		// Set before beginBillingAttempt in production; the metered branch only
		// owns the remote reservation, not the local estimate.
		EstimatedCredits: 100000,
	}
	seedRemoteBillingPersonalKey(t, db, &grant, bc, "active")

	client := &capturingMeteredQuotaClient{}
	remote := &RemoteBilling{localService: &BillingService{db: db}, grpcClient: client}
	// withImageMeteredUsage declares the billing identity: operation
	// image_generation (not the routing endpoint images.generations).
	ctx := withImageMeteredUsage(context.Background(), &adapter.ImageRequest{})

	if err := remote.preDeductViaGRPC(ctx, bc); err != nil {
		var failed BillingAttempt
		_ = db.First(&failed, "attempt_id = ?", attemptID).Error
		t.Fatalf("preDeductViaGRPC() error = %v (attempt status=%s code=%v msg=%v)", err, failed.Status, failed.ErrorCode, failed.ErrorMessage)
	}
	if client.meteredCalls != 1 || client.legacyCalls != 0 {
		t.Fatalf("metered/legacy calls = %d/%d, want 1/0", client.meteredCalls, client.legacyCalls)
	}
	if client.meteredReq == nil {
		t.Fatal("metered request was not captured")
	}
	if client.meteredReq.ModelID != foreignModelID.String() ||
		client.meteredReq.ProviderName != "openai" || client.meteredReq.ModelName != "gpt-image-2" {
		t.Fatalf("metered identity = %q/%q/%q", client.meteredReq.ModelID, client.meteredReq.ProviderName, client.meteredReq.ModelName)
	}
	if client.meteredReq.Usage.Operation != "image_generation" || client.meteredReq.Usage.Meter != "image" ||
		client.meteredReq.Usage.BaseUnit != "per_image" || client.meteredReq.Usage.Quantity != 1 {
		t.Fatalf("metered usage = %#v", client.meteredReq.Usage)
	}
	if bc.DeductionID == "" {
		t.Fatal("deduction id was not bound to the billing context")
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", attemptID).Error; err != nil {
		t.Fatalf("load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusPre {
		t.Fatalf("attempt status = %q, want %q", attempt.Status, billingAttemptStatusPre)
	}
}

// failingMeteredQuotaClient reports an insufficient balance for a metered
// reservation without calling anything else.
type failingMeteredQuotaClient struct{ calls int }

func (c *failingMeteredQuotaClient) PreDeductQuota(context.Context, *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	return nil, errors.New("legacy path must not run")
}

func (c *failingMeteredQuotaClient) PreDeductMeteredUsage(context.Context, *PreDeductMeteredUsageRequest) (*PreDeductMeteredUsageResponse, error) {
	c.calls++
	return &PreDeductMeteredUsageResponse{Success: false, ErrorMessage: "insufficient credits for reservation"}, nil
}

func (c *failingMeteredQuotaClient) SettleQuota(context.Context, *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	return nil, errors.New("unexpected settle")
}

func (c *failingMeteredQuotaClient) CheckCreditBalance(context.Context, string, int64) (bool, int64, error) {
	return false, 0, errors.New("unexpected balance check")
}

func (c *failingMeteredQuotaClient) Close() error { return nil }

func TestPreDeductMeteredInsufficiencyRollsBackWithoutUpstream(t *testing.T) {
	t.Setenv(meteredPredeductEnv, "image")
	db := openRemoteBillingTestDB(t)
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatalf("migrate developer grant: %v", err)
	}
	grant, version := seedMeteredAccessGrant(t, db)
	bc := &BillingContext{
		OrganizationID: grant.OrganizationID, WorkspaceID: grant.WorkspaceID,
		AttemptID: uuid.NewString(), RequestID: uuid.NewString(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		InvocationSource: InvocationSourceAPI, AuthMethod: "personal_api_key",
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: grant.PrincipalID,
		AccessGrantID: grant.ID, GrantAuthorizationVersion: &version,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		ModelID: uuid.New(), ModelName: "gpt-image-2", ProviderName: "openai",
		EstimatedCredits: 100000,
	}
	seedRemoteBillingPersonalKey(t, db, &grant, bc, "active")

	client := &failingMeteredQuotaClient{}
	remote := &RemoteBilling{localService: &BillingService{db: db}, grpcClient: client}
	ctx := withImageMeteredUsage(context.Background(), &adapter.ImageRequest{})
	err := remote.preDeductViaGRPC(ctx, bc)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("preDeductViaGRPC() error = %v, want %v", err, ErrInsufficientBalance)
	}
	if client.calls != 1 {
		t.Fatalf("metered calls = %d, want 1", client.calls)
	}
	// The locally reserved grant credits must return to the subject.
	var stored accessmodel.Grant
	if err := db.First(&stored, "id = ?", grant.ID).Error; err != nil {
		t.Fatalf("load grant: %v", err)
	}
	if stored.RemainQuota != 1_000_000 || stored.UsedQuota != 0 {
		t.Fatalf("grant was not rolled back: remain=%d used=%d", stored.RemainQuota, stored.UsedQuota)
	}
	if bc.DeductionID != "" {
		t.Fatalf("deduction id must stay empty after a failed reservation, got %q", bc.DeductionID)
	}
}

func TestPreDeductKeepsLegacyBranchWhenSwitchOff(t *testing.T) {
	t.Setenv(meteredPredeductEnv, "off")
	db := openRemoteBillingTestDB(t)
	remote := &RemoteBilling{localService: &BillingService{db: db}, grpcClient: failingSettleQuotaClient{}}
	bc := &BillingContext{
		OrganizationID: uuid.NewString(), AttemptID: uuid.NewString(), RequestID: uuid.NewString(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true, EstimatedCredits: 0,
	}
	ctx := withImageMeteredUsage(context.Background(), &adapter.ImageRequest{})
	err := remote.preDeductViaGRPC(ctx, bc)
	// The legacy branch fails on the missing quota subject; the metered branch
	// would not even reach the RPC client, so this proves the branch was skipped.
	if err == nil {
		t.Fatal("legacy branch unexpectedly succeeded without a quota subject")
	}
	if strings.Contains(err.Error(), "grpc metered pre-deduct") {
		t.Fatalf("switch off must not reach the metered path: %v", err)
	}
}
