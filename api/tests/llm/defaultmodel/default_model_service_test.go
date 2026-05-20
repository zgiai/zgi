package defaultmodel_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	defaultmodelmodel "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/model"
	defaultmodelrepo "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/repository"
	defaultmodelservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	llmmodelrepo "github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
	llmmodelservice "github.com/zgiai/ginext/internal/modules/llm/llmmodel/service"
	llmsharedtypes "github.com/zgiai/ginext/internal/modules/llm/shared/types"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"gorm.io/gorm"
)

type stubDefaultModelRepo struct {
	getItem   *defaultmodelmodel.DefaultModel
	getErr    error
	upserted  *defaultmodelmodel.DefaultModel
	deleted   []string
	listItems []*defaultmodelmodel.DefaultModel
}

func (s *stubDefaultModelRepo) ListByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*defaultmodelmodel.DefaultModel, error) {
	return s.listItems, nil
}

func (s *stubDefaultModelRepo) GetByOrganizationAndUseCase(ctx context.Context, organizationID uuid.UUID, useCase string) (*defaultmodelmodel.DefaultModel, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getItem, nil
}

func (s *stubDefaultModelRepo) Upsert(ctx context.Context, item *defaultmodelmodel.DefaultModel) error {
	cloned := *item
	s.upserted = &cloned
	return nil
}

func (s *stubDefaultModelRepo) DeleteByOrganizationAndUseCase(ctx context.Context, organizationID uuid.UUID, useCase string) error {
	s.deleted = append(s.deleted, useCase)
	return nil
}

type stubAvailableModelsService struct {
	models []*llmmodelservice.AvailableModel
}

