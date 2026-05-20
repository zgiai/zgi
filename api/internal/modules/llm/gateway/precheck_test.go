package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
)

type fakeOfficialCreditChecker struct {
	balance int64
	err     error
}

func (f *fakeOfficialCreditChecker) GetOfficialBalance(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.balance, nil
}

func TestChannelRouter_SelectCandidateRoutesForAttemptWindow(t *testing.T) {
	router := &ChannelRouter{}
	routes := []*channelmodel.LLMRoute{
		{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Priority: 20, Weight: 1},
		{ID: uuid.MustParse("22222222-2222-2222-2222-222222222222"), Priority: 20, Weight: 1},
		{ID: uuid.MustParse("33333333-3333-3333-3333-333333333333"), Priority: 10, Weight: 1},
		{ID: uuid.MustParse("44444444-4444-4444-4444-444444444444"), Priority: 10, Weight: 1},
		{ID: uuid.MustParse("55555555-5555-5555-5555-555555555555"), Priority: 10, Weight: 1},
		{ID: uuid.MustParse("66666666-6666-6666-6666-666666666666"), Priority: 1, Weight: 1},
	}

	selected := router.selectCandidateRoutesForAttemptWindow(routes, 3)
	if len(selected) != 5 {
		t.Fatalf("len(selected) = %d, want 5", len(selected))
	}
	for _, route := range selected {
		if route.ID == uuid.MustParse("66666666-6666-6666-6666-666666666666") {
			t.Fatalf("unexpected low-priority route included: %s", route.ID)
		}
	}
}

func TestBillingRouting_CheckBalanceAndPreDeduct_Official_InsufficientBalance_ReturnsStructuredUserError(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: false}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{billing: remote, localBilling: local}
	ps := &ProviderSelection{UseSystemProvider: true}
	channelID := uuid.New()
	bc := &BillingContext{APIKeyID: "k1", UseSystemProvider: true, ChannelID: &channelID}

	err := s.checkBalanceAndPreDeduct(context.Background(), ps, uuid.New(), uuid.New(), 123, bc)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("checkBalanceAndPreDeduct err = %v, want %v", err, ErrInsufficientBalance)
	}

	var userErr *BillingUserError
	if !errors.As(err, &userErr) {
		t.Fatalf("errors.As(err, *BillingUserError) = false, err = %v", err)
	}
	if userErr.Kind != BillingUserErrorKindOrganizationBalanceInsufficient {
		t.Fatalf("userErr.Kind = %q, want %q", userErr.Kind, BillingUserErrorKindOrganizationBalanceInsufficient)
	}
}

func TestBillingRouting_CheckBalanceAndPreDeduct_WorkspaceQuota_ReturnsStructuredUserError(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true, preDeductErr: ErrInsufficientQuota}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{billing: remote, localBilling: local}
	ps := &ProviderSelection{UseSystemProvider: true}
	channelID := uuid.New()
	bc := &BillingContext{
		APIKeyID:          "k1",
		UseSystemProvider: true,
		ChannelID:         &channelID,
		QuotaSubjectType:  quotaSubjectTypeWorkspace,
		QuotaSubjectID:    "ws-1",
	}

	err := s.checkBalanceAndPreDeduct(context.Background(), ps, uuid.New(), uuid.New(), 123, bc)
	if !errors.Is(err, ErrInsufficientQuota) {
		t.Fatalf("checkBalanceAndPreDeduct err = %v, want %v", err, ErrInsufficientQuota)
	}

	var userErr *BillingUserError
	if !errors.As(err, &userErr) {
		t.Fatalf("errors.As(err, *BillingUserError) = false, err = %v", err)
	}
	if userErr.Kind != BillingUserErrorKindWorkspaceQuotaInsufficient {
		t.Fatalf("userErr.Kind = %q, want %q", userErr.Kind, BillingUserErrorKindWorkspaceQuotaInsufficient)
	}
}

