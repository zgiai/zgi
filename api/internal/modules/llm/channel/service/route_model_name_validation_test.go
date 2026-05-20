package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	channeldto "github.com/zgiai/zgi/api/internal/modules/llm/channel/dto"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

func TestCreateRoute_RejectsLegacyShortModelNameWithFullNameHint(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo: &fakeModelRepo{
			models: []*llmmodelmodel.LLMModel{
				{Model: "ByteDance-Seed/Seed-OSS-36B-Instruct", IsActive: true},
			},
		},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "SiliconFlow Seed",
		ChannelProvider: "openai-compatible",
		APIKey:          "test-key",
		APIBaseURL:      "https://api.siliconflow.cn/v1",
		Models:          []string{"Seed-OSS-36B-Instruct"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "must use the full model name")
	require.Contains(t, err.Error(), "ByteDance-Seed/Seed-OSS-36B-Instruct")
	require.Nil(t, repo.created)
	require.Equal(t, 0, validator.createCalls)
	require.Nil(t, credSvc.createdReq)
}

func TestUpdateRoute_RejectsLegacyShortModelNameWithFullNameHint(t *testing.T) {
	credID := uuid.New()
	repo := &fakeTenantRouteRepo{
		routeByID: &channelmodel.LLMRoute{
			ID:              uuid.New(),
			OrganizationID:  uuid.New(),
			Type:            "PRIVATE",
			CredentialID:    &credID,
			Name:            "Existing Route",
			ChannelProvider: "openai-compatible",
			APIBaseURL:      "https://api.siliconflow.cn/v1",
			Models:          []string{"ByteDance-Seed/Seed-OSS-36B-Instruct"},
			IsEnabled:       true,
		},
	}
	credSvc := &fakeTenantCredentialService{
		cred: &credentialmodel.TenantCredential{ID: credID},
	}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo: &fakeModelRepo{
			models: []*llmmodelmodel.LLMModel{
				{Model: "ByteDance-Seed/Seed-OSS-36B-Instruct", IsActive: true},
			},
		},
	}

	_, err := svc.UpdateRoute(context.Background(), repo.routeByID.OrganizationID, repo.routeByID.ID, &channeldto.UpdateRouteRequest{
		Models: []string{"Seed-OSS-36B-Instruct"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "must use the full model name")
	require.Contains(t, err.Error(), "ByteDance-Seed/Seed-OSS-36B-Instruct")
	require.Nil(t, repo.updated)
	require.Equal(t, 0, validator.createCalls)
}
