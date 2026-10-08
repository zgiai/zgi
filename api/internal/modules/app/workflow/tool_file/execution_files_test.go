package tool_file

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func executionFileFixture(t *testing.T) (*ToolFileManager, *toolFileHandlerPresignStorage, CreateFileByRawParams, ExecutionFileAccess) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatal(err)
	}
	backend := &toolFileHandlerPresignStorage{toolFileHandlerTestStorage: toolFileHandlerTestStorage{files: make(map[string][]byte)}, presignedURL: "https://storage.invalid/private"}
	manager := NewToolFileManager(db, backend)
	org, user, workspace, app, conversation, message := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	i := &execution.Identity{Version: 1, WorkspaceID: workspace, ApplicationType: "agent", ApplicationID: app,
		Entry: "webapp", SubjectKind: "account", ConversationID: conversation, PublicationRef: "published-v1"}
	params := CreateFileByRawParams{UserID: user, TenantID: org, ConversationID: &conversation, FileData: []byte("private-result"), MimeType: "text/plain",
		ExecutionIdentity: i, ExecutionMessageID: &message}
	access := ExecutionFileAccess{OrganizationID: org, SubjectID: user, WorkspaceID: workspace, ApplicationType: "agent",
		ApplicationID: app, Entry: "webapp", SubjectKind: "account", ConversationID: conversation}
	return manager, backend, params, access
}

func TestExecutionFilesRequireCompleteOwnerBeforeSaving(t *testing.T) {
	manager, backend, params, _ := executionFileFixture(t)
	cases := map[string]func(*CreateFileByRawParams){
		"message":      func(p *CreateFileByRawParams) { p.ExecutionMessageID = nil },
		"identity":     func(p *CreateFileByRawParams) { p.ExecutionIdentity = nil },
		"conversation": func(p *CreateFileByRawParams) { p.ConversationID = nil },
		"mismatch":     func(p *CreateFileByRawParams) { id := uuid.NewString(); p.ConversationID = &id },
		"subject":      func(p *CreateFileByRawParams) { p.UserID = "" },
		"organization": func(p *CreateFileByRawParams) { p.TenantID = "not-a-uuid" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			invalid := params
			mutate(&invalid)
			if _, err := manager.CreateFileByRaw(t.Context(), invalid); !errors.Is(err, ErrExecutionFileAccess) {
				t.Fatalf("error = %v", err)
			}
			if len(backend.files) != 0 {
				t.Fatal("invalid owner wrote storage")
			}
		})
	}
	file, err := manager.CreateFileByRaw(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	params.ExecutionIdentity.PublicationRef = "changed-after-save"
	stored, err := manager.GetToolFileByID(t.Context(), file.ID)
	if err != nil || stored.ExecutionIdentity.PublicationRef != "published-v1" || *stored.ExecutionMessageID != *params.ExecutionMessageID {
		t.Fatalf("owner was not frozen: %+v, %v", stored, err)
	}
}