func (s *stubAvailableModelsService) ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*llmmodelservice.AvailableModel, error) {
	filtered := make([]*llmmodelservice.AvailableModel, 0, len(s.models))
	for _, item := range s.models {
		if item == nil {
			continue
		}
		if provider != "" && item.Provider != provider {
			continue
		}
		if useCase != "" {
			matched := false
			for _, candidate := range item.UseCases {
				if candidate == useCase {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (s *stubAvailableModelsService) RefreshCache(ctx context.Context, organizationID uuid.UUID) error {
	return nil
}

func (s *stubAvailableModelsService) InvalidateTenantCache(organizationID uuid.UUID) {}

func (s *stubAvailableModelsService) InvalidateGlobalCache() {}

func (s *stubAvailableModelsService) SetOfficialRouteBootstrapper(bootstrapper interfaces.OfficialRouteBootstrapper) {
}

type stubModelRepo struct {
	list []*llmmodelmodel.LLMModel
}

func (s *stubModelRepo) Create(ctx context.Context, m *llmmodelmodel.LLMModel) error { return nil }
func (s *stubModelRepo) GetByID(ctx context.Context, id uuid.UUID) (*llmmodelmodel.LLMModel, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *stubModelRepo) GetByName(ctx context.Context, name string) (*llmmodelmodel.LLMModel, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *stubModelRepo) ListByNames(ctx context.Context, names []string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, nil
}
func (s *stubModelRepo) ListAvailableByNames(ctx context.Context, names []string, provider string, useCase string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, nil
}
func (s *stubModelRepo) ListAvailableFiltered(ctx context.Context, provider string, useCase string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, nil
}
func (s *stubModelRepo) GetByProviderAndName(ctx context.Context, provider string, name string) (*llmmodelmodel.LLMModel, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *stubModelRepo) List(ctx context.Context, providerID *uuid.UUID, provider string, useCase string, isActive *bool, offset, limit int) ([]*llmmodelmodel.LLMModel, int64, error) {
	return s.list, int64(len(s.list)), nil
}
func (s *stubModelRepo) Update(ctx context.Context, m *llmmodelmodel.LLMModel) error { return nil }
func (s *stubModelRepo) Delete(ctx context.Context, id uuid.UUID) error              { return nil }
func (s *stubModelRepo) ListByProvider(ctx context.Context, providerID string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, nil
}

type stubCustomModelRepo struct {
	list []*llmmodelmodel.CustomModel
}

func (s *stubCustomModelRepo) Create(ctx context.Context, m *llmmodelmodel.CustomModel) error {
	return nil
}
func (s *stubCustomModelRepo) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*llmmodelmodel.CustomModel, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *stubCustomModelRepo) GetByProviderAndName(ctx context.Context, organizationID, providerID uuid.UUID, name string) (*llmmodelmodel.CustomModel, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *stubCustomModelRepo) GetByProviderAndModel(ctx context.Context, organizationID uuid.UUID, provider string, name string) (*llmmodelmodel.CustomModel, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *stubCustomModelRepo) ListByNames(ctx context.Context, organizationID uuid.UUID, names []string, isActive *bool) ([]*llmmodelmodel.CustomModel, error) {
	return nil, nil
}
func (s *stubCustomModelRepo) List(ctx context.Context, organizationID uuid.UUID, providerID *uuid.UUID, provider string, useCase string, isActive *bool, offset, limit int) ([]*llmmodelmodel.CustomModel, int64, error) {
	return s.list, int64(len(s.list)), nil
}
func (s *stubCustomModelRepo) Update(ctx context.Context, m *llmmodelmodel.CustomModel) error {
	return nil
}
func (s *stubCustomModelRepo) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return nil
}
func (s *stubCustomModelRepo) ListByProvider(ctx context.Context, organizationID, providerID uuid.UUID) ([]*llmmodelmodel.CustomModel, error) {
	return nil, nil
}

var _ defaultmodelrepo.DefaultModelRepository = (*stubDefaultModelRepo)(nil)
var _ llmmodelservice.AvailableModelsService = (*stubAvailableModelsService)(nil)
var _ llmmodelrepo.ModelRepository = (*stubModelRepo)(nil)
var _ llmmodelrepo.CustomModelRepository = (*stubCustomModelRepo)(nil)

func TestResolveUseCaseReturnsExplicitParams(t *testing.T) {
	organizationID := uuid.New()
	repo := &stubDefaultModelRepo{
		getItem: &defaultmodelmodel.DefaultModel{
			OrganizationID: organizationID,
			UseCase:        string(llmmodelmodel.UseCaseTextChat),
			Provider:       "openai",
			Model:          "gpt-4o-mini",
			Params: llmsharedtypes.JSONObject{
				"temperature": 0.2,
			},
		},
	}
	available := &stubAvailableModelsService{
		models: []*llmmodelservice.AvailableModel{
			{
				Provider: "openai",
				Name:     "gpt-4o-mini",
				UseCases: []string{string(llmmodelmodel.UseCaseTextChat)},
			},
		},
	}
	globalRepo := &stubModelRepo{}
	customRepo := &stubCustomModelRepo{}
	service := defaultmodelservice.NewDefaultModelService(repo, available, globalRepo, customRepo)

	resolved, err := service.ResolveUseCase(context.Background(), organizationID.String(), llmmodelmodel.UseCaseTextChat, nil, nil)
	if err != nil {
		t.Fatalf("ResolveUseCase returned error: %v", err)
	}
	if resolved.Source != defaultmodelservice.SourceExplicit {
		t.Fatalf("expected explicit source, got %q", resolved.Source)
	}
	if resolved.Provider != "openai" || resolved.Model != "gpt-4o-mini" {
		t.Fatalf("unexpected resolved model: %+v", resolved)
	}
	if resolved.Params["temperature"] != 0.2 {
		t.Fatalf("expected explicit params to be preserved, got %+v", resolved.Params)
	}
}

func TestResolveUseCaseAutoPicksRecommendedModel(t *testing.T) {
	organizationID := uuid.New()
	repo := &stubDefaultModelRepo{getErr: gorm.ErrRecordNotFound}
	available := &stubAvailableModelsService{
		models: []*llmmodelservice.AvailableModel{
			{
				Provider: "openai",
				Name:     "gpt-4.1-mini",
				UseCases: []string{string(llmmodelmodel.UseCaseTextChat)},
			},
			{
				Provider: "openai",
				Name:     "gpt-4.1",
				UseCases: []string{string(llmmodelmodel.UseCaseTextChat)},
			},
		},
	}
	globalRepo := &stubModelRepo{
		list: []*llmmodelmodel.LLMModel{
			{Provider: "openai", Model: "gpt-4.1-mini", SortOrder: 20},
			{Provider: "openai", Model: "gpt-4.1", IsRecommended: true, SortOrder: 30},
		},
	}
	customRepo := &stubCustomModelRepo{}
	service := defaultmodelservice.NewDefaultModelService(repo, available, globalRepo, customRepo)

	resolved, err := service.ResolveUseCase(context.Background(), organizationID.String(), llmmodelmodel.UseCaseTextChat, nil, nil)
	if err != nil {
		t.Fatalf("ResolveUseCase returned error: %v", err)
	}
	if resolved.Source != defaultmodelservice.SourceAuto {
		t.Fatalf("expected auto source, got %q", resolved.Source)
	}
	if resolved.Model != "gpt-4.1" {
		t.Fatalf("expected recommended model gpt-4.1, got %q", resolved.Model)
	}
	if len(resolved.Params) != 0 {
		t.Fatalf("expected auto params to be empty object, got %+v", resolved.Params)
	}
}

func TestUpsertNormalizesNilParams(t *testing.T) {
	organizationID := uuid.New()
	actorID := uuid.New()
	repo := &stubDefaultModelRepo{}
	available := &stubAvailableModelsService{
		models: []*llmmodelservice.AvailableModel{
			{
				Provider: "openai",
				Name:     "text-embedding-3-large",
				UseCases: []string{string(llmmodelmodel.UseCaseEmbedding)},
			},
		},
	}
	service := defaultmodelservice.NewDefaultModelService(repo, available, &stubModelRepo{}, &stubCustomModelRepo{})

	item, err := service.Upsert(
		context.Background(),
		organizationID,
		&actorID,
		llmmodelmodel.UseCaseEmbedding,
		"openai",
		"text-embedding-3-large",
		nil,
	)
	if err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	if item == nil || item.Params == nil {
		t.Fatalf("expected upserted item params to be initialized, got %+v", item)
	}
	if repo.upserted == nil || repo.upserted.Params == nil {
		t.Fatalf("expected repository to receive normalized params, got %+v", repo.upserted)
	}
	if len(repo.upserted.Params) != 0 {
		t.Fatalf("expected params to default to empty object, got %+v", repo.upserted.Params)
	}
}