func TestEvaluateCandidateRouteWarnings_RequiresAllCandidateLanesLow(t *testing.T) {
	db := openWorkspaceQuotaTestDB(t)
	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	privateRouteID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	if err := db.Create(&ChannelWallet{
		ChannelID:      privateRouteID,
		OrganizationID: orgID,
		Balance:        800000,
		Status:         channelWalletStatusActive,
	}).Error; err != nil {
		t.Fatalf("seed channel wallet: %v", err)
	}

	s := &llmGatewayServiceImpl{db: db, officialCreditChecker: &fakeOfficialCreditChecker{balance: 300000}}

	healthy, warnings, err := s.evaluateCandidateRouteWarnings(context.Background(), orgID, []*channelmodel.LLMRoute{
		{ID: uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"), IsOfficial: true, Type: shared.RouteTypeZGICloud},
		{ID: privateRouteID, IsOfficial: false, Type: shared.RouteTypePrivate, OrganizationID: orgID},
	})
	if err != nil {
		t.Fatalf("evaluateCandidateRouteWarnings returned error: %v", err)
	}
	if !healthy {
		t.Fatalf("healthy = false, want true")
	}
	if len(warnings) != 0 {
		t.Fatalf("len(warnings) = %d, want 0", len(warnings))
	}
}

func TestEvaluateCandidateRouteWarnings_OfficialLowBalanceUsesScaledThreshold(t *testing.T) {
	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	s := &llmGatewayServiceImpl{
		officialCreditChecker: &fakeOfficialCreditChecker{balance: 499999},
	}

	healthy, warnings, err := s.evaluateCandidateRouteWarnings(context.Background(), orgID, []*channelmodel.LLMRoute{
		{ID: uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"), IsOfficial: true, Type: shared.RouteTypeZGICloud},
	})
	if err != nil {
		t.Fatalf("evaluateCandidateRouteWarnings returned error: %v", err)
	}
	if healthy {
		t.Fatalf("healthy = true, want false")
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1", len(warnings))
	}
	if warnings[0].Kind != AppModelRouteWarningKindOrganizationBalanceLow {
		t.Fatalf("warning kind = %q, want %q", warnings[0].Kind, AppModelRouteWarningKindOrganizationBalanceLow)
	}
	if warnings[0].CurrentValue != 499999 {
		t.Fatalf("currentValue = %d, want 499999", warnings[0].CurrentValue)
	}
	if warnings[0].Threshold != workflowOrganizationLowBalanceThreshold {
		t.Fatalf("threshold = %d, want %d", warnings[0].Threshold, workflowOrganizationLowBalanceThreshold)
	}
}

func TestEvaluateCandidateRouteWarnings_PrivateChannelLowBalanceUsesScaledThreshold(t *testing.T) {
	db := openWorkspaceQuotaTestDB(t)
	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	privateRouteID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	if err := db.Create(&ChannelWallet{
		ChannelID:      privateRouteID,
		OrganizationID: orgID,
		Balance:        499999,
		Status:         channelWalletStatusActive,
	}).Error; err != nil {
		t.Fatalf("seed channel wallet: %v", err)
	}

	s := &llmGatewayServiceImpl{
		db:                    db,
		officialCreditChecker: &fakeOfficialCreditChecker{balance: 999999},
	}

	healthy, warnings, err := s.evaluateCandidateRouteWarnings(context.Background(), orgID, []*channelmodel.LLMRoute{
		{ID: privateRouteID, IsOfficial: false, Type: shared.RouteTypePrivate, OrganizationID: orgID},
	})
	if err != nil {
		t.Fatalf("evaluateCandidateRouteWarnings returned error: %v", err)
	}
	if healthy {
		t.Fatalf("healthy = true, want false")
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1", len(warnings))
	}
	if warnings[0].Kind != AppModelRouteWarningKindPrivateChannelBalanceLow {
		t.Fatalf("warning kind = %q, want %q", warnings[0].Kind, AppModelRouteWarningKindPrivateChannelBalanceLow)
	}
	if warnings[0].CurrentValue != 499999 {
		t.Fatalf("currentValue = %d, want 499999", warnings[0].CurrentValue)
	}
	if warnings[0].Threshold != workflowPrivateChannelLowBalanceThreshold {
		t.Fatalf("threshold = %d, want %d", warnings[0].Threshold, workflowPrivateChannelLowBalanceThreshold)
	}
}

func TestBuildWorkspaceQuotaWarning_UsesScaledThreshold(t *testing.T) {
	db := openWorkspaceQuotaTestDB(t)
	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	limit := int64(500000)

	if err := db.AutoMigrate(&WorkspaceQuota{}); err != nil {
		t.Fatalf("failed to migrate workspace quota: %v", err)
	}

	if err := db.Create(&WorkspaceQuota{
		WorkspaceID:    "ws-low",
		OrganizationID: orgID,
		UsedQuota:      1,
		RemainQuota:    99999,
		QuotaLimit:     &limit,
	}).Error; err != nil {
		t.Fatalf("seed workspace quota: %v", err)
	}

	s := &llmGatewayServiceImpl{db: db}
	warning, err := s.buildWorkspaceQuotaWarning(context.Background(), orgID, "ws-low")
	if err != nil {
		t.Fatalf("buildWorkspaceQuotaWarning returned error: %v", err)
	}
	if warning == nil {
		t.Fatalf("warning = nil, want non-nil")
	}
	if warning.Kind != AppModelRouteWarningKindWorkspaceQuotaLow {
		t.Fatalf("warning kind = %q, want %q", warning.Kind, AppModelRouteWarningKindWorkspaceQuotaLow)
	}
	if warning.CurrentValue != 99999 {
		t.Fatalf("currentValue = %d, want 99999", warning.CurrentValue)
	}
	if warning.Threshold != workflowWorkspaceLowQuotaThreshold {
		t.Fatalf("threshold = %d, want %d", warning.Threshold, workflowWorkspaceLowQuotaThreshold)
	}
}
