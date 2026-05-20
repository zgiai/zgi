package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/config"
	shared_dto "github.com/zgiai/ginext/internal/dto"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	auth_service "github.com/zgiai/ginext/internal/modules/user/auth/service"
)

type fakeFeatureService struct {
	enabled bool
}

func (f fakeFeatureService) GetSystemFeatures(ctx context.Context) (interface{}, error) {
	return nil, nil
}

func (f fakeFeatureService) IsPublicDeployment() bool {
	return true
}

func (f fakeFeatureService) IsFeatureEnabled(featureName string) bool {
	return f.enabled && featureName == "social_oauth_login"
}

type fakeSSOService struct {
	issueStateFn     func(ctx context.Context) (string, error)
	consumeStateFn   func(ctx context.Context, state string) error
	resolveAccountFn func(ctx context.Context, identity *shared_dto.SSOIdentity) (*auth_model.Account, error)
	issueTicketFn    func(ctx context.Context, account *auth_model.Account, sso *shared_dto.SSOProviderToken) (string, error)
	consumeTicketFn  func(ctx context.Context, ticket, ipAddress string) (*shared_dto.LoginResponse, error)
}

func (f fakeSSOService) IssueSSOState(ctx context.Context) (string, error) {
	return f.issueStateFn(ctx)
}

func (f fakeSSOService) ConsumeSSOState(ctx context.Context, state string) error {
	return f.consumeStateFn(ctx, state)
}

func (f fakeSSOService) ResolveOrCreateSSOAccount(ctx context.Context, identity *shared_dto.SSOIdentity) (*auth_model.Account, error) {
	return f.resolveAccountFn(ctx, identity)
}

func (f fakeSSOService) IssueSSOLoginTicket(ctx context.Context, account *auth_model.Account, sso *shared_dto.SSOProviderToken) (string, error) {
	return f.issueTicketFn(ctx, account, sso)
}

func (f fakeSSOService) ConsumeSSOLoginTicket(ctx context.Context, ticket, ipAddress string) (*shared_dto.LoginResponse, error) {
	return f.consumeTicketFn(ctx, ticket, ipAddress)
}

type fakeCasdoorClient struct {
	authURL        string
	exchangeResult *shared_dto.SSOExchangeResult
	exchangeErr    error
}

func (f fakeCasdoorClient) AuthorizationURL(ctx context.Context, state string) (string, error) {
	return f.authURL + "?state=" + state, nil
}

func (f fakeCasdoorClient) ExchangeCode(ctx context.Context, code string) (*shared_dto.SSOExchangeResult, error) {
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	return f.exchangeResult, nil
}

func TestStartCasdoorSSORedirectsToProvider(t *testing.T) {
	h := &AuthHandler{
		featureService: fakeFeatureService{enabled: true},
		ssoService: fakeSSOService{
			issueStateFn: func(ctx context.Context) (string, error) {
				return "state-1", nil
			},
		},
		casdoorClient: fakeCasdoorClient{authURL: "https://door.example.com/login/oauth/authorize"},
	}

	c, recorder := newAuthHandlerTestContext(http.MethodGet, "/sso/casdoor/start", nil)
	h.StartCasdoorSSO(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "https://door.example.com/login/oauth/authorize?state=state-1", recorder.Header().Get("Location"))
}

