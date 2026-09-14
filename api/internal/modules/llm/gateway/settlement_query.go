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
}

// queryConsoleSettlement uses the same internal API base URL as official model
// forwarding, so model and settlement traffic cannot drift to different hosts.
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
