package digitalhuman

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"gorm.io/gorm"
)

// A Runtime Workspace serves bound DigitalHumans under /api/connect/v1 next
// to Agents (ADR 0010). Its caller is the Runtime Workspace credential acting
// for one end user, whose subject the Runtime Workspace already derived and
// registered. These checks stand in for the DigitalHuman's own API key.

// RuntimeWorkspaceAccess answers what the DigitalHuman domain needs to trust
// a Runtime Workspace caller, without depending on that module.
type RuntimeWorkspaceAccess interface {
	// CallerActive reports whether the credential is active and its Runtime
	// Workspace still binds the DigitalHuman as a callable app.
	CallerActive(ctx context.Context, runtimeWorkspaceID, credentialID, digitalHumanID uuid.UUID) error
	// SubjectBound reports whether an end-user subject was issued by a
	// Runtime Workspace that still binds the DigitalHuman, for runs started
	// later on that end user's behalf.
	SubjectBound(ctx context.Context, subjectID, digitalHumanID uuid.UUID) error
	// CanReadRuntimeExternalFile reports whether the end user uploaded the
	// file through that Runtime Workspace.
	CanReadRuntimeExternalFile(ctx context.Context, runtimeWorkspaceID, subjectID uuid.UUID, fileID string) (bool, error)
}

// Gin keys the Runtime Workspace boundary sets for a bound DigitalHuman.
const (
	RuntimeWorkspaceIDContextKey    = "runtime_workspace_id"
	RuntimeCredentialIDContextKey   = "credential_id"
	RuntimeSubjectIDContextKey      = "runtime_subject_id"
	RuntimeDigitalHumanIDContextKey = "digital_human_id"
	externalRuntimeCallerContext    = "digital_human_runtime_workspace_caller"
	runtimeWorkspaceInstanceIssuer  = "runtime_workspace"
	// runtimeWorkspaceSubjectIssuer is the issuer Runtime Workspaces register
	// their end-user subjects with.
	runtimeWorkspaceSubjectIssuer = "runtime_workspace_api"
	// runtimeWorkspaceEventContextKey keeps on an API run's invocation event
	// the Runtime Workspace it came through.
	runtimeWorkspaceEventContextKey = "runtime_workspace_id"
)

// runtimeWorkspaceCaller is a Runtime Workspace credential acting for one
// end user of a bound DigitalHuman.
type runtimeWorkspaceCaller struct {
	RuntimeWorkspaceID uuid.UUID
	CredentialID       uuid.UUID
	SubjectID          uuid.UUID
}

// WithRuntimeWorkspaces lets Runtime Workspace callers reach this
// DigitalHuman's external API.
func (s *AccessService) WithRuntimeWorkspaces(access RuntimeWorkspaceAccess) *AccessService {
	if s != nil {
		s.runtimeWorkspaces = access
	}
	return s
}

var runtimeWorkspaceAccessFactory atomic.Value

// UseRuntimeWorkspaceAccess registers how access services reach Runtime
// Workspaces. It is set where the Runtime Workspace routes are installed, so
// every access service, including those built for background execution,
// trusts the same callers.
func UseRuntimeWorkspaceAccess(factory func(*gorm.DB) RuntimeWorkspaceAccess) {
	if factory != nil {
		runtimeWorkspaceAccessFactory.Store(factory)
	}
}

// runtimeWorkspaceAccess returns the Runtime Workspace checks, or nil when
// Runtime Workspaces are not served.
func (s *AccessService) runtimeWorkspaceAccess() RuntimeWorkspaceAccess {
	if s == nil {
		return nil
	}
	if s.runtimeWorkspaces != nil {
		return s.runtimeWorkspaces
	}
	factory, _ := runtimeWorkspaceAccessFactory.Load().(func(*gorm.DB) RuntimeWorkspaceAccess)
	if factory == nil || s.db == nil {
		return nil
	}
	return factory(s.db)
}

