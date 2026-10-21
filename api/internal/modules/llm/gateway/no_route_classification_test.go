package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// A catalog model that no enabled route serves has no provider configured for
// it. Pre published it as llm.provider.unavailable ("temporarily
// unavailable") while Console answers the same case llm.provider.none_available.
func TestCatalogModelWithoutRouteIsNoProviderAvailable(t *testing.T) {
	router := &ChannelRouter{strategyFactory: NewStrategyFactory()}
	routes := []*channelmodel.LLMRoute{{
		ID:         uuid.New(),
		Type:       shared.RouteTypeZGICloud,
		Models:     []string{"some-other-model"},
		IsEnabled:  true,
		IsOfficial: true,
	}}
	model := &llmmodel.LLMModel{Model: "qa-nochannel-model", Provider: "openai", ChatCompletions: true}

	_, err := router.filterCandidateRoutes(context.Background(), routes, model.Model, model.Provider, false, model, false)
	if !errors.Is(err, ErrNoProviderAvailable) {
		t.Fatalf("err = %v, want ErrNoProviderAvailable", err)
	}
	if code, _ := ApplicationCodeForError(err); code != llmerrors.AppCodeNoProviderAvailable {
		t.Fatalf("application code = %s, want %s", code, llmerrors.AppCodeNoProviderAvailable)
	}
}
