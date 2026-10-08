package agents

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type executionFileConversationStub struct {
	runtimeservice.Service
	conversation *runtimemodel.Conversation
	denied       bool
}

func (s *executionFileConversationStub) GetConversationByCaller(context.Context, runtimeservice.Scope, runtimeservice.Caller, uuid.UUID) (*runtimemodel.Conversation, error) {
	if s.denied {
		return nil, runtimeservice.ErrNotFound
	}
	return s.conversation, nil
}

type executionFileStorage struct {
	storage.Storage
	data  map[string][]byte
	reads int
}

func (s *executionFileStorage) Save(key string, data []byte) error {
	s.data[key] = append([]byte(nil), data...)
	return nil
}
func (s *executionFileStorage) Load(key string) ([]byte, error) {
	s.reads++
	data, ok := s.data[key]
	if !ok {
		return nil, errors.New("missing file")
	}
	return data, nil
}

func TestAgentExecutionFileHandlerChecksLiveConversationAndEndUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, entry := range []string{"console", "webapp", "external-api"} {
		t.Run(entry, func(t *testing.T) {
			org, workspace, account, user, agent, conversationID, messageID, webApp := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
			scope := runtimeservice.Scope{OrganizationID: org, AccountID: account, WorkspaceID: &workspace}
			caller := runtimeservice.Caller{Type: "agent", ID: &agent, Source: entry}
			conversation := &runtimemodel.Conversation{ID: conversationID, OrganizationID: org, AccountID: account, WorkspaceID: &workspace, CallerType: "agent", CallerID: &agent, Source: entry}
			owner, kind := account, "account"
			if entry == "webapp" {
				caller.SourceWebAppID = &webApp
				conversation.SourceWebAppID = &webApp
			}
			if entry == "external-api" {
				scope.ExternalUserID = &user
				conversation.ExternalUserID = &user
				owner, kind = user, "api_end_user"
			}
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&tool_file.ToolFile{}); err != nil {
				t.Fatal(err)
			}
			backend := &executionFileStorage{data: map[string][]byte{}}
			manager := tool_file.NewToolFileManager(db, backend)
			previous := tool_file.GlobalToolFileManager
			tool_file.GlobalToolFileManager = manager
			t.Cleanup(func() { tool_file.GlobalToolFileManager = previous })
			cid, mid := conversationID.String(), messageID.String()
			file, err := manager.CreateFileByRaw(t.Context(), tool_file.CreateFileByRawParams{UserID: owner.String(), TenantID: org.String(), ConversationID: &cid, FileData: []byte("result"), MimeType: "text/plain", ExecutionMessageID: &mid,
				ExecutionIdentity: &execution.Identity{Version: 1, WorkspaceID: workspace.String(), ApplicationType: "agent", ApplicationID: agent.String(), Entry: entry, SubjectKind: kind, ConversationID: cid, PublicationRef: "v1"}})
			if err != nil {
				t.Fatal(err)
			}
			service := &executionFileConversationStub{conversation: conversation}
			handler := &AgentsHandler{chatRuntimeService: service}
			router := gin.New()
			router.GET("/conversations/:conversation_id/files/:file_id", func(c *gin.Context) { handler.serveRuntimeExecutionFile(c, scope, caller) })
			request := httptest.NewRequest(http.MethodGet, "/conversations/"+cid+"/files/"+file.ID, nil)
			t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "false")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK || recorder.Body.String() != "result" {
				t.Fatalf("own file = %d %s", recorder.Code, recorder.Body.String())
			}
			if entry == "external-api" && scope.AccountID != account {
				t.Fatal("billing account changed")
			}
			service.denied = true
			recorder = httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK || backend.reads != 1 {
				t.Fatal("live conversation denial was bypassed")
			}
			service.denied = false
			if entry == "external-api" {
				other := uuid.New()
				scope.ExternalUserID = &other
			} else {
				scope.AccountID = uuid.New()
			}
			recorder = httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK || backend.reads != 1 {
				t.Fatal("end-user isolation was bypassed")
			}
		})
	}
}

func TestAgentExecutionFileAccessRejectsOtherApplicationsEntriesAndRuntimeWorkspace(t *testing.T) {
	org, account, app, workspace, cid := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	scope := runtimeservice.Scope{OrganizationID: org, AccountID: account, WorkspaceID: &workspace}
	caller := runtimeservice.Caller{Type: "agent", ID: &app, Source: "console"}
	conversation := &runtimemodel.Conversation{ID: cid, OrganizationID: org, AccountID: account, CallerType: "agent", CallerID: &app, Source: "console"}
	if _, ok := agentExecutionFileAccess(scope, caller, conversation); !ok {
		t.Fatal("valid owner denied")
	}
	for _, change := range []func(*runtimemodel.Conversation){func(c *runtimemodel.Conversation) { c.CallerType = "digital_human" }, func(c *runtimemodel.Conversation) { other := uuid.New(); c.CallerID = &other }, func(c *runtimemodel.Conversation) { c.Source = "webapp" }} {
		other := *conversation
		change(&other)
		if _, ok := agentExecutionFileAccess(scope, caller, &other); ok {
			t.Fatal("foreign conversation accepted")
		}
	}
	scope.RuntimeWorkspaceID = &workspace
	if _, ok := agentExecutionFileAccess(scope, caller, conversation); ok {
		t.Fatal("Runtime Workspace gained protected file access")
	}
}