// AuthenticateRuntimeWorkspace admits a Runtime Workspace caller in place of a
// DigitalHuman API key, as middleware.
func (h *ExternalHandler) AuthenticateRuntimeWorkspace(c *gin.Context) {
	if !h.PrepareRuntimeWorkspaceCaller(c) {
		c.Abort()
		return
	}
	c.Next()
}

// PrepareRuntimeWorkspaceCaller admits a Runtime Workspace caller in place of
// a DigitalHuman API key and reports whether it was admitted; it writes the
// error otherwise. The Runtime Workspace boundary has authenticated the
// credential, resolved the app key to this DigitalHuman, and registered the
// end user before this runs.
func (h *ExternalHandler) PrepareRuntimeWorkspaceCaller(c *gin.Context) bool {
	caller, digitalHumanID, ok := runtimeWorkspaceCallerFrom(c)
	runtimeWorkspaces := h.access.runtimeWorkspaceAccess()
	if !ok || runtimeWorkspaces == nil {
		h.writeAuthError(c, false)
		return false
	}
	access, err := h.access.publishedAccessByID(c.Request.Context(), digitalHumanID)
	if err != nil || !access.Settings.APIEnabled {
		h.writeAuthError(c, err == nil || errors.Is(err, ErrPublishInvalid))
		return false
	}
	if err := runtimeWorkspaces.CallerActive(c.Request.Context(), caller.RuntimeWorkspaceID, caller.CredentialID, digitalHumanID); err != nil {
		h.writeAuthError(c, false)
		return false
	}
	// The credential stands where an API key would: it is the principal of
	// every run and task change the end user makes through it.
	key := &APIKey{ID: caller.CredentialID, DigitalHumanID: digitalHumanID, OrganizationID: access.Definition.OrganizationID,
		WorkspaceID: access.Definition.WorkspaceID, Status: APIKeyStatusActive}
	c.Set(externalAPIKeyContext, key)
	c.Set(externalAccessContext, access)
	c.Set(externalRuntimeCallerContext, caller)
	return true
}

func runtimeWorkspaceCallerFrom(c *gin.Context) (runtimeWorkspaceCaller, uuid.UUID, bool) {
	parse := func(key string) uuid.UUID {
		id, err := uuid.Parse(strings.TrimSpace(c.GetString(key)))
		if err != nil {
			return uuid.Nil
		}
		return id
	}
	caller := runtimeWorkspaceCaller{RuntimeWorkspaceID: parse(RuntimeWorkspaceIDContextKey), CredentialID: parse(RuntimeCredentialIDContextKey), SubjectID: parse(RuntimeSubjectIDContextKey)}
	digitalHumanID := parse(RuntimeDigitalHumanIDContextKey)
	// Metadata reads such as the config carry no end user; every other
	// operation needs one, which apiUserScope enforces.
	if caller.RuntimeWorkspaceID == uuid.Nil || caller.CredentialID == uuid.Nil || digitalHumanID == uuid.Nil {
		return runtimeWorkspaceCaller{}, uuid.Nil, false
	}
	return caller, digitalHumanID, true
}

// runtimeWorkspaceScope is the end user's scope for a Runtime Workspace
// caller. The subject is the Runtime Workspace's, never derived from user.
func runtimeWorkspaceScope(caller runtimeWorkspaceCaller, access *PublishedAccess) Scope {
	subjectID, runtimeWorkspaceID := caller.SubjectID, caller.RuntimeWorkspaceID
	return Scope{
		OrganizationID: access.Definition.OrganizationID, WorkspaceID: access.Definition.WorkspaceID,
		ActorID: subjectID, PrincipalType: string(invocation.PrincipalService), PrincipalID: caller.CredentialID,
		UserSubjectKind: string(usersubject.APIEndUser), UserSubjectID: &subjectID, Surface: SurfaceAPI,
		RuntimeWorkspaceID: &runtimeWorkspaceID,
	}
}

