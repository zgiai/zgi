package external

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
)

func TestExternalValidateFileAccessAllowsCurrentAPIKeyTemporaryFile(t *testing.T) {
	handler := newExternalFileAccessHandler(map[string]*dto.UploadFile{
		"file-1": externalTemporaryFile("file-1", "api-key-1"),
	})

	err := handler.validateFileAccess(context.Background(), "file-1", "workspace-1", "api-key-1")
	if err != nil {
		t.Fatalf("validateFileAccess returned error: %v", err)
	}
}

func TestExternalValidateFileAccessRejectsForeignTemporaryFile(t *testing.T) {
	handler := newExternalFileAccessHandler(map[string]*dto.UploadFile{
		"file-1": externalTemporaryFile("file-1", "api-key-2"),
	})

	err := handler.validateFileAccess(context.Background(), "file-1", "workspace-1", "api-key-1")
	if err == nil {
		t.Fatalf("validateFileAccess error = nil, want foreign API key rejection")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Fatalf("validateFileAccess error = %q, want API key access error", err.Error())
	}
}

func TestExternalValidateFileAccessRejectsForeignTenantFile(t *testing.T) {
	handler := newExternalFileAccessHandler(map[string]*dto.UploadFile{
		"file-1": {
			ID:             "file-1",
			TenantID:       "workspace-2",
			OrganizationID: "workspace-2",
			CreatedBy:      "api-key-1",
		},
	})

	err := handler.validateFileAccess(context.Background(), "file-1", "workspace-1", "api-key-1")
	if err == nil {
		t.Fatalf("validateFileAccess error = nil, want tenant rejection")
	}
	if !strings.Contains(err.Error(), "tenant") {
		t.Fatalf("validateFileAccess error = %q, want tenant access error", err.Error())
	}
}

func TestExternalValidateAndProcessFileVariablesChecksOwnerWithoutExtractor(t *testing.T) {
	handler := newExternalFileAccessHandler(map[string]*dto.UploadFile{
		"foreign-file": externalTemporaryFile("foreign-file", "api-key-2"),
	})
	inputs := map[string]interface{}{
		"document": map[string]interface{}{
			"upload_file_id": "foreign-file",
		},
	}

	err := handler.validateAndProcessFileVariables(context.Background(), inputs, "workspace-1", "api-key-1")
	if err == nil {
		t.Fatalf("validateAndProcessFileVariables error = nil, want foreign API key rejection")
	}
	if !strings.Contains(err.Error(), "document") || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("validateAndProcessFileVariables error = %q, want variable and API key context", err.Error())
	}
}

func TestExternalValidateAndProcessFileVariablesAllowsCurrentAPIKeyFilesWithoutExtractor(t *testing.T) {
	handler := newExternalFileAccessHandler(map[string]*dto.UploadFile{
		"file-1": externalTemporaryFile("file-1", "api-key-1"),
		"file-2": externalTemporaryFile("file-2", "api-key-1"),
	})
	inputs := map[string]interface{}{
		"document": map[string]interface{}{
			"upload_file_id": "file-1",
		},
		"documents": []interface{}{
			map[string]interface{}{"upload_file_id": "file-1"},
			map[string]interface{}{"upload_file_id": "file-2"},
		},
	}

	err := handler.validateAndProcessFileVariables(context.Background(), inputs, "workspace-1", "api-key-1")
	if err != nil {
		t.Fatalf("validateAndProcessFileVariables returned error: %v", err)
	}
	if _, exists := inputs["document_content"]; exists {
		t.Fatalf("document_content should not be added when content extractor is unavailable: %#v", inputs)
	}
	if _, exists := inputs["documents_content"]; exists {
		t.Fatalf("documents_content should not be added when content extractor is unavailable: %#v", inputs)
	}
}

