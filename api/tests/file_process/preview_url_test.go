package file_process_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/ginext/internal/dto"
	filehandler "github.com/zgiai/ginext/internal/modules/file_process/handler"
	filemodel "github.com/zgiai/ginext/internal/modules/file_process/model"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/util"
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

func newPreviewURLRouter(fileService interfaces.FileService, accountID, organizationID string) *gin.Engine {
	router := gin.New()
	handler := filehandler.NewFileHandler(fileService, nil, nil, nil, nil)
	router.GET("/files/:file_id/preview-url", func(c *gin.Context) {
		c.Set("account_id", accountID)
		util.SetOrganizationID(c, organizationID)
		handler.GetFileOriginalPreviewURL(c)
	})
	return router
}

type previewURLFileService struct {
	file *dto.UploadFile
	url  string
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
	return nil, nil
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
