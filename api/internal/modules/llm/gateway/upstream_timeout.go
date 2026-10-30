package gateway

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/requesttimeout"
)

const (
	minUpstreamTimeoutSeconds    = 15
	defaultPrivateAdapterTimeout = 500 * time.Second
	// officialAdapterMargin keeps the hop to Console open slightly longer than
	// Console's own budget for the same request, so Console's timeout answer
	// (a cataloged 504) arrives before pre gives up on the connection.
	officialAdapterMargin      = 30 * time.Second
	upstreamTimeoutOverrideKey = "upstream_timeout_seconds"
)

// adapterTimeoutFromSelection is the HTTP budget of one adapter call.
//
// Official (Console platform proxy): the request's X-ZGI-Request-Timeout, or
// the default, plus a margin. Console bounds the provider call itself, and its
// channel override governs there, so a pre-side override is ignored.
//
// Private (direct provider): header > channel override > 500s default, bounded
// by the request maximum instead of a fixed 600s.
func adapterTimeoutFromSelection(ctx context.Context, selection *ProviderSelection) time.Duration {
	if selection != nil && selection.UseSystemProvider {
		return requesttimeout.Effective(ctx) + officialAdapterMargin
	}
	if timeout, ok := requesttimeout.Requested(ctx); ok {
		return timeout
	}
	timeout := defaultPrivateAdapterTimeout
	if selection != nil && selection.ParamOverride != nil {
		if raw, ok := selection.ParamOverride[upstreamTimeoutOverrideKey]; ok {
			if seconds := parseTimeoutSeconds(raw); seconds > 0 {
				timeout = time.Duration(seconds) * time.Second
			}
		}
	}
	return clampUpstreamTimeout(timeout)
}

func clampUpstreamTimeout(timeout time.Duration) time.Duration {
	minTimeout := time.Duration(minUpstreamTimeoutSeconds) * time.Second
	if timeout < minTimeout {
		return minTimeout
	}
	if limit := requesttimeout.Max(); timeout > limit {
		return limit
	}
	return timeout
}

func parseTimeoutSeconds(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case string:
		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0
		}
		return seconds
	case json.Number:
		seconds, err := value.Int64()
		if err != nil {
			parsed, ferr := value.Float64()
			if ferr != nil {
				return 0
			}
			return int(parsed)
		}
		return int(seconds)
	default:
		return 0
	}
}
