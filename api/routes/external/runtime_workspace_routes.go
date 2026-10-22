package external

import (
	"context"
	"errors"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/externalapikey"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	redisPkg "github.com/zgiai/zgi/api/pkg/redis"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

const (
	runtimeExternalUserHeader      = "X-ZGI-External-User-ID"
	runtimeAuditPersistenceTimeout = 2 * time.Second
)

type RuntimeWorkspaceRouteDeps struct {
	DB           *gorm.DB
	FileService  interfaces.FileService
	AgentHandler *agents.AgentsHandler
	// DigitalHuman serves bound DigitalHumans; without it they answer not
	// found (ADR 0010).
	DigitalHuman *digitalhuman.ExternalHandler
}

func RegisterRuntimeWorkspaceRoutes(router *gin.RouterGroup, deps RuntimeWorkspaceRouteDeps) {
	if !runtimeworkspace.RuntimeEnabled() {
		return
	}
	if router == nil || deps.DB == nil || deps.FileService == nil || deps.AgentHandler == nil {
		panic("runtime workspace routes require db, file service, and agent handler")
	}
	service := runtimeworkspace.NewService(deps.DB)
	// Every DigitalHuman access service, including those built for
	// background execution, trusts the callers of bound Runtime Workspaces.
	digitalhuman.UseRuntimeWorkspaceAccess(func(db *gorm.DB) digitalhuman.RuntimeWorkspaceAccess { return runtimeworkspace.NewService(db) })
	// Every chat runtime meters and stops Runtime Workspace end users by the
	// same points quotas (ADR 0010).
	runtimeservice.UseEndUserQuotaGuard(runtimeworkspace.NewQuotaGuard(deps.DB))
	runtimeworkspace.UseEndUserDataOwner(digitalhuman.NewEndUserData())
	dh := deps.DigitalHuman
	app := func(agent, digitalHuman gin.HandlerFunc) gin.HandlerFunc {
		return runtimeAppRoute(dh, agent, digitalHuman)
	}
	var dhRoute func(name string) gin.HandlerFunc
	if dh != nil {
		dhRoute = func(name string) gin.HandlerFunc { return dh.RuntimeWorkspaceHandler(name) }
	} else {
		dhRoute = func(string) gin.HandlerFunc { return nil }
	}
	group := router.Group("/connect/v1")
	group.Use(runtimeWorkspaceCredentialAuth(service), runtimeWorkspaceRequestLease(service), runtimeWorkspaceAudit(service), runtimeWorkspaceRateLimit(), runtimeWorkspaceSubjectAuth(service))
	group.GET("/apps", func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		apps, err := service.ListApps(c.Request.Context(), principal.RuntimeWorkspace.LogicalID())
		if err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, gin.H{"items": apps})
	})

	// The integrating server manages its end users by the IDs it sends
	// (ADR 0010). An end user exists once they have made a request.
	endUser := func(handle func(c *gin.Context, principal *runtimeworkspace.Principal, subjectID uuid.UUID)) gin.HandlerFunc {
		return func(c *gin.Context) {
			principal, ok := runtimePrincipal(c)
			if !ok {
				return
			}
			subjectID, err := runtimeworkspace.EndUserSubject(principal.RuntimeWorkspace.LogicalID(), c.Param("user"))
			if err != nil {
				runtimeFail(c, err)
				return
			}
			handle(c, principal, subjectID)
		}
	}
	group.GET("/end-users/:user", endUser(func(c *gin.Context, principal *runtimeworkspace.Principal, subjectID uuid.UUID) {
		view, err := service.GetEndUser(c.Request.Context(), principal.RuntimeWorkspace.LogicalID(), subjectID)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, view)
	}))
	group.PATCH("/end-users/:user", endUser(func(c *gin.Context, principal *runtimeworkspace.Principal, subjectID uuid.UUID) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
		if err != nil {
			runtimeFail(c, runtimeworkspace.ErrInvalidEndUser)
			return
		}
		patch, err := runtimeworkspace.ParseEndUserPatch(body)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		view, err := service.UpdateEndUser(c.Request.Context(), principal.RuntimeWorkspace.LogicalID(), subjectID,
			runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorCredential, ID: principal.Credential.ID}, patch)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, view)
	}))
	group.POST("/end-users/:user/export", endUser(func(c *gin.Context, principal *runtimeworkspace.Principal, subjectID uuid.UUID) {
		export, err := service.ExportEndUser(c.Request.Context(), principal.RuntimeWorkspace.LogicalID(), subjectID,
			runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorCredential, ID: principal.Credential.ID})
		if err != nil {
			runtimeFail(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, export)
	}))
	group.DELETE("/end-users/:user", endUser(func(c *gin.Context, principal *runtimeworkspace.Principal, subjectID uuid.UUID) {
		if err := service.EraseEndUser(c.Request.Context(), principal.RuntimeWorkspace.LogicalID(), subjectID,
			runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorCredential, ID: principal.Credential.ID}); err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, gin.H{"result": "success"})
	}))

	appGroup := group.Group("/apps/:app_key")
	appGroup.GET("", func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		app, err := service.GetApp(c.Request.Context(), principal.RuntimeWorkspace.LogicalID(), c.Param("app_key"))
		if err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, app)
	})
	// Each app route serves the bound app's own operation: an Agent's or a
	// DigitalHuman's (ADR 0010). Operations one type lacks answer not found.
	appGroup.GET("/config", app(deps.AgentHandler.GetAPIKeyAgentRuntimeConfig, dhRoute("config")))
	appGroup.GET("/parameters", app(runtimeAppParameters(service, deps.FileService), dhRoute("config")))
	appGroup.POST("/chat", app(deps.AgentHandler.ChatAPIKeyAgent, dhRoute("chat")))
	appGroup.GET("/conversations", app(deps.AgentHandler.ListAPIKeyAgentRuntimeConversations, dhRoute("conversations.list")))
	appGroup.GET("/conversations/search", app(deps.AgentHandler.SearchAPIKeyAgentRuntimeConversations, dhRoute("conversations.search")))
	appGroup.POST("/conversations", app(deps.AgentHandler.CreateAPIKeyAgentRuntimeConversation, nil))
	appGroup.GET("/conversations/:conversation_id", app(deps.AgentHandler.GetAPIKeyAgentRuntimeConversation, dhRoute("conversations.get")))
	appGroup.PATCH("/conversations/:conversation_id", app(deps.AgentHandler.UpdateAPIKeyAgentRuntimeConversation, dhRoute("conversations.update")))
	appGroup.DELETE("/conversations/:conversation_id", app(deps.AgentHandler.DeleteAPIKeyAgentRuntimeConversation, dhRoute("conversations.delete")))
	appGroup.GET("/conversations/:conversation_id/messages", app(deps.AgentHandler.ListAPIKeyAgentRuntimeMessages, dhRoute("conversations.messages")))
	appGroup.GET("/conversations/:conversation_id/events", app(deps.AgentHandler.StreamAPIKeyAgentRuntimeEvents, nil))
	appGroup.POST("/conversations/:conversation_id/stop", app(deps.AgentHandler.StopAPIKeyAgentRuntimeConversation, nil))
	appGroup.GET("/conversations/:conversation_id/files/:file_id", app(nil, dhRoute("conversations.file")))
	appGroup.POST("/conversations/:conversation_id/messages/:message_id/workflow-continuation", app(deps.AgentHandler.ContinueAPIKeyAgentRuntimeWorkflowContinuation, dhRoute("continue.workflow")))
	appGroup.POST("/conversations/:conversation_id/messages/:message_id/user-input/:request_id/continue", app(deps.AgentHandler.ContinueAPIKeyAgentRuntimeUserInput, dhRoute("continue.user_input")))
	appGroup.POST("/conversations/:conversation_id/messages/:message_id/tool-governance/:correlation_id/continue", app(nil, dhRoute("continue.tool_governance")))
	appGroup.POST("/conversations/:conversation_id/messages/:message_id/connector-confirmation", app(nil, dhRoute("continue.connector_confirmation")))
	appGroup.POST("/conversations/:conversation_id/messages/:message_id/token-budget/:request_id/continue", app(deps.AgentHandler.ContinueAPIKeyAgentRuntimeTokenBudget, dhRoute("continue.token_budget")))
	appGroup.POST("/conversations/:conversation_id/messages/:message_id/regenerate", app(nil, dhRoute("regenerate")))
	appGroup.POST("/messages/:message_id/regenerate", app(deps.AgentHandler.RegenerateAPIKeyAgentRuntimeMessage, nil))
	appGroup.GET("/runs/:run_id", app(nil, dhRoute("runs.get")))
	appGroup.GET("/runs/:run_id/events", app(nil, dhRoute("runs.events")))
	appGroup.POST("/runs/:run_id/stop", app(nil, dhRoute("runs.stop")))
	appGroup.POST("/runs/:run_id/runtime-input", app(nil, dhRoute("runs.runtime_input")))
	appGroup.GET("/tasks", app(nil, dhRoute("tasks.list")))
	appGroup.POST("/tasks", app(nil, dhRoute("tasks.create")))
	appGroup.POST("/tasks/schedule-preview", app(nil, dhRoute("tasks.schedule_preview")))
	appGroup.GET("/tasks/:task_id", app(nil, dhRoute("tasks.get")))
	appGroup.PATCH("/tasks/:task_id", app(nil, dhRoute("tasks.update")))
	appGroup.POST("/tasks/:task_id/actions/:action", app(nil, dhRoute("tasks.action")))
	appGroup.GET("/tasks/:task_id/runs", app(nil, dhRoute("tasks.runs")))
	appGroup.GET("/tasks/:task_id/activities", app(nil, dhRoute("tasks.activities")))
	appGroup.GET("/reports", app(nil, dhRoute("reports.list")))
	appGroup.GET("/reports/:report_id", app(nil, dhRoute("reports.get")))
	appGroup.POST("/reports/:report_id/review", app(nil, dhRoute("reports.review")))
	appGroup.GET("/reports/:report_id/files/:file_id", app(nil, dhRoute("reports.file")))
	appGroup.GET("/notifications", app(nil, dhRoute("notifications.list")))
	appGroup.POST("/notifications/read", app(nil, dhRoute("notifications.read")))
	appGroup.GET("/notifications/unread-count", app(nil, dhRoute("notifications.unread_count")))
	appGroup.POST("/notifications/read-all", app(nil, dhRoute("notifications.read_all")))
	appGroup.GET("/memory", app(deps.AgentHandler.GetAPIKeyAgentMemory, dhRoute("memory.get")))
	appGroup.GET("/memory/export", app(deps.AgentHandler.ExportAPIKeyAgentMemory, nil))
	appGroup.PUT("/memory/:key", app(deps.AgentHandler.PutAPIKeyAgentMemory, nil))
	appGroup.DELETE("/memory/:key", app(deps.AgentHandler.DeleteAPIKeyAgentMemoryValue, nil))
	appGroup.DELETE("/memory/items/:key", app(nil, dhRoute("memory.delete_item")))
	appGroup.POST("/memory/clear", app(nil, dhRoute("memory.clear")))
	appGroup.DELETE("/memory", app(deps.AgentHandler.DeleteAllAPIKeyAgentMemory, nil))
	appGroup.POST("/memory/operations/:operation_id/undo", app(deps.AgentHandler.UndoAPIKeyAgentMemoryOperation, nil))
	appGroup.GET("/memory/settings", app(deps.AgentHandler.GetAPIKeyAgentMemorySettings, nil))
	appGroup.PATCH("/memory/settings", app(deps.AgentHandler.PatchAPIKeyAgentMemorySettings, nil))

	fileGroup := group.Group("/files")
	fileGroup.POST("", runtimeUploadFile(deps.DB, deps.FileService))
	fileGroup.GET("", runtimeListFiles(deps.DB))
	fileGroup.GET("/:file_id", runtimeGetFile(deps.DB, deps.FileService))
	fileGroup.DELETE("/:file_id", runtimeDeleteFile(deps.DB, deps.FileService))
	fileGroup.GET("/:file_id/download-url", runtimeDownloadURL(deps.DB, deps.FileService))
}

