package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/commandrules"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
)

// RegisterConnectorCommandRuleRoutes registers the organization server command
// rules (ADR 0029). Members read them, so they know what their DigitalHumans
// may run; only administrators change them.
func RegisterConnectorCommandRuleRoutes(router *gin.RouterGroup, rules *commandrules.Service, accountService interfaces.AccountService) {
	if router == nil || rules == nil || accountService == nil {
		panic("connector command rule routes require the rule store and account services")
	}
	handler := &connectorCommandRuleHandler{rules: rules, admins: accountService}
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(accountService))
	group.GET("/connector-command-rules", handler.list)
	group.POST("/connector-command-rules", handler.create)
	group.DELETE("/connector-command-rules/:rule_id", handler.delete)
}

type connectorCommandRuleHandler struct {
	rules  *commandrules.Service
	admins interfaces.AccountService
}

type connectorCommandRuleRequest struct {
	Prefix string               `json:"prefix"`
	Action serverops.RuleAction `json:"action"`
	Note   string               `json:"note"`
}

func (handler *connectorCommandRuleHandler) list(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	rules, err := handler.rules.List(c.Request.Context(), organizationID)
	if err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInternal, true, err))
		return
	}
	admin, _ := handler.admins.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID.String(), accountID.String())
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": rules, "can_manage": admin}})
}

func (handler *connectorCommandRuleHandler) create(c *gin.Context) {
	organizationID, accountID, ok := handler.administrator(c)
	if !ok {
		return
	}
	var request connectorCommandRuleRequest
	if err := decodeConnectorJSON(c, &request); err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	rule, err := handler.rules.Create(c.Request.Context(), organizationID, accountID, request.Prefix, request.Action, request.Note)
	if errors.Is(err, commandrules.ErrInvalid) {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	if err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInternal, true, err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": rule})
}

func (handler *connectorCommandRuleHandler) delete(c *gin.Context) {
	organizationID, _, ok := handler.administrator(c)
	if !ok {
		return
	}
	ruleID, parseErr := uuid.Parse(c.Param("rule_id"))
	if parseErr != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	err := handler.rules.Delete(c.Request.Context(), organizationID, ruleID)
	if errors.Is(err, commandrules.ErrNotFound) {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil))
		return
	}
	if err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInternal, true, err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (handler *connectorCommandRuleHandler) administrator(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	admin, err := handler.admins.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID.String(), accountID.String())
	if err != nil || !admin {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil))
		return uuid.Nil, uuid.Nil, false
	}
	return organizationID, accountID, true
}
