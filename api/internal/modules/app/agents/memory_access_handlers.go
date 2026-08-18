package agents

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/pkg/response"
)

type directMemoryUpdateRequest struct {
	Content          string `json:"content" binding:"required"`
	ExpectedRevision *int64 `json:"expected_revision,omitempty"`
}

type memoryAccessScope struct {
	workspaceID    uuid.UUID
	agentID        uuid.UUID
	userScope      string
	userID         uuid.UUID
	slots          []agentmemory.RuntimeSlot
	writeSlots     []agentmemory.RuntimeSlot
	writesEnabled  bool
	actorType      string
	sourceKind     string
	configScope    string
	configRevision string
}

func (h *AgentsHandler) GetWebAppMemory(c *gin.Context) {
	if access, ok := h.webAppMemoryAccess(c); ok {
		h.getDirectMemory(c, access)
	}
}
func (h *AgentsHandler) PutWebAppMemory(c *gin.Context) {
	if access, ok := h.webAppMemoryAccess(c); ok {
		h.putDirectMemory(c, access)
	}
}
func (h *AgentsHandler) DeleteWebAppMemoryValue(c *gin.Context) {
	if access, ok := h.webAppMemoryAccess(c); ok {
		h.deleteDirectMemoryValue(c, access)
	}
}
func (h *AgentsHandler) DeleteAllWebAppMemory(c *gin.Context) {
	if access, ok := h.webAppMemoryAccess(c); ok {
		h.deleteAllDirectMemory(c, access)
	}
}
func (h *AgentsHandler) ExportWebAppMemory(c *gin.Context) {
	if access, ok := h.webAppMemoryAccess(c); ok {
		h.exportDirectMemory(c, access)
	}
}
func (h *AgentsHandler) UndoWebAppMemoryOperation(c *gin.Context) {
	if access, ok := h.webAppMemoryAccess(c); ok {
		h.undoDirectMemory(c, access)
	}
}

func (h *AgentsHandler) GetAPIKeyAgentMemory(c *gin.Context) {
	if access, ok := h.apiKeyMemoryAccess(c); ok {
		h.getDirectMemory(c, access)
	}
}
func (h *AgentsHandler) PutAPIKeyAgentMemory(c *gin.Context) {
	if access, ok := h.apiKeyMemoryAccess(c); ok {
		h.putDirectMemory(c, access)
	}
}
func (h *AgentsHandler) DeleteAPIKeyAgentMemoryValue(c *gin.Context) {
	if access, ok := h.apiKeyMemoryAccess(c); ok {
		h.deleteDirectMemoryValue(c, access)
	}
}
func (h *AgentsHandler) DeleteAllAPIKeyAgentMemory(c *gin.Context) {
	if access, ok := h.apiKeyMemoryAccess(c); ok {
		h.deleteAllDirectMemory(c, access)
	}
}
func (h *AgentsHandler) ExportAPIKeyAgentMemory(c *gin.Context) {
	if access, ok := h.apiKeyMemoryAccess(c); ok {
		h.exportDirectMemory(c, access)
	}
}
func (h *AgentsHandler) UndoAPIKeyAgentMemoryOperation(c *gin.Context) {
	if access, ok := h.apiKeyMemoryAccess(c); ok {
		h.undoDirectMemory(c, access)
	}
}

func (h *AgentsHandler) webAppMemoryAccess(c *gin.Context) (memoryAccessScope, bool) {
	if !c.GetBool("is_authenticated") {
		response.Fail(c, response.ErrUnauthorized)
		return memoryAccessScope{}, false
	}
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return memoryAccessScope{}, false
	}
	if runtimeCtx.Scope.WorkspaceID == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return memoryAccessScope{}, false
	}
	if runtimeCtx.Caller.ID == nil {
		response.Fail(c, response.ErrInvalidParam)
		return memoryAccessScope{}, false
	}
	writeSlots := agentMemoryRuntimeStoreSlots(runtimeCtx.RunConfig.AgentMemorySlots)
	privacySlots, err := h.memoryService().PrivacySlots(c.Request.Context(), *runtimeCtx.Scope.WorkspaceID, *runtimeCtx.Caller.ID, writeSlots, agentmemory.UserScopeAccount, runtimeCtx.Scope.AccountID)
	if err != nil {
		h.failRuntime(c, err)
		return memoryAccessScope{}, false
	}
	return memoryAccessScope{
		workspaceID:    *runtimeCtx.Scope.WorkspaceID,
		agentID:        *runtimeCtx.Caller.ID,
		userScope:      agentmemory.UserScopeAccount,
		userID:         runtimeCtx.Scope.AccountID,
		slots:          privacySlots,
		writeSlots:     writeSlots,
		writesEnabled:  runtimeCtx.RunConfig.AgentMemoryEnabled,
		actorType:      agentmemory.EventActorUser,
		sourceKind:     agentmemory.SourceKindExplicit,
		configScope:    runtimeCtx.RunConfig.AgentMemoryConfigScope,
		configRevision: runtimeCtx.RunConfig.AgentMemoryConfigRevision,
	}, true
}