func runtimeAppParameters(service *runtimeworkspace.Service, fileService interfaces.FileService) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		app, err := service.GetApp(c.Request.Context(), principal.RuntimeWorkspace.LogicalID(), c.Param("app_key"))
		if err != nil {
			runtimeFail(c, err)
			return
		}
		uploadConfig := fileService.GetUploadConfig()
		systemParameters := gin.H{}
		fileLimit := 0
		if uploadConfig != nil {
			fileLimit = uploadConfig.WorkflowFileUploadLimit
			systemParameters = gin.H{
				"file_size_limit": uploadConfig.FileSizeLimit, "image_file_size_limit": uploadConfig.ImageFileSizeLimit,
				"audio_file_size_limit": uploadConfig.AudioFileSizeLimit, "video_file_size_limit": uploadConfig.VideoFileSizeLimit,
				"file_upload_count_limit": uploadConfig.WorkflowFileUploadLimit,
			}
		}
		response.Success(c, gin.H{
			"features": gin.H{
				"home_title": app.HomeTitle, "opening_statement": app.OpeningStatement, "input_placeholder": app.InputPlaceholder,
				"theme_color": app.ThemeColor, "suggested_questions": app.SuggestedQuestions, "supports_vision": app.SupportsVision,
			},
			"file_upload": gin.H{
				"enabled": app.FileUploadEnabled, "number_limits": fileLimit, "allowed_file_types": fileService.GetSupportedFileTypes(),
				"transfer_methods": []string{"local_file"},
				"image":            gin.H{"enabled": app.FileUploadEnabled && app.SupportsVision, "number_limits": fileLimit, "transfer_methods": []string{"local_file"}},
			},
			"agent_memory":      gin.H{"enabled": app.AgentMemoryEnabled, "requires_user": app.MemoryRequiresUser},
			"system_parameters": systemParameters,
		})
	}
}

