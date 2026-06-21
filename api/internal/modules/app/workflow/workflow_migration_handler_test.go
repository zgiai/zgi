package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestMigrateUserRequiresMigrationContextBeforeServiceCall(t *testing.T) {
	service := &workflowMigrationService{}
	handler := &WorkflowHandler{userMigrationService: service}
	ctx, recorder := newWorkflowMigrationContext("", "", false)

	handler.MigrateUser(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrMigrationHeadersRequired)
	if service.called {
		t.Fatalf("migration service should not be called without migration context")
	}
}

func TestMigrateUserUsesMiddlewareAccountContext(t *testing.T) {
	service := &workflowMigrationService{
		result: &MigrationResult{
			ConversationsMigrated:  2,
			MessagesMigrated:       3,
			AuthenticatedAccountID: "auth-account",
		},
	}
	handler := &WorkflowHandler{userMigrationService: service}
	ctx, recorder := newWorkflowMigrationContext("virtual-account", "auth-account", true)

	handler.MigrateUser(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !service.called {
		t.Fatalf("migration service was not called")
	}
	if service.virtualAccountID != "virtual-account" {
		t.Fatalf("virtual account id = %q, want %q", service.virtualAccountID, "virtual-account")
	}
	if service.authenticatedAccountID != "auth-account" {
		t.Fatalf("authenticated account id = %q, want %q", service.authenticatedAccountID, "auth-account")
	}

	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if body.Code != "0" {
		t.Fatalf("code = %q, want 0", body.Code)
	}
}

func TestMigrateUserMapsSameAccountError(t *testing.T) {
	service := &workflowMigrationService{err: errors.New("cannot migrate user to the same account")}
	handler := &WorkflowHandler{userMigrationService: service}
	ctx, recorder := newWorkflowMigrationContext("same-account", "same-account", true)

	handler.MigrateUser(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrSameAccountMigration)
}

func newWorkflowMigrationContext(virtualAccountID, authenticatedAccountID string, migrationRequired bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/workflows/migrate-user", nil)
	ctx.Set("virtual_account_id", virtualAccountID)
	ctx.Set("authenticated_account_id", authenticatedAccountID)
	ctx.Set("migration_required", migrationRequired)
	return ctx, recorder
}

type workflowMigrationService struct {
	result                 *MigrationResult
	err                    error
	called                 bool
	virtualAccountID       string
	authenticatedAccountID string
}

func (s *workflowMigrationService) MigrateUserData(_ context.Context, virtualAccountID, authenticatedAccountID string) (*MigrationResult, error) {
	s.called = true
	s.virtualAccountID = virtualAccountID
	s.authenticatedAccountID = authenticatedAccountID
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &MigrationResult{AuthenticatedAccountID: authenticatedAccountID}, nil
}

func (s *workflowMigrationService) ValidateMigrationRequest(_, _ string) error {
	return nil
}

func (s *workflowMigrationService) GetMigrationStatistics(context.Context, string) (*MigrationStatistics, error) {
	return &MigrationStatistics{}, nil
}
