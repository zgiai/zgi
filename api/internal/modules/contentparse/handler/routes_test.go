package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	contentmodel "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	contentsvc "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type fakeProviderAdminService struct{}

func (f *fakeProviderAdminService) GetByID(context.Context, uuid.UUID) (*contentmodel.ProviderConfig, error) {
	return &contentmodel.ProviderConfig{}, nil
}
func (f *fakeProviderAdminService) ListByScope(context.Context, string, *uuid.UUID, *uuid.UUID) ([]*contentmodel.ProviderConfig, error) {
	return []*contentmodel.ProviderConfig{}, nil
}
func (f *fakeProviderAdminService) Create(context.Context, *contentmodel.ProviderConfig) error {
	return nil
}

func (f *fakeProviderAdminService) UpsertByScopeAndKey(context.Context, *contentmodel.ProviderConfig) error {
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

type fakeProviderSettingsService struct {
	listCalled   bool
	upsertCalled bool
	checkCalled  bool
}

func (f *fakeProviderSettingsService) List(context.Context, uuid.UUID) (*contentsvc.ParserSettingsList, error) {
	f.listCalled = true
	return &contentsvc.ParserSettingsList{}, nil
}

func (f *fakeProviderSettingsService) Upsert(context.Context, uuid.UUID, *uuid.UUID, string, contentsvc.ParserSettingsInput) (*contentsvc.ParserProviderSettings, error) {
	f.upsertCalled = true
	return &contentsvc.ParserProviderSettings{}, nil
}

func (f *fakeProviderSettingsService) Check(context.Context, uuid.UUID, *uuid.UUID, string) (*contentsvc.ParserProviderSettings, error) {
	f.checkCalled = true
	return &contentsvc.ParserProviderSettings{}, nil
}

type providerSettingsAccountService struct {
	interfaces.AccountService

	allowed             bool
	authorizationCalled bool
	lastOrganizationID  string
	lastAccountID       string
}

func (s *providerSettingsAccountService) IsOrganizationAdminOrOwner(_ context.Context, organizationID, accountID string) (bool, error) {
	s.authorizationCalled = true
	s.lastOrganizationID = organizationID
	s.lastAccountID = accountID
	return s.allowed, nil
}

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

func TestProviderSettingsRoutesAllowReadWithoutOrganizationAdminOrOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.NewString()
	accountSvc := &providerSettingsAccountService{allowed: false}
	settingsSvc := &fakeProviderSettingsService{}

	router := newProviderSettingsRouteTestRouter(organizationID, accountSvc, settingsSvc)

	req := httptest.NewRequest(http.MethodGet, "/content-parse/provider-settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for provider settings read request, got %d, body=%s", w.Code, w.Body.String())
	}
	if accountSvc.authorizationCalled {
		t.Fatal("read request should not require organization admin authorization")
	}
	if !settingsSvc.listCalled {
		t.Fatal("provider settings service should be called for read request")
	}
}

func TestProviderSettingsRoutesRequireOrganizationAdminOrOwnerForWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.NewString()
	accountSvc := &providerSettingsAccountService{allowed: false}
	settingsSvc := &fakeProviderSettingsService{}

	router := newProviderSettingsRouteTestRouter(organizationID, accountSvc, settingsSvc)

	req := httptest.NewRequest(http.MethodPut, "/content-parse/provider-settings/reducto", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for non-admin provider settings write request, got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"403001"`) {
		t.Fatalf("expected permission denied error code 403001, got body=%s", w.Body.String())
	}
	if !accountSvc.authorizationCalled {
		t.Fatal("expected organization admin authorization check")
	}
	if accountSvc.lastOrganizationID != organizationID || accountSvc.lastAccountID != "acc-1" {
		t.Fatalf("authorization scope = (%q, %q), want (%q, acc-1)", accountSvc.lastOrganizationID, accountSvc.lastAccountID, organizationID)
	}
	if settingsSvc.upsertCalled {
		t.Fatal("provider settings service should not be called for non-admin write request")
	}
}

func TestProviderSettingsRoutesAllowOrganizationAdminOrOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.NewString()
	accountSvc := &providerSettingsAccountService{allowed: true}
	settingsSvc := &fakeProviderSettingsService{}

	router := newProviderSettingsRouteTestRouter(organizationID, accountSvc, settingsSvc)

	req := httptest.NewRequest(http.MethodPut, "/content-parse/provider-settings/reducto", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for admin provider settings request, got %d, body=%s", w.Code, w.Body.String())
	}
	if !accountSvc.authorizationCalled {
		t.Fatal("expected organization admin authorization check")
	}
	if !settingsSvc.upsertCalled {
		t.Fatal("expected provider settings service to be called for admin request")
	}
}

func newProviderSettingsRouteTestRouter(organizationID string, accountSvc *providerSettingsAccountService, settingsSvc *fakeProviderSettingsService) *gin.Engine {
	router := gin.New()
	group := router.Group("/content-parse")
	group.Use(func(c *gin.Context) {
		c.Set("account_id", "acc-1")
		c.Set("organization_id", organizationID)
		c.Set("tenant_id", organizationID)
		c.Set("account_service", accountSvc)
		c.Next()
	})
	NewProviderSettingsHandler(settingsSvc).RegisterRoutes(group)
	return router
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
