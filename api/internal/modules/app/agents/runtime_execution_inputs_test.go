package agents

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type executionUploadTestFiles struct {
	interfaces.FileService
	file    *dto.UploadFile
	deleted int
}

func TestPublishedInputReaderMatchesVerifiedConversationAndNotBillingAccount(t *testing.T) {
	for _, entry := range []string{"webapp", "external-api"} {
		t.Run(entry, func(t *testing.T) {
			org, workspace, app, user, billing, conversation, webapp := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
			prepared := &runtimeservice.PreparedChat{Scope: runtimeservice.Scope{OrganizationID: org, AccountID: user, WorkspaceID: &workspace},
				Caller:       runtimeservice.Caller{Type: "agent", ID: &app, Source: entry, SourceWebAppID: &webapp},
				Conversation: &runtimemodel.Conversation{ID: conversation, OrganizationID: org, AccountID: user, CallerType: "agent", CallerID: &app, Source: entry, SourceWebAppID: &webapp}}
			kind := "account"
			if entry == "external-api" {
				prepared.Scope.AccountID, prepared.Conversation.AccountID = billing, billing
				prepared.Scope.ExternalUserID, prepared.Conversation.ExternalUserID = &user, &user
				kind = "api_end_user"
			}
			scope := execution.Scope{OrganizationID: org.String(), SubjectID: user.String(), Identity: &execution.Identity{
				Version: 1, WorkspaceID: workspace.String(), ApplicationType: "agent", ApplicationID: app.String(),
				Entry: entry, SubjectKind: kind, ConversationID: conversation.String(), PublicationRef: "published-v1"}}
			h := &AgentsHandler{fileService: &executionUploadTestFiles{}}
			if _, err := h.resolveAgentExecutionInputs(prepared, scope); err != nil {
				t.Fatal(err)
			}
			wrong := scope
			wrong.SubjectID = billing.String()
			if _, err := h.resolveAgentExecutionInputs(prepared, wrong); err == nil {
				t.Fatal("billing account substituted for input owner")
			}
			prepared.Conversation.Source = "console"
			if _, err := h.resolveAgentExecutionInputs(prepared, scope); err == nil {
				t.Fatal("cross-entry conversation accepted")
			}
		})
	}
}

func (s *executionUploadTestFiles) UploadFile(context.Context, string, []byte, string, string, string, filemodel.CreatedByRole, *interfaces.FileSource, *string, bool, bool) (*dto.UploadFile, error) {
	return s.file, nil
}
func (s *executionUploadTestFiles) DeleteFiles(context.Context, []string) error {
	s.deleted++
	return nil
}

func TestWebAppExecutionUploadBindsBeforeRespondingAndCleansFailures(t *testing.T) {
	for _, scenario := range []string{"enabled", "disabled", "store unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&execution.UploadBinding{}); err != nil {
				t.Fatal(err)
			}
			org, workspace, agent, user := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
			published := &dto.AgentWebAppRuntimeConfigResponse{OrganizationID: org, WorkspaceID: workspace, AgentID: agent,
				Config: dto.AgentConfigResponse{FileUpload: true, ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: scenario != "disabled"}}}
			service := &stubWebAppStatusHandlerService{publishedConfigResp: published, runtimeCapabilityResp: &dto.AgentWebAppRuntimeCapabilityResponse{Allowed: true}}
			files := &executionUploadTestFiles{file: &dto.UploadFile{ID: uuid.NewString(), Name: "input.csv", IsTemporary: true, CreatedBy: user, CreatedByRole: dto.CreatedByRoleEndUser}}
			h := NewAgentsHandler(service, nil, nil, nil, db)
			h.fileService = files
			if scenario == "store unavailable" {
				h.db = nil
			}
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("file", "input.csv")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte("a,b\n1,2\n")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/files/upload", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			c.Set("account_id", user)
			c.Set("is_authenticated", true)
			c.Params = gin.Params{{Key: "web_app_id", Value: uuid.NewString()}}
			h.UploadWebAppFile(c)
			if scenario == "store unavailable" {
				if files.deleted != 1 || bytes.Contains(recorder.Body.Bytes(), []byte(files.file.ID)) {
					t.Fatalf("unbound upload returned: deleted=%d body=%s", files.deleted, recorder.Body)
				}
				return
			}
			if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(files.file.ID)) {
				t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body)
			}
			owner := execution.UploadOwner{OrganizationID: org, WorkspaceID: workspace, ApplicationType: "agent", ApplicationID: agent, Entry: "webapp", SubjectKind: "account", SubjectID: user}
			err = authorizeAgentExecutionUpload(t.Context(), db, owner, files.file)
			if (scenario == "enabled") != (err == nil) {
				t.Fatalf("binding enabled=%s err=%v", scenario, err)
			}
			if scenario == "enabled" {
				other := owner
				other.ApplicationID = uuid.NewString()
				if err := authorizeAgentExecutionUpload(t.Context(), db, other, files.file); err == nil {
					t.Fatal("same account reused upload in another application")
				}
				files.file.CreatedBy = uuid.NewString()
				if err := authorizeAgentExecutionUpload(t.Context(), db, owner, files.file); err == nil {
					t.Fatal("credential owner substituted for final user")
				}
			}
		})
	}
}
