package tool_file

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appconfig "github.com/zgiai/ginext/config"
)

func TestHTTPHandler_GetToolFile_ReturnsBinaryForSignedURL(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		App: appconfig.AppConfig{
			FilesURL:           "https://api.zgi.im",
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})

	db := openToolFileTestDB(t)
	manager := NewToolFileManager(db, newMemoryStorage())
	toolFile, err := manager.CreateFileByRaw(context.Background(), CreateFileByRawParams{
		UserID:   "user-1",
		TenantID: "workspace-1",
		FileData: []byte("png-bytes"),
		MimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("CreateFileByRaw returned error: %v", err)
	}

	fileSignature := NewFileSignature(appconfig.GlobalConfig)
	signedURL, err := fileSignature.SignToolFile(toolFile.ID, ".png")
	if err != nil {
		t.Fatalf("SignToolFile returned error: %v", err)
	}

	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	gin.SetMode(gin.TestMode)
	handler := NewHTTPHandler(manager)

	permanentURL, err := fileSignature.SignToolFileWithMode(toolFile.ID, ".png", ToolFileURLModePermanent)
	if err != nil {
		t.Fatalf("SignToolFileWithMode returned error: %v", err)
	}

	legacyTimestamp := fmt.Sprintf("%d", time.Now().Unix())
	legacyNonce := "legacy-test-nonce"
	legacySign := generateTestSignature(fmt.Sprintf("tool-file|%s|%s|%s", toolFile.ID, legacyTimestamp, legacyNonce), appconfig.GlobalConfig.App.SecretKey)
	legacyURL := fmt.Sprintf("https://api.zgi.im/console/api/files/tools/%s.png?timestamp=%s&nonce=%s&sign=%s", toolFile.ID, legacyTimestamp, legacyNonce, url.QueryEscape(legacySign))

	expiredExpiresAt := fmt.Sprintf("%d", time.Now().Add(-time.Hour).Unix())
	expiredNonce := "expired-test-nonce"
	expiredSign := generateTestSignature(fmt.Sprintf("tool-file|%s|%s|%s", toolFile.ID, expiredExpiresAt, expiredNonce), appconfig.GlobalConfig.App.SecretKey)
	expiredURL := fmt.Sprintf("https://api.zgi.im/console/api/files/tools/%s.png?expires_at=%s&nonce=%s&sign=%s", toolFile.ID, expiredExpiresAt, expiredNonce, url.QueryEscape(expiredSign))

	tamperedParsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("Parse signedURL returned error: %v", err)
	}
	tamperedQuery := tamperedParsed.Query()
	tamperedQuery.Set("expires_at", "0")
	tamperedParsed.RawQuery = tamperedQuery.Encode()

	testCases := []struct {
		name       string
		routePath  string
		requestURI string
		wantCode   int
	}{
		{
			name:       "console_api_path",
			routePath:  "/console/api/files/tools/:tool_file_id",
			requestURI: parsed.RequestURI(),
			wantCode:   http.StatusOK,
		},
		{
			name:       "console_api_path_permanent",
			routePath:  "/console/api/files/tools/:tool_file_id",
			requestURI: mustParseURL(t, permanentURL).RequestURI(),
			wantCode:   http.StatusOK,
		},
		{
			name:       "console_api_path_legacy_timestamp",
			routePath:  "/console/api/files/tools/:tool_file_id",
			requestURI: mustParseURL(t, legacyURL).RequestURI(),
			wantCode:   http.StatusOK,
		},
		{
			name:       "legacy_root_path_new_scheme",
			routePath:  "/files/tools/:tool_file_id",
			requestURI: strings.Replace(parsed.RequestURI(), "/console/api/files/tools/", "/files/tools/", 1),
			wantCode:   http.StatusOK,
		},
		{
			name:       "legacy_root_path_legacy_scheme",
			routePath:  "/files/tools/:tool_file_id",
			requestURI: strings.Replace(mustParseURL(t, legacyURL).RequestURI(), "/console/api/files/tools/", "/files/tools/", 1),
			wantCode:   http.StatusOK,
		},
		{
			name:       "missing_signature_rejected",
			routePath:  "/console/api/files/tools/:tool_file_id",
			requestURI: parsed.Path,
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "tampered_expiry_rejected",
			routePath:  "/console/api/files/tools/:tool_file_id",
			requestURI: tamperedParsed.RequestURI(),
			wantCode:   http.StatusNotFound,
		},
		{
			name:       "expired_new_scheme_rejected",
			routePath:  "/console/api/files/tools/:tool_file_id",
			requestURI: mustParseURL(t, expiredURL).RequestURI(),
			wantCode:   http.StatusNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.GET(tc.routePath, handler.GetToolFile)

			req := httptest.NewRequest(http.MethodGet, tc.requestURI, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.wantCode == http.StatusOK {
				if got := rec.Header().Get("Content-Type"); got != "image/png" {
					t.Fatalf("content-type = %q, want %q", got, "image/png")
				}
				if got := rec.Body.String(); got != "png-bytes" {
					t.Fatalf("body = %q, want %q", got, "png-bytes")
				}
			}
		})
	}
}

