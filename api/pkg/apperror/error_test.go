package apperror_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror"
)

var (
	codeProviderTimeout = apperror.MustCode("llm.provider.timeout")
	codeStorageFailed   = apperror.MustCode("storage.object.write_failed")
)

func TestNewCarriesCodeAndDiagnosticContext(t *testing.T) {
	t.Parallel()

	err := apperror.New(
		codeProviderTimeout,
		apperror.WithOperation("gateway.chat_completion"),
		apperror.WithParams(
			apperror.StringParam("provider", "example"),
			apperror.IntParam("attempt", 2),
			apperror.UintParam("limit", 3),
			apperror.FloatParam("backoff_seconds", 1.5),
			apperror.BoolParam("streaming", true),
		),
	)

	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("As(%T) did not find an AppError", err)
	}
	if appErr.Code() != codeProviderTimeout {
		t.Fatalf("Code() = %q, want %q", appErr.Code(), codeProviderTimeout)
	}
	if appErr.Operation() != "gateway.chat_completion" {
		t.Fatalf("Operation() = %q", appErr.Operation())
	}
	if got := err.Error(); got != "gateway.chat_completion: llm.provider.timeout" {
		t.Fatalf("Error() = %q", got)
	}

	params := appErr.Params()
	if params["provider"] != "example" || params["attempt"] != int64(2) || params["limit"] != uint64(3) || params["backoff_seconds"] != 1.5 || params["streaming"] != true {
		t.Fatalf("Params() = %#v", params)
	}
}

func TestWrapPreservesCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("connection reset")
	err := apperror.Wrap(cause, codeProviderTimeout, apperror.WithOperation("provider.stream"))
	if err == nil {
		t.Fatal("Wrap returned nil")
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is not discoverable with errors.Is")
	}
	if got := err.Error(); got != "provider.stream: llm.provider.timeout: connection reset" {
		t.Fatalf("Error() = %q", got)
	}
	if got, ok := apperror.CodeOf(fmt.Errorf("outer context: %w", err)); !ok || got != codeProviderTimeout {
		t.Fatalf("CodeOf() = %q, %v", got, ok)
	}
}

func TestWrapNilReturnsNil(t *testing.T) {
	t.Parallel()

	if err := apperror.Wrap(nil, codeProviderTimeout); err != nil {
		t.Fatalf("Wrap(nil) = %#v, want nil", err)
	}
}

func TestIsCodeTraversesWrappedAndJoinedErrors(t *testing.T) {
	t.Parallel()

	timeoutErr := apperror.Wrap(errors.New("deadline"), codeProviderTimeout)
	storageErr := apperror.Wrap(errors.New("disk full"), codeStorageFailed)
	joined := errors.Join(fmt.Errorf("request failed: %w", timeoutErr), storageErr)

	if !apperror.IsCode(joined, codeProviderTimeout) {
		t.Fatal("IsCode did not find wrapped timeout code")
	}
	if !apperror.IsCode(joined, codeStorageFailed) {
		t.Fatal("IsCode did not find joined storage code")
	}
	if apperror.IsCode(joined, apperror.MustCode("auth.session.expired")) {
		t.Fatal("IsCode matched an unrelated code")
	}
	if apperror.IsCode(nil, codeProviderTimeout) {
		t.Fatal("IsCode matched nil")
	}
}

func TestParamsAreCopiedAndDuplicateValuesAreDeterministic(t *testing.T) {
	t.Parallel()

	err := apperror.New(
		codeProviderTimeout,
		apperror.WithParams(
			apperror.StringParam("provider", "first"),
			apperror.StringParam("", "ignored"),
		),
		apperror.WithParams(apperror.StringParam("provider", "last")),
	)
	appErr, _ := apperror.As(err)

	first := appErr.Params()
	first["provider"] = "mutated"
	first["new"] = "value"
	second := appErr.Params()

	if second["provider"] != "last" {
		t.Fatalf("internal parameter was mutated: %#v", second)
	}
	if _, exists := second["new"]; exists {
		t.Fatalf("new caller parameter leaked into Error: %#v", second)
	}
	if _, exists := second[""]; exists {
		t.Fatalf("empty parameter name was retained: %#v", second)
	}
}

func TestErrorIsSafeForConcurrentReads(t *testing.T) {
	t.Parallel()

	err := apperror.Wrap(
		errors.New("provider timeout"),
		codeProviderTimeout,
		apperror.WithOperation("gateway.chat_completion"),
		apperror.WithParams(apperror.StringParam("provider", "example")),
	)
	appErr, _ := apperror.As(err)

	const readers = 64
	var waitGroup sync.WaitGroup
	waitGroup.Add(readers)
	for range readers {
		go func() {
			defer waitGroup.Done()
			for range 1_000 {
				if appErr.Code() != codeProviderTimeout || appErr.Operation() == "" || appErr.Params()["provider"] != "example" {
					t.Errorf("concurrent read returned inconsistent Error")
					return
				}
				if !apperror.IsCode(err, codeProviderTimeout) {
					t.Errorf("concurrent IsCode failed")
					return
				}
			}
		}()
	}
	waitGroup.Wait()
}

func TestNilReceiverAccessors(t *testing.T) {
	t.Parallel()

	var appErr *apperror.Error
	if appErr.Error() != "<nil>" || appErr.Unwrap() != nil || appErr.Code() != "" || appErr.Operation() != "" || appErr.Params() != nil {
		t.Fatal("nil receiver accessors are not safe")
	}
}

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		_ = apperror.New(codeProviderTimeout)
	}
}

func BenchmarkWrapWithContext(b *testing.B) {
	cause := errors.New("deadline")
	b.ReportAllocs()
	for b.Loop() {
		_ = apperror.Wrap(
			cause,
			codeProviderTimeout,
			apperror.WithOperation("gateway.chat_completion"),
			apperror.WithParams(
				apperror.StringParam("provider", "example"),
				apperror.IntParam("attempt", 2),
			),
		)
	}
}

func BenchmarkCodeOf(b *testing.B) {
	err := fmt.Errorf("outer: %w", apperror.Wrap(errors.New("deadline"), codeProviderTimeout))
	b.ReportAllocs()
	for b.Loop() {
		_, _ = apperror.CodeOf(err)
	}
}

func BenchmarkIsCode(b *testing.B) {
	err := fmt.Errorf("outer: %w", apperror.Wrap(errors.New("deadline"), codeProviderTimeout))
	b.ReportAllocs()
	for b.Loop() {
		_ = apperror.IsCode(err, codeProviderTimeout)
	}
}
