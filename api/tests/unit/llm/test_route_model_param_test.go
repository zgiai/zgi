package llm_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	credentialdto "github.com/zgiai/zgi/api/internal/modules/llm/credential/dto"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	credentialsvc "github.com/zgiai/zgi/api/internal/modules/llm/credential/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// ============================================================================
// Mock implementations for TestRoute tests
// ============================================================================

// mockTenantRouteRepo implements repository.TenantRouteRepository
type mockTenantRouteRepo struct {
	mock.Mock
}

func (m *mockTenantRouteRepo) Create(ctx context.Context, route *model.LLMRoute) error {
	return m.Called(ctx, route).Error(0)
}

func (m *mockTenantRouteRepo) BatchCreate(ctx context.Context, routes []*model.LLMRoute) error {
	return m.Called(ctx, routes).Error(0)
}

func (m *mockTenantRouteRepo) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*model.LLMRoute, error) {
	args := m.Called(ctx, tenantID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.LLMRoute), args.Error(1)
}

func (m *mockTenantRouteRepo) List(ctx context.Context, tenantID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.LLMRoute, int64, error) {
	args := m.Called(ctx, tenantID, isEnabled, offset, limit)
	return args.Get(0).([]*model.LLMRoute), args.Get(1).(int64), args.Error(2)
}

func (m *mockTenantRouteRepo) Update(ctx context.Context, route *model.LLMRoute) error {
	return m.Called(ctx, route).Error(0)
}

func (m *mockTenantRouteRepo) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return m.Called(ctx, tenantID, id).Error(0)
}

func (m *mockTenantRouteRepo) GetEnabledRoutes(ctx context.Context, tenantID uuid.UUID) ([]*model.LLMRoute, error) {
	args := m.Called(ctx, tenantID)
	return args.Get(0).([]*model.LLMRoute), args.Error(1)
}

func (m *mockTenantRouteRepo) FindByModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*model.LLMRoute, error) {
	args := m.Called(ctx, organizationID, modelName)
	return args.Get(0).([]*model.LLMRoute), args.Error(1)
}

func (m *mockTenantRouteRepo) CountByCredentialID(ctx context.Context, organizationID uuid.UUID, credentialID uuid.UUID) (int64, error) {
	args := m.Called(ctx, organizationID, credentialID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockTenantRouteRepo) GetDistinctProviders(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	args := m.Called(ctx, tenantID)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockTenantRouteRepo) GetPlatformChannels(ctx context.Context) ([]*model.LLMRoute, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*model.LLMRoute), args.Error(1)
}

var _ repository.TenantRouteRepository = (*mockTenantRouteRepo)(nil)

// mockTenantCredSvc implements credentialsvc.TenantCredentialService
type mockTenantCredSvc struct {
	mock.Mock
}

func (m *mockTenantCredSvc) Create(ctx context.Context, tenantID uuid.UUID, req *credentialdto.CreateTenantCredentialRequest) (*credentialmodel.TenantCredential, error) {
	args := m.Called(ctx, tenantID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*credentialmodel.TenantCredential), args.Error(1)
}

func (m *mockTenantCredSvc) GetOrCreateByAPIKey(ctx context.Context, tenantID uuid.UUID, req *credentialdto.CreateTenantCredentialRequest) (*credentialmodel.TenantCredential, bool, error) {
	args := m.Called(ctx, tenantID, req)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*credentialmodel.TenantCredential), args.Bool(1), args.Error(2)
}

func (m *mockTenantCredSvc) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*credentialmodel.TenantCredential, error) {
	args := m.Called(ctx, tenantID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*credentialmodel.TenantCredential), args.Error(1)
}

func (m *mockTenantCredSvc) List(ctx context.Context, tenantID uuid.UUID, req *credentialdto.ListCredentialRequest) ([]*credentialmodel.TenantCredential, int64, error) {
	args := m.Called(ctx, tenantID, req)
	return args.Get(0).([]*credentialmodel.TenantCredential), args.Get(1).(int64), args.Error(2)
}

func (m *mockTenantCredSvc) Update(ctx context.Context, tenantID, id uuid.UUID, req *credentialdto.UpdateTenantCredentialRequest) (*credentialmodel.TenantCredential, error) {
	args := m.Called(ctx, tenantID, id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*credentialmodel.TenantCredential), args.Error(1)
}

func (m *mockTenantCredSvc) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return m.Called(ctx, tenantID, id).Error(0)
}

func (m *mockTenantCredSvc) GetDecryptedAPIKey(ctx context.Context, tenantID, id uuid.UUID) (string, error) {
	args := m.Called(ctx, tenantID, id)
	return args.String(0), args.Error(1)
}

func (m *mockTenantCredSvc) TestCredential(ctx context.Context, tenantID, id uuid.UUID, modelName string, apiBaseURL string) (*credentialdto.TestCredentialResult, error) {
	args := m.Called(ctx, tenantID, id, modelName, apiBaseURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*credentialdto.TestCredentialResult), args.Error(1)
}

