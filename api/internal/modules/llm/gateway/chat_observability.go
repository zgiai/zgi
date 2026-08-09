package gateway

import (
	"context"
	"errors"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/database"
)

func reportLLMSelectionFailure(ctx context.Context, err error, model, organizationID, shadowTenantID string) {
	eventName := "llm.provider.selection_failed"
	level := observability.LevelError
	classification := observability.ErrorClassification{
		Category: observability.ErrorCategoryApplication,
		Source:   observability.ErrorSourceZGI,
		Code:     "provider_selection_failed",
	}
	if databaseClassification := database.ClassifyError(err); databaseClassification.Source == observability.ErrorSourceInfrastructure {
		eventName = "llm.provider.selection_failed"
		classification = databaseClassification
	} else if errors.Is(err, adapter.ErrCapabilityUnsupported) {
		eventName = "llm.route.not_configured"
		level = observability.LevelWarning
		classification = observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "model_capability_not_configured",
		}
	} else if errors.Is(err, llmerrors.DomainErrPrivateChannelUpstreamUnavailable) {
		eventName = "llm.provider.unavailable"
		classification = observability.ErrorClassification{
			Category:  observability.ErrorCategoryDependency,
			Source:    observability.ErrorSourceProvider,
			Code:      "private_channel_upstream_unavailable",
			Retryable: true,
		}
	} else if errors.Is(err, ErrNoProviderAvailable) || errors.Is(err, llmerrors.DomainErrNoProviderAvailable) {
		eventName = "llm.provider.unavailable"
		level = observability.LevelWarning
		classification = observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "no_provider_available",
		}
	} else if errors.Is(err, llmerrors.DomainErrModelNotFound) || errors.Is(err, llmerrors.DomainErrRouteNotFound) {
		eventName = "llm.route.not_configured"
		level = observability.LevelWarning
		classification = observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "model_route_not_configured",
		}
	}

	observability.CaptureError(ctx, eventName, err,
		observability.WithLevel(level),
		observability.WithErrorClassification(classification),
		observability.Tags(map[string]string{"llm.provider": "unknown", "llm.model": model}),
		observability.Attributes(map[string]any{
			"organization_id":  organizationID,
			"shadow_tenant_id": shadowTenantID,
		}),
	)
}

func reportLLMProviderUnavailable(ctx context.Context, err error, model, organizationID, shadowTenantID string) {
	observability.CaptureError(ctx, "llm.provider.unavailable", err,
		observability.WithLevel(observability.LevelWarning),
		observability.WithErrorClassification(observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "no_provider_available",
		}),
		observability.Tags(map[string]string{"llm.provider": "unknown", "llm.model": model}),
		observability.Attributes(map[string]any{
			"organization_id":  organizationID,
			"shadow_tenant_id": shadowTenantID,
		}),
	)
}

func reportedNoProviderAvailableError(ctx context.Context, model, organizationID, shadowTenantID string) error {
	err := NewNoProviderAvailableError(model, organizationID)
	reportLLMProviderUnavailable(ctx, err, model, organizationID, shadowTenantID)
	return err
}

// reportLLMBillingSettlementFailure records a settlement failure that occurs
// after a terminal provider stream error has already been delivered. At that
// point the response channel cannot carry a second failure to HTTP middleware,
// so this semantic report is the only reliable operator signal.
func reportLLMBillingSettlementFailure(ctx context.Context, err error, billingCtx *BillingContext, routeID string, useSystemProvider bool) {
	if err == nil {
		return
	}

	provider := "unknown"
	model := "unknown"
	organizationID := ""
	requestID := ""
	attemptID := ""
	if billingCtx != nil {
		if billingCtx.ProviderName != "" {
			provider = billingCtx.ProviderName
		}
		if billingCtx.ModelName != "" {
			model = billingCtx.ModelName
		}
		organizationID = billingCtx.OrganizationID
		requestID = billingCtx.RequestID
		attemptID = billingCtx.AttemptID
	}

	observability.CaptureError(ctx, "llm.billing.settlement_failed", err,
		observability.WithErrorClassification(observability.ErrorClassification{
			Category:  observability.ErrorCategoryApplication,
			Source:    observability.ErrorSourceZGI,
			Code:      "billing_settlement_failed",
			Retryable: true,
		}),
		observability.Tags(map[string]string{
			"llm.provider": provider,
			"llm.model":    model,
			"billing.mode": billingMode(useSystemProvider),
		}),
		observability.Attributes(map[string]any{
			"organization_id": organizationID,
			"request_id":      requestID,
			"attempt_id":      attemptID,
			"route_id":        routeID,
		}),
	)
}

