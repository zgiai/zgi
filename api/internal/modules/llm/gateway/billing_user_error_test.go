package gateway

import (
	"errors"
	"fmt"
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
)

// T9 acceptance 2: the four quota causes (organization balance, private channel
// balance, workspace quota, API key quota) must surface separately. Each cause
// keeps its own BillingUserErrorKind so no caller can collapse them into a
// single generic insufficient_quota outcome.
func TestT9PredeductErrorDistinguishesFourQuotaCauses(t *testing.T) {
	cases := []struct {
		name              string
		cause             error
		billingCtx        *BillingContext
		useSystemProvider bool
		wantKind          BillingUserErrorKind
	}{
		{
			name:              "organization balance",
			cause:             ErrInsufficientBalance,
			billingCtx:        &BillingContext{QuotaSubjectType: quotaSubjectTypeOrganization},
			useSystemProvider: true,
			wantKind:          BillingUserErrorKindOrganizationBalanceInsufficient,
		},
		{
			name:              "private channel balance",
			cause:             ErrInsufficientBalance,
			billingCtx:        &BillingContext{QuotaSubjectType: quotaSubjectTypeOrganization},
			useSystemProvider: false,
			wantKind:          BillingUserErrorKindPrivateChannelBalanceInsufficient,
		},
		{
			name:              "workspace quota",
			cause:             ErrInsufficientQuota,
			billingCtx:        &BillingContext{QuotaSubjectType: quotaSubjectTypeWorkspace},
			useSystemProvider: false,
			wantKind:          BillingUserErrorKindWorkspaceQuotaInsufficient,
		},
		{
			name:              "api key quota",
			cause:             ErrInsufficientQuota,
			billingCtx:        &BillingContext{QuotaSubjectType: quotaSubjectTypeAPIKey},
			useSystemProvider: false,
			wantKind:          BillingUserErrorKindAPIKeyQuotaInsufficient,
		},
	}

	seen := make(map[BillingUserErrorKind]string, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := wrapBillingPredeductErrorForUser(tc.cause, tc.billingCtx, tc.useSystemProvider)
			var userErr *BillingUserError
			if !errors.As(err, &userErr) || userErr == nil {
				t.Fatalf("error %v did not carry a BillingUserError", err)
			}
			if userErr.Kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", userErr.Kind, tc.wantKind)
			}
			if !errors.Is(err, tc.cause) {
				t.Fatalf("error %v no longer unwraps cause %v", err, tc.cause)
			}
			if previous, exists := seen[userErr.Kind]; exists {
				t.Fatalf("kind %q reused for %q and %q; causes collapsed", userErr.Kind, previous, tc.name)
			}
			seen[userErr.Kind] = tc.name
		})
	}
}

// A personal key spends its member grant, so the grant subject shares the
// API-key quota cause instead of falling back to raw ErrInsufficientQuota.
func TestT9PredeductErrorMapsMemberGrantToAPIKeyQuota(t *testing.T) {
	err := wrapBillingPredeductErrorForUser(
		ErrInsufficientQuota,
		&BillingContext{QuotaSubjectType: quotaSubjectTypeAccessGrant},
		false,
	)
	var userErr *BillingUserError
	if !errors.As(err, &userErr) || userErr == nil || userErr.Kind != BillingUserErrorKindAPIKeyQuotaInsufficient {
		t.Fatalf("kind = %#v, want %q", userErr, BillingUserErrorKindAPIKeyQuotaInsufficient)
	}
}

// The public prededuct wrapper (used by every media path) must surface the
// API-key quota cause instead of a generic pre-deduct failure, while keeping
// the cause and the domain sentinel unwrappable.
func TestT9PredeductWrapperSurfacesAPIKeyQuotaCause(t *testing.T) {
	bc := &BillingContext{
		QuotaSubjectType: quotaSubjectTypeAPIKey,
		BillingLane:      UsageBillingLanePrivate,
		RequestID:        "req-t9-key-quota",
		AttemptID:        "req-t9-key-quota-a1",
	}
	selection := &ProviderSelection{UseSystemProvider: false}

	err := wrapBillingPreDeductError(fmt.Errorf("pre-deduct quota: %w", ErrInsufficientQuota), bc, selection)
	var userErr *BillingUserError
	if !errors.As(err, &userErr) || userErr == nil || userErr.Kind != BillingUserErrorKindAPIKeyQuotaInsufficient {
		t.Fatalf("error %v kind = %#v, want %q", err, userErr, BillingUserErrorKindAPIKeyQuotaInsufficient)
	}
	if !errors.Is(err, ErrInsufficientQuota) {
		t.Fatalf("error %v did not preserve ErrInsufficientQuota", err)
	}
	if !errors.Is(err, llmerrors.DomainErrBillingFailed) {
		t.Fatalf("error %v did not preserve DomainErrBillingFailed", err)
	}
}