func TestExternalStopWorkflowTaskUsesAPIKeyScope(t *testing.T) {
	workflowService := &externalWorkflowStopService{}
	handler := &ExternalWorkflowHandler{workflowService: workflowService}
	keyInfo := &middleware.APIKeyInfo{
		ID:       uuid.New(),
		AgentID:  uuid.New(),
		TenantID: uuid.New(),
	}
	ctx, recorder := newExternalWorkflowStopContext("task-1", keyInfo, `{"user":"api-user"}`)

	handler.StopWorkflowTask(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !workflowService.stopCalled {
		t.Fatalf("expected StopWorkflowTask to be called")
	}
	if !workflowService.validateCalled {
		t.Fatalf("expected ValidateExternalWorkflowRunAccess to be called")
	}
	if workflowService.validateWorkspaceID != keyInfo.TenantID.String() ||
		workflowService.validateAgentID != keyInfo.AgentID.String() ||
		workflowService.validateRunID != "task-1" ||
		workflowService.validateAPIKeyID != keyInfo.ID.String() {
		t.Fatalf("validate args = workspace:%q agent:%q run:%q key:%q, want API key workspace/agent/task/key",
			workflowService.validateWorkspaceID,
			workflowService.validateAgentID,
			workflowService.validateRunID,
			workflowService.validateAPIKeyID,
		)
	}
	if workflowService.stopTenantID != keyInfo.TenantID.String() ||
		workflowService.stopAgentID != keyInfo.AgentID.String() ||
		workflowService.stopTaskID != "task-1" ||
		workflowService.stopAccountID != keyInfo.ID.String() {
		t.Fatalf("stop args = tenant:%q agent:%q task:%q account:%q, want API key tenant/agent/task/key",
			workflowService.stopTenantID,
			workflowService.stopAgentID,
			workflowService.stopTaskID,
			workflowService.stopAccountID,
		)
	}
}

func TestExternalStopWorkflowTaskDeniesInaccessibleRun(t *testing.T) {
	workflowService := &externalWorkflowStopService{validateErr: errors.New("workflow run not found or access denied")}
	handler := &ExternalWorkflowHandler{workflowService: workflowService}
	keyInfo := &middleware.APIKeyInfo{
		ID:       uuid.New(),
		AgentID:  uuid.New(),
		TenantID: uuid.New(),
	}
	ctx, recorder := newExternalWorkflowStopContext("foreign-task", keyInfo, `{"user":"api-user"}`)

	handler.StopWorkflowTask(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if !workflowService.validateCalled {
		t.Fatalf("expected ValidateExternalWorkflowRunAccess to be called")
	}
	if workflowService.stopCalled {
		t.Fatalf("StopWorkflowTask should not be called when external run access is denied")
	}
	if !strings.Contains(recorder.Body.String(), "not found or not accessible") {
		t.Fatalf("body = %s, want inaccessible task message", recorder.Body.String())
	}
}

func TestExternalGetWorkflowRunDetailUsesAPIKeyRunScope(t *testing.T) {
	runID := uuid.NewString()
	workflowService := &externalWorkflowStopService{
		runDetail: &dto.WorkflowRunDetailResponse{ID: runID},
	}
	handler := &ExternalWorkflowHandler{workflowService: workflowService}
	keyInfo := &middleware.APIKeyInfo{
		ID:       uuid.New(),
		AgentID:  uuid.New(),
		TenantID: uuid.New(),
	}
	ctx, recorder := newExternalWorkflowRunDetailContext(runID, keyInfo)

	handler.GetWorkflowRunDetail(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !workflowService.validateCalled {
		t.Fatalf("expected ValidateExternalWorkflowRunAccess to be called")
	}
	if !workflowService.detailCalled {
		t.Fatalf("expected GetWorkflowRunDetail to be called")
	}
	if workflowService.validateWorkspaceID != keyInfo.TenantID.String() ||
		workflowService.validateAgentID != keyInfo.AgentID.String() ||
		workflowService.validateRunID != runID ||
		workflowService.validateAPIKeyID != keyInfo.ID.String() {
		t.Fatalf("validate args = workspace:%q agent:%q run:%q key:%q, want API key workspace/agent/run/key",
			workflowService.validateWorkspaceID,
			workflowService.validateAgentID,
			workflowService.validateRunID,
			workflowService.validateAPIKeyID,
		)
	}
}

func TestExternalGetWorkflowRunDetailDeniesInaccessibleRunBeforeDetailLookup(t *testing.T) {
	runID := uuid.NewString()
	workflowService := &externalWorkflowStopService{validateErr: errors.New("workflow run not found or access denied")}
	handler := &ExternalWorkflowHandler{workflowService: workflowService}
	keyInfo := &middleware.APIKeyInfo{
		ID:       uuid.New(),
		AgentID:  uuid.New(),
		TenantID: uuid.New(),
	}
	ctx, recorder := newExternalWorkflowRunDetailContext(runID, keyInfo)

	handler.GetWorkflowRunDetail(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if !workflowService.validateCalled {
		t.Fatalf("expected ValidateExternalWorkflowRunAccess to be called")
	}
	if workflowService.detailCalled {
		t.Fatalf("GetWorkflowRunDetail should not be called when external run access is denied")
	}
}

func newExternalFileAccessHandler(files map[string]*dto.UploadFile) *ExternalWorkflowHandler {
	return &ExternalWorkflowHandler{
		fileService: &externalFileAccessService{files: files},
	}
}

func newExternalWorkflowStopContext(taskID string, keyInfo *middleware.APIKeyInfo, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workflows/tasks/"+taskID+"/stop", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "task_id", Value: taskID}}
	ctx.Set("api_key_info", keyInfo)
	return ctx, recorder
}

func newExternalWorkflowRunDetailContext(runID string, keyInfo *middleware.APIKeyInfo) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workflows/runs/"+runID, nil)
	ctx.Params = gin.Params{{Key: "run_id", Value: runID}}
	ctx.Set("api_key_info", keyInfo)
	return ctx, recorder
}

func externalTemporaryFile(id string, createdBy string) *dto.UploadFile {
	return &dto.UploadFile{
		ID:             id,
		TenantID:       config.TempFileTenantID,
		OrganizationID: config.TempFileTenantID,
		IsTemporary:    true,
		CreatedByRole:  dto.CreatedByRoleEndUser,
		CreatedBy:      createdBy,
	}
}

type externalFileAccessService struct {
	files map[string]*dto.UploadFile
}

func (s *externalFileAccessService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return nil
}

func (s *externalFileAccessService) UploadFile(context.Context, string, []byte, string, string, string, filemodel.CreatedByRole, *interfaces.FileSource, *string, bool, bool) (*dto.UploadFile, error) {
	return nil, nil
}

func (s *externalFileAccessService) GetFilePreview(context.Context, string) (string, error) {
	return "", nil
}

func (s *externalFileAccessService) GetFilePreviewWithOCR(context.Context, string, bool) (string, error) {
	return "", nil
}

func (s *externalFileAccessService) GetFile(context.Context, string) (string, error) {
	return "", nil
}

func (s *externalFileAccessService) ExtractFileWithSetting(context.Context, string, interfaces.FileExtractionSetting) (string, error) {
	return "", nil
}

func (s *externalFileAccessService) GetSupportedFileTypes() []string {
	return nil
}

func (s *externalFileAccessService) IsFileSizeWithinLimit(string, int64) bool {
	return true
}

func (s *externalFileAccessService) ParseFileContent(context.Context, string) {}

func (s *externalFileAccessService) GetFileByID(_ context.Context, fileID string) (*dto.UploadFile, error) {
	file, ok := s.files[fileID]
	if !ok {
		return nil, errors.New("file not found")
	}
	return file, nil
}

func (s *externalFileAccessService) DownloadFile(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (s *externalFileAccessService) ListFiles(context.Context, string, string, *dto.FileListRequest, []string) (*dto.FileListResponse, error) {
	return nil, nil
}

func (s *externalFileAccessService) ListArchivedFiles(context.Context, string, string, *dto.FileListRequest, []string) (*dto.FileListResponse, error) {
	return nil, nil
}

func (s *externalFileAccessService) GetStorageUsage(context.Context, string) (int64, error) {
	return 0, nil
}

func (s *externalFileAccessService) DeleteFiles(context.Context, []string) error {
	return nil
}

func (s *externalFileAccessService) UpdateContentText(context.Context, string, string) error {
	return nil
}

func (s *externalFileAccessService) CleanupExpiredTemporaryFiles(context.Context, time.Duration) (int64, error) {
	return 0, nil
}

func (s *externalFileAccessService) GetFileURL(context.Context, string) (string, error) {
	return "", nil
}

type externalWorkflowStopService struct {
	externalWorkflowService

	validateErr         error
	validateCalled      bool
	validateWorkspaceID string
	validateAgentID     string
	validateRunID       string
	validateAPIKeyID    string
	stopErr             error
	stopCalled          bool
	stopTenantID        string
	stopAgentID         string
	stopTaskID          string
	stopAccountID       string
	detailCalled        bool
	detailTenantID      string
	detailAgentID       string
	detailRunID         string
	runDetail           *dto.WorkflowRunDetailResponse
}

func (s *externalWorkflowStopService) ValidateExternalWorkflowRunAccess(_ context.Context, workspaceID, agentID, runID, apiKeyID string) error {
	s.validateCalled = true
	s.validateWorkspaceID = workspaceID
	s.validateAgentID = agentID
	s.validateRunID = runID
	s.validateAPIKeyID = apiKeyID
	return s.validateErr
}

func (s *externalWorkflowStopService) StopWorkflowTask(_ context.Context, tenantID, agentID, taskID string, accountID string) error {
	s.stopCalled = true
	s.stopTenantID = tenantID
	s.stopAgentID = agentID
	s.stopTaskID = taskID
	s.stopAccountID = accountID
	return s.stopErr
}

func (s *externalWorkflowStopService) GetWorkflowRunDetail(_ context.Context, tenantID, agentID, runID string) (*dto.WorkflowRunDetailResponse, error) {
	s.detailCalled = true
	s.detailTenantID = tenantID
	s.detailAgentID = agentID
	s.detailRunID = runID
	if s.runDetail != nil {
		return s.runDetail, nil
	}
	return &dto.WorkflowRunDetailResponse{ID: runID}, nil
}
