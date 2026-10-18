package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/user/auth/statuscache"
	"github.com/zgiai/zgi/api/pkg/database"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWebAppAuthRechecksAccountStatus(t *testing.T) {
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
	jwtpkg.Init(&config.Config{JWT: config.JWTConfig{Secret: "webapp-account-status-test", JWTExpire: time.Hour}})
	t.Cleanup(func() {
		jwtpkg.Init(nil)
		redisutil.SetClient(previousClient)
		database.SetDB(previousDB)
		_ = client.Close()
		_ = sqlDB.Close()
	})
	accountID := uuid.NewString()
	require.NoError(t, db.Exec("INSERT INTO accounts (id, status) VALUES (?, ?)", accountID, authmodel.AccountStatusActive).Error)
	token, err := jwtpkg.GenerateTokenFixed(accountID, "")
	require.NoError(t, err)
	called := false
	router := gin.New()
	router.GET("/protected", WebAppAuthMiddleware(), func(c *gin.Context) {
		called = true
		require.Equal(t, accountID, c.GetString("account_id"))
		require.True(t, c.GetBool("is_authenticated"))
		c.Status(http.StatusOK)
	})
	request := func(virtualID string) *httptest.ResponseRecorder {
		called = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if virtualID != "" {
			r.Header.Set("X-User-Account-Id", virtualID)
		}
		router.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, http.StatusOK, request("").Code)
	require.True(t, called)
	// Account activity is updated asynchronously; finish it before restoring globals.
	require.Eventually(t, func() bool {
		var count int64
		return db.Table("accounts").Where("id = ? AND last_active_at IS NOT NULL", accountID).Count(&count).Error == nil && count == 1
	}, time.Second, time.Millisecond)
	for _, status := range []authmodel.AccountStatus{authmodel.AccountStatusBanned, authmodel.AccountStatusFrozen, authmodel.AccountStatusClosed} {
		t.Run(string(status), func(t *testing.T) {
			require.NoError(t, db.Table("accounts").Where("id = ?", accountID).Update("status", status).Error)
			statuscache.InvalidateAccountStatus(t.Context(), accountID)
			for _, virtualID := range []string{"", uuid.NewString()} {
				w := request(virtualID)
				require.NotEqual(t, http.StatusOK, w.Code, "disabled JWT must not use anonymous migration to bypass account status")
				require.False(t, called)
			}
		})
	}
	require.NoError(t, db.Table("accounts").Where("id = ?", accountID).Update("status", authmodel.AccountStatusActive).Error)
	statuscache.InvalidateAccountStatus(t.Context(), accountID)
	require.Equal(t, http.StatusOK, request("").Code)
	require.True(t, called)
	require.NoError(t, db.Exec("DELETE FROM accounts WHERE id = ?", accountID).Error)
	statuscache.InvalidateAccountStatus(t.Context(), accountID)
	require.NotEqual(t, http.StatusOK, request("").Code)
	require.False(t, called, "a deleted account must fail closed even with a valid JWT")
}

func TestWebAppAuthKeepsAnonymousIdentities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, header := range []string{"Authorization", "X-User-Account-Id"} {
		t.Run(header, func(t *testing.T) {
			id := uuid.NewString()
			called := false
			router := gin.New()
			router.GET("/anonymous", WebAppAuthMiddleware(), func(c *gin.Context) {
				called = true
				require.Equal(t, id, c.GetString("account_id"))
				require.False(t, c.GetBool("is_authenticated"))
				c.Status(http.StatusOK)
			})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/anonymous", nil)
			value := id
			if header == "Authorization" {
				value = "Bearer " + id
			}
			r.Header.Set(header, value)
			router.ServeHTTP(w, r)
			require.Equal(t, http.StatusOK, w.Code)
			require.True(t, called)
		})
	}
}
