package llm_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/service"
)

// MockProviderRepository is a mock implementation of ProviderRepository
type MockProviderRepository struct {
	mock.Mock
}

func (m *MockProviderRepository) Create(ctx context.Context, p *model.LLMProvider) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockProviderRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.LLMProvider, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.LLMProvider), args.Error(1)
}

func (m *MockProviderRepository) GetByName(ctx context.Context, name string) (*model.LLMProvider, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.LLMProvider), args.Error(1)
}

func (m *MockProviderRepository) List(ctx context.Context, isActive *bool, offset, limit int) ([]*model.LLMProvider, int64, error) {
	args := m.Called(ctx, isActive, offset, limit)
	return args.Get(0).([]*model.LLMProvider), args.Get(1).(int64), args.Error(2)
}

func (m *MockProviderRepository) Update(ctx context.Context, p *model.LLMProvider) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockProviderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockProviderRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	args := m.Called(ctx, name)
	return args.Bool(0), args.Error(1)
}

// MockProviderConfigRepository
type MockProviderConfigRepository struct {
	mock.Mock
}

func (m *MockProviderConfigRepository) Create(ctx context.Context, config *model.ProviderConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockProviderConfigRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*model.ProviderConfig, error) {
	args := m.Called(ctx, tenantID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ProviderConfig), args.Error(1)
}

func (m *MockProviderConfigRepository) GetByProviderID(ctx context.Context, tenantID, providerID uuid.UUID) (*model.ProviderConfig, error) {
	args := m.Called(ctx, tenantID, providerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ProviderConfig), args.Error(1)
}

func (m *MockProviderConfigRepository) List(ctx context.Context, tenantID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.ProviderConfig, int64, error) {
	args := m.Called(ctx, tenantID, isEnabled, offset, limit)
	return args.Get(0).([]*model.ProviderConfig), args.Get(1).(int64), args.Error(2)
}

func (m *MockProviderConfigRepository) Update(ctx context.Context, config *model.ProviderConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockProviderConfigRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	args := m.Called(ctx, tenantID, id)
	return args.Error(0)
}

func (m *MockProviderConfigRepository) Upsert(ctx context.Context, config *model.ProviderConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

// MockCustomProviderRepository
type MockCustomProviderRepository struct {
	mock.Mock
}

func (m *MockCustomProviderRepository) Create(ctx context.Context, p *model.CustomProvider) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockCustomProviderRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*model.CustomProvider, error) {
	args := m.Called(ctx, tenantID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.CustomProvider), args.Error(1)
}

func (m *MockCustomProviderRepository) GetByProvider(ctx context.Context, tenantID uuid.UUID, provider string) (*model.CustomProvider, error) {
	args := m.Called(ctx, tenantID, provider)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.CustomProvider), args.Error(1)
}

func (m *MockCustomProviderRepository) List(ctx context.Context, tenantID uuid.UUID, isActive *bool, offset, limit int) ([]*model.CustomProvider, int64, error) {
	args := m.Called(ctx, tenantID, isActive, offset, limit)
	return args.Get(0).([]*model.CustomProvider), args.Get(1).(int64), args.Error(2)
}

func (m *MockCustomProviderRepository) Update(ctx context.Context, p *model.CustomProvider) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockCustomProviderRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	args := m.Called(ctx, tenantID, id)
	return args.Error(0)
}

func (m *MockCustomProviderRepository) ExistsByProvider(ctx context.Context, tenantID uuid.UUID, provider string) (bool, error) {
	args := m.Called(ctx, tenantID, provider)
	return args.Bool(0), args.Error(1)
}

// TestProviderService_CreateGlobal tests the CreateGlobal method
func TestProviderService_CreateGlobal(t *testing.T) {
	ctx := context.Background()
	mockGlobalRepo := new(MockProviderRepository)
	mockConfigRepo := new(MockProviderConfigRepository)
	mockCustomRepo := new(MockCustomProviderRepository)
	svc := service.NewProviderService(nil, mockGlobalRepo, mockConfigRepo, mockCustomRepo, nil, nil, nil)

	t.Run("success", func(t *testing.T) {
		req := &dto.CreateProviderRequest{
			Name:         "openai",
			ProviderName: "OpenAI",
			APIBaseURL:   "https://api.openai.com/v1",
		}

		mockGlobalRepo.On("ExistsByName", ctx, "openai").Return(false, nil).Once()
		mockGlobalRepo.On("Create", ctx, mock.AnythingOfType("*model.LLMProvider")).Return(nil).Once()

		result, err := svc.CreateGlobal(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "openai", result.Provider)
		assert.Equal(t, "OpenAI", result.ProviderName)
		assert.True(t, result.IsActive)
		assert.Equal(t, "vendor", result.ProviderType)
		mockGlobalRepo.AssertExpectations(t)
	})

	t.Run("already exists", func(t *testing.T) {
		req := &dto.CreateProviderRequest{
			Name:         "existing",
			ProviderName: "Existing Provider",
		}

		mockGlobalRepo.On("ExistsByName", ctx, "existing").Return(true, nil).Once()

		result, err := svc.CreateGlobal(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, service.ErrProviderExists, err)
		mockGlobalRepo.AssertExpectations(t)
	})
}

// TestProviderService_GetGlobal tests the GetGlobal method
func TestProviderService_GetGlobal(t *testing.T) {
	ctx := context.Background()
	mockGlobalRepo := new(MockProviderRepository)
	mockConfigRepo := new(MockProviderConfigRepository)
	mockCustomRepo := new(MockCustomProviderRepository)
	svc := service.NewProviderService(nil, mockGlobalRepo, mockConfigRepo, mockCustomRepo, nil, nil, nil)

	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		expected := &model.LLMProvider{
			ID:           id,
			Provider:     "openai",
			ProviderName: "OpenAI",
		}

		mockGlobalRepo.On("GetByID", ctx, id).Return(expected, nil).Once()

		result, err := svc.GetGlobal(ctx, id)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, id, result.ID)
		mockGlobalRepo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		id := uuid.New()

		mockGlobalRepo.On("GetByID", ctx, id).Return(nil, service.ErrProviderNotFound).Once()

		result, err := svc.GetGlobal(ctx, id)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockGlobalRepo.AssertExpectations(t)
	})
}