func runtimeWorkspaceMetadataRequest(method, fullPath string) bool {
	return method == http.MethodGet && (strings.HasSuffix(fullPath, "/apps/:app_key") || strings.HasSuffix(fullPath, "/apps/:app_key/config") || strings.HasSuffix(fullPath, "/apps/:app_key/parameters"))
}

func runtimeWorkspaceCredentialAuth(service *runtimeworkspace.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey, err := externalapikey.Extract(c.GetHeader("Authorization"), c.GetHeader("X-API-Key"))
		if err != nil {
			runtimeFail(c, runtimeworkspace.ErrUnauthorized)
			c.Abort()
			return
		}
		principal, err := service.AuthenticateCredential(c.Request.Context(), rawKey)
		if err != nil {
			runtimeFail(c, err)
			c.Abort()
			return
		}
		c.Set("runtime_workspace_principal", principal)
		c.Set("credential_id", principal.Credential.ID.String())
		c.Set("runtime_workspace_id", principal.RuntimeWorkspace.LogicalID().String())
		c.Set("runtime_resource_workspace_id", principal.RuntimeWorkspace.ResourceWorkspaceID().String())
		c.Next()
	}
}

func runtimeWorkspaceRequestLease(service *runtimeworkspace.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		runtimeID := principal.RuntimeWorkspace.LogicalID()
		if err := service.AcquireRequestLease(c.Request.Context(), runtimeID); err != nil {
			runtimeFail(c, err)
			c.Abort()
			return
		}
		defer func() {
			releaseCtx, cancel := runtimeAuditContext(c.Request.Context())
			defer cancel()
			if err := service.ReleaseRequestLease(releaseCtx, runtimeID); err != nil {
				logger.Warn("runtime workspace request lease release failed", "error", err, "runtime_workspace_id", runtimeID.String())
			}
		}()
		c.Next()
	}
}

