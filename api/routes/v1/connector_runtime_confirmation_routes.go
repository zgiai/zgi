package v1

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
)

// ConnectorRuntimeConfirmationService is the transport-neutral continuation
// boundary used by the shared Chat, Agent, and Workflow confirmation surface.
type ConnectorRuntimeConfirmationService interface {
	Continue(context.Context, connectorconfirmation.Command) (connectorconfirmation.Result, error)
	ReadStatus(context.Context, connectorconfirmation.StatusQuery) (connectorconfirmation.StatusResult, error)
}

// ConnectorRuntimeConfirmationResolutionRecorder durably projects the safe
// terminal into the host identified by the frozen server-side origin.
type ConnectorRuntimeConfirmationResolutionRecorder interface {
	RecordConnectorConfirmationResolution(
		context.Context,
		connectorconfirmation.Command,
		connectorconfirmation.Result,
	) error
}

type ConnectorRuntimeConfirmationRouteDeps struct {
	Service        ConnectorRuntimeConfirmationService
	Recorder       ConnectorRuntimeConfirmationResolutionRecorder
	AccountService interfaces.AccountService
}

// RegisterConnectorRuntimeConfirmationRoutes registers authenticated one-shot
// continuation and read-only status endpoints. It does not expose Connector policy or
// accept a client-supplied provider operation payload.
func RegisterConnectorRuntimeConfirmationRoutes(
	router *gin.RouterGroup,
	deps ConnectorRuntimeConfirmationRouteDeps,
) {
	if router == nil || deps.Service == nil || deps.Recorder == nil || deps.AccountService == nil {
		panic("connector runtime confirmation routes require confirmation, recorder, and account services")
	}
	handler := &connectorRuntimeConfirmationHandler{service: deps.Service, recorder: deps.Recorder}
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	handler.register(group)
}

type connectorRuntimeConfirmationHandler struct {
	service  ConnectorRuntimeConfirmationService
	recorder ConnectorRuntimeConfirmationResolutionRecorder
}

func (handler *connectorRuntimeConfirmationHandler) register(group *gin.RouterGroup) {
	group.POST("/connectors/runtime/confirmations", handler.continueConfirmation)
	group.GET("/connectors/runtime/confirmations/:receipt", handler.readStatus)
}

func (handler *connectorRuntimeConfirmationHandler) readStatus(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	receipt := c.Param("receipt")
	receiptID, err := uuid.Parse(receipt)
	if err != nil || receiptID == uuid.Nil || receiptID.String() != receipt || c.Request.URL.RawQuery != "" {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := handler.service.ReadStatus(c.Request.Context(), connectorconfirmation.StatusQuery{OrganizationID: organizationID, OwnerAccountID: accountID, RouteReceipt: connectorexecution.RouteClaimReceipt(receipt)})
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	connectorSuccess(c, http.StatusOK, result)
}

type connectorRuntimeConfirmationRequest struct {
	RouteReceipt string                                  `json:"route_receipt"`
	Decision     connectorexecution.ConfirmationDecision `json:"decision"`
}

type connectorRuntimeConfirmationResponse struct {
	State      connectorconfirmation.State `json:"state"`
	DecisionID *uuid.UUID                  `json:"decision_id,omitempty"`
	ErrorCode  connectorcore.ErrorCode     `json:"error_code,omitempty"`
}

func (handler *connectorRuntimeConfirmationHandler) continueConfirmation(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	var request connectorRuntimeConfirmationRequest
	if err := decodeConnectorJSON(c, &request); err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	receiptID, receiptErr := uuid.Parse(request.RouteReceipt)
	if receiptErr != nil || receiptID == uuid.Nil || receiptID.String() != request.RouteReceipt ||
		(request.Decision != connectorexecution.ConfirmationDecisionConfirmed &&
			request.Decision != connectorexecution.ConfirmationDecisionCancelled) {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	command := connectorconfirmation.Command{
		OrganizationID: organizationID,
		OwnerAccountID: accountID,
		RouteReceipt:   connectorexecution.RouteClaimReceipt(request.RouteReceipt),
		Decision:       request.Decision,
	}
	result, err := handler.service.Continue(c.Request.Context(), command)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	resolution := result
	resolution.ExecutionResult = nil
	if err := handler.recorder.RecordConnectorConfirmationResolution(
		c.Request.Context(), command, resolution,
	); err != nil {
		connectorRouteError(c, err)
		return
	}
	projected := connectorRuntimeConfirmationResponse{
		State: result.State, ErrorCode: result.ErrorCode,
	}
	if result.DecisionID != uuid.Nil {
		decisionID := result.DecisionID
		projected.DecisionID = &decisionID
	}
	connectorSuccess(c, http.StatusOK, projected)
}
