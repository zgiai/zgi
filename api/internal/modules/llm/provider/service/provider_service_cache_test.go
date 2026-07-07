package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type providerServiceAvailableModelsFake struct {
	invalidated []uuid.UUID
}

func (f *providerServiceAvailableModelsFake) ListAvailable(context.Context, uuid.UUID, string, string) ([]*llmmodelservice.AvailableModel, error) {
	return nil, errors.New("not implemented")
}
func (f *providerServiceAvailableModelsFake) RefreshCache(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *providerServiceAvailableModelsFake) InvalidateTenantCache(organizationID uuid.UUID) {
	f.invalidated = append(f.invalidated, organizationID)
}
func (f *providerServiceAvailableModelsFake) InvalidateGlobalCache() {}
func (f *providerServiceAvailableModelsFake) SetOfficialRouteBootstrapper(interfaces.OfficialRouteBootstrapper) {
}

type providerServiceCustomRepoFake struct {
	provider *model.CustomProvider
}

func (f *providerServiceCustomRepoFake) Create(_ context.Context, provider *model.CustomProvider) error {
	f.provider = provider
	return nil
}
func (f *providerServiceCustomRepoFake) GetByID(context.Context, uuid.UUID, uuid.UUID) (*model.CustomProvider, error) {
	if f.provider == nil {
		return nil, ErrProviderNotFound
	}
	return f.provider, nil
}
func (f *providerServiceCustomRepoFake) GetByProvider(context.Context, uuid.UUID, string) (*model.CustomProvider, error) {
	return nil, errors.New("not implemented")
}
func (f *providerServiceCustomRepoFake) List(context.Context, uuid.UUID, *bool, int, int) ([]*model.CustomProvider, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *providerServiceCustomRepoFake) Update(_ context.Context, provider *model.CustomProvider) error {
	f.provider = provider
	return nil
}
func (f *providerServiceCustomRepoFake) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	f.provider = nil
	return nil
}
func (f *providerServiceCustomRepoFake) ExistsByProvider(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}

func TestCustomProviderChangesInvalidateAvailableModelsCache(t *testing.T) {
	organizationID := uuid.New()
	providerID := uuid.New()
	availableSvc := &providerServiceAvailableModelsFake{}
	customRepo := &providerServiceCustomRepoFake{}
	svc := &providerService{
		customRepo:      customRepo,
		availableModels: availableSvc,
	}

	if _, err := svc.CreateCustom(context.Background(), organizationID, &dto.CreateCustomProviderRequest{
		Provider:     "custom-openai",
		ProviderName: "Custom OpenAI",
	}); err != nil {
		t.Fatalf("CreateCustom returned error: %v", err)
	}

	disabled := false
	customRepo.provider.ID = providerID
	if _, err := svc.UpdateCustom(context.Background(), organizationID, providerID, &dto.UpdateCustomProviderRequest{
		IsActive: &disabled,
	}); err != nil {
		t.Fatalf("UpdateCustom returned error: %v", err)
	}

	if err := svc.DeleteCustom(context.Background(), organizationID, providerID); err != nil {
		t.Fatalf("DeleteCustom returned error: %v", err)
	}

	if len(availableSvc.invalidated) != 3 {
		t.Fatalf("invalidated count = %d, want 3", len(availableSvc.invalidated))
	}
	for _, got := range availableSvc.invalidated {
		if got != organizationID {
			t.Fatalf("invalidated tenant = %s, want %s", got, organizationID)
		}
	}
}

var _ llmmodelservice.AvailableModelsService = (*providerServiceAvailableModelsFake)(nil)
