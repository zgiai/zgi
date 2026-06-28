package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	datasetModel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	workspaceModel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestKnowledgeBaseFileRefHandlerMarksRefFailedWhenCreateEnqueueFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	refID := uuid.New()
	assetID := uuid.New()
	syncRunID := uuid.New()
	refSvc := &fakeKnowledgeBaseFileRefHandlerService{
		createResult: &service.KnowledgeBaseFileRefCreateResult{
			Items: []*service.KnowledgeBaseFileRefCreateItem{
				{
					AssetID: assetID,
					Ref: &service.KnowledgeBaseAssetRefView{
						ID:        refID,
						DatasetID: "dataset-1",
					},
					SyncRunID:    &syncRunID,
					GenerationNo: 2,
					Success:      true,
				},
			},
		},
	}
	router := newKnowledgeBaseFileRefTestRouter(refSvc, &fakeDatasetRefSyncEnqueuer{err: errors.New("queue unavailable")}, "org-1", "workspace-1", "account-1", true)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/datasets/dataset-1/file-refs", bytes.NewBufferString(`{"asset_ids":["`+assetID.String()+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if refSvc.failedReq.RefID != refID ||
		refSvc.failedReq.SyncRunID != syncRunID ||
		refSvc.failedReq.ErrorCode != "enqueue_failed" ||
		refSvc.failedReq.ErrorMessage != "queue unavailable" ||
		refSvc.failedReq.WorkspaceID == nil ||
		*refSvc.failedReq.WorkspaceID != "workspace-1" {
		t.Fatalf("failed_req=%+v", refSvc.failedReq)
	}
}

func TestKnowledgeBaseFileRefHandlerRejectsCreateWithoutManagePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	refSvc := &fakeKnowledgeBaseFileRefHandlerService{}
	router := newKnowledgeBaseFileRefTestRouter(refSvc, nil, "org-1", "workspace-1", "account-1", false)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/datasets/dataset-1/file-refs", bytes.NewBufferString(`{"asset_ids":["`+uuid.NewString()+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if refSvc.createCalled {
		t.Fatal("CreateRefs should not be called without knowledge base manage permission")
	}
}

func TestKnowledgeBaseFileRefHandlerRejectsListCandidatesWithoutManagePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	refSvc := &fakeKnowledgeBaseFileRefHandlerService{}
	router := newKnowledgeBaseFileRefTestRouter(refSvc, nil, "org-1", "workspace-1", "account-1", false)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/datasets/dataset-1/file-candidates", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if refSvc.listCandidatesCalled {
		t.Fatal("ListCandidates should not be called without knowledge base manage permission")
	}
}

func TestKnowledgeBaseFileRefHandlerRejectsListRefsWithoutManagePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	refSvc := &fakeKnowledgeBaseFileRefHandlerService{}
	router := newKnowledgeBaseFileRefTestRouter(refSvc, nil, "org-1", "workspace-1", "account-1", false)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/datasets/dataset-1/file-refs", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if refSvc.listRefsCalled {
		t.Fatal("ListRefs should not be called without knowledge base manage permission")
	}
}

