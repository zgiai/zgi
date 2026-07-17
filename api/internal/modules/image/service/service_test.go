package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type fakeAvailableModels struct {
	items []*llmmodelsvc.AvailableModel
}

func (f *fakeAvailableModels) ListAvailable(context.Context, uuid.UUID, string, string) ([]*llmmodelsvc.AvailableModel, error) {
	return f.items, nil
}

func (f *fakeAvailableModels) RefreshCache(context.Context, uuid.UUID) error { return nil }
func (f *fakeAvailableModels) InvalidateTenantCache(uuid.UUID)               {}
func (f *fakeAvailableModels) InvalidateGlobalCache()                        {}
func (f *fakeAvailableModels) SetOfficialRouteBootstrapper(interfaces.OfficialRouteBootstrapper) {
}

type fakeRouteLister struct {
	routes map[string][]*channelmodel.RouteQueryResult
}

func (f fakeRouteLister) GetRoutesForModel(_ context.Context, _ uuid.UUID, modelName string) ([]*channelmodel.RouteQueryResult, error) {
	if f.routes != nil {
		return f.routes[modelName], nil
	}
	return []*channelmodel.RouteQueryResult{{RouteID: uuid.New()}}, nil
}

func TestListModelsReturnsRegisteredAvailableImageModels(t *testing.T) {
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{
			{Provider: "openai", Name: "gpt-image-2"},
			{Provider: "qwen", Name: "qwen-image"},
			{Provider: "qwen", Name: "qwen-image-2.0"},
		}},
		fakeRouteLister{},
		nil,
		nil,
		nil,
	)

	models, err := svc.ListModels(context.Background(), Scope{OrganizationID: uuid.New()})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	want := map[string]bool{
		"openai/gpt-image-2":  false,
		"qwen/qwen-image":     false,
		"qwen/qwen-image-2.0": false,
	}
	for _, model := range models {
		key := model.Provider + "/" + model.Model
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("ListModels missing %s in %#v", key, models)
		}
	}
}

func TestListModelsIncludesRegisteredModelWhenOnlyRouteIsAvailable(t *testing.T) {
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{
			{Provider: "openai", Name: "gpt-image-2"},
			{Provider: "qwen", Name: "qwen-image-2.0"},
		}},
		fakeRouteLister{routes: map[string][]*channelmodel.RouteQueryResult{
			"qwen-image": {{RouteID: uuid.New()}},
		}},
		nil,
		nil,
		nil,
	)

	models, err := svc.ListModels(context.Background(), Scope{OrganizationID: uuid.New()})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}
	for _, model := range models {
		if model.Provider == "qwen" && model.Model == "qwen-image" {
			return
		}
	}
	t.Fatalf("ListModels missing qwen/qwen-image in %#v", models)
}

func TestImageResponseFormatOmitsURLForOpenAIGPTImage(t *testing.T) {
	got := imageResponseFormat(registry.ImageModel{Provider: "openai", Model: "gpt-image-2"})
	if got != "" {
		t.Fatalf("imageResponseFormat() = %q, want empty for OpenAI GPT image models", got)
	}
}

func TestImageResponseFormatUsesURLForOtherImageModels(t *testing.T) {
	got := imageResponseFormat(registry.ImageModel{Provider: "qwen", Model: "qwen-image"})
	if got != "url" {
		t.Fatalf("imageResponseFormat() = %q, want url", got)
	}
}
