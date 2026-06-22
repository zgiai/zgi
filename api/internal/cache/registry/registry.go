package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/cache/keys"
)

type Options struct {
	ModuleRefreshInterval time.Duration
	GlobalRefreshInterval time.Duration
	RateLimiter           RateLimiter
}

type Registry struct {
	mu     sync.RWMutex
	items  map[string]Refresher
	limit  RateLimiter
	module time.Duration
	global time.Duration
}

// New creates a module cache registry. Refresh and RefreshAll are intended for
// manually triggered refreshes and apply rate limits when a limiter is present.
// Internal callers can use RefreshInternal and RefreshAllInternal to bypass
// manual refresh limits.
func New(opts Options) *Registry {
	moduleInterval := opts.ModuleRefreshInterval
	if moduleInterval == 0 {
		moduleInterval = DefaultModuleRefreshInterval
	}
	globalInterval := opts.GlobalRefreshInterval
	if globalInterval == 0 {
		globalInterval = DefaultGlobalRefreshInterval
	}

	return &Registry{
		items:  make(map[string]Refresher),
		limit:  opts.RateLimiter,
		module: moduleInterval,
		global: globalInterval,
	}
}

func (r *Registry) Register(refresher Refresher) error {
	if refresher == nil {
		return errors.New("cache refresher is nil")
	}
	prefix, err := parseModulePrefix(refresher.Prefix())
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[prefix]; ok {
		return fmt.Errorf("cache refresher prefix %q already registered", prefix)
	}
	r.items[prefix] = refresher
	return nil
}

func (r *Registry) Unregister(modulePrefix string) bool {
	prefix, err := parseModulePrefix(modulePrefix)
	if err != nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[prefix]; !ok {
		return false
	}
	delete(r.items, prefix)
	return true
}

func (r *Registry) Destroy() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := len(r.items)
	r.items = make(map[string]Refresher)
	return count
}

func (r *Registry) Prefixes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefixes := make([]string, 0, len(r.items))
	for prefix := range r.items {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	return prefixes
}

func (r *Registry) Invalidate(ctx context.Context, modulePrefix string, scope Scope) error {
	return r.InvalidateBatch(ctx, modulePrefix, []Scope{scope})
}

func (r *Registry) InvalidateBatch(ctx context.Context, modulePrefix string, scopes []Scope) error {
	refs, err := r.matchForModule(modulePrefix)
	if err != nil {
		return err
	}
	return r.runBatch(ctx, refs, normalizeScopes(scopes), func(refresher Refresher, scope Scope) error {
		return refresher.Invalidate(ctx, scope)
	})
}

func (r *Registry) InvalidateAll(ctx context.Context, scope Scope) error {
	return r.InvalidateAllBatch(ctx, []Scope{scope})
}

func (r *Registry) InvalidateAllBatch(ctx context.Context, scopes []Scope) error {
	return r.runBatch(ctx, r.matchAll(), normalizeScopes(scopes), func(refresher Refresher, scope Scope) error {
		return refresher.Invalidate(ctx, scope)
	})
}

func (r *Registry) Refresh(ctx context.Context, modulePrefix string, scope Scope, actorID string) error {
	return r.RefreshBatch(ctx, modulePrefix, []Scope{scope}, actorID)
}

func (r *Registry) RefreshBatch(ctx context.Context, modulePrefix string, scopes []Scope, actorID string) error {
	_ = actorID
	prefix, err := parseModulePrefix(modulePrefix)
	if err != nil {
		return err
	}
	refs, err := r.matchRequired(prefix)
	if err != nil {
		return err
	}

	scopes = normalizeScopes(scopes)
	return r.runBatch(ctx, refs, scopes, func(refresher Refresher, scope Scope) error {
		refPrefix, err := parseModulePrefix(refresher.Prefix())
		if err != nil {
			return err
		}
		if err := r.checkLimit(ctx, "module:"+refPrefix+":"+scope.Key(), r.module); err != nil {
			return err
		}
		return refresher.Refresh(ctx, scope)
	})
}

func (r *Registry) RefreshInternal(ctx context.Context, modulePrefix string, scope Scope) error {
	return r.RefreshInternalBatch(ctx, modulePrefix, []Scope{scope})
}

