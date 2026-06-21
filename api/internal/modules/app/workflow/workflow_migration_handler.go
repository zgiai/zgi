package workflow

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// MigrateUser handles POST /api/v1/workflows/migrate-user
// @Summary Migrate virtual user data to authenticated account
// @Description Migrates all virtual user data (conversations, messages, workflow runs) to authenticated account
// @Tags Workflow
// @Accept json
// @Produce json
// @Success 200 {object} dto.MigrateUserResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /workflows/migrate-user [post]
func (h *WorkflowHandler) MigrateUser(c *gin.Context) {
	h.migrateUser(c, "")
}

// MigrateUserForWebApp handles POST /api/v1/workflows/:web_app_id/migrate-user.
// It preserves the migration behavior of the legacy endpoint, but first checks
// that the authenticated account can use the target webapp.
func (h *WorkflowHandler) MigrateUserForWebApp(c *gin.Context) {
	webAppID := strings.TrimSpace(c.Param("web_app_id"))
	if webAppID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	h.migrateUser(c, webAppID)
}

func (h *WorkflowHandler) migrateUser(c *gin.Context, webAppID string) {
	// Get account information from context (set by WebAppAuthMiddleware)
	virtualAccountID := c.GetString("virtual_account_id")
	authenticatedAccountID := c.GetString("authenticated_account_id")
	migrationRequired := c.GetBool("migration_required")

	logger.Info("MigrateUser handler called",
		"web_app_id", webAppID,
		"virtual_account_id", virtualAccountID,
		"authenticated_account_id", authenticatedAccountID,
		"migration_required", migrationRequired)

	// Validate migration request - ensure both headers were provided
	if !migrationRequired {
		logger.WarnContext(c.Request.Context(), "migration request missing required headers")
		response.Fail(c, response.ErrMigrationHeadersRequired)
		return
	}

	// Additional validation - ensure both account IDs are present
	if virtualAccountID == "" || authenticatedAccountID == "" {
		logger.WarnContext(c.Request.Context(), "invalid account ids for migration",
			"virtual_account_id", virtualAccountID,
			"authenticated_account_id", authenticatedAccountID,
		)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if webAppID != "" {
		authorizer := h.getWebAppMigrationAuthorizer()
		if authorizer == nil {
			logger.CriticalContext(c.Request.Context(), "webapp migration authorizer is unavailable", "web_app_id", webAppID, errors.New("authorizer unavailable"))
			response.Fail(c, response.ErrSystemError)
			return
		}
		if err := authorizer.AuthorizeWebAppMigration(c.Request.Context(), webAppID, authenticatedAccountID); err != nil {
			logger.WarnContext(c.Request.Context(), "webapp migration authorization failed",
				"web_app_id", webAppID,
				"authenticated_account_id", authenticatedAccountID,
				err,
			)
			failWebAppMigrationAuthorization(c, err)
			return
		}
	}

	// Execute migration using UserMigrationService
	logger.Info("Starting user data migration",
		"web_app_id", webAppID,
		"virtual_account_id", virtualAccountID,
		"authenticated_account_id", authenticatedAccountID)

	result, err := h.userMigrationService.MigrateUserData(
		c.Request.Context(),
		virtualAccountID,
		authenticatedAccountID,
	)

	if err != nil {
		logger.CriticalContext(c.Request.Context(), "user migration failed",
			"virtual_account_id", virtualAccountID,
			"authenticated_account_id", authenticatedAccountID,
			err,
		)

		// Check for specific error types and return appropriate error codes
		if err.Error() == "cannot migrate user to the same account" {
			response.Fail(c, response.ErrSameAccountMigration)
			return
		}

		// Generic migration failure
		response.Fail(c, response.ErrUserMigrationFailed)
		return
	}

	// Log successful migration with statistics
	logger.Info("User migration completed successfully",
		"virtual_account_id", virtualAccountID,
		"authenticated_account_id", authenticatedAccountID,
		"conversations_migrated", result.ConversationsMigrated,
		"messages_migrated", result.MessagesMigrated,
		"workflow_runs_migrated", result.WorkflowRunLogsMigrated,
		"node_logs_migrated", result.NodeRuntimeLogsMigrated)

	// Return migration result
	response.Success(c, result)
}

func failWebAppMigrationAuthorization(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errWebAppMigrationInvalidRequest):
		response.Fail(c, response.ErrInvalidParam)
	case errors.Is(err, errWebAppMigrationNotFound):
		response.Fail(c, response.ErrAppNotFound)
	case errors.Is(err, errWebAppMigrationOffline):
		response.Fail(c, response.ErrWebAppOffline)
	case errors.Is(err, errWebAppMigrationAccessDenied):
		response.Fail(c, response.ErrPermissionDenied)
	default:
		response.Fail(c, response.ErrSystemError)
	}
}
