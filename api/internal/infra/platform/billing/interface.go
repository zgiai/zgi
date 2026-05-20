package billing

import (
	"context"
)

// Usage represents the consumption details of an LLM call.
type Usage struct {
	Model            string
	Provider         string
	PromptTokens     int
	CompletionTokens int
	RequestID        string
	Metadata         map[string]string
}

// PreDeductRequest represents a request to pre-deduct quota before LLM call.
type PreDeductRequest struct {
	OrganizationID   string
	EstimatedCredits int64
	Model            string
	Provider         string
	RequestID        string
}

// PreDeductResponse represents the response from pre-deduct operation.
type PreDeductResponse struct {
	Allowed        bool
	DeductionID    string
	RemainingQuota int64
	Reason         string
}

// SettleRequest represents a request to settle quota after LLM call.
type SettleRequest struct {
	OrganizationID   string
	DeductionID      string
	EstimatedCredits int64
	ActualCredits    int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	Model            string
	Provider         string
	Status           string
	ErrorMessage     string
	RequestID        string
}

// BillingProvider defines the interface for credit management and validation.
type BillingProvider interface {
	// PreCheck validates if the request should proceed (balance, quota, etc.)
	// This is a simple check without locking quota.
	PreCheck(ctx context.Context, tenantID, model, provider string) (allowed bool, reason string, err error)

	// RecordUsage reports the final consumption.
	// This is a fire-and-forget recording without refund capability.
	RecordUsage(ctx context.Context, tenantID string, usage Usage) error

	// PreDeduct pre-deducts estimated quota before LLM call.
	// Returns a deduction_id for later settlement.
	// This locks the quota to prevent over-consumption.
	PreDeduct(ctx context.Context, req *PreDeductRequest) (*PreDeductResponse, error)

	// Settle settles the actual quota consumption after LLM call.
	// Refunds the difference between estimated and actual credits.
	Settle(ctx context.Context, req *SettleRequest) error
}
