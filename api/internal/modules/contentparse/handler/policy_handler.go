package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"github.com/zgiai/ginext/internal/modules/contentparse/service"
	"github.com/zgiai/ginext/pkg/response"
)

type PolicyHandler struct {
	service service.PolicyAdminService
}

func NewPolicyHandler(service service.PolicyAdminService) *PolicyHandler {
	return &PolicyHandler{service: service}
}

func (h *PolicyHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/policies", h.ListPolicies)
	rg.GET("/policies/:id", h.GetPolicy)
	rg.POST("/policies", h.CreatePolicy)
	rg.PUT("/policies/:id", h.UpdatePolicy)
	rg.DELETE("/policies/:id", h.DeletePolicy)

	rg.GET("/policies/:id/rules", h.ListRules)
	rg.POST("/policies/:id/rules", h.CreateRule)
	rg.PUT("/policy-rules/:rule_id", h.UpdateRule)
	rg.DELETE("/policy-rules/:rule_id", h.DeleteRule)
}

func (h *PolicyHandler) ListPolicies(c *gin.Context) {
	scope := defaultString(c.Query("scope"), "system")
	workspaceID, err := parseOptionalUUID(c.Query("workspace_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid workspace_id")
		return
	}
	items, err := h.service.ListPoliciesByScope(c.Request.Context(), scope, workspaceID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, items)
}

func (h *PolicyHandler) GetPolicy(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid policy id")
		return
	}
	item, err := h.service.GetPolicyByID(c.Request.Context(), id)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "policy not found"})
		return
	}
	response.Success(c, item)
}

func (h *PolicyHandler) CreatePolicy(c *gin.Context) {
	var item model.RoutePolicy
	if err := c.ShouldBindJSON(&item); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if err := h.service.CreatePolicy(c.Request.Context(), &item); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, item)
}

func (h *PolicyHandler) UpdatePolicy(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid policy id")
		return
	}
	var item model.RoutePolicy
	if err := c.ShouldBindJSON(&item); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	item.ID = id
	if err := h.service.UpdatePolicy(c.Request.Context(), &item); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, item)
}

func (h *PolicyHandler) DeletePolicy(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid policy id")
		return
	}
	if err := h.service.DeletePolicy(c.Request.Context(), id); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *PolicyHandler) ListRules(c *gin.Context) {
	policyID, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid policy id")
		return
	}
	items, err := h.service.ListRulesByPolicyID(c.Request.Context(), policyID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, items)
}

func (h *PolicyHandler) CreateRule(c *gin.Context) {
	policyID, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid policy id")
		return
	}
	var item model.RoutePolicyRule
	if err := c.ShouldBindJSON(&item); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	item.PolicyID = policyID
	if err := h.service.CreateRule(c.Request.Context(), &item); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, item)
}

func (h *PolicyHandler) UpdateRule(c *gin.Context) {
	ruleID, err := parseRequiredUUID(c.Param("rule_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid rule id")
		return
	}
	var item model.RoutePolicyRule
	if err := c.ShouldBindJSON(&item); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	item.ID = ruleID
	if err := h.service.UpdateRule(c.Request.Context(), &item); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, item)
}

func (h *PolicyHandler) DeleteRule(c *gin.Context) {
	ruleID, err := parseRequiredUUID(c.Param("rule_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid rule id")
		return
	}
	if err := h.service.DeleteRule(c.Request.Context(), ruleID); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"id": ruleID})
}
