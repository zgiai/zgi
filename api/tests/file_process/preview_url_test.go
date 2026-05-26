package file_process_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	filehandler "github.com/zgiai/zgi/api/internal/modules/file_process/handler"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestGetFileOriginalPreviewURL_TemporaryFileUsesCreatorAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
		previewURL     = "https://files.example/console/api/files/file-1/file-preview?sign=ok"
	)

	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: "00000000-0000-0000-0000-000000000000",
			Name:           "ava.png",
			Extension:      "png",
			MimeType:       "image/png",
			CreatedBy:      accountID,
			IsTemporary:    true,
		},
		url: previewURL,
	}
	router := newPreviewURLRouter(fileService, accountID, organizationID)

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/preview-url", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != "0" {
		t.Fatalf("code = %s, body = %s", resp.Code, rec.Body.String())
	}
	if resp.Data.URL != previewURL {
		t.Fatalf("url = %q, want %q", resp.Data.URL, previewURL)
	}
}

func TestGetFileOriginalPreviewURL_TemporaryFileRejectsOtherCreator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
	)

	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: "00000000-0000-0000-0000-000000000000",
			Name:           "ava.png",
			Extension:      "png",
			MimeType:       "image/png",
			CreatedBy:      "account-2",
			IsTemporary:    true,
		},
		url: "https://files.example/preview",
	}
	router := newPreviewURLRouter(fileService, accountID, organizationID)

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/preview-url", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestGetFileOriginalPreviewURL_SupportsGeneratedFileFormats(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		extension string
		mimeType  string
	}{
		{name: "html", extension: "html", mimeType: "text/html"},
		{name: "json", extension: "json", mimeType: "application/json"},
		{name: "docx", extension: "docx", mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{name: "xlsx", extension: "xlsx", mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const (
				accountID      = "account-1"
				organizationID = "organization-1"
				fileID         = "file-1"
				previewURL     = "https://files.example/preview"
			)

			fileService := &previewURLFileService{
				file: &dto.UploadFile{
					ID:             fileID,
					OrganizationID: "00000000-0000-0000-0000-000000000000",
					Name:           "generated." + tt.extension,
					Extension:      tt.extension,
					MimeType:       tt.mimeType,
					CreatedBy:      accountID,
					IsTemporary:    true,
				},
				url: previewURL,
			}
			router := newPreviewURLRouter(fileService, accountID, organizationID)

			req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/preview-url", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDownloadFile_TemporaryFileRejectsOtherCreator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
	)

	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: "00000000-0000-0000-0000-000000000000",
			Name:           "secret.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      "account-2",
			IsTemporary:    true,
		},
		content: []byte("a,b\n1,2\n"),
	}
	router := newFileRouter(fileService, accountID, organizationID, nil)

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/download", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestDownloadFile_RejectsCrossOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
	)

	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: "organization-2",
			Name:           "secret.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      accountID,
		},
		content: []byte("a,b\n1,2\n"),
	}
	router := newFileRouter(fileService, accountID, organizationID, nil)

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/download", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestDownloadFile_RejectsWorkspaceWithoutPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		workspaceID    = "workspace-1"
		fileID         = "file-1"
	)

	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID,
			WorkspaceID:    stringPtr(workspaceID),
			Name:           "secret.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      accountID,
		},
		content: []byte("a,b\n1,2\n"),
	}
	router := newFileRouter(fileService, accountID, organizationID, &previewURLOrganizationService{
		allowed: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/download", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestDownloadFile_ReturnsAttachmentWithUTF8Filename(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
	)

	content := []byte("a,b\n1,2\n")
	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID,
			Name:           "报告.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      accountID,
			Size:           int64(len(content)),
		},
		content: content,
	}
	router := newFileRouter(fileService, accountID, organizationID, nil)

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/download", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(content) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), string(content))
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, `attachment; filename="`) {
		t.Fatalf("Content-Disposition = %q, want attachment fallback filename", disposition)
	}
	if !strings.Contains(disposition, `filename*=UTF-8''%E6%8A%A5%E5%91%8A.csv`) {
		t.Fatalf("Content-Disposition = %q, want UTF-8 filename*", disposition)
	}
}

func TestFilePreview_AllowsJWTWithoutSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
	)
	content := []byte("a,b\n1,2\n")
	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID,
			Name:           "table.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      accountID,
			Size:           int64(len(content)),
		},
		content: content,
	}
	router := newImagePreviewRouter(fileService, accountID, organizationID, nil)

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/file-preview", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(content) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), string(content))
	}
}

