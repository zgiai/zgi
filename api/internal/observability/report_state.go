package observability

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
)

type reportStateContextKey struct{}

// ReportState tracks whether a request has already emitted a semantic error.
// It prevents outer fallback middleware from reporting the same failure again
// under a less useful generic name.
type ReportState struct {
	reportedError atomic.Bool
	mu            sync.RWMutex
	errors        []error
}

// WithReportState attaches request-scoped deduplication state.
func WithReportState(ctx context.Context) (context.Context, *ReportState) {
	if ctx == nil {
		ctx = context.Background()
	}
	if existing, ok := ctx.Value(reportStateContextKey{}).(*ReportState); ok && existing != nil {
		return ctx, existing
	}
	state := &ReportState{}
	return context.WithValue(ctx, reportStateContextKey{}, state), state
}

// Reported reports whether an error-level semantic event was emitted for this
// request. Warning diagnostics do not suppress a later request failure.
func (s *ReportState) Reported() bool {
	return s != nil && s.reportedError.Load()
}

// ReportedError reports whether this concrete failure, or an equivalent wrap,
// was already emitted. A different later error must still reach the HTTP
// fallback reporter even when an earlier provider attempt was reported.
func (s *ReportState) ReportedError(err error) bool {
	if s == nil || err == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, reported := range s.errors {
		if sameErrorInstance(err, reported) {
			return true
		}
		// Context sentinels are shared by unrelated operations. A billing
		// timeout wrapping DeadlineExceeded is not the same failure as an
		// earlier provider timeout that returned the sentinel directly.
		if sameErrorInstance(reported, context.Canceled) || sameErrorInstance(reported, context.DeadlineExceeded) {
			continue
		}
		if errorChainContainsInstance(err, reported) {
			return true
		}
	}
	return false
}

func errorChainContainsInstance(err, target error) bool {
	if err == nil || target == nil {
		return false
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			if sameErrorInstance(child, target) || errorChainContainsInstance(child, target) {
				return true
			}
		}
	case interface{ Unwrap() error }:
		child := wrapped.Unwrap()
		return sameErrorInstance(child, target) || errorChainContainsInstance(child, target)
	}
	return false
}

func sameErrorInstance(left, right error) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftType := reflect.TypeOf(left)
	if leftType != reflect.TypeOf(right) || !leftType.Comparable() {
		return false
	}
	return reflect.ValueOf(left).Interface() == reflect.ValueOf(right).Interface()
}

func markErrorReported(ctx context.Context, err error, level Level) {
	if ctx == nil {
		return
	}
	if state, ok := ctx.Value(reportStateContextKey{}).(*ReportState); ok && state != nil {
		if level == LevelError && err != nil {
			state.mu.Lock()
			state.errors = append(state.errors, err)
			state.mu.Unlock()
		}
		if level == LevelError {
			state.reportedError.Store(true)
		}
	}
}
