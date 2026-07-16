package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var usageBillUpsertColumns = []string{
	"request_id",
	"organization_id",
	"app_id",
	"app_type",
	"workspace_id",
	"api_key_id",
	"quota_subject_type",
	"quota_subject_id",
	"model_id",
	"model_name",
	"provider_id",
	"provider_name",
	"route_id",
	"channel_id",
	"billing_lane",
	"remote_deduction_id",
	"use_system_provider",
	"status",
	"prompt_tokens",
	"completion_tokens",
	"total_tokens",
	"official_points",
	"private_points",
	"total_points",
	"pricing_source",
	"usage_source",
	"pricing_snapshot",
	"response_time_ms",
	"error_code",
	"error_message",
	"request_created_at",
	"settled_at",
}

func usageBillStatusFromBillingContext(status string) string {
	if billingContextStatusIsSuccess(status) {
		return usageBillStatusSuccess
	}
	if billingContextStatusIsPartial(status) {
		return usageBillStatusPartial
	}
	return usageBillStatusFailed
}

func (b *BillingService) upsertUsageBill(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
	status string,
	errorCode *string,
	errorMessage *string,
) error {
	usageBill, err := b.buildUsageBill(bc, status, errorCode, errorMessage)
	if err != nil {
		return err
	}

	return tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "attempt_id"}},
			DoUpdates: clause.AssignmentColumns(usageBillUpsertColumns),
		}).
		Create(usageBill).Error
}

func (b *BillingService) buildUsageBill(
	bc *BillingContext,
	status string,
	errorCode *string,
	errorMessage *string,
) (*UsageBill, error) {
	if bc == nil {
		return nil, fmt.Errorf("billing context is nil")
	}

	requestCreatedAt, settledAt := normalizeUsageBillTimes(bc.RequestCreatedAt, bc.SettledAt)

	appID, appType := normalizedAppUsagePair(bc.AppID, bc.AppType)
	workspaceID := normalizedTextPtr(bc.WorkspaceID)
	quotaSubjectType := normalizedTextPtr(bc.QuotaSubjectType)
	quotaSubjectID := normalizedTextPtr(bc.QuotaSubjectID)
	billingLane, err := normalizeUsageBillingLane(bc.BillingLane, bc.UseSystemProvider)
	if err != nil {
		return nil, err
	}
	useSystemProvider := usageBillingLaneUsesSystemProvider(billingLane)

	promptTokens, completionTokens, totalTokens := usageBillTokens(bc, status)
	officialPoints, privatePoints := usageBillPoints(bc, status, billingLane)
	pricingSnapshot := bc.PricingSnapshot
	if len(pricingSnapshot) == 0 || string(pricingSnapshot) == "null" {
		pricingSnapshot = datatypes.JSON([]byte("{}"))
	}

	if errorCode == nil {
		errorCode = normalizedTextPtr("")
	}
	if errorMessage == nil {
		errorMessage = normalizedTextPtr(bc.ErrorMessage)
	}

	return &UsageBill{
		AttemptID:         strings.TrimSpace(bc.AttemptID),
		RequestID:         strings.TrimSpace(bc.RequestID),
		OrganizationID:    strings.TrimSpace(bc.OrganizationID),
		AppID:             appID,
		AppType:           appType,
		WorkspaceID:       workspaceID,
		APIKeyID:          strings.TrimSpace(bc.APIKeyID),
		QuotaSubjectType:  quotaSubjectType,
		QuotaSubjectID:    quotaSubjectID,
		ModelID:           bc.ModelID,
		ModelName:         strings.TrimSpace(bc.ModelName),
		ProviderID:        bc.ProviderID,
		ProviderName:      strings.TrimSpace(bc.ProviderName),
		RouteID:           normalizedUUIDPtr(bc.RouteID),
		ChannelID:         normalizedUUIDPtr(bc.ChannelID),
		BillingLane:       billingLane,
		RemoteDeductionID: remoteDeductionIDForUsageBill(bc, billingLane),
		UseSystemProvider: useSystemProvider,
		Status:            status,
		PromptTokens:      promptTokens,
		CompletionTokens:  completionTokens,
		TotalTokens:       totalTokens,
		OfficialPoints:    officialPoints,
		PrivatePoints:     privatePoints,
		TotalPoints:       officialPoints + privatePoints,
		PricingSource:     bc.PricingSource,
		UsageSource:       bc.UsageSource,
		PricingSnapshot:   pricingSnapshot,
		ResponseTimeMS:    maxInt64(bc.ResponseTime, 0),
		ErrorCode:         errorCode,
		ErrorMessage:      errorMessage,
		RequestCreatedAt:  requestCreatedAt,
		SettledAt:         settledAt,
	}, nil
}

func normalizeUsageBillTimes(requestCreatedAt, settledAt time.Time) (time.Time, time.Time) {
	if requestCreatedAt.IsZero() {
		requestCreatedAt = time.Now().UTC()
	} else {
		requestCreatedAt = requestCreatedAt.UTC()
	}

	if settledAt.IsZero() {
		settledAt = time.Now().UTC()
	} else {
		settledAt = settledAt.UTC()
	}

	if settledAt.Before(requestCreatedAt) {
		settledAt = requestCreatedAt
	}

	return requestCreatedAt, settledAt
}

func usageBillTokens(bc *BillingContext, status string) (int64, int64, int64) {
	if status != usageBillStatusSuccess && status != usageBillStatusPartial {
		return 0, 0, 0
	}

	promptTokens := maxInt64(int64(bc.PromptTokens), 0)
	completionTokens := maxInt64(int64(bc.CompletionTokens), 0)
	totalTokens := maxInt64(int64(bc.TotalTokens), 0)
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}

	return promptTokens, completionTokens, totalTokens
}

func usageBillPoints(bc *BillingContext, status string, billingLane UsageBillingLane) (int64, int64) {
	if status != usageBillStatusSuccess && status != usageBillStatusPartial {
		return 0, 0
	}

	actualCredits := maxInt64(bc.ActualCredits, 0)
	if billingLane == UsageBillingLanePlatform {
		return actualCredits, 0
	}
	return 0, actualCredits
}

func remoteDeductionIDForUsageBill(bc *BillingContext, billingLane UsageBillingLane) *string {
	if billingLane != UsageBillingLanePlatform || bc == nil {
		return nil
	}
	return normalizedTextPtr(bc.DeductionID)
}

func normalizedAppUsagePair(appID *uuid.UUID, appType *string) (*uuid.UUID, *string) {
	if appID == nil || appType == nil {
		return nil, nil
	}
	trimmedType := strings.TrimSpace(*appType)
	if trimmedType == "" {
		return nil, nil
	}
	appIDCopy := *appID
	appTypeCopy := trimmedType
	return &appIDCopy, &appTypeCopy
}

func normalizedTextPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizedUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	uuidCopy := *value
	return &uuidCopy
}

func maxInt64(value, min int64) int64 {
	if value < min {
		return min
	}
	return value
}
