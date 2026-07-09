package handler

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/pkg/response"
)

// APIKeyHandler handles HTTP requests for API key operations
type APIKeyHandler struct {
	apiKeyService     service.APIKeyService
	tenantAccountRepo workspace_repo.WorkspaceMemberRepository
	accountService    interfaces.AccountService
	tenantService     interfaces.WorkspaceManagementService
	enterpriseService interfaces.OrganizationService
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(
	apiKeyService service.APIKeyService,
	tenantAccountRepo workspace_repo.WorkspaceMemberRepository,
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	enterpriseService interfaces.OrganizationService,
) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService:     apiKeyService,
		tenantAccountRepo: tenantAccountRepo,
		accountService:    accountService,
		tenantService:     tenantService,
		enterpriseService: enterpriseService,
	}
}

func (h *APIKeyHandler) currentAPIKeyOrganizationScope(c *gin.Context) ([]string, bool) {
	if _, exists := c.Get("account_id"); !exists {
		response.Fail(c, response.ErrUnauthorized)
		return nil, false
	}

	organizationIDValue, exists := c.Get("organization_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return nil, false
	}

	organizationID, ok := organizationIDValue.(string)
	if !ok || organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return nil, false
	}

	return h.expandAPIKeyOrganizationScope(c, organizationID), true
}

