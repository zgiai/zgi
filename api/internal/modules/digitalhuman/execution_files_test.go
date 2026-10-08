package digitalhuman

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDigitalHumanExecutionFileScopeRequiresVerifiedInteractiveSubject(t *testing.T) {
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		t.Run(surface, func(t *testing.T) {
			scope := testScope()
			scope.UserSubjectKind, scope.UserSubjectID, scope.Surface = "account", &scope.ActorID, surface
			if surface == SurfaceAPI {
				scope.UserSubjectKind = "api_end_user"
			}
			app, cid, web := uuid.New(), uuid.New(), uuid.New()
			conversation := runtimemodel.Conversation{ID: cid, OrganizationID: scope.OrganizationID, AccountID: scope.ActorID,
				WorkspaceID: &scope.WorkspaceID, CallerType: runtimemodel.ConversationCallerDigitalHuman, CallerID: &app,
				Source: digitalHumanConversationSourceForSurface(surface)}
			if surface == SurfaceWebApp {
				scope.WebAppID, conversation.SourceWebAppID = &web, &web
			}
			access, ok := digitalHumanExecutionFileAccess(scope, app, &conversation)
			if !ok || access.ApplicationType != "digital_human" || access.SubjectID != scope.ActorID.String() || access.Entry != conversation.Source {
				t.Fatalf("verified access = %+v, %v", access, ok)
			}
			// Credential rotation must not change the final user's file ownership.
			scope.PrincipalID = uuid.New()
			if rotated, ok := digitalHumanExecutionFileAccess(scope, app, &conversation); !ok || rotated != access {
				t.Fatal("credential rotation changed ownership")
			}
			cases := map[string]func(*Scope, *runtimemodel.Conversation){
				"missing subject":    func(s *Scope, _ *runtimemodel.Conversation) { s.UserSubjectID = nil },
				"anonymous":          func(s *Scope, _ *runtimemodel.Conversation) { s.UserSubjectKind = "anonymous" },
				"principal fallback": func(s *Scope, _ *runtimemodel.Conversation) { s.ActorID = s.PrincipalID },
				"foreign subject": func(s *Scope, _ *runtimemodel.Conversation) {
					other := uuid.New()
					s.ActorID, s.UserSubjectID = other, &other
				},
				"foreign organization": func(s *Scope, _ *runtimemodel.Conversation) { s.OrganizationID = uuid.New() },
				"foreign workspace":    func(s *Scope, _ *runtimemodel.Conversation) { s.WorkspaceID = uuid.New() },
				"agent impersonation":  func(_ *Scope, c *runtimemodel.Conversation) { c.CallerType = "agent" },
				"foreign application":  func(_ *Scope, c *runtimemodel.Conversation) { other := uuid.New(); c.CallerID = &other },
				"foreign entry":        func(_ *Scope, c *runtimemodel.Conversation) { c.Source = "channel" },
				"deleted":              func(_ *Scope, c *runtimemodel.Conversation) { now := time.Now(); c.DeletedAt = &now },
				"task":                 func(s *Scope, _ *runtimemodel.Conversation) { s.Surface = SurfaceTask },
				"channel":              func(s *Scope, _ *runtimemodel.Conversation) { s.Surface = SurfaceChannel },
			}
			if surface == SurfaceWebApp {
				cases["foreign webapp"] = func(s *Scope, _ *runtimemodel.Conversation) { other := uuid.New(); s.WebAppID = &other }
			}
			for name, mutate := range cases {
				t.Run(name, func(t *testing.T) {
					s, c := scope, conversation
					mutate(&s, &c)
					if _, ok := digitalHumanExecutionFileAccess(s, app, &c); ok {
						t.Fatal("unverified owner accepted")
					}
				})
			}
		})
	}
}

type executionFileRuntimeStub struct {
	RuntimeHandlerService
	conversation *runtimemodel.Conversation
	denied       bool
}

func (s *executionFileRuntimeStub) GetConversation(_ context.Context, scope Scope, _ uuid.UUID, _ uuid.UUID) (*runtimemodel.Conversation, error) {
	if scope.UserSubjectID == nil || scope.UserSubjectKind != "account" || scope.Surface != SurfaceConsole {
		return nil, ErrInvalidRequest
	}
	if s.denied {
		return nil, ErrConversationNotFound
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

func TestDigitalHumanExecutionFileConsoleRouteRechecksAccessWhenExecutionDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope, app, cid, mid := testScope(), uuid.New(), uuid.New(), uuid.New()
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
	cidText, midText := cid.String(), mid.String()
	file, err := manager.CreateFileByRaw(t.Context(), tool_file.CreateFileByRawParams{UserID: scope.ActorID.String(), TenantID: scope.OrganizationID.String(),
		ConversationID: &cidText, FileData: []byte("digital-human-result"), MimeType: "text/plain", ExecutionMessageID: &midText,
		ExecutionIdentity: &execution.Identity{Version: 1, WorkspaceID: scope.WorkspaceID.String(), ApplicationType: "digital_human", ApplicationID: app.String(),
			Entry: "console", SubjectKind: "account", ConversationID: cidText, PublicationRef: "published-v1"}})
	if err != nil {
		t.Fatal(err)
	}
	service := &executionFileRuntimeStub{conversation: &runtimemodel.Conversation{ID: cid, OrganizationID: scope.OrganizationID, WorkspaceID: &scope.WorkspaceID,
		AccountID: scope.ActorID, CallerType: "digital_human", CallerID: &app, Source: "console"}}
	router := digitalHumanRuntimeHandlerRouter(t, service, scope)
	t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "false")
	request := httptest.NewRequest(http.MethodGet, "/digital-humans/"+app.String()+"/runtime/conversations/"+cid.String()+"/files/"+file.ID, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "digital-human-result" {
		t.Fatalf("own result = %d %s", recorder.Code, recorder.Body.String())
	}
	service.denied = true
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK || backend.reads != 1 {
		t.Fatal("revoked access read storage")
	}
}
