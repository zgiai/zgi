package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/util"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"gorm.io/datatypes"
)

type runtimeHandlerServiceStub struct {
	RuntimeHandlerService
	submit            func(context.Context, Scope, uuid.UUID, ManualInvocationRequest) (*Run, bool, error)
	get               func(context.Context, Scope, uuid.UUID, uuid.UUID) (*Run, error)
	runtimeLog        func(context.Context, Scope, uuid.UUID, uuid.UUID) (*RunRuntimeLog, error)
	cancel            func(context.Context, Scope, uuid.UUID, uuid.UUID) (*Run, error)
	candidates        func(context.Context, Scope, uuid.UUID, ResourceCandidateQuery) (*ResourceCandidateList, error)
	listConversations func(context.Context, Scope, uuid.UUID, int, int) ([]*runtimemodel.Conversation, int64, error)
}

func (s runtimeHandlerServiceStub) SubmitManual(ctx context.Context, scope Scope, id uuid.UUID, request ManualInvocationRequest) (*Run, bool, error) {
	return s.submit(ctx, scope, id, request)
}

func (s runtimeHandlerServiceStub) GetRun(ctx context.Context, scope Scope, id, runID uuid.UUID) (*Run, error) {
	return s.get(ctx, scope, id, runID)
}

func (s runtimeHandlerServiceStub) GetRunRuntimeLog(ctx context.Context, scope Scope, id, runID uuid.UUID) (*RunRuntimeLog, error) {
	return s.runtimeLog(ctx, scope, id, runID)
}

func (s runtimeHandlerServiceStub) CancelRun(ctx context.Context, scope Scope, id, runID uuid.UUID) (*Run, error) {
	return s.cancel(ctx, scope, id, runID)
}

func (s runtimeHandlerServiceStub) ListResourceCandidates(ctx context.Context, scope Scope, id uuid.UUID, query ResourceCandidateQuery) (*ResourceCandidateList, error) {
	return s.candidates(ctx, scope, id, query)
}

func (s runtimeHandlerServiceStub) ListConversations(ctx context.Context, scope Scope, id uuid.UUID, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	return s.listConversations(ctx, scope, id, page, limit)
}

