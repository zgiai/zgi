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
	handleWebhook      func(context.Context, uuid.UUID, channelruntime.WebhookRequest) (*ChannelWebhookResult, error)
	deleteInstallation func(context.Context, Scope, uuid.UUID, uuid.UUID, DeleteChannelInstallationRequest) error
}

func (stub channelHandlerServiceStub) HandleWebhook(ctx context.Context, installationID uuid.UUID, request channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
	return stub.handleWebhook(ctx, installationID, request)
}

func (stub channelHandlerServiceStub) DeleteInstallation(ctx context.Context, scope Scope, digitalHumanID, installationID uuid.UUID, request DeleteChannelInstallationRequest) error {
	return stub.deleteInstallation(ctx, scope, digitalHumanID, installationID, request)
}

func TestChannelInstallationDeleteParsesRemoveBindings(t *testing.T) {
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	digitalHumanID, installationID := uuid.New(), uuid.New()
	called := false
	service := channelHandlerServiceStub{deleteInstallation: func(_ context.Context, gotScope Scope, gotDigitalHumanID, gotInstallationID uuid.UUID, request DeleteChannelInstallationRequest) error {
		called = true
		if gotScope != scope || gotDigitalHumanID != digitalHumanID || gotInstallationID != installationID || !request.RemoveBindings {
			t.Fatalf("DeleteInstallation() = scope %#v, digital human %s, installation %s, request %#v", gotScope, gotDigitalHumanID, gotInstallationID, request)
		}
		return nil
	}}
	router := channelAuthenticatedTestRouter(t, service, scope)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/digital-humans/"+digitalHumanID.String()+"/channels/installations/"+installationID.String()+"?remove_bindings=true", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !called {
		t.Fatalf("response = %d %s, called = %t", recorder.Code, recorder.Body.String(), called)
	}
}

func TestChannelWebhookHandlerReturnsExactChallenge(t *testing.T) {
	installationID := uuid.New()
	service := channelHandlerServiceStub{handleWebhook: func(_ context.Context, gotID uuid.UUID, request channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
		if gotID != installationID || string(request.Body) != `{"challenge":"expected"}` {
			t.Fatalf("webhook request = %s, installation = %s", request.Body, gotID)
		}
		return &ChannelWebhookResult{Challenge: "expected"}, nil
	}}
	router := channelWebhookTestRouter(t, service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webhooks/channels/feishu/"+installationID.String(), bytes.NewBufferString(`{"challenge":"expected"}`))

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"challenge":"expected"}` {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestChannelWebhookHandlerDoesNotExposeInternalError(t *testing.T) {
	service := channelHandlerServiceStub{handleWebhook: func(context.Context, uuid.UUID, channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
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
	service := channelHandlerServiceStub{handleWebhook: func(context.Context, uuid.UUID, channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
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
