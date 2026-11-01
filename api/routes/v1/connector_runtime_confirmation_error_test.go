package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/util"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// A refused session grant keeps its legacy 403, code and retryable flag; only
// the message comes from the catalog in the person's language.
func TestConnectorRuntimeConfirmationRouteLocalizesSessionGrantRefusal(t *testing.T) {
	definitions, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), connectorcore.CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(definitions)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, acceptLanguage, cookie string
		want                         appcatalog.Locale
	}{
		{name: "english", acceptLanguage: "en-US", want: appcatalog.LocaleEnglishUS},
		{name: "chinese header", acceptLanguage: "zh-Hans", want: appcatalog.LocaleChineseSimplified},
		{name: "console cookie wins", acceptLanguage: "en-US", cookie: "zh-Hans", want: appcatalog.LocaleChineseSimplified},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := &connectorRuntimeConfirmationServiceSpy{err: connectorcore.NewError(connectorcore.ErrorCodeSessionGrantUnsupported, false, nil)}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			group := router.Group("/console/api")
			group.Use(func(c *gin.Context) {
				util.SetOrganizationID(c, uuid.NewString())
				c.Set("account_id", uuid.NewString())
				c.Next()
			})
			(&connectorRuntimeConfirmationHandler{service: service, recorder: service, errorProjector: projector}).register(group)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/console/api/connectors/runtime/confirmations",
				bytes.NewBufferString(`{"route_receipt":"`+uuid.NewString()+`","decision":"confirmed","remember_for_session":true}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept-Language", testCase.acceptLanguage)
			if testCase.cookie != "" {
				request.AddCookie(&http.Cookie{Name: "locale", Value: testCase.cookie})
			}
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var body connectorErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			definition, ok := definitions.Definition(connectorcore.AppCodeSessionGrantUnsupported)
			wantMessage := definition.Messages[testCase.want]
			if !ok || wantMessage == "" {
				t.Fatalf("catalog has no %s message", testCase.want)
			}
			if body.Code != connectorcore.ErrorCodeSessionGrantUnsupported || body.Retryable || body.Message != wantMessage {
				t.Fatalf("response = %#v, want message %q", body, wantMessage)
			}
			if got := recorder.Header().Get(apptransport.HeaderApplicationErrorCode); got != connectorcore.AppCodeSessionGrantUnsupported.String() {
				t.Fatalf("application error code header = %q", got)
			}
		})
	}
}
