package integrations

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisDailyQuotaEnforcesLimitAndIsolatesOrganizationsAndDays(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	now := time.Now().UTC()
	quota := NewRedisDailyQuota(client, 2)
	quota.now = func() time.Time { return now }
	ctx := context.Background()

	if err := quota.Acquire(ctx, "organization-1"); err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	if err := quota.Acquire(ctx, "organization-1"); err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}
	if err := quota.Acquire(ctx, "organization-1"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("third Acquire() error = %v, want ErrQuotaExceeded", err)
	}
	key := dailyQuotaTestKey("organization-1", now)
	if value, err := server.Get(key); err != nil || value != "3" {
		t.Fatalf("quota value = %q, error = %v, want 3", value, err)
	}

	if err := quota.Acquire(ctx, "organization-2"); err != nil {
		t.Fatalf("other organization Acquire() error = %v", err)
	}
	if value, err := server.Get(dailyQuotaTestKey("organization-2", now)); err != nil || value != "1" {
		t.Fatalf("other organization quota value = %q, error = %v, want 1", value, err)
	}

	now = now.Add(24 * time.Hour)
	if err := quota.Acquire(ctx, "organization-1"); err != nil {
		t.Fatalf("next day Acquire() error = %v", err)
	}
	if value, err := server.Get(dailyQuotaTestKey("organization-1", now)); err != nil || value != "1" {
		t.Fatalf("next day quota value = %q, error = %v, want 1", value, err)
	}
}

func TestRedisDailyQuotaFailsClosedWhenUnavailableOrInvalid(t *testing.T) {
	ctx := context.Background()
	if err := NewRedisDailyQuota(nil, 1).Acquire(ctx, "organization-1"); err == nil {
		t.Fatal("Acquire() with nil client error = nil")
	}

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	if err := NewRedisDailyQuota(client, 0).Acquire(ctx, "organization-1"); err == nil {
		t.Fatal("Acquire() with invalid limit error = nil")
	}
}

func dailyQuotaTestKey(organizationID string, now time.Time) string {
	return fmt.Sprintf("zgi:integration:web-search:daily:%s:%s", organizationID, now.UTC().Format("2006-01-02"))
}
