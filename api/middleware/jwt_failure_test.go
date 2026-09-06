package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestJWTRevocationFailureContracts(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	previousClient := redisutil.GetClient()
	redisutil.SetClient(client)
	jwtpkg.Init(&config.Config{JWT: config.JWTConfig{Secret: "middleware-revocation-test", JWTExpire: time.Hour}})
	t.Cleanup(func() { redisutil.SetClient(previousClient); jwtpkg.Init(nil); _ = client.Close() })
	token, err := jwtpkg.GenerateTokenFixed("account-a", "")
	require.NoError(t, err)
	_, err = jwtpkg.RevokeToken(context.Background(), token)
	require.NoError(t, err)
	router := gin.New()
	called := false
	router.GET("/protected", JWT(), func(c *gin.Context) { called = true; c.Status(200) })
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 401, request().Code)
	server.SetError("synthetic-private-diagnostic")
	w := request()
	require.Equal(t, 500, w.Code, "storage failure must not tell the browser to destroy credentials")
	require.NotContains(t, w.Body.String(), "synthetic-private-diagnostic")
	require.False(t, called, "neither revoked nor unchecked credentials may reach protected data")
}
