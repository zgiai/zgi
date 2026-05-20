package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/dto"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/ginext/pkg/response"
)

type fakeModelService struct {
	models        []*model.ModelView
	parameters    model.ConfigParameters
	parametersErr error
	err           error
	seenProvider  string
}

func (f *fakeModelService) CreateGlobal(ctx context.Context, req *dto.CreateModelRequest) (*model.LLMModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) GetGlobal(ctx context.Context, id uuid.UUID) (*model.LLMModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) ListGlobal(ctx context.Context, req *dto.ListModelRequest) ([]*model.LLMModel, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeModelService) UpdateGlobal(ctx context.Context, id uuid.UUID, req *dto.UpdateModelRequest) (*model.LLMModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) DeleteGlobal(ctx context.Context, id uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeModelService) ConfigureModel(ctx context.Context, organizationID uuid.UUID, req *dto.ConfigureModelRequest) (*model.ModelConfig, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) GetModelConfig(ctx context.Context, organizationID, modelID uuid.UUID) (*model.ModelConfig, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) ListModelConfigs(ctx context.Context, organizationID uuid.UUID, req *dto.ListModelRequest) ([]*model.ModelConfig, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeModelService) CreateCustom(ctx context.Context, organizationID uuid.UUID, req *dto.CreateCustomModelRequest) (*model.CustomModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) GetCustom(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) ListCustom(ctx context.Context, organizationID uuid.UUID, req *dto.ListModelRequest) ([]*model.CustomModel, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeModelService) UpdateCustom(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateCustomModelRequest) (*model.CustomModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) DeleteCustom(ctx context.Context, organizationID, id uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeModelService) ListTenantModels(ctx context.Context, organizationID uuid.UUID, useCase string, provider string) ([]*model.ModelView, error) {
	f.seenProvider = provider
	if provider == "" {
		return f.models, f.err
	}
	filtered := make([]*model.ModelView, 0, len(f.models))
	for _, item := range f.models {
		if item.Provider == provider {
			filtered = append(filtered, item)
		}
	}
	return filtered, f.err
}

func (f *fakeModelService) GetModelParameters(ctx context.Context, organizationID uuid.UUID, provider, modelName string) (model.ConfigParameters, error) {
	return f.parameters, f.parametersErr
}

func (f *fakeModelService) ToggleProviderModels(ctx context.Context, organizationID uuid.UUID, provider string, isEnabled bool) error {
	return errors.New("not implemented")
}

func (f *fakeModelService) BatchToggleModels(ctx context.Context, organizationID uuid.UUID, modelIDs []uuid.UUID, isEnabled bool) error {
	return errors.New("not implemented")
}

func (f *fakeModelService) ListOfficialModels(ctx context.Context) ([]*model.LLMModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) CheckAvailability(ctx context.Context, organizationID uuid.UUID, modelID uuid.UUID) (*dto.ModelAvailabilityResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) BatchCheckAvailability(ctx context.Context, organizationID uuid.UUID, req *dto.BatchModelAvailabilityRequest) (*dto.BatchModelAvailabilityResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelService) SetAvailableModelsService(svc service.AvailableModelsService) {}

func TestListTenantModels_FilterByProvider_Hit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	svc := &fakeModelService{
		models: []*model.ModelView{
			{ID: uuid.New(), Provider: "test-provider", Model: "ernie-x1-turbo-32k"},
			{ID: uuid.New(), Provider: "other-provider", Model: "gpt-4"},
		},
	}
	h := NewModelHandler(svc)

	router := gin.New()
	router.GET("/llm/models", h.ListTenantModels)

	req := httptest.NewRequest(http.MethodGet, "/llm/models?provider=test-provider", nil)
	req.Header.Set("X-Organization-ID", organizationID.String())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test-provider", svc.seenProvider)

	data := decodeTenantModelListData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	require.Len(t, items, 1)
	assert.Equal(t, float64(1), data["total"])

	item := items[0].(map[string]interface{})
	assert.Equal(t, "test-provider", item["provider"])
	assert.Equal(t, "ernie-x1-turbo-32k", item["model"])
}

func TestListTenantModels_NoProviderFilter_ReturnAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	svc := &fakeModelService{
		models: []*model.ModelView{
			{ID: uuid.New(), Provider: "test-provider", Model: "ernie-x1-turbo-32k"},
			{ID: uuid.New(), Provider: "other-provider", Model: "gpt-4"},
		},
	}
	h := NewModelHandler(svc)

	router := gin.New()
	router.GET("/llm/models", h.ListTenantModels)

	req := httptest.NewRequest(http.MethodGet, "/llm/models", nil)
	req.Header.Set("X-Organization-ID", organizationID.String())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, svc.seenProvider)

	data := decodeTenantModelListData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	require.Len(t, items, 2)
	assert.Equal(t, float64(2), data["total"])
}

func TestListTenantModels_FilterByProvider_Miss(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	svc := &fakeModelService{
		models: []*model.ModelView{
			{ID: uuid.New(), Provider: "test-provider", Model: "ernie-x1-turbo-32k"},
		},
	}
	h := NewModelHandler(svc)

	router := gin.New()
	router.GET("/llm/models", h.ListTenantModels)

	req := httptest.NewRequest(http.MethodGet, "/llm/models?provider=other-provider", nil)
	req.Header.Set("X-Organization-ID", organizationID.String())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "other-provider", svc.seenProvider)

	data := decodeTenantModelListData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	require.Len(t, items, 0)
	assert.Equal(t, float64(0), data["total"])
}

func TestGetModelParameters_ReturnsRawConfigParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	precision := 2
	h := NewModelHandler(&fakeModelService{
		parameters: model.ConfigParameters{
			{
				Name:        "temperature",
				TemplateKey: "temperature",
				Type:        "float",
				Required:    false,
				Default:     json.RawMessage("1"),
				Min:         json.RawMessage("0"),
				Max:         json.RawMessage("2"),
				Precision:   &precision,
			},
		},
	})

	router := gin.New()
	router.GET("/llm/models/parameters", h.GetModelParameters)

	req := httptest.NewRequest(http.MethodGet, "/llm/models/parameters?provider=openai&model=gpt-4.1", nil)
	req.Header.Set("X-Organization-ID", organizationID.String())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "0", resp.Code)

	items, ok := resp.Data.([]interface{})
	require.True(t, ok)
	require.Len(t, items, 1)

	item := items[0].(map[string]interface{})
	assert.Equal(t, "temperature", item["name"])
	assert.Equal(t, "temperature", item["template_key"])
	assert.Equal(t, "float", item["type"])
	assert.Equal(t, float64(2), item["precision"])
	_, hasLabel := item["label"]
	assert.False(t, hasLabel)
}

func decodeTenantModelListData(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()

	var resp response.Response
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Equal(t, "0", resp.Code)

	data, ok := resp.Data.(map[string]interface{})
	require.True(t, ok)
	return data
}
