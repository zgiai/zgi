package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type logoutHandlerService struct {
	interfaces.AccountService
	access, refresh string
	called          bool
	err             error
}

func (s *logoutHandlerService) Logout(_ context.Context, access, refresh string) error {
	s.called, s.access, s.refresh = true, access, refresh
	return s.err
}

func TestConsoleLogoutHandler(t *testing.T) {
	for _, tc := range []struct {
		name, header, body, refresh string
		called, fail                bool
	}{
		{name: "current pair", header: "Bearer access-fixture", body: `{"refresh_token":"refresh-fixture"}`, refresh: "refresh-fixture", called: true},
		{name: "legacy bodyless", header: "Bearer access-fixture", called: true},
		{name: "case insensitive scheme", header: "bearer access-fixture", called: true},
		{name: "missing header"},
		{name: "wrong scheme", header: "Basic access-fixture"},
		{name: "missing token", header: "Bearer"},
		{name: "malformed body", header: "Bearer access-fixture", body: "{"},
		{name: "revocation unavailable", header: "Bearer access-fixture", called: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &logoutHandlerService{}
			if tc.fail {
				svc.err = errors.New("private-fixture-detail")
			}
			h := &AuthHandler{accountService: svc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/console/api/logout", strings.NewReader(tc.body))
			c.Request.Header.Set("Authorization", tc.header)
			h.Logout(c)
			require.Equal(t, tc.called, svc.called)
			if tc.called {
				require.Equal(t, "access-fixture", svc.access)
				require.Equal(t, tc.refresh, svc.refresh)
			}
			if tc.called && !tc.fail {
				require.Equal(t, 200, w.Code)
			} else {
				require.NotEqual(t, 200, w.Code)
			}
			require.NotContains(t, w.Body.String(), "private-fixture-detail")
		})
	}
}
