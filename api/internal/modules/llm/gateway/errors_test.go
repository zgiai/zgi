package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestWrapPricingCalculationErrorPreservesClassificationAndCause(t *testing.T) {
	cause := errors.New("pricing rule unavailable")
	got := wrapPricingCalculationError(cause)
	if !errors.Is(got, ErrPricingCalculationFailed) {
		t.Fatalf("error = %v, want pricing classification", got)
	}
	if !errors.Is(got, cause) {
		t.Fatalf("error = %v, want original cause", got)
	}
}

func TestProviderSelectionConversionErrorPreservesConcreteCauses(t *testing.T) {
	transportErr := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}
	configErr := errors.New("invalid route configuration")
	err := NewProviderSelectionConversionError(
		fmt.Errorf("route one: %w", transportErr),
		fmt.Errorf("route two: %w", configErr),
	)

	if !IsProviderSelectionConversionError(err) {
		t.Fatalf("error = %v, want provider-selection conversion marker", err)
	}
	if !IsProviderSelectionPreparationError(err) {
		t.Fatalf("error = %v, want provider-selection preparation marker", err)
	}
	if !errors.Is(err, transportErr) || !errors.Is(err, configErr) {
		t.Fatalf("error = %v, want both concrete causes", err)
	}
}

func TestShadowContextErrorMarksPreSelectionBoundary(t *testing.T) {
	cause := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}
	err := &shadowContextError{cause: cause}

	if !IsProviderSelectionPreparationError(err) {
		t.Fatalf("error = %v, want provider-selection preparation marker", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want concrete database cause", err)
	}
}

func TestReportedProviderFailureErrorPreservesIdentity(t *testing.T) {
	cause := context.DeadlineExceeded
	err := NewReportedProviderFailureError(cause)

	if !IsProviderFailureReported(err) || !errors.Is(err, cause) {
		t.Fatalf("error = %v, want reported marker with original deadline", err)
	}
}

func TestClientIOErrorPreservesIdentity(t *testing.T) {
	cause := errors.New("client disconnected")
	err := NewClientIOError(cause)

	if !IsClientIOError(err) || !errors.Is(err, cause) {
		t.Fatalf("error = %v, want client I/O marker with original cause", err)
	}
}

func TestNewNoProviderAvailableErrorPreservesTypedCauseAndContext(t *testing.T) {
	err := NewNoProviderAvailableError("qwen-plus", "org-1")
	if !errors.Is(err, ErrNoProviderAvailable) {
		t.Fatalf("error = %v, want ErrNoProviderAvailable", err)
	}
	if !strings.Contains(err.Error(), "qwen-plus") || !strings.Contains(err.Error(), "org-1") {
		t.Fatalf("error = %v, want model and tenant context", err)
	}
}
