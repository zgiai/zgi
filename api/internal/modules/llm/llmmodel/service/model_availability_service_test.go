package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	providermodel "github.com/zgiai/ginext/internal/modules/llm/provider/model"
)

func TestModelAvailabilityService_DisabledProviderMakesModelUnavailable(t *testing.T) {
	orgID := uuid.New()
	providerID := uuid.New()
	modelID := uuid.New()

	globalModel := &model.LLMModel{
		ID:       modelID,
		Provider: "openai",
		Model:    "gpt-4o",
		IsActive: true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{{
			ID:              uuid.New(),
			OrganizationID:  orgID,
			ChannelProvider: "deepseek",
			Models:          []string{"gpt-4o"},
			IsEnabled:       true,
		}},
	}

	svc := NewModelAvailabilityServiceWithProviderRepos(
		&fakeGlobalRepo{models: []*model.LLMModel{globalModel}},
		&fakeModelConfigRepo{},
		routeRepo,
		&fakeProviderRepo{
			providers: []*providermodel.LLMProvider{{
				ID:       providerID,
				Provider: "openai",
				IsActive: true,
			}},
		},
		&fakeProviderConfigRepo{
			configs: []*providermodel.ProviderConfig{{
				OrganizationID: orgID,
				ProviderID:     providerID,
				IsEnabled:      false,
			}},
		},
	)

	result, err := svc.CheckModelAvailable(context.Background(), orgID, modelID)
	require.NoError(t, err)
	require.False(t, result.Available)
	require.Equal(t, 0, result.ChannelCount)
}
