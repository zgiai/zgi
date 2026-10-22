package digitalhuman

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
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
	access := &PublishedAccess{Definition: definition, Version: &Version{ID: uuid.New(), FileUploadPolicy: &chatfile.Policy{AllowedFileTypes: []string{}}}}

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

func TestLinuxEnabledWebAppRequiresLoginWithoutAnonymousFallback(t *testing.T) {
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, appcatalog.DefaultDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	access := &PublishedAccess{Definition: &Definition{ID: uuid.New(), WebAppID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()}, Version: &Version{ExecutionRuntime: &ExecutionRuntimeConfig{Enabled: true}}}
	if !webAppLoginRequired(access) {
		t.Fatal("published Linux config did not require login")
	}
	for _, authenticated := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/chat", nil)
		c.Set(webAppAccessContext, access)
		c.Set(webAppIdentityContext, webAppRequestIdentity{ID: uuid.New(), Authenticated: authenticated})
		handler := &ExternalHandler{base: &Handler{projector: projector}}
		_, _, _, ok := handler.webAppScope(c)
		if ok != authenticated || (!authenticated && recorder.Code != http.StatusUnauthorized) {
			t.Fatalf("authenticated=%t accepted=%t status=%d", authenticated, ok, recorder.Code)
		}
	}
	access.Version.ExecutionRuntime = nil
	// Uploads ask for sign-in only when a file is attached, not to chat.
	if webAppLoginRequired(access) || !fileUploadEnabled(access.Version) {
		t.Fatal("file uploads required login to chat")
	}
	access.Version.FileUploadPolicy = &chatfile.Policy{AllowedFileTypes: []string{}}
	if webAppLoginRequired(access) || fileUploadEnabled(access.Version) {
		t.Fatal("both disabled still required login or allowed uploads")
	}
}

func TestChatWebAppAsksAnonymousVisitorToSignInBeforeSendingFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	access := &PublishedAccess{Definition: &Definition{ID: uuid.New(), WebAppID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New()}, Version: &Version{ID: uuid.New()}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"request_id":"` + uuid.NewString() + `","query":"hi","file_ids":["` + uuid.NewString() + `"]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(webAppAccessContext, access)
	c.Set(webAppIdentityContext, webAppRequestIdentity{ID: uuid.New()})
	handler := &ExternalHandler{base: &Handler{projector: projector}, access: &AccessService{}}

	handler.ChatWebApp(c)

	assertDigitalHumanError(t, recorder, http.StatusForbidden, AppCodeWebAppFileRequiresLogin.String())
	if !strings.Contains(recorder.Body.String(), "Sign in to send files") {
		t.Fatalf("body = %s, want the sign-in message", recorder.Body.String())
	}
}