// TestProviderService_ListGlobal tests the ListGlobal method
func TestProviderService_ListGlobal(t *testing.T) {
	ctx := context.Background()
	mockGlobalRepo := new(MockProviderRepository)
	mockConfigRepo := new(MockProviderConfigRepository)
	mockCustomRepo := new(MockCustomProviderRepository)
	svc := service.NewProviderService(nil, mockGlobalRepo, mockConfigRepo, mockCustomRepo, nil, nil, nil)

	t.Run("success", func(t *testing.T) {
		providers := []*model.LLMProvider{
			{ID: uuid.New(), Provider: "openai", ProviderName: "OpenAI"},
			{ID: uuid.New(), Provider: "anthropic", ProviderName: "Anthropic"},
		}

		mockGlobalRepo.On("List", ctx, (*bool)(nil), 0, 50).Return(providers, int64(2), nil).Once()

		req := &dto.ListProviderRequest{Page: 1, PageSize: 50}
		result, total, err := svc.ListGlobal(ctx, req)

		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, int64(2), total)
		mockGlobalRepo.AssertExpectations(t)
	})
}

// TestProviderService_UpdateGlobal tests the UpdateGlobal method
func TestProviderService_UpdateGlobal(t *testing.T) {
	ctx := context.Background()
	mockGlobalRepo := new(MockProviderRepository)
	mockConfigRepo := new(MockProviderConfigRepository)
	mockCustomRepo := new(MockCustomProviderRepository)
	svc := service.NewProviderService(nil, mockGlobalRepo, mockConfigRepo, mockCustomRepo, nil, nil, nil)

	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		existing := &model.LLMProvider{
			ID:           id,
			Provider:     "openai",
			ProviderName: "OpenAI",
			IsActive:     true,
		}

		newProviderName := "OpenAI Updated"
		req := &dto.UpdateProviderRequest{
			ProviderName: &newProviderName,
		}

		mockGlobalRepo.On("GetByID", ctx, id).Return(existing, nil).Once()
		mockGlobalRepo.On("Update", ctx, mock.AnythingOfType("*model.LLMProvider")).Return(nil).Once()

		result, err := svc.UpdateGlobal(ctx, id, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "OpenAI Updated", result.ProviderName)
		mockGlobalRepo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		id := uuid.New()
		req := &dto.UpdateProviderRequest{}

		mockGlobalRepo.On("GetByID", ctx, id).Return(nil, service.ErrProviderNotFound).Once()

		result, err := svc.UpdateGlobal(ctx, id, req)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockGlobalRepo.AssertExpectations(t)
	})
}

// TestProviderService_DeleteGlobal tests the DeleteGlobal method
func TestProviderService_DeleteGlobal(t *testing.T) {
	ctx := context.Background()
	mockGlobalRepo := new(MockProviderRepository)
	mockConfigRepo := new(MockProviderConfigRepository)
	mockCustomRepo := new(MockCustomProviderRepository)
	svc := service.NewProviderService(nil, mockGlobalRepo, mockConfigRepo, mockCustomRepo, nil, nil, nil)

	t.Run("success", func(t *testing.T) {
		id := uuid.New()

		mockGlobalRepo.On("Delete", ctx, id).Return(nil).Once()

		err := svc.DeleteGlobal(ctx, id)

		assert.NoError(t, err)
		mockGlobalRepo.AssertExpectations(t)
	})
}

// TestProviderService_ConfigureProvider tests the ConfigureProvider method
func TestProviderService_ConfigureProvider(t *testing.T) {
	ctx := context.Background()
	mockGlobalRepo := new(MockProviderRepository)
	mockConfigRepo := new(MockProviderConfigRepository)
	mockCustomRepo := new(MockCustomProviderRepository)
	svc := service.NewProviderService(nil, mockGlobalRepo, mockConfigRepo, mockCustomRepo, nil, nil, nil)

	t.Run("success", func(t *testing.T) {
		tenantID := uuid.New()
		providerID := uuid.New()
		existingProvider := &model.LLMProvider{
			ID:       providerID,
			Provider: "openai",
		}

		req := &dto.ConfigureProviderRequest{
			ProviderID:        providerID,
			CustomDisplayName: "My OpenAI",
		}

		mockGlobalRepo.On("GetByID", ctx, providerID).Return(existingProvider, nil).Once()
		mockConfigRepo.On("Upsert", ctx, mock.AnythingOfType("*model.ProviderConfig")).Return(nil).Once()

		result, err := svc.ConfigureProvider(ctx, tenantID, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, tenantID, result.OrganizationID)
		assert.Equal(t, providerID, result.ProviderID)
		assert.Equal(t, "My OpenAI", result.CustomDisplayName)
		mockGlobalRepo.AssertExpectations(t)
		mockConfigRepo.AssertExpectations(t)
	})

	t.Run("provider not found", func(t *testing.T) {
		tenantID := uuid.New()
		providerID := uuid.New()

		req := &dto.ConfigureProviderRequest{
			ProviderID: providerID,
		}

		mockGlobalRepo.On("GetByID", ctx, providerID).Return(nil, service.ErrProviderNotFound).Once()

		result, err := svc.ConfigureProvider(ctx, tenantID, req)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockGlobalRepo.AssertExpectations(t)
	})
}
