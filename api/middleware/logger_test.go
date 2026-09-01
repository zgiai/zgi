package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLogsUseRouteTemplateAndPreserveUnknownPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	previous := logger.L()
	logger.SetLogger(zap.New(core))
	t.Cleanup(func() { logger.SetLogger(previous) })

	router := gin.New()
	router.Use(Logger(), AuditLogger())
	router.POST("/console/api/public/invites/:token/accept", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	const secretToken = "secret-invitation-token"
	known := httptest.NewRequest(http.MethodPost, "/console/api/public/invites/"+secretToken+"/accept", nil)
	router.ServeHTTP(httptest.NewRecorder(), known)
	methodMismatch := httptest.NewRequest(http.MethodPut, "/console/api/public/invites/"+secretToken+"/accept", nil)
	router.ServeHTTP(httptest.NewRecorder(), methodMismatch)
	malformed := httptest.NewRequest(http.MethodPost, "/console/api/public/invites/"+secretToken+"/unexpected", nil)
	router.ServeHTTP(httptest.NewRecorder(), malformed)

	const unknownPath = "/console/api/unmatched/diagnostic-path"
	unknown := httptest.NewRequest(http.MethodPost, unknownPath, nil)
	router.ServeHTTP(httptest.NewRecorder(), unknown)

	entries := observed.All()
	require.Len(t, entries, 8)
	knownLogs := 0
	methodMismatchLogs := 0
	malformedLogs := 0
	unknownLogs := 0
	for _, entry := range entries {
		fields := entry.ContextMap()
		path, _ := fields["path"].(string)
		require.NotContains(t, path, secretToken)
		switch path {
		case "/console/api/public/invites/:token/accept":
			knownLogs++
			require.Equal(t, "/console/api/public/invites/:token/accept", fields["route"])
		case "/console/api/public/invites/[REDACTED]/accept":
			methodMismatchLogs++
			_, hasRoute := fields["route"]
			require.False(t, hasRoute)
		case "/console/api/public/invites/[REDACTED]/unexpected":
			malformedLogs++
			_, hasRoute := fields["route"]
			require.False(t, hasRoute)
		case unknownPath:
			unknownLogs++
			_, hasRoute := fields["route"]
			require.False(t, hasRoute)
		default:
			t.Fatalf("unexpected logged path %q", path)
		}
	}
	require.Equal(t, 2, knownLogs, "access and audit logs must both use the route template")
	require.Equal(t, 2, methodMismatchLogs, "method-mismatched invite paths must redact the token")
	require.Equal(t, 2, malformedLogs, "unmatched invite paths must redact the token")
	require.Equal(t, 2, unknownLogs, "access and audit logs must retain unmatched paths")
}
