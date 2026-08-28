package digitalhuman

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/util"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type channelHandlerServiceStub struct {
	ChannelHandlerService
	handleWebhook       func(context.Context, string, uuid.UUID, channelruntime.WebhookRequest) (*ChannelWebhookResult, error)
	probeWebhook        func(context.Context, string, uuid.UUID) error
	verifyInstallation  func(context.Context, Scope, uuid.UUID, uuid.UUID) (*ChannelInstallationVerification, error)
	startConnectionTest func(context.Context, Scope, uuid.UUID, uuid.UUID, StartChannelConnectionTestRequest) (*ChannelConnectionTest, error)
	deleteInstallation  func(context.Context, Scope, uuid.UUID, uuid.UUID) error
}

func (stub channelHandlerServiceStub) HandleWebhook(ctx context.Context, provider string, installationID uuid.UUID, request channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
	return stub.handleWebhook(ctx, provider, installationID, request)
}

func (stub channelHandlerServiceStub) ProbeWebhook(ctx context.Context, provider string, installationID uuid.UUID) error {
	if stub.probeWebhook == nil {
		return nil
	}
	return stub.probeWebhook(ctx, provider, installationID)
}

func (stub channelHandlerServiceStub) DeleteInstallation(ctx context.Context, scope Scope, digitalHumanID, installationID uuid.UUID) error {
	return stub.deleteInstallation(ctx, scope, digitalHumanID, installationID)
}

func (stub channelHandlerServiceStub) VerifyInstallation(ctx context.Context, scope Scope, digitalHumanID, installationID uuid.UUID) (*ChannelInstallationVerification, error) {
	return stub.verifyInstallation(ctx, scope, digitalHumanID, installationID)
}

func (stub channelHandlerServiceStub) StartConnectionTest(
	ctx context.Context,
	scope Scope,
	digitalHumanID, installationID uuid.UUID,
	request StartChannelConnectionTestRequest,
) (*ChannelConnectionTest, error) {
	return stub.startConnectionTest(ctx, scope, digitalHumanID, installationID, request)
}

func TestChannelInstallationVerifyHandlerUsesScopedInstallation(t *testing.T) {
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	digitalHumanID, installationID := uuid.New(), uuid.New()
	called := false
	service := channelHandlerServiceStub{verifyInstallation: func(_ context.Context, gotScope Scope, gotDigitalHumanID, gotInstallationID uuid.UUID) (*ChannelInstallationVerification, error) {
		called = true
		if gotScope != scope || gotDigitalHumanID != digitalHumanID || gotInstallationID != installationID {
			t.Fatalf("VerifyInstallation() = scope %#v, digital human %s, installation %s", gotScope, gotDigitalHumanID, gotInstallationID)
		}
		return &ChannelInstallationVerification{Status: "verified", CredentialVerified: true, BotIdentityVerified: true}, nil
	}}
	router := channelAuthenticatedTestRouter(t, service, scope)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/digital-humans/"+digitalHumanID.String()+"/channels/installations/"+installationID.String()+"/verify", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !called || !strings.Contains(recorder.Body.String(), `"status":"verified"`) {
		t.Fatalf("response = %d %s, called = %t", recorder.Code, recorder.Body.String(), called)
	}
}