func TestFilePreview_JWTRejectsTemporaryFileFromOtherCreator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
	)
	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: "00000000-0000-0000-0000-000000000000",
			Name:           "secret.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      "account-2",
			IsTemporary:    true,
		},
		content: []byte("a,b\n1,2\n"),
	}
	router := newImagePreviewRouter(fileService, accountID, organizationID, nil)

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/file-preview", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestFilePreview_JWTRejectsCrossOrganizationFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
	)
	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: "organization-2",
			Name:           "secret.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      accountID,
		},
		content: []byte("a,b\n1,2\n"),
	}
	router := newImagePreviewRouter(fileService, accountID, organizationID, nil)

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/file-preview", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestFilePreview_JWTRejectsWorkspaceWithoutDownloadPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		workspaceID    = "workspace-1"
		fileID         = "file-1"
	)
	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID,
			WorkspaceID:    stringPtr(workspaceID),
			Name:           "secret.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      accountID,
		},
		content: []byte("a,b\n1,2\n"),
	}
	router := newImagePreviewRouter(fileService, accountID, organizationID, &previewURLOrganizationService{
		allowed: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/file-preview", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestFilePreview_SignedURLBypassesJWTFileAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		App: appconfig.AppConfig{
			FilesURL:           "https://files.example",
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	defer func() {
		appconfig.GlobalConfig = previous
	}()

	const (
		accountID      = "account-1"
		organizationID = "organization-1"
		fileID         = "file-1"
	)
	content := []byte("a,b\n1,2\n")
	fileService := &previewURLFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: "organization-2",
			Name:           "signed.csv",
			Extension:      "csv",
			MimeType:       "text/csv",
			CreatedBy:      "account-2",
			Size:           int64(len(content)),
		},
		content: content,
	}
	router := newImagePreviewRouter(fileService, accountID, organizationID, &previewURLOrganizationService{
		allowed: false,
	})
	signedURL, err := util.GetSignedFileURLWithConfig(fileID, "https://files.example", "test-secret")
	if err != nil {
		t.Fatalf("create signed url: %v", err)
	}
	query := signedURL[strings.Index(signedURL, "?"):]

	req := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/file-preview"+query, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(content) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), string(content))
	}
}

func newPreviewURLRouter(fileService interfaces.FileService, accountID, organizationID string) *gin.Engine {
	return newFileRouter(fileService, accountID, organizationID, nil)
}

func newImagePreviewRouter(fileService interfaces.FileService, accountID, organizationID string, organizationService interfaces.OrganizationService) *gin.Engine {
	router := gin.New()
	handler := filehandler.NewImagePreviewHandler(fileService, nil, organizationService)
	router.GET("/files/:file_id/file-preview", func(c *gin.Context) {
		c.Set("auth_method", "jwt")
		c.Set("account_id", accountID)
		util.SetOrganizationID(c, organizationID)
		handler.GetFilePreview(c)
	})
	return router
}

func newFileRouter(fileService interfaces.FileService, accountID, organizationID string, organizationService interfaces.OrganizationService) *gin.Engine {
	router := gin.New()
	handler := filehandler.NewFileHandler(fileService, nil, nil, nil, organizationService)
	router.GET("/files/:file_id/preview-url", func(c *gin.Context) {
		c.Set("account_id", accountID)
		util.SetOrganizationID(c, organizationID)
		handler.GetFileOriginalPreviewURL(c)
	})
	router.GET("/files/:file_id/download", func(c *gin.Context) {
		c.Set("account_id", accountID)
		util.SetOrganizationID(c, organizationID)
		handler.DownloadFile(c)
	})
	return router
}

type previewURLFileService struct {
	file    *dto.UploadFile
	url     string
	content []byte
}

func (s *previewURLFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return &interfaces.FileUploadConfigResponse{}
}

func (s *previewURLFileService) UploadFile(context.Context, string, []byte, string, string, string, filemodel.CreatedByRole, *interfaces.FileSource, *string, bool, bool) (*dto.UploadFile, error) {
	return nil, nil
}

func (s *previewURLFileService) GetFilePreview(context.Context, string) (string, error) {
	return "", nil
}

func (s *previewURLFileService) GetFilePreviewWithOCR(context.Context, string, bool) (string, error) {
	return "", nil
}

func (s *previewURLFileService) GetFile(context.Context, string) (string, error) {
	return "", nil
}

func (s *previewURLFileService) ExtractFileWithSetting(context.Context, string, interfaces.FileExtractionSetting) (string, error) {
	return "", nil
}

func (s *previewURLFileService) GetSupportedFileTypes() []string {
	return nil
}

func (s *previewURLFileService) IsFileSizeWithinLimit(string, int64) bool {
	return true
}

func (s *previewURLFileService) ParseFileContent(context.Context, string) {}

func (s *previewURLFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return s.file, nil
}

func (s *previewURLFileService) DownloadFile(context.Context, string) ([]byte, error) {
	return s.content, nil
}

func (s *previewURLFileService) ListFiles(context.Context, string, string, *dto.FileListRequest, []string) (*dto.FileListResponse, error) {
	return nil, nil
}

func (s *previewURLFileService) ListArchivedFiles(context.Context, string, string, *dto.FileListRequest, []string) (*dto.FileListResponse, error) {
	return nil, nil
}

func (s *previewURLFileService) GetStorageUsage(context.Context, string) (int64, error) {
	return 0, nil
}

func (s *previewURLFileService) DeleteFiles(context.Context, []string) error {
	return nil
}

func (s *previewURLFileService) UpdateContentText(context.Context, string, string) error {
	return nil
}

func (s *previewURLFileService) CleanupExpiredTemporaryFiles(context.Context, time.Duration) (int64, error) {
	return 0, nil
}

func (s *previewURLFileService) GetFileURL(context.Context, string) (string, error) {
	return s.url, nil
}

type previewURLOrganizationService struct {
	interfaces.OrganizationService

	allowed bool
}

func (s *previewURLOrganizationService) CheckWorkspacePermission(context.Context, string, string, string, workspace_model.WorkspacePermissionCode) (bool, error) {
	return s.allowed, nil
}

func stringPtr(value string) *string {
	return &value
}