func TestRuntimeHandlerAcceptsDurableManualInvocation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope, digitalHumanID, runID, requestID := testScope(), uuid.New(), uuid.New(), uuid.New()
	service := runtimeHandlerServiceStub{submit: func(_ context.Context, gotScope Scope, gotID uuid.UUID, request ManualInvocationRequest) (*Run, bool, error) {
		if gotScope != scope || gotID != digitalHumanID || request.RequestID != requestID || request.Query != "hello" {
			t.Fatalf("submit = scope %#v, id %s, request %#v", gotScope, gotID, request)
		}
		return &Run{ID: runID, Status: RunStatusQueued}, true, nil
	}}
	router := digitalHumanRuntimeHandlerRouter(t, service, scope)
	request := httptest.NewRequest(http.MethodPost, "/digital-humans/"+digitalHumanID.String()+"/invocations", bytes.NewBufferString(`{"request_id":"`+requestID.String()+`","query":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data struct {
			Created bool `json:"created"`
			Run     Run  `json:"run"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Data.Created || body.Data.Run.ID != runID {
		t.Fatalf("response = %#v", body)
	}
}

func TestRuntimeHandlerRejectsClientSuppliedAgentIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope, digitalHumanID := testScope(), uuid.New()
	router := digitalHumanRuntimeHandlerRouter(t, runtimeHandlerServiceStub{}, scope)
	request := httptest.NewRequest(http.MethodPost, "/digital-humans/"+digitalHumanID.String()+"/invocations", bytes.NewBufferString(`{"request_id":"`+uuid.NewString()+`","query":"hello","agent_id":"forbidden"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertDigitalHumanError(t, recorder, http.StatusBadRequest, AppCodeRequestInvalid.String())
}

func TestRuntimeHandlerProjectsScopedRunNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope, digitalHumanID, runID := testScope(), uuid.New(), uuid.New()
	service := runtimeHandlerServiceStub{get: func(context.Context, Scope, uuid.UUID, uuid.UUID) (*Run, error) {
		return nil, ErrRunNotFound
	}}
	router := digitalHumanRuntimeHandlerRouter(t, service, scope)
	request := httptest.NewRequest(http.MethodGet, "/digital-humans/"+digitalHumanID.String()+"/runs/"+runID.String(), nil)
	request.Header.Set("Accept-Language", "zh-CN")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertDigitalHumanError(t, recorder, http.StatusNotFound, AppCodeRunNotFound.String())
}

func TestRuntimeHandlerReturnsScopedRuntimeLogSteps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope, digitalHumanID, runID := testScope(), uuid.New(), uuid.New()
	service := runtimeHandlerServiceStub{runtimeLog: func(_ context.Context, gotScope Scope, gotID, gotRunID uuid.UUID) (*RunRuntimeLog, error) {
		if gotScope != scope || gotID != digitalHumanID || gotRunID != runID {
			t.Fatalf("runtime log scope = %#v, digital human = %s, run = %s", gotScope, gotID, gotRunID)
		}
		return &RunRuntimeLog{Steps: []dto.AgentRuntimeStep{{ID: "model-1", Type: "model_call", Status: "succeeded"}}}, nil
	}}
	router := digitalHumanRuntimeHandlerRouter(t, service, scope)
	request := httptest.NewRequest(http.MethodGet, "/digital-humans/"+digitalHumanID.String()+"/runs/"+runID.String()+"/runtime-log", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"type":"model_call"`)) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRuntimeHandlerRedactsCredentialReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope, digitalHumanID, runID := testScope(), uuid.New(), uuid.New()
	service := runtimeHandlerServiceStub{get: func(context.Context, Scope, uuid.UUID, uuid.UUID) (*Run, error) {
		return &Run{
			ID:                runID,
			RunSpec:           datatypes.JSON(`{"bindings":[{"resource_id":"safe","credential_ref":"vault/private"}]}`),
			ExecutionSnapshot: datatypes.JSON(`{"bindings":[{"credential_ref":"vault/private"}]}`),
			ExecutionTrace:    datatypes.JSON(`{"skill_invocations":[{"skill_id":"safe","access_token":"secret"}]}`),
		}, nil
	}}
	router := digitalHumanRuntimeHandlerRouter(t, service, scope)
	request := httptest.NewRequest(http.MethodGet, "/digital-humans/"+digitalHumanID.String()+"/runs/"+runID.String(), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || bytes.Contains(recorder.Body.Bytes(), []byte("vault/private")) ||
		bytes.Contains(recorder.Body.Bytes(), []byte("secret")) || !bytes.Contains(recorder.Body.Bytes(), []byte("safe")) {
		t.Fatalf("unsafe run response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRuntimeHandlerListsScopedResourceCandidates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope, digitalHumanID := testScope(), uuid.New()
	service := runtimeHandlerServiceStub{candidates: func(_ context.Context, gotScope Scope, gotID uuid.UUID, query ResourceCandidateQuery) (*ResourceCandidateList, error) {
		if gotScope != scope || gotID != digitalHumanID || query.Type != "workflow" || query.Search != "publish" || query.Limit != 25 {
			t.Fatalf("candidate request = scope %#v, id %s, query %#v", gotScope, gotID, query)
		}
		return &ResourceCandidateList{Items: []ResourceCandidate{{
			Type: "workflow", ResourceID: "workflow-1", DisplayName: "Published workflow",
			ResourceRevision: "sha256:safe", Available: true,
		}}, Total: 1}, nil
	}}
	router := digitalHumanRuntimeHandlerRouter(t, service, scope)
	request := httptest.NewRequest(http.MethodGet, "/digital-humans/"+digitalHumanID.String()+"/draft/binding-candidates?type=workflow&query=publish&limit=25", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte("Published workflow")) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRuntimeHandlerListsDigitalHumanConversationsAndRedactsMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope, digitalHumanID, conversationID := testScope(), uuid.New(), uuid.New()
	service := runtimeHandlerServiceStub{listConversations: func(_ context.Context, gotScope Scope, gotID uuid.UUID, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
		if gotScope != scope || gotID != digitalHumanID || page != 2 || limit != 10 {
			t.Fatalf("list conversations = scope %#v, id %s, page %d, limit %d", gotScope, gotID, page, limit)
		}
		return []*runtimemodel.Conversation{{
			ID: conversationID, OrganizationID: scope.OrganizationID, WorkspaceID: &scope.WorkspaceID,
			AccountID: scope.ActorID, Title: "Research", Source: digitalHumanConversationSource,
			Metadata:  map[string]interface{}{"safe": "visible", "access_token": "must-not-leak"},
			CreatedAt: time.Unix(10, 0), UpdatedAt: time.Unix(20, 0),
		}}, 21, nil
	}}
	router := digitalHumanRuntimeHandlerRouter(t, service, scope)
	request := httptest.NewRequest(http.MethodGet, "/digital-humans/"+digitalHumanID.String()+"/runtime/conversations?page=2&limit=10", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte("Research")) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte("visible")) || bytes.Contains(recorder.Body.Bytes(), []byte("must-not-leak")) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"has_more":true`)) {
		t.Fatalf("unsafe conversation response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func digitalHumanRuntimeHandlerRouter(t *testing.T, service RuntimeHandlerService, scope Scope) *gin.Engine {
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
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, scope.OrganizationID.String())
		util.SetWorkspaceID(c, scope.WorkspaceID.String())
		c.Set("account_id", scope.ActorID.String())
		c.Next()
	})
	NewRuntimeHandler(service, projector).RegisterRoutes(router.Group(""))
	return router
}
