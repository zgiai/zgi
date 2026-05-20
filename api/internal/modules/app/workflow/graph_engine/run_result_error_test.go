package graph_engine

import (
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
)

func TestErrorFromNodeRunResult_PreservesBillingUserErrorChain(t *testing.T) {
	originalErr := errors.Join(
		errors.New("all providers failed"),
		&gateway.BillingUserError{
			Kind:  gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient,
			Cause: gateway.ErrInsufficientBalance,
		},
	)

	result := &shared.NodeRunResult{
		Status: shared.FAILED,
		Err:    originalErr,
		ErrMsg: "failed to invoke LLM: billing operation failed: private_channel_balance_insufficient",
	}

	err := errorFromNodeRunResult(result)

	var userErr *gateway.BillingUserError
	if !errors.As(err, &userErr) {
		t.Fatalf("errors.As(err, *BillingUserError) = false, err = %v", err)
	}
	if userErr.Kind != gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient {
		t.Fatalf("userErr.Kind = %q, want %q", userErr.Kind, gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient)
	}
}
