package workflow

import (
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
	// Get account information from context (set by WebAppAuthMiddleware)
	virtualAccountID := c.GetString("virtual_account_id")
	authenticatedAccountID := c.GetString("authenticated_account_id")
	migrationRequired := c.GetBool("migration_required")

	logger.Info("MigrateUser handler called",
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

	// Execute migration using UserMigrationService
	logger.Info("Starting user data migration",
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
