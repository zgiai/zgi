package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/util"
)

type capturingResumeService struct {
	runtimeservice.Service
	err            error
	conversationID uuid.UUID
	messageID      uuid.UUID
}

func (s *capturingResumeService) RunInterruptedTurnResumeStream(_ context.Context, _ runtimeservice.Scope, conversationID, messageID uuid.UUID, _ func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	s.conversationID, s.messageID = conversationID, messageID
	return nil, s.err
}

func resumeTestRequest(t *testing.T, capture *capturingResumeService, conversationID, messageID uuid.UUID) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", uuid.NewString())
		util.SetOrganizationID(c, uuid.NewString())
		c.Next()
	})
	NewHandler(capture, nil).RegisterRoutes(router.Group("/console/api"))
	path := fmt.Sprintf("/console/api/aichat/conversations/%s/messages/%s/resume", conversationID, messageID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
	return recorder.Body.String()
}

func TestResumeInterruptedTurnRunsTheRuntimeResume(t *testing.T) {
	conversationID, messageID := uuid.New(), uuid.New()
	capture := &capturingResumeService{}
	resumeTestRequest(t, capture, conversationID, messageID)
	if capture.conversationID != conversationID || capture.messageID != messageID {
		t.Fatalf("resumed %s/%s, want %s/%s", capture.conversationID, capture.messageID, conversationID, messageID)
	}
}

func TestResumeInterruptedTurnReportsATurnThatCannotResume(t *testing.T) {
	capture := &capturingResumeService{err: fmt.Errorf("%w: resumed once already", runtimeservice.ErrInterruptedTurnNotResumable)}
	body := resumeTestRequest(t, capture, uuid.New(), uuid.New())
	if !strings.Contains(body, interruptedTurnNotResumableCode) {
		t.Fatalf("body = %q, want the %s error event", body, interruptedTurnNotResumableCode)
	}
}
