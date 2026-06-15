package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func newKnowledgeBaseFileRefTestRouter(refSvc service.KnowledgeBaseFileRefService, enqueuer datasetFileRefSyncEnqueuer, organizationID string, workspaceID string, accountID string, allowManage bool) *gin.Engine {
	router := gin.New()
	handler := NewKnowledgeBaseFileRefHandler(
		refSvc,
		enqueuer,
		nil,
		nil,
		&fakeKnowledgeBaseFileRefDatasetReader{workspaceID: workspaceID},
		&fakeKnowledgeBaseFileRefOrganizationService{allow: allowManage},
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
	return router
}

type fakeKnowledgeBaseFileRefHandlerService struct {
	createResult *service.KnowledgeBaseFileRefCreateResult
	failedReq    service.KnowledgeBaseFileRefSyncFailureRequest
	createCalled bool
}

func (s *fakeKnowledgeBaseFileRefHandlerService) ListCandidates(ctx context.Context, req service.KnowledgeBaseFileCandidateRequest) (*service.KnowledgeBaseFileCandidateResult, error) {
	return nil, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) ListRefs(ctx context.Context, req service.KnowledgeBaseFileRefListRequest) (*service.KnowledgeBaseFileRefListResult, error) {
	return nil, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) CreateRefs(ctx context.Context, req service.KnowledgeBaseFileRefCreateRequest) (*service.KnowledgeBaseFileRefCreateResult, error) {
	s.createCalled = true
	return s.createResult, nil
}

func (s *fakeKnowledgeBaseFileRefHandlerService) GenerateCandidateEmbeddings(ctx context.Context, req service.KnowledgeBaseFileCandidateEmbeddingRequest) (*service.KnowledgeBaseFileCandidateEmbeddingResult, error) {
	return nil, nil
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
	err error
}

func (e *fakeDatasetRefSyncEnqueuer) EnqueueDatasetRefSync(ctx context.Context, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error {
	return e.err
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
