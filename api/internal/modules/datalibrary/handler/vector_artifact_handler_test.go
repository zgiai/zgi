package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"github.com/zgiai/ginext/internal/modules/datalibrary/repository"
	"github.com/zgiai/ginext/internal/modules/datalibrary/service"
	"github.com/zgiai/ginext/internal/util"
)

func TestVectorArtifactHandlerListsArtifacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	artifactID := uuid.New()
	assetID := uuid.New()
	versionID := uuid.New()
	chunkSetID := uuid.New()
	vectorSvc := &fakeVectorArtifactService{
		views: []*service.VectorArtifactView{
			{
				ID:                 artifactID,
				OrganizationID:     "org-1",
				AssetID:            assetID,
				VersionID:          versionID,
				ChunkArtifactSetID: chunkSetID,
				EmbeddingProvider:  "openai",
				EmbeddingModel:     "text-embedding-3-large",
				EmbeddingDimension: 3072,
				VectorCollection:   "data_library_vectors",
				VectorCount:        42,
				Status:             model.VectorArtifactStatusReady,
			},
		},
		total: 1,
	}
	router := newVectorArtifactTestRouter(vectorSvc, "org-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/vector-artifacts?asset_id="+assetID.String()+"&version_id="+versionID.String()+"&chunk_artifact_set_id="+chunkSetID.String()+"&status=ready&limit=10", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if vectorSvc.lastFilter.OrganizationID != "org-1" ||
		vectorSvc.lastFilter.AssetID != assetID ||
		vectorSvc.lastFilter.VersionID != versionID ||
		vectorSvc.lastFilter.ChunkArtifactSetID != chunkSetID ||
		vectorSvc.lastFilter.Status != model.VectorArtifactStatusReady ||
		vectorSvc.lastFilter.Limit != 10 {
		t.Fatalf("filter=%+v", vectorSvc.lastFilter)
	}

	var payload struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID                string `json:"id"`
				EmbeddingProvider string `json:"embedding_provider"`
				VectorCount       int64  `json:"vector_count"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Total != 1 ||
		len(payload.Data.Items) != 1 ||
		payload.Data.Items[0].ID != artifactID.String() ||
		payload.Data.Items[0].EmbeddingProvider != "openai" ||
		payload.Data.Items[0].VectorCount != 42 {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestVectorArtifactHandlerGetRejectsOtherOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	vectorSvc := &fakeVectorArtifactService{
		view: &service.VectorArtifactView{
			ID:             uuid.New(),
			OrganizationID: "org-2",
		},
	}
	router := newVectorArtifactTestRouter(vectorSvc, "org-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/vector-artifacts/"+vectorSvc.view.ID.String(), nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func newVectorArtifactTestRouter(vectorSvc service.VectorArtifactService, organizationID string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if organizationID != "" {
			util.SetOrganizationID(c, organizationID)
		}
		c.Next()
	})
	NewVectorArtifactHandler(vectorSvc, nil).RegisterRoutes(router.Group(""))
	return router
}

type fakeVectorArtifactService struct {
	view       *service.VectorArtifactView
	views      []*service.VectorArtifactView
	total      int64
	lastFilter repository.VectorArtifactListFilter
}

func (s *fakeVectorArtifactService) CreateArtifact(ctx context.Context, item *model.VectorArtifact) (*service.VectorArtifactView, error) {
	return s.view, nil
}

func (s *fakeVectorArtifactService) GetArtifactViewByID(ctx context.Context, id uuid.UUID) (*service.VectorArtifactView, error) {
	return s.view, nil
}

func (s *fakeVectorArtifactService) ListArtifactViews(ctx context.Context, filter repository.VectorArtifactListFilter) ([]*service.VectorArtifactView, int64, error) {
	s.lastFilter = filter
	return s.views, s.total, nil
}

func (s *fakeVectorArtifactService) LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*service.VectorArtifactView, error) {
	return s.view, nil
}

var _ service.VectorArtifactService = (*fakeVectorArtifactService)(nil)
