package gateway

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const (
	minUpstreamTimeoutSeconds     = 15
	maxUpstreamTimeoutSeconds     = 600
	defaultPrivateAdapterTimeout  = 500 * time.Second
	defaultOfficialAdapterTimeout = 120 * time.Second
	upstreamTimeoutOverrideKey    = "upstream_timeout_seconds"
)

func adapterTimeoutFromSelection(selection *ProviderSelection) time.Duration {
	if selection != nil && selection.UseSystemProvider {
		return defaultOfficialAdapterTimeout
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
	maxTimeout := time.Duration(maxUpstreamTimeoutSeconds) * time.Second
	if timeout < minTimeout {
		return minTimeout
	}
	if timeout > maxTimeout {
		return maxTimeout
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
