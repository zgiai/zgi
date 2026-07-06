package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/availability/dto"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"gorm.io/gorm"
)

type availabilityModelRepoFake struct {
	model *llmmodel.LLMModel
	err   error
}

func (f *availabilityModelRepoFake) Create(context.Context, *llmmodel.LLMModel) error {
	return errors.New("not implemented")
}
func (f *availabilityModelRepoFake) GetByID(context.Context, uuid.UUID) (*llmmodel.LLMModel, error) {
	return f.model, f.err
}
func (f *availabilityModelRepoFake) GetByName(context.Context, string) (*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityModelRepoFake) ListByNames(context.Context, []string) ([]*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityModelRepoFake) ListAvailableByNames(context.Context, []string, string, string) ([]*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityModelRepoFake) ListAvailableFiltered(context.Context, string, string) ([]*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityModelRepoFake) GetByProviderAndName(context.Context, string, string) (*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityModelRepoFake) List(context.Context, *uuid.UUID, string, string, string, *bool, int, int) ([]*llmmodel.LLMModel, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *availabilityModelRepoFake) Update(context.Context, *llmmodel.LLMModel) error {
	return errors.New("not implemented")
}
func (f *availabilityModelRepoFake) Delete(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *availabilityModelRepoFake) ListByProvider(context.Context, string) ([]*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}

type availabilityConfigRepoFake struct {
	config *llmmodel.ModelConfig
	err    error
}

func (f *availabilityConfigRepoFake) Create(context.Context, *llmmodel.ModelConfig) error {
	return errors.New("not implemented")
}
func (f *availabilityConfigRepoFake) GetByID(context.Context, uuid.UUID, uuid.UUID) (*llmmodel.ModelConfig, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityConfigRepoFake) GetByModelID(context.Context, uuid.UUID, uuid.UUID) (*llmmodel.ModelConfig, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.config == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.config, nil
}
func (f *availabilityConfigRepoFake) List(context.Context, uuid.UUID, *bool, int, int) ([]*llmmodel.ModelConfig, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *availabilityConfigRepoFake) ListAvailableConfigs(context.Context, uuid.UUID) ([]*llmmodel.ModelConfig, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityConfigRepoFake) Update(context.Context, *llmmodel.ModelConfig) error {
	return errors.New("not implemented")
}
func (f *availabilityConfigRepoFake) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *availabilityConfigRepoFake) Upsert(context.Context, *llmmodel.ModelConfig) error {
	return errors.New("not implemented")
}
func (f *availabilityConfigRepoFake) BatchCreate(context.Context, []*llmmodel.ModelConfig) error {
	return errors.New("not implemented")
}

type availabilityRouteRepoFake struct {
	routes []*channelmodel.LLMRoute
	err    error
}

func (f *availabilityRouteRepoFake) Create(context.Context, *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) BatchCreate(context.Context, []*channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) GetByID(context.Context, uuid.UUID, uuid.UUID) (*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) List(context.Context, uuid.UUID, *bool, int, int) ([]*channelmodel.LLMRoute, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) Update(context.Context, *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) GetEnabledRoutes(context.Context, uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	return f.routes, f.err
}
func (f *availabilityRouteRepoFake) FindByModel(context.Context, uuid.UUID, string) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) CountByCredentialID(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) GetDistinctProviders(context.Context, uuid.UUID) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (f *availabilityRouteRepoFake) GetPlatformChannels(context.Context) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

func TestAvailabilityServiceRejectsDeprecatedModel(t *testing.T) {
	modelID := uuid.New()
	modelName := "old-model"
	svc := NewAvailabilityServiceWithProviderRepos(
		&availabilityModelRepoFake{model: &llmmodel.LLMModel{
			ID:       modelID,
			Provider: "openai",
			Model:    modelName,
			IsActive: true,
			Status:   llmmodel.ModelStatusDeprecated,
		}},
		&availabilityConfigRepoFake{},
		&availabilityRouteRepoFake{routes: []*channelmodel.LLMRoute{{
			ID:         uuid.New(),
			Models:     []string{modelName},
			IsOfficial: true,
			IsEnabled:  true,
		}}},
		nil,
		nil,
	)

	got, err := svc.CheckModelAvailability(context.Background(), uuid.New(), modelID)
	if err != nil {
		t.Fatalf("CheckModelAvailability returned error: %v", err)
	}
	if got.Status != dto.ModelUnavailable {
		t.Fatalf("status = %q, want %q", got.Status, dto.ModelUnavailable)
	}
}

func TestAvailabilityServiceRejectsTenantDisabledModel(t *testing.T) {
	modelID := uuid.New()
	modelName := "tenant-disabled-model"
	svc := NewAvailabilityServiceWithProviderRepos(
		&availabilityModelRepoFake{model: &llmmodel.LLMModel{
			ID:       modelID,
			Provider: "openai",
			Model:    modelName,
			IsActive: true,
			Status:   llmmodel.ModelStatusActive,
		}},
		&availabilityConfigRepoFake{config: &llmmodel.ModelConfig{ModelID: modelID, IsEnabled: false}},
		&availabilityRouteRepoFake{routes: []*channelmodel.LLMRoute{{
			ID:         uuid.New(),
			Models:     []string{modelName},
			IsOfficial: true,
			IsEnabled:  true,
		}}},
		nil,
		nil,
	)

	got, err := svc.CheckModelAvailability(context.Background(), uuid.New(), modelID)
	if err != nil {
		t.Fatalf("CheckModelAvailability returned error: %v", err)
	}
	if got.Status != dto.ModelUnavailable {
		t.Fatalf("status = %q, want %q", got.Status, dto.ModelUnavailable)
	}
}

func TestAvailabilityBatchReturnsLookupError(t *testing.T) {
	wantErr := errors.New("model repo down")
	svc := NewAvailabilityServiceWithProviderRepos(
		&availabilityModelRepoFake{err: wantErr},
		&availabilityConfigRepoFake{},
		&availabilityRouteRepoFake{},
		nil,
		nil,
	)

	_, err := svc.BatchCheckAvailability(context.Background(), uuid.New(), []uuid.UUID{uuid.New()})
	if !errors.Is(err, wantErr) {
		t.Fatalf("BatchCheckAvailability error = %v, want %v", err, wantErr)
	}
}
