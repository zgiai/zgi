package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestDocumentAssetHandlerListAssetsScopesOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	svc := &fakeDocumentAssetService{
		views: []*service.DocumentAssetView{
			{
				ID:             assetID,
				OrganizationID: "org-1",
				Title:          "Handbook.pdf",
				SourceFileID:   "file-1",
				Status:         model.DocumentAssetStatusReady,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
		},
		total: 1,
	}
	router := newDocumentAssetTestRouter(svc, "org-1", "workspace-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets?limit=10", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if svc.lastFilter.OrganizationID != "org-1" || svc.lastFilter.WorkspaceID == nil || *svc.lastFilter.WorkspaceID != "workspace-1" {
		t.Fatalf("filter=%+v", svc.lastFilter)
	}

	var payload struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Total != 1 || len(payload.Data.Items) != 1 || payload.Data.Items[0].ID != assetID.String() {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerSyncFileCreatesArchivedAsset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	syncSvc := &fakeFileAssetSyncService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Handbook.pdf",
			SourceFileID:   "file-1",
			Status:         model.DocumentAssetStatusArchived,
		},
		created: true,
	}
	router := newDocumentAssetTestRouterWithSync(&fakeDocumentAssetService{}, syncSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/sync-file", bytes.NewBufferString(`{"file_id":"file-1"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if syncSvc.lastOrganizationID != "org-1" || syncSvc.lastFileID != "file-1" || syncSvc.lastCreatedBy != "account-1" {
		t.Fatalf("sync request org=%s file=%s by=%s", syncSvc.lastOrganizationID, syncSvc.lastFileID, syncSvc.lastCreatedBy)
	}

	var payload struct {
		Data struct {
			Created bool `json:"created"`
			Asset   struct {
				ID string `json:"id"`
			} `json:"asset"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Data.Created || payload.Data.Asset.ID != assetID.String() {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerSyncFileRequiresFileID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newDocumentAssetTestRouterWithSync(&fakeDocumentAssetService{}, &fakeFileAssetSyncService{}, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/sync-file", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerSyncFilesReturnsBatchResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	syncSvc := &fakeFileAssetSyncService{
		batchResult: &service.BatchFileAssetSyncResult{
			Items: []service.FileAssetSyncResult{
				{
					FileID:  "file-1",
					Created: true,
					Asset: &service.DocumentAssetView{
						ID:             assetID,
						OrganizationID: "org-1",
						SourceFileID:   "file-1",
					},
				},
				{
					FileID: "missing",
					Error:  "source file is missing",
				},
			},
			Total:        2,
			CreatedCount: 1,
			FailedCount:  1,
		},
	}
	router := newDocumentAssetTestRouterWithSync(&fakeDocumentAssetService{}, syncSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/sync-files", bytes.NewBufferString(`{"file_ids":["file-1","missing"]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if syncSvc.lastOrganizationID != "org-1" || syncSvc.lastCreatedBy != "account-1" {
		t.Fatalf("sync request org=%s by=%s", syncSvc.lastOrganizationID, syncSvc.lastCreatedBy)
	}
	if len(syncSvc.lastFileIDs) != 2 || syncSvc.lastFileIDs[0] != "file-1" || syncSvc.lastFileIDs[1] != "missing" {
		t.Fatalf("file ids=%+v", syncSvc.lastFileIDs)
	}

	var payload struct {
		Data struct {
			Total        int `json:"total"`
			CreatedCount int `json:"created_count"`
			FailedCount  int `json:"failed_count"`
			Items        []struct {
				FileID string `json:"file_id"`
				Error  string `json:"error"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Total != 2 || payload.Data.CreatedCount != 1 || payload.Data.FailedCount != 1 || payload.Data.Items[1].Error == "" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerSyncFilesRequiresFileIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newDocumentAssetTestRouterWithSync(&fakeDocumentAssetService{}, &fakeFileAssetSyncService{}, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/sync-files", bytes.NewBufferString(`{"file_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerGetAssetRejectsOtherOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             uuid.New(),
			OrganizationID: "org-2",
			Title:          "Other.pdf",
			SourceFileID:   "file-1",
		},
	}
	router := newDocumentAssetTestRouter(svc, "org-1", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets/"+svc.view.ID.String(), nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerGetAssetRejectsInvalidUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newDocumentAssetTestRouter(&fakeDocumentAssetService{}, "org-1", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets/not-a-uuid", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerPlansProcessingWithoutExecuting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
	}
	router := newDocumentAssetTestRouterWithSync(svc, nil, "org-1", "workspace-1", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/"+assetID.String()+"/processing-plan", bytes.NewBufferString(`{"target_level":"split"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Data struct {
			Plan struct {
				AssetID   string `json:"asset_id"`
				Target    string `json:"target_level"`
				WillParse bool   `json:"will_parse"`
				WillSplit bool   `json:"will_split"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Plan.AssetID != assetID.String() ||
		payload.Data.Plan.Target != model.DocumentProcessingLevelSplit ||
		!payload.Data.Plan.WillParse ||
		!payload.Data.Plan.WillSplit {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerProcessingPlanRejectsOtherOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-2",
			Title:          "Other",
			SourceFileID:   "file-1",
		},
	}
	router := newDocumentAssetTestRouter(svc, "org-1", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/"+assetID.String()+"/processing-plan", bytes.NewBufferString(`{"target_level":"parse"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerProcessingPlanRejectsInvalidTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
	}
	router := newDocumentAssetTestRouter(svc, "org-1", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/"+assetID.String()+"/processing-plan", bytes.NewBufferString(`{"target_level":"archive"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerCreatesPlannedProcessingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	workspaceID := "workspace-1"
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			WorkspaceID:    &workspaceID,
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
	}
	processingSvc := &fakeProcessingRequestService{
		view: &service.ProcessingRequestView{
			ID:             uuid.New(),
			OrganizationID: "org-1",
			WorkspaceID:    &workspaceID,
			AssetID:        assetID,
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusPlanned,
			Plan: &service.ProcessingRequestPlan{
				AssetID:     assetID,
				TargetLevel: model.DocumentProcessingLevelSplit,
				WillParse:   true,
				WillSplit:   true,
			},
		},
	}
	router := newDocumentAssetTestRouterWithProcessing(svc, nil, processingSvc, "org-1", "workspace-1", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/"+assetID.String()+"/processing-requests", bytes.NewBufferString(`{"target_level":"split","request_metadata":{"reason":"manual"}}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if processingSvc.lastRequest.OrganizationID != "org-1" ||
		processingSvc.lastRequest.WorkspaceID == nil ||
		*processingSvc.lastRequest.WorkspaceID != "workspace-1" ||
		processingSvc.lastRequest.AssetID != assetID ||
		processingSvc.lastRequest.TargetLevel != model.DocumentProcessingLevelSplit ||
		processingSvc.lastRequest.RequestedBy != "account-1" {
		t.Fatalf("request=%+v", processingSvc.lastRequest)
	}

	var payload struct {
		Data struct {
			ProcessingRequest struct {
				ID           string `json:"id"`
				Status       string `json:"status"`
				ExecutorKey  string `json:"executor_key"`
				AttemptCount int    `json:"attempt_count"`
				Plan         struct {
					WillParse bool `json:"will_parse"`
					WillSplit bool `json:"will_split"`
				} `json:"plan"`
			} `json:"processing_request"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.ProcessingRequest.ID == "" ||
		payload.Data.ProcessingRequest.Status != model.ProcessingRequestStatusPlanned ||
		!payload.Data.ProcessingRequest.Plan.WillParse ||
		!payload.Data.ProcessingRequest.Plan.WillSplit {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerProcessingRequestRejectsInvalidTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
	}
	processingSvc := &fakeProcessingRequestService{err: service.ErrProcessingLevelInvalid}
	router := newDocumentAssetTestRouterWithProcessing(svc, nil, processingSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/assets/"+assetID.String()+"/processing-requests", bytes.NewBufferString(`{"target_level":"archive"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerListsProcessingRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	requestID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
	}
	processingSvc := &fakeProcessingRequestService{
		views: []*service.ProcessingRequestView{
			{
				ID:             requestID,
				OrganizationID: "org-1",
				AssetID:        assetID,
				TargetLevel:    model.DocumentProcessingLevelParse,
				Status:         model.ProcessingRequestStatusRunning,
				ExecutorKey:    "parse-worker",
				AttemptCount:   1,
				Plan: &service.ProcessingRequestPlan{
					AssetID:     assetID,
					TargetLevel: model.DocumentProcessingLevelParse,
					WillParse:   true,
				},
			},
		},
		total: 1,
	}
	router := newDocumentAssetTestRouterWithProcessing(svc, nil, processingSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets/"+assetID.String()+"/processing-requests?status=running&executor_key=parse-worker&limit=10", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if processingSvc.lastFilter.OrganizationID != "org-1" ||
		processingSvc.lastFilter.AssetID != assetID ||
		processingSvc.lastFilter.Status != model.ProcessingRequestStatusRunning ||
		processingSvc.lastFilter.ExecutorKey != "parse-worker" ||
		processingSvc.lastFilter.Limit != 10 {
		t.Fatalf("filter=%+v", processingSvc.lastFilter)
	}

	var payload struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID           string `json:"id"`
				Status       string `json:"status"`
				ExecutorKey  string `json:"executor_key"`
				AttemptCount int    `json:"attempt_count"`
				Plan         struct {
					WillParse bool `json:"will_parse"`
				} `json:"plan"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Total != 1 ||
		len(payload.Data.Items) != 1 ||
		payload.Data.Items[0].ID != requestID.String() ||
		payload.Data.Items[0].Status != model.ProcessingRequestStatusRunning ||
		payload.Data.Items[0].ExecutorKey != "parse-worker" ||
		payload.Data.Items[0].AttemptCount != 1 ||
		!payload.Data.Items[0].Plan.WillParse {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerListsGlobalProcessingRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	processingSvc := &fakeProcessingRequestService{
		views: []*service.ProcessingRequestView{
			{
				ID:             requestID,
				OrganizationID: "org-1",
				AssetID:        uuid.New(),
				TargetLevel:    model.DocumentProcessingLevelVectorize,
				Status:         model.ProcessingRequestStatusQueued,
			},
		},
		total: 1,
	}
	router := newDocumentAssetTestRouterWithProcessing(&fakeDocumentAssetService{}, nil, processingSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/processing-requests?target_level=vectorize&status=queued&executor_key=vector-worker&limit=25", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if processingSvc.lastFilter.OrganizationID != "org-1" ||
		processingSvc.lastFilter.AssetID != uuid.Nil ||
		processingSvc.lastFilter.TargetLevel != model.DocumentProcessingLevelVectorize ||
		processingSvc.lastFilter.Status != model.ProcessingRequestStatusQueued ||
		processingSvc.lastFilter.ExecutorKey != "vector-worker" ||
		processingSvc.lastFilter.Limit != 25 {
		t.Fatalf("filter=%+v", processingSvc.lastFilter)
	}
}

func TestDocumentAssetHandlerSummarizesProcessingRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	processingSvc := &fakeProcessingRequestService{
		queueSummary: []service.ProcessingRequestQueueSummaryView{
			{
				TargetLevel: model.DocumentProcessingLevelSplit,
				Status:      model.ProcessingRequestStatusQueued,
				ExecutorKey: "split-worker",
				Count:       4,
			},
		},
	}
	router := newDocumentAssetTestRouterWithProcessing(&fakeDocumentAssetService{}, nil, processingSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/processing-requests/summary?target_level=split&status=queued&executor_key=split-worker", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if processingSvc.lastQueueSummaryFilter.OrganizationID != "org-1" ||
		processingSvc.lastQueueSummaryFilter.TargetLevel != model.DocumentProcessingLevelSplit ||
		processingSvc.lastQueueSummaryFilter.Status != model.ProcessingRequestStatusQueued ||
		processingSvc.lastQueueSummaryFilter.ExecutorKey != "split-worker" {
		t.Fatalf("filter=%+v", processingSvc.lastQueueSummaryFilter)
	}

	var payload struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				TargetLevel string `json:"target_level"`
				Status      string `json:"status"`
				ExecutorKey string `json:"executor_key"`
				Count       int64  `json:"count"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Total != 1 ||
		len(payload.Data.Items) != 1 ||
		payload.Data.Items[0].TargetLevel != model.DocumentProcessingLevelSplit ||
		payload.Data.Items[0].Status != model.ProcessingRequestStatusQueued ||
		payload.Data.Items[0].ExecutorKey != "split-worker" ||
		payload.Data.Items[0].Count != 4 {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerQueuesProcessingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	processingSvc := &fakeProcessingRequestService{
		view: &service.ProcessingRequestView{
			ID:             requestID,
			OrganizationID: "org-1",
			Status:         model.ProcessingRequestStatusQueued,
		},
	}
	router := newDocumentAssetTestRouterWithProcessing(&fakeDocumentAssetService{}, nil, processingSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/processing-requests/"+requestID.String()+"/queue", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if processingSvc.lastQueueOrganizationID != "org-1" || processingSvc.lastQueueID != requestID {
		t.Fatalf("queue org=%q id=%s", processingSvc.lastQueueOrganizationID, processingSvc.lastQueueID)
	}
}

func TestDocumentAssetHandlerClaimsProcessingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	processingSvc := &fakeProcessingRequestService{
		view: &service.ProcessingRequestView{
			ID:             requestID,
			OrganizationID: "org-1",
			Status:         model.ProcessingRequestStatusRunning,
			ExecutorKey:    "parse-worker",
		},
	}
	router := newDocumentAssetTestRouterWithProcessing(&fakeDocumentAssetService{}, nil, processingSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/processing-requests/claim", bytes.NewBufferString(`{"executor_key":"parse-worker"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if processingSvc.lastClaimOrganizationID != "org-1" || processingSvc.lastClaimExecutorKey != "parse-worker" {
		t.Fatalf("claim org=%q executor=%q", processingSvc.lastClaimOrganizationID, processingSvc.lastClaimExecutorKey)
	}
}

func TestDocumentAssetHandlerStartsCompletesAndFailsProcessingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	processingSvc := &fakeProcessingRequestService{
		view: &service.ProcessingRequestView{
			ID:             requestID,
			OrganizationID: "org-1",
		},
	}
	router := newDocumentAssetTestRouterWithProcessing(&fakeDocumentAssetService{}, nil, processingSvc, "org-1", "", "account-1")

	startResp := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/data-library/processing-requests/"+requestID.String()+"/start", bytes.NewBufferString(`{"executor_key":"parse-worker"}`))
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(startResp, startReq)
	if startResp.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", startResp.Code, startResp.Body.String())
	}
	if processingSvc.lastStartOrganizationID != "org-1" ||
		processingSvc.lastStartID != requestID ||
		processingSvc.lastStartExecutorKey != "parse-worker" {
		t.Fatalf("start org=%q id=%s executor=%q", processingSvc.lastStartOrganizationID, processingSvc.lastStartID, processingSvc.lastStartExecutorKey)
	}

	completeResp := httptest.NewRecorder()
	completeReq := httptest.NewRequest(http.MethodPost, "/data-library/processing-requests/"+requestID.String()+"/complete", bytes.NewBufferString(`{"execution_metadata":{"duration_ms":1200}}`))
	completeReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(completeResp, completeReq)
	if completeResp.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", completeResp.Code, completeResp.Body.String())
	}
	if processingSvc.lastCompleteOrganizationID != "org-1" ||
		processingSvc.lastCompleteID != requestID ||
		processingSvc.lastCompleteMetadata["duration_ms"] == nil {
		t.Fatalf("complete org=%q id=%s metadata=%+v", processingSvc.lastCompleteOrganizationID, processingSvc.lastCompleteID, processingSvc.lastCompleteMetadata)
	}

	failResp := httptest.NewRecorder()
	failReq := httptest.NewRequest(http.MethodPost, "/data-library/processing-requests/"+requestID.String()+"/fail", bytes.NewBufferString(`{"error_code":"parse_failed","error_message":"parser returned empty content","execution_metadata":{"stage":"parse"}}`))
	failReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(failResp, failReq)
	if failResp.Code != http.StatusOK {
		t.Fatalf("fail status=%d body=%s", failResp.Code, failResp.Body.String())
	}
	if processingSvc.lastFailOrganizationID != "org-1" ||
		processingSvc.lastFailID != requestID ||
		processingSvc.lastFailErrorCode != "parse_failed" ||
		processingSvc.lastFailErrorMessage != "parser returned empty content" ||
		processingSvc.lastFailMetadata["stage"] != "parse" {
		t.Fatalf("fail org=%q id=%s code=%q message=%q metadata=%+v", processingSvc.lastFailOrganizationID, processingSvc.lastFailID, processingSvc.lastFailErrorCode, processingSvc.lastFailErrorMessage, processingSvc.lastFailMetadata)
	}
}

func TestDocumentAssetHandlerCancelsProcessingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	processingSvc := &fakeProcessingRequestService{
		view: &service.ProcessingRequestView{
			ID:             requestID,
			OrganizationID: "org-1",
			Status:         model.ProcessingRequestStatusCancelled,
		},
	}
	router := newDocumentAssetTestRouterWithProcessing(&fakeDocumentAssetService{}, nil, processingSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/processing-requests/"+requestID.String()+"/cancel", bytes.NewBufferString(`{"reason":"user requested"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if processingSvc.lastCancelOrganizationID != "org-1" ||
		processingSvc.lastCancelID != requestID ||
		processingSvc.lastCancelReason != "user requested" {
		t.Fatalf("cancel org=%q id=%s reason=%q", processingSvc.lastCancelOrganizationID, processingSvc.lastCancelID, processingSvc.lastCancelReason)
	}
}

func TestDocumentAssetHandlerRetriesProcessingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	force := true
	processingSvc := &fakeProcessingRequestService{
		view: &service.ProcessingRequestView{
			ID:             uuid.New(),
			OrganizationID: "org-1",
			Status:         model.ProcessingRequestStatusPlanned,
		},
	}
	router := newDocumentAssetTestRouterWithProcessing(&fakeDocumentAssetService{}, nil, processingSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/processing-requests/"+requestID.String()+"/retry", bytes.NewBufferString(`{"force":true,"request_metadata":{"note":"retry"}}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if processingSvc.lastRetryOrganizationID != "org-1" ||
		processingSvc.lastRetryID != requestID ||
		processingSvc.lastRetryRequestedBy != "account-1" ||
		processingSvc.lastRetryForce == nil ||
		*processingSvc.lastRetryForce != force ||
		processingSvc.lastRetryMetadata["note"] != "retry" {
		t.Fatalf("retry org=%q id=%s requested_by=%q force=%v metadata=%+v", processingSvc.lastRetryOrganizationID, processingSvc.lastRetryID, processingSvc.lastRetryRequestedBy, processingSvc.lastRetryForce, processingSvc.lastRetryMetadata)
	}
}

func TestDocumentAssetHandlerListProcessingRequestsRejectsOtherOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-2",
			Title:          "Other",
			SourceFileID:   "file-1",
		},
	}
	router := newDocumentAssetTestRouterWithProcessing(svc, nil, &fakeProcessingRequestService{}, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets/"+assetID.String()+"/processing-requests", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerListReuseEventsScopesAssetAndOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	eventID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
		reuseEvents: []*service.ReuseEventView{
			{
				ID:             eventID,
				OrganizationID: "org-1",
				AssetID:        assetID,
				ConsumerType:   model.ReuseConsumerKnowledgeBase,
				ConsumerID:     "kb-1",
			},
		},
		reuseTotal: 1,
	}
	router := newDocumentAssetTestRouter(svc, "org-1", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets/"+assetID.String()+"/reuse-events?consumer_type=knowledge_base", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if svc.lastReuseFilter.OrganizationID != "org-1" ||
		svc.lastReuseFilter.AssetID == nil ||
		*svc.lastReuseFilter.AssetID != assetID ||
		svc.lastReuseFilter.ConsumerType != model.ReuseConsumerKnowledgeBase {
		t.Fatalf("reuse filter=%+v", svc.lastReuseFilter)
	}

	var payload struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Total != 1 || len(payload.Data.Items) != 1 || payload.Data.Items[0].ID != eventID.String() {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerListReuseEventsRejectsOtherOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-2",
			Title:          "Other",
			SourceFileID:   "file-1",
		},
	}
	router := newDocumentAssetTestRouter(svc, "org-1", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets/"+assetID.String()+"/reuse-events", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerListAssetKnowledgeBaseRefsScopesAssetAndOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	versionID := uuid.New()
	refID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
	}
	refSvc := &fakeKnowledgeBaseAssetRefService{
		views: []*service.KnowledgeBaseAssetRefView{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
				VersionID:      &versionID,
				Status:         model.KnowledgeBaseAssetRefStatusActive,
			},
		},
		total: 1,
	}
	router := newDocumentAssetTestRouterWithKnowledgeBaseRefs(svc, nil, nil, refSvc, "org-1", "", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets/"+assetID.String()+"/knowledge-base-refs?dataset_id=dataset-1&status=active", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if refSvc.lastFilter.OrganizationID != "org-1" ||
		refSvc.lastFilter.DatasetID != "dataset-1" ||
		refSvc.lastFilter.AssetID != assetID ||
		refSvc.lastFilter.Status != model.KnowledgeBaseAssetRefStatusActive {
		t.Fatalf("filter=%+v", refSvc.lastFilter)
	}

	var payload struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Total != 1 || len(payload.Data.Items) != 1 || payload.Data.Items[0].ID != refID.String() {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerListAssetKnowledgeBaseRefsRejectsOtherOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-2",
			Title:          "Other",
			SourceFileID:   "file-1",
		},
	}
	router := newDocumentAssetTestRouterWithKnowledgeBaseRefs(svc, nil, nil, &fakeKnowledgeBaseAssetRefService{}, "org-1", "", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/assets/"+assetID.String()+"/knowledge-base-refs", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerListKnowledgeBaseAssetRefsFiltersDatasetAndVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	versionID := uuid.New()
	refID := uuid.New()
	refSvc := &fakeKnowledgeBaseAssetRefService{
		views: []*service.KnowledgeBaseAssetRefView{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
				VersionID:      &versionID,
			},
		},
		total: 1,
	}
	router := newDocumentAssetTestRouterWithKnowledgeBaseRefs(&fakeDocumentAssetService{}, nil, nil, refSvc, "org-1", "", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/knowledge-base-asset-refs?dataset_id=dataset-1&asset_id="+assetID.String()+"&version_id="+versionID.String(), nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if refSvc.lastFilter.OrganizationID != "org-1" ||
		refSvc.lastFilter.DatasetID != "dataset-1" ||
		refSvc.lastFilter.AssetID != assetID ||
		refSvc.lastFilter.VersionID != versionID {
		t.Fatalf("filter=%+v", refSvc.lastFilter)
	}

	var payload struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Total != 1 || len(payload.Data.Items) != 1 || payload.Data.Items[0].ID != refID.String() {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerListKnowledgeBaseAssetRefsRejectsInvalidAssetID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newDocumentAssetTestRouterWithKnowledgeBaseRefs(&fakeDocumentAssetService{}, nil, nil, &fakeKnowledgeBaseAssetRefService{}, "org-1", "", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/knowledge-base-asset-refs?asset_id=bad-id", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerCreatesKnowledgeBaseAssetRef(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	versionID := uuid.New()
	chunkSetID := uuid.New()
	vectorID := uuid.New()
	refID := uuid.New()
	workspaceID := "workspace-1"
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			WorkspaceID:    &workspaceID,
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
		version: &model.DocumentVersion{
			ID:      versionID,
			AssetID: assetID,
		},
	}
	refSvc := &fakeKnowledgeBaseAssetRefService{
		view: &service.KnowledgeBaseAssetRefView{
			ID:                 refID,
			OrganizationID:     "org-1",
			WorkspaceID:        &workspaceID,
			DatasetID:          "dataset-1",
			AssetID:            assetID,
			VersionID:          &versionID,
			ChunkArtifactSetID: &chunkSetID,
			VectorArtifactID:   &vectorID,
			Status:             model.KnowledgeBaseAssetRefStatusActive,
		},
	}
	router := newDocumentAssetTestRouterWithKnowledgeBaseRefs(svc, nil, nil, refSvc, "org-1", workspaceID, "account-1")

	body := `{"dataset_id":"dataset-1","asset_id":"` + assetID.String() + `","version_id":"` + versionID.String() + `","chunk_artifact_set_id":"` + chunkSetID.String() + `","vector_artifact_id":"` + vectorID.String() + `","metadata_json":{"source":"picker"}}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/knowledge-base-asset-refs", bytes.NewBufferString(body))
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if refSvc.created == nil ||
		refSvc.created.OrganizationID != "org-1" ||
		refSvc.created.WorkspaceID == nil ||
		*refSvc.created.WorkspaceID != workspaceID ||
		refSvc.created.DatasetID != "dataset-1" ||
		refSvc.created.AssetID != assetID ||
		refSvc.created.VersionID == nil ||
		*refSvc.created.VersionID != versionID ||
		refSvc.created.ChunkArtifactSetID == nil ||
		*refSvc.created.ChunkArtifactSetID != chunkSetID ||
		refSvc.created.VectorArtifactID == nil ||
		*refSvc.created.VectorArtifactID != vectorID ||
		refSvc.created.CreatedBy != "account-1" ||
		refSvc.created.MetadataJSON["source"] != "picker" {
		t.Fatalf("created=%+v", refSvc.created)
	}

	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.ID != refID.String() {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestDocumentAssetHandlerCreateKnowledgeBaseAssetRefRejectsWrongVersionAsset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	versionID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
		version: &model.DocumentVersion{
			ID:      versionID,
			AssetID: uuid.New(),
		},
	}
	router := newDocumentAssetTestRouterWithKnowledgeBaseRefs(svc, nil, nil, &fakeKnowledgeBaseAssetRefService{}, "org-1", "", "account-1")

	body := `{"dataset_id":"dataset-1","asset_id":"` + assetID.String() + `","version_id":"` + versionID.String() + `"}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/knowledge-base-asset-refs", bytes.NewBufferString(body))
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerDisablesKnowledgeBaseAssetRef(t *testing.T) {
	gin.SetMode(gin.TestMode)
	refID := uuid.New()
	refSvc := &fakeKnowledgeBaseAssetRefService{
		view: &service.KnowledgeBaseAssetRefView{
			ID:             refID,
			OrganizationID: "org-1",
			Status:         model.KnowledgeBaseAssetRefStatusDisabled,
		},
	}
	router := newDocumentAssetTestRouterWithKnowledgeBaseRefs(&fakeDocumentAssetService{}, nil, nil, refSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/knowledge-base-asset-refs/"+refID.String()+"/disable", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if refSvc.lastDisableOrganizationID != "org-1" || refSvc.lastDisableID != refID {
		t.Fatalf("disable org=%s id=%s", refSvc.lastDisableOrganizationID, refSvc.lastDisableID)
	}
}

func TestDocumentAssetHandlerCreatesDatabaseAssetRef(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	versionID := uuid.New()
	parseArtifactID := uuid.New()
	extractionArtifactID := uuid.New()
	tableID := uuid.NewString()
	refID := uuid.New()
	workspaceID := "workspace-1"
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			WorkspaceID:    &workspaceID,
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
		version: &model.DocumentVersion{
			ID:              versionID,
			AssetID:         assetID,
			ParseArtifactID: &parseArtifactID,
		},
	}
	dbRefSvc := &fakeDatabaseAssetRefService{
		view: &service.DatabaseAssetRefView{
			ID:                   refID,
			OrganizationID:       "org-1",
			WorkspaceID:          &workspaceID,
			DataSourceID:         "database-1",
			TableID:              &tableID,
			AssetID:              assetID,
			VersionID:            versionID,
			ParseArtifactID:      &parseArtifactID,
			ExtractionArtifactID: &extractionArtifactID,
			Status:               model.DatabaseAssetRefStatusActive,
		},
	}
	router := newDocumentAssetTestRouterWithDatabaseRefs(svc, nil, nil, nil, dbRefSvc, "org-1", workspaceID, "account-1")

	body := `{"data_source_id":"database-1","table_id":"` + tableID + `","asset_id":"` + assetID.String() + `","version_id":"` + versionID.String() + `","parse_artifact_id":"` + parseArtifactID.String() + `","extraction_artifact_id":"` + extractionArtifactID.String() + `","metadata_json":{"schema_mapping":"draft"}}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/database-asset-refs", bytes.NewBufferString(body))
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if dbRefSvc.created == nil ||
		dbRefSvc.created.OrganizationID != "org-1" ||
		dbRefSvc.created.WorkspaceID == nil ||
		*dbRefSvc.created.WorkspaceID != workspaceID ||
		dbRefSvc.created.DataSourceID != "database-1" ||
		dbRefSvc.created.TableID == nil ||
		*dbRefSvc.created.TableID != tableID ||
		dbRefSvc.created.AssetID != assetID ||
		dbRefSvc.created.VersionID != versionID ||
		dbRefSvc.created.ParseArtifactID == nil ||
		*dbRefSvc.created.ParseArtifactID != parseArtifactID ||
		dbRefSvc.created.ExtractionArtifactID == nil ||
		*dbRefSvc.created.ExtractionArtifactID != extractionArtifactID ||
		dbRefSvc.created.CreatedBy != "account-1" ||
		dbRefSvc.created.MetadataJSON["schema_mapping"] != "draft" {
		t.Fatalf("created=%+v", dbRefSvc.created)
	}
}

func TestDocumentAssetHandlerCreateDatabaseAssetRefRejectsWrongParseArtifact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assetID := uuid.New()
	versionID := uuid.New()
	parseArtifactID := uuid.New()
	svc := &fakeDocumentAssetService{
		view: &service.DocumentAssetView{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
		version: &model.DocumentVersion{
			ID:              versionID,
			AssetID:         assetID,
			ParseArtifactID: &parseArtifactID,
		},
	}
	router := newDocumentAssetTestRouterWithDatabaseRefs(svc, nil, nil, nil, &fakeDatabaseAssetRefService{}, "org-1", "", "account-1")

	body := `{"data_source_id":"database-1","asset_id":"` + assetID.String() + `","version_id":"` + versionID.String() + `","parse_artifact_id":"` + uuid.NewString() + `"}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/data-library/database-asset-refs", bytes.NewBufferString(body))
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestDocumentAssetHandlerListAndDisableDatabaseAssetRefs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	refID := uuid.New()
	assetID := uuid.New()
	versionID := uuid.New()
	dbRefSvc := &fakeDatabaseAssetRefService{
		view: &service.DatabaseAssetRefView{
			ID:             refID,
			OrganizationID: "org-1",
			Status:         model.DatabaseAssetRefStatusDisabled,
		},
		views: []*service.DatabaseAssetRefView{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DataSourceID:   "database-1",
				AssetID:        assetID,
				VersionID:      versionID,
			},
		},
		total: 1,
	}
	router := newDocumentAssetTestRouterWithDatabaseRefs(&fakeDocumentAssetService{}, nil, nil, nil, dbRefSvc, "org-1", "", "account-1")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-library/database-asset-refs?data_source_id=database-1&asset_id="+assetID.String()+"&version_id="+versionID.String(), nil)
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	if dbRefSvc.lastFilter.OrganizationID != "org-1" ||
		dbRefSvc.lastFilter.DataSourceID != "database-1" ||
		dbRefSvc.lastFilter.AssetID != assetID ||
		dbRefSvc.lastFilter.VersionID != versionID {
		t.Fatalf("filter=%+v", dbRefSvc.lastFilter)
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/data-library/database-asset-refs/"+refID.String()+"/disable", nil)
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", resp.Code, resp.Body.String())
	}
	if dbRefSvc.lastDisableOrganizationID != "org-1" || dbRefSvc.lastDisableID != refID {
		t.Fatalf("disable org=%s id=%s", dbRefSvc.lastDisableOrganizationID, dbRefSvc.lastDisableID)
	}
}

func newDocumentAssetTestRouter(svc service.DocumentAssetService, organizationID string, workspaceID string) *gin.Engine {
	return newDocumentAssetTestRouterWithSync(svc, nil, organizationID, workspaceID, "")
}

func newDocumentAssetTestRouterWithSync(svc service.DocumentAssetService, syncSvc service.FileAssetSyncService, organizationID string, workspaceID string, accountID string) *gin.Engine {
	return newDocumentAssetTestRouterWithProcessing(svc, syncSvc, nil, organizationID, workspaceID, accountID)
}

func newDocumentAssetTestRouterWithProcessing(svc service.DocumentAssetService, syncSvc service.FileAssetSyncService, processingSvc service.ProcessingRequestService, organizationID string, workspaceID string, accountID string) *gin.Engine {
	return newDocumentAssetTestRouterWithKnowledgeBaseRefs(svc, syncSvc, processingSvc, nil, organizationID, workspaceID, accountID)
}

func newDocumentAssetTestRouterWithKnowledgeBaseRefs(svc service.DocumentAssetService, syncSvc service.FileAssetSyncService, processingSvc service.ProcessingRequestService, refSvc service.KnowledgeBaseAssetRefService, organizationID string, workspaceID string, accountID string) *gin.Engine {
	return newDocumentAssetTestRouterWithDatabaseRefs(svc, syncSvc, processingSvc, refSvc, nil, organizationID, workspaceID, accountID)
}

func newDocumentAssetTestRouterWithDatabaseRefs(svc service.DocumentAssetService, syncSvc service.FileAssetSyncService, processingSvc service.ProcessingRequestService, refSvc service.KnowledgeBaseAssetRefService, dbRefSvc service.DatabaseAssetRefService, organizationID string, workspaceID string, accountID string) *gin.Engine {
	router := gin.New()
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
	NewDocumentAssetHandler(svc, syncSvc, processingSvc, refSvc, dbRefSvc).RegisterRoutes(router.Group(""))
	return router
}

type fakeDocumentAssetService struct {
	view            *service.DocumentAssetView
	views           []*service.DocumentAssetView
	total           int64
	version         *model.DocumentVersion
	reuseEvents     []*service.ReuseEventView
	reuseTotal      int64
	lastFilter      repository.DocumentAssetListFilter
	lastReuseFilter repository.ReuseEventListFilter
}

func (s *fakeDocumentAssetService) CreateAsset(context.Context, *model.DocumentAsset) error {
	return nil
}

func (s *fakeDocumentAssetService) GetAssetByID(context.Context, uuid.UUID) (*model.DocumentAsset, error) {
	return nil, nil
}

func (s *fakeDocumentAssetService) GetAssetViewByID(context.Context, uuid.UUID) (*service.DocumentAssetView, error) {
	return s.view, nil
}

func (s *fakeDocumentAssetService) FindAssetBySourceFileID(context.Context, string, string) (*model.DocumentAsset, error) {
	return nil, nil
}

func (s *fakeDocumentAssetService) ListAssets(context.Context, repository.DocumentAssetListFilter) ([]*model.DocumentAsset, int64, error) {
	return nil, 0, nil
}

func (s *fakeDocumentAssetService) ListAssetViews(ctx context.Context, filter repository.DocumentAssetListFilter) ([]*service.DocumentAssetView, int64, error) {
	s.lastFilter = filter
	return s.views, s.total, nil
}

func (s *fakeDocumentAssetService) ListReuseEvents(ctx context.Context, filter repository.ReuseEventListFilter) ([]*service.ReuseEventView, int64, error) {
	s.lastReuseFilter = filter
	return s.reuseEvents, s.reuseTotal, nil
}

func (s *fakeDocumentAssetService) CreateVersion(context.Context, *model.DocumentVersion) error {
	return nil
}

func (s *fakeDocumentAssetService) GetVersionByID(context.Context, uuid.UUID) (*model.DocumentVersion, error) {
	return s.version, nil
}

func (s *fakeDocumentAssetService) ListVersionsByAssetID(context.Context, uuid.UUID) ([]*model.DocumentVersion, error) {
	return nil, nil
}

var _ service.DocumentAssetService = (*fakeDocumentAssetService)(nil)

type fakeFileAssetSyncService struct {
	view               *service.DocumentAssetView
	created            bool
	batchResult        *service.BatchFileAssetSyncResult
	err                error
	lastOrganizationID string
	lastFileID         string
	lastFileIDs        []string
	lastCreatedBy      string
}

func (s *fakeFileAssetSyncService) SyncFileAsArchivedAsset(ctx context.Context, organizationID string, fileID string, createdBy string) (*service.DocumentAssetView, bool, error) {
	s.lastOrganizationID = organizationID
	s.lastFileID = fileID
	s.lastCreatedBy = createdBy
	return s.view, s.created, s.err
}

func (s *fakeFileAssetSyncService) SyncFilesAsArchivedAssets(ctx context.Context, organizationID string, fileIDs []string, createdBy string) (*service.BatchFileAssetSyncResult, error) {
	s.lastOrganizationID = organizationID
	s.lastFileIDs = fileIDs
	s.lastCreatedBy = createdBy
	return s.batchResult, s.err
}

var _ service.FileAssetSyncService = (*fakeFileAssetSyncService)(nil)

type fakeProcessingRequestService struct {
	view                       *service.ProcessingRequestView
	views                      []*service.ProcessingRequestView
	queueSummary               []service.ProcessingRequestQueueSummaryView
	total                      int64
	err                        error
	lastRequest                service.ProcessingRequest
	lastFilter                 repository.ProcessingRequestListFilter
	lastQueueSummaryFilter     repository.ProcessingRequestQueueSummaryFilter
	lastEnqueueOrganizationID  string
	lastEnqueueID              uuid.UUID
	lastEnqueueExecutor        service.ProcessingRequestExecutor
	lastClaimOrganizationID    string
	lastClaimExecutorKey       string
	lastClaimExecutor          service.RegisteredProcessingRequestExecutor
	lastQueueOrganizationID    string
	lastQueueID                uuid.UUID
	lastStartOrganizationID    string
	lastStartID                uuid.UUID
	lastStartExecutorKey       string
	lastCompleteOrganizationID string
	lastCompleteID             uuid.UUID
	lastCompleteMetadata       map[string]any
	lastFailOrganizationID     string
	lastFailID                 uuid.UUID
	lastFailErrorCode          string
	lastFailErrorMessage       string
	lastFailMetadata           map[string]any
	lastCancelOrganizationID   string
	lastCancelID               uuid.UUID
	lastCancelReason           string
	lastRetryOrganizationID    string
	lastRetryID                uuid.UUID
	lastRetryRequestedBy       string
	lastRetryForce             *bool
	lastRetryMetadata          map[string]any
}

func (s *fakeProcessingRequestService) CreatePlannedRequest(ctx context.Context, req service.ProcessingRequest) (*service.ProcessingRequestView, error) {
	s.lastRequest = req
	return s.view, s.err
}

func (s *fakeProcessingRequestService) ListRequests(ctx context.Context, filter repository.ProcessingRequestListFilter) ([]*service.ProcessingRequestView, int64, error) {
	s.lastFilter = filter
	return s.views, s.total, s.err
}

func (s *fakeProcessingRequestService) QueueSummary(ctx context.Context, filter repository.ProcessingRequestQueueSummaryFilter) ([]service.ProcessingRequestQueueSummaryView, error) {
	s.lastQueueSummaryFilter = filter
	return s.queueSummary, s.err
}

func (s *fakeProcessingRequestService) EnqueueRequest(ctx context.Context, organizationID string, id uuid.UUID, executor service.ProcessingRequestExecutor) (*service.ProcessingRequestView, error) {
	s.lastEnqueueOrganizationID = organizationID
	s.lastEnqueueID = id
	s.lastEnqueueExecutor = executor
	return s.view, s.err
}

func (s *fakeProcessingRequestService) ClaimNextQueuedRequest(ctx context.Context, organizationID string, executorKey string) (*service.ProcessingRequestView, error) {
	s.lastClaimOrganizationID = organizationID
	s.lastClaimExecutorKey = executorKey
	return s.view, s.err
}

func (s *fakeProcessingRequestService) ClaimNextQueuedRequestForExecutor(ctx context.Context, organizationID string, executor service.RegisteredProcessingRequestExecutor) (*service.ProcessingRequestView, error) {
	s.lastClaimOrganizationID = organizationID
	s.lastClaimExecutor = executor
	if executor != nil {
		s.lastClaimExecutorKey = executor.Key()
	}
	return s.view, s.err
}

func (s *fakeProcessingRequestService) QueueRequest(ctx context.Context, organizationID string, id uuid.UUID) (*service.ProcessingRequestView, error) {
	s.lastQueueOrganizationID = organizationID
	s.lastQueueID = id
	return s.view, s.err
}

func (s *fakeProcessingRequestService) RetryRequest(ctx context.Context, organizationID string, id uuid.UUID, requestedBy string, force *bool, metadata map[string]any) (*service.ProcessingRequestView, error) {
	s.lastRetryOrganizationID = organizationID
	s.lastRetryID = id
	s.lastRetryRequestedBy = requestedBy
	s.lastRetryForce = force
	s.lastRetryMetadata = metadata
	return s.view, s.err
}

func (s *fakeProcessingRequestService) StartRequest(ctx context.Context, organizationID string, id uuid.UUID, executorKey string) (*service.ProcessingRequestView, error) {
	s.lastStartOrganizationID = organizationID
	s.lastStartID = id
	s.lastStartExecutorKey = executorKey
	return s.view, s.err
}

func (s *fakeProcessingRequestService) CompleteRequest(ctx context.Context, organizationID string, id uuid.UUID, metadata map[string]any) (*service.ProcessingRequestView, error) {
	s.lastCompleteOrganizationID = organizationID
	s.lastCompleteID = id
	s.lastCompleteMetadata = metadata
	return s.view, s.err
}

func (s *fakeProcessingRequestService) FailRequest(ctx context.Context, organizationID string, id uuid.UUID, errorCode string, errorMessage string, metadata map[string]any) (*service.ProcessingRequestView, error) {
	s.lastFailOrganizationID = organizationID
	s.lastFailID = id
	s.lastFailErrorCode = errorCode
	s.lastFailErrorMessage = errorMessage
	s.lastFailMetadata = metadata
	return s.view, s.err
}

func (s *fakeProcessingRequestService) CancelRequest(ctx context.Context, organizationID string, id uuid.UUID, reason string) (*service.ProcessingRequestView, error) {
	s.lastCancelOrganizationID = organizationID
	s.lastCancelID = id
	s.lastCancelReason = reason
	return s.view, s.err
}

var _ service.ProcessingRequestService = (*fakeProcessingRequestService)(nil)

type fakeKnowledgeBaseAssetRefService struct {
	view                      *service.KnowledgeBaseAssetRefView
	views                     []*service.KnowledgeBaseAssetRefView
	total                     int64
	err                       error
	created                   *model.KnowledgeBaseAssetRef
	lastFilter                repository.KnowledgeBaseAssetRefListFilter
	lastDisableOrganizationID string
	lastDisableID             uuid.UUID
}

func (s *fakeKnowledgeBaseAssetRefService) CreateRef(ctx context.Context, item *model.KnowledgeBaseAssetRef) (*service.KnowledgeBaseAssetRefView, error) {
	s.created = item
	return s.view, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) GetRefViewByID(ctx context.Context, id uuid.UUID) (*service.KnowledgeBaseAssetRefView, error) {
	return s.view, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) ListRefViews(ctx context.Context, filter repository.KnowledgeBaseAssetRefListFilter) ([]*service.KnowledgeBaseAssetRefView, int64, error) {
	s.lastFilter = filter
	return s.views, s.total, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) FindActiveRefView(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID, versionID uuid.UUID) (*service.KnowledgeBaseAssetRefView, error) {
	return s.view, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) FindActiveAssetRefView(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID) (*service.KnowledgeBaseAssetRefView, error) {
	return s.view, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) ListActiveAssetRefViews(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*service.KnowledgeBaseAssetRefView, error) {
	return s.views, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) DisableRef(ctx context.Context, organizationID string, id uuid.UUID) (*service.KnowledgeBaseAssetRefView, error) {
	s.lastDisableOrganizationID = organizationID
	s.lastDisableID = id
	return s.view, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) MarkRefPending(ctx context.Context, organizationID string, id uuid.UUID) (*service.KnowledgeBaseAssetRefView, uuid.UUID, error) {
	return s.view, uuid.New(), s.err
}

func (s *fakeKnowledgeBaseAssetRefService) MarkRefSyncing(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID) (*service.KnowledgeBaseAssetRefView, error) {
	return s.view, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) MarkRefSynced(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, datasetDocumentID uuid.UUID, generationNo int64) (*service.KnowledgeBaseAssetRefView, error) {
	return s.view, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) MarkRefFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*service.KnowledgeBaseAssetRefView, error) {
	return s.view, s.err
}

func (s *fakeKnowledgeBaseAssetRefService) RemoveRef(ctx context.Context, organizationID string, id uuid.UUID) (*service.KnowledgeBaseAssetRefView, error) {
	return s.view, s.err
}

var _ service.KnowledgeBaseAssetRefService = (*fakeKnowledgeBaseAssetRefService)(nil)

type fakeDatabaseAssetRefService struct {
	view                      *service.DatabaseAssetRefView
	views                     []*service.DatabaseAssetRefView
	total                     int64
	err                       error
	created                   *model.DatabaseAssetRef
	lastFilter                repository.DatabaseAssetRefListFilter
	lastDisableOrganizationID string
	lastDisableID             uuid.UUID
}

func (s *fakeDatabaseAssetRefService) CreateRef(ctx context.Context, item *model.DatabaseAssetRef) (*service.DatabaseAssetRefView, error) {
	s.created = item
	return s.view, s.err
}

func (s *fakeDatabaseAssetRefService) GetRefViewByID(ctx context.Context, id uuid.UUID) (*service.DatabaseAssetRefView, error) {
	return s.view, s.err
}

func (s *fakeDatabaseAssetRefService) ListRefViews(ctx context.Context, filter repository.DatabaseAssetRefListFilter) ([]*service.DatabaseAssetRefView, int64, error) {
	s.lastFilter = filter
	return s.views, s.total, s.err
}

func (s *fakeDatabaseAssetRefService) FindActiveRefView(ctx context.Context, organizationID string, dataSourceID string, tableID *string, assetID uuid.UUID, versionID uuid.UUID) (*service.DatabaseAssetRefView, error) {
	return s.view, s.err
}

func (s *fakeDatabaseAssetRefService) DisableRef(ctx context.Context, organizationID string, id uuid.UUID) (*service.DatabaseAssetRefView, error) {
	s.lastDisableOrganizationID = organizationID
	s.lastDisableID = id
	return s.view, s.err
}

var _ service.DatabaseAssetRefService = (*fakeDatabaseAssetRefService)(nil)
