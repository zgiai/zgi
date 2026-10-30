package agents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

// An over-budget query used to answer with the diagnostic
// "invalid input: query exceeds model context budget". The legacy envelope is
// kept, the message is the localized catalog copy and the catalog code is in
// the application error header.
func TestFailRuntimeQueryTooLongKeepsLegacyEnvelopeWithCatalogMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal,
		append(appcatalog.DefaultDefinitions(), runtimeservice.CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	cause := apperror.Wrap(fmt.Errorf("%w: query exceeds model context budget", runtimeservice.ErrInvalidInput), runtimeservice.CodeQueryTooLong)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/console/api/agents/chat", nil)
	c.Request.Header.Set("Accept-Language", "zh-CN")
	(&AgentsHandler{appErrorProjector: projector}).failRuntime(c, cause)

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != strconv.Itoa(response.ErrInvalidParam.Code) {
		t.Fatalf("code = %q, want legacy %d", body.Code, response.ErrInvalidParam.Code)
	}
	presentation, err := catalog.Present(runtimeservice.CodeQueryTooLong, appcatalog.LocaleChineseSimplified, nil)
	if err != nil {
		t.Fatal(err)
	}
	if body.Message != presentation.Message {
		t.Fatalf("message = %q, want %q", body.Message, presentation.Message)
	}
	if got := recorder.Header().Get(apptransport.HeaderApplicationErrorCode); got != runtimeservice.CodeQueryTooLong.String() {
		t.Fatalf("app error header = %q", got)
	}
}