func TestExecutionFilesRejectCrossScopeBeforeStorageAndLegacyReaders(t *testing.T) {
	manager, backend, params, access := executionFileFixture(t)
	file, err := manager.CreateFileByRaw(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*ExecutionFileAccess){
		"org":          func(a *ExecutionFileAccess) { a.OrganizationID = uuid.NewString() },
		"user":         func(a *ExecutionFileAccess) { a.SubjectID = uuid.NewString() },
		"workspace":    func(a *ExecutionFileAccess) { a.WorkspaceID = uuid.NewString() },
		"app":          func(a *ExecutionFileAccess) { a.ApplicationID = uuid.NewString() },
		"type":         func(a *ExecutionFileAccess) { a.ApplicationType = "digital_human" },
		"entry":        func(a *ExecutionFileAccess) { a.Entry = "console" },
		"subject-kind": func(a *ExecutionFileAccess) { a.SubjectKind = "api_end_user" },
		"conversation": func(a *ExecutionFileAccess) { a.ConversationID = uuid.NewString() },
		"anonymous":    func(a *ExecutionFileAccess) { a.SubjectID = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			other := access
			mutate(&other)
			if _, _, err := manager.GetExecutionFileBinary(t.Context(), other, file.ID); !errors.Is(err, ErrExecutionFileAccess) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if _, _, err := manager.GetFileBinary(t.Context(), file.ID); !errors.Is(err, ErrExecutionFileAccess) {
		t.Fatalf("legacy binary = %v", err)
	}
	if _, _, err := manager.GetFileStream(t.Context(), file.ID); !errors.Is(err, ErrExecutionFileAccess) {
		t.Fatalf("legacy stream = %v", err)
	}
	if _, _, err := manager.GetPresignedFileURL(t.Context(), file.ID, time.Hour); !errors.Is(err, ErrExecutionFileAccess) {
		t.Fatalf("legacy direct = %v", err)
	}
	if backend.loadCalls != 0 || backend.presignCalls != 0 {
		t.Fatal("unauthorized access touched storage")
	}
	// Read authorization is independent of compute admission and the current
	// publication. The original frozen publication remains in the stored row.
	data, stored, err := manager.GetExecutionFileBinary(t.Context(), access, file.ID)
	if err != nil || string(data) != "private-result" || stored.ExecutionIdentity.PublicationRef != "published-v1" {
		t.Fatalf("authorized read = %q, %v", data, err)
	}
	now := time.Now()
	if err := manager.db.Model(&ToolFile{}).Where("id = ?", file.ID).Update("deleted_at", &now).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.GetExecutionFileBinary(t.Context(), access, file.ID); !errors.Is(err, ErrExecutionFileAccess) {
		t.Fatal("deleted artifact was readable")
	}
}

func TestExecutionFilesRejectValidLegacySignaturesIncludingRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager, backend, params, _ := executionFileFixture(t)
	file, err := manager.CreateFileByRaw(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	signer := NewFileSignature(&config.Config{App: config.AppConfig{SecretKey: "test-only", FilesURL: "http://example.test"}})
	handler := &HTTPHandler{manager: manager, signature: signer}
	router := gin.New()
	router.GET("/console/api/files/tools/:tool_file_id", handler.GetToolFile)
	for _, mode := range []ToolFileURLMode{ToolFileURLModePermanent, ToolFileURLModeSigned} {
		signed, err := signer.SignToolFileWithMode(file.ID, ".txt", mode)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(signed)
		if err != nil {
			t.Fatal(err)
		}
		for _, suffix := range []string{"", "&download=1", "&delivery=direct"} {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, parsed.RequestURI()+suffix, nil))
			if strings.Contains(recorder.Body.String(), "private-result") || recorder.Header().Get("Location") != "" || recorder.Code == http.StatusOK {
				t.Fatalf("signature bypass: %d %s", recorder.Code, recorder.Body.String())
			}
		}
	}
	if backend.loadCalls != 0 || backend.presignCalls != 0 {
		t.Fatal("signed bypass touched storage")
	}
}

func TestExecutionFileHTTPUsesAuthenticatedProxyAndByteRanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager, backend, params, access := executionFileFixture(t)
	file, err := manager.CreateFileByRaw(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHTTPHandler(manager)
	router := gin.New()
	// The test adapter models an entry which has already checked live access.
	allowed := true
	router.GET("/conversations/:conversation_id/files/:file_id", func(c *gin.Context) {
		if !allowed {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		handler.ServeExecutionFile(c, access)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/conversations/"+access.ConversationID+"/files/"+file.ID+"?download=1&delivery=direct", nil)
	request.Header.Set("Range", "bytes=0-6")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "private" || recorder.Header().Get("Cache-Control") != "private, no-store" || recorder.Header().Get("Content-Security-Policy") == "" || backend.presignCalls != 0 {
		t.Fatalf("response = %d %v %s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	allowed = false
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || backend.loadCalls != 1 {
		t.Fatal("revocation did not prevent storage access")
	}
}
