package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/credential/dto"
	"github.com/zgiai/ginext/internal/modules/llm/credential/service"
	"github.com/zgiai/ginext/pkg/response"
)

type TenantCredentialHandler struct {
	service service.TenantCredentialService
}

func NewTenantCredentialHandler(svc service.TenantCredentialService) *TenantCredentialHandler {
	return &TenantCredentialHandler{service: svc}
}

func (h *TenantCredentialHandler) Create(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.CreateTenantCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	credential, err := h.service.Create(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, credential)
}

func (h *TenantCredentialHandler) Get(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid credential id")
		return
	}
	credential, err := h.service.GetByID(c.Request.Context(), organizationID, id)
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}
	response.Success(c, credential)
}

func (h *TenantCredentialHandler) List(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ListCredentialRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	credentials, total, err := h.service.List(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": credentials, "total": total, "page": req.Page, "page_size": req.PageSize})
}

func (h *TenantCredentialHandler) Update(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid credential id")
		return
	}
	var req dto.UpdateTenantCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	credential, err := h.service.Update(c.Request.Context(), organizationID, id, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, credential)
}

func (h *TenantCredentialHandler) Delete(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid credential id")
		return
	}
	if err := h.service.Delete(c.Request.Context(), organizationID, id); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, nil)
}

// Test tests a tenant credential by making a simple API call
func (h *TenantCredentialHandler) Test(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid credential id")
		return
	}
	var req dto.TestCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "model is required")
		return
	}
	result, err := h.service.TestCredential(c.Request.Context(), organizationID, id, req.Model, "")
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}
	response.Success(c, result)
}

func getOrganizationID(c *gin.Context) (uuid.UUID, error) {
	tenantIDStr := c.GetString("organization_id")
	if tenantIDStr == "" {
		tenantIDStr = c.GetHeader("X-Organization-ID")
	}
	if tenantIDStr == "" {
		return uuid.Nil, service.ErrInvalidAPIKey
	}
	return uuid.Parse(tenantIDStr)
}