func TestHTTPHandler_GetToolFile_ReturnsUTF8CharsetForTextFiles(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		App: appconfig.AppConfig{
			FilesURL:           "https://api.zgi.im",
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})

	db := openToolFileTestDB(t)
	manager := NewToolFileManager(db, newMemoryStorage())
	toolFile, err := manager.CreateFileByRaw(context.Background(), CreateFileByRawParams{
		UserID:   "user-1",
		TenantID: "workspace-1",
		FileData: []byte("# Cafe\nnaive resume"),
		MimeType: "text/markdown",
		Filename: stringPtr("resume.md"),
	})
	if err != nil {
		t.Fatalf("CreateFileByRaw returned error: %v", err)
	}

	fileSignature := NewFileSignature(appconfig.GlobalConfig)
	signedURL, err := fileSignature.SignToolFile(toolFile.ID, ".md")
	if err != nil {
		t.Fatalf("SignToolFile returned error: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/console/api/files/tools/:tool_file_id", NewHTTPHandler(manager).GetToolFile)

	req := httptest.NewRequest(http.MethodGet, mustParseURL(t, signedURL+"&download=1").RequestURI(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("content-type = %q, want %q", got, "text/markdown; charset=utf-8")
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, `attachment; filename="resume.md"`) {
		t.Fatalf("content-disposition = %q, want attachment filename", got)
	}
	if got := rec.Body.String(); got != "# Cafe\nnaive resume" {
		t.Fatalf("body = %q, want UTF-8 markdown content", got)
	}
}

func TestToolFileResponseContentType_AttachesUTF8ForTextLikeTypes(t *testing.T) {
	testCases := []struct {
		name     string
		mimeType string
		want     string
	}{
		{name: "plain_text", mimeType: "text/plain", want: "text/plain; charset=utf-8"},
		{name: "markdown", mimeType: "text/markdown", want: "text/markdown; charset=utf-8"},
		{name: "csv", mimeType: "text/csv", want: "text/csv; charset=utf-8"},
		{name: "json", mimeType: "application/json", want: "application/json; charset=utf-8"},
		{name: "existing_charset", mimeType: "text/plain; charset=gbk", want: "text/plain; charset=gbk"},
		{name: "binary", mimeType: "image/png", want: "image/png"},
		{name: "empty", mimeType: "", want: "application/octet-stream"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolFileResponseContentType(tc.mimeType); got != tc.want {
				t.Fatalf("toolFileResponseContentType(%q) = %q, want %q", tc.mimeType, got, tc.want)
			}
		})
	}
}

func TestToolFileContentDisposition_UsesAttachmentWithUTF8Filename(t *testing.T) {
	disposition := toolFileContentDisposition("monthly report.md")
	if !strings.Contains(disposition, `attachment; filename="monthly report.md"`) {
		t.Fatalf("content-disposition = %q, want ascii filename", disposition)
	}
	if !strings.Contains(disposition, `filename*=utf-8''monthly%20report.md`) {
		t.Fatalf("content-disposition = %q, want utf-8 filename", disposition)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", raw, err)
	}
	return parsed
}

func stringPtr(value string) *string {
	return &value
}
