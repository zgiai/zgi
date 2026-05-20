package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	contentmodel "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	contentsvc "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
)

type fakeProviderAdminService struct{}

func (f *fakeProviderAdminService) GetByID(context.Context, uuid.UUID) (*contentmodel.ProviderConfig, error) {
	return &contentmodel.ProviderConfig{}, nil
}
func (f *fakeProviderAdminService) ListByScope(context.Context, string, *uuid.UUID) ([]*contentmodel.ProviderConfig, error) {
	return []*contentmodel.ProviderConfig{}, nil
}
func (f *fakeProviderAdminService) Create(context.Context, *contentmodel.ProviderConfig) error {
	return nil
}
func (f *fakeProviderAdminService) Update(context.Context, *contentmodel.ProviderConfig) error {
	return nil
}
func (f *fakeProviderAdminService) Delete(context.Context, uuid.UUID) error { return nil }

type fakePolicyAdminService struct{}

func (f *fakePolicyAdminService) GetPolicyByID(context.Context, uuid.UUID) (*contentmodel.RoutePolicy, error) {
	return &contentmodel.RoutePolicy{}, nil
}
func (f *fakePolicyAdminService) ListPoliciesByScope(context.Context, string, *uuid.UUID) ([]*contentmodel.RoutePolicy, error) {
	return []*contentmodel.RoutePolicy{}, nil
}
func (f *fakePolicyAdminService) CreatePolicy(context.Context, *contentmodel.RoutePolicy) error {
	return nil
}
func (f *fakePolicyAdminService) UpdatePolicy(context.Context, *contentmodel.RoutePolicy) error {
	return nil
}
func (f *fakePolicyAdminService) DeletePolicy(context.Context, uuid.UUID) error { return nil }
func (f *fakePolicyAdminService) ListRulesByPolicyID(context.Context, uuid.UUID) ([]*contentmodel.RoutePolicyRule, error) {
	return []*contentmodel.RoutePolicyRule{}, nil
}
func (f *fakePolicyAdminService) CreateRule(context.Context, *contentmodel.RoutePolicyRule) error {
	return nil
}
func (f *fakePolicyAdminService) UpdateRule(context.Context, *contentmodel.RoutePolicyRule) error {
	return nil
}
func (f *fakePolicyAdminService) DeleteRule(context.Context, uuid.UUID) error { return nil }

type fakeHealthService struct{}

func (f *fakeHealthService) Record(context.Context, *contentmodel.ProviderHealthCheck) error {
	return nil
}
func (f *fakeHealthService) ListByProviderConfigID(context.Context, uuid.UUID, int) ([]*contentmodel.ProviderHealthCheck, error) {
	return []*contentmodel.ProviderHealthCheck{}, nil
}
func (f *fakeHealthService) GetLatestByProviderConfigID(context.Context, uuid.UUID) (*contentmodel.ProviderHealthCheck, error) {
	return &contentmodel.ProviderHealthCheck{}, nil
}

type fakeArtifactService struct{}

func (f *fakeArtifactService) GetByID(context.Context, uuid.UUID) (*contentmodel.Artifact, error) {
	return &contentmodel.Artifact{}, nil
}
func (f *fakeArtifactService) GetBySignature(context.Context, string, string, string, string) (*contentmodel.Artifact, error) {
	return &contentmodel.Artifact{}, nil
}
func (f *fakeArtifactService) Upsert(context.Context, *contentmodel.Artifact) error { return nil }

type fakeRunQueryService struct{}

func (f *fakeRunQueryService) CreateParseRun(context.Context, *contentmodel.ParseRun) error {
	return nil
}
func (f *fakeRunQueryService) GetParseRunByID(context.Context, uuid.UUID) (*contentmodel.ParseRun, error) {
	artifactID := uuid.New()
	return &contentmodel.ParseRun{ArtifactID: &artifactID}, nil
}
func (f *fakeRunQueryService) ListParseRunsByDocumentID(context.Context, uuid.UUID, int) ([]*contentmodel.ParseRun, error) {
	artifactID := uuid.New()
	return []*contentmodel.ParseRun{{ArtifactID: &artifactID}}, nil
}
func (f *fakeRunQueryService) ListParseRunsByDatasetID(context.Context, uuid.UUID, int) ([]*contentmodel.ParseRun, error) {
	artifactID := uuid.New()
	return []*contentmodel.ParseRun{{ArtifactID: &artifactID}}, nil
}
func (f *fakeRunQueryService) GetLatestDatasetShadowSummary(_ context.Context, datasetID uuid.UUID, _ int) (*contentsvc.DatasetShadowSummary, error) {
	return &contentsvc.DatasetShadowSummary{
		DatasetID:       datasetID,
		DocumentCount:   1,
		SuccessCount:    1,
		LatestDocuments: []contentsvc.DatasetShadowDocumentSummary{{RunID: uuid.New(), Status: "succeeded"}},
	}, nil
}
func (f *fakeRunQueryService) CreateChunkingRun(context.Context, *contentmodel.ChunkingRun) error {
	return nil
}
func (f *fakeRunQueryService) ListChunkingRunsByParseRunID(context.Context, uuid.UUID) ([]*contentmodel.ChunkingRun, error) {
	return []*contentmodel.ChunkingRun{{}}, nil
}

var (
	_ contentsvc.ProviderAdminService = (*fakeProviderAdminService)(nil)
	_ contentsvc.PolicyAdminService   = (*fakePolicyAdminService)(nil)
	_ contentsvc.HealthService        = (*fakeHealthService)(nil)
	_ contentsvc.ArtifactService      = (*fakeArtifactService)(nil)
	_ contentsvc.RunQueryService      = (*fakeRunQueryService)(nil)
)

func TestRegisterInternalRoutes_RegisterExpectedPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api/internal")

	RegisterInternalRoutes(
		group,
		NewProviderHandler(&fakeProviderAdminService{}),
		NewPolicyHandler(&fakePolicyAdminService{}),
		NewHealthHandler(&fakeHealthService{}),
		NewArtifactHandler(&fakeArtifactService{}),
		NewRunHandler(&fakeRunQueryService{}, &fakeArtifactService{}),
		nil,
	)

	cases := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/console/api/internal/content-parse/providers", http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/policies", http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/artifacts/" + uuid.NewString(), http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/runs?document_id=" + uuid.NewString(), http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/runs/" + uuid.NewString(), http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/runs/" + uuid.NewString() + "/chunking", http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/shadow/documents/" + uuid.NewString(), http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/shadow/datasets/" + uuid.NewString(), http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/providers/" + uuid.NewString() + "/health", http.StatusOK},
		{http.MethodGet, "/console/api/internal/content-parse/providers/" + uuid.NewString() + "/health/latest", http.StatusOK},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("%s %s returned 404", tc.method, tc.path)
		}
		if w.Code != tc.want {
			t.Fatalf("%s %s returned %d want %d body=%s", tc.method, tc.path, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestRunHandlerListRunsRequiresDocumentOrDataset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api/internal")

	RegisterInternalRoutes(
		group,
		nil,
		nil,
		nil,
		nil,
		NewRunHandler(&fakeRunQueryService{}, &fakeArtifactService{}),
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/console/api/internal/content-parse/runs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestLatestDocumentShadowRouteRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api/internal")

	RegisterInternalRoutes(
		group,
		nil,
		nil,
		nil,
		nil,
		NewRunHandler(&fakeRunQueryService{}, &fakeArtifactService{}),
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/console/api/internal/content-parse/shadow/documents/"+uuid.NewString(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("shadow document route returned 404")
	}
}
