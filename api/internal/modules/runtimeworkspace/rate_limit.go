package runtimeworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrRateLimited        = errors.New("runtime workspace rate limit exceeded")
	ErrEndUserRateLimited = errors.New("runtime workspace end-user rate limit exceeded")
)

// RuntimeRequestsPerMinute reads the optional per-exposure-group policy. A
// zero value deliberately disables the limiter so existing Runtime Workspaces
// preserve their current behavior until an administrator opts in.
func RuntimeRequestsPerMinute(policyJSON string) int {
	var policy struct {
		RequestsPerMinute int `json:"requests_per_minute"`
		RateLimit         *struct {
			RequestsPerMinute int `json:"requests_per_minute"`
		} `json:"rate_limit"`
	}
	if json.Unmarshal([]byte(policyJSON), &policy) != nil {
		return 0
	}
	if policy.RateLimit != nil && policy.RateLimit.RequestsPerMinute > 0 {
		return policy.RateLimit.RequestsPerMinute
	}
	if policy.RequestsPerMinute > 0 {
		return policy.RequestsPerMinute
	}
	return 0
}

// RuntimeEndUserRequestsPerMinute reads the optional limit on each end user
// (ADR 0012); zero disables it.
func RuntimeEndUserRequestsPerMinute(policyJSON string) int {
	var policy struct {
		RateLimit *struct {
			PerEndUserRequestsPerMinute int `json:"per_end_user_requests_per_minute"`
		} `json:"rate_limit"`
	}
	if json.Unmarshal([]byte(policyJSON), &policy) != nil || policy.RateLimit == nil || policy.RateLimit.PerEndUserRequestsPerMinute < 0 {
		return 0
	}
	return policy.RateLimit.PerEndUserRequestsPerMinute
}

type RateLimitDecision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// AllowRateLimitedRequest applies a small fixed-window limiter using the
// existing Redis deployment. It is intentionally fail-open when Redis is not
// configured or temporarily unavailable; authentication and resource scope
// remain mandatory and the caller can alert on the returned error.
func AllowRateLimitedRequest(ctx context.Context, client *redis.Client, key string, limit int, now time.Time) (RateLimitDecision, error) {
	decision := RateLimitDecision{Allowed: true, Limit: limit, Remaining: limit}
	if client == nil || limit <= 0 {
		return decision, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return decision, errors.New("runtime workspace rate-limit key is empty")
	}
	if now.IsZero() {
		now = time.Now()
	}
	window := now.UTC().Unix() / 60
	redisKey := fmt.Sprintf("zgi:runtime-workspace:rate:%s:%d", key, window)
	count, err := client.Incr(ctx, redisKey).Result()
	if err != nil {
		return decision, err
	}
	if count == 1 {
		// Keep the key around for the current minute and a small grace period so
		// an expired key cannot be resurrected by a late request.
		if err := client.Expire(ctx, redisKey, 61*time.Second).Err(); err != nil {
			return decision, err
		}
	}
	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	decision.Remaining = remaining
	if count <= int64(limit) {
		return decision, nil
	}
	decision.Allowed = false
	decision.RetryAfter = time.Duration(60-now.UTC().Second()) * time.Second
	if decision.RetryAfter <= 0 {
		decision.RetryAfter = time.Second
	}
	return decision, ErrRateLimited
}
