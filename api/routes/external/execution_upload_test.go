package external

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/dto"
	agentapp "github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/middleware"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type executionAPIUploadFiles struct {
	*externalFileAccessService
	deleted int
}

func (s *executionAPIUploadFiles) DeleteFiles(context.Context, []string) error {
	s.deleted++
	return nil
}

func TestAPIExecutionUploadUsesPublishedResourcesAndStableEndUser(t *testing.T) {
	for _, scenario := range []string{"enabled", "disabled", "store unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&execution.UploadBinding{}); err != nil {
				t.Fatal(err)
			}
			key := &middleware.APIKeyInfo{ID: uuid.New(), AgentID: uuid.New(), TenantID: uuid.New()}
			published := &dto.AgentWebAppRuntimeConfigResponse{AgentID: key.AgentID.String(), OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
				Config: dto.AgentConfigResponse{ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: scenario != "disabled"}}}
			files := &executionAPIUploadFiles{externalFileAccessService: &externalFileAccessService{uploaded: &dto.UploadFile{
				ID: uuid.NewString(), Name: "input.csv", Size: 8, MimeType: "text/csv", CreatedAt: time.Now()}}}
			h := &ExternalWorkflowHandler{db: db, fileService: files, agentRuntime: &externalAgentRuntimeStub{config: published},
				agentRepository: externalAgentRepoStub{agent: &agentapp.Agent{ID: key.AgentID, AgentsType: "AGENT"}}}
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
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			c.Request.Header.Set(agentapp.ExternalUserIDHeader, "customer-test")
			c.Set("api_key_info", key)
			h.UploadFile(c)
			if scenario == "store unavailable" {
				if files.deleted != 1 || bytes.Contains(recorder.Body.Bytes(), []byte(files.uploaded.ID)) {
					t.Fatalf("unbound upload returned: deleted=%d body=%s", files.deleted, recorder.Body)
				}
				return
			}
			if recorder.Code != http.StatusCreated {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
			owner := execution.UploadOwner{OrganizationID: published.OrganizationID, WorkspaceID: published.WorkspaceID, ApplicationType: "agent",
				ApplicationID: key.AgentID.String(), Entry: "external-api", SubjectKind: "api_end_user", SubjectID: files.uploadUserID}
			if owner.SubjectID == key.ID.String() {
				t.Fatal("credential became final user")
			}
			key.ID = uuid.New() // Credential rotation must not alter the stable user.
			if owner.SubjectID != agentapp.DeriveExternalUserID(key.TenantID, key.AgentID, "customer-test").String() {
				t.Fatal("credential rotation changed end user")
			}
			err = execution.AuthorizeUpload(t.Context(), db, files.uploaded.ID, owner)
			if (scenario == "enabled") != (err == nil) {
				t.Fatalf("binding scenario=%s err=%v", scenario, err)
			}
		})
	}
}
