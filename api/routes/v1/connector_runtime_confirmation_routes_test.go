package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"github.com/zgiai/zgi/api/internal/util"
)

const connectorRuntimeConfirmationRouteSecret = "connector-runtime-confirmation-route-secret"

func TestConnectorRuntimeConfirmationRouteAcceptsOnlyOpaqueDecisionAndProjectsSafeResult(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	receipt := uuid.New().String()
	decisionID := uuid.New()
	service := &connectorRuntimeConfirmationServiceSpy{result: connectorconfirmation.Result{
		State: connectorconfirmation.StateCompleted, DecisionID: decisionID,
		Origin: connectorexecution.ConfirmationOrigin{
			Surface: connectorcore.TargetTypeChat, ConversationID: connectorRuntimeConfirmationRouteSecret,
		},
		ExecutionResult: &connectorexecution.ExecutionResult{
			State: connectorexecution.ExecutionStateCompleted,
			Value: map[string]any{"provider_value": connectorRuntimeConfirmationRouteSecret},
		},
	}}
	router := newConnectorRuntimeConfirmationRouteTestRouter(service, organizationID, ownerID)

	recorder := performConnectorRuntimeConfirmationRequest(
		t,
		router,
		`{"route_receipt":"`+receipt+`","decision":"confirmed","remember_for_session":true}`,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(service.calls) != 1 {
		t.Fatalf("service calls = %d", len(service.calls))
	}
	command := service.calls[0]
	if command.OrganizationID != organizationID || command.OwnerAccountID != ownerID ||
		string(command.RouteReceipt) != receipt || command.Decision != connectorexecution.ConfirmationDecisionConfirmed ||
		!command.RememberForSession {
		t.Fatalf("service command = %#v", command)
	}
	if len(service.recordCalls) != 1 || service.recordCalls[0].result.ExecutionResult != nil ||
		service.recordCalls[0].result.Origin.ConversationID != connectorRuntimeConfirmationRouteSecret {
		t.Fatalf("resolution recorder calls = %#v", service.recordCalls)
	}
	var responseBody struct {
		Code string `json:"code"`
		Data struct {
			State              connectorconfirmation.State `json:"state"`
			DecisionID         *uuid.UUID                  `json:"decision_id"`
			ErrorCode          connectorcore.ErrorCode     `json:"error_code"`
			RememberForSession bool                        `json:"remember_for_session"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}
	if responseBody.Code != "0" || responseBody.Data.State != connectorconfirmation.StateCompleted ||
		responseBody.Data.DecisionID == nil || *responseBody.Data.DecisionID != decisionID ||
		responseBody.Data.ErrorCode != "" || !responseBody.Data.RememberForSession {
		t.Fatalf("response = %#v", responseBody)
	}
	for _, forbidden := range []string{
		connectorRuntimeConfirmationRouteSecret,
		`"origin"`, `"execution_result"`, `"value"`, `"credential"`, `"policy"`,
	} {
		if strings.Contains(strings.ToLower(recorder.Body.String()), strings.ToLower(forbidden)) {
			t.Fatalf("response exposed %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestConnectorRuntimeConfirmationRouteProjectsFailedAndUnknownTerminalStates(t *testing.T) {
	tests := []struct {
		name      string
		result    connectorconfirmation.Result
		wantState connectorconfirmation.State
		wantCode  connectorcore.ErrorCode
	}{
		{
			name: "failed",
			result: connectorconfirmation.Result{
				State: connectorconfirmation.StateFailed, ErrorCode: connectorcore.ErrorCodeProviderUnavailable,
			},
			wantState: connectorconfirmation.StateFailed, wantCode: connectorcore.ErrorCodeProviderUnavailable,
		},
		{
			name: "unknown",
			result: connectorconfirmation.Result{
				State: connectorconfirmation.StateUnknown, ErrorCode: connectorcore.ErrorCodeOutcomeUnknown,
			},
			wantState: connectorconfirmation.StateUnknown, wantCode: connectorcore.ErrorCodeOutcomeUnknown,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := &connectorRuntimeConfirmationServiceSpy{result: testCase.result}
			router := newConnectorRuntimeConfirmationRouteTestRouter(service, uuid.New(), uuid.New())
			recorder := performConnectorRuntimeConfirmationRequest(
				t,
				router,
				`{"route_receipt":"`+uuid.New().String()+`","decision":"confirmed"}`,
			)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var body struct {
				Data connectorRuntimeConfirmationResponse `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("json.Unmarshal(response) error = %v", err)
			}
			if body.Data.State != testCase.wantState || body.Data.ErrorCode != testCase.wantCode ||
				body.Data.DecisionID != nil {
				t.Fatalf("response data = %#v", body.Data)
			}
		})
	}
}