func runtimeWorkspaceSubjectAuth(service *runtimeworkspace.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			c.Abort()
			return
		}
		externalUser := strings.TrimSpace(c.GetHeader(runtimeExternalUserHeader))
		if externalUser == "" {
			externalUser = strings.TrimSpace(c.GetHeader(agents.ExternalUserIDHeader))
		}
		appKey := strings.TrimSpace(c.Param("app_key"))
		fullPath := c.FullPath()
		metadataRequest := runtimeWorkspaceMetadataRequest(c.Request.Method, fullPath)
		needsSubject := strings.Contains(fullPath, "/files") || appKey != "" && !metadataRequest
		if needsSubject && externalUser == "" {
			runtimeFail(c, runtimeworkspace.ErrSubjectRequired)
			c.Abort()
			return
		}
		authAppKey := appKey
		if metadataRequest {
			authAppKey = ""
		}
		if err := service.ResolveRequestPrincipal(c.Request.Context(), principal, externalUser, authAppKey); err != nil {
			runtimeFail(c, err)
			c.Abort()
			return
		}
		if metadataRequest && appKey != "" {
			binding, err := service.ResolveApp(c.Request.Context(), principal.RuntimeWorkspace.LogicalID(), appKey)
			if err != nil {
				runtimeFail(c, err)
				c.Abort()
				return
			}
			principal.Agent = binding
		}

		// A Runtime Subject is the external user's stable runtime account. It is
		// deliberately independent from rotating credentials so conversations
		// remain stable when keys are rotated.
		accountID := principal.RuntimeWorkspace.LogicalID().String()
		if principal.SubjectID != uuid.Nil {
			accountID = principal.SubjectID.String()
			c.Request.Header.Set(agents.ExternalUserIDHeader, principal.ExternalUserID)
			c.Set("runtime_subject_id", principal.SubjectID.String())
		}
		c.Set("account_id", accountID)
		runtimeContext := runtimeworkspace.RuntimeContext{
			RuntimeWorkspaceID: principal.RuntimeWorkspace.LogicalID(),
			WorkspaceID:        principal.RuntimeWorkspace.ResourceWorkspaceID(),
			OrganizationID:     principal.RuntimeWorkspace.OrganizationID,
			CredentialID:       principal.Credential.ID,
			SubjectID:          principal.SubjectID,
			Scopes:             []string{"apps:read", "chat", "conversations", "conversation:search", "message:regenerate", "continuations", "files", "memory", "memory:settings"},
		}
		if principal.ExternalUserID != "" {
			runtimeContext.ExternalUserIDHash = externalapikey.Hash(principal.ExternalUserID)
		}
		if principal.Agent != nil {
			runtimeContext.AgentID = principal.Agent.AgentID
		}
		c.Set("runtime_context", runtimeContext)
		c.Set("agent_id", "")
		c.Set("invoke_from", "external-api")
		c.Set("created_from", "external-api")
		c.Set("created_by_role", "end_user")
		util.SetWorkspaceScopeCompat(c, principal.RuntimeWorkspace.ResourceWorkspaceID().String())
		util.SetOrganizationID(c, principal.RuntimeWorkspace.OrganizationID.String())
		// The chat runtime consumes this narrow capability interface to authorize
		// permanent Runtime files. It must not fall back to Workspace membership,
		// because Runtime Subjects are intentionally not members.
		c.Set("runtime_file_authorizer", service)
		if principal.Agent != nil {
			if principal.Agent.AppType == runtimeworkspace.AppTypeDigitalHuman {
				c.Set(runtimeAppTypeContextKey, runtimeworkspace.AppTypeDigitalHuman)
				c.Set(digitalhuman.RuntimeDigitalHumanIDContextKey, principal.Agent.AgentID.String())
			} else {
				c.Set(runtimeAppTypeContextKey, runtimeworkspace.AppTypeAgent)
				c.Set("agent_id", principal.Agent.AgentID.String())
			}
		}
		c.Next()
	}
}