func (h *APIKeyHandler) expandAPIKeyOrganizationScope(c *gin.Context, organizationID string) []string {
	ids := []string{organizationID}
	if h.enterpriseService == nil || h.tenantService == nil {
		return uniqueStrings(ids)
	}

	groupID := ""
	if group, err := h.enterpriseService.GetOrganizationByWorkspaceID(c.Request.Context(), organizationID); err == nil && group != nil {
		groupID = group.ID
	} else if group, err := h.enterpriseService.GetOrganizationByID(c.Request.Context(), organizationID); err == nil && group != nil {
		groupID = group.ID
	}

	if groupID == "" {
		return uniqueStrings(ids)
	}

	workspaceIDs, err := h.tenantService.GetWorkspaceIDsByOrganizationID(c.Request.Context(), groupID)
	if err != nil || len(workspaceIDs) == 0 {
		return uniqueStrings(ids)
	}

	ids = append(ids, workspaceIDs...)
	ids = append(ids, groupID)
	return uniqueStrings(ids)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// CreateAPIKey creates a new API key
// @Summary Create API key
// @Description Create a new API key for accessing LLM services
// @Tags LLM API Keys
// @Accept json
// @Produce json
// @Param request body dto.CreateAPIKeyRequest true "API key creation request"
// @Success 200 {object} response.Response{data=dto.CreateAPIKeyResponse}
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /llm/api-keys [post]
func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	accountID, exists := c.Get("account_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	// Set tenant_id from context if not provided in request
	accountIDStr := accountID.(string)
	if req.OrganizationID == nil || *req.OrganizationID == "" {
		tenantIDStr := organizationID.(string)
		req.OrganizationID = &tenantIDStr
	} else {
		// If tenant_id is provided in request and different from context, verify permission
		contextOrganizationID := organizationID.(string)
		if *req.OrganizationID != contextOrganizationID {
			currentGroup, err := h.enterpriseService.GetOrganizationByWorkspaceID(c.Request.Context(), contextOrganizationID)
			var currentGroupID string
			if err == nil && currentGroup != nil {
				currentGroupID = currentGroup.ID
			} else {
				group, err := h.enterpriseService.GetOrganizationByID(c.Request.Context(), contextOrganizationID)
				if err == nil && group != nil {
					currentGroupID = group.ID
				}
			}

			if currentGroupID != "" {
				targetGroup, err := h.enterpriseService.GetOrganizationByWorkspaceID(c.Request.Context(), *req.OrganizationID)
				if err == nil && targetGroup != nil && currentGroupID == targetGroup.ID {
				} else {
					hasAccess, err := h.tenantAccountRepo.IsMemberInWorkspace(c.Request.Context(), accountIDStr, *req.OrganizationID)
					if err != nil {
						response.FailWithMessage(c, response.ErrSystemError, "failed to verify tenant access")
						return
					}
					if !hasAccess {
						response.FailWithMessage(c, response.ErrUnauthorized, "you don't have access to this tenant")
						return
					}
				}
			} else {
				hasAccess, err := h.tenantAccountRepo.IsMemberInWorkspace(c.Request.Context(), accountIDStr, *req.OrganizationID)
				if err != nil {
					response.FailWithMessage(c, response.ErrSystemError, "failed to verify tenant access")
					return
				}
				if !hasAccess {
					response.FailWithMessage(c, response.ErrUnauthorized, "you don't have access to this tenant")
					return
				}
			}
		}
	}

	result, err := h.apiKeyService.CreateAPIKey(c.Request.Context(), &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// GetAPIKey gets an API key by ID
// @Summary Get API key
// @Description Get API key details by ID
// @Tags LLM API Keys
// @Accept json
// @Produce json
// @Param id path string true "API key ID"
// @Success 200 {object} response.Response{data=dto.APIKeyResponse}
// @Failure 401 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /llm/api-keys/{id} [get]
func (h *APIKeyHandler) GetAPIKey(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "API key ID is required")
		return
	}

	organizationIDs, ok := h.currentAPIKeyOrganizationScope(c)
	if !ok {
		return
	}

	result, err := h.apiKeyService.GetAPIKey(c.Request.Context(), id, organizationIDs)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// ListAPIKeys lists API keys with filters and pagination
// @Summary List API keys
// @Description List API keys with filters and pagination
// @Tags LLM API Keys
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Param status query string false "Status filter (active, inactive, revoked)"
// @Param search query string false "Search by name"
// @Success 200 {object} response.Response{data=dto.ListAPIKeyResponse}
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /llm/api-keys [get]
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	var req dto.ListAPIKeyRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	organizationIDs, ok := h.currentAPIKeyOrganizationScope(c)
	if !ok {
		return
	}

	if req.OrganizationID == nil || *req.OrganizationID == "" {
		req.OrganizationIDs = organizationIDs
	} else if !containsString(organizationIDs, *req.OrganizationID) {
		response.FailWithMessage(c, response.ErrUnauthorized, "you don't have access to this tenant")
		return
	}

	result, err := h.apiKeyService.ListAPIKeys(c.Request.Context(), &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// UpdateAPIKey updates an API key
// @Summary Update API key
// @Description Update API key details
// @Tags LLM API Keys
// @Accept json
// @Produce json
// @Param id path string true "API key ID"
// @Param request body dto.UpdateAPIKeyRequest true "API key update request"
// @Success 200 {object} response.Response{data=dto.APIKeyResponse}
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /llm/api-keys/{id} [put]
func (h *APIKeyHandler) UpdateAPIKey(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "API key ID is required")
		return
	}

	var req dto.UpdateAPIKeyRequest
	if err := decodeUpdateAPIKeyRequest(c.Request.Body, &req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	organizationIDs, ok := h.currentAPIKeyOrganizationScope(c)
	if !ok {
		return
	}

	result, err := h.apiKeyService.UpdateAPIKey(c.Request.Context(), id, organizationIDs, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

func decodeUpdateAPIKeyRequest(body io.Reader, req *dto.UpdateAPIKeyRequest) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, req); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	req.ClearQuotaLimit = bytes.Equal(bytes.TrimSpace(raw["quota_limit"]), []byte("null"))
	req.ClearExpiresAt = bytes.Equal(bytes.TrimSpace(raw["expires_at"]), []byte("null"))
	return nil
}

// DeleteAPIKey deletes an API key
// @Summary Delete API key
// @Description Delete an API key (soft delete)
// @Tags LLM API Keys
// @Accept json
// @Produce json
// @Param id path string true "API key ID"
// @Success 200 {object} response.Response{data=dto.DeleteAPIKeyResponse}
// @Failure 401 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /llm/api-keys/{id} [delete]
func (h *APIKeyHandler) DeleteAPIKey(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "API key ID is required")
		return
	}

	organizationIDs, ok := h.currentAPIKeyOrganizationScope(c)
	if !ok {
		return
	}

	result, err := h.apiKeyService.DeleteAPIKey(c.Request.Context(), id, organizationIDs)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// ValidateAPIKey validates an API key
// @Summary Validate API key
// @Description Validate if an API key is valid and active
// @Tags LLM API Keys
// @Accept json
// @Produce json
// @Param request body dto.ValidateAPIKeyRequest true "API key validation request"
// @Success 200 {object} response.Response{data=dto.ValidateAPIKeyResponse}
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /llm/api-keys/validate [post]
func (h *APIKeyHandler) ValidateAPIKey(c *gin.Context) {
	var req dto.ValidateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	result, err := h.apiKeyService.ValidateAPIKey(c.Request.Context(), req.Key)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}
