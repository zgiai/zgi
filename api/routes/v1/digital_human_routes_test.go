package v1

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

type digitalHumanRoutesService struct{ digitalhuman.HandlerService }
type digitalHumanRoutesRuntime struct {
	digitalhuman.RuntimeHandlerService
}
type digitalHumanRoutesChannels struct {
	digitalhuman.ChannelHandlerService
}
type digitalHumanRoutesTasks struct {
	digitalhuman.TaskHandlerService
}

func TestDigitalHumanPermissionForRequest(t *testing.T) {
	tests := []struct {
		method, path string
		want         workspacemodel.WorkspacePermissionCode
	}{
		{http.MethodGet, "/console/api/digital-humans", workspacemodel.WorkspacePermissionDigitalHumanView},
		{http.MethodPost, "/console/api/digital-humans", workspacemodel.WorkspacePermissionDigitalHumanCreate},
		{http.MethodPatch, "/console/api/digital-humans/:id/draft", workspacemodel.WorkspacePermissionDigitalHumanUpdate},
		{http.MethodDelete, "/console/api/digital-humans/:id", workspacemodel.WorkspacePermissionDigitalHumanDelete},
		{http.MethodPost, "/console/api/digital-humans/:id/publish", workspacemodel.WorkspacePermissionDigitalHumanPublish},
		{http.MethodGet, "/console/api/digital-humans/:id/versions", workspacemodel.WorkspacePermissionDigitalHumanView},
		{http.MethodGet, "/console/api/digital-humans/:id/versions/:version_id/restore-preview", workspacemodel.WorkspacePermissionDigitalHumanView},
		{http.MethodPost, "/console/api/digital-humans/:id/versions/restore", workspacemodel.WorkspacePermissionDigitalHumanPublish},
		{http.MethodPatch, "/console/api/digital-humans/:id/access", workspacemodel.WorkspacePermissionDigitalHumanPublish},
		{http.MethodPost, "/console/api/digital-humans/:id/api-keys", workspacemodel.WorkspacePermissionDigitalHumanPublish},
		{http.MethodPost, "/console/api/digital-humans/:id/invocations", workspacemodel.WorkspacePermissionDigitalHumanRun},
		// The pre-creation skill candidate route serves the create flow, so it
		// needs the create permission; only GET is allowed.
		{http.MethodGet, "/console/api/digital-human-skill-candidates", workspacemodel.WorkspacePermissionDigitalHumanCreate},
		{http.MethodPost, "/console/api/digital-human-skill-candidates", ""},
		// The pre-creation connector candidate route has the same contract: the
		// creation flow needs the caller's own connections before a draft exists.
		{http.MethodGet, "/console/api/digital-human-connector-candidates", workspacemodel.WorkspacePermissionDigitalHumanCreate},
		{http.MethodPost, "/console/api/digital-human-connector-candidates", ""},
		{http.MethodGet, "/console/api/digital-humans/:id/draft/binding-candidates", workspacemodel.WorkspacePermissionDigitalHumanView},
		{http.MethodGet, "/console/api/digital-humans/:id/runs/:run_id", workspacemodel.WorkspacePermissionDigitalHumanLogsView},
		{http.MethodGet, "/console/api/digital-humans/:id/memory", workspacemodel.WorkspacePermissionDigitalHumanMemoryView},
		{http.MethodPost, "/console/api/digital-humans/:id/memory/proposals", workspacemodel.WorkspacePermissionDigitalHumanMemoryManage},
		{http.MethodPatch, "/console/api/digital-humans/:id/memory/state", workspacemodel.WorkspacePermissionDigitalHumanMemoryManage},
		{http.MethodPost, "/console/api/digital-humans/:id/memory/operations/:operation_id/undo", workspacemodel.WorkspacePermissionDigitalHumanMemoryManage},
		{http.MethodPost, "/console/api/digital-humans/:id/memory/repair", workspacemodel.WorkspacePermissionDigitalHumanMemoryManage},
		{http.MethodPost, "/console/api/digital-humans/:id/runs/:run_id/cancel", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodGet, "/console/api/digital-humans/:id/runtime/conversations", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodGet, "/console/api/digital-humans/:id/runtime/conversations/:conversation_id/messages", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodPatch, "/console/api/digital-humans/:id/runtime/conversations/:conversation_id", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodDelete, "/console/api/digital-humans/:id/runtime/conversations/:conversation_id", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodGet, "/console/api/digital-humans/:id/channels/installations", workspacemodel.WorkspacePermissionDigitalHumanView},
		{http.MethodGet, "/console/api/digital-humans/:id/channels/discoveries", workspacemodel.WorkspacePermissionDigitalHumanUpdate},
		{http.MethodPost, "/console/api/digital-humans/:id/channels/installations", workspacemodel.WorkspacePermissionDigitalHumanUpdate},
		{http.MethodDelete, "/console/api/digital-humans/:id/channels/bindings/:binding_id", workspacemodel.WorkspacePermissionDigitalHumanUpdate},
		{http.MethodGet, "/console/api/digital-humans/:id/channels/events", workspacemodel.WorkspacePermissionDigitalHumanLogsView},
		{http.MethodGet, "/console/api/digital-humans/:id/channels/deliveries", workspacemodel.WorkspacePermissionDigitalHumanLogsView},
		{http.MethodPost, "/console/api/digital-humans/:id/channels/deliveries/:delivery_id/retry", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodPost, "/console/api/digital-humans/:id/channels/deliveries/:delivery_id/dismiss", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodGet, "/console/api/digital-humans/:id/tasks", workspacemodel.WorkspacePermissionDigitalHumanView},
		{http.MethodPost, "/console/api/digital-humans/:id/tasks", workspacemodel.WorkspacePermissionDigitalHumanUpdate},
		{http.MethodPatch, "/console/api/digital-humans/:id/tasks/:task_id", workspacemodel.WorkspacePermissionDigitalHumanUpdate},
		{http.MethodGet, "/console/api/digital-humans/:id/my-tasks", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodPost, "/console/api/digital-humans/:id/my-tasks", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodPatch, "/console/api/digital-humans/:id/my-tasks/:task_id", workspacemodel.WorkspacePermissionDigitalHumanRun},
		{http.MethodPost, "/console/api/digital-humans/:id/my-tasks/:task_id/actions/:action", workspacemodel.WorkspacePermissionDigitalHumanRun},
	}
	for _, test := range tests {
		if got := digitalHumanPermissionForRequest(test.method, test.path); got != test.want {
			t.Errorf("permission(%s %s) = %s, want %s", test.method, test.path, got, test.want)
		}
	}
}

