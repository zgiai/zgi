package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	"github.com/zgiai/zgi/api/internal/util"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type handlerServiceStub struct {
	HandlerService
	create func(context.Context, Scope, CreateRequest) (*Detail, error)
	update func(context.Context, Scope, uuid.UUID, UpdateRequest) (*Detail, error)
}

func (s handlerServiceStub) Create(ctx context.Context, scope Scope, request CreateRequest) (*Detail, error) {
	return s.create(ctx, scope, request)
}

func (s handlerServiceStub) Update(ctx context.Context, scope Scope, id uuid.UUID, request UpdateRequest) (*Detail, error) {
	return s.update(ctx, scope, id, request)
}

func TestHandlerCreatesScopedDigitalHuman(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	id := uuid.New()
	service := handlerServiceStub{create: func(_ context.Context, gotScope Scope, request CreateRequest) (*Detail, error) {
		if gotScope != scope {
			t.Fatalf("scope = %#v, want %#v", gotScope, scope)
		}
		if request.Name != "Atlas" {
			t.Fatalf("request = %#v", request)
		}
		return &Detail{Definition: &Definition{ID: id, Name: request.Name}, Draft: &Version{}}, nil
	}}
	router := digitalHumanHandlerRouter(t, service, &scope)
	request := httptest.NewRequest(http.MethodPost, "/digital-humans", bytes.NewBufferString(`{"name":"Atlas"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data Detail `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Definition == nil || body.Data.Definition.ID != id {
		t.Fatalf("response = %#v", body)
	}
}

func TestHandlerRejectsUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	router := digitalHumanHandlerRouter(t, handlerServiceStub{}, &scope)
	request := httptest.NewRequest(http.MethodPost, "/digital-humans", bytes.NewBufferString(`{"name":"Atlas","agent_id":"forbidden"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	assertDigitalHumanError(t, recorder, http.StatusBadRequest, AppCodeRequestInvalid.String())
}

func TestHandlerLocalizesRevisionConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	id := uuid.New()
	service := handlerServiceStub{update: func(context.Context, Scope, uuid.UUID, UpdateRequest) (*Detail, error) {
		return nil, applicationError(ErrRevisionConflict, "digital_human.update")
	}}
	router := digitalHumanHandlerRouter(t, service, &scope)
	request := httptest.NewRequest(http.MethodPatch, "/digital-humans/"+id.String(), bytes.NewBufferString(`{"expected_revision":1,"name":"Nova"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "zh-CN")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	assertDigitalHumanError(t, recorder, http.StatusConflict, AppCodeRevisionConflict.String())
	if body := recorder.Body.String(); !bytes.Contains([]byte(body), []byte("数字人配置已发生变化")) {
		t.Fatalf("body = %s", body)
	}
}

func TestHandlerRequiresWorkspaceScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := digitalHumanHandlerRouter(t, handlerServiceStub{}, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/digital-humans", nil))
	assertDigitalHumanError(t, recorder, http.StatusUnauthorized, appcatalog.CodeAuthenticationRequired.String())
}

func TestHandlerProjectsSafeExecutionCapability(t *testing.T) {
	scope := testScope()
	t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", `{"digital_human:console":["`+scope.OrganizationID.String()+`"]}`)
	h := &Handler{executionApprovals: executionruntime.Approvals{
		Enabled:          true,
		GatewayReady:     true,
		DefaultProfileID: "standard",
		Profiles: map[string]executionruntime.ProfileApproval{
			"standard": {Backend: "local-gvisor", OrganizationIDs: []string{scope.OrganizationID.String()}, PolicyIDs: []string{"offline"}},
		},
	}}
	result := h.decorateExecutionRuntimeCapability(scope, &Detail{})
	detail, ok := result.(*Detail)
	if !ok || detail.ExecutionRuntimeCapability == nil || !detail.ExecutionRuntimeCapability.Available || !detail.ExecutionRuntimeCapability.CanEnable {
		t.Fatalf("capability = %#v", result)
	}
	encoded, err := json.Marshal(detail.ExecutionRuntimeCapability)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("standard")) || bytes.Contains(encoded, []byte("local-gvisor")) {
		t.Fatalf("runtime internals leaked in capability: %s", encoded)
	}
	if entry := detail.ExecutionRuntimeCapability.EntryCapabilities["console"]; !entry.Available || entry.Reason != "" {
		t.Fatalf("console capability = %+v", entry)
	}
	if entry := detail.ExecutionRuntimeCapability.EntryCapabilities["channel"]; entry.Available || entry.Reason != "entry_not_enabled" {
		t.Fatalf("channel capability = %+v", entry)
	}
}

func TestHandlerProjectsDigitalHumanEntryAdmission(t *testing.T) {
	scope := testScope()
	base := &Handler{executionApprovals: executionruntime.Approvals{
		Enabled:          true,
		GatewayReady:     true,
		DefaultProfileID: "standard",
		Profiles: map[string]executionruntime.ProfileApproval{
			"standard": {Backend: "local-gvisor", OrganizationIDs: []string{scope.OrganizationID.String()}, PolicyIDs: []string{"offline"}},
		},
	}}

	t.Run("explicit empty admission closes every entry", func(t *testing.T) {
		t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", `{}`)
		result := base.decorateExecutionRuntimeCapability(scope, &Detail{}).(*Detail)
		capability := result.ExecutionRuntimeCapability
		if capability.Available || capability.CanEnable || capability.Reason != "entry_not_enabled" {
			t.Fatalf("capability = %+v", capability)
		}
		if len(capability.EntryCapabilities) != 5 {
			t.Fatalf("entry capabilities = %+v", capability.EntryCapabilities)
		}
	})
	t.Run("one admitted entry makes capability usable", func(t *testing.T) {
		t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", `{"digital_human:channel":["`+scope.OrganizationID.String()+`"]}`)
		result := base.decorateExecutionRuntimeCapability(scope, &Detail{}).(*Detail)
		capability := result.ExecutionRuntimeCapability
		if !capability.Available || !capability.CanEnable || capability.Reason != "" {
			t.Fatalf("capability = %+v", capability)
		}
		if !capability.EntryCapabilities["channel"].Available || capability.EntryCapabilities["console"].Available {
			t.Fatalf("entry capabilities = %+v", capability.EntryCapabilities)
		}
	})
	t.Run("all authenticated organizations skip the entry allowlist", func(t *testing.T) {
		// A private deployment opens the runtime to every organization with
		// one setting; the organization is in neither the profile nor any
		// entry allowlist.
		t.Setenv("ZGI_AGENT_RUNTIME_ORG_MODE", "all_authenticated")
		t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", "")
		h := &Handler{executionApprovals: executionruntime.Approvals{
			Enabled: true, GatewayReady: true, DefaultProfileID: "standard",
			OrganizationAccessMode: executionruntime.OrganizationAccessModeAllAuthenticated,
			Profiles: map[string]executionruntime.ProfileApproval{
				"standard": {Backend: "local-gvisor", PolicyIDs: []string{"offline"}},
			},
		}}
		capability := h.decorateExecutionRuntimeCapability(scope, &Detail{}).(*Detail).ExecutionRuntimeCapability
		if !capability.Available || !capability.CanEnable || capability.Reason != "" {
			t.Fatalf("capability = %+v", capability)
		}
		for entry, admitted := range capability.EntryCapabilities {
			if !admitted.Available {
				t.Fatalf("entry %s refused in all_authenticated mode: %+v", entry, admitted)
			}
		}
	})
	t.Run("legacy empty environment does not admit digital human", func(t *testing.T) {
		t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", "")
		result := base.decorateExecutionRuntimeCapability(scope, &Detail{}).(*Detail)
		capability := result.ExecutionRuntimeCapability
		if capability.Available || capability.CanEnable || capability.Reason != "entry_not_enabled" {
			t.Fatalf("capability = %+v", capability)
		}
	})
}

func digitalHumanHandlerRouter(t *testing.T, service HandlerService, scope *Scope) *gin.Engine {
	t.Helper()
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if scope != nil {
		router.Use(func(c *gin.Context) {
			util.SetOrganizationID(c, scope.OrganizationID.String())
			util.SetWorkspaceID(c, scope.WorkspaceID.String())
			c.Set("account_id", scope.ActorID.String())
			c.Next()
		})
	}
	NewHandler(service, projector).RegisterRoutes(router.Group(""))
	return router
}

func assertDigitalHumanError(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != wantCode {
		t.Fatalf("code = %q, want %q; body = %s", body.Code, wantCode, recorder.Body.String())
	}
}
