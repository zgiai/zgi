package external

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRuntimeWorkspaceRoutesExposePublishedAgentContract(t *testing.T) {
	t.Setenv("ZGI_RUNTIME_WORKSPACE_ENABLED", "true")
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRuntimeWorkspaceRoutes(engine.Group("/api"), RuntimeWorkspaceRouteDeps{DB: &gorm.DB{}, FileService: runtimeWorkspaceRouteFileService{}, AgentHandler: &agents.AgentsHandler{}})
	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/connect/v1/apps",
		"GET /api/connect/v1/apps/:app_key",
		"GET /api/connect/v1/apps/:app_key/config",
		"GET /api/connect/v1/apps/:app_key/parameters",
		"POST /api/connect/v1/apps/:app_key/chat",
		"GET /api/connect/v1/apps/:app_key/conversations",
		"GET /api/connect/v1/apps/:app_key/conversations/search",
		"POST /api/connect/v1/apps/:app_key/conversations",
		"GET /api/connect/v1/apps/:app_key/conversations/:conversation_id",
		"PATCH /api/connect/v1/apps/:app_key/conversations/:conversation_id",
		"DELETE /api/connect/v1/apps/:app_key/conversations/:conversation_id",
		"GET /api/connect/v1/apps/:app_key/conversations/:conversation_id/messages",
		"GET /api/connect/v1/apps/:app_key/conversations/:conversation_id/events",
		"POST /api/connect/v1/apps/:app_key/conversations/:conversation_id/stop",
		"POST /api/connect/v1/apps/:app_key/conversations/:conversation_id/messages/:message_id/workflow-continuation",
		"POST /api/connect/v1/apps/:app_key/conversations/:conversation_id/messages/:message_id/user-input/:request_id/continue",
		"POST /api/connect/v1/apps/:app_key/conversations/:conversation_id/messages/:message_id/token-budget/:request_id/continue",
		"POST /api/connect/v1/apps/:app_key/messages/:message_id/regenerate",
		"GET /api/connect/v1/apps/:app_key/memory",
		"GET /api/connect/v1/apps/:app_key/memory/export",
		"GET /api/connect/v1/apps/:app_key/memory/settings",
		"PATCH /api/connect/v1/apps/:app_key/memory/settings",
		"POST /api/connect/v1/apps/:app_key/memory/operations/:operation_id/undo",
		"PUT /api/connect/v1/apps/:app_key/memory/:key",
		"DELETE /api/connect/v1/apps/:app_key/memory/:key",
		"DELETE /api/connect/v1/apps/:app_key/memory",
		"POST /api/connect/v1/files",
		"GET /api/connect/v1/files",
		"GET /api/connect/v1/files/:file_id",
		"DELETE /api/connect/v1/files/:file_id",
		"GET /api/connect/v1/files/:file_id/download-url",
	} {
		if !routes[route] {
			t.Errorf("missing phase 1 route %s", route)
		}
	}
	if routes["GET /api/connect/v1/apps/:app_key/memory/:key"] {
		t.Error("unsupported single Memory value route is exposed")
	}
}

func TestRuntimeWorkspaceMetadataRoutesDoNotRequireSubject(t *testing.T) {
	for _, path := range []string{
		"/api/connect/v1/apps/:app_key",
		"/api/connect/v1/apps/:app_key/config",
		"/api/connect/v1/apps/:app_key/parameters",
	} {
		if !runtimeWorkspaceMetadataRequest("GET", path) {
			t.Fatalf("metadata route %q requires a Runtime Subject", path)
		}
	}
	if runtimeWorkspaceMetadataRequest("POST", "/api/connect/v1/apps/:app_key/chat") || runtimeWorkspaceMetadataRequest("GET", "/api/connect/v1/apps/:app_key/conversations") {
		t.Fatal("stateful Runtime Workspace route was classified as metadata")
	}
}

