package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestRespondPhoneAuthErrorUsesGenericInvalidCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/phone/password-login", nil)

	handler := &PhoneAuthHandler{}
	handler.respondPhoneAuthError(ctx, fmt.Errorf("login by phone password: %w", auth_service.ErrPhonePasswordMismatch))

	require.Equal(t, http.StatusBadRequest, recorder.Code)

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "201017", body.Code)
	require.Equal(t, response.ErrInvalidCredentials.Message, body.Message)
	require.Equal(t, response.ErrInvalidCredentials, response.ErrEmailPasswordMismatch)
}
