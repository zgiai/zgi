package modelmeta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	appconfig "github.com/zgiai/zgi/api/config"
	"gorm.io/gorm"
)

func TestNewServiceDefaultsToZGICuratedModelSource(t *testing.T) {
	originalConfig := appconfig.GlobalConfig
	t.Cleanup(func() {
		appconfig.GlobalConfig = originalConfig
	})
	appconfig.GlobalConfig = &appconfig.Config{}

	svc := NewService(nil)

	require.Equal(t, "https://models.zgi.ai/v1", svc.apiBaseURL)
}

func TestSyncAllProvidersUsesRemoteProviderDiscovery(t *testing.T) {
	var requestedModelsPath string
	var requestedModelsProvider string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/providers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"provider":"qwen"}]}`))
		case "/models":
			requestedModelsPath = r.URL.Path
			requestedModelsProvider = r.URL.Query().Get("provider")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[],"page":1,"page_size":100,"total":0,"total_pages":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	svc := &Service{
		apiBaseURL: server.URL,
		httpClient: server.Client(),
	}

	results, err := svc.SyncAllProviders(context.Background())

	require.NoError(t, err)
	require.Contains(t, results, "qwen")
	require.Len(t, results, 1)
	require.Equal(t, "/models", requestedModelsPath)
	require.Equal(t, "qwen", requestedModelsProvider)
}

func TestSyncProviderModelsFullSyncMarksMissingActiveModelsDeprecated(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	insertCatalogApplyProvider(t, db, "qwen", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-plus", "active", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-coder", "active", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-old", "deprecated", false)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, "qwen", r.URL.Query().Get("provider"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"provider":"qwen","model":"qwen-plus","model_name":"Qwen Plus","status":"active"}],"page":1,"page_size":100,"total":1,"total_pages":1}`))
	}))
	t.Cleanup(server.Close)

	previous := currentModelCacheInvalidator()
	t.Cleanup(func() {
		SetModelCacheInvalidator(previous)
	})
	invalidator := &catalogApplyCacheInvalidatorFake{}
	SetModelCacheInvalidator(invalidator)

	svc := &Service{
		db:         db,
		apiBaseURL: server.URL,
		httpClient: server.Client(),
	}
	result, err := svc.SyncProviderModels(context.Background(), "qwen", nil)

	require.NoError(t, err)
	require.Equal(t, SyncResultStatusSuccess, result.Status)
	require.Equal(t, 1, result.DeprecatedModels)
	require.Equal(t, 1, invalidator.calls)
	requireCatalogApplyModelLifecycle(t, db, "qwen", "qwen-coder", "deprecated", true, true)
	requireCatalogApplyModelLifecycle(t, db, "qwen", "qwen-old", "deprecated", true, true)
	requireCatalogApplyModelLifecycle(t, db, "qwen", "qwen-plus", "active", true, true)
}

func TestSyncProviderModelsSelectedModelsDoesNotDeprecateMissingModels(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	insertCatalogApplyProvider(t, db, "qwen", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-plus", "active", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-coder", "active", false)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"provider":"qwen","model":"qwen-plus","model_name":"Qwen Plus","status":"active"}],"page":1,"page_size":100,"total":1,"total_pages":1}`))
	}))
	t.Cleanup(server.Close)

	svc := &Service{
		db:         db,
		apiBaseURL: server.URL,
		httpClient: server.Client(),
	}
	result, err := svc.SyncProviderModels(context.Background(), "qwen", []string{"qwen-plus"})

	require.NoError(t, err)
	require.Equal(t, 0, result.DeprecatedModels)
	requireCatalogApplyModelLifecycle(t, db, "qwen", "qwen-coder", "active", true, true)
}

func TestSyncProviderModelsDoesNotDeprecateWhenRemoteReturnsEmpty(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	insertCatalogApplyProvider(t, db, "qwen", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-coder", "active", false)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[],"page":1,"page_size":100,"total":0,"total_pages":1}`))
	}))
	t.Cleanup(server.Close)

	svc := &Service{
		db:         db,
		apiBaseURL: server.URL,
		httpClient: server.Client(),
	}
	result, err := svc.SyncProviderModels(context.Background(), "qwen", nil)

	require.NoError(t, err)
	require.Equal(t, 0, result.DeprecatedModels)
	requireCatalogApplyModelLifecycle(t, db, "qwen", "qwen-coder", "active", true, true)
}

func TestComputeDiffReportsOnlyActiveMissingModelsAsDeprecated(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	insertCatalogApplyProvider(t, db, "qwen", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-plus", "active", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-coder", "active", false)
	insertCatalogApplyModel(t, db, "qwen", "qwen-old", "deprecated", false)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"provider":"qwen","model":"qwen-plus","model_name":"Qwen Plus","status":"active"}],"page":1,"page_size":100,"total":1,"total_pages":1}`))
	}))
	t.Cleanup(server.Close)

	svc := &Service{
		db:         db,
		apiBaseURL: server.URL,
		httpClient: server.Client(),
	}
	diff, err := svc.ComputeDiff(context.Background(), "qwen")

	require.NoError(t, err)
	require.Equal(t, 1, diff.Summary.DeprecatedModels)
	require.Len(t, diff.Changes.Deprecated, 1)
	require.Equal(t, "qwen-coder", diff.Changes.Deprecated[0].Model)

	summary, err := svc.computeModelSummary(context.Background(), "qwen")
	require.NoError(t, err)
	require.Equal(t, 2, summary.Local)
	require.Equal(t, 1, summary.LocalOnly)
}

func requireCatalogApplyModelLifecycle(t *testing.T, db *gorm.DB, provider, model, status string, isActive, isSystemEnabled bool) {
	t.Helper()
	var row struct {
		Status          string
		IsActive        bool
		IsSystemEnabled bool
	}
	require.NoError(t, db.Table("llm_models").
		Select("status", "is_active", "is_system_enabled").
		Where("provider = ? AND name = ?", provider, model).
		First(&row).Error)
	require.Equal(t, status, row.Status)
	require.Equal(t, isActive, row.IsActive)
	require.Equal(t, isSystemEnabled, row.IsSystemEnabled)
}
