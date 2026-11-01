// Package requesttimeout carries the caller-chosen total duration of one
// gateway request (header X-ZGI-Request-Timeout, in whole seconds) from the
// public edge to the adapters, and on to Console: the same header on the
// platform proxy call and the same value as gRPC metadata on billing holds.
// The rules mirror Console's internal/infra/requesttimeout.
package requesttimeout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
	"google.golang.org/grpc/metadata"
)

const (
	// HeaderName is the HTTP request header a caller sets. Only the Console
	// platform proxy receives it; direct provider adapters never do.
	HeaderName = "X-ZGI-Request-Timeout"
	// MetadataKey is the same value on gRPC calls to Console.
	MetadataKey = "x-zgi-request-timeout"

	maxEnvKey             = "ZGI_PROXY_MAX_REQUEST_TIMEOUT_SECONDS"
	defaultEnvKey         = "ZGI_PROXY_UPSTREAM_TIMEOUT_SECONDS"
	defaultMaxSeconds     = 3600
	defaultSeconds        = 1200
	minRequestTimeoutSecs = 1
	minDefaultSeconds     = 15
)

// ErrInvalid reports a header value that is not a whole number of seconds
// inside [1, Max()]. Callers answer it with 400 instead of clamping.
var ErrInvalid = errors.New("invalid request timeout")

// warn reports unusable env values; tests replace it.
var warn = func(msg string) { logger.Warn(msg) }

var (
	limitsOnce   sync.Once
	maxValue     time.Duration
	defaultValue time.Duration
)

func loadLimits() {
	limitsOnce.Do(func() {
		maxValue = loadMax()
		defaultValue = loadDefault(maxValue)
	})
}

// Max is the largest total duration a caller may request.
func Max() time.Duration {
	loadLimits()
	return maxValue
}

// Default is the total duration of a request that sent no header.
func Default() time.Duration {
	loadLimits()
	return defaultValue
}

func loadMax() time.Duration {
	raw := strings.TrimSpace(os.Getenv(maxEnvKey))
	if raw == "" {
		return defaultMaxSeconds * time.Second
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < minRequestTimeoutSecs {
		warn(fmt.Sprintf("%s=%q is not a positive number; using the default %ds", maxEnvKey, raw, defaultMaxSeconds))
		return defaultMaxSeconds * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func loadDefault(limit time.Duration) time.Duration {
	value := defaultSeconds * time.Second
	if raw := strings.TrimSpace(os.Getenv(defaultEnvKey)); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < minDefaultSeconds {
			warn(fmt.Sprintf("%s=%q is not a number >= %d; using the default %ds", defaultEnvKey, raw, minDefaultSeconds, defaultSeconds))
		} else {
			value = time.Duration(seconds) * time.Second
		}
	}
	if value > limit {
		warn(fmt.Sprintf("%s default %s is above the request maximum %s; using the maximum", defaultEnvKey, value, limit))
		value = limit
	}
	return value
}

// Parse validates a header value. Only plain decimal digits are accepted, so
// "1.5", "+60", "60s" and "-1" are all rejected rather than reinterpreted.
func Parse(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.TrimLeft(raw, "0123456789") != "" {
		return 0, fmt.Errorf("%w: %s must be a whole number of seconds", ErrInvalid, HeaderName)
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < minRequestTimeoutSecs {
		return 0, fmt.Errorf("%w: %s must be at least %d", ErrInvalid, HeaderName, minRequestTimeoutSecs)
	}
	timeout := time.Duration(seconds) * time.Second
	if limit := Max(); timeout > limit {
		return 0, fmt.Errorf("%w: %s must not exceed %d", ErrInvalid, HeaderName, int(limit/time.Second))
	}
	return timeout, nil
}

type contextKey struct{}

type contextValue struct {
	timeout   time.Duration
	requested bool
}

// WithTimeout records a total duration the caller asked for explicitly.
func WithTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if ctx == nil || timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, contextValue{timeout: timeout, requested: true})
}

// Requested returns the duration only when the caller asked for it explicitly.
func Requested(ctx context.Context) (time.Duration, bool) {
	if ctx == nil {
		return 0, false
	}
	value, ok := ctx.Value(contextKey{}).(contextValue)
	if !ok || !value.requested {
		return 0, false
	}
	return value.timeout, true
}

// Effective is the requested duration, else Default().
func Effective(ctx context.Context) time.Duration {
	if timeout, ok := Requested(ctx); ok {
		return timeout
	}
	return Default()
}

// AppendOutgoingMetadata sends the effective duration with a gRPC call to
// Console, so a billing hold outlives the request it pays for.
func AppendOutgoingMetadata(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	seconds := int(Effective(ctx) / time.Second)
	return metadata.AppendToOutgoingContext(ctx, MetadataKey, strconv.Itoa(seconds))
}
