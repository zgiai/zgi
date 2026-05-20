package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/model"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/pkg/response"
)

// SubscriptionHandler handles plugin subscription API requests
type SubscriptionHandler struct {
	service        service.MemberSubscriptionService
	accountService interfaces.AccountService
}

// NewSubscriptionHandler creates a new subscription handler
func NewSubscriptionHandler(svc service.MemberSubscriptionService, accountService interfaces.AccountService) *SubscriptionHandler {
	return &SubscriptionHandler{service: svc, accountService: accountService}
}

// Error codes for subscription operations
var (
	ErrSubscriptionFailed   = response.ErrorCode{Code: 215010, Message: "Failed to manage subscription", UserVisible: true}
	ErrSubscriptionNotFound = response.ErrorCode{Code: 215011, Message: "Subscription not found", UserVisible: true}
	ErrOrgNotInContext      = response.ErrorCode{Code: 415002, Message: "Organization ID not found in context", UserVisible: true}
)

// Subscribe subscribes the current organization to a plugin
// POST /workspaces/current/plugin-runner/plugins/:id/subscribe
func (h *SubscriptionHandler) Subscribe(c *gin.Context) {
	groupID, err := getOrganizationIDFromContext(c, h.accountService)
	if err != nil {
		response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
		return
	}

	pluginID := c.Param("id")
	accountID := getAccountIDFromContext(c)
	isMember, err := ensureOrganizationMember(c, h.accountService, groupID, accountID)
	if err != nil {
		response.FailWithMessage(c, ErrSubscriptionFailed, err.Error())
		return
	}
	if !isMember {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	var req model.SubscribePluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Config is optional, so binding error is not fatal
		req.Config = ""
	}

	// Use pluginID (Marketplace ID) and let service resolve the installation
	sub, err := h.service.Subscribe(c.Request.Context(), groupID, accountID, "", pluginID, accountID, req.Config)
	if err != nil {
		response.FailWithMessage(c, ErrSubscriptionFailed, err.Error())
		return
	}

	response.Success(c, model.SubscriptionResponse{
		ID:           sub.ID,
		GroupID:      sub.GroupID,
		PluginID:     sub.PluginID,
		Enabled:      sub.Enabled,
		Config:       sub.Config,
		SubscribedBy: sub.SubscribedBy,
		CreatedAt:    sub.CreatedAt,
	})
}

// Unsubscribe removes the current organization's subscription to a plugin
// DELETE /workspaces/current/plugin-runner/plugins/:id/subscribe
func (h *SubscriptionHandler) Unsubscribe(c *gin.Context) {
	groupID, err := getOrganizationIDFromContext(c, h.accountService)
	if err != nil {
		response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
		return
	}

	pluginID := c.Param("id")
	accountID := getAccountIDFromContext(c)
	isMember, err := ensureOrganizationMember(c, h.accountService, groupID, accountID)
	if err != nil {
		response.FailWithMessage(c, ErrSubscriptionFailed, err.Error())
		return
	}
	if !isMember {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	if err := h.service.Unsubscribe(c.Request.Context(), groupID, accountID, pluginID); err != nil {
		response.FailWithMessage(c, ErrSubscriptionFailed, err.Error())
		return
	}

	response.Success(c, nil)
}

// ListSubscriptions lists all subscriptions for the current organization
// GET /workspaces/current/plugin-runner/plugins/subscribed
func (h *SubscriptionHandler) ListSubscriptions(c *gin.Context) {
	groupID, err := getOrganizationIDFromContext(c, h.accountService)
	if err != nil {
		response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
		return
	}

	accountID := getAccountIDFromContext(c)
	isMember, err := ensureOrganizationMember(c, h.accountService, groupID, accountID)
	if err != nil {
		response.FailWithMessage(c, ErrSubscriptionFailed, err.Error())
		return
	}
	if !isMember {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	subs, err := h.service.ListSubscribedPlugins(c.Request.Context(), groupID, accountID)
	if err != nil {
		response.FailWithMessage(c, ErrSubscriptionFailed, err.Error())
		return
	}

	result := make([]model.SubscriptionResponse, 0, len(subs))
	for _, s := range subs {
		result = append(result, model.SubscriptionResponse{
			ID:           s.ID,
			GroupID:      s.GroupID,
			PluginID:     s.PluginID,
			Enabled:      s.Enabled,
			Config:       s.Config,
			SubscribedBy: s.SubscribedBy,
			CreatedAt:    s.CreatedAt,
		})
	}

	response.Success(c, result)
}

// IsSubscribed checks if the current organization is subscribed to a plugin
// GET /workspaces/current/plugin-runner/plugins/:id/subscribed
func (h *SubscriptionHandler) IsSubscribed(c *gin.Context) {
	groupID, err := getOrganizationIDFromContext(c, h.accountService)
	if err != nil {
		response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
		return
	}

	pluginID := c.Param("id")
	accountID := getAccountIDFromContext(c)
	isMember, err := ensureOrganizationMember(c, h.accountService, groupID, accountID)
	if err != nil {
		response.FailWithMessage(c, ErrSubscriptionFailed, err.Error())
		return
	}
	if !isMember {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	subscribed, err := h.service.IsSubscribed(c.Request.Context(), groupID, accountID, pluginID)
	if err != nil {
		response.FailWithMessage(c, ErrSubscriptionFailed, err.Error())
		return
	}

	response.Success(c, gin.H{"subscribed": subscribed})
}

func (h *SubscriptionHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/plugins/subscribed", h.ListSubscriptions)
	router.GET("/plugins/:id/subscribed", h.IsSubscribed)
	router.POST("/plugins/:id/subscribe", h.Subscribe)
	router.DELETE("/plugins/:id/subscribe", h.Unsubscribe)
}
