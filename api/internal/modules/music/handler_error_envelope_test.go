package music

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// TestMusicValidationErrorCarriesTheCatalogEnvelope pins the validation and
// auth exits: they used to answer with a bare {code, message} product word and
// no headers, so a client could not classify or correlate them.
func TestMusicValidationErrorCarriesTheCatalogEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	definitions = append(definitions, llmerrors.CatalogDefinitions()...)
	definitions = append(definitions, llmerrors.BillingCatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatalf("compose catalog: %v", err)
	}
	projector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatalf("create projector: %v", err)
	}
	handler := &Handler{errorProjector: projector}

	for _, testCase := range []struct {
		name      string
		write     func(c *gin.Context)
		wantCode  string
		wantState int
	}{
		{
			name: "invalid task id",
			write: func(c *gin.Context) {
				handler.writeMusicCatalogError(c, AppCodeRequestInvalid, "music.task.get", messageInvalidTaskID)
			},
			wantCode:  "music.request.invalid",
			wantState: http.StatusBadRequest,
		},
		{
			name: "invalid auth context",
			write: func(c *gin.Context) {
				handler.writeMusicCatalogError(c, llmerrors.AppCodeAPIKeyInvalid, "music.task.scope", messageInvalidAuthContext)
			},
			wantCode:  "llm.api_key.invalid",
			wantState: http.StatusUnauthorized,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/music/tasks/x", nil)
			c.Set("request_id", "req-music-test")

			testCase.write(c)

			if recorder.Code != testCase.wantState {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, testCase.wantState, recorder.Body.String())
			}
			if got := recorder.Header().Get(apptransport.HeaderApplicationErrorCode); got != testCase.wantCode {
				t.Fatalf("header = %q, want %q", got, testCase.wantCode)
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != testCase.wantCode {
				t.Fatalf("code = %v, want %q", body["code"], testCase.wantCode)
			}
			for _, field := range []string{"message", "retryable", "error_id", "request_id"} {
				if _, ok := body[field]; !ok {
					t.Fatalf("body is missing %q: %s", field, recorder.Body.String())
				}
			}
			if body["request_id"] != "req-music-test" {
				t.Fatalf("request_id = %v", body["request_id"])
			}
		})
	}

}