func (h *AgentsHandler) apiKeyMemoryAccess(c *gin.Context) (memoryAccessScope, bool) {
	agentID, err := uuid.Parse(strings.TrimSpace(c.GetString("agent_id")))
	if err != nil || agentID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParam)
		return memoryAccessScope{}, false
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(c.GetString("workspace_id")))
	if err != nil || workspaceID == uuid.Nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return memoryAccessScope{}, false
	}
	externalUser := strings.TrimSpace(c.Query("user"))
	if externalUser == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, externalAgentMemoryUserRequiredMessage)
		return memoryAccessScope{}, false
	}
	published, err := h.appService.GetPublishedAgentRuntimeConfig(c.Request.Context(), agentID.String())
	if err != nil {
		h.failRuntime(c, err)
		return memoryAccessScope{}, false
	}
	writeSlots := agentMemoryRuntimeStoreSlots(agentMemoryRuntimeSlots(published.Config.AgentMemorySlots))
	privacySlots, err := h.memoryService().PrivacySlots(c.Request.Context(), workspaceID, agentID, writeSlots, agentmemory.UserScopeEndUser, externalAgentMemoryUserID(workspaceID, agentID, externalUser))
	if err != nil {
		h.failRuntime(c, err)
		return memoryAccessScope{}, false
	}
	return memoryAccessScope{
		workspaceID:    workspaceID,
		agentID:        agentID,
		userScope:      agentmemory.UserScopeEndUser,
		userID:         externalAgentMemoryUserID(workspaceID, agentID, externalUser),
		slots:          privacySlots,
		writeSlots:     writeSlots,
		writesEnabled:  published.Config.AgentMemoryEnabled,
		actorType:      agentmemory.EventActorOrganizer,
		sourceKind:     agentmemory.SourceKindManager,
		configScope:    agentmemory.ConfigScopePublished,
		configRevision: published.Config.AgentMemoryConfigRevision,
	}, true
}

func agentMemoryRuntimeStoreSlots(slots []runtimeservice.AgentMemorySlotConfig) []agentmemory.RuntimeSlot {
	out := make([]agentmemory.RuntimeSlot, 0, len(slots))
	for _, slot := range slots {
		out = append(out, agentmemory.RuntimeSlot{Key: slot.Key, Name: slot.Name, Description: slot.Description, MaxChars: slot.MaxChars, Enabled: slot.Enabled, SortOrder: slot.SortOrder})
	}
	return out
}

func (h *AgentsHandler) memoryService() *agentmemory.Service { return agentmemory.NewService(h.db) }

func (access memoryAccessScope) mutationMetadata() agentmemory.MutationMetadata {
	return agentmemory.MutationMetadata{
		ActorType:      access.actorType,
		Source:         agentmemory.EventSourceAPI,
		SourceKind:     access.sourceKind,
		ConfigScope:    access.configScope,
		ConfigRevision: access.configRevision,
	}
}

func (h *AgentsHandler) getDirectMemory(c *gin.Context, access memoryAccessScope) {
	exported, err := h.memoryService().ExportUserMemory(c.Request.Context(), access.workspaceID, access.agentID, access.slots, access.userScope, access.userID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, exported)
}
func (h *AgentsHandler) exportDirectMemory(c *gin.Context, access memoryAccessScope) {
	c.Header("Content-Disposition", `attachment; filename="agent-memory.json"`)
	h.getDirectMemory(c, access)
}
func (h *AgentsHandler) putDirectMemory(c *gin.Context, access memoryAccessScope) {
	if !access.writesEnabled {
		response.FailWithMessage(c, response.ErrInvalidParam, "Agent Memory is disabled")
		return
	}
	var req directMemoryUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	key := strings.TrimSpace(c.Param("key"))
	value, err := h.memoryService().UpdateValue(c.Request.Context(), access.workspaceID, access.agentID, access.writeSlots, access.userScope, access.userID, agentmemory.UpdateValueRequest{Key: key, Content: req.Content, ExpectedRevision: req.ExpectedRevision}, access.mutationMetadata())
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, value)
}
func (h *AgentsHandler) deleteDirectMemoryValue(c *gin.Context, access memoryAccessScope) {
	var expected *int64
	if raw := strings.TrimSpace(c.Query("expected_revision")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			expected = &parsed
		} else {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}
	metadata := access.mutationMetadata()
	metadata.ExpectedRevision = expected
	value, err := h.memoryService().ClearValue(c.Request.Context(), access.workspaceID, access.agentID, access.slots, access.userScope, access.userID, c.Param("key"), metadata)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, value)
}
func (h *AgentsHandler) deleteAllDirectMemory(c *gin.Context, access memoryAccessScope) {
	err := h.memoryService().ClearAllValues(c.Request.Context(), access.workspaceID, access.agentID, access.userScope, access.userID, access.mutationMetadata())
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
func (h *AgentsHandler) undoDirectMemory(c *gin.Context, access memoryAccessScope) {
	operationID, err := uuid.Parse(strings.TrimSpace(c.Param("operation_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	result, err := h.memoryService().UndoAutomaticOperation(c.Request.Context(), access.workspaceID, access.agentID, access.userScope, access.userID, operationID, access.slots)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, result)
}