func TestConnectorRuntimeConfirmationRouteRejectsUnknownFieldsAndInvalidDecisions(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	receipt := uuid.New().String()
	tests := []string{
		``,
		`{}`,
		`{"route_receipt":"` + receipt + `"}`,
		`{"route_receipt":"` + receipt + `","decision":"remember"}`,
		`{"route_receipt":"forged","decision":"confirmed"}`,
		`{"route_receipt":"` + receipt + `","decision":"confirmed","input":{"secret":"` + connectorRuntimeConfirmationRouteSecret + `"}}`,
		`{"route_receipt":"` + receipt + `","decision":"confirmed","remember":true}`,
		`{"route_receipt":"` + receipt + `","decision":"confirmed"}{"route_receipt":"` + receipt + `","decision":"confirmed"}`,
	}
	for _, body := range tests {
		service := &connectorRuntimeConfirmationServiceSpy{}
		router := newConnectorRuntimeConfirmationRouteTestRouter(service, organizationID, ownerID)
		recorder := performConnectorRuntimeConfirmationRequest(t, router, body)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q status = %d, response = %s", body, recorder.Code, recorder.Body.String())
		}
		if len(service.calls) != 0 {
			t.Fatalf("body %q unexpectedly called service: %#v", body, service.calls)
		}
		if strings.Contains(recorder.Body.String(), connectorRuntimeConfirmationRouteSecret) {
			t.Fatalf("body %q leaked submitted input: %s", body, recorder.Body.String())
		}
	}
}

func TestConnectorRuntimeConfirmationRouteProjectsOwnerAndInternalErrorsSafely(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   connectorcore.ErrorCode
	}{
		{
			name: "owner mismatch", err: connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil),
			wantStatus: http.StatusForbidden, wantCode: connectorcore.ErrorCodeAccessDenied,
		},
		{
			name:       "session grant on a one-shot surface",
			err:        connectorcore.NewError(connectorcore.ErrorCodeSessionGrantUnsupported, false, nil),
			wantStatus: http.StatusForbidden, wantCode: connectorcore.ErrorCodeSessionGrantUnsupported,
		},
		{
			name: "unstructured internal", err: errors.New(connectorRuntimeConfirmationRouteSecret),
			wantStatus: http.StatusInternalServerError, wantCode: connectorcore.ErrorCodeInternal,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := &connectorRuntimeConfirmationServiceSpy{err: testCase.err}
			router := newConnectorRuntimeConfirmationRouteTestRouter(service, uuid.New(), uuid.New())
			recorder := performConnectorRuntimeConfirmationRequest(
				t,
				router,
				`{"route_receipt":"`+uuid.New().String()+`","decision":"cancelled"}`,
			)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var body connectorErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("json.Unmarshal(response) error = %v", err)
			}
			if body.Code != testCase.wantCode {
				t.Fatalf("error response = %#v", body)
			}
			if strings.Contains(recorder.Body.String(), connectorRuntimeConfirmationRouteSecret) {
				t.Fatalf("response leaked internal error: %s", recorder.Body.String())
			}
		})
	}
}

func TestConnectorRuntimeConfirmationRouteRequiresDurableHostResolutionBeforeSuccess(t *testing.T) {
	service := &connectorRuntimeConfirmationServiceSpy{
		result: connectorconfirmation.Result{
			State: connectorconfirmation.StateCompleted,
			Origin: connectorexecution.ConfirmationOrigin{
				Surface: connectorcore.TargetTypeChat, ConversationID: uuid.NewString(), MessageID: uuid.NewString(),
			},
		},
		recordErr: connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil),
	}
	router := newConnectorRuntimeConfirmationRouteTestRouter(service, uuid.New(), uuid.New())
	recorder := performConnectorRuntimeConfirmationRequest(
		t,
		router,
		`{"route_receipt":"`+uuid.NewString()+`","decision":"confirmed"}`,
	)
	if recorder.Code == http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(service.calls) != 1 || len(service.recordCalls) != 1 {
		t.Fatalf("continue calls = %d, record calls = %d", len(service.calls), len(service.recordCalls))
	}
}

func newConnectorRuntimeConfirmationRouteTestRouter(
	service ConnectorRuntimeConfirmationService,
	organizationID, ownerID uuid.UUID,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, organizationID.String())
		c.Set("account_id", ownerID.String())
		c.Next()
	})
	recorder, ok := service.(ConnectorRuntimeConfirmationResolutionRecorder)
	if !ok {
		panic("test confirmation service must also record host resolution")
	}
	(&connectorRuntimeConfirmationHandler{service: service, recorder: recorder}).register(group)
	return router
}

