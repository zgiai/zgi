package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrModulePrefixEmpty  = errors.New("cache module prefix is empty")
	ErrModuleNotFound     = errors.New("cache module not registered")
	ErrGlobalPrefixInName = errors.New("cache module prefix must not include global cache prefix")
)

const (
	DefaultModuleRefreshInterval = time.Minute
	DefaultGlobalRefreshInterval = 5 * time.Minute
)

type Scope struct {
	OrganizationID *uuid.UUID
	AccountID      *uuid.UUID
}

func (s Scope) Key() string {
	parts := make([]string, 0, 4)
	if s.OrganizationID != nil {
		parts = append(parts, "org", s.OrganizationID.String())
	}
	if s.AccountID != nil {
		parts = append(parts, "account", s.AccountID.String())
	}
	if len(parts) == 0 {
		return "global"
	}
	return strings.Join(parts, ":")
}

func (s Scope) IsZero() bool {
	return s.OrganizationID == nil && s.AccountID == nil
}

type Refresher interface {
	Prefix() string
	Invalidate(ctx context.Context, scope Scope) error
	Refresh(ctx context.Context, scope Scope) error
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, interval time.Duration) (bool, time.Duration, error)
}

type RateLimitedError struct {
	Key        string
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	if e == nil {
		return "cache refresh rate limited"
	}
	return fmt.Sprintf("cache refresh rate limited for %s, retry after %s", e.Key, e.RetryAfter)
}