const runtimeAppTypeContextKey = "runtime_app_type"

// runtimeAppRoute serves an app route with the handler of the bound app's
// type: an Agent's or a DigitalHuman's. A type without a handler for the
// route answers that the app has no such operation.
func runtimeAppRoute(digitalHumans *digitalhuman.ExternalHandler, agent, digitalHuman gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(runtimeAppTypeContextKey) == runtimeworkspace.AppTypeDigitalHuman {
			if digitalHumans == nil || digitalHuman == nil {
				runtimeFail(c, runtimeworkspace.ErrAppNotFound)
				return
			}
			if !digitalHumans.PrepareRuntimeWorkspaceCaller(c) {
				return
			}
			digitalHuman(c)
			return
		}
		if agent == nil {
			runtimeFail(c, runtimeworkspace.ErrAppNotFound)
			return
		}
		agent(c)
	}
}

func runtimePrincipal(c *gin.Context) (*runtimeworkspace.Principal, bool) {
	value, ok := c.Get("runtime_workspace_principal")
	principal, valid := value.(*runtimeworkspace.Principal)
	if !ok || !valid || principal == nil || principal.RuntimeWorkspace == nil {
		runtimeFail(c, runtimeworkspace.ErrUnauthorized)
		return nil, false
	}
	return principal, true
}

func runtimeAuditContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), runtimeAuditPersistenceTimeout)
}

func runtimeWorkspaceAudit(service *runtimeworkspace.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestCtx := c.Request.Context()
		c.Next()
		principal, ok := c.Get("runtime_workspace_principal")
		runtimePrincipal, valid := principal.(*runtimeworkspace.Principal)
		if !ok || !valid || runtimePrincipal == nil || runtimePrincipal.RuntimeWorkspace == nil || runtimePrincipal.Credential == nil {
			return
		}
		route := c.FullPath()
		if strings.TrimSpace(route) == "" {
			route = c.Request.URL.Path
		}
		fields := []interface{}{
			"log_type", "runtime_workspace_audit",
			"method", c.Request.Method,
			"route", route,
			"status", c.Writer.Status(),
			"runtime_workspace_id", runtimePrincipal.RuntimeWorkspace.LogicalID().String(),
			"resource_workspace_id", runtimePrincipal.RuntimeWorkspace.ResourceWorkspaceID().String(),
			"credential_id", runtimePrincipal.Credential.ID.String(),
			"subject_id", runtimePrincipal.SubjectID.String(),
		}
		if runtimePrincipal.Agent != nil {
			fields = append(fields, "agent_id", runtimePrincipal.Agent.AgentID.String())
		}
		logger.Info("runtime workspace audit", fields...)
		if service == nil {
			return
		}
		var agentID uuid.UUID
		appType := ""
		if runtimePrincipal.Agent != nil {
			agentID = runtimePrincipal.Agent.AgentID
			appType = runtimePrincipal.Agent.AppType
		}
		details := map[string]any{
			"external_user_id_hash": runtimeExternalUserHash(runtimePrincipal.ExternalUserID),
		}
		if requestErr := requestCtx.Err(); requestErr != nil {
			details["request_canceled"] = errors.Is(requestErr, context.Canceled)
			details["request_context_error"] = requestErr.Error()
		}
		result := ""
		if outcome := strings.TrimSpace(c.GetString(agents.ExternalAgentRuntimeOutcomeContextKey)); outcome == agents.ExternalAgentRuntimeOutcomeFailure || requestCtx.Err() != nil {
			result = runtimeworkspace.AccessEventResultFailure
		}
		auditCtx, cancel := runtimeAuditContext(requestCtx)
		defer cancel()
		if err := service.RecordAccessEvent(auditCtx, runtimeworkspace.AccessEventInput{
			RuntimeWorkspaceID: runtimePrincipal.RuntimeWorkspace.LogicalID(),
			OrganizationID:     runtimePrincipal.RuntimeWorkspace.OrganizationID,
			WorkspaceID:        runtimePrincipal.RuntimeWorkspace.ResourceWorkspaceID(),
			CredentialID:       runtimePrincipal.Credential.ID,
			SubjectID:          runtimePrincipal.SubjectID,
			AgentID:            agentID,
			AppType:            appType,
			Method:             c.Request.Method,
			Route:              route,
			StatusCode:         c.Writer.Status(),
			Result:             result,
			Details:            details,
		}); err != nil {
			logger.Warn("runtime workspace audit persistence failed", "error", err, "route", route)
		}
	}
}

func runtimeExternalUserHash(externalUserID string) string {
	if strings.TrimSpace(externalUserID) == "" {
		return ""
	}
	return externalapikey.Hash(externalUserID)
}

func runtimeWorkspaceRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		principalValue, ok := c.Get("runtime_workspace_principal")
		principal, valid := principalValue.(*runtimeworkspace.Principal)
		if !ok || !valid || principal == nil || principal.RuntimeWorkspace == nil || principal.Credential == nil {
			c.Next()
			return
		}
		limit := runtimeworkspace.RuntimeRequestsPerMinute(principal.RuntimeWorkspace.PolicyJSON)
		if limit <= 0 {
			c.Next()
			return
		}
		key := runtimeWorkspaceRateLimitKey(principal)
		decision, err := runtimeworkspace.AllowRateLimitedRequest(c.Request.Context(), redisPkg.GetClient(), key, limit, time.Now())
		c.Header("X-RateLimit-Limit", strconv.Itoa(decision.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(decision.Remaining))
		if errors.Is(err, runtimeworkspace.ErrRateLimited) {
			retryAfter := int(decision.RetryAfter / time.Second)
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			runtimeFail(c, err)
			c.Abort()
			return
		}
		if err != nil {
			logger.Warn("runtime workspace rate limit unavailable", "error", err)
		}
		c.Next()
	}
}

func runtimeWorkspaceRateLimitKey(principal *runtimeworkspace.Principal) string {
	if principal == nil || principal.RuntimeWorkspace == nil {
		return ""
	}
	// The policy is owned by the Runtime Workspace, not by an individual
	// credential. Rotating or adding credentials must not multiply its quota.
	return principal.RuntimeWorkspace.LogicalID().String()
}

func runtimeFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, runtimeworkspace.ErrRateLimited):
		response.Fail(c, response.ErrRateLimitExceeded)
	case errors.Is(err, runtimeworkspace.ErrUnauthorized), errors.Is(err, runtimeworkspace.ErrSubjectRequired):
		response.Fail(c, response.ErrUnauthorized)
	case errors.Is(err, runtimeworkspace.ErrAppNotFound), errors.Is(err, runtimeworkspace.ErrFileNotFound), errors.Is(err, runtimeworkspace.ErrEndUserNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, runtimeworkspace.ErrInvalidWorkspace):
		response.Fail(c, response.ErrInvalidParam)
	case errors.Is(err, runtimeworkspace.ErrAppDisabled), errors.Is(err, runtimeworkspace.ErrWorkspaceDisabled):
		response.Fail(c, response.ErrPermissionDenied)
	default:
		response.Fail(c, response.ErrSystemError)
	}
}

func runtimeUploadFile(db *gorm.DB, fileService interfaces.FileService) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		file, err := c.FormFile("file")
		if err != nil {
			response.FailWithMessage(c, response.ErrInvalidParam, "file is required")
			return
		}
		limit := runtimeUploadLimit(fileService, file.Filename)
		if limit <= 0 || file.Size > limit {
			c.JSON(http.StatusRequestEntityTooLarge, response.Response{Code: strconv.Itoa(response.ErrFileTooLarge.Code), Message: response.ErrFileTooLarge.Message})
			return
		}
		content, err := readMultipartFile(file, limit)
		if err != nil {
			if errors.Is(err, filemodel.ErrFileTooLarge) {
				c.JSON(http.StatusRequestEntityTooLarge, response.Response{Code: strconv.Itoa(response.ErrFileTooLarge.Code), Message: response.ErrFileTooLarge.Message})
				return
			}
			response.Fail(c, response.ErrSystemError)
			return
		}
		mimeType := file.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = http.DetectContentType(content)
		}
		source := interfaces.FileSourceWorkflow
		resourceWorkspaceID := principal.RuntimeWorkspace.ResourceWorkspaceID().String()
		uploaded, err := fileService.UploadFile(c.Request.Context(), file.Filename, content, mimeType, principal.SubjectID.String(), principal.RuntimeWorkspace.OrganizationID.String(), filemodel.CreatedByRoleEndUser, &source, &resourceWorkspaceID, false, false)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		if uploaded == nil || strings.TrimSpace(uploaded.ID) == "" {
			runtimeFail(c, errors.New("runtime workspace file upload returned no file"))
			return
		}
		binding := &runtimeworkspace.ExternalFile{FileID: uploaded.ID, RuntimeWorkspaceID: principal.RuntimeWorkspace.LogicalID(), WorkspaceID: principal.RuntimeWorkspace.ResourceWorkspaceID(), OrganizationID: principal.RuntimeWorkspace.OrganizationID, SubjectID: principal.SubjectID, SubjectKind: "api_end_user", Surface: "runtime_workspace_api"}
		if err := db.WithContext(c.Request.Context()).Create(binding).Error; err != nil {
			_ = fileService.DeleteFiles(c.Request.Context(), []string{uploaded.ID})
			runtimeFail(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"id": uploaded.ID, "name": uploaded.Name, "size": uploaded.Size, "extension": uploaded.Extension, "mime_type": uploaded.MimeType, "created_at": uploaded.CreatedAt.Unix()})
	}
}

