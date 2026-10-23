package v1

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// RegisterRuntimeWorkspaceManagementRoutes registers the internal console
// management API. These routes are JWT protected and never share the external
// Runtime Workspace credential middleware.
func RegisterRuntimeWorkspaceManagementRoutes(router *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, workspaceService interfaces.WorkspaceManagementService) {
	if !runtimeworkspace.RuntimeEnabled() {
		return
	}
	if router == nil || db == nil || accountService == nil || workspaceService == nil {
		panic("runtime workspace management routes require db, account service, and workspace service")
	}
	replayProtector, err := runtimeworkspace.NewCredentialReplayProtector(config.Current().Encryption.APIKeyEncryptionKey)
	if err != nil {
		panic("runtime workspace credential replay protection is unavailable")
	}
	service := runtimeworkspace.NewService(db, runtimeworkspace.WithCredentialReplayProtector(replayProtector))
	// End-user erasure and export reach the DigitalHuman data of end users.
	runtimeworkspace.UseEndUserDataOwner(digitalhuman.NewEndUserData())
	group := router.Group("/runtime-workspaces")
	group.Use(middleware.JWTWithOrganizationAndService(accountService))
	group.GET("", runtimeWorkspaceManagementScope(), runtimeWorkspaceManagerRequiredWithDB(db, workspaceService), func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, "")
		if !ok {
			return
		}
		var workspace runtimeworkspace.RuntimeWorkspace
		if err := db.WithContext(c.Request.Context()).Where("workspace_id = ? OR runtime_id = ?", workspaceID, workspaceID).First(&workspace).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// A missing runtime workspace is a valid setup state for the
				// console. Returning a successful null payload lets the UI render
				// the enablement flow without treating it as a transport failure.
				response.Success(c, nil)
				return
			}
			managementFail(c, err)
			return
		}
		response.Success(c, workspace)
	})
	group.POST("", runtimeWorkspaceManagementScope(), runtimeWorkspaceManagerRequiredWithDB(db, workspaceService), func(c *gin.Context) {
		workspaceID, accountID, ok := managementScope(c, "")
		if !ok {
			return
		}
		workspace, err := service.CreateWorkspace(c.Request.Context(), workspaceID, accountID)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, workspace)
	})
	scoped := group.Group("/:workspace_id")
	scoped.Use(runtimeWorkspaceManagementScope(), runtimeWorkspaceManagerRequiredWithDB(db, workspaceService))
	scoped.GET("", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		var workspace runtimeworkspace.RuntimeWorkspace
		if err := db.WithContext(c.Request.Context()).Where("workspace_id = ? OR runtime_id = ?", workspaceID, workspaceID).First(&workspace).Error; err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, workspace)
	})
	scoped.PATCH("", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		var req struct {
			Status     *string         `json:"status,omitempty"`
			PolicyJSON json.RawMessage `json:"policy_json,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		updated, err := service.UpdateWorkspace(c.Request.Context(), workspaceID, req.Status, req.PolicyJSON)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, updated)
	})
	scoped.POST("/credentials", func(c *gin.Context) {
		workspaceID, accountID, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		var req struct {
			Name      string     `json:"name"`
			ExpiresAt *time.Time `json:"expires_at,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		created, err := service.CreateCredential(c.Request.Context(), workspaceID, accountID, req.Name, req.ExpiresAt, c.GetHeader("Idempotency-Key"))
		if err != nil {
			managementFail(c, err)
			return
		}
		if created.Replayed {
			c.Header("Idempotent-Replayed", "true")
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, response.Response{Code: "0", Message: "success", Data: created})
	})
	scoped.GET("/credentials", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		runtimeID, err := service.ResolveRuntimeID(c.Request.Context(), workspaceID)
		if err != nil {
			managementFail(c, err)
			return
		}
		var credentials []runtimeworkspace.Credential
		if err := db.WithContext(c.Request.Context()).Where("runtime_workspace_id = ?", runtimeID).Order("created_at DESC").Find(&credentials).Error; err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, gin.H{"items": credentials})
	})
	scoped.POST("/credentials/:credential_id/revoke", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		credentialID, err := uuid.Parse(c.Param("credential_id"))
		if err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		if err := service.RevokeCredential(c.Request.Context(), workspaceID, credentialID); err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, gin.H{"result": "success"})
	})
	scoped.GET("/agent-candidates", func(c *gin.Context) {
		workspaceID, accountID, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		scope := strings.TrimSpace(c.DefaultQuery("scope", "current"))
		var sourceWorkspaceID *uuid.UUID
		if rawSourceWorkspaceID := strings.TrimSpace(c.Query("source_workspace_id")); rawSourceWorkspaceID != "" {
			parsed, err := uuid.Parse(rawSourceWorkspaceID)
			if err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}
			sourceWorkspaceID = &parsed
		}
		candidates, err := service.ListAgentCandidates(c.Request.Context(), workspaceID, accountID, scope, sourceWorkspaceID)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, candidates)
	})
	scoped.GET("/agents", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		apps, err := service.ListBindings(c.Request.Context(), workspaceID)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, apps)
	})
	scoped.GET("/audit-events", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		runtimeID, err := service.ResolveRuntimeID(c.Request.Context(), workspaceID)
		if err != nil {
			managementFail(c, err)
			return
		}
		page := managementPositiveQueryInt(c.Query("page"), 1, 1_000_000)
		limit := managementPositiveQueryInt(c.Query("limit"), 50, 200)
		query := db.WithContext(c.Request.Context()).Where("runtime_workspace_id = ?", runtimeID)
		if result := strings.TrimSpace(c.Query("result")); result == runtimeworkspace.AccessEventResultSuccess || result == runtimeworkspace.AccessEventResultFailure {
			query = query.Where("result = ?", result)
		}
		var total int64
		if err := query.Model(&runtimeworkspace.AccessEvent{}).Count(&total).Error; err != nil {
			managementFail(c, err)
			return
		}
		var events []runtimeworkspace.AccessEvent
		if err := query.Order("created_at DESC").Limit(limit).Offset((page - 1) * limit).Find(&events).Error; err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, gin.H{"items": events, "page": page, "limit": limit, "total": total, "has_more": int64(page*limit) < total})
	})
	scoped.GET("/end-users", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		list, err := service.ListEndUsers(c.Request.Context(), workspaceID, runtimeworkspace.EndUserListQuery{
			Search: c.Query("search"), Status: c.Query("status"), QuotaState: c.Query("quota_state"), Plan: c.Query("plan"),
			Sort: c.Query("sort"), Ascending: c.Query("order") == "asc",
			Page:  managementPositiveQueryInt(c.Query("page"), 1, 1_000_000),
			Limit: managementPositiveQueryInt(c.Query("limit"), 50, 200),
		})
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, list)
	})
	scoped.GET("/end-users/summary", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		summary, err := service.SummarizeEndUsers(c.Request.Context(), workspaceID)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, summary)
	})
	scoped.GET("/end-users/usage-series", func(c *gin.Context) {
		workspaceID, _, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		series, err := service.EndUserUsageSeries(c.Request.Context(), workspaceID, nil,
			managementPositiveQueryInt(c.Query("days"), 30, 90), c.Query("tz"))
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, series)
	})
	scoped.POST("/end-users/batch", func(c *gin.Context) {
		workspaceID, accountID, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
		if err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		var req struct {
			SubjectIDs []uuid.UUID `json:"subject_ids"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		patch, err := runtimeworkspace.ParseEndUserPatch(body)
		if err != nil {
			managementFail(c, err)
			return
		}
		result, err := service.UpdateEndUsers(c.Request.Context(), workspaceID, req.SubjectIDs,
			runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorAccount, ID: accountID}, patch)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, result)
	})
	endUser := func(handle func(c *gin.Context, workspaceID, accountID, subjectID uuid.UUID)) gin.HandlerFunc {
		return func(c *gin.Context) {
			workspaceID, accountID, ok := managementScope(c, c.Param("workspace_id"))
			if !ok {
				return
			}
			subjectID, err := uuid.Parse(c.Param("subject_id"))
			if err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}
			handle(c, workspaceID, accountID, subjectID)
		}
	}
	scoped.GET("/end-users/:subject_id", endUser(func(c *gin.Context, workspaceID, _, subjectID uuid.UUID) {
		view, err := service.GetEndUser(c.Request.Context(), workspaceID, subjectID)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, view)
	}))
	scoped.PATCH("/end-users/:subject_id", endUser(func(c *gin.Context, workspaceID, accountID, subjectID uuid.UUID) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
		if err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		patch, err := runtimeworkspace.ParseEndUserPatch(body)
		if err != nil {
			managementFail(c, err)
			return
		}
		view, err := service.UpdateEndUser(c.Request.Context(), workspaceID, subjectID,
			runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorAccount, ID: accountID}, patch)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, view)
	}))
	scoped.POST("/end-users/:subject_id/export", endUser(func(c *gin.Context, workspaceID, accountID, subjectID uuid.UUID) {
		export, err := service.ExportEndUser(c.Request.Context(), workspaceID, subjectID,
			runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorAccount, ID: accountID})
		if err != nil {
			managementFail(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, export)
	}))
	scoped.DELETE("/end-users/:subject_id", endUser(func(c *gin.Context, workspaceID, accountID, subjectID uuid.UUID) {
		if err := service.EraseEndUser(c.Request.Context(), workspaceID, subjectID,
			runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorAccount, ID: accountID}); err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, gin.H{"result": "success"})
	}))
	scoped.GET("/end-users/:subject_id/credits", endUser(func(c *gin.Context, workspaceID, _, subjectID uuid.UUID) {
		ledger, err := service.ListCredits(c.Request.Context(), workspaceID, subjectID,
			managementPositiveQueryInt(c.Query("page"), 1, 1_000_000), managementPositiveQueryInt(c.Query("limit"), 50, 200))
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, ledger)
	}))
	scoped.POST("/end-users/:subject_id/credits", endUser(func(c *gin.Context, workspaceID, accountID, subjectID uuid.UUID) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 16<<10))
		if err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		grant, err := runtimeworkspace.ParseCreditGrant(body)
		if err != nil {
			managementFail(c, err)
			return
		}
		result, err := service.GrantCredits(c.Request.Context(), workspaceID, subjectID,
			runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorAccount, ID: accountID}, grant)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, result)
	}))
	scoped.GET("/end-users/:subject_id/entitlement", endUser(func(c *gin.Context, workspaceID, _, subjectID uuid.UUID) {
		entitlement, err := service.EndUserEntitlement(c.Request.Context(), workspaceID, subjectID)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, entitlement)
	}))
	scoped.GET("/end-users/:subject_id/usage-series", endUser(func(c *gin.Context, workspaceID, _, subjectID uuid.UUID) {
		series, err := service.EndUserUsageSeries(c.Request.Context(), workspaceID, &subjectID,
			managementPositiveQueryInt(c.Query("days"), 30, 90), c.Query("tz"))
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, series)
	}))
	scoped.GET("/end-users/:subject_id/events", endUser(func(c *gin.Context, workspaceID, _, subjectID uuid.UUID) {
		events, err := service.ListEndUserEvents(c.Request.Context(), workspaceID, subjectID)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, gin.H{"items": events})
	}))
	scoped.PUT("/agents", func(c *gin.Context) {
		workspaceID, accountID, ok := managementScope(c, c.Param("workspace_id"))
		if !ok {
			return
		}
		var req struct {
			ExpectedRevision *int64    `json:"expected_revision" binding:"required"`
			AgentIDs         *[]string `json:"agent_ids" binding:"required,max=100,dive,uuid"`
			// DigitalHumanIDs is optional; when it is left out, the bound
			// DigitalHumans stay as they are, so older clients keep them.
			DigitalHumanIDs *[]string `json:"digital_human_ids" binding:"omitempty,max=100,dive,uuid"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.ExpectedRevision == nil || req.AgentIDs == nil || *req.ExpectedRevision < 0 {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		refs := make([]runtimeworkspace.AppRef, 0, len(*req.AgentIDs))
		for _, rawAgentID := range *req.AgentIDs {
			agentID, err := uuid.Parse(strings.TrimSpace(rawAgentID))
			if err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}
			refs = append(refs, runtimeworkspace.AppRef{Type: runtimeworkspace.AppTypeAgent, ID: agentID})
		}
		if req.DigitalHumanIDs != nil {
			for _, rawID := range *req.DigitalHumanIDs {
				id, err := uuid.Parse(strings.TrimSpace(rawID))
				if err != nil {
					response.Fail(c, response.ErrInvalidParam)
					return
				}
				refs = append(refs, runtimeworkspace.AppRef{Type: runtimeworkspace.AppTypeDigitalHuman, ID: id})
			}
		} else {
			current, err := service.ListBindings(c.Request.Context(), workspaceID)
			if err != nil {
				managementFail(c, err)
				return
			}
			for _, item := range current.Items {
				if id, err := uuid.Parse(item.AgentID); err == nil && item.AppType == runtimeworkspace.AppTypeDigitalHuman && item.Status == runtimeworkspace.AgentBindingActive {
					refs = append(refs, runtimeworkspace.AppRef{Type: runtimeworkspace.AppTypeDigitalHuman, ID: id})
				}
			}
		}
		apps, err := service.SyncAppBindings(c.Request.Context(), workspaceID, accountID, *req.ExpectedRevision, refs)
		if err != nil {
			managementFail(c, err)
			return
		}
		response.Success(c, apps)
	})
}