type digitalHumanRoutesAccountService struct{ interfaces.AccountService }
type digitalHumanRoutesOrganizationService struct{ interfaces.OrganizationService }

func TestRegisterDigitalHumanRoutesRegistersLifecycleAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")

	RegisterDigitalHumanRoutes(group, DigitalHumanRouteDeps{
		Service:                 digitalHumanRoutesService{},
		Runtime:                 digitalHumanRoutesRuntime{},
		Channels:                digitalHumanRoutesChannels{},
		Tasks:                   digitalHumanRoutesTasks{},
		Access:                  &digitalhuman.AccessService{},
		AccountService:          digitalHumanRoutesAccountService{},
		OrganizationService:     digitalHumanRoutesOrganizationService{},
		ApplicationErrorCatalog: digitalHumanRoutesCatalog(t),
	})

	routes := router.Routes()
	if got, want := len(routes), 78; got != want {
		t.Fatalf("registered route count = %d, want %d", got, want)
	}
	want := map[string]bool{
		"GET /console/api/digital-human-skill-candidates":                                                                 false,
		"GET /console/api/digital-human-connector-candidates":                                                             false,
		"POST /console/api/digital-humans":                                                                                false,
		"GET /console/api/digital-humans":                                                                                 false,
		"GET /console/api/digital-humans/:id":                                                                             false,
		"PATCH /console/api/digital-humans/:id":                                                                           false,
		"GET /console/api/digital-humans/:id/draft":                                                                       false,
		"PATCH /console/api/digital-humans/:id/draft":                                                                     false,
		"POST /console/api/digital-humans/:id/publish":                                                                    false,
		"GET /console/api/digital-humans/:id/versions":                                                                    false,
		"GET /console/api/digital-humans/:id/delete-impact":                                                               false,
		"DELETE /console/api/digital-humans/:id":                                                                          false,
		"GET /console/api/digital-humans/:id/draft/bindings":                                                              false,
		"PUT /console/api/digital-humans/:id/draft/bindings":                                                              false,
		"POST /console/api/digital-humans/:id/invocations":                                                                false,
		"GET /console/api/digital-humans/:id/access":                                                                      false,
		"PATCH /console/api/digital-humans/:id/access":                                                                    false,
		"GET /console/api/digital-humans/:id/api-keys":                                                                    false,
		"POST /console/api/digital-humans/:id/api-keys":                                                                   false,
		"PATCH /console/api/digital-humans/:id/api-keys/:api_key_id":                                                      false,
		"DELETE /console/api/digital-humans/:id/api-keys/:api_key_id":                                                     false,
		"GET /console/api/digital-humans/:id/memory":                                                                      false,
		"POST /console/api/digital-humans/:id/memory/proposals":                                                           false,
		"PATCH /console/api/digital-humans/:id/memory/state":                                                              false,
		"POST /console/api/digital-humans/:id/memory/operations/:operation_id/undo":                                       false,
		"POST /console/api/digital-humans/:id/memory/repair":                                                              false,
		"GET /console/api/digital-humans/:id/draft/binding-candidates":                                                    false,
		"GET /console/api/digital-humans/:id/runs":                                                                        false,
		"GET /console/api/digital-humans/:id/runs/:run_id":                                                                false,
		"POST /console/api/digital-humans/:id/runs/:run_id/cancel":                                                        false,
		"GET /console/api/digital-humans/:id/runtime/conversations":                                                       false,
		"GET /console/api/digital-humans/:id/runtime/search":                                                              false,
		"GET /console/api/digital-humans/:id/runtime/conversations/:conversation_id/messages":                             false,
		"POST /console/api/digital-humans/:id/runtime/conversations/:conversation_id/messages/:message_id/runtime-inputs": false,
		"POST /console/api/digital-humans/:id/runtime/conversations/:conversation_id/messages/:message_id/user-input/:request_id/continue": false,
		"GET /console/api/digital-humans/:id/runtime/conversations/:conversation_id/events":                                                false,
		"POST /console/api/digital-humans/:id/channels/installations":                                                                      false,
		"GET /console/api/digital-humans/:id/channels/installations":                                                                       false,
		"PATCH /console/api/digital-humans/:id/channels/installations/:installation_id":                                                    false,
		"DELETE /console/api/digital-humans/:id/channels/installations/:installation_id":                                                   false,
		"GET /console/api/digital-humans/:id/channels/events":                                                                              false,
		"GET /console/api/digital-humans/:id/channels/deliveries":                                                                          false,
		"POST /console/api/digital-humans/:id/channels/deliveries/:delivery_id/retry":                                                      false,
		"POST /console/api/digital-humans/:id/channels/deliveries/:delivery_id/dismiss":                                                    false,
		"GET /console/api/digital-humans/:id/tasks":                                                                                        false,
		"POST /console/api/digital-humans/:id/tasks":                                                                                       false,
		"GET /console/api/digital-humans/:id/tasks/:task_id":                                                                               false,
		"PATCH /console/api/digital-humans/:id/tasks/:task_id":                                                                             false,
		"POST /console/api/digital-humans/:id/tasks/:task_id/actions/:action":                                                              false,
		"GET /console/api/digital-humans/:id/tasks/:task_id/runs":                                                                          false,
		"GET /console/api/digital-humans/:id/tasks/:task_id/activities":                                                                    false,
		"GET /console/api/digital-humans/:id/my-tasks":                                                                                     false,
		"POST /console/api/digital-humans/:id/my-tasks":                                                                                    false,
		"GET /console/api/digital-humans/:id/my-tasks/:task_id":                                                                            false,
		"PATCH /console/api/digital-humans/:id/my-tasks/:task_id":                                                                          false,
		"POST /console/api/digital-humans/:id/my-tasks/:task_id/actions/:action":                                                           false,
		"GET /console/api/digital-humans/:id/my-tasks/:task_id/runs":                                                                       false,
		"GET /console/api/digital-humans/:id/my-tasks/:task_id/activities":                                                                 false,
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, registered := range want {
		if !registered {
			t.Errorf("route %s was not registered", route)
		}
	}
}

func TestRegisterDigitalHumanRoutesFailsFastOnMissingDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defer func() {
		if recover() == nil {
			t.Fatal("RegisterDigitalHumanRoutes() did not panic")
		}
	}()
	RegisterDigitalHumanRoutes(gin.New().Group("/console/api"), DigitalHumanRouteDeps{})
}

func digitalHumanRoutesCatalog(t *testing.T) *appcatalog.Catalog {
	t.Helper()
	definitions := append(appcatalog.DefaultDefinitions(), digitalhuman.CatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
