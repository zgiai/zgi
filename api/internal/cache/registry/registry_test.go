package registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeRefresher struct {
	prefix      string
	invalidated int
	refreshed   int
	err         error
	calls       *[]string
}

func (f *fakeRefresher) Prefix() string {
	return f.prefix
}

func (f *fakeRefresher) Invalidate(ctx context.Context, scope Scope) error {
	_ = ctx
	_ = scope
	f.invalidated++
	if f.calls != nil {
		*f.calls = append(*f.calls, "invalidate:"+f.prefix+":"+scope.Key())
	}
	return f.err
}

func (f *fakeRefresher) Refresh(ctx context.Context, scope Scope) error {
	_ = ctx
	_ = scope
	f.refreshed++
	if f.calls != nil {
		*f.calls = append(*f.calls, "refresh:"+f.prefix+":"+scope.Key())
	}
	return f.err
}

func TestRegisterRejectsInvalidPrefix(t *testing.T) {
	reg := New(Options{})

	tests := []struct {
		name      string
		refresher Refresher
		want      error
	}{
		{name: "nil", refresher: nil},
		{name: "empty", refresher: &fakeRefresher{prefix: "  "}, want: ErrModulePrefixEmpty},
		{name: "global prefix", refresher: &fakeRefresher{prefix: "zgi_cache.llm"}, want: ErrGlobalPrefixInName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reg.Register(tt.refresher)
			if err == nil {
				t.Fatal("Register() error = nil, want error")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("Register() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRegisterRejectsDuplicatePrefix(t *testing.T) {
	reg := New(Options{})

	if err := reg.Register(&fakeRefresher{prefix: "llm.models"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := reg.Register(&fakeRefresher{prefix: "llm:models"}); err == nil {
		t.Fatal("Register() duplicate error = nil, want error")
	}
}

func TestUnregisterAndReregister(t *testing.T) {
	reg := New(Options{})
	models := &fakeRefresher{prefix: "llm.models"}

	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !reg.Unregister("llm:models") {
		t.Fatal("Unregister() = false, want true")
	}
	if reg.Unregister("missing") {
		t.Fatal("Unregister() missing = true, want false")
	}
	if err := reg.Invalidate(context.Background(), "llm.models", Scope{}); !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("Invalidate() after unregister error = %v, want ErrModuleNotFound", err)
	}
	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() after unregister error = %v", err)
	}
}

func TestDestroyClearsRegistry(t *testing.T) {
	reg := New(Options{})
	if err := reg.Register(&fakeRefresher{prefix: "llm.models"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := reg.Register(&fakeRefresher{prefix: "billing"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if got := reg.Destroy(); got != 2 {
		t.Fatalf("Destroy() = %d, want 2", got)
	}
	if got := reg.Destroy(); got != 0 {
		t.Fatalf("second Destroy() = %d, want 0", got)
	}
	if prefixes := reg.Prefixes(); len(prefixes) != 0 {
		t.Fatalf("Prefixes() after destroy = %#v, want empty", prefixes)
	}
	if err := reg.RefreshInternal(context.Background(), "llm.models", Scope{}); !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("RefreshInternal() after destroy error = %v, want ErrModuleNotFound", err)
	}
}

func TestInvalidateMatchesModulePrefixAndChildren(t *testing.T) {
	reg := New(Options{})
	models := &fakeRefresher{prefix: "llm.models"}
	available := &fakeRefresher{prefix: "llm.models.available"}
	billing := &fakeRefresher{prefix: "billing"}

	for _, refresher := range []Refresher{billing, available, models} {
		if err := reg.Register(refresher); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	if err := reg.Invalidate(context.Background(), "llm.models", Scope{}); err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}

	if models.invalidated != 1 {
		t.Fatalf("models invalidated = %d, want 1", models.invalidated)
	}
	if available.invalidated != 1 {
		t.Fatalf("available invalidated = %d, want 1", available.invalidated)
	}
	if billing.invalidated != 0 {
		t.Fatalf("billing invalidated = %d, want 0", billing.invalidated)
	}
}

func TestModulePrefixSelectionAndOrder(t *testing.T) {
	reg := New(Options{})
	var calls []string
	refreshers := []Refresher{
		&fakeRefresher{prefix: "llmish", calls: &calls},
		&fakeRefresher{prefix: "llm.models.available", calls: &calls},
		&fakeRefresher{prefix: "billing.llm", calls: &calls},
		&fakeRefresher{prefix: "llm", calls: &calls},
		&fakeRefresher{prefix: "llm.models", calls: &calls},
	}
	for _, refresher := range refreshers {
		if err := reg.Register(refresher); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	if err := reg.RefreshInternal(context.Background(), "llm", Scope{}); err != nil {
		t.Fatalf("RefreshInternal() error = %v", err)
	}

	want := []string{
		"refresh:llm:global",
		"refresh:llm.models:global",
		"refresh:llm.models.available:global",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestRefreshAllIsRateLimited(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	reg := New(Options{
		GlobalRefreshInterval: time.Minute,
		RateLimiter:           limiter,
	})
	models := &fakeRefresher{prefix: "llm.models"}
	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := reg.RefreshAll(context.Background(), Scope{}, "tester"); err != nil {
		t.Fatalf("first RefreshAll() error = %v", err)
	}
	if err := reg.RefreshAll(context.Background(), Scope{}, "tester"); err == nil {
		t.Fatal("second RefreshAll() error = nil, want rate limit error")
	} else {
		var rateErr *RateLimitedError
		if !errors.As(err, &rateErr) {
			t.Fatalf("second RefreshAll() error = %T, want *RateLimitedError", err)
		}
	}
	if models.refreshed != 1 {
		t.Fatalf("refreshed = %d, want 1", models.refreshed)
	}
}

func TestRefreshRejectsEmptyModulePrefix(t *testing.T) {
	reg := New(Options{})
	if err := reg.Refresh(context.Background(), "", Scope{}, "tester"); !errors.Is(err, ErrModulePrefixEmpty) {
		t.Fatalf("Refresh() error = %v, want ErrModulePrefixEmpty", err)
	}
}

func TestRefreshUnknownModuleDoesNotConsumeLimit(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	reg := New(Options{
		ModuleRefreshInterval: time.Minute,
		RateLimiter:           limiter,
	})
	models := &fakeRefresher{prefix: "llm.models"}

	if err := reg.Refresh(context.Background(), "llm.models", Scope{}, "actor-a"); !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("Refresh() missing error = %v, want ErrModuleNotFound", err)
	}
	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := reg.Refresh(context.Background(), "llm.models", Scope{}, "actor-a"); err != nil {
		t.Fatalf("Refresh() after register error = %v", err)
	}
}

func TestRefreshLimitIsPerModuleScopeNotActor(t *testing.T) {
	reg := New(Options{
		ModuleRefreshInterval: time.Minute,
		RateLimiter:           NewMemoryRateLimiter(),
	})
	models := &fakeRefresher{prefix: "llm.models"}
	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := reg.Refresh(context.Background(), "llm.models", Scope{}, "actor-a"); err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}
	if err := reg.Refresh(context.Background(), "llm.models", Scope{}, "actor-b"); err == nil {
		t.Fatal("second Refresh() by different actor error = nil, want rate limit")
	}
}

func TestRefreshLimitIsIsolatedByScope(t *testing.T) {
	reg := New(Options{
		ModuleRefreshInterval: time.Minute,
		RateLimiter:           NewMemoryRateLimiter(),
	})
	models := &fakeRefresher{prefix: "llm.models"}
	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	orgA := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	orgB := uuid.MustParse("00000000-0000-0000-0000-00000000000b")

	if err := reg.Refresh(context.Background(), "llm.models", Scope{OrganizationID: &orgA}, "actor-a"); err != nil {
		t.Fatalf("Refresh() org A error = %v", err)
	}
	if err := reg.Refresh(context.Background(), "llm.models", Scope{OrganizationID: &orgB}, "actor-a"); err != nil {
		t.Fatalf("Refresh() org B error = %v", err)
	}
}

func TestInternalRefreshBypassesLimit(t *testing.T) {
	reg := New(Options{
		ModuleRefreshInterval: time.Minute,
		GlobalRefreshInterval: time.Minute,
		RateLimiter:           NewMemoryRateLimiter(),
	})
	models := &fakeRefresher{prefix: "llm.models"}
	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := reg.RefreshInternal(context.Background(), "llm.models", Scope{}); err != nil {
		t.Fatalf("first RefreshInternal() error = %v", err)
	}
	if err := reg.RefreshInternal(context.Background(), "llm.models", Scope{}); err != nil {
		t.Fatalf("second RefreshInternal() error = %v", err)
	}
	if err := reg.RefreshAllInternal(context.Background(), Scope{}); err != nil {
		t.Fatalf("first RefreshAllInternal() error = %v", err)
	}
	if err := reg.RefreshAllInternal(context.Background(), Scope{}); err != nil {
		t.Fatalf("second RefreshAllInternal() error = %v", err)
	}
	if models.refreshed != 4 {
		t.Fatalf("refreshed = %d, want 4", models.refreshed)
	}
}

func TestInvalidateIsNotRateLimited(t *testing.T) {
	reg := New(Options{
		ModuleRefreshInterval: time.Minute,
		RateLimiter:           NewMemoryRateLimiter(),
	})
	models := &fakeRefresher{prefix: "llm.models"}
	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := reg.Invalidate(context.Background(), "llm.models", Scope{}); err != nil {
		t.Fatalf("first Invalidate() error = %v", err)
	}
	if err := reg.Invalidate(context.Background(), "llm.models", Scope{}); err != nil {
		t.Fatalf("second Invalidate() error = %v", err)
	}
	if models.invalidated != 2 {
		t.Fatalf("invalidated = %d, want 2", models.invalidated)
	}
}

func TestBatchRefreshUsesEveryScope(t *testing.T) {
	reg := New(Options{})
	var calls []string
	models := &fakeRefresher{prefix: "llm.models", calls: &calls}
	if err := reg.Register(models); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	orgA := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	orgB := uuid.MustParse("00000000-0000-0000-0000-00000000000b")

	if err := reg.RefreshInternalBatch(context.Background(), "llm.models", []Scope{
		{OrganizationID: &orgA},
		{OrganizationID: &orgB},
	}); err != nil {
		t.Fatalf("RefreshInternalBatch() error = %v", err)
	}

	want := []string{
		"refresh:llm.models:org:00000000-0000-0000-0000-00000000000a",
		"refresh:llm.models:org:00000000-0000-0000-0000-00000000000b",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestRunJoinsRefresherErrors(t *testing.T) {
	reg := New(Options{})
	errA := errors.New("error a")
	errB := errors.New("error b")
	if err := reg.Register(&fakeRefresher{prefix: "llm.models", err: errA}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := reg.Register(&fakeRefresher{prefix: "llm.models.available", err: errB}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err := reg.RefreshInternal(context.Background(), "llm.models", Scope{})
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("RefreshInternal() error = %v, want joined error a and b", err)
	}
}

func TestCanceledContextStopsLaterRefreshers(t *testing.T) {
	reg := New(Options{})
	var calls []string
	cancelErr := fmt.Errorf("first cancels context")
	ctx, cancel := context.WithCancel(context.Background())
	second := &fakeRefresher{prefix: "llm.models.available", calls: &calls}
	if err := reg.Register(&cancelingRefresher{fakeRefresher: fakeRefresher{prefix: "llm.models", calls: &calls}, cancel: cancel, err: cancelErr}); err != nil {
		t.Fatalf("Register() first error = %v", err)
	}
	if err := reg.Register(second); err != nil {
		t.Fatalf("Register() second error = %v", err)
	}

	err := reg.RefreshInternal(ctx, "llm.models", Scope{})
	if !errors.Is(err, cancelErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("RefreshInternal() error = %v, want cancelErr and context.Canceled", err)
	}
	want := []string{"refresh:llm.models:global"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

type cancelingRefresher struct {
	fakeRefresher
	cancel context.CancelFunc
	err    error
}

func (c cancelingRefresher) Refresh(ctx context.Context, scope Scope) error {
	_ = ctx
	if c.calls != nil {
		*c.calls = append(*c.calls, "refresh:"+c.prefix+":"+scope.Key())
	}
	c.cancel()
	return c.err
}

func TestPrefixesAreSorted(t *testing.T) {
	reg := New(Options{})
	for _, prefix := range []string{"llm.models.available", "billing", "llm.models"} {
		if err := reg.Register(&fakeRefresher{prefix: prefix}); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	got := reg.Prefixes()
	want := []string{"billing", "llm.models", "llm.models.available"}
	if !slices.Equal(got, want) {
		t.Fatalf("Prefixes() = %#v, want %#v", got, want)
	}
}