func (r *Registry) RefreshInternalBatch(ctx context.Context, modulePrefix string, scopes []Scope) error {
	refs, err := r.matchForModule(modulePrefix)
	if err != nil {
		return err
	}
	return r.runBatch(ctx, refs, normalizeScopes(scopes), func(refresher Refresher, scope Scope) error {
		return refresher.Refresh(ctx, scope)
	})
}

func (r *Registry) RefreshAll(ctx context.Context, scope Scope, actorID string) error {
	return r.RefreshAllBatch(ctx, []Scope{scope}, actorID)
}

func (r *Registry) RefreshAllBatch(ctx context.Context, scopes []Scope, actorID string) error {
	_ = actorID
	scopes = normalizeScopes(scopes)
	refs := r.matchAll()
	var errs []error
	for _, scope := range scopes {
		if err := r.checkLimit(ctx, "global:"+scope.Key(), r.global); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := r.runBatch(ctx, refs, []Scope{scope}, func(refresher Refresher, scope Scope) error {
			return refresher.Refresh(ctx, scope)
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) RefreshAllInternal(ctx context.Context, scope Scope) error {
	return r.RefreshAllInternalBatch(ctx, []Scope{scope})
}

func (r *Registry) RefreshAllInternalBatch(ctx context.Context, scopes []Scope) error {
	return r.runBatch(ctx, r.matchAll(), normalizeScopes(scopes), func(refresher Refresher, scope Scope) error {
		return refresher.Refresh(ctx, scope)
	})
}

func (r *Registry) runBatch(ctx context.Context, refs []Refresher, scopes []Scope, fn func(Refresher, Scope) error) error {
	var errs []error
	for _, scope := range scopes {
		for _, refresher := range refs {
			if err := ctx.Err(); err != nil {
				errs = append(errs, err)
				return errors.Join(errs...)
			}
			if err := fn(refresher, scope); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) matchForModule(modulePrefix string) ([]Refresher, error) {
	prefix, err := parseModulePrefix(modulePrefix)
	if err != nil {
		return nil, err
	}
	return r.matchRequired(prefix)
}

func (r *Registry) matchRequired(modulePrefix string) ([]Refresher, error) {
	refs := r.match(modulePrefix)
	if len(refs) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrModuleNotFound, modulePrefix)
	}
	return refs, nil
}

func (r *Registry) matchAll() []Refresher {
	return r.match("")
}

func (r *Registry) match(modulePrefix string) []Refresher {
	modulePrefix = keys.CanonicalModulePrefix(modulePrefix)

	r.mu.RLock()
	defer r.mu.RUnlock()

	prefixes := make([]string, 0, len(r.items))
	for prefix := range r.items {
		if modulePrefix == "" || prefix == modulePrefix || strings.HasPrefix(prefix, modulePrefix+".") {
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Strings(prefixes)

	refs := make([]Refresher, 0, len(prefixes))
	for _, prefix := range prefixes {
		refs = append(refs, r.items[prefix])
	}
	return refs
}

func (r *Registry) checkLimit(ctx context.Context, key string, interval time.Duration) error {
	if r.limit == nil || interval <= 0 {
		return nil
	}
	allowed, retryAfter, err := r.limit.Allow(ctx, key, interval)
	if err != nil {
		return fmt.Errorf("check cache refresh limit: %w", err)
	}
	if !allowed {
		return &RateLimitedError{Key: key, RetryAfter: retryAfter}
	}
	return nil
}

func parseModulePrefix(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrModulePrefixEmpty
	}
	globalPrefix := keys.CanonicalModulePrefix(keys.DefaultGlobalPrefix)
	prefix := keys.CanonicalModulePrefix(raw)
	if prefix == "" {
		return "", ErrModulePrefixEmpty
	}
	if prefix == globalPrefix || strings.HasPrefix(prefix, globalPrefix+".") {
		return "", fmt.Errorf("%w: %s", ErrGlobalPrefixInName, raw)
	}
	return prefix, nil
}

func normalizeScopes(scopes []Scope) []Scope {
	if len(scopes) == 0 {
		return []Scope{{}}
	}
	return append([]Scope(nil), scopes...)
}
