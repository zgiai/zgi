package storage

import (
	"testing"
	"time"

	runtime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
)

func TestAgentRuntimeSuspensionRetrySelection(t *testing.T) {
	dsn := testutil.CreateTestPostgresDSN(t)
	store, err := Open(config.Config{DatabaseURL: dsn, MaxWorkers: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, id := range []string{"failed-oldest", "healthy"} {
		binding := admissionBinding(id, "org")
		binding.ExpiresAt = now.Add(-time.Hour)
		if id == "healthy" {
			binding.ExpiresAt = now.Add(-time.Minute)
		}
		binding, _, err = store.ReserveBinding(t.Context(), binding)
		if err != nil {
			t.Fatal(err)
		}
		binding.Generation++
		binding.Status = runtime.BindingStatusActive
		if id == "failed-oldest" {
			binding.Status = runtime.BindingStatusSuspending
		}
		binding.UpdatedAt = now
		if err := store.UpdateBinding(t.Context(), binding, binding.Generation-1); err != nil {
			t.Fatal(err)
		}
	}
	// Reopen the pool, as after a process restart. Retry state lives in the DB,
	// and filtering must happen before LIMIT 1, not after selecting the oldest.
	restarted, err := Open(config.Config{DatabaseURL: dsn, MaxWorkers: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	for _, test := range []struct {
		at   time.Time
		want string
	}{
		{now, "healthy"},
		{now.Add(runtime.IdleSuspendRetryDelay - time.Microsecond), "healthy"},
		{now.Add(runtime.IdleSuspendRetryDelay), "failed-oldest"},
	} {
		bindings, err := restarted.ExpiredBindings(t.Context(), test.at, 1)
		if err != nil || len(bindings) != 1 || bindings[0].ID != test.want {
			t.Fatalf("at %s bindings=%+v error=%v, want %s", test.at, bindings, err, test.want)
		}
	}
}
