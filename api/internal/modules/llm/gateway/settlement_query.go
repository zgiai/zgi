package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type settlementQueryResponse struct {
	Found                 bool            `json:"found"`
	SettlementID          string          `json:"settlement_id"`
	DeductionID           string          `json:"deduction_id"`
	AttemptID             string          `json:"attempt_id"`
	OrganizationID        string          `json:"organization_id"`
	SettlementStatus      string          `json:"settlement_status"`
	ReservedCredits       int64           `json:"reserved_credits"`
	SettledCredits        int64           `json:"settled_credits"`
	RefundedCredits       int64           `json:"refunded_credits"`
	ActualQuantity        int64           `json:"actual_quantity"`
	EstimatedQuantity     int64           `json:"estimated_quantity"`
	Operation             string          `json:"operation"`
	Meter                 string          `json:"meter"`
	BaseUnit              string          `json:"base_unit"`
	PricingSnapshot       json.RawMessage `json:"pricing_snapshot"`
	RequestFingerprint    string          `json:"request_fingerprint"`
	SettlementFingerprint string          `json:"settlement_fingerprint"`
	RequestID             string          `json:"request_id"`
	FailureMessage        string          `json:"failure_message"`
	Retryable             bool            `json:"retryable"`
	PromptTokens          int             `json:"prompt_tokens"`
	CompletionTokens      int             `json:"completion_tokens"`
	TotalTokens           int             `json:"total_tokens"`
	CacheReadTokens       int             `json:"cache_read_tokens"`
	CacheWriteTokens      int             `json:"cache_write_tokens"`
	CacheWrite5mTokens    int             `json:"cache_write_5m_tokens"`
	CacheWrite1hTokens    int             `json:"cache_write_1h_tokens"`
	ReasoningOutputTokens int             `json:"reasoning_output_tokens"`
	AudioInputTokens      int             `json:"audio_input_tokens"`
	AudioOutputTokens     int             `json:"audio_output_tokens"`
}

// queryConsoleSettlement uses the same internal API base URL as official model
// forwarding, so model and settlement traffic cannot drift to different hosts.
const (
	settlementQueryInitialBackoff = 100 * time.Millisecond
	settlementQueryMaxBackoff     = 2 * time.Second
	settlementQueryDeadline       = 8 * time.Second
)

func consoleSettlementInFlight(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PENDING", "SETTLING":
		return true
	default:
		return false
	}
}

func waitForConsoleSettlement(ctx context.Context, queryURL, organizationID, attemptID string) (*settlementQueryResponse, error) {
	deadline := time.Now().Add(settlementQueryDeadline)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	backoff := settlementQueryInitialBackoff
	var result *settlementQueryResponse
	var err error
	for {
		result, err = queryConsoleSettlement(ctx, queryURL, organizationID, attemptID)
		if err == nil && result != nil && result.Found {
			status := result.SettlementStatus
			if strings.EqualFold(status, "SETTLED") {
				return result, nil
			}
			if strings.EqualFold(status, "FAILED_FINAL") || !result.Retryable || !consoleSettlementInFlight(status) {
				return result, nil
			}
		} else if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !time.Now().Add(backoff).Before(deadline) {
			if err != nil {
				return nil, err
			}
			if result == nil {
				return &settlementQueryResponse{}, nil
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < settlementQueryMaxBackoff {
			backoff *= 2
			if backoff > settlementQueryMaxBackoff {
				backoff = settlementQueryMaxBackoff
			}
		}
	}
}

func queryConsoleSettlement(ctx context.Context, base, organizationID, attemptID string) (*settlementQueryResponse, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil, fmt.Errorf("console internal API base URL is not configured")
	}
	u, err := url.Parse(base + "/" + url.PathEscape(attemptID))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("organization_id", organizationID)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if secret := strings.TrimSpace(os.Getenv("CONSOLE_INTERNAL_API_KEY")); secret != "" {
		req.Header.Set("X-ZGI-Internal-Secret", secret)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("console settlement query returned %s", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("console settlement query returned %s", resp.Status)
	}
	var result settlementQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
