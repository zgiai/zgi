package skillloop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// defaultModelRetryDelays waits before the first and second retry of a model
// call. Provider throttling and overload usually clear within seconds.
var defaultModelRetryDelays = []time.Duration{time.Second, 4 * time.Second}

// connectionLost marks an error of the connection under a model call as a
// closed stream: the provider closed it early, reset it, or a TLS record
// failed. Adapters pass such read errors through as they are, so without the
// mark the round would not retry them and the user would see an internal
// error. Timeouts and cancellations keep their own errors.
func connectionLost(err error) error {
	if err == nil || errors.Is(err, adapter.ErrStreamClosed) || errors.Is(err, adapter.ErrTimeout) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var timeoutErr interface{ Timeout() bool }
	if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
		return err
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return fmt.Errorf("%w: %w", adapter.ErrStreamClosed, err)
	}
	return err
}

func (r *Runner) modelRetryDelays() []time.Duration {
	if r != nil && r.ModelRetryDelays != nil {
		return r.ModelRetryDelays
	}
	return defaultModelRetryDelays
}

// transientModelError reports a failed model call that another attempt can
// succeed at: provider throttling, a server or proxy error, or a stream cut
// off before any answer reached the user.
//
// Timeouts are not retried: each one has already waited the full timeout. A
// rejection of the request, a billing or quota state, and a failure after the
// user saw part of the answer are not retried either.
func transientModelError(err error) bool {
	if err == nil || errors.Is(err, ErrModelIdleTimeout) || adapter.IsDeterministicRejection(err) {
		return false
	}
	var streamed *streamedFinalAnswerError
	if errors.As(err, &streamed) {
		return false
	}
	for _, permanent := range []error{
		adapter.ErrAuthFailed,
		adapter.ErrInsufficientBalance,
		adapter.ErrQuotaExhausted,
		adapter.ErrBillingUnavailable,
		adapter.ErrBillingSettleFailed,
		adapter.ErrPricingNotConfigured,
		adapter.ErrExchangeRateUnavailable,
		adapter.ErrModelNotFound,
		adapter.ErrTimeout,
	} {
		if errors.Is(err, permanent) {
			return false
		}
	}
	var adapterErr *adapter.AdapterError
	if errors.As(err, &adapterErr) && adapterErr.StatusCode >= 400 && adapterErr.StatusCode < 500 {
		return adapterErr.StatusCode == 429
	}
	return errors.Is(err, adapter.ErrRateLimited) ||
		errors.Is(err, adapter.ErrUpstreamError) ||
		errors.Is(err, adapter.ErrProxyError) ||
		errors.Is(err, adapter.ErrStreamClosed)
}
