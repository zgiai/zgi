package gateway

import (
	"context"
	"testing"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

func TestEvaluateCandidateRouteWarningsReportsKnownUnavailableCredentials(t *testing.T) {
	tests := []struct {
		name        string
		reasons     []upstreamstate.GuardReason
		wantHealthy bool
		wantReason  string
		wantScope   AppModelRouteWarningScope
	}{
		{
			name:       "one unavailable credential is a partial warning",
			reasons:    []upstreamstate.GuardReason{upstreamstate.GuardReasonBalanceExhausted, ""},
			wantReason: string(upstreamstate.GuardReasonBalanceExhausted),
			wantScope:  AppModelRouteWarningScopePartial,
		},
		{
			name:       "all unavailable credentials are an all warning",
			reasons:    []upstreamstate.GuardReason{upstreamstate.GuardReasonQuotaExhausted, upstreamstate.GuardReasonQuotaExhausted},
			wantReason: string(upstreamstate.GuardReasonQuotaExhausted),
			wantScope:  AppModelRouteWarningScopeAll,
		},
		{
			name:       "mixed unavailable reasons use the generic reason",
			reasons:    []upstreamstate.GuardReason{upstreamstate.GuardReasonBillingUnavailable, upstreamstate.GuardReasonAuthInvalid},
			wantReason: "credential_unavailable",
			wantScope:  AppModelRouteWarningScopeAll,
		},
		{
			name:        "healthy credentials do not warn",
			reasons:     []upstreamstate.GuardReason{"", ""},
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
			routes := make([]*channelmodel.LLMRoute, 0, len(testCase.reasons))
			for _, reason := range testCase.reasons {
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

				availability := upstreamstate.AvailabilityAvailable
				if reason == upstreamstate.GuardReasonAuthInvalid {
					availability = upstreamstate.AvailabilityInvalidKey
				} else if reason != "" {
					availability = upstreamstate.AvailabilityExhausted
				}
				if err := db.Create(&upstreamstate.State{
					CredentialID:      credentialID,
					OrganizationID:    organizationID,
					Generation:        1,
					BalanceCapability: upstreamstate.BalanceCapabilityUnsupported,
					Availability:      availability,
					LastCheckStatus:   upstreamstate.CheckStatusUnsupported,
					WarningThresholds: []upstreamstate.WarningThreshold{},
					BlockReason:       reason,
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
				t.Fatalf("warnings = %#v, want one upstream unavailable warning", warnings)
			}
			warning := warnings[0]
			if warning.Kind != AppModelRouteWarningKindPrivateChannelUpstreamUnavailable || warning.Reason != testCase.wantReason || warning.Scope != testCase.wantScope {
				t.Fatalf("warning = %#v, want kind %q reason %q scope %q", warning, AppModelRouteWarningKindPrivateChannelUpstreamUnavailable, testCase.wantReason, testCase.wantScope)
			}
		})
	}
}
