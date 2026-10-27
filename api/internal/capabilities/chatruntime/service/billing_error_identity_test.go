package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/response"
)

// Billing failures without a user-actionable kind used to be labelled
// llm.invocation.failed, and the failed message persisted only "Internal server
// error". They now keep their billing catalog code and the persisted message is
// that code's catalog copy.
func TestStableAichatErrorCodeClassifiesBillingFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "gateway settle", err: fmt.Errorf("%w: %w", llmerrors.DomainErrBillingFailed, fmt.Errorf("%w: request_id=r1: boom", gateway.ErrBillingSettleFailed)), want: "billing.settle.failed"},
		{name: "adapter settle", err: fmt.Errorf("stream: %w", adapter.ErrBillingSettleFailed), want: "billing.settle.failed"},
		{name: "pre-deduct", err: fmt.Errorf("%w: %w", llmerrors.DomainErrBillingFailed, fmt.Errorf("%w: route_id=x: rpc down", gateway.ErrBillingPreDeductFailed)), want: "billing.prededuct.failed"},
		{name: "generic billing", err: fmt.Errorf("%w: lane mismatch", llmerrors.DomainErrBillingFailed), want: "billing.settle.failed"},
		{name: "exchange rate inside settle", err: fmt.Errorf("%w: %w", gateway.ErrBillingSettleFailed, gateway.ErrExchangeRateUnavailable), want: "billing.exchange_rate.unavailable"},
	}
	for _, tc := range cases {
		if got := stableAichatErrorCode(tc.err); got != tc.want {
			t.Fatalf("%s: code = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCompletePreparedErrorPersistsCatalogMessageAndCode(t *testing.T) {
	catalog, err := appcatalog.New(
		appcatalog.LocaleEnglishUS,
		appcatalog.CodeInternal,
		append(append(appcatalog.DefaultDefinitions(), llmerrors.CatalogDefinitions()...), llmerrors.BillingCatalogDefinitions()...)...,
	)
	if err != nil {
		t.Fatalf("compose catalog: %v", err)
	}
	SetApplicationErrorCatalog(catalog)
	t.Cleanup(func() { SetApplicationErrorCatalog(nil) })

	messages := &errorRecordingMessageRepo{}
	svc := &service{repos: &repository.Repositories{Message: messages, Conversation: noopAfterMessageConversationRepo{}}}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
	}
	cause := fmt.Errorf("%w: %w", llmerrors.DomainErrBillingFailed, fmt.Errorf("%w: request_id=r1 route_id=secret", gateway.ErrBillingSettleFailed))
	ctx := WithStreamLocale(context.Background(), "zh-CN")

	if err := svc.completePreparedError(ctx, prepared, cause); err != nil {
		t.Fatalf("completePreparedError: %v", err)
	}

	presentation, err := catalog.Present(llmerrors.AppCodeBillingSettleFailed, appcatalog.LocaleChineseSimplified, nil)
	if err != nil {
		t.Fatalf("present: %v", err)
	}
	if messages.errorMessage != presentation.Message || messages.errorMessage == response.ErrSystemError.Message {
		t.Fatalf("persisted error = %q, want the zh-Hans catalog message %q", messages.errorMessage, presentation.Message)
	}
	if messages.metadata["error_code"] != "billing.settle.failed" {
		t.Fatalf("persisted metadata = %#v, want error_code billing.settle.failed", messages.metadata)
	}
}

type errorRecordingMessageRepo struct {
	repository.MessageRepository
	metadata     map[string]interface{}
	errorMessage string
}

func (r *errorRecordingMessageRepo) UpdateMetadata(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.metadata = metadata
	return nil
}

func (r *errorRecordingMessageRepo) UpdateError(_ context.Context, _ uuid.UUID, message string) error {
	r.errorMessage = message
	return nil
}

type noopAfterMessageConversationRepo struct {
	repository.ConversationRepository
}

func (noopAfterMessageConversationRepo) UpdateAfterMessage(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func TestQueryTooLongKeepsInvalidInputAndStableCode(t *testing.T) {
	err := errQueryTooLong()
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("errQueryTooLong no longer matches ErrInvalidInput: %v", err)
	}
	if got := stableAichatErrorCode(err); got != CodeQueryTooLong.String() {
		t.Fatalf("code = %q, want %s", got, CodeQueryTooLong)
	}
}
