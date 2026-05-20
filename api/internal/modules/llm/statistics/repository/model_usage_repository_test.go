package repository

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	"github.com/zgiai/ginext/internal/modules/llm/statistics/dto"
	"gorm.io/gorm"
)

func TestGetModelUsage_AggregatesSettledUsageBills(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:model_usage_repo?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&gateway.UsageBill{}))

	repo := NewStatisticsRepository(db)
	orgID := uuid.NewString()
	otherOrgID := uuid.NewString()

	model1 := uuid.New()
	model2 := uuid.New()
	provider1 := uuid.New()
	provider2 := uuid.New()
	workflowID := uuid.New()
	datasetID := uuid.New()
	workflowType := "workflow"
	datasetType := "dataset"
	day1 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	seedBills := []gateway.UsageBill{
		{
			AttemptID:         "attempt-1",
			RequestID:         "request-1",
			OrganizationID:    orgID,
			AppID:             &workflowID,
			AppType:           &workflowType,
			APIKeyID:          uuid.NewString(),
			QuotaSubjectType:  strPtr("workspace"),
			QuotaSubjectID:    strPtr("ws-1"),
			ModelID:           model1,
			ModelName:         "gpt-4o",
			ProviderID:        provider1,
			ProviderName:      "openai",
			UseSystemProvider: true,
			Status:            "success",
			PromptTokens:      5,
			CompletionTokens:  7,
			TotalTokens:       12,
			OfficialPoints:    10,
			PrivatePoints:     0,
			TotalPoints:       10,
			RequestCreatedAt:  day1,
			SettledAt:         day1.Add(2 * time.Second),
		},
		{
			AttemptID:         "attempt-2",
			RequestID:         "request-2",
			OrganizationID:    orgID,
			AppID:             &datasetID,
			AppType:           &datasetType,
			APIKeyID:          uuid.NewString(),
			QuotaSubjectType:  strPtr("workspace"),
			QuotaSubjectID:    strPtr("ws-2"),
			ModelID:           model1,
			ModelName:         "gpt-4o",
			ProviderID:        provider1,
			ProviderName:      "openai",
			UseSystemProvider: false,
			Status:            "success",
			PromptTokens:      2,
			CompletionTokens:  3,
			TotalTokens:       5,
			OfficialPoints:    0,
			PrivatePoints:     4,
			TotalPoints:       4,
			RequestCreatedAt:  day1.Add(5 * time.Minute),
			SettledAt:         day1.Add(6 * time.Minute),
		},
		{
			AttemptID:         "attempt-3",
			RequestID:         "request-3",
			OrganizationID:    orgID,
			APIKeyID:          uuid.NewString(),
			QuotaSubjectType:  strPtr("organization"),
			QuotaSubjectID:    strPtr(orgID),
			ModelID:           model2,
			ModelName:         "claude-3-7-sonnet",
			ProviderID:        provider2,
			ProviderName:      "anthropic",
			UseSystemProvider: true,
			Status:            "success",
			PromptTokens:      1,
			CompletionTokens:  1,
			TotalTokens:       2,
			OfficialPoints:    8,
			PrivatePoints:     0,
			TotalPoints:       8,
			RequestCreatedAt:  day2,
			SettledAt:         day2.Add(2 * time.Second),
		},
		{
			AttemptID:         "attempt-4",
			RequestID:         "request-4",
			OrganizationID:    orgID,
			AppID:             &workflowID,
			AppType:           &workflowType,
			APIKeyID:          uuid.NewString(),
			QuotaSubjectType:  strPtr("workspace"),
			QuotaSubjectID:    strPtr("ws-1"),
			ModelID:           model1,
			ModelName:         "gpt-4o",
			ProviderID:        provider1,
			ProviderName:      "openai",
			UseSystemProvider: true,
			Status:            "failed",
			RequestCreatedAt:  day2.Add(5 * time.Minute),
			SettledAt:         day2.Add(6 * time.Minute),
		},
		{
			AttemptID:         "attempt-other-org",
			RequestID:         "request-other-org",
			OrganizationID:    otherOrgID,
			APIKeyID:          uuid.NewString(),
			QuotaSubjectType:  strPtr("organization"),
			QuotaSubjectID:    strPtr(otherOrgID),
			ModelID:           model1,
			ModelName:         "gpt-4o",
			ProviderID:        provider1,
			ProviderName:      "openai",
			UseSystemProvider: true,
			Status:            "success",
			PromptTokens:      100,
			CompletionTokens:  100,
			TotalTokens:       200,
			OfficialPoints:    100,
			PrivatePoints:     0,
			TotalPoints:       100,
			RequestCreatedAt:  day1,
			SettledAt:         day1.Add(time.Second),
		},
	}
	require.NoError(t, db.Create(&seedBills).Error)

	resp, err := repo.GetModelUsage(context.Background(), orgID, &dto.ModelUsageRequest{
		StartTime: day1.Add(-time.Hour).Unix(),
		EndTime:   day2.Add(24 * time.Hour).Unix(),
	})
	require.NoError(t, err)

	require.Equal(t, int64(4), resp.Summary.AttemptCount)
	require.Equal(t, int64(3), resp.Summary.SuccessCount)
	require.Equal(t, int64(1), resp.Summary.FailedCount)
	require.Equal(t, int64(0), resp.Summary.PartialCount)
	require.Equal(t, int64(8), resp.Summary.PromptTokens)
	require.Equal(t, int64(11), resp.Summary.CompletionTokens)
	require.Equal(t, int64(19), resp.Summary.TotalTokens)
	require.Equal(t, int64(18), resp.Summary.OfficialPoints)
	require.Equal(t, int64(4), resp.Summary.PrivatePoints)
	require.Equal(t, int64(22), resp.Summary.TotalPoints)

	require.Len(t, resp.ByModel, 2)
	require.Equal(t, "gpt-4o", resp.ByModel[0].ModelName)
	require.Equal(t, int64(3), resp.ByModel[0].AttemptCount)
	require.Equal(t, int64(2), resp.ByModel[0].SuccessCount)
	require.Equal(t, int64(1), resp.ByModel[0].FailedCount)
	require.Equal(t, int64(14), resp.ByModel[0].TotalPoints)

	require.Len(t, resp.ByAppType, 3)
	require.Equal(t, "workflow", resp.ByAppType[0].AppType)
	require.Equal(t, int64(2), resp.ByAppType[0].AttemptCount)
	require.Equal(t, int64(10), resp.ByAppType[0].TotalPoints)
	require.Equal(t, "unknown", resp.ByAppType[1].AppType)
	require.Equal(t, int64(8), resp.ByAppType[1].TotalPoints)
	require.Equal(t, "dataset", resp.ByAppType[2].AppType)
	require.Equal(t, int64(4), resp.ByAppType[2].TotalPoints)

	require.Len(t, resp.DailyTrend, 2)
	require.Equal(t, "2026-04-01", resp.DailyTrend[0].Date)
	require.Equal(t, int64(2), resp.DailyTrend[0].AttemptCount)
	require.Equal(t, int64(14), resp.DailyTrend[0].TotalPoints)
	require.Equal(t, "2026-04-02", resp.DailyTrend[1].Date)
	require.Equal(t, int64(2), resp.DailyTrend[1].AttemptCount)
	require.Equal(t, int64(8), resp.DailyTrend[1].TotalPoints)

	platformLane := "platform"
	platformResp, err := repo.GetModelUsage(context.Background(), orgID, &dto.ModelUsageRequest{
		StartTime:   day1.Add(-time.Hour).Unix(),
		EndTime:     day2.Add(24 * time.Hour).Unix(),
		BillingLane: &platformLane,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), platformResp.Summary.AttemptCount)
	require.Equal(t, int64(18), platformResp.Summary.OfficialPoints)
	require.Equal(t, int64(0), platformResp.Summary.PrivatePoints)

	privateLane := "private"
	privateResp, err := repo.GetModelUsage(context.Background(), orgID, &dto.ModelUsageRequest{
		StartTime:   day1.Add(-time.Hour).Unix(),
		EndTime:     day2.Add(24 * time.Hour).Unix(),
		BillingLane: &privateLane,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), privateResp.Summary.AttemptCount)
	require.Equal(t, int64(0), privateResp.Summary.OfficialPoints)
	require.Equal(t, int64(4), privateResp.Summary.PrivatePoints)
}