var _ credentialsvc.TenantCredentialService = (*mockTenantCredSvc)(nil)

type mockChannelValidator struct {
	mock.Mock
}

func (m *mockChannelValidator) ValidateModelsForCreation(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL string, models []string) (*channelprovider.ValidationResult, error) {
	args := m.Called(ctx, organizationID, channelProvider, apiKey, apiBaseURL, models)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*channelprovider.ValidationResult), args.Error(1)
}

func (m *mockChannelValidator) ValidateModels(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL string, models []string) (*channelprovider.ValidationResult, error) {
	args := m.Called(ctx, organizationID, channelProvider, apiKey, apiBaseURL, models)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*channelprovider.ValidationResult), args.Error(1)
}

func (m *mockChannelValidator) TestModel(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL, modelName, testMethod string) (*channelprovider.TestResult, error) {
	args := m.Called(ctx, organizationID, channelProvider, apiKey, apiBaseURL, modelName, testMethod)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*channelprovider.TestResult), args.Error(1)
}

var _ service.ChannelValidator = (*mockChannelValidator)(nil)

// ============================================================================
// TestRoute tests - verifying model parameter support
// ============================================================================

func newTestService(routeRepo *mockTenantRouteRepo, credSvc *mockTenantCredSvc, validator service.ChannelValidator) service.ChannelService {
	return service.NewChannelService(routeRepo, credSvc, validator, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

// TestTestRoute_WithSpecificModel verifies that when a model is provided in the request,
// the service uses that model instead of defaulting to route.Models[0].
// This is the core regression test for the bug where Body model param was ignored.
func TestTestRoute_WithSpecificModel(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	routeID := uuid.New()
	credID := uuid.New()

	routeRepo := new(mockTenantRouteRepo)
	credSvc := new(mockTenantCredSvc)
	validator := new(mockChannelValidator)
	svc := newTestService(routeRepo, credSvc, validator)

	route := &model.LLMRoute{
		ID:              routeID,
		OrganizationID:  tenantID,
		Type:            shared.RouteTypePrivate,
		CredentialID:    &credID,
		ChannelProvider: "openai-compatible",
		Models:          []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
		APIBaseURL:      "https://api.openai.com/v1",
	}

	routeRepo.On("GetByID", ctx, tenantID, routeID).Return(route, nil)
	credSvc.On("GetDecryptedAPIKey", ctx, tenantID, credID).Return("sk-test", nil)
	validator.On("TestModel", ctx, tenantID, "openai-compatible", "sk-test", "https://api.openai.com/v1", "gpt-4o-mini", "").
		Return(&channelprovider.TestResult{
			Success:        true,
			Message:        "OK",
			ResponseTimeMs: 150,
		}, nil)

	result, err := svc.TestRoute(ctx, tenantID, routeID, "gpt-4o-mini")

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, int64(150), result.ResponseTime)

	// Verify validator received the requested model, not the default first model.
	credSvc.AssertCalled(t, "GetDecryptedAPIKey", ctx, tenantID, credID)
	validator.AssertCalled(t, "TestModel", ctx, tenantID, "openai-compatible", "sk-test", "https://api.openai.com/v1", "gpt-4o-mini", "")
	validator.AssertNotCalled(t, "TestModel", ctx, tenantID, "openai-compatible", "sk-test", "https://api.openai.com/v1", "gpt-4o", "")
	routeRepo.AssertExpectations(t)
}

// TestTestRoute_EmptyModelFallsBackToFirstModel verifies that when no model is provided,
// the service falls back to using route.Models[0].
func TestTestRoute_EmptyModelFallsBackToFirstModel(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	routeID := uuid.New()
	credID := uuid.New()

	routeRepo := new(mockTenantRouteRepo)
	credSvc := new(mockTenantCredSvc)
	validator := new(mockChannelValidator)
	svc := newTestService(routeRepo, credSvc, validator)

	route := &model.LLMRoute{
		ID:              routeID,
		OrganizationID:  tenantID,
		Type:            shared.RouteTypePrivate,
		CredentialID:    &credID,
		ChannelProvider: "openai-compatible",
		Models:          []string{"gpt-4o", "gpt-4o-mini"},
		APIBaseURL:      "https://api.openai.com/v1",
	}

	routeRepo.On("GetByID", ctx, tenantID, routeID).Return(route, nil)
	credSvc.On("GetDecryptedAPIKey", ctx, tenantID, credID).Return("sk-test", nil)
	validator.On("TestModel", ctx, tenantID, "openai-compatible", "sk-test", "https://api.openai.com/v1", "gpt-4o", "").
		Return(&channelprovider.TestResult{
			Success:        true,
			Message:        "OK",
			ResponseTimeMs: 200,
		}, nil)

	// Empty model string should fallback to first model
	result, err := svc.TestRoute(ctx, tenantID, routeID, "")

	assert.NoError(t, err)
	assert.True(t, result.Success)
	credSvc.AssertCalled(t, "GetDecryptedAPIKey", ctx, tenantID, credID)
	validator.AssertCalled(t, "TestModel", ctx, tenantID, "openai-compatible", "sk-test", "https://api.openai.com/v1", "gpt-4o", "")
	routeRepo.AssertExpectations(t)
}

// TestTestRoute_OfficialRouteCannotTest verifies official routes return appropriate error
func TestTestRoute_OfficialRouteCannotTest(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	routeID := uuid.New()

	routeRepo := new(mockTenantRouteRepo)
	credSvc := new(mockTenantCredSvc)
	validator := new(mockChannelValidator)
	svc := newTestService(routeRepo, credSvc, validator)

	route := &model.LLMRoute{
		ID:             routeID,
		OrganizationID: tenantID,
		IsOfficial:     true,
	}

	routeRepo.On("GetByID", ctx, tenantID, routeID).Return(route, nil)

	result, err := svc.TestRoute(ctx, tenantID, routeID, "gpt-4o")

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "Official channels")
	credSvc.AssertNotCalled(t, "GetDecryptedAPIKey", mock.Anything, mock.Anything, mock.Anything)
	validator.AssertNotCalled(t, "TestModel", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestTestRoute_NoCredential verifies routes without credentials return appropriate error
func TestTestRoute_NoCredential(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	routeID := uuid.New()

	routeRepo := new(mockTenantRouteRepo)
	credSvc := new(mockTenantCredSvc)
	validator := new(mockChannelValidator)
	svc := newTestService(routeRepo, credSvc, validator)

	route := &model.LLMRoute{
		ID:             routeID,
		OrganizationID: tenantID,
		Type:           shared.RouteTypePrivate,
		CredentialID:   nil, // No credential
		Models:         []string{"gpt-4o"},
	}

	routeRepo.On("GetByID", ctx, tenantID, routeID).Return(route, nil)

	result, err := svc.TestRoute(ctx, tenantID, routeID, "gpt-4o")

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "no credential")
	validator.AssertNotCalled(t, "TestModel", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestTestRoute_NoModels verifies routes with empty model list return appropriate error
func TestTestRoute_NoModels(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	routeID := uuid.New()
	credID := uuid.New()

	routeRepo := new(mockTenantRouteRepo)
	credSvc := new(mockTenantCredSvc)
	validator := new(mockChannelValidator)
	svc := newTestService(routeRepo, credSvc, validator)

	route := &model.LLMRoute{
		ID:             routeID,
		OrganizationID: tenantID,
		Type:           shared.RouteTypePrivate,
		CredentialID:   &credID,
		Models:         []string{}, // Empty model list
	}

	routeRepo.On("GetByID", ctx, tenantID, routeID).Return(route, nil)

	// Even with empty model list, if a specific model is requested it should NOT fail
	// because the user explicitly specified which model to test
	result, err := svc.TestRoute(ctx, tenantID, routeID, "")

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "no models configured")
	validator.AssertNotCalled(t, "TestModel", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestTestRoute_RouteNotFound verifies error when route doesn't exist
func TestTestRoute_RouteNotFound(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	routeID := uuid.New()

	routeRepo := new(mockTenantRouteRepo)
	credSvc := new(mockTenantCredSvc)
	validator := new(mockChannelValidator)
	svc := newTestService(routeRepo, credSvc, validator)

	routeRepo.On("GetByID", ctx, tenantID, routeID).Return(nil, service.ErrRouteNotFound)

	result, err := svc.TestRoute(ctx, tenantID, routeID, "gpt-4o")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, service.ErrRouteNotFound, err)
}

// TestTestRoute_CredentialTestFailure verifies graceful handling when credential test fails
func TestTestRoute_CredentialTestFailure(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	routeID := uuid.New()
	credID := uuid.New()

	routeRepo := new(mockTenantRouteRepo)
	credSvc := new(mockTenantCredSvc)
	validator := new(mockChannelValidator)
	svc := newTestService(routeRepo, credSvc, validator)

	route := &model.LLMRoute{
		ID:              routeID,
		OrganizationID:  tenantID,
		Type:            shared.RouteTypePrivate,
		CredentialID:    &credID,
		ChannelProvider: "openai-compatible",
		Models:          []string{"gpt-4o"},
		APIBaseURL:      "https://api.openai.com/v1",
	}

	routeRepo.On("GetByID", ctx, tenantID, routeID).Return(route, nil)
	credSvc.On("GetDecryptedAPIKey", ctx, tenantID, credID).Return("sk-test", nil)
	validator.On("TestModel", ctx, tenantID, "openai-compatible", "sk-test", "https://api.openai.com/v1", "gpt-4o", "").
		Return(&channelprovider.TestResult{
			Success: false,
			Message: "invalid API key",
		}, nil)

	result, err := svc.TestRoute(ctx, tenantID, routeID, "gpt-4o")

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "invalid API key")
}
