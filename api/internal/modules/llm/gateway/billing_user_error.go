package gateway

import (
	"errors"
	"fmt"

	llmerrors "github.com/zgiai/ginext/internal/modules/llm/errors"
)

type BillingUserErrorKind string

const (
	BillingUserErrorKindOrganizationBalanceInsufficient   BillingUserErrorKind = "organization_balance_insufficient"
	BillingUserErrorKindWorkspaceQuotaInsufficient        BillingUserErrorKind = "workspace_quota_insufficient"
	BillingUserErrorKindPrivateChannelBalanceInsufficient BillingUserErrorKind = "private_channel_balance_insufficient"
)

type BillingUserError struct {
	Kind  BillingUserErrorKind
	Cause error
}

func (e *BillingUserError) Error() string {
	if e == nil {
		return "billing user error"
	}
	return string(e.Kind)
}

func (e *BillingUserError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func wrapBillingUserError(kind BillingUserErrorKind, cause error) error {
	if cause == nil {
		cause = ErrBillingFailed
	}
	return fmt.Errorf("%w: %w", llmerrors.DomainErrBillingFailed, &BillingUserError{Kind: kind, Cause: cause})
}

func organizationBalanceInsufficientError() error {
	return wrapBillingUserError(BillingUserErrorKindOrganizationBalanceInsufficient, ErrInsufficientBalance)
}

func privateChannelBalanceInsufficientError() error {
	return wrapBillingUserError(BillingUserErrorKindPrivateChannelBalanceInsufficient, ErrInsufficientBalance)
}

func workspaceQuotaInsufficientError(cause error) error {
	if cause == nil || !errors.Is(cause, ErrInsufficientQuota) {
		cause = ErrInsufficientQuota
	}
	return wrapBillingUserError(BillingUserErrorKindWorkspaceQuotaInsufficient, cause)
}
