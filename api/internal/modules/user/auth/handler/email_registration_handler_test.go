package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
)

func TestEmailRegistrationHandlerRejectsShortPassword(t *testing.T) {
	service := &emailRegistrationHandlerService{}
	handler := NewEmailRegistrationHandler(service)
	c, recorder := newAccountContextHandlerTestContext(
		http.MethodPost,
		"/register/finish",
		[]byte(`{"token":"verified-token","name":"User","password":"short","password_confirm":"short"}`),
	)

	handler.Finish(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.False(t, service.finishCalled)
}

type emailRegistrationHandlerService struct {
	finishCalled bool
}

func (f *emailRegistrationHandlerService) SendCode(context.Context, auth_service.EmailRegistrationSendRequest, string) (*auth_service.EmailRegistrationSendResponse, error) {
	return nil, nil
}

func (f *emailRegistrationHandlerService) VerifyCode(context.Context, auth_service.EmailRegistrationVerifyRequest) (*auth_service.EmailRegistrationVerifyResponse, error) {
	return nil, nil
}

func (f *emailRegistrationHandlerService) Finish(context.Context, auth_service.EmailRegistrationFinishRequest, string) (*shared_dto.LoginResponse, error) {
	f.finishCalled = true
	return nil, nil
}
