package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type fakeRouteAccountService struct {
	interfaces.AccountService
	allowed bool
}

func (f fakeRouteAccountService) IsOrganizationAdminOrOwner(context.Context, string, string) (bool, error) {
	return f.allowed, nil
}

func TestTenantModelWriteRoutesRequireOrganizationAdminOrOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	modelID := uuid.NewString()
	customID := uuid.NewString()
	cases := []struct {
		name      string
		method    string
		path      string
		body      string
		assertHit func(t *testing.T, svc *fakeModelService)
	}{
		{
			name:   "configure model price",
			method: http.MethodPost,
			path:   "/llm/models/config",
			body:   `{"model_id":"` + modelID + `","input_price_override":"0.1"}`,
			assertHit: func(t *testing.T, svc *fakeModelService) {
				require.Equal(t, 1, svc.configureCalls)
			},
		},
		{
			name:   "toggle provider models",
			method: http.MethodPost,
			path:   "/llm/models/provider/toggle",
			body:   `{"provider":"qwen","is_enabled":true}`,
			assertHit: func(t *testing.T, svc *fakeModelService) {
				require.Equal(t, 1, svc.toggleProviderCalls)
			},
		},
		{
			name:   "batch toggle models",
			method: http.MethodPost,
			path:   "/llm/models/batch/toggle",
			body:   `{"provider":"qwen","models":["qwen-plus"],"is_enabled":false}`,
			assertHit: func(t *testing.T, svc *fakeModelService) {
				require.Equal(t, 1, svc.batchToggleCalls)
			},
		},
		{
			name:   "create custom model",
			method: http.MethodPost,
			path:   "/llm/models/custom",
			body:   `{"provider":"qwen","model":"qwen-custom","model_name":"Qwen Custom","use_cases":["text-chat"]}`,
			assertHit: func(t *testing.T, svc *fakeModelService) {
				require.Equal(t, 1, svc.createCustomCalls)
			},
		},
		{
			name:   "update custom model",
			method: http.MethodPut,
			path:   "/llm/models/custom/" + customID,
			body:   `{"model_name":"Qwen Custom Updated"}`,
			assertHit: func(t *testing.T, svc *fakeModelService) {
				require.Equal(t, 1, svc.updateCustomCalls)
			},
		},
		{
			name:   "delete custom model",
			method: http.MethodDelete,
			path:   "/llm/models/custom/" + customID,
			assertHit: func(t *testing.T, svc *fakeModelService) {
				require.Equal(t, 1, svc.deleteCustomCalls)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/normal_member", func(t *testing.T) {
			svc := &fakeModelService{}
			router := tenantModelPermissionTestRouter(svc, false)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body)))

			require.Equal(t, http.StatusForbidden, w.Code)
			require.Zero(t, svc.configureCalls)
			require.Zero(t, svc.toggleProviderCalls)
			require.Zero(t, svc.batchToggleCalls)
			require.Zero(t, svc.createCustomCalls)
			require.Zero(t, svc.updateCustomCalls)
			require.Zero(t, svc.deleteCustomCalls)
		})

		t.Run(tc.name+"/admin", func(t *testing.T) {
			svc := &fakeModelService{}
			router := tenantModelPermissionTestRouter(svc, true)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body)))

			require.Equal(t, http.StatusOK, w.Code)
			tc.assertHit(t, svc)
		})
	}
}

func tenantModelPermissionTestRouter(svc *fakeModelService, allowed bool) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("organization_id", uuid.NewString())
		c.Set("account_id", "account-1")
		c.Set("account_service", fakeRouteAccountService{allowed: allowed})
		c.Next()
	})
	RegisterTenantModelRoutes(router.Group("/llm"), NewModelHandler(svc), nil)
	return router
}