func reportLLMProviderFailure(ctx context.Context, err error, eventName, provider, model, organizationID string, attemptIndex int, channelID any, useSystemProvider, finalAttempt bool) {
	// These failures are deterministic client/business rejections. They are
	// represented by the invocation log and 4xx protocol response; reporting
	// them as retryable provider incidents would create false engineering alerts.
	if adapter.IsDeterministicRejection(err) {
		return
	}
	classification := observability.ErrorClassification{
		Category:  observability.ErrorCategoryDependency,
		Source:    observability.ErrorSourceProvider,
		Code:      "upstream_request_failed",
		Retryable: true,
	}
	level := providerAttemptLevel(finalAttempt)
	if channelClassification, channelLevel, ok := classifyPersistentChannelFailure(err, useSystemProvider); ok {
		classification = channelClassification
		level = channelLevel
	} else if errors.Is(err, context.DeadlineExceeded) {
		classification.Category = observability.ErrorCategoryTimeout
		classification.Code = "upstream_timeout"
	}

	observability.CaptureError(ctx, eventName, err,
		observability.WithLevel(level),
		observability.WithErrorClassification(classification),
		observability.Tags(map[string]string{
			"llm.provider": provider,
			"llm.model":    model,
		}),
		observability.Attributes(map[string]any{
			"organization_id":     organizationID,
			"attempt_index":       attemptIndex,
			"channel_id":          channelID,
			"use_system_provider": useSystemProvider,
		}),
	)
}

func persistentChannelFailureCode(err error) (string, bool) {
	switch {
	case errors.Is(err, adapter.ErrAuthFailed), errors.Is(err, adapter.ErrInvalidConfig):
		return "credentials_invalid", true
	case errors.Is(err, adapter.ErrInsufficientBalance),
		errors.Is(err, adapter.ErrQuotaExhausted),
		errors.Is(err, adapter.ErrBillingUnavailable):
		return "billing_unavailable", true
	default:
		return "", false
	}
}

// IsPersistentChannelFailure reports credential and upstream-account states
// whose owner depends on whether the selected channel is tenant- or
// platform-managed. The Gateway emits the authoritative channel-aware event;
// higher layers should not add a second owner-agnostic incident.
func IsPersistentChannelFailure(err error) bool {
	_, ok := persistentChannelFailureCode(err)
	return ok
}

func classifyPersistentChannelFailure(err error, useSystemProvider bool) (observability.ErrorClassification, observability.Level, bool) {
	code, ok := persistentChannelFailureCode(err)
	if !ok {
		return observability.ErrorClassification{}, observability.LevelError, false
	}
	classification := observability.ErrorClassification{
		Category: observability.ErrorCategoryConfiguration,
		Code:     code,
	}
	level := observability.LevelError
	if useSystemProvider {
		classification.Source = observability.ErrorSourceZGI
		classification.Code = "system_channel_" + code
	} else {
		classification.Source = observability.ErrorSourceTenant
		classification.Code = "private_channel_" + code
		level = observability.LevelWarning
	}
	return classification, level, true
}

func reportLLMAdapterFailure(ctx context.Context, err error, provider, model, organizationID string, attemptIndex int, channelID any, useSystemProvider, finalAttempt bool) {
	classification := observability.ErrorClassification{
		Category: observability.ErrorCategoryApplication,
		Source:   observability.ErrorSourceZGI,
		Code:     "adapter_creation_failed",
	}
	level := providerAttemptLevel(finalAttempt)
	if channelClassification, channelLevel, ok := classifyPersistentChannelFailure(err, useSystemProvider); ok {
		classification = channelClassification
		level = channelLevel
	}
	observability.CaptureError(ctx, "llm.adapter.creation_failed", err,
		observability.WithLevel(level),
		observability.WithErrorClassification(classification),
		observability.Tags(map[string]string{
			"llm.provider": provider,
			"llm.model":    model,
		}),
		observability.Attributes(map[string]any{
			"organization_id":     organizationID,
			"attempt_index":       attemptIndex,
			"channel_id":          channelID,
			"use_system_provider": useSystemProvider,
		}),
	)
}

func reportLLMProviderFailureForSelection(ctx context.Context, err error, eventName string, selection *ProviderSelection, billingCtx *BillingContext, attemptIndex int, finalAttempt bool) {
	if err == nil || selection == nil {
		return
	}
	organizationID := ""
	if billingCtx != nil {
		organizationID = billingCtx.OrganizationID
	}
	reportLLMProviderFailure(ctx, err, eventName,
		selection.Provider.Provider,
		selection.Model.Model,
		organizationID,
		attemptIndex,
		getChannelID(selection),
		selection.UseSystemProvider,
		finalAttempt,
	)
}

func reportLLMAdapterFailureForSelection(ctx context.Context, err error, selection *ProviderSelection, billingCtx *BillingContext, attemptIndex int, finalAttempt bool) {
	if err == nil || selection == nil {
		return
	}
	organizationID := ""
	if billingCtx != nil {
		organizationID = billingCtx.OrganizationID
	}
	reportLLMAdapterFailure(ctx, err,
		selection.Provider.Provider,
		selection.Model.Model,
		organizationID,
		attemptIndex,
		getChannelID(selection),
		selection.UseSystemProvider,
		finalAttempt,
	)
}

func providerAttemptLevel(finalAttempt bool) observability.Level {
	if finalAttempt {
		return observability.LevelError
	}
	return observability.LevelWarning
}