func TestGetModelUsage_IncludesCustomModelFromUsageBills(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:model_usage_custom_"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&gateway.UsageBill{}))

	repo := NewStatisticsRepository(db)
	orgID := uuid.NewString()
	modelID := uuid.New()
	providerID := uuid.New()
	now := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)

	require.NoError(t, db.Create(&gateway.UsageBill{
		AttemptID:         "attempt-custom-ollama",
		RequestID:         "request-custom-ollama",
		OrganizationID:    orgID,
		APIKeyID:          uuid.NewString(),
		QuotaSubjectType:  strPtr("api_key"),
		QuotaSubjectID:    strPtr("key-1"),
		ModelID:           modelID,
		ModelName:         "qwen3.5:4b",
		ProviderID:        providerID,
		ProviderName:      "ollama",
		UseSystemProvider: false,
		Status:            "success",
		PromptTokens:      18,
		CompletionTokens:  12,
		TotalTokens:       30,
		OfficialPoints:    0,
		PrivatePoints:     7,
		TotalPoints:       7,
		RequestCreatedAt:  now,
		SettledAt:         now.Add(time.Second),
	}).Error)

	resp, err := repo.GetModelUsage(context.Background(), orgID, &dto.ModelUsageRequest{
		StartTime: now.Add(-time.Hour).Unix(),
		EndTime:   now.Add(time.Hour).Unix(),
	})
	require.NoError(t, err)

	require.Equal(t, int64(7), resp.Summary.PrivatePoints)
	require.Equal(t, int64(7), resp.Summary.TotalPoints)
	require.Len(t, resp.ByModel, 1)
	require.Equal(t, modelID, resp.ByModel[0].ModelID)
	require.Equal(t, "qwen3.5:4b", resp.ByModel[0].ModelName)
	require.Equal(t, providerID, resp.ByModel[0].ProviderID)
	require.Equal(t, "ollama", resp.ByModel[0].ProviderName)
	require.Equal(t, int64(7), resp.ByModel[0].PrivatePoints)
}

