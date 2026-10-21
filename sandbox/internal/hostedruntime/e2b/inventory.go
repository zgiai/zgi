package e2b

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// Inventory never connects/resumes a sandbox. Unattributed ZGI metadata may
// belong to another database sharing this account and is never auto-deleted.
func (b *Backend) Inventory(ctx context.Context) ([]agentexecution.ResourceObservation, error) {
	// Never forward provider credentials to a redirect target, including a
	// same-account gateway misconfiguration. Keep the configured TLS transport.
	client := &http.Client{Transport: b.client.httpClient.Transport, Timeout: b.client.httpClient.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var result []agentexecution.ResourceObservation
	seen := make(map[string]bool)
	for _, state := range []string{"running", "paused"} {
		nextToken := ""
		seenTokens := make(map[string]bool)
		for page := 0; ; page++ {
			if page == 10 {
				return nil, fmt.Errorf("E2B audit exceeds pagination limit")
			}
			query := url.Values{"state": {state}, "limit": {"100"}}
			if nextToken != "" {
				query.Set("nextToken", nextToken)
			}
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.client.apiURL+"/v2/sandboxes?"+query.Encode(), nil)
			if err != nil {
				return nil, err
			}
			request.Header.Set("X-API-KEY", b.client.apiKey)
			response, err := client.Do(request)
			if err != nil {
				return nil, fmt.Errorf("%w: E2B audit request failed", agentexecution.ErrBackendUnavailable)
			}
			if response.StatusCode != http.StatusOK {
				_ = response.Body.Close()
				return nil, providerStatusError(response.StatusCode)
			}
			var items []struct {
				ID       string            `json:"sandboxID"`
				State    string            `json:"state"`
				Metadata map[string]string `json:"metadata"`
			}
			err = decodeLimitedJSON(response.Body, &items)
			next := strings.TrimSpace(response.Header.Get("X-Next-Token"))
			_ = response.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("%w: invalid E2B audit response", agentexecution.ErrBackendUnavailable)
			}
			for _, item := range items {
				if item.Metadata["zgi.binding_id"] == "" {
					continue
				}
				if item.ID == "" || (item.State != "running" && item.State != "paused") {
					return nil, fmt.Errorf("%w: incomplete E2B audit resource", agentexecution.ErrBackendUnavailable)
				}
				if !seen[item.ID] {
					seen[item.ID] = true
					result = append(result, agentexecution.ResourceObservation{Ref: item.ID, BindingID: item.Metadata["zgi.binding_id"], State: item.State})
				}
			}
			if next == "" {
				break
			}
			if seenTokens[next] {
				return nil, fmt.Errorf("E2B audit pagination did not advance")
			}
			seenTokens[next] = true
			nextToken = next
		}
	}
	return result, nil
}