func managementPositiveQueryInt(raw string, fallback, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func runtimeWorkspaceManagerRequired(workspaceService interfaces.WorkspaceManagementService) gin.HandlerFunc {
	return runtimeWorkspaceManagerRequiredWithDB(nil, workspaceService)
}

func runtimeWorkspaceManagerRequiredWithDB(db *gorm.DB, workspaceService interfaces.WorkspaceManagementService) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaceID := strings.TrimSpace(util.GetWorkspaceID(c))
		accountID := strings.TrimSpace(middleware.GetAccountID(c))
		permissionWorkspaceID := workspaceID
		if db != nil && workspaceID != "" {
			var runtimeWorkspace runtimeworkspace.RuntimeWorkspace
			if err := db.WithContext(c.Request.Context()).
				Where("runtime_id = ? OR workspace_id = ?", workspaceID, workspaceID).
				First(&runtimeWorkspace).Error; err == nil {
				permissionWorkspaceID = runtimeWorkspace.ResourceWorkspaceID().String()
			}
		}
		if workspaceID == "" || accountID == "" || !workspaceService.CheckPermission(c.Request.Context(), permissionWorkspaceID, accountID) {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}
		c.Next()
	}
}

func runtimeWorkspaceManagementScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaceID := strings.TrimSpace(c.Param("workspace_id"))
		if workspaceID == "" {
			workspaceID = strings.TrimSpace(c.GetHeader("X-Workspace-ID"))
		}
		if _, err := uuid.Parse(workspaceID); err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			c.Abort()
			return
		}
		util.SetWorkspaceScopeCompat(c, workspaceID)
		c.Next()
	}
}