// validateRuntimeWorkspaceFiles accepts files the end user uploaded through
// the same Runtime Workspace.
func (s *AccessService) validateRuntimeWorkspaceFiles(ctx context.Context, scope Scope, fileIDs []string) error {
	runtimeWorkspaces := s.runtimeWorkspaceAccess()
	if runtimeWorkspaces == nil || scope.RuntimeWorkspaceID == nil || scope.UserSubjectID == nil {
		return ErrNotFound
	}
	for _, raw := range fileIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil || id == uuid.Nil {
			return ErrInvalidRequest
		}
		allowed, err := runtimeWorkspaces.CanReadRuntimeExternalFile(ctx, *scope.RuntimeWorkspaceID, *scope.UserSubjectID, id.String())
		if err != nil {
			return err
		}
		if !allowed {
			return ErrNotFound
		}
	}
	return nil
}

// publishedAccessByID loads a DigitalHuman's published access by its ID.
func (s *AccessService) publishedAccessByID(ctx context.Context, digitalHumanID uuid.UUID) (*PublishedAccess, error) {
	var definition Definition
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ? AND deleted_at IS NULL", digitalHumanID, StatusActive).Take(&definition).Error; err != nil {
		return nil, ErrNotFound
	}
	return s.publishedAccess(ctx, definition.ID, definition.OrganizationID, definition.WorkspaceID)
}

// RuntimeWorkspaceHandler returns the external API operation a Runtime
// Workspace serves under the given name for a bound DigitalHuman, or nil.
func (h *ExternalHandler) RuntimeWorkspaceHandler(name string) gin.HandlerFunc {
	handlers := map[string]gin.HandlerFunc{
		"config": h.GetConfig, "chat": h.Chat,
		"runs.get": h.GetRun, "runs.events": h.StreamRunEvents, "runs.stop": h.StopRun, "runs.runtime_input": h.SubmitRunRuntimeInput,
		"conversations.list": h.ListConversations, "conversations.search": h.SearchConversations, "conversations.get": h.GetConversation,
		"conversations.update": h.UpdateConversation, "conversations.delete": h.DeleteConversation, "conversations.messages": h.ListConversationMessages,
		"conversations.file": h.GetAPIExecutionFile, "regenerate": h.RegenerateAPIMessage,
		"continue.user_input": h.ContinueAPIUserInput, "continue.tool_governance": h.ContinueAPIToolGovernance,
		"continue.token_budget": h.ContinueAPITaskTokenBudget, "continue.workflow": h.ContinueAPITaskApproval,
		"tasks.list": h.ListAPITasks, "tasks.create": h.CreateAPITask, "tasks.get": h.GetAPITask, "tasks.update": h.UpdateAPITask,
		"tasks.action": h.ApplyAPITaskAction, "tasks.runs": h.ListAPITaskRuns, "tasks.activities": h.ListAPITaskActivities,
		"tasks.schedule_preview": h.PreviewAPITaskSchedule,
		"memory.get":             h.GetAPIEndUserMemory, "memory.delete_item": h.DeleteAPIEndUserMemoryItem, "memory.clear": h.ClearAPIEndUserMemory,
	}
	if _, ok := h.tasks.(TaskReportService); ok {
		handlers["reports.list"], handlers["reports.get"] = h.ListAPITaskReports, h.GetAPITaskReport
		handlers["reports.review"], handlers["reports.file"] = h.ReviewAPITaskReport, h.GetAPITaskReportFile
		handlers["notifications.list"], handlers["notifications.read"] = h.ListAPINotifications, h.ReadAPINotifications
	}
	if _, ok := h.notifications.(EndUserNotificationCounts); ok {
		handlers["notifications.unread_count"], handlers["notifications.read_all"] = h.CountAPIUnreadNotifications, h.ReadAllAPINotifications
	}
	return handlers[name]
}

// applyRunEndUserBilling meters an external end user's run, so a Runtime
// Workspace end user's model calls count against their quota (ADR 0010).
func applyRunEndUserBilling(config *chatruntime.RunConfig, run *Run) {
	if config == nil || run == nil || run.UserSubjectID == nil || *run.UserSubjectID == uuid.Nil || run.UserSubjectKind != string(usersubject.APIEndUser) {
		return
	}
	config.BillingEndUserSubjectID = run.UserSubjectID.String()
}
