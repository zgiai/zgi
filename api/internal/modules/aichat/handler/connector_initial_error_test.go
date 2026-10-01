package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorpreferences "github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestInitialConnectorErrorsUseSafeChatHTTPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		code core.ErrorCode
		want response.ErrorCode
	}{
		{core.ErrorCodeInvalidInput, response.ErrInvalidParam},
		{core.ErrorCodeConnectionNotReady, response.ErrInvalidParam},
		{core.ErrorCodeAccessDenied, response.ErrPermissionDenied},
		{core.ErrorCodeInternal, response.ErrSystemError},
	} {
		t.Run(string(test.code), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/console/api/aichat/chat", nil)
			err := fmt.Errorf("private diagnostic: %w", core.NewError(test.code, false, errors.New("private-upstream-key")))
			NewHandler(nil, nil).fail(ctx, err)
			var body struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != strconv.Itoa(test.want.Code) {
				t.Fatalf("code = %s, want %d", body.Code, test.want.Code)
			}
			if strings.Contains(recorder.Body.String(), "private") || strings.Contains(recorder.Body.String(), string(test.code)) {
				t.Fatalf("diagnostics leaked: %s", recorder.Body.String())
			}
		})
	}
}

func TestInitialConnectorDefaultsErrorsPreserveLegacyHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	definitions := append(appcatalog.DefaultDefinitions(), connectorpreferences.CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		language string
		locale   appcatalog.Locale
		code     apperror.Code
		legacy   response.ErrorCode
		status   int
	}{
		{"en-US,en;q=0.9", appcatalog.LocaleEnglishUS, connectorpreferences.AppCodeRevisionConflict, response.ErrInvalidParam, http.StatusBadRequest},
		{"zh-CN,zh;q=0.9", appcatalog.LocaleChineseSimplified, connectorpreferences.AppCodeRevisionConflict, response.ErrInvalidParam, http.StatusBadRequest},
		{"en-US", appcatalog.LocaleEnglishUS, connectorpreferences.AppCodeInternal, response.ErrSystemError, http.StatusInternalServerError},
		{"zh-CN", appcatalog.LocaleChineseSimplified, connectorpreferences.AppCodeInternal, response.ErrSystemError, http.StatusInternalServerError},
	} {
		t.Run(test.language, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/console/api/aichat/chat", nil)
			ctx.Request.Header.Set("Accept-Language", test.language)
			wrapped := apperror.Wrap(errors.New("private database detail and secret"), test.code,
				apperror.WithOperation("chat.initialize"))
			NewHandler(nil, nil, projector).fail(ctx, wrapped)
			if recorder.Code != test.status {
				t.Fatalf("HTTP status = %d, want legacy %d", recorder.Code, test.status)
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 2 || string(body["code"]) != `"`+strconv.Itoa(test.legacy.Code)+`"` {
				t.Fatalf("legacy JSON shape changed: %s", recorder.Body.String())
			}
			var message string
			if err := json.Unmarshal(body["message"], &message); err != nil {
				t.Fatal(err)
			}
			want, err := productCatalog.Present(test.code, test.locale, nil)
			if err != nil {
				t.Fatal(err)
			}
			if message != want.Message || message == test.legacy.Message {
				t.Fatalf("localized message = %q, want %q", message, want.Message)
			}
			if recorder.Header().Get(apptransport.HeaderApplicationErrorCode) != test.code.String() {
				t.Fatal("missing stable app error identity for reviewed retry")
			}
			if strings.Contains(recorder.Body.String(), "private") || strings.Contains(recorder.Body.String(), "secret") {
				t.Fatalf("cause leaked: %s", recorder.Body.String())
			}
		})
	}
}

func TestInitialConnectorDefaultsUnmappedProjectionKeepsSafeLegacyFallback(t *testing.T) {
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, appcatalog.DefaultDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	unmappedProjector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, projector := range []*apptransport.Projector{nil, unmappedProjector} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("POST", "/console/api/aichat/chat", nil)
		NewHandler(nil, nil, projector).fail(ctx, apperror.Wrap(errors.New("private diagnostic"), connectorpreferences.AppCodeRevisionConflict))
		var body response.Response
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusBadRequest || body.Code != strconv.Itoa(response.ErrInvalidParam.Code) || body.Message != response.ErrInvalidParam.Message {
			t.Fatalf("legacy fallback changed: %d %s", recorder.Code, recorder.Body.String())
		}
		if recorder.Header().Get(apptransport.HeaderApplicationErrorCode) != "" {
			t.Fatal("an unprojected error must not claim a cataloged identity")
		}
	}
}