func managementScope(c *gin.Context, requested string) (uuid.UUID, uuid.UUID, bool) {
	workspaceIDText := strings.TrimSpace(requested)
	resolvedWorkspaceID := strings.TrimSpace(util.GetWorkspaceID(c))
	if workspaceIDText != "" && resolvedWorkspaceID != "" && workspaceIDText != resolvedWorkspaceID {
		response.Fail(c, response.ErrPermissionDenied)
		return uuid.Nil, uuid.Nil, false
	}
	if workspaceIDText == "" {
		workspaceIDText = resolvedWorkspaceID
	}
	workspaceID, err := uuid.Parse(workspaceIDText)
	accountID, accountErr := uuid.Parse(strings.TrimSpace(middleware.GetAccountID(c)))
	if err != nil || accountErr != nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return uuid.Nil, uuid.Nil, false
	}
	return workspaceID, accountID, true
}

func managementFail(c *gin.Context, err error) {
	if errors.Is(err, runtimeworkspace.ErrCreditConflict) {
		c.JSON(http.StatusConflict, response.Response{Code: "runtime_credit_conflict", Message: "This external ID was already used for a grant of another amount"})
		return
	}
	if errors.Is(err, runtimeworkspace.ErrBindingRevisionConflict) {
		c.JSON(http.StatusConflict, response.Response{Code: "runtime_agent_bindings_changed", Message: "The assistant selection was updated by another administrator"})
		return
	}
	if errors.Is(err, runtimeworkspace.ErrSourceWorkspacePermission) {
		c.JSON(http.StatusForbidden, response.Response{Code: "runtime_source_workspace_permission_required", Message: "Workspace owner or administrator permission is required for the source assistant"})
		return
	}
	if errors.Is(err, runtimeworkspace.ErrIdempotencyConflict) {
		c.JSON(http.StatusConflict, response.Response{Code: "runtime_credential_idempotency_conflict", Message: "Idempotency-Key was already used for another credential request"})
		return
	}
	if errors.Is(err, runtimeworkspace.ErrOperationInProgress) {
		c.Header("Retry-After", "1")
		c.JSON(http.StatusConflict, response.Response{Code: "runtime_credential_operation_in_progress", Message: "Credential creation is still in progress"})
		return
	}
	if errors.Is(err, runtimeworkspace.ErrSecretReplayExpired) {
		c.JSON(http.StatusConflict, response.Response{Code: "runtime_credential_replay_expired", Message: "Credential was created but its secret can no longer be replayed"})
		return
	}
	if errors.Is(err, runtimeworkspace.ErrInvalidWorkspace) || errors.Is(err, runtimeworkspace.ErrInvalidCredential) || errors.Is(err, runtimeworkspace.ErrInvalidBinding) {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if errors.Is(err, runtimeworkspace.ErrAppKeyConflict) {
		c.JSON(http.StatusConflict, response.Response{Code: "213003", Message: response.ErrAPIKeyAlreadyExists.Message})
		return
	}
	if errors.Is(err, runtimeworkspace.ErrAppNotFound) || errors.Is(err, runtimeworkspace.ErrEndUserNotFound) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	if errors.Is(err, runtimeworkspace.ErrAppDisabled) {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if errors.Is(err, runtimeworkspace.ErrWorkspaceDisabled) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Fail(c, response.ErrSystemError)
}