func TestCasdoorCallbackRedirectsWithTicket(t *testing.T) {
	setSSOHandlerTestConfig(t, "https://app.example.com/sso/callback")

	h := &AuthHandler{
		featureService: fakeFeatureService{enabled: true},
		ssoService: fakeSSOService{
			consumeStateFn: func(ctx context.Context, state string) error {
				require.Equal(t, "state-1", state)
				return nil
			},
			resolveAccountFn: func(ctx context.Context, identity *shared_dto.SSOIdentity) (*auth_model.Account, error) {
				require.Equal(t, "sub-1", identity.Subject)
				return &auth_model.Account{ID: "acc-1", Name: "Casdoor User"}, nil
			},
			issueTicketFn: func(ctx context.Context, account *auth_model.Account, sso *shared_dto.SSOProviderToken) (string, error) {
				require.Equal(t, "acc-1", account.ID)
				require.NotNil(t, sso)
				require.Equal(t, shared_dto.SSOProviderCasdoor, sso.Provider)
				require.Equal(t, "id-token-1", sso.IDToken)
				return "ticket-1", nil
			},
		},
		casdoorClient: fakeCasdoorClient{
			authURL: "https://door.example.com/login/oauth/authorize",
			exchangeResult: &shared_dto.SSOExchangeResult{
				Identity: &shared_dto.SSOIdentity{
					Subject: "sub-1",
					Email:   "user@example.com",
				},
				Token: &shared_dto.SSOProviderToken{
					Provider: shared_dto.SSOProviderCasdoor,
					IDToken:  "id-token-1",
				},
			},
		},
	}

	c, recorder := newAuthHandlerTestContext(http.MethodGet, "/sso/casdoor/callback?code=code-1&state=state-1", nil)
	h.HandleCasdoorCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "https://app.example.com/sso/callback?ticket=ticket-1", recorder.Header().Get("Location"))
}

func TestCasdoorCallbackRedirectsWithExchangeReason(t *testing.T) {
	setSSOHandlerTestConfig(t, "https://app.example.com/sso/callback")

	h := &AuthHandler{
		featureService: fakeFeatureService{enabled: true},
		ssoService: fakeSSOService{
			consumeStateFn: func(ctx context.Context, state string) error {
				return nil
			},
		},
		casdoorClient: fakeCasdoorClient{
			exchangeErr: auth_service.ErrOIDCIdentityContactAbsent,
		},
	}

	c, recorder := newAuthHandlerTestContext(http.MethodGet, "/sso/casdoor/callback?code=code-1&state=state-1", nil)
	h.HandleCasdoorCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)

	redirectURL, err := url.Parse(recorder.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "exchange_failed", redirectURL.Query().Get("error"))
	require.Equal(t, "identity_contact_required", redirectURL.Query().Get("reason"))
}

func setSSOHandlerTestConfig(t *testing.T, callbackURL string) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Auth: config.AuthConfig{
			SSO: config.SSOConfig{FrontendCallbackURL: callbackURL},
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}

func TestConsumeSSOLoginTicketReturnsLoginPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &AuthHandler{
		ssoService: fakeSSOService{
			consumeTicketFn: func(ctx context.Context, ticket, ipAddress string) (*shared_dto.LoginResponse, error) {
				require.Equal(t, "ticket-1", ticket)
				require.NotEmpty(t, ipAddress)
				return &shared_dto.LoginResponse{
					AccessToken:  "access-1",
					RefreshToken: "refresh-1",
					Account: &shared_dto.AccountProfileResponse{
						ID:    "acc-1",
						Name:  "Casdoor User",
						Email: "user@example.com",
					},
					SSO: &shared_dto.SSOProviderToken{
						Provider: shared_dto.SSOProviderCasdoor,
						IDToken:  "id-token-1",
					},
				}, nil
			},
		},
	}

	body, err := json.Marshal(map[string]string{"ticket": "ticket-1"})
	require.NoError(t, err)

	c, recorder := newAuthHandlerTestContext(http.MethodPost, "/sso/casdoor/consume-ticket", body)
	h.ConsumeSSOLoginTicket(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "access-1")
	require.Contains(t, recorder.Body.String(), "refresh-1")
	require.Contains(t, recorder.Body.String(), "id-token-1")
	require.Contains(t, recorder.Body.String(), "casdoor")
}

func newAuthHandlerTestContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(recorder)
	c.Request = request
	c.Request.RemoteAddr = "127.0.0.1:12345"

	return c, recorder
}

var _ interfaces.FeatureService = fakeFeatureService{}
