package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWorkflowRunEventsWebAppIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		authorization string
		virtualID     string
		want          bool
	}{
		{name: "virtual header", virtualID: "88e60b24-997c-4b42-a63f-9be088037d74", want: true},
		{name: "uuid bearer", authorization: "Bearer 88e60b24-997c-4b42-a63f-9be088037d74", want: true},
		{name: "jwt bearer", authorization: "Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature", want: false},
		{name: "missing", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/workflow-runs/run-1/events", nil)
			if tt.authorization != "" {
				ctx.Request.Header.Set("Authorization", tt.authorization)
			}
			if tt.virtualID != "" {
				ctx.Request.Header.Set("X-User-Account-Id", tt.virtualID)
			}
			if got := isWorkflowRunEventsWebAppIdentity(ctx); got != tt.want {
				t.Fatalf("isWorkflowRunEventsWebAppIdentity() = %v, want %v", got, tt.want)
			}
		})
	}
}