func runtimeListFiles(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		page := runtimePositiveQueryInt(c.Query("page"), 1, 1_000_000)
		limit := runtimePositiveQueryInt(c.Query("limit"), 20, 100)
		query := db.WithContext(c.Request.Context()).Table("runtime_external_files AS runtime_files").
			Joins("JOIN upload_files AS files ON files.id = runtime_files.file_id").
			Where("runtime_files.runtime_workspace_id = ? AND runtime_files.subject_id = ?", principal.RuntimeWorkspace.LogicalID(), principal.SubjectID)
		var total int64
		if err := query.Count(&total).Error; err != nil {
			runtimeFail(c, err)
			return
		}
		items := make([]runtimeworkspace.FileView, 0, limit)
		if err := query.Select("files.id, files.name, files.size, files.mime_type, files.extension, files.created_at").
			Order("runtime_files.created_at DESC").Limit(limit).Offset((page - 1) * limit).Scan(&items).Error; err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, gin.H{"items": items, "page": page, "limit": limit, "total": total, "has_more": int64(page*limit) < total})
	}
}

func runtimeGetFile(db *gorm.DB, fileService interfaces.FileService) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		binding, err := runtimeFileBinding(c, db, principal)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		file, err := fileService.GetFileByID(c.Request.Context(), binding.FileID)
		if err != nil || file == nil {
			runtimeFail(c, runtimeworkspace.ErrFileNotFound)
			return
		}
		response.Success(c, runtimeworkspace.FileView{ID: file.ID, Name: file.Name, Size: file.Size, MimeType: file.MimeType, Extension: file.Extension, CreatedAt: file.CreatedAt})
	}
}

func runtimeDownloadURL(db *gorm.DB, fileService interfaces.FileService) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		binding, err := runtimeFileBinding(c, db, principal)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		url, err := fileService.GetFileURL(c.Request.Context(), binding.FileID)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, gin.H{"url": url})
	}
}

func runtimeDeleteFile(db *gorm.DB, fileService interfaces.FileService) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := runtimePrincipal(c)
		if !ok {
			return
		}
		binding, err := runtimeFileBinding(c, db, principal)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		if err := fileService.DeleteFiles(c.Request.Context(), []string{binding.FileID}); err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, gin.H{"result": "success"})
	}
}

func runtimeFileBinding(c *gin.Context, db *gorm.DB, principal *runtimeworkspace.Principal) (*runtimeworkspace.ExternalFile, error) {
	fileID, err := uuid.Parse(strings.TrimSpace(c.Param("file_id")))
	if err != nil || fileID == uuid.Nil {
		return nil, runtimeworkspace.ErrFileNotFound
	}
	var binding runtimeworkspace.ExternalFile
	if err := db.WithContext(c.Request.Context()).Where("file_id = ? AND runtime_workspace_id = ? AND subject_id = ?", fileID, principal.RuntimeWorkspace.LogicalID(), principal.SubjectID).First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, runtimeworkspace.ErrFileNotFound
		}
		return nil, err
	}
	return &binding, nil
}

func runtimePositiveQueryInt(raw string, fallback, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func runtimeUploadLimit(fileService interfaces.FileService, filename string) int64 {
	config := fileService.GetUploadConfig()
	if config == nil {
		return 0
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	limitMB := config.FileSizeLimit
	switch {
	case filemodel.IsImageExtension(extension):
		limitMB = config.ImageFileSizeLimit
	case filemodel.IsVideoExtension(extension):
		limitMB = config.VideoFileSizeLimit
	case filemodel.IsAudioExtension(extension):
		limitMB = config.AudioFileSizeLimit
	}
	if limitMB <= 0 {
		return 0
	}
	return limitMB * 1024 * 1024
}

func readMultipartFile(file *multipart.FileHeader, limit int64) ([]byte, error) {
	opened, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	content, err := io.ReadAll(io.LimitReader(opened, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, filemodel.ErrFileTooLarge
	}
	return content, nil
}