func TestChannelConnectionTestHandlerAcceptsGroupConversationType(t *testing.T) {
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	digitalHumanID, installationID := uuid.New(), uuid.New()
	service := channelHandlerServiceStub{startConnectionTest: func(
		_ context.Context,
		gotScope Scope,
		gotDigitalHumanID, gotInstallationID uuid.UUID,
		request StartChannelConnectionTestRequest,
	) (*ChannelConnectionTest, error) {
		if gotScope != scope || gotDigitalHumanID != digitalHumanID || gotInstallationID != installationID ||
			request.ConversationType != channelruntime.ConversationTypeGroup {
			t.Fatalf("StartConnectionTest() = scope %#v, digital human %s, installation %s, request %#v", gotScope, gotDigitalHumanID, gotInstallationID, request)
		}
		return &ChannelConnectionTest{
			ID: uuid.New(), Status: ChannelConnectionTestAwaitingMessage,
			ConversationType: channelruntime.ConversationTypeGroup,
		}, nil
	}}
	router := channelAuthenticatedTestRouter(t, service, scope)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/digital-humans/"+digitalHumanID.String()+"/channels/installations/"+installationID.String()+"/connection-tests",
		bytes.NewBufferString(`{"conversation_type":"group"}`),
	)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"conversation_type":"group"`) {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestChannelInstallationDeleteHandlerUsesSimpleRequest(t *testing.T) {
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	digitalHumanID, installationID := uuid.New(), uuid.New()
	called := false
	service := channelHandlerServiceStub{deleteInstallation: func(_ context.Context, gotScope Scope, gotDigitalHumanID, gotInstallationID uuid.UUID) error {
		called = true
		if gotScope != scope || gotDigitalHumanID != digitalHumanID || gotInstallationID != installationID {
			t.Fatalf("DeleteInstallation() = scope %#v, digital human %s, installation %s", gotScope, gotDigitalHumanID, gotInstallationID)
		}
		return nil
	}}
	router := channelAuthenticatedTestRouter(t, service, scope)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/digital-humans/"+digitalHumanID.String()+"/channels/installations/"+installationID.String(), nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !called {
		t.Fatalf("response = %d %s, called = %t", recorder.Code, recorder.Body.String(), called)
	}
}

func TestChannelAuthenticatedRoutesDoNotExposeManualConversationBinding(t *testing.T) {
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	router := channelAuthenticatedTestRouter(t, channelHandlerServiceStub{}, scope)
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "/bindings") || strings.Contains(route.Path, "/discoveries") {
			t.Fatalf("manual conversation route is still exposed: %s %s", route.Method, route.Path)
		}
	}
}

func TestChannelWebhookHandlerReturnsExactChallenge(t *testing.T) {
	installationID := uuid.New()
	service := channelHandlerServiceStub{handleWebhook: func(_ context.Context, provider string, gotID uuid.UUID, request channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
		if provider != "feishu" || gotID != installationID || string(request.Body) != `{"challenge":"expected"}` {
			t.Fatalf("webhook request = %s, provider = %s, installation = %s", request.Body, provider, gotID)
		}
		return &ChannelWebhookResult{Response: channelruntime.WebhookResponse{
			StatusCode: http.StatusOK,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
			Body:       []byte(`{"challenge":"expected"}`),
		}}, nil
	}}
	router := channelWebhookTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webhooks/channels/feishu/"+installationID.String(), bytes.NewBufferString(`{"challenge":"expected"}`))

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"challenge":"expected"}` {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestChannelWebhookProbeChecksInstallationWithoutCreatingEvent(t *testing.T) {
	installationID := uuid.New()
	called := false
	service := channelHandlerServiceStub{probeWebhook: func(_ context.Context, provider string, gotID uuid.UUID) error {
		called = true
		if provider != "dingtalk" || gotID != installationID {
			t.Fatalf("probe provider = %q, installation = %s", provider, gotID)
		}
		return nil
	}}
	router := channelWebhookTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/webhooks/channels/dingtalk/"+installationID.String(), nil)

	router.ServeHTTP(recorder, request)

	if !called || recorder.Code != http.StatusOK || recorder.Body.String() != "success" {
		t.Fatalf("response = %d %q, called = %t", recorder.Code, recorder.Body.String(), called)
	}
}

func TestChannelWebhookHandlerDoesNotExposeInternalError(t *testing.T) {
	service := channelHandlerServiceStub{handleWebhook: func(context.Context, string, uuid.UUID, channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
		return nil, errors.New("database failed with verification_token=do-not-leak")
	}}
	router := channelWebhookTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webhooks/channels/feishu/"+uuid.NewString(), bytes.NewBufferString(`{"event":"value"}`))

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "verification_token") || strings.Contains(recorder.Body.String(), "do-not-leak") {
		t.Fatalf("public response exposed internal error: %s", recorder.Body.String())
	}
}

func TestChannelWebhookHandlerRejectsOversizedBodyBeforeService(t *testing.T) {
	called := false
	service := channelHandlerServiceStub{handleWebhook: func(context.Context, string, uuid.UUID, channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
		called = true
		return nil, nil
	}}
	router := channelWebhookTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost, "/webhooks/channels/feishu/"+uuid.NewString(),
		strings.NewReader(strings.Repeat("x", int(maxChannelWebhookRequestBytes)+1)),
	)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || called {
		t.Fatalf("response = %d, service called = %t", recorder.Code, called)
	}
}

func TestChannelWebhookHandlerForwardsGenericProviderMetadata(t *testing.T) {
	installationID := uuid.New()
	called := false
	service := channelHandlerServiceStub{handleWebhook: func(_ context.Context, provider string, gotID uuid.UUID, request channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
		called = true
		if provider != "acme-chat" || gotID != installationID || request.Headers.Get("X-Acme-Signature") != "signature" ||
			len(request.Query["workspace"]) != 1 || request.Query["workspace"][0] != "example" || request.ReceivedAt.IsZero() {
			t.Fatalf("generic webhook request = %#v, provider = %q, installation = %s", request, provider, gotID)
		}
		return &ChannelWebhookResult{Response: channelruntime.WebhookResponse{
			StatusCode: http.StatusAccepted,
			Headers:    http.Header{"X-Acme-Acknowledged": []string{"true"}},
			Body:       []byte(`{"accepted":true}`),
		}}, nil
	}}
	router := channelWebhookTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/webhooks/channels/acme-chat/"+installationID.String()+"?workspace=example",
		bytes.NewBufferString(`{"event":"value"}`),
	)
	request.Header.Set("X-Acme-Signature", "signature")

	router.ServeHTTP(recorder, request)

	if !called || recorder.Code != http.StatusAccepted || recorder.Header().Get("X-Acme-Acknowledged") != "true" ||
		strings.TrimSpace(recorder.Body.String()) != `{"accepted":true}` {
		t.Fatalf("response = %d %s, headers = %#v, called = %t", recorder.Code, recorder.Body.String(), recorder.Header(), called)
	}
}

func channelWebhookTestRouter(t *testing.T, service ChannelHandlerService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
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
	NewChannelHandler(service, projector).RegisterPublicRoutes(router.Group(""))
	return router
}

func channelAuthenticatedTestRouter(t *testing.T, service ChannelHandlerService, scope Scope) *gin.Engine {
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
	NewChannelHandler(service, projector).RegisterAuthenticatedRoutes(router.Group(""))
	return router
}