func performConnectorRuntimeConfirmationRequest(
	t *testing.T,
	router http.Handler,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/console/api/connectors/runtime/confirmations",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

type connectorRuntimeConfirmationServiceSpy struct {
	statusCalls  []connectorconfirmation.StatusQuery
	pendingCalls []struct {
		organizationID uuid.UUID
		ownerID        uuid.UUID
		page, pageSize int
	}
	result      connectorconfirmation.Result
	err         error
	recordErr   error
	calls       []connectorconfirmation.Command
	recordCalls []connectorRuntimeConfirmationRecordCall
}

func (service *connectorRuntimeConfirmationServiceSpy) ListPending(_ context.Context, organizationID, ownerID uuid.UUID, page, pageSize int) (connectorconfirmation.PendingResult, error) {
	service.pendingCalls = append(service.pendingCalls, struct {
		organizationID uuid.UUID
		ownerID        uuid.UUID
		page, pageSize int
	}{organizationID, ownerID, page, pageSize})
	return connectorconfirmation.PendingResult{Items: []connectorexecution.PendingConfirmationRef{{RouteReceipt: connectorexecution.RouteClaimReceipt(uuid.NewString()), CreatedAt: time.Now().UTC()}}, Page: page, PageSize: pageSize}, service.err
}

func (service *connectorRuntimeConfirmationServiceSpy) ReadStatus(_ context.Context, query connectorconfirmation.StatusQuery) (connectorconfirmation.StatusResult, error) {
	service.statusCalls = append(service.statusCalls, query)
	return connectorconfirmation.StatusResult{Resolution: "resolved", State: service.result.State, DecisionID: service.result.DecisionID, ErrorCode: service.result.ErrorCode}, service.err
}

func TestConnectorRuntimeStatusReadIsOwnerScopedAndDoesNotContinue(t *testing.T) {
	org, owner := uuid.New(), uuid.New()
	receipt := uuid.New().String()
	service := &connectorRuntimeConfirmationServiceSpy{result: connectorconfirmation.Result{State: connectorconfirmation.StateUnknown, ErrorCode: connectorcore.ErrorCodeOutcomeUnknown}}
	router := newConnectorRuntimeConfirmationRouteTestRouter(service, org, owner)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/console/api/connectors/runtime/confirmations/"+receipt, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"resolution":"resolved"`) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("read response: %d %s", response.Code, response.Body.String())
	}
	if len(service.calls) != 0 || len(service.recordCalls) != 0 || len(service.statusCalls) != 1 {
		t.Fatal("GET invoked a write path")
	}
	q := service.statusCalls[0]
	if q.OrganizationID != org || q.OwnerAccountID != owner || string(q.RouteReceipt) != receipt {
		t.Fatal("scope not supplied by authenticated actor")
	}
	for _, suffix := range []string{"invalid", receipt + "?decision=confirmed"} {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/console/api/connectors/runtime/confirmations/"+suffix, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid query: %d", response.Code)
		}
	}
	if len(service.statusCalls) != 1 {
		t.Fatal("invalid query reached service")
	}
}

func TestConnectorPendingConfirmationInboxIsOwnerScopedAndReadOnly(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	service := &connectorRuntimeConfirmationServiceSpy{}
	router := newConnectorRuntimeConfirmationRouteTestRouter(service, organizationID, ownerID)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/console/api/connectors/runtime/confirmations?page=2&page_size=1", nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"page":2`) {
		t.Fatalf("pending inbox response = %d %s", response.Code, response.Body.String())
	}
	if len(service.pendingCalls) != 1 || service.pendingCalls[0].organizationID != organizationID || service.pendingCalls[0].ownerID != ownerID || service.pendingCalls[0].page != 2 || service.pendingCalls[0].pageSize != 1 || len(service.calls) != 0 || len(service.recordCalls) != 0 {
		t.Fatal("pending inbox did not retain authenticated read-only owner scope")
	}
	for _, query := range []string{"?page=0", "?page_size=101", "?owner_account_id=" + uuid.NewString(), "?page=1&page=2"} {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/console/api/connectors/runtime/confirmations"+query, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid inbox query %q returned %d", query, response.Code)
		}
	}
	if len(service.pendingCalls) != 1 {
		t.Fatal("invalid inbox query reached service")
	}
}

type connectorRuntimeConfirmationRecordCall struct {
	command connectorconfirmation.Command
	result  connectorconfirmation.Result
}

func (service *connectorRuntimeConfirmationServiceSpy) Continue(
	_ context.Context,
	command connectorconfirmation.Command,
) (connectorconfirmation.Result, error) {
	service.calls = append(service.calls, command)
	return service.result, service.err
}

func (service *connectorRuntimeConfirmationServiceSpy) RecordConnectorConfirmationResolution(
	_ context.Context,
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) error {
	service.recordCalls = append(service.recordCalls, connectorRuntimeConfirmationRecordCall{
		command: command,
		result:  result,
	})
	return service.recordErr
}

func TestConnectorRuntimePresentationQueryIsExplicitAndReadOnly(t *testing.T) {
	service := &connectorRuntimeConfirmationServiceSpy{}
	router := newConnectorRuntimeConfirmationRouteTestRouter(service, uuid.New(), uuid.New())
	for _, test := range []struct {
		query string
		valid bool
	}{{"?presentation_version=1", true}, {"?presentation_version=2", false}, {"?presentation_version=1&presentation_version=1", false}, {"?presentation_version=1&decision=confirmed", false}, {"?presentation_version=%zz", false}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/console/api/connectors/runtime/confirmations/"+uuid.NewString()+test.query, nil))
		if (response.Code == http.StatusOK) != test.valid {
			t.Fatalf("unexpected status for %s: %d", test.query, response.Code)
		}
	}
	if len(service.statusCalls) != 1 || service.statusCalls[0].PresentationVersion != 1 || len(service.calls) != 0 || len(service.recordCalls) != 0 {
		t.Fatal("bad query dispatch")
	}
}
