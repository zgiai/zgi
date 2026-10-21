package v1

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type preferenceRouteStub struct {
	org, owner uuid.UUID
	command    preferences.UpdateCommand
	calls      int
	err        error
}

func (stub *preferenceRouteStub) Get(_ context.Context, org, owner uuid.UUID) (preferences.Snapshot, error) {
	stub.org = org
	stub.owner = owner
	stub.calls++
	return preferences.Snapshot{Defaults: map[string]preferences.Default{}}, stub.err
}
func (stub *preferenceRouteStub) Update(_ context.Context, command preferences.UpdateCommand) (preferences.Snapshot, error) {
	stub.command = command
	stub.calls++
	return preferences.Snapshot{Revision: command.ExpectedRevision + 1, Defaults: map[string]preferences.Default{command.ConnectorID: {Selection: command.Selection}}}, stub.err
}
func preferenceTestRouter(t *testing.T, stub *preferenceRouteStub, org, owner uuid.UUID) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), preferences.CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, org.String())
		c.Set("account_id", owner.String())
		c.Next()
	})
	(&connectorHandler{preferences: stub, errorProjector: projector}).register(group)
	return router
}

func TestConnectorPreferenceRoutesUseSessionIdentityAndStableEnvelope(t *testing.T) {
	org, owner := uuid.New(), uuid.New()
	stub := &preferenceRouteStub{}
	router := preferenceTestRouter(t, stub, org, owner)
	get := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-preferences/chat/me?owner_account_id="+uuid.NewString(), "", nil)
	if get.Code != 200 || !strings.Contains(get.Body.String(), `"code":"0"`) || !strings.Contains(get.Body.String(), `"defaults":{}`) || stub.org != org || stub.owner != owner {
		t.Fatalf("GET = %d %s", get.Code, get.Body.String())
	}
	put := connectorRouteRequest(t, router, http.MethodPut, "/console/api/connector-preferences/chat/me/github", `{"mode":"none","expected_revision":0}`, map[string]string{"Idempotency-Key": "preference-route-mutation-0001"})
	if put.Code != 200 || stub.command.OrganizationID != org || stub.command.OwnerAccountID != owner || stub.command.IdempotencyKey != "preference-route-mutation-0001" || stub.command.ConnectorID != "github" || stub.command.Selection.Mode != preferences.ModeNone || !strings.Contains(put.Body.String(), `"revision":1`) {
		t.Fatalf("PUT = %d %s %#v", put.Code, put.Body.String(), stub.command)
	}
	before := stub.calls
	for _, body := range []string{`{"mode":"none"}`, `{"mode":"none","expected_revision":null}`, `{"mode":"none","expected_revision":0,"owner_account_id":"forged-owner"}`, `{"mode":"none","expected_revision":0,"key":"never-accept-credentials"}`} {
		response := connectorRouteRequest(t, router, http.MethodPut, "/console/api/connector-preferences/chat/me/github", body, nil)
		if response.Code != 400 || stub.calls != before || strings.Contains(response.Body.String(), "never-accept") {
			t.Fatalf("invalid request = %d %s", response.Code, response.Body.String())
		}
	}
}

func TestConnectorPreferenceRoutesLocalizeAllErrorsWithoutExposingCauses(t *testing.T) {
	for _, definition := range preferences.CatalogDefinitions() {
		for _, locale := range []string{"en-US", "zh-Hans"} {
			t.Run(definition.Code.String()+locale, func(t *testing.T) {
				stub := &preferenceRouteStub{err: apperror.Wrap(errors.New("private-key-and-storage-cause"), definition.Code)}
				router := preferenceTestRouter(t, stub, uuid.New(), uuid.New())
				response := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-preferences/chat/me", "", map[string]string{"Accept-Language": locale})
				if response.Code != definition.HTTPStatus || !strings.Contains(response.Body.String(), definition.Messages[appcatalog.Locale(locale)]) || strings.Contains(response.Body.String(), "private-key") || response.Header().Get(apptransport.HeaderApplicationErrorCode) != definition.Code.String() {
					t.Fatalf("projection = %d %s", response.Code, response.Body.String())
				}
			})
		}
	}
	stub := &preferenceRouteStub{}
	router := preferenceTestRouter(t, stub, uuid.Nil, uuid.Nil)
	response := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-preferences/chat/me", "", map[string]string{"Accept-Language": "zh-Hans"})
	if response.Code != http.StatusUnauthorized || stub.calls != 0 {
		t.Fatalf("anonymous preference read = %d", response.Code)
	}
}
