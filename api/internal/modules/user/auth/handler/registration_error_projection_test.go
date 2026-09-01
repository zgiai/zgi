package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	authservice "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestRegistrationHandlersKeepLegacyContractForExpectedInviteRejections(t *testing.T) {
	projector := newRegistrationErrorProjector(t)
	tests := []struct {
		name        string
		err         error
		language    string
		wantCode    response.ErrorCode
		wantMessage string
	}{
		{
			name: "unavailable invitation",
			err: errors.Join(
				apperror.Wrap(
					fmt.Errorf("%w: %w", authservice.ErrRegistrationInvitationAcceptance, workspace_service.ErrOrganizationInviteUnavailable),
					authservice.AppCodeRegistrationInvitationUnavailable,
				),
				errors.New("cleanup database contained private diagnostics"),
			),
			language:    "zh-CN,zh;q=0.9",
			wantCode:    response.ErrTokenInvalid,
			wantMessage: "该邀请已失效或无法使用，请联系管理员获取新的邀请链接。",
		},
		{
			name: "organization member name conflict",
			err: apperror.Wrap(
				fmt.Errorf("%w: %w", authservice.ErrRegistrationInvitationAcceptance, workspace_service.ErrMemberNameExists),
				authservice.AppCodeRegistrationMemberNameConflict,
			),
			language:    "en-US",
			wantCode:    response.ErrInvalidParam,
			wantMessage: "This member name is already in use in the organization. Choose another name and try again.",
		},
	}

	for _, test := range tests {
		for _, registrationHandler := range []struct {
			name    string
			respond func(*gin.Context, error)
		}{
			{name: "email", respond: (&EmailRegistrationHandler{errorProjector: projector}).respondError},
			{name: "phone", respond: (&PhoneAuthHandler{errorProjector: projector}).respondPhoneAuthError},
		} {
			t.Run(test.name+"/"+registrationHandler.name, func(t *testing.T) {
				ctx, recorder := newRegistrationErrorContext(test.language)

				registrationHandler.respond(ctx, test.err)

				body := decodeRegistrationErrorResponse(t, recorder)
				require.Equal(t, strconv.Itoa(test.wantCode.Code), body.Code)
				require.Equal(t, test.wantMessage, body.Message)
				require.Equal(t, legacyRegistrationHTTPStatus(test.wantCode), recorder.Code)
				require.NotContains(t, recorder.Body.String(), "private diagnostics")
			})
		}
	}
}

func TestRegistrationHandlersDoNotExposeUnknownInviteAcceptanceErrors(t *testing.T) {
	databaseErr := errors.New("database host details and private diagnostics")
	err := fmt.Errorf("%w: %w", authservice.ErrRegistrationInvitationAcceptance, databaseErr)
	projector := newRegistrationErrorProjector(t)
	tests := []struct {
		name     string
		respond  func(*gin.Context, error)
		wantCode response.ErrorCode
	}{
		{name: "email", respond: (&EmailRegistrationHandler{errorProjector: projector}).respondError, wantCode: response.ErrSystemError},
		{name: "phone", respond: (&PhoneAuthHandler{errorProjector: projector}).respondPhoneAuthError, wantCode: response.ErrSystemError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := newRegistrationErrorContext("zh-CN")

			test.respond(ctx, err)

			body := decodeRegistrationErrorResponse(t, recorder)
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			require.Equal(t, strconv.Itoa(test.wantCode.Code), body.Code)
			require.Equal(t, test.wantCode.Message, body.Message)
			require.NotContains(t, recorder.Body.String(), databaseErr.Error())
		})
	}
}

func TestRegistrationHandlersDoNotOverlayCatalogFallbackOnLegacyResponse(t *testing.T) {
	productCatalog, err := appcatalog.NewDefault()
	require.NoError(t, err)
	projector, err := apptransport.NewProjector(productCatalog)
	require.NoError(t, err)
	registrationErr := apperror.Wrap(
		fmt.Errorf("%w: %w", authservice.ErrRegistrationInvitationAcceptance, workspace_service.ErrOrganizationInviteUnavailable),
		authservice.AppCodeRegistrationInvitationUnavailable,
	)

	for _, registrationHandler := range []struct {
		name    string
		respond func(*gin.Context, error)
	}{
		{name: "email", respond: (&EmailRegistrationHandler{errorProjector: projector}).respondError},
		{name: "phone", respond: (&PhoneAuthHandler{errorProjector: projector}).respondPhoneAuthError},
	} {
		t.Run(registrationHandler.name, func(t *testing.T) {
			ctx, recorder := newRegistrationErrorContext("zh-CN")

			registrationHandler.respond(ctx, registrationErr)

			body := decodeRegistrationErrorResponse(t, recorder)
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			require.Equal(t, strconv.Itoa(response.ErrSystemError.Code), body.Code)
			require.Equal(t, response.ErrSystemError.Message, body.Message)
			require.NotContains(t, body.Message, "服务暂时出现问题")
		})
	}
}

func newRegistrationErrorProjector(t *testing.T) *apptransport.Projector {
	t.Helper()
	definitions := append(appcatalog.DefaultDefinitions(), authservice.CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	require.NoError(t, err)
	projector, err := apptransport.NewProjector(productCatalog)
	require.NoError(t, err)
	return projector
}

func newRegistrationErrorContext(language string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/register/finish", nil)
	ctx.Request.Header.Set("Accept-Language", language)
	return ctx, recorder
}

func decodeRegistrationErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	Code    string `json:"code"`
	Message string `json:"message"`
} {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

func legacyRegistrationHTTPStatus(code response.ErrorCode) int {
	switch code.Code {
	case response.ErrTokenInvalid.Code:
		return http.StatusUnauthorized
	default:
		return http.StatusBadRequest
	}
}
