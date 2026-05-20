package gateway

import (
	"testing"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// TestTenantAutonomyStrategy_SupportsModel tests the TenantAutonomyStrategy implementation
func TestTenantAutonomyStrategy_SupportsModel(t *testing.T) {
	strategy := NewTenantAutonomyStrategy()

	tests := []struct {
		name          string
		route         *channelmodel.LLMRoute
		modelName     string
		modelProvider string
		want          bool
	}{
		{
			name: "exact_model_match_with_matching_provider",
			route: &channelmodel.LLMRoute{
				ID:              uuid.New(),
				ChannelProvider: "openai",
				Models:          []string{"gpt-4", "gpt-3.5-turbo"},
			},
			modelName:     "gpt-4",
			modelProvider: "openai",
			want:          true,
		},
		{
			name: "wildcard_support_allows_any_model",
			route: &channelmodel.LLMRoute{
				ID:              uuid.New(),
				ChannelProvider: "openai",
				Models:          []string{"*"},
			},
			modelName:     "any-model-name",
			modelProvider: "openai",
			want:          true,
		},
		{
			name: "provider_mismatch_but_model_list_contains_model_still_matches",
			route: &channelmodel.LLMRoute{
				ID:              uuid.New(),
				ChannelProvider: "anthropic",
				Models:          []string{"claude-3-opus", "claude-3-sonnet"},
			},
			modelName:     "claude-3-opus",
			modelProvider: "openai",
			want:          true,
		},
		{
			name: "empty_models_list_is_misconfiguration",
			route: &channelmodel.LLMRoute{
				ID:              uuid.New(),
				ChannelProvider: "openai",
				Models:          []string{},
			},
			modelName:     "gpt-4",
			modelProvider: "openai",
			want:          false,
		},
		{
			name: "model_not_in_list_returns_false",
			route: &channelmodel.LLMRoute{
				ID:              uuid.New(),
				ChannelProvider: "openai",
				Models:          []string{"gpt-3.5-turbo"},
			},
			modelName:     "gpt-4",
			modelProvider: "openai",
			want:          false,
		},
		{
			name: "nil_models_list_treated_as_empty",
			route: &channelmodel.LLMRoute{
				ID:              uuid.New(),
				ChannelProvider: "openai",
				Models:          nil,
			},
			modelName:     "gpt-4",
			modelProvider: "openai",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strategy.SupportsModel(tt.route, tt.modelName, tt.modelProvider)
			if got != tt.want {
				t.Errorf("TenantAutonomyStrategy.SupportsModel() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStrategyFactory_GetStrategy tests the strategy factory's route selection logic
func TestStrategyFactory_GetStrategy(t *testing.T) {
	factory := NewStrategyFactory()

	tests := []struct {
		name         string
		route        *channelmodel.LLMRoute
		expectedType string // always "TenantAutonomy"
	}{
		{
			name: "user_owned_route_returns_tenant_strategy",
			route: &channelmodel.LLMRoute{
				Type: shared.RouteTypePrivate,
			},
			expectedType: "TenantAutonomy",
		},
		{
			name: "system_ref_route_returns_tenant_strategy",
			route: &channelmodel.LLMRoute{
				Type: shared.RouteTypeZGICloud,
			},
			expectedType: "TenantAutonomy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := factory.GetStrategy(tt.route)
			if strategy == nil {
				t.Fatal("GetStrategy() returned nil")
			}
			if strategy.GetStrategyName() != tt.expectedType {
				t.Errorf("GetStrategy() returned strategy %s, want %s",
					strategy.GetStrategyName(), tt.expectedType)
			}
		})
	}
}

// TestTenantAutonomyStrategy_GetMethods tests getter methods
func TestTenantAutonomyStrategy_GetMethods(t *testing.T) {
	strategy := NewTenantAutonomyStrategy()

	t.Run("GetStrategyName", func(t *testing.T) {
		if name := strategy.GetStrategyName(); name != "TenantAutonomy" {
			t.Errorf("GetStrategyName() = %s, want TenantAutonomy", name)
		}
	})

	t.Run("GetModelList", func(t *testing.T) {
		route := &channelmodel.LLMRoute{
			Models: []string{"gpt-4", "gpt-3.5-turbo"},
		}
		models := strategy.GetModelList(route)
		if len(models) != 2 {
			t.Errorf("GetModelList() returned %d models, want 2", len(models))
		}
	})

	t.Run("GetProvider", func(t *testing.T) {
		route := &channelmodel.LLMRoute{
			ChannelProvider: "openai",
		}
		provider := strategy.GetProvider(route)
		if provider != "openai" {
			t.Errorf("GetProvider() = %s, want openai", provider)
		}
	})
}
