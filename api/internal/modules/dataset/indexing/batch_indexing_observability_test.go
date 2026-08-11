package indexing

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/observability"
)

func TestClassifyIndexingEmbeddingErrorPreservesOwnership(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		source    observability.ErrorSource
		category  observability.ErrorCategory
		level     observability.Level
		retryable bool
	}{
		{name: "missing route", err: llmerrors.DomainErrRouteNotFound, source: observability.ErrorSourceTenant, category: observability.ErrorCategoryConfiguration, level: observability.LevelWarning},
		{name: "tenant quota", err: gateway.ErrInsufficientQuota, source: observability.ErrorSourceTenant, category: observability.ErrorCategoryConfiguration, level: observability.LevelWarning},
		{name: "model pricing configuration", err: &gateway.BillingUserError{Kind: gateway.BillingUserErrorKindModelPricingNotConfigured, Cause: gateway.ErrPricingNotConfigured}, source: observability.ErrorSourceTenant, category: observability.ErrorCategoryConfiguration, level: observability.LevelWarning},
		{name: "billing failure", err: fmt.Errorf("settle: %w", gateway.ErrBillingSettleFailed), source: observability.ErrorSourceZGI, category: observability.ErrorCategoryApplication, level: observability.LevelError, retryable: true},
		{name: "billing timeout retains billing ownership", err: errors.Join(gateway.ErrBillingSettleFailed, context.DeadlineExceeded), source: observability.ErrorSourceZGI, category: observability.ErrorCategoryApplication, level: observability.LevelError, retryable: true},
		{name: "selection database transport", err: gateway.NewProviderSelectionConversionError(&net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}), source: observability.ErrorSourceInfrastructure, category: observability.ErrorCategoryDatabase, level: observability.LevelError, retryable: true},
		{name: "provider timeout", err: adapter.ErrTimeout, source: observability.ErrorSourceProvider, category: observability.ErrorCategoryTimeout, level: observability.LevelError, retryable: true},
		{name: "provider failure", err: adapter.ErrUpstreamError, source: observability.ErrorSourceProvider, category: observability.ErrorCategoryDependency, level: observability.LevelError, retryable: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classification, level := classifyIndexingEmbeddingError(test.err)
			if classification.Source != test.source || classification.Category != test.category || classification.Retryable != test.retryable || level != test.level {
				t.Fatalf("classification/level = %#v/%q", classification, level)
			}
		})
	}
}

type persistentEmbeddingFailure struct {
	calls int
	err   error
}

func (f *persistentEmbeddingFailure) EmbedText(context.Context, string) ([]float64, error) {
	return nil, f.err
}

func (f *persistentEmbeddingFailure) EmbedTexts(_ context.Context, _ []string) ([][]float64, error) {
	f.calls++
	return nil, f.err
}

func (*persistentEmbeddingFailure) GetDimension() int { return 0 }
func (*persistentEmbeddingFailure) GetModel() string  { return "test" }

func TestEmbedIndexingBatchSkipsPerItemRetryForPersistentConfigurationFailure(t *testing.T) {
	service := &persistentEmbeddingFailure{err: gateway.NewNoProviderAvailableError("embed-model", "org-1")}
	items := []indexingItem{{Text: "one"}, {Text: "two"}, {Text: "three"}}

	vectors, errs := embedIndexingBatch(context.Background(), &model.Dataset{}, service, items, "segments")

	if service.calls != 1 {
		t.Fatalf("EmbedTexts calls = %d, want one batch attempt", service.calls)
	}
	if len(vectors) != len(items) || len(errs) != len(items) {
		t.Fatalf("vectors/errors = %d/%d, want %d", len(vectors), len(errs), len(items))
	}
	for i, err := range errs {
		if !errors.Is(err, gateway.ErrNoProviderAvailable) {
			t.Fatalf("errs[%d] = %v, want ErrNoProviderAvailable", i, err)
		}
	}
}

func TestEmbedIndexingBatchRetriesPerItemForTransientZGIFailure(t *testing.T) {
	service := &persistentEmbeddingFailure{err: fmt.Errorf("settle: %w", gateway.ErrBillingSettleFailed)}
	items := []indexingItem{{Text: "one"}, {Text: "two"}, {Text: "three"}}

	_, errs := embedIndexingBatch(context.Background(), &model.Dataset{}, service, items, "segments")

	if service.calls != len(items)+1 {
		t.Fatalf("EmbedTexts calls = %d, want batch plus %d item retries", service.calls, len(items))
	}
	for i, err := range errs {
		if !errors.Is(err, gateway.ErrBillingSettleFailed) {
			t.Fatalf("errs[%d] = %v, want ErrBillingSettleFailed", i, err)
		}
	}
}

func TestEmbedIndexingBatchFastFailsPersistentChannelFailure(t *testing.T) {
	service := &persistentEmbeddingFailure{err: fmt.Errorf("provider credentials: %w", adapter.ErrAuthFailed)}
	items := []indexingItem{{Text: "one"}, {Text: "two"}, {Text: "three"}}

	_, errs := embedIndexingBatch(context.Background(), &model.Dataset{}, service, items, "segments")

	if service.calls != 1 {
		t.Fatalf("EmbedTexts calls = %d, want one channel-aware Gateway attempt", service.calls)
	}
	for i, err := range errs {
		if !errors.Is(err, adapter.ErrAuthFailed) {
			t.Fatalf("errs[%d] = %v, want ErrAuthFailed", i, err)
		}
	}
}

func TestEmbedIndexingBatchFastFailsUnsupportedCapability(t *testing.T) {
	service := &persistentEmbeddingFailure{err: fmt.Errorf("embedding adapter: %w", adapter.ErrCapabilityUnsupported)}
	items := []indexingItem{{Text: "one"}, {Text: "two"}, {Text: "three"}}

	_, errs := embedIndexingBatch(context.Background(), &model.Dataset{}, service, items, "segments")

	if service.calls != 1 {
		t.Fatalf("EmbedTexts calls = %d, want one batch attempt", service.calls)
	}
	for i, err := range errs {
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("errs[%d] = %v, want ErrCapabilityUnsupported", i, err)
		}
	}
}

func TestEmbedIndexingBatchFastFailsSelectionDatabaseOutage(t *testing.T) {
	selectionErr := gateway.NewProviderSelectionConversionError(
		&net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")},
	)
	service := &persistentEmbeddingFailure{err: selectionErr}
	items := []indexingItem{{Text: "one"}, {Text: "two"}, {Text: "three"}}

	_, errs := embedIndexingBatch(context.Background(), &model.Dataset{}, service, items, "segments")

	if service.calls != 1 {
		t.Fatalf("EmbedTexts calls = %d, want one batch attempt", service.calls)
	}
	for i, err := range errs {
		if !errors.Is(err, selectionErr) {
			t.Fatalf("errs[%d] = %v, want selection database error", i, err)
		}
	}
}
