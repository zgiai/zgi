package agents

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"github.com/zgiai/zgi/api/pkg/response"
)

type directMemoryUpdateRequest struct {
	Content          string `json:"content" binding:"required"`
	ExpectedRevision *int64 `json:"expected_revision,omitempty"`
}

type memoryAccessScope struct {
	workspaceID        uuid.UUID
	agentID            uuid.UUID
	userScope          string
	userID             uuid.UUID
	operationID        *uuid.UUID
	slots              []agentmemory.RuntimeSlot
	writeSlots         []agentmemory.RuntimeSlot
	writesEnabled      bool
	autoCaptureAllowed bool
	actorType          string
	sourceKind         string
	configScope        string
	configRevision     string
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

func (h *AgentsHandler) GetWebAppMemorySettings(c *gin.Context) {
	if access, ok := h.webAppMemoryAccess(c); ok {
		h.getDirectMemorySettings(c, access)
	}
}

func (h *AgentsHandler) PatchWebAppMemorySettings(c *gin.Context) {
	if access, ok := h.webAppMemoryAccess(c); ok {
		h.patchDirectMemorySettings(c, access)
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

func (h *AgentsHandler) GetAPIKeyAgentMemorySettings(c *gin.Context) {
	if access, ok := h.apiKeyMemoryAccess(c); ok {
		h.getDirectMemorySettings(c, access)
	}
}

func (h *AgentsHandler) PatchAPIKeyAgentMemorySettings(c *gin.Context) {
	if access, ok := h.apiKeyMemoryAccess(c); ok {
		h.patchDirectMemorySettings(c, access)
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
	privacyContractSlots := writeSlots
	if !runtimeCtx.RunConfig.AgentMemoryEnabled {
		privacyContractSlots = nil
	}
	privacySlots, err := h.memoryService().PrivacySlots(c.Request.Context(), *runtimeCtx.Scope.WorkspaceID, *runtimeCtx.Caller.ID, privacyContractSlots, agentmemory.UserScopeAccount, runtimeCtx.Scope.AccountID)
	if err != nil {
		h.failRuntime(c, err)
		return memoryAccessScope{}, false
	}
	return memoryAccessScope{
		workspaceID:        *runtimeCtx.Scope.WorkspaceID,
		agentID:            *runtimeCtx.Caller.ID,
		userScope:          agentmemory.UserScopeAccount,
		userID:             runtimeCtx.Scope.AccountID,
		slots:              privacySlots,
		writeSlots:         writeSlots,
		writesEnabled:      runtimeCtx.RunConfig.AgentMemoryEnabled,
		autoCaptureAllowed: runtimeCtx.RunConfig.AgentMemoryAutoExtractionEnabled,
		actorType:          agentmemory.EventActorUser,
		sourceKind:         agentmemory.SourceKindManager,
		configScope:        runtimeCtx.RunConfig.AgentMemoryConfigScope,
		configRevision:     runtimeCtx.RunConfig.AgentMemoryConfigRevision,
	}, true
}

func (h *AgentsHandler) apiKeyMemoryAccess(c *gin.Context) (memoryAccessScope, bool) {
	_, runtimeWorkspaceRequest := c.Get("runtime_workspace_principal")
	if runtimeWorkspaceRequest && !relationshipmemory.RuntimeEnabled() {
		response.Fail(c, response.ErrPermissionDenied)
		return memoryAccessScope{}, false
	}
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
	externalUser, ok := externalUserFromRequest(c, c.Query("user"))
	if !ok {
		return memoryAccessScope{}, false
	}
	if len([]rune(externalUser)) > 128 {
		response.FailWithMessage(c, response.ErrInvalidParam, "user must not exceed 128 characters")
		return memoryAccessScope{}, false
	}
	published, err := h.appService.GetPublishedAgentRuntimeConfig(c.Request.Context(), agentID.String())
	if err != nil {
		h.failRuntime(c, err)
		return memoryAccessScope{}, false
	}
	writeSlots := agentMemoryRuntimeStoreSlots(agentMemoryRuntimeSlots(published.Config.AgentMemorySlots))
	privacyContractSlots := writeSlots
	if !published.Config.AgentMemoryEnabled {
		privacyContractSlots = nil
	}
	externalUserID := DeriveExternalUserID(workspaceID, agentID, externalUser)
	if runtimeSubject := strings.TrimSpace(c.GetString("runtime_subject_id")); runtimeSubject != "" {
		parsedSubject, parseErr := uuid.Parse(runtimeSubject)
		if parseErr != nil || parsedSubject == uuid.Nil {
			response.Fail(c, response.ErrUnauthorized)
			return memoryAccessScope{}, false
		}
		externalUserID = parsedSubject
	}
	var operationID *uuid.UUID
	if runtimeWorkspaceRequest {
		if rawOperationID := strings.TrimSpace(c.GetHeader("Idempotency-Key")); rawOperationID != "" {
			parsedOperationID, parseErr := uuid.Parse(rawOperationID)
			if parseErr != nil || parsedOperationID == uuid.Nil {
				response.Fail(c, response.ErrInvalidParam)
				return memoryAccessScope{}, false
			}
			operationID = &parsedOperationID
		}
	}
	privacySlots, err := h.memoryService().PrivacySlots(c.Request.Context(), workspaceID, agentID, privacyContractSlots, agentmemory.UserScopeEndUser, externalUserID)
	if err != nil {
		h.failRuntime(c, err)
		return memoryAccessScope{}, false
	}
	return memoryAccessScope{
		workspaceID:        workspaceID,
		agentID:            agentID,
		userScope:          agentmemory.UserScopeEndUser,
		userID:             externalUserID,
		operationID:        operationID,
		slots:              privacySlots,
		writeSlots:         writeSlots,
		writesEnabled:      published.Config.AgentMemoryEnabled,
		autoCaptureAllowed: published.Config.AgentMemoryAutoExtractionEnabled,
		actorType:          agentmemory.EventActorOrganizer,
		sourceKind:         agentmemory.SourceKindManager,
		configScope:        agentmemory.ConfigScopePublished,
		configRevision:     published.Config.AgentMemoryConfigRevision,
	}, true
}

func agentMemoryRuntimeStoreSlots(slots []runtimeservice.AgentMemorySlotConfig) []agentmemory.RuntimeSlot {
	out := make([]agentmemory.RuntimeSlot, 0, len(slots))
	for _, slot := range slots {
		out = append(out, agentmemory.RuntimeSlot{Key: slot.Key, Name: slot.Name, Description: slot.Description, MaxChars: slot.MaxChars, Enabled: slot.Enabled, SortOrder: slot.SortOrder})
	}
	return out
}

type agentUserMemoryService interface {
	PrivacySlots(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID) ([]agentmemory.RuntimeSlot, error)
	ExportUserMemory(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID) (*agentmemory.MemoryExportResponse, error)
	UpdateValue(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID, agentmemory.UpdateValueRequest, agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error)
	ClearValue(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID, string, agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error)
	ClearAllValues(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, agentmemory.MutationMetadata) error
	UndoAutomaticOperation(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot) (*agentmemory.UndoResponse, error)
}

func (h *AgentsHandler) memoryService() agentUserMemoryService {
	if !relationshipmemory.RuntimeEnabled() {
		return agentmemory.NewService(h.db)
	}
	return relationshipmemory.NewAgentAdapter(h.db)
}

func (access memoryAccessScope) mutationMetadata() agentmemory.MutationMetadata {
	return agentmemory.MutationMetadata{
		ActorType:      access.actorType,
		Source:         agentmemory.EventSourceAPI,
		SourceKind:     access.sourceKind,
		OperationID:    access.operationID,
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
	if req.ExpectedRevision == nil {
		var valid bool
		req.ExpectedRevision, valid = ifMatchRevision(c.GetHeader("If-Match"))
		if !valid {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
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
	} else {
		var valid bool
		expected, valid = ifMatchRevision(c.GetHeader("If-Match"))
		if !valid {
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

func ifMatchRevision(value string) (*int64, bool) {
	value = strings.Trim(strings.TrimSpace(value), "\"")
	if value == "" || value == "*" {
		return nil, true
	}
	revision, err := strconv.ParseInt(value, 10, 64)
	if err != nil || revision < 0 {
		return nil, false
	}
	return &revision, true
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

type directMemorySettingsRequest struct {
	MemoryEnabled      *bool `json:"memory_enabled"`
	AutoCaptureEnabled *bool `json:"auto_capture_enabled"`
	ExpectedRevision   int64 `json:"expected_revision" binding:"required"`
}

func (h *AgentsHandler) getDirectMemorySettings(c *gin.Context, access memoryAccessScope) {
	if !relationshipmemory.RuntimeEnabled() {
		response.FailWithMessage(c, response.ErrInvalidParam, "User relationship memory is not enabled")
		return
	}
	settings, err := relationshipmemory.NewAgentAdapter(h.db).GetSettings(c.Request.Context(), access.workspaceID, access.agentID, access.userScope, access.userID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *AgentsHandler) patchDirectMemorySettings(c *gin.Context, access memoryAccessScope) {
	if !relationshipmemory.RuntimeEnabled() {
		response.FailWithMessage(c, response.ErrInvalidParam, "User relationship memory is not enabled")
		return
	}
	var req directMemorySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if req.AutoCaptureEnabled != nil && *req.AutoCaptureEnabled && !access.autoCaptureAllowed {
		response.FailWithMessage(c, response.ErrInvalidParam, "automatic capture is not allowed by the published Agent configuration")
		return
	}
	if req.MemoryEnabled != nil && *req.MemoryEnabled && !access.writesEnabled {
		response.FailWithMessage(c, response.ErrInvalidParam, "relationship memory is not enabled by the published Agent configuration")
		return
	}
	settings, err := relationshipmemory.NewAgentAdapter(h.db).UpdateSettings(c.Request.Context(), access.workspaceID, access.agentID, access.userScope, access.userID, req.MemoryEnabled, req.AutoCaptureEnabled, req.ExpectedRevision)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, settings)
}
