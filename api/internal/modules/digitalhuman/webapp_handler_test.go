package digitalhuman

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
)

func TestRegisterWebAppRoutesIncludesSharedChatRuntimeCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&ExternalHandler{}).RegisterWebAppRoutes(router.Group("/api"))
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, required := range []string{
		http.MethodPost + " /api/v1/digital-human-webapps/:web_app_id/conversations/:conversation_id/messages/:message_id/tool-governance/:correlation_id/continue",
		http.MethodGet + " /api/v1/digital-human-webapps/:web_app_id/config",
		http.MethodPost + " /api/v1/digital-human-webapps/:web_app_id/chat",
		http.MethodGet + " /api/v1/digital-human-webapps/:web_app_id/conversations/:conversation_id/events",
		http.MethodGet + " /api/v1/digital-human-webapps/:web_app_id/conversations/:conversation_id/files/:file_id",
		http.MethodPost + " /api/v1/digital-human-webapps/:web_app_id/conversations/:conversation_id/stop",
		http.MethodPost + " /api/v1/digital-human-webapps/:web_app_id/conversations/:conversation_id/messages/:message_id/runtime-inputs",
		http.MethodPost + " /api/v1/digital-human-webapps/:web_app_id/conversations/:conversation_id/messages/:message_id/user-input/:request_id/continue",
		http.MethodGet + " /api/v1/digital-human-webapps/:web_app_id/files/upload",
		http.MethodPost + " /api/v1/digital-human-webapps/:web_app_id/files/upload",
		http.MethodGet + " /api/v1/digital-human-webapps/:web_app_id/tasks",
		http.MethodPost + " /api/v1/digital-human-webapps/:web_app_id/tasks",
		http.MethodGet + " /api/v1/digital-human-webapps/:web_app_id/tasks/:task_id",
		http.MethodPatch + " /api/v1/digital-human-webapps/:web_app_id/tasks/:task_id",
		http.MethodPost + " /api/v1/digital-human-webapps/:web_app_id/tasks/:task_id/actions/:action",
		http.MethodGet + " /api/v1/digital-human-webapps/:web_app_id/tasks/:task_id/runs",
		http.MethodGet + " /api/v1/digital-human-webapps/:web_app_id/tasks/:task_id/activities",
	} {
		if !routes[required] {
			t.Fatalf("webapp route %q was not registered", required)
		}
	}
}

func TestRegisterAPIRoutesIncludesPrivateTaskContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&ExternalHandler{}).RegisterAPIRoutes(router.Group("/api"))
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, required := range []string{
		http.MethodPost + " /api/v1/digital-humans/conversations/:conversation_id/messages/:message_id/tool-governance/:correlation_id/continue",
		http.MethodGet + " /api/v1/digital-humans/tasks",
		http.MethodGet + " /api/v1/digital-humans/conversations/:conversation_id/files/:file_id",
		http.MethodPost + " /api/v1/digital-humans/tasks",
		http.MethodGet + " /api/v1/digital-humans/tasks/:task_id",
		http.MethodPatch + " /api/v1/digital-humans/tasks/:task_id",
		http.MethodPost + " /api/v1/digital-humans/tasks/:task_id/actions/:action",
		http.MethodGet + " /api/v1/digital-humans/tasks/:task_id/runs",
		http.MethodGet + " /api/v1/digital-humans/tasks/:task_id/activities",
		http.MethodPost + " /api/v1/digital-humans/conversations/:conversation_id/messages/:message_id/user-input/:request_id/continue",
	} {
		if !routes[required] {
			t.Fatalf("API task route %q was not registered", required)
		}
	}
}

func TestWebAppAccountStatusMatchesAuthenticatedAccountPolicy(t *testing.T) {
	for _, status := range []authmodel.AccountStatus{
		authmodel.AccountStatusActive,
		authmodel.AccountStatusPending,
		authmodel.AccountStatusUninitialized,
	} {
		if !webAppAccountStatusAccepted(status) {
			t.Fatalf("status %q should be accepted", status)
		}
	}
	for _, status := range []authmodel.AccountStatus{
		authmodel.AccountStatusBanned,
		authmodel.AccountStatusFrozen,
		authmodel.AccountStatusClosed,
	} {
		if webAppAccountStatusAccepted(status) {
			t.Fatalf("status %q should be rejected", status)
		}
	}
}

func TestWebAppScopeSeparatesAnonymousPrincipalFromAccountMemorySubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	definition := &Definition{
		ID: uuid.New(), WebAppID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
	}
	access := &PublishedAccess{Definition: definition, Version: &Version{ID: uuid.New()}}

	t.Run("anonymous", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Set(webAppAccessContext, access)
		identity := webAppRequestIdentity{ID: uuid.New()}
		context.Set(webAppIdentityContext, identity)
		handler := &ExternalHandler{}
		scope, _, gotIdentity, ok := handler.webAppScope(context)
		if !ok || gotIdentity != identity {
			t.Fatal("anonymous webapp scope was not resolved")
		}
		if scope.UserSubjectID != nil || scope.PrincipalType != string(invocation.PrincipalExternalUser) || scope.ActorID != identity.ID {
			t.Fatalf("anonymous scope = %#v", scope)
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Set(webAppAccessContext, access)
		identity := webAppRequestIdentity{ID: uuid.New(), Authenticated: true}
		context.Set(webAppIdentityContext, identity)
		handler := &ExternalHandler{}
		scope, _, _, ok := handler.webAppScope(context)
		if !ok || scope.UserSubjectID == nil || *scope.UserSubjectID != identity.ID ||
			scope.UserSubjectKind != string(usersubject.Account) || scope.PrincipalType != string(invocation.PrincipalAccount) {
			t.Fatalf("authenticated scope = %#v", scope)
		}
	})
}