func TestKnowledgeBaseFileRefHandlerEnqueuesCandidateEmbeddingGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	refSvc := &fakeKnowledgeBaseFileRefHandlerService{
		generateErr: errors.New("GenerateCandidateEmbeddings should not be called synchronously"),
	}
	enqueuer := &fakeDatasetRefSyncEnqueuer{}
	router := newKnowledgeBaseFileRefTestRouter(refSvc, enqueuer, "org-1", "workspace-1", "account-1", true)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/datasets/dataset-1/file-candidates/"+assetID.String()+"/embeddings", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body struct {
		Code string `json:"code"`
		Data struct {
			AssetID           string `json:"asset_id"`
			Accepted          bool   `json:"accepted"`
			ProcessingRequest struct {
				ID          string `json:"id"`
				TargetLevel string `json:"target_level"`
				Status      string `json:"status"`
			} `json:"processing_request"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != "0" || body.Data.AssetID != assetID.String() || !body.Data.Accepted {
		t.Fatalf("unexpected body=%s", resp.Body.String())
	}
	if body.Data.ProcessingRequest.ID == "" ||
		body.Data.ProcessingRequest.TargetLevel != datalibModel.DocumentProcessingLevelVectorize ||
		body.Data.ProcessingRequest.Status != datalibModel.ProcessingRequestStatusQueued {
		t.Fatalf("processing_request=%+v", body.Data.ProcessingRequest)
	}
	if refSvc.generateCalled {
		t.Fatal("GenerateCandidateEmbeddings should not run in the request handler")
	}
	if enqueuer.embedding.AssetID != assetID ||
		enqueuer.embedding.OrganizationID != "org-1" ||
		enqueuer.embedding.WorkspaceID == nil ||
		*enqueuer.embedding.WorkspaceID != "workspace-1" ||
		enqueuer.embedding.DatasetID != "dataset-1" ||
		enqueuer.embedding.RequestedBy != "account-1" ||
		enqueuer.embedding.ProcessingRequestID.String() != body.Data.ProcessingRequest.ID {
		t.Fatalf("embedding enqueue=%+v", enqueuer.embedding)
	}
}

func newKnowledgeBaseFileRefTestRouter(refSvc service.KnowledgeBaseFileRefService, enqueuer datasetFileRefSyncEnqueuer, organizationID string, workspaceID string, accountID string, allowManage bool) *gin.Engine {
	router := gin.New()
	handler := NewKnowledgeBaseFileRefHandler(
		refSvc,
		enqueuer,
		nil,
		nil,
		&fakeKnowledgeBaseFileRefDatasetReader{workspaceID: workspaceID},
		&fakeKnowledgeBaseFileRefOrganizationService{allow: allowManage},
		&fakeKnowledgeBaseFileRefProcessingService{},
	)
	router.Use(func(c *gin.Context) {
		if organizationID != "" {
			util.SetOrganizationID(c, organizationID)
		}
		if workspaceID != "" {
			util.SetWorkspaceID(c, workspaceID)
		}
		if accountID != "" {
			c.Set("account_id", accountID)
		}
		c.Next()
	})
	router.POST("/datasets/:dataset_id/file-refs", handler.CreateFileRefs)
	router.GET("/datasets/:dataset_id/file-candidates", handler.ListFileCandidates)
	router.GET("/datasets/:dataset_id/file-refs", handler.ListFileRefs)
	router.POST("/datasets/:dataset_id/file-candidates/:asset_id/embeddings", handler.GenerateFileCandidateEmbeddings)
	return router
}

type fakeKnowledgeBaseFileRefHandlerService struct {
	createResult         *service.KnowledgeBaseFileRefCreateResult
	failedReq            service.KnowledgeBaseFileRefSyncFailureRequest
	createCalled         bool
	listCandidatesCalled bool
	listRefsCalled       bool
	generateErr          error
	generateCalled       bool
}

func (s *fakeKnowledgeBaseFileRefHandlerService) ListCandidates(ctx context.Context, req service.KnowledgeBaseFileCandidateRequest) (*service.KnowledgeBaseFileCandidateResult, error) {
	s.listCandidatesCalled = true
	return &service.KnowledgeBaseFileCandidateResult{}, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) ListRefs(ctx context.Context, req service.KnowledgeBaseFileRefListRequest) (*service.KnowledgeBaseFileRefListResult, error) {
	s.listRefsCalled = true
	return &service.KnowledgeBaseFileRefListResult{}, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) CreateRefs(ctx context.Context, req service.KnowledgeBaseFileRefCreateRequest) (*service.KnowledgeBaseFileRefCreateResult, error) {
	s.createCalled = true
	return s.createResult, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) GenerateCandidateEmbeddings(ctx context.Context, req service.KnowledgeBaseFileCandidateEmbeddingRequest) (*service.KnowledgeBaseFileCandidateEmbeddingResult, error) {
	s.generateCalled = true
	return nil, s.generateErr
}

func (s *fakeKnowledgeBaseFileRefHandlerService) GetRef(ctx context.Context, req service.KnowledgeBaseFileRefGetRequest) (*service.KnowledgeBaseAssetRefView, error) {
	return nil, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) RetryRef(ctx context.Context, req service.KnowledgeBaseFileRefRetryRequest) (*service.KnowledgeBaseFileRefCreateItem, error) {
	return nil, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) MarkRefSyncFailed(ctx context.Context, req service.KnowledgeBaseFileRefSyncFailureRequest) (*service.KnowledgeBaseAssetRefView, error) {
	s.failedReq = req
	return &service.KnowledgeBaseAssetRefView{
		ID:               req.RefID,
		DatasetID:        req.DatasetID,
		SyncStatus:       "failed",
		SyncRunID:        &req.SyncRunID,
		SyncErrorCode:    &req.ErrorCode,
		SyncErrorMessage: &req.ErrorMessage,
	}, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) RemoveRef(ctx context.Context, req service.KnowledgeBaseFileRefGetRequest) (*service.KnowledgeBaseAssetRefView, error) {
	return nil, nil
}

type fakeDatasetRefSyncEnqueuer struct {
	err       error
	embedding service.KnowledgeBaseFileCandidateEmbeddingRequest
}

func (e *fakeDatasetRefSyncEnqueuer) EnqueueDatasetRefSync(ctx context.Context, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error {
	return e.err
}

func (e *fakeDatasetRefSyncEnqueuer) EnqueueFileCandidateEmbedding(ctx context.Context, req service.KnowledgeBaseFileCandidateEmbeddingRequest) error {
	e.embedding = req
	return e.err
}

type fakeKnowledgeBaseFileRefProcessingService struct {
	created service.ProcessingRequest
	failed  uuid.UUID
}

func (s *fakeKnowledgeBaseFileRefProcessingService) CreatePlannedRequest(ctx context.Context, req service.ProcessingRequest) (*service.ProcessingRequestView, error) {
	s.created = req
	return &service.ProcessingRequestView{
		ID:              uuid.New(),
		OrganizationID:  req.OrganizationID,
		WorkspaceID:     req.WorkspaceID,
		AssetID:         req.AssetID,
		TargetLevel:     req.TargetLevel,
		Status:          datalibModel.ProcessingRequestStatusPlanned,
		RequestedBy:     req.RequestedBy,
		Force:           req.Force,
		RequestMetadata: req.RequestMetadata,
	}, nil
}

func (s *fakeKnowledgeBaseFileRefProcessingService) GetRequest(ctx context.Context, organizationID string, id uuid.UUID) (*service.ProcessingRequestView, error) {
	return &service.ProcessingRequestView{
		ID:             id,
		OrganizationID: organizationID,
		TargetLevel:    datalibModel.DocumentProcessingLevelVectorize,
		Status:         datalibModel.ProcessingRequestStatusRunning,
	}, nil
}

func (s *fakeKnowledgeBaseFileRefProcessingService) QueueRequest(ctx context.Context, organizationID string, id uuid.UUID) (*service.ProcessingRequestView, error) {
	return &service.ProcessingRequestView{
		ID:             id,
		OrganizationID: organizationID,
		TargetLevel:    datalibModel.DocumentProcessingLevelVectorize,
		Status:         datalibModel.ProcessingRequestStatusQueued,
	}, nil
}

func (s *fakeKnowledgeBaseFileRefProcessingService) FailRequest(ctx context.Context, organizationID string, id uuid.UUID, errorCode string, errorMessage string, metadata map[string]any) (*service.ProcessingRequestView, error) {
	s.failed = id
	return &service.ProcessingRequestView{
		ID:             id,
		OrganizationID: organizationID,
		Status:         datalibModel.ProcessingRequestStatusFailed,
		ErrorCode:      errorCode,
		ErrorMessage:   errorMessage,
	}, nil
}

type fakeKnowledgeBaseFileRefDatasetReader struct {
	workspaceID string
}

func (r *fakeKnowledgeBaseFileRefDatasetReader) GetDatasetByID(ctx context.Context, datasetID string) (*datasetModel.Dataset, error) {
	return &datasetModel.Dataset{
		ID:          datasetID,
		WorkspaceID: r.workspaceID,
	}, nil
}

type fakeKnowledgeBaseFileRefOrganizationService struct {
	allow bool
}

func (s *fakeKnowledgeBaseFileRefOrganizationService) CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspaceModel.WorkspacePermissionCode) (bool, error) {
	if permissionCode != workspaceModel.WorkspacePermissionKnowledgeBaseManage {
		return false, nil
	}
	return s.allow, nil
}
