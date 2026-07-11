package client

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
)

func TestConvertGatewayPrecheckResultPreservesUpstreamUnavailableReason(t *testing.T) {
	result := convertGatewayPrecheckResult(&gateway.AppModelRoutePrecheckResult{
		Status: gateway.AppModelRoutePrecheckStatusWarning,
		Warnings: []gateway.AppModelRouteWarning{{
			Kind:   gateway.AppModelRouteWarningKindPrivateChannelUpstreamUnavailable,
			Reason: "credential_unavailable",
		}},
	})
	if result.Status != AppModelPrecheckStatusWarning || len(result.Warnings) != 1 {
		t.Fatalf("result = %#v, want one warning", result)
	}
	warning := result.Warnings[0]
	if warning.Kind != AppModelPrecheckWarningPrivateChannelUpstreamUnavailable || warning.Reason != "credential_unavailable" {
		t.Fatalf("warning = %#v, want upstream unavailable reason", warning)
	}
}
