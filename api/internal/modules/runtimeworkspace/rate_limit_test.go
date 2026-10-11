package runtimeworkspace

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRuntimeRequestsPerMinuteSupportsFlatAndNestedPolicies(t *testing.T) {
	if got := RuntimeRequestsPerMinute(`{"requests_per_minute": 12}`); got != 12 {
		t.Fatalf("flat rate limit = %d, want 12", got)
	}
	if got := RuntimeRequestsPerMinute(`{"rate_limit":{"requests_per_minute":7}}`); got != 7 {
		t.Fatalf("nested rate limit = %d, want 7", got)
	}
	if got := RuntimeRequestsPerMinute(`{"requests_per_minute":-1}`); got != 0 {
		t.Fatalf("invalid rate limit = %d, want disabled", got)
	}
}

func TestAllowRateLimitedRequestUsesRedisFixedWindow(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	now := time.Date(2026, 9, 22, 10, 5, 20, 0, time.UTC)
	for i := 0; i < 2; i++ {
		decision, err := AllowRateLimitedRequest(t.Context(), client, "credential-1", 2, now)
		if err != nil || !decision.Allowed || decision.Remaining != 1-i {
			t.Fatalf("request %d decision = %#v, err = %v", i+1, decision, err)
		}
	}
	decision, err := AllowRateLimitedRequest(t.Context(), client, "credential-1", 2, now)
	if err != ErrRateLimited || decision.Allowed || decision.Remaining != 0 || decision.RetryAfter <= 0 {
		t.Fatalf("limited decision = %#v, err = %v", decision, err)
	}
	allowedNextWindow, err := AllowRateLimitedRequest(t.Context(), client, "credential-1", 2, now.Add(time.Minute))
	if err != nil || !allowedNextWindow.Allowed {
		t.Fatalf("next window decision = %#v, err = %v", allowedNextWindow, err)
	}
}
