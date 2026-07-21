package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

func TestEvaluateCandidateRouteWarningsReportsOnlyModelLevelUpstreamBalanceRisk(t *testing.T) {
	tests := []struct {
		name        string
		states      []precheckBalanceState
		wantHealthy bool
		wantScope   AppModelRouteWarningScope
	}{
		{
			name: "one low credential does not warn when other credentials have enough balance",
			states: []precheckBalanceState{
				{remaining: "2", threshold: "5"},
				{remaining: "8", threshold: "5"},
				{remaining: "9", threshold: "5"},
			},
			wantHealthy: true,
		},
		{
			name: "all low credentials are an all warning",
			states: []precheckBalanceState{
				{remaining: "2", threshold: "5"},
				{remaining: "3", threshold: "5"},
			},
			wantScope: AppModelRouteWarningScopeAll,
		},
		{
			name: "all usable credentials warn when another credential is unavailable",
			states: []precheckBalanceState{
				{blockReason: upstreamstate.GuardReasonBalanceExhausted},
				{remaining: "2", threshold: "5"},
			},
			wantScope: AppModelRouteWarningScopeAll,
		},
		{
			name: "stale low balance does not warn",
			states: []precheckBalanceState{
				{remaining: "2", threshold: "5", stale: true},
				{remaining: "8", threshold: "5"},
			},
			wantHealthy: true,
		},
		{
			name: "one low balance and one balance without a threshold does not warn",
			states: []precheckBalanceState{
				{remaining: "2", threshold: "5"},
				{remaining: "8"},
			},
			wantHealthy: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := openGatewayUpstreamGuardDB(t)
			if err := db.AutoMigrate(&ChannelWallet{}); err != nil {
				t.Fatalf("migrate channel wallets: %v", err)
			}

			organizationID := uuid.New()
			routes := make([]*channelmodel.LLMRoute, 0, len(testCase.states))
			for _, balanceState := range testCase.states {
				credentialID := uuid.New()
				route := &channelmodel.LLMRoute{
					ID:             uuid.New(),
					OrganizationID: organizationID,
					Type:           shared.RouteTypePrivate,
					CredentialID:   &credentialID,
				}
				routes = append(routes, route)

				if err := db.Create(&ChannelWallet{
					ChannelID:      route.ID,
					OrganizationID: organizationID,
					Balance:        workflowPrivateChannelLowBalanceThreshold,
					Status:         channelWalletStatusActive,
				}).Error; err != nil {
					t.Fatalf("create channel wallet: %v", err)
				}

				observedAt := time.Now()
				if balanceState.stale {
					observedAt = observedAt.Add(-3 * time.Hour)
				}
				thresholds := []upstreamstate.WarningThreshold{}
				if balanceState.threshold != "" {
					thresholds = append(thresholds, upstreamstate.WarningThreshold{Currency: "USD", Amount: balanceState.threshold})
				}
				availability := upstreamstate.AvailabilityAvailable
				if balanceState.blockReason != "" {
					availability = upstreamstate.AvailabilityExhausted
				}
				if err := db.Create(&upstreamstate.State{
					CredentialID:      credentialID,
					OrganizationID:    organizationID,
					Generation:        1,
					BalanceCapability: upstreamstate.BalanceCapabilitySupported,
					BalanceSnapshot: &upstreamstate.BalanceSnapshot{
						Scope: "account_balance",
						Items: []upstreamstate.BalanceAmount{{Currency: "USD", Remaining: balanceState.remaining}},
					},
					BalanceObservedAt: &observedAt,
					WarningThresholds: thresholds,
					Availability:      availability,
					LastCheckStatus:   upstreamstate.CheckStatusSuccess,
					BlockReason:       balanceState.blockReason,
				}).Error; err != nil {
					t.Fatalf("create upstream state: %v", err)
				}
			}

			service := &llmGatewayServiceImpl{
				db:            db,
				upstreamState: upstreamstate.NewService(db, stubCryptoService{}),
			}
			healthy, warnings, err := service.evaluateCandidateRouteWarnings(context.Background(), organizationID, routes)
			if err != nil {
				t.Fatalf("evaluateCandidateRouteWarnings() error = %v", err)
			}
			if healthy != testCase.wantHealthy {
				t.Fatalf("healthy = %t, want %t", healthy, testCase.wantHealthy)
			}
			if testCase.wantHealthy {
				if len(warnings) != 0 {
					t.Fatalf("warnings = %#v, want none", warnings)
				}
				return
			}
			if len(warnings) != 1 {
				t.Fatalf("warnings = %#v, want one upstream low balance warning", warnings)
			}
			warning := warnings[0]
			if warning.Kind != AppModelRouteWarningKindPrivateChannelUpstreamBalanceLow || warning.Scope != testCase.wantScope {
				t.Fatalf("warning = %#v, want kind %q scope %q", warning, AppModelRouteWarningKindPrivateChannelUpstreamBalanceLow, testCase.wantScope)
			}
		})
	}
}

type precheckBalanceState struct {
	remaining   string
	threshold   string
	blockReason upstreamstate.GuardReason
	stale       bool
}
