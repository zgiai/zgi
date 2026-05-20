package v1

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	contentparse_model "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	contentparse_service "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	graphflow_model "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	dataset_indexing "github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

func TestDatasetGraphHandlerRejectsInvalidDatasetIDBeforePermissionCheck(t *testing.T) {
	router, datasetService, graphService := newDatasetGraphTestRouter(datasetGraphTestOptions{})

	req := httptest.NewRequest(http.MethodGet, "/datasets/not-a-uuid/graph", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "104005", body["code"])
	require.Zero(t, datasetService.getCalls)
	require.Zero(t, datasetService.checkCalls)
	require.Zero(t, graphService.calls)
}

func TestDatasetGraphHandlerRejectsDatasetFromAnotherOrganization(t *testing.T) {
	router, datasetService, graphService := newDatasetGraphTestRouter(datasetGraphTestOptions{
		dataset: &dataset_model.Dataset{
			ID:             "11111111-1111-1111-1111-111111111111",
			OrganizationID: "other-organization",
			WorkspaceID:    "workspace-1",
		},
		hasPermission: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/datasets/11111111-1111-1111-1111-111111111111/graph", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "202009", body["code"])
	require.Equal(t, 1, datasetService.getCalls)
	require.Zero(t, datasetService.checkCalls)
	require.Zero(t, graphService.calls)
}

func TestDatasetGraphHandlerRejectsUnauthorizedDatasetWorkspace(t *testing.T) {
	router, datasetService, graphService := newDatasetGraphTestRouter(datasetGraphTestOptions{
		dataset: &dataset_model.Dataset{
			ID:             "11111111-1111-1111-1111-111111111111",
			OrganizationID: "organization-1",
			WorkspaceID:    "workspace-1",
		},
		hasPermission: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/datasets/11111111-1111-1111-1111-111111111111/graph", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "202009", body["code"])
	require.Equal(t, 1, datasetService.getCalls)
	require.Equal(t, 1, datasetService.checkCalls)
	require.Equal(t, "workspace-1", datasetService.checkWorkspaceID)
	require.Zero(t, graphService.calls)
}

func TestDatasetGraphHandlerReturnsGraphAfterWorkspacePermissionCheck(t *testing.T) {
	router, datasetService, graphService := newDatasetGraphTestRouter(datasetGraphTestOptions{
		dataset: &dataset_model.Dataset{
			ID:             "11111111-1111-1111-1111-111111111111",
			OrganizationID: "organization-1",
			WorkspaceID:    "workspace-1",
		},
		hasPermission: true,
		graphResponse: &graphflow_model.GraphDataResponse{
			Nodes: []graphflow_model.GraphNode{
				{ID: "ent:1", Label: "Alice", Category: "Person"},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/datasets/11111111-1111-1111-1111-111111111111/graph", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "0", body["code"])
	require.Equal(t, 1, datasetService.getCalls)
	require.Equal(t, 1, datasetService.checkCalls)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", datasetService.checkDatasetID)
	require.Equal(t, "account-1", datasetService.checkAccountID)
	require.Equal(t, "workspace-1", datasetService.checkWorkspaceID)
	require.Equal(t, 1, graphService.calls)
}

func TestDatasetGraphHandlerReturnsNotFoundForMissingDataset(t *testing.T) {
	router, datasetService, graphService := newDatasetGraphTestRouter(datasetGraphTestOptions{
		getErr: errors.New("record not found"),
	})

	req := httptest.NewRequest(http.MethodGet, "/datasets/11111111-1111-1111-1111-111111111111/graph", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "202001", body["code"])
	require.Equal(t, 1, datasetService.getCalls)
	require.Zero(t, datasetService.checkCalls)
	require.Zero(t, graphService.calls)
}

func TestDatasetContentParseShadowQualityHandlerChecksDatasetPermission(t *testing.T) {
	router, datasetService, runService := newDatasetContentParseShadowQualityTestRouter(datasetGraphTestOptions{
		dataset: &dataset_model.Dataset{
			ID:             "11111111-1111-1111-1111-111111111111",
			OrganizationID: "organization-1",
			WorkspaceID:    "workspace-1",
		},
		hasPermission: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/datasets/11111111-1111-1111-1111-111111111111/content-parse/shadow-quality?limit=25", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "0", body["code"])
	require.Equal(t, 1, datasetService.getCalls)
	require.Equal(t, 1, datasetService.checkCalls)
	require.Equal(t, "workspace-1", datasetService.checkWorkspaceID)
	require.Equal(t, 1, runService.calls)
	require.Equal(t, 25, runService.limit)
}

func TestDatasetContentParseShadowQualityHandlerRejectsUnauthorizedDataset(t *testing.T) {
	router, datasetService, runService := newDatasetContentParseShadowQualityTestRouter(datasetGraphTestOptions{
		dataset: &dataset_model.Dataset{
			ID:             "11111111-1111-1111-1111-111111111111",
			OrganizationID: "organization-1",
			WorkspaceID:    "workspace-1",
		},
		hasPermission: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/datasets/11111111-1111-1111-1111-111111111111/content-parse/shadow-quality", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "202009", body["code"])
	require.Equal(t, 1, datasetService.getCalls)
	require.Equal(t, 1, datasetService.checkCalls)
	require.Zero(t, runService.calls)
}

func TestDatasetContentParseShadowSamplingHandlerChecksDatasetPermission(t *testing.T) {
	router, datasetService, sampler := newDatasetContentParseShadowSamplingTestRouter(datasetGraphTestOptions{
		dataset: &dataset_model.Dataset{
			ID:             "11111111-1111-1111-1111-111111111111",
			OrganizationID: "organization-1",
			WorkspaceID:    "workspace-1",
		},
		hasPermission: true,
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/datasets/11111111-1111-1111-1111-111111111111/content-parse/shadow-quality/sample",
		bytes.NewBufferString(`{"limit":3,"document_ids":["22222222-2222-2222-2222-222222222222"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "0", body["code"])
	require.Equal(t, 1, datasetService.getCalls)
	require.Equal(t, 1, datasetService.checkCalls)
	require.Equal(t, "workspace-1", datasetService.checkWorkspaceID)
	require.Equal(t, 1, sampler.calls)
	require.Equal(t, 3, sampler.limit)
	require.Equal(t, []string{"22222222-2222-2222-2222-222222222222"}, sampler.documentIDs)
}

func TestDatasetContentParseShadowSamplingHandlerRejectsUnauthorizedDataset(t *testing.T) {
	router, datasetService, sampler := newDatasetContentParseShadowSamplingTestRouter(datasetGraphTestOptions{
		dataset: &dataset_model.Dataset{
			ID:             "11111111-1111-1111-1111-111111111111",
			OrganizationID: "organization-1",
			WorkspaceID:    "workspace-1",
		},
		hasPermission: false,
	})

	req := httptest.NewRequest(http.MethodPost, "/datasets/11111111-1111-1111-1111-111111111111/content-parse/shadow-quality/sample", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	body := decodeJSONBody(t, rec)
	require.Equal(t, "202009", body["code"])
	require.Equal(t, 1, datasetService.getCalls)
	require.Equal(t, 1, datasetService.checkCalls)
	require.Zero(t, sampler.calls)
}

type datasetGraphTestOptions struct {
	dataset       *dataset_model.Dataset
	getErr        error
	checkErr      error
	hasPermission bool
	graphResponse *graphflow_model.GraphDataResponse
}

func newDatasetGraphTestRouter(opts datasetGraphTestOptions) (*gin.Engine, *fakeDatasetGraphPermissionService, *fakeDatasetGraphService) {
	gin.SetMode(gin.TestMode)

	datasetService := &fakeDatasetGraphPermissionService{
		dataset:       opts.dataset,
		getErr:        opts.getErr,
		checkErr:      opts.checkErr,
		hasPermission: opts.hasPermission,
	}
	graphService := &fakeDatasetGraphService{response: opts.graphResponse}

	router := gin.New()
	router.GET("/datasets/:dataset_id/graph", func(c *gin.Context) {
		c.Set("account_id", "account-1")
		c.Set("organization_id", "organization-1")
		c.Set("tenant_id", "organization-1")
		c.Next()
	}, newDatasetGraphHandler(datasetService, graphService))

	return router, datasetService, graphService
}

func newDatasetContentParseShadowQualityTestRouter(opts datasetGraphTestOptions) (*gin.Engine, *fakeDatasetGraphPermissionService, *fakeContentParseRunQueryService) {
	gin.SetMode(gin.TestMode)

	datasetService := &fakeDatasetGraphPermissionService{
		dataset:       opts.dataset,
		getErr:        opts.getErr,
		checkErr:      opts.checkErr,
		hasPermission: opts.hasPermission,
	}
	runService := &fakeContentParseRunQueryService{}

	router := gin.New()
	router.GET("/datasets/:dataset_id/content-parse/shadow-quality", func(c *gin.Context) {
		c.Set("account_id", "account-1")
		c.Set("organization_id", "organization-1")
		c.Set("tenant_id", "organization-1")
		c.Next()
	}, newDatasetContentParseShadowQualityHandler(datasetService, runService))

	return router, datasetService, runService
}

func newDatasetContentParseShadowSamplingTestRouter(opts datasetGraphTestOptions) (*gin.Engine, *fakeDatasetGraphPermissionService, *fakeContentParseShadowSampler) {
	gin.SetMode(gin.TestMode)

	datasetService := &fakeDatasetGraphPermissionService{
		dataset:       opts.dataset,
		getErr:        opts.getErr,
		checkErr:      opts.checkErr,
		hasPermission: opts.hasPermission,
	}
	sampler := &fakeContentParseShadowSampler{}

	router := gin.New()
	router.POST("/datasets/:dataset_id/content-parse/shadow-quality/sample", func(c *gin.Context) {
		c.Set("account_id", "account-1")
		c.Set("organization_id", "organization-1")
		c.Set("tenant_id", "organization-1")
		c.Next()
	}, newDatasetContentParseShadowSamplingHandler(datasetService, sampler))

	return router, datasetService, sampler
}

type fakeDatasetGraphPermissionService struct {
	dataset       *dataset_model.Dataset
	getErr        error
	checkErr      error
	hasPermission bool

	getCalls         int
	checkCalls       int
	checkDatasetID   string
	checkAccountID   string
	checkWorkspaceID string
}

func (s *fakeDatasetGraphPermissionService) GetDatasetByID(ctx context.Context, datasetID string) (*dataset_model.Dataset, error) {
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.dataset != nil {
		return s.dataset, nil
	}
	return &dataset_model.Dataset{
		ID:             datasetID,
		OrganizationID: "organization-1",
		WorkspaceID:    "workspace-1",
	}, nil
}

func (s *fakeDatasetGraphPermissionService) CheckDatasetPermission(ctx context.Context, datasetID, accountID, workspaceID string) (bool, error) {
	s.checkCalls++
	s.checkDatasetID = datasetID
	s.checkAccountID = accountID
	s.checkWorkspaceID = workspaceID
	if s.checkErr != nil {
		return false, s.checkErr
	}
	return s.hasPermission, nil
}

type fakeDatasetGraphService struct {
	response *graphflow_model.GraphDataResponse
	calls    int
}

func (s *fakeDatasetGraphService) GetGraphData(ctx context.Context, datasetID string) (*graphflow_model.GraphDataResponse, error) {
	s.calls++
	if s.response != nil {
		return s.response, nil
	}
	return &graphflow_model.GraphDataResponse{}, nil
}

type fakeContentParseRunQueryService struct {
	calls int
	limit int
}

func (s *fakeContentParseRunQueryService) CreateParseRun(context.Context, *contentparse_model.ParseRun) error {
	return nil
}

func (s *fakeContentParseRunQueryService) GetParseRunByID(context.Context, uuid.UUID) (*contentparse_model.ParseRun, error) {
	return nil, nil
}

func (s *fakeContentParseRunQueryService) ListParseRunsByDocumentID(context.Context, uuid.UUID, int) ([]*contentparse_model.ParseRun, error) {
	return nil, nil
}

func (s *fakeContentParseRunQueryService) ListParseRunsByDatasetID(context.Context, uuid.UUID, int) ([]*contentparse_model.ParseRun, error) {
	return nil, nil
}

func (s *fakeContentParseRunQueryService) GetLatestDatasetShadowSummary(_ context.Context, datasetID uuid.UUID, limit int) (*contentparse_service.DatasetShadowSummary, error) {
	s.calls++
	s.limit = limit
	return &contentparse_service.DatasetShadowSummary{
		DatasetID:     datasetID,
		DocumentCount: 1,
		Readiness: contentparse_service.DatasetShadowReadinessSummary{
			DocumentCount: 1,
			ReadyCount:    1,
			Decision:      "ready",
		},
	}, nil
}

func (s *fakeContentParseRunQueryService) CreateChunkingRun(context.Context, *contentparse_model.ChunkingRun) error {
	return nil
}

func (s *fakeContentParseRunQueryService) ListChunkingRunsByParseRunID(context.Context, uuid.UUID) ([]*contentparse_model.ChunkingRun, error) {
	return nil, nil
}

var _ contentparse_service.RunQueryService = (*fakeContentParseRunQueryService)(nil)

type fakeContentParseShadowSampler struct {
	calls          int
	limit          int
	documentIDs    []string
	datasetID      string
	organizationID string
}

func (s *fakeContentParseShadowSampler) StartContentParseShadowSampling(_ context.Context, datasetID, organizationID string, limit int, documentIDs []string) (*dataset_indexing.ContentParseShadowSamplingResult, error) {
	s.calls++
	s.datasetID = datasetID
	s.organizationID = organizationID
	s.limit = limit
	s.documentIDs = append([]string(nil), documentIDs...)
	return &dataset_indexing.ContentParseShadowSamplingResult{
		DatasetID:      datasetID,
		RequestedLimit: limit,
		ScheduledCount: 1,
	}, nil
}
