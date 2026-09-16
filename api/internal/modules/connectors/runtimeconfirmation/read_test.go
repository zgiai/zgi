package runtimeconfirmation

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"strings"
	"testing"
)

func TestServiceReadStatusNeverRoutesOrWrites(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	router := &runtimeConfirmationRouterSpy{}
	service := mustRuntimeConfirmationService(t, store, router)
	query := StatusQuery{OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID, RouteReceipt: command.RouteReceipt}
	for _, state := range []execution.ConfirmationContinuationState{execution.ConfirmationContinuationPending, execution.ConfirmationContinuationCompleted, execution.ConfirmationContinuationCancelled, execution.ConfirmationContinuationFailed, execution.ConfirmationContinuationUnknown} {
		store.continuation.State = state
		store.continuation.DecisionID = uuid.New()
		result, err := service.ReadStatus(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		if result.Resolution != "resolved" && state != execution.ConfirmationContinuationPending {
			t.Fatalf("missing resolution: %#v", result)
		}
		if state == execution.ConfirmationContinuationPending && (result.Resolution != "unresolved" || result.State != "" || result.DecisionID != uuid.Nil) {
			t.Fatalf("pending invented a terminal: %#v", result)
		}
		data, _ := json.Marshal(result)
		if strings.Contains(string(data), runtimeConfirmationSecretSentinel) || strings.Contains(string(data), "Origin") {
			t.Fatal("secret or host data exposed")
		}
	}
	if router.calls != 0 || store.recordCalls != 0 {
		t.Fatal("read status routed or persisted")
	}
	query.OwnerAccountID = uuid.New()
	if _, err := service.ReadStatus(context.Background(), query); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("cross owner: %v", err)
	}
	query.OwnerAccountID = command.OwnerAccountID
	query.OrganizationID = uuid.New()
	if _, err := service.ReadStatus(context.Background(), query); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("cross org: %v", err)
	}
	query.OrganizationID = command.OrganizationID
	query.RouteReceipt = "not-a-receipt"
	if _, err := service.ReadStatus(context.Background(), query); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatalf("invalid receipt: %v", err)
	}
	query.RouteReceipt = command.RouteReceipt
	store.continuation.Frozen.Request.ActingPrincipalID = uuid.New()
	if _, err := service.ReadStatus(context.Background(), query); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("frozen owner mismatch: %v", err)
	}
}

func TestReadConfirmationPresentationNegotiation(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	router := &runtimeConfirmationRouterSpy{}
	service := mustRuntimeConfirmationService(t, store, router)
	preview := &execution.ConfirmationPreview{AccountIdentity: "PRIVATE-SENDER", Kind: "mail", Complete: true, Fields: []execution.ConfirmationPreviewField{{Label: "Recipient", Value: "PRIVATE-RECIPIENT"}}}
	store.continuation.Frozen.Preview = preview
	query := StatusQuery{OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID, RouteReceipt: command.RouteReceipt}
	legacy, err := service.ReadStatus(t.Context(), query)
	if err != nil || legacy.Presentation != nil {
		t.Fatal("legacy response changed", err)
	}
	query.PresentationVersion = 1
	result, err := service.ReadStatus(t.Context(), query)
	if err != nil || result.Presentation.Preview.AccountIdentity != "PRIVATE-SENDER" || result.Resolution != "unresolved" {
		t.Fatal("preview missing", err)
	}
	query.OwnerAccountID = uuid.New()
	if _, err := service.ReadStatus(t.Context(), query); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatal("cross-owner preview allowed")
	}
	query.OwnerAccountID = command.OwnerAccountID
	query.OrganizationID = uuid.New()
	if _, err := service.ReadStatus(t.Context(), query); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatal("cross-org preview allowed")
	}
	query.OrganizationID = command.OrganizationID
	query.PresentationVersion = 2
	if _, err := service.ReadStatus(t.Context(), query); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatal("unknown version allowed")
	}
	if router.calls != 0 || store.recordCalls != 0 {
		t.Fatal("preview read executed or changed operation")
	}
}
