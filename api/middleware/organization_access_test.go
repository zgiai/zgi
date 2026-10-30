package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/memberstatus"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/database"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type organizationAccessAccountService struct {
	interfaces.AccountService
	organizationID string
	accessErr      error
	checked        []string
}

func (s *organizationAccessAccountService) EnsureCurrentOrganizationID(context.Context, string) (string, error) {
	return s.organizationID, nil
}

func (s *organizationAccessAccountService) CheckOrganizationAccess(_ context.Context, accountID, organizationID string) error {
	s.checked = append(s.checked, accountID+"/"+organizationID)
	return s.accessErr
}

func newOrganizationAccessProjector(t *testing.T) *apptransport.Projector {
	t.Helper()
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal,
		append(appcatalog.DefaultDefinitions(), memberstatus.CatalogDefinitions()...)...)
	require.NoError(t, err)
	projector, err := apptransport.NewProjector(productCatalog)
	require.NoError(t, err)
	return projector
}

// newOrganizationAccessJWT installs the account-status globals the JWT
// middleware reads and returns a valid token for an active account.
func newOrganizationAccessJWT(t *testing.T) (string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec("CREATE TABLE accounts (id TEXT PRIMARY KEY, status TEXT, last_active_at DATETIME, updated_at DATETIME, deleted_at DATETIME)").Error)
	previousDB := database.GetDB()
	database.SetDB(db)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	previousClient := redisutil.GetClient()
	redisutil.SetClient(client)
	jwtpkg.Init(&config.Config{JWT: config.JWTConfig{Secret: "organization-access-test", JWTExpire: time.Hour}})
	accountID := uuid.NewString()
	require.NoError(t, db.Exec("INSERT INTO accounts (id, status) VALUES (?, ?)", accountID, authmodel.AccountStatusActive).Error)
	t.Cleanup(func() {
		// Account activity is touched asynchronously; let it finish before
		// the database it writes to is closed.
		require.Eventually(t, func() bool {
			var count int64
			return db.Table("accounts").Where("id = ? AND last_active_at IS NOT NULL", accountID).Count(&count).Error == nil && count == 1
		}, time.Second, time.Millisecond)
		jwtpkg.Init(nil)
		redisutil.SetClient(previousClient)
		database.SetDB(previousDB)
		_ = client.Close()
		_ = sqlDB.Close()
	})
	token, err := jwtpkg.GenerateTokenFixed(accountID, "")
	require.NoError(t, err)
	return accountID, token
}

func TestJWTWithOrganizationRejectsDisabledOrganizationMember(t *testing.T) {
	accountID, token := newOrganizationAccessJWT(t)
	projector := newOrganizationAccessProjector(t)

	for name, test := range map[string]struct {
		accessErr  error
		wantStatus int
		wantCode   response.ErrorCode
	}{
		"active member":              {nil, http.StatusOK, response.ErrorCode{}},
		"disabled member":            {memberstatus.NewMemberDisabledError("auth.check_organization_access"), http.StatusForbidden, response.ErrOrganizationMemberDisabled},
		"foreign workspace":          {interfaces.ErrCurrentWorkspaceOutsideOrganization, http.StatusBadRequest, response.ErrWorkspaceNotInOrganization},
		"check failure fails closed": {errors.New("database unavailable"), http.StatusInternalServerError, response.ErrSystemError},
	} {
		t.Run(name, func(t *testing.T) {
			accountService := &organizationAccessAccountService{organizationID: "org-1", accessErr: test.accessErr}
			called := false
			router := gin.New()
			router.Use(SetApplicationErrorProjector(projector))
			router.GET("/protected", JWTWithOrganizationAndService(accountService), func(c *gin.Context) {
				called = true
				c.Status(http.StatusOK)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Accept-Language", "en-US")
			router.ServeHTTP(recorder, request)

			require.Equal(t, test.wantStatus, recorder.Code)
			require.Equal(t, []string{accountID + "/org-1"}, accountService.checked)
			require.Equal(t, test.accessErr == nil, called)
			if test.accessErr == nil {
				return
			}
			var body response.Response
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.Equal(t, strconv.Itoa(test.wantCode.Code), body.Code)
			require.NotContains(t, body.Message, "database unavailable")
			if test.wantCode == response.ErrOrganizationMemberDisabled {
				require.Equal(t, memberstatus.AppCodeMemberDisabled.String(), recorder.Header().Get(apptransport.HeaderApplicationErrorCode))
				require.Equal(t, "Your access to this organization has been disabled. Contact an organization administrator.", body.Message)
			}
		})
	}
}

func TestJWTWithTenantRejectsDisabledOrganizationMember(t *testing.T) {
	_, token := newOrganizationAccessJWT(t)
	accountService := &organizationAccessAccountService{
		organizationID: "org-1",
		accessErr:      memberstatus.NewMemberDisabledError("auth.check_organization_access"),
	}
	called := false
	router := gin.New()
	router.GET("/tools", SetAccountService(accountService), JWTWithTenant(), func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/tools", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.False(t, called)
	var body response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, strconv.Itoa(response.ErrOrganizationMemberDisabled.Code), body.Code)
	require.Equal(t, response.ErrOrganizationMemberDisabled.Message, body.Message, "without a projector the legacy message is used")
}

func TestFailOrganizationMemberDisabledNormalizesUncatalogedErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Accept-Language", "zh-Hans")
	c.Set(applicationErrorProjectorKey, newOrganizationAccessProjector(t))

	FailOrganizationMemberDisabled(c, errors.New("internal detail"))

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusForbidden, recorder.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "你在该组织的访问权限已被停用，请联系组织管理员。", body.Message)
	require.True(t, apperror.IsCode(memberstatus.NewMemberDisabledError("x"), memberstatus.AppCodeMemberDisabled))
}
