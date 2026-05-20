package llm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
)

type privateModelLookupRepo struct {
	records []*llmmodelmodel.CustomModel
}

func (r *privateModelLookupRepo) Create(context.Context, *llmmodelmodel.CustomModel) error {
	return nil
}

func (r *privateModelLookupRepo) GetByID(context.Context, uuid.UUID, uuid.UUID) (*llmmodelmodel.CustomModel, error) {
	return nil, nil
}

func (r *privateModelLookupRepo) GetByProviderAndName(context.Context, uuid.UUID, uuid.UUID, string) (*llmmodelmodel.CustomModel, error) {
	return nil, nil
}

func (r *privateModelLookupRepo) GetByProviderAndModel(context.Context, uuid.UUID, string, string) (*llmmodelmodel.CustomModel, error) {
	return nil, nil
}

func (r *privateModelLookupRepo) ListByNames(context.Context, uuid.UUID, []string, *bool) ([]*llmmodelmodel.CustomModel, error) {
	return r.records, nil
}

func (r *privateModelLookupRepo) List(context.Context, uuid.UUID, *uuid.UUID, string, string, *bool, int, int) ([]*llmmodelmodel.CustomModel, int64, error) {
	return r.records, int64(len(r.records)), nil
}

func (r *privateModelLookupRepo) Update(context.Context, *llmmodelmodel.CustomModel) error {
	return nil
}

func (r *privateModelLookupRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (r *privateModelLookupRepo) ListByProvider(context.Context, uuid.UUID, uuid.UUID) ([]*llmmodelmodel.CustomModel, error) {
	return nil, nil
}

func TestPrivateModelLookupAllowsDuplicateNamesForSameProvider(t *testing.T) {
	repo := &privateModelLookupRepo{
		records: []*llmmodelmodel.CustomModel{
			{Name: "all-minilm:latest", Provider: "ollama"},
			{Name: "all-minilm:latest", Provider: "ollama"},
		},
	}
	svc := llmmodelsvc.NewPrivateModelLookupService(repo)

	_, err := svc.ResolveActiveModels(context.Background(), uuid.New(), []string{"all-minilm:latest"})
	if err != nil {
		t.Fatalf("ResolveActiveModels() error = %v, want nil", err)
	}
}

func TestPrivateModelLookupRejectsDuplicateNamesForDifferentProviders(t *testing.T) {
	repo := &privateModelLookupRepo{
		records: []*llmmodelmodel.CustomModel{
			{Name: "embed-model", Provider: "ollama"},
			{Name: "embed-model", Provider: "openai-compatible"},
		},
	}
	svc := llmmodelsvc.NewPrivateModelLookupService(repo)

	_, err := svc.ResolveActiveModels(context.Background(), uuid.New(), []string{"embed-model"})
	if err == nil || !strings.Contains(err.Error(), "multiple custom providers") {
		t.Fatalf("ResolveActiveModels() error = %v, want multiple custom providers error", err)
	}
}

func TestPrivateModelLookupRejectsDuplicateNamesForSameProviderDifferentIDs(t *testing.T) {
	repo := &privateModelLookupRepo{
		records: []*llmmodelmodel.CustomModel{
			{Name: "qwen3.5:4b", Provider: "ollama", ProviderID: uuid.New()},
			{Name: "qwen3.5:4b", Provider: "ollama", ProviderID: uuid.New()},
		},
	}
	svc := llmmodelsvc.NewPrivateModelLookupService(repo)

	_, err := svc.ResolveActiveModels(context.Background(), uuid.New(), []string{"qwen3.5:4b"})
	if err == nil || !strings.Contains(err.Error(), "multiple custom providers") {
		t.Fatalf("ResolveActiveModels() error = %v, want multiple custom providers error", err)
	}
}

func TestPrivateModelLookupResolvesDuplicateNameForRequestedProvider(t *testing.T) {
	ollamaProviderID := uuid.New()
	customProviderID := uuid.New()
	repo := &privateModelLookupRepo{
		records: []*llmmodelmodel.CustomModel{
			{Name: "qwen3.5:9b", Provider: "ollama", ProviderID: ollamaProviderID},
			{Name: "qwen3.5:9b", Provider: "custom-1", ProviderID: customProviderID},
		},
	}
	svc := llmmodelsvc.NewPrivateModelLookupService(repo)

	model, err := svc.ResolveActiveModelForProvider(context.Background(), uuid.New(), "ollama", "qwen3.5:9b")
	if err != nil {
		t.Fatalf("ResolveActiveModelForProvider() error = %v, want nil", err)
	}
	if model == nil || model.ProviderID != ollamaProviderID {
		t.Fatalf("ResolveActiveModelForProvider() = %+v, want ollama provider id %s", model, ollamaProviderID)
	}
}

func TestPrivateModelLookupNameIndexesAllowDuplicateExactNames(t *testing.T) {
	repo := &privateModelLookupRepo{
		records: []*llmmodelmodel.CustomModel{
			{Name: "qwen3.5:9b", Provider: "ollama", ProviderID: uuid.New()},
			{Name: "qwen3.5:9b", Provider: "custom-1", ProviderID: uuid.New()},
		},
	}
	svc := llmmodelsvc.NewPrivateModelLookupService(repo)

	exactNames, _, err := svc.LoadActiveModelNameIndexes(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("LoadActiveModelNameIndexes() error = %v, want nil", err)
	}
	if len(exactNames) != 1 || exactNames[0] != "qwen3.5:9b" {
		t.Fatalf("LoadActiveModelNameIndexes() exactNames = %v, want [qwen3.5:9b]", exactNames)
	}
}
