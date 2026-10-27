package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

func TestAgentLegacyFailureStatusMapsPermissionAndNotFound(t *testing.T) {
	tests := []struct {
		name string
		body gin.H
		err  error
		want int
	}{
		{name: "permission body", body: agentPermissionDeniedBody(), want: http.StatusForbidden},
		{name: "not found body", body: agentNotFoundBody(), want: http.StatusNotFound},
		{name: "wrapped permission sentinel", body: gin.H{"code": "399001"}, err: fmt.Errorf("lock: %w", errAgentPermissionDenied), want: http.StatusForbidden},
		{name: "runtime permission", body: gin.H{"code": "399001"}, err: runtimeservice.ErrPermissionDenied, want: http.StatusForbidden},
		{name: "plain permission message", body: gin.H{"code": "399001"}, err: errors.New("permission denied"), want: http.StatusForbidden},
		{name: "agent not found message", body: gin.H{"code": "399001"}, err: errors.New("agent not found"), want: http.StatusNotFound},
		{name: "other failure keeps 200", body: gin.H{"code": "399001"}, err: errors.New("invalid model"), want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, agentLegacyFailureStatus(tt.body, tt.err))
		})
	}
}

func TestWriteAgentLegacyFailureKeepsBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeAgentLegacyFailure(c, agentPermissionDeniedBody(), errAgentPermissionDenied)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "403001", body["code"])
	require.Equal(t, "Permission denied", body["message"])
}
