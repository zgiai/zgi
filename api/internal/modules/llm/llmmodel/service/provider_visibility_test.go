package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

type fakeProviderRepo struct {
	providers []*providermodel.LLMProvider
}

func (f *fakeProviderRepo) Create(ctx context.Context, provider *providermodel.LLMProvider) error {
	return errors.New("not implemented")
}

func (f *fakeProviderRepo) GetByID(ctx context.Context, id uuid.UUID) (*providermodel.LLMProvider, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProviderRepo) GetByName(ctx context.Context, name string) (*providermodel.LLMProvider, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProviderRepo) List(ctx context.Context, isActive *bool, offset, limit int) ([]*providermodel.LLMProvider, int64, error) {
	var out []*providermodel.LLMProvider
	for _, provider := range f.providers {
		if isActive != nil && provider.IsActive != *isActive {
			continue
		}
		out = append(out, provider)
	}
	return out, int64(len(out)), nil
}

func (f *fakeProviderRepo) Update(ctx context.Context, provider *providermodel.LLMProvider) error {
	return errors.New("not implemented")
}

func (f *fakeProviderRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeProviderRepo) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, errors.New("not implemented")
}

type fakeProviderConfigRepo struct {
	configs []*providermodel.ProviderConfig
}

func (f *fakeProviderConfigRepo) Create(ctx context.Context, config *providermodel.ProviderConfig) error {
	return errors.New("not implemented")
}

func (f *fakeProviderConfigRepo) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*providermodel.ProviderConfig, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProviderConfigRepo) GetByProviderID(ctx context.Context, organizationID, providerID uuid.UUID) (*providermodel.ProviderConfig, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProviderConfigRepo) List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*providermodel.ProviderConfig, int64, error) {
	var out []*providermodel.ProviderConfig
	for _, cfg := range f.configs {
		if cfg.OrganizationID != organizationID {
			continue
		}
		if isEnabled != nil && cfg.IsEnabled != *isEnabled {
			continue
		}
		out = append(out, cfg)
	}
	return out, int64(len(out)), nil
}

func (f *fakeProviderConfigRepo) Update(ctx context.Context, config *providermodel.ProviderConfig) error {
	return errors.New("not implemented")
}

func (f *fakeProviderConfigRepo) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeProviderConfigRepo) Upsert(ctx context.Context, config *providermodel.ProviderConfig) error {
	return errors.New("not implemented")
}

type fakeCustomProviderRepo struct {
	providers []*providermodel.CustomProvider
}

func (f *fakeCustomProviderRepo) Create(ctx context.Context, provider *providermodel.CustomProvider) error {
	return errors.New("not implemented")
}

func (f *fakeCustomProviderRepo) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*providermodel.CustomProvider, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeCustomProviderRepo) GetByProvider(ctx context.Context, organizationID uuid.UUID, provider string) (*providermodel.CustomProvider, error) {
	for _, item := range f.providers {
		if item.OrganizationID == organizationID && item.Provider == provider {
			return item, nil
		}
	}
	return nil, errors.New("not implemented")
}

func (f *fakeCustomProviderRepo) List(ctx context.Context, organizationID uuid.UUID, isActive *bool, offset, limit int) ([]*providermodel.CustomProvider, int64, error) {
	var out []*providermodel.CustomProvider
	for _, provider := range f.providers {
		if provider.OrganizationID != organizationID {
			continue
		}
		if isActive != nil && provider.IsActive != *isActive {
			continue
		}
		out = append(out, provider)
	}
	return out, int64(len(out)), nil
}

func (f *fakeCustomProviderRepo) Update(ctx context.Context, provider *providermodel.CustomProvider) error {
	return errors.New("not implemented")
}

func (f *fakeCustomProviderRepo) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeCustomProviderRepo) ExistsByProvider(ctx context.Context, organizationID uuid.UUID, provider string) (bool, error) {
	return false, errors.New("not implemented")
}
