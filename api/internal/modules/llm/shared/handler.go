package shared

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/ginext/pkg/response"
)

// BaseHandler provides common handler utilities for code reuse
type BaseHandler struct{}

// ParseUUID parses UUID from path parameter
func (h *BaseHandler) ParseUUID(c *gin.Context, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid "+param)
		return uuid.Nil, false
	}
	return id, true
}

// GetOrganizationID extracts organization ID from context or header.
func (h *BaseHandler) GetOrganizationID(c *gin.Context) (uuid.UUID, bool) {
	tenantIDStr := c.GetString("organization_id")
	if tenantIDStr == "" {
		tenantIDStr = c.GetHeader("X-Organization-ID")
	}
	if tenantIDStr == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "tenant id required")
		return uuid.Nil, false
	}
	id, err := uuid.Parse(tenantIDStr)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return uuid.Nil, false
	}
	return id, true
}

// GetTenantID is kept as a compatibility alias while organization-scoped handlers migrate.
func (h *BaseHandler) GetTenantID(c *gin.Context) (uuid.UUID, bool) {
	return h.GetOrganizationID(c)
}

// BindJSON binds JSON request body
func (h *BaseHandler) BindJSON(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return false
	}
	return true
}

// BindQuery binds query parameters
func (h *BaseHandler) BindQuery(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindQuery(req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return false
	}
	return true
}

// Success sends success response
func (h *BaseHandler) Success(c *gin.Context, data interface{}) {
	response.Success(c, data)
}

// SuccessList sends paginated success response
func (h *BaseHandler) SuccessList(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	response.Success(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Error sends error response
func (h *BaseHandler) Error(c *gin.Context, err error) {
	response.FailWithMessage(c, response.ErrSystemError, err.Error())
}

// NotFound sends not found response
func (h *BaseHandler) NotFound(c *gin.Context, err error) {
	response.FailWithMessage(c, response.ErrNotFound, err.Error())
}

// InvalidParam sends invalid parameter response
func (h *BaseHandler) InvalidParam(c *gin.Context, msg string) {
	response.FailWithMessage(c, response.ErrInvalidParam, msg)
}

// PaginationParams holds common pagination parameters
type PaginationParams struct {
	Page     int `form:"page,default=1"`
	PageSize int `form:"page_size,default=20"`
}

// Normalize ensures pagination params have valid values
func (p *PaginationParams) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
}

// Offset returns the offset for database query
func (p *PaginationParams) Offset() int {
	return (p.Page - 1) * p.PageSize
}
