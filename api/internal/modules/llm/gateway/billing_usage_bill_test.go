package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBillingService_SettleWritesUsageBill_PrivateSuccessAndFailed(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-private-usage-bill"
	seedAPIKey(t, db, keyID, orgID.String())
	seedPrivateWallet(t, db, channelID, orgID, 20, channelWalletStatusActive)

	svc := &BillingService{db: db}
	modelID := uuid.New()
	providerID := uuid.New()

	successStartedAt := time.Now().UTC().Add(-2 * time.Minute)
	successCtx := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-private-success",
		AttemptID:         "req-private-success-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ModelID:           modelID,
		ModelName:         "gpt-4o-mini",
		ProviderID:        providerID,
		ProviderName:      "openai",
		RouteID:           &channelID,
		ChannelID:         &channelID,
		EstimatedCredits:  5,
		ActualCredits:     4,
		PromptTokens:      10,
		CompletionTokens:  6,
		TotalTokens:       16,
		UseSystemProvider: false,
		Status:            "success",
		RequestCreatedAt:  successStartedAt,
	}

	require.NoError(t, svc.PreDeduct(context.Background(), successCtx))
	require.NoError(t, svc.Settle(context.Background(), successCtx))

	var successBill UsageBill
	require.NoError(t, db.Where("attempt_id = ?", successCtx.AttemptID).First(&successBill).Error)
	require.Equal(t, usageBillStatusSuccess, successBill.Status)
	require.Equal(t, UsageBillingLanePrivate, successBill.BillingLane)
	require.Nil(t, successBill.RemoteDeductionID)
	require.Equal(t, int64(0), successBill.OfficialPoints)
	require.Equal(t, int64(4), successBill.PrivatePoints)
	require.Equal(t, int64(4), successBill.TotalPoints)
	require.Equal(t, int64(16), successBill.TotalTokens)
	require.WithinDuration(t, successStartedAt, successBill.RequestCreatedAt, time.Second)

	failedStartedAt := time.Now().UTC().Add(-time.Minute)
	failedCtx := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-private-failed",
		AttemptID:         "req-private-failed-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ModelID:           modelID,
		ModelName:         "gpt-4o-mini",
		ProviderID:        providerID,
		ProviderName:      "openai",
		RouteID:           &channelID,
		ChannelID:         &channelID,
		EstimatedCredits:  3,
		UseSystemProvider: false,
		Status:            "error",
		ErrorMessage:      "provider timeout",
		RequestCreatedAt:  failedStartedAt,
	}

	require.NoError(t, svc.PreDeduct(context.Background(), failedCtx))
	require.NoError(t, svc.Settle(context.Background(), failedCtx))

	var failedBill UsageBill
	require.NoError(t, db.Where("attempt_id = ?", failedCtx.AttemptID).First(&failedBill).Error)
	require.Equal(t, usageBillStatusFailed, failedBill.Status)
	require.Equal(t, int64(0), failedBill.OfficialPoints)
	require.Equal(t, int64(0), failedBill.PrivatePoints)
	require.Equal(t, int64(0), failedBill.TotalPoints)
	require.Equal(t, int64(0), failedBill.TotalTokens)
	require.NotNil(t, failedBill.ErrorMessage)
	require.Equal(t, "provider timeout", *failedBill.ErrorMessage)
}

func TestBillingService_SettleWritesUsageBill_CustomOllamaPrivateSuccess(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-custom-ollama-usage-bill"
	seedAPIKey(t, db, keyID, orgID.String())
	seedPrivateWallet(t, db, channelID, orgID, 20, channelWalletStatusActive)

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-custom-ollama-success",
		AttemptID:         "req-custom-ollama-success-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ModelID:           uuid.New(),
		ModelName:         "qwen3.5:4b",
		ProviderID:        uuid.New(),
		ProviderName:      "ollama",
		RouteID:           &channelID,
		ChannelID:         &channelID,
		EstimatedCredits:  5,
		ActualCredits:     3,
		PromptTokens:      18,
		CompletionTokens:  12,
		TotalTokens:       30,
		UseSystemProvider: false,
		Status:            "success",
		RequestCreatedAt:  time.Now().UTC(),
	}

	require.NoError(t, svc.PreDeduct(context.Background(), bc))
	require.NoError(t, svc.Settle(context.Background(), bc))

	var bill UsageBill
	require.NoError(t, db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error)
	require.False(t, bill.UseSystemProvider)
	require.Equal(t, UsageBillingLanePrivate, bill.BillingLane)
	require.Equal(t, "qwen3.5:4b", bill.ModelName)
	require.Equal(t, "ollama", bill.ProviderName)
	require.Equal(t, int64(0), bill.OfficialPoints)
	require.Equal(t, int64(3), bill.PrivatePoints)
	require.Equal(t, int64(3), bill.TotalPoints)
	require.Equal(t, int64(30), bill.TotalTokens)
}

func TestBillingService_BuildUsageBill_OfficialSuccess(t *testing.T) {
	svc := &BillingService{}
	startedAt := time.Now().UTC().Add(-time.Minute)
	bc := &BillingContext{
		APIKeyID:          uuid.NewString(),
		OrganizationID:    uuid.NewString(),
		RequestID:         "req-official-success",
		AttemptID:         "req-official-success-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    uuid.NewString(),
		ModelID:           uuid.New(),
		ModelName:         "gpt-4o",
		ProviderID:        uuid.New(),
		ProviderName:      "openai",
		EstimatedCredits:  6,
		ActualCredits:     6,
		PromptTokens:      12,
		CompletionTokens:  8,
		TotalTokens:       20,
		BillingLane:       UsageBillingLanePlatform,
		UseSystemProvider: true,
		DeductionID:       "deduction-official-success",
		Status:            "success",
		RequestCreatedAt:  startedAt,
	}

	bill, err := svc.buildUsageBill(bc, usageBillStatusSuccess, nil, nil)
	require.NoError(t, err)
	require.Equal(t, usageBillStatusSuccess, bill.Status)
	require.Equal(t, UsageBillingLanePlatform, bill.BillingLane)
	require.True(t, bill.UseSystemProvider)
	require.NotNil(t, bill.RemoteDeductionID)
	require.Equal(t, "deduction-official-success", *bill.RemoteDeductionID)
	require.Equal(t, int64(6), bill.OfficialPoints)
	require.Equal(t, int64(0), bill.PrivatePoints)
	require.Equal(t, int64(20), bill.TotalTokens)
	require.WithinDuration(t, startedAt, bill.RequestCreatedAt, time.Second)
}