func TestRuntimeAuditContextSurvivesRequestCancellation(t *testing.T) {
	type contextKey string
	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), contextKey("request-id"), "request-1"))
	cancelParent()
	auditCtx, cancelAudit := runtimeAuditContext(parent)
	defer cancelAudit()
	if err := auditCtx.Err(); err != nil {
		t.Fatalf("detached audit context error = %v", err)
	}
	if value := auditCtx.Value(contextKey("request-id")); value != "request-1" {
		t.Fatalf("audit context value = %v, want request-1", value)
	}
	if _, ok := auditCtx.Deadline(); !ok {
		t.Fatal("detached audit context has no deadline")
	}
}

func TestRuntimeWorkspaceRoutesAreFailClosed(t *testing.T) {
	t.Setenv("ZGI_RUNTIME_WORKSPACE_ENABLED", "")
	engine := gin.New()
	RegisterRuntimeWorkspaceRoutes(engine.Group("/api"), RuntimeWorkspaceRouteDeps{})
	if len(engine.Routes()) != 0 {
		t.Fatalf("runtime routes registered while disabled: %#v", engine.Routes())
	}
}

func TestRuntimeFileBindingScopesLookupByWorkspaceAndSubject(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	fileID, runtimeWorkspaceID, subjectID := uuid.New(), uuid.New(), uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "runtime_external_files" WHERE file_id = $1 AND runtime_workspace_id = $2 AND subject_id = $3 ORDER BY "runtime_external_files"."file_id" LIMIT $4`)).
		WithArgs(fileID, runtimeWorkspaceID, subjectID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"file_id", "runtime_workspace_id", "subject_id"}))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/connect/v1/files/"+fileID.String(), nil)
	c.Params = gin.Params{{Key: "file_id", Value: fileID.String()}}
	_, err = runtimeFileBinding(c, db, &runtimeworkspace.Principal{RuntimeWorkspace: &runtimeworkspace.RuntimeWorkspace{WorkspaceID: runtimeWorkspaceID}, SubjectID: subjectID})
	if !errors.Is(err, runtimeworkspace.ErrFileNotFound) {
		t.Fatalf("runtimeFileBinding error = %v, want ErrFileNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeWorkspaceRateLimitKeyIsSharedByCredentials(t *testing.T) {
	runtimeID := uuid.New()
	first := runtimeWorkspaceRateLimitKey(&runtimeworkspace.Principal{
		RuntimeWorkspace: &runtimeworkspace.RuntimeWorkspace{RuntimeID: runtimeID, WorkspaceID: uuid.New()},
		Credential:       &runtimeworkspace.Credential{ID: uuid.New()},
	})
	second := runtimeWorkspaceRateLimitKey(&runtimeworkspace.Principal{
		RuntimeWorkspace: &runtimeworkspace.RuntimeWorkspace{RuntimeID: runtimeID, WorkspaceID: uuid.New()},
		Credential:       &runtimeworkspace.Credential{ID: uuid.New()},
	})
	if first == "" || first != second || first != runtimeID.String() {
		t.Fatalf("rate-limit keys = %q/%q, want shared Runtime Workspace key %s", first, second, runtimeID)
	}
}

func TestRuntimeWorkspaceRequestLeaseCoversTheWholeHandler(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	runtimeID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "runtime_workspaces" SET "in_flight_requests"=in_flight_requests \+ 1 WHERE runtime_id = \$1 AND status = \$2`).
		WithArgs(runtimeID, runtimeworkspace.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "runtime_workspaces" SET "in_flight_requests"=in_flight_requests - 1 WHERE runtime_id = \$1 AND in_flight_requests > 0`).
		WithArgs(runtimeID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/api/connect/v1/apps", nil)
	ctx.Set("runtime_workspace_principal", &runtimeworkspace.Principal{
		RuntimeWorkspace: &runtimeworkspace.RuntimeWorkspace{RuntimeID: runtimeID, WorkspaceID: uuid.New()},
		Credential:       &runtimeworkspace.Credential{ID: uuid.New()},
	})
	runtimeWorkspaceRequestLease(runtimeworkspace.NewService(db))(ctx)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type runtimeWorkspaceRouteFileService struct{}

func (runtimeWorkspaceRouteFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return &interfaces.FileUploadConfigResponse{}
}
func (runtimeWorkspaceRouteFileService) UploadFile(context.Context, string, []byte, string, string, string, filemodel.CreatedByRole, *interfaces.FileSource, *string, bool, bool) (*dto.UploadFile, error) {
	return nil, nil
}
func (runtimeWorkspaceRouteFileService) ReplaceFileContent(context.Context, string, string, []byte, string, string, string) (*dto.UploadFile, error) {
	return nil, nil
}
func (runtimeWorkspaceRouteFileService) GetFilePreview(context.Context, string) (string, error) {
	return "", nil
}
func (runtimeWorkspaceRouteFileService) GetFilePreviewWithOCR(context.Context, string, bool) (string, error) {
	return "", nil
}
func (runtimeWorkspaceRouteFileService) GetFile(context.Context, string) (string, error) {
	return "", nil
}
func (runtimeWorkspaceRouteFileService) ExtractFileWithSetting(context.Context, string, interfaces.FileExtractionSetting) (string, error) {
	return "", nil
}
func (runtimeWorkspaceRouteFileService) GetSupportedFileTypes() []string          { return nil }
func (runtimeWorkspaceRouteFileService) IsFileSizeWithinLimit(string, int64) bool { return true }
func (runtimeWorkspaceRouteFileService) ParseFileContent(context.Context, string) {}
func (runtimeWorkspaceRouteFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return nil, nil
}
func (runtimeWorkspaceRouteFileService) IsRuntimeExternalFile(context.Context, string) (bool, error) {
	return false, nil
}
func (runtimeWorkspaceRouteFileService) DownloadFile(context.Context, string) ([]byte, error) {
	return nil, nil
}
func (runtimeWorkspaceRouteFileService) ListFiles(context.Context, string, string, *dto.FileListRequest, []string) (*dto.FileListResponse, error) {
	return nil, nil
}
func (runtimeWorkspaceRouteFileService) ListArchivedFiles(context.Context, string, string, *dto.FileListRequest, []string) (*dto.FileListResponse, error) {
	return nil, nil
}
func (runtimeWorkspaceRouteFileService) GetStorageUsage(context.Context, string) (int64, error) {
	return 0, nil
}
func (runtimeWorkspaceRouteFileService) DeleteFiles(context.Context, []string) error { return nil }
func (runtimeWorkspaceRouteFileService) UpdateContentText(context.Context, string, string) error {
	return nil
}
func (runtimeWorkspaceRouteFileService) GetFileURL(context.Context, string) (string, error) {
	return "", nil
}

func TestRuntimeCredentialScopeOfRoutes(t *testing.T) {
	for _, tc := range []struct{ method, path, scope string }{
		{http.MethodGet, "/api/connect/v1/apps", runtimeworkspace.CredentialScopeRuntime},
		{http.MethodPost, "/api/connect/v1/apps/:app_key/chat", runtimeworkspace.CredentialScopeRuntime},
		{http.MethodGet, "/api/connect/v1/files/:file_id", runtimeworkspace.CredentialScopeRuntime},
		{http.MethodGet, "/api/connect/v1/plans", ""},
		{http.MethodGet, "/api/connect/v1/end-users/:user", runtimeworkspace.CredentialScopeEndUsersRead},
		{http.MethodGet, "/api/connect/v1/end-users/:user/connections", runtimeworkspace.CredentialScopeEndUsersRead},
		{http.MethodPatch, "/api/connect/v1/end-users/:user", runtimeworkspace.CredentialScopeEndUsersManage},
		{http.MethodPost, "/api/connect/v1/end-users/:user/export", runtimeworkspace.CredentialScopeEndUsersManage},
		{http.MethodGet, "/api/connect/v1/end-users/:user/exports/:export_id/download", runtimeworkspace.CredentialScopeEndUsersManage},
		{http.MethodGet, "", runtimeworkspace.CredentialScopeRuntime},
	} {
		if got := runtimeCredentialScope(tc.method, tc.path); got != tc.scope {
			t.Errorf("%s %s needs %q, want %q", tc.method, tc.path, got, tc.scope)
		}
	}
}
