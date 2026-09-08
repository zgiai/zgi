package developeraccess

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestWriteErrorProjectsDeveloperQuotaMessageWithoutChangingLegacyContract(t *testing.T) {
	t.Parallel()

	definitions := append(appcatalog.DefaultDefinitions(), llmerrors.CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatalf("compose product catalog: %v", err)
	}
	projector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatalf("create error projector: %v", err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/developer-access/keys", nil)
	context.Request.Header.Set("Accept-Language", "zh-CN")

	NewHandler(nil, projector).writeError(context, ErrQuotaExceeded)

	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("HTTP status = %d, want %d", recorder.Code, http.StatusPaymentRequired)
	}
	if got := recorder.Header().Get(apptransport.HeaderApplicationErrorCode); got != llmerrors.AppCodeDeveloperQuotaExhausted.String() {
		t.Fatalf("application error code = %q, want %q", got, llmerrors.AppCodeDeveloperQuotaExhausted.String())
	}
	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Code != "501002" {
		t.Fatalf("legacy response code = %q, want 501002", body.Code)
	}
	wantMessage := "你的开发者 API 额度已用完或被进行中的请求占用。请检查用量或联系工作空间管理员；创建新密钥不会增加共享额度。"
	if body.Message != wantMessage {
		t.Fatalf("message = %q, want %q", body.Message, wantMessage)
	}
}