func TestGetModelUsage_FiltersAIChatAppType(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:model_usage_aichat_"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&gateway.UsageBill{}))

	repo := NewStatisticsRepository(db)
	orgID := uuid.NewString()
	modelID := uuid.New()
	providerID := uuid.New()
	appID := uuid.New()
	aichatType := "aichat"
	workflowType := "workflow"
	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)

	require.NoError(t, db.Create([]gateway.UsageBill{
		{
			AttemptID:         "attempt-aichat",
			RequestID:         "request-aichat",
			OrganizationID:    orgID,
			AppID:             &appID,
			AppType:           &aichatType,
			APIKeyID:          uuid.NewString(),
			QuotaSubjectType:  strPtr("organization"),
			QuotaSubjectID:    strPtr(orgID),
			ModelID:           modelID,
			ModelName:         "gpt-4o-mini",
			ProviderID:        providerID,
			ProviderName:      "openai",
			UseSystemProvider: true,
			Status:            "success",
			PromptTokens:      3,
			CompletionTokens:  4,
			TotalTokens:       7,
			OfficialPoints:    6,
			TotalPoints:       6,
			RequestCreatedAt:  now,
			SettledAt:         now.Add(time.Second),
		},
		{
			AttemptID:         "attempt-workflow",
			RequestID:         "request-workflow",
			OrganizationID:    orgID,
			AppType:           &workflowType,
			APIKeyID:          uuid.NewString(),
			QuotaSubjectType:  strPtr("organization"),
			QuotaSubjectID:    strPtr(orgID),
			ModelID:           modelID,
			ModelName:         "gpt-4o-mini",
			ProviderID:        providerID,
			ProviderName:      "openai",
			UseSystemProvider: true,
			Status:            "success",
			PromptTokens:      10,
			CompletionTokens:  10,
			TotalTokens:       20,
			OfficialPoints:    20,
			TotalPoints:       20,
			RequestCreatedAt:  now,
			SettledAt:         now.Add(time.Second),
		},
	}).Error)

	resp, err := repo.GetModelUsage(context.Background(), orgID, &dto.ModelUsageRequest{
		StartTime: now.Add(-time.Hour).Unix(),
		EndTime:   now.Add(time.Hour).Unix(),
		AppType:   &aichatType,
	})
	require.NoError(t, err)

	require.Equal(t, int64(1), resp.Summary.AttemptCount)
	require.Equal(t, int64(7), resp.Summary.TotalTokens)
	require.Equal(t, int64(6), resp.Summary.TotalPoints)
	require.Len(t, resp.ByAppType, 1)
	require.Equal(t, "aichat", resp.ByAppType[0].AppType)
	require.Equal(t, int64(6), resp.ByAppType[0].TotalPoints)
}

func strPtr(value string) *string {
	return &value
}
