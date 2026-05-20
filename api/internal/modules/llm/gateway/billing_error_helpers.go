package gateway

import (
	"fmt"

	"github.com/google/uuid"
	llmerrors "github.com/zgiai/ginext/internal/modules/llm/errors"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

func billingMode(useSystemProvider bool) string {
	if useSystemProvider {
		return "remote"
	}
	return "local"
}

func billingCode(base string, useSystemProvider bool) string {
	if useSystemProvider {
		return base + "_REMOTE"
	}
	return base + "_LOCAL"
}

func routeIDFromSelection(ps *ProviderSelection) string {
	if ps != nil && ps.HasRoute() {
		return ps.RouteID.String()
	}
	return ""
}

func routeIDString(channelID *uuid.UUID) string {
	if channelID == nil {
		return ""
	}
	return channelID.String()
}

func requestIDString(billingCtx *BillingContext) string {
	if billingCtx == nil {
		return ""
	}
	return billingCtx.RequestID
}

func deductionIDString(billingCtx *BillingContext) string {
	if billingCtx == nil {
		return ""
	}
	return billingCtx.DeductionID
}

func attemptIDString(billingCtx *BillingContext) string {
	if billingCtx == nil {
		return ""
	}
	return billingCtx.AttemptID
}

func quotaSubjectTypeString(billingCtx *BillingContext) string {
	if billingCtx == nil {
		return ""
	}
	return billingCtx.QuotaSubjectType
}

func quotaSubjectIDString(billingCtx *BillingContext) string {
	if billingCtx == nil {
		return ""
	}
	return billingCtx.QuotaSubjectID
}

func invocationResultFromBillingStatus(status string) string {
	if billingContextStatusIsSuccess(status) {
		return "success"
	}
	return "error"
}

func logBillingEvent(
	code string,
	billingCtx *BillingContext,
	routeID string,
	useSystemProvider bool,
	phase string,
	result string,
	err error,
) {
	if err != nil {
		logger.Error("LLM billing event failed",
			err,
			zap.String("billing_code", code),
			zap.String("request_id", requestIDString(billingCtx)),
			zap.String("attempt_id", attemptIDString(billingCtx)),
			zap.String("deduction_id", deductionIDString(billingCtx)),
			zap.String("route_id", routeID),
			zap.Bool("use_system_provider", useSystemProvider),
			zap.String("billing_lane", billingLaneString(billingCtx, useSystemProvider)),
			zap.String("billing_mode", billingMode(useSystemProvider)),
			zap.String("quota_subject_type", quotaSubjectTypeString(billingCtx)),
			zap.String("quota_subject_id", quotaSubjectIDString(billingCtx)),
			zap.String("phase", phase),
			zap.String("result", result),
		)
		return
	}
	logger.Info("LLM billing event",
		zap.String("billing_code", code),
		zap.String("request_id", requestIDString(billingCtx)),
		zap.String("attempt_id", attemptIDString(billingCtx)),
		zap.String("deduction_id", deductionIDString(billingCtx)),
		zap.String("route_id", routeID),
		zap.Bool("use_system_provider", useSystemProvider),
		zap.String("billing_lane", billingLaneString(billingCtx, useSystemProvider)),
		zap.String("billing_mode", billingMode(useSystemProvider)),
		zap.String("quota_subject_type", quotaSubjectTypeString(billingCtx)),
		zap.String("quota_subject_id", quotaSubjectIDString(billingCtx)),
		zap.String("phase", phase),
		zap.String("result", result),
	)
}

func wrapBillingPreDeductError(err error, billingCtx *BillingContext, providerSelection *ProviderSelection) error {
	if err == nil {
		return nil
	}

	usageLane, _ := usageBillingLaneFromContext(providerSelection, billingCtx)
	useSystemProvider := usageBillingLaneUsesSystemProvider(usageLane)
	if userErr := wrapBillingPredeductErrorForUser(err, billingCtx, useSystemProvider); userErr != err {
		return userErr
	}

	preDeductErr := fmt.Errorf(
		"%w: request_id=%s attempt_id=%s route_id=%s use_system_provider=%t billing_mode=%s: %w",
		ErrBillingPreDeductFailed,
		requestIDString(billingCtx),
		attemptIDString(billingCtx),
		routeIDFromSelection(providerSelection),
		useSystemProvider,
		billingMode(useSystemProvider),
		err,
	)
	return fmt.Errorf("%w: %w", llmerrors.DomainErrBillingFailed, preDeductErr)
}

func wrapBillingSettleError(
	err error,
	billingCtx *BillingContext,
	useSystemProvider bool,
	routeID string,
) error {
	if err == nil {
		return nil
	}

	settleErr := fmt.Errorf(
		"%w: request_id=%s attempt_id=%s route_id=%s use_system_provider=%t billing_mode=%s: %w",
		ErrBillingSettleFailed,
		requestIDString(billingCtx),
		attemptIDString(billingCtx),
		routeID,
		useSystemProvider,
		billingMode(useSystemProvider),
		err,
	)
	return fmt.Errorf("%w: %w", llmerrors.DomainErrBillingFailed, settleErr)
}

func wrapBillingLaneMismatchError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", llmerrors.DomainErrBillingFailed, err)
}
