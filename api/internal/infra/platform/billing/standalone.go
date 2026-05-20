package billing

import (
	"context"
)

// StandaloneBilling is the default implementation for self-hosted/open-source users.
// it always allows requests and does no recording, essentially disabling billing limits.
type StandaloneBilling struct{}

func NewStandaloneBilling() *StandaloneBilling {
	return &StandaloneBilling{}
}

func (s *StandaloneBilling) PreCheck(ctx context.Context, organizationID, model, provider string) (bool, string, error) {
	// Open source version allows everything by default.
	return true, "standalone mode allowed", nil
}

func (s *StandaloneBilling) RecordUsage(ctx context.Context, organizationID string, usage Usage) error {
	// No-op for standalone version.
	return nil
}

// PreDeduct always allows requests in standalone mode.
func (s *StandaloneBilling) PreDeduct(ctx context.Context, req *PreDeductRequest) (*PreDeductResponse, error) {
	// Open source version: always allow, use request_id as deduction_id
	return &PreDeductResponse{
		Allowed:        true,
		DeductionID:    req.RequestID,
		RemainingQuota: -1, // -1 indicates unlimited quota
		Reason:         "standalone mode - unlimited quota",
	}, nil
}

// Settle is a no-op in standalone mode.
func (s *StandaloneBilling) Settle(ctx context.Context, req *SettleRequest) error {
	// No-op for standalone version.
	return nil
}
