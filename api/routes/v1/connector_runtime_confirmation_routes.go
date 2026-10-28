package v1

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// ConnectorRuntimeConfirmationService is the transport-neutral continuation
// boundary used by the shared Chat, Agent, and Workflow confirmation surface.
type ConnectorRuntimeConfirmationService interface {
	Continue(context.Context, connectorconfirmation.Command) (connectorconfirmation.Result, error)
	ReadStatus(context.Context, connectorconfirmation.StatusQuery) (connectorconfirmation.StatusResult, error)
	ListPending(context.Context, uuid.UUID, uuid.UUID, int, int) (connectorconfirmation.PendingResult, error)
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

// ConnectorRuntimeConfirmationContinuer resumes the DigitalHuman turn a
// decided confirmation paused (ADR 0024). It records the resolution itself.
type ConnectorRuntimeConfirmationContinuer interface {
	ResumeAfterConnectorConfirmation(
		context.Context,
		connectorconfirmation.Command,
		connectorconfirmation.Result,
	) error
}

type ConnectorRuntimeConfirmationRouteDeps struct {
	Service        ConnectorRuntimeConfirmationService
	Recorder       ConnectorRuntimeConfirmationResolutionRecorder
	DigitalHuman   ConnectorRuntimeConfirmationContinuer
	AccountService interfaces.AccountService
}

// RegisterConnectorRuntimeConfirmationRoutes registers authenticated one-shot
// continuation and read-only status endpoints. It does not expose Connector policy or
// accept a client-supplied provider operation payload. remember_for_session is
// only a user-selected governance flag; the server builds the grant from the
// frozen operation.
func RegisterConnectorRuntimeConfirmationRoutes(
	router *gin.RouterGroup,
	deps ConnectorRuntimeConfirmationRouteDeps,
) {
	if router == nil || deps.Service == nil || deps.Recorder == nil || deps.AccountService == nil {
		panic("connector runtime confirmation routes require confirmation, recorder, and account services")
	}
	handler := &connectorRuntimeConfirmationHandler{service: deps.Service, recorder: deps.Recorder, digitalHuman: deps.DigitalHuman}
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	handler.register(group)
}

type connectorRuntimeConfirmationHandler struct {
	service      ConnectorRuntimeConfirmationService
	recorder     ConnectorRuntimeConfirmationResolutionRecorder
	digitalHuman ConnectorRuntimeConfirmationContinuer
}

func (handler *connectorRuntimeConfirmationHandler) register(group *gin.RouterGroup) {
	group.POST("/connectors/runtime/confirmations", handler.continueConfirmation)
	group.GET("/connectors/runtime/confirmations", handler.listPending)
	group.GET("/connectors/runtime/confirmations/:receipt", handler.readStatus)
}

func (handler *connectorRuntimeConfirmationHandler) listPending(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	page, pageSize := 1, 20
	if err == nil {
		for key, value := range values {
			if len(value) != 1 || key != "page" && key != "page_size" {
				err = connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
				break
			}
			parsed, parseErr := strconv.Atoi(value[0])
			if parseErr != nil {
				err = parseErr
				break
			}
			if key == "page" {
				page = parsed
			} else {
				pageSize = parsed
			}
		}
	}
	if err != nil || page < 1 || page > 1000 || pageSize < 1 || pageSize > 100 {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := handler.service.ListPending(c.Request.Context(), organizationID, accountID, page, pageSize)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	connectorSuccess(c, http.StatusOK, result)
}

func (handler *connectorRuntimeConfirmationHandler) readStatus(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	receipt := c.Param("receipt")
	receiptID, err := uuid.Parse(receipt)
	query, queryErr := url.ParseQuery(c.Request.URL.RawQuery)
	version := 0
	if len(query) == 1 && len(query["presentation_version"]) == 1 && query.Get("presentation_version") == "1" {
		version = 1
	}
	if err != nil || receiptID == uuid.Nil || receiptID.String() != receipt || queryErr != nil || (c.Request.URL.RawQuery != "" && version != 1) {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := handler.service.ReadStatus(c.Request.Context(), connectorconfirmation.StatusQuery{OrganizationID: organizationID, OwnerAccountID: accountID, RouteReceipt: connectorexecution.RouteClaimReceipt(receipt), PresentationVersion: version})
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	connectorSuccess(c, http.StatusOK, result)
}

type connectorRuntimeConfirmationRequest struct {
	RouteReceipt       string                                  `json:"route_receipt"`
	Decision           connectorexecution.ConfirmationDecision `json:"decision"`
	RememberForSession bool                                    `json:"remember_for_session,omitempty"`
}

type connectorRuntimeConfirmationResponse struct {
	State              connectorconfirmation.State `json:"state"`
	DecisionID         *uuid.UUID                  `json:"decision_id,omitempty"`
	ErrorCode          connectorcore.ErrorCode     `json:"error_code,omitempty"`
	RememberForSession bool                        `json:"remember_for_session,omitempty"`
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
		OrganizationID:     organizationID,
		OwnerAccountID:     accountID,
		RouteReceipt:       connectorexecution.RouteClaimReceipt(request.RouteReceipt),
		Decision:           request.Decision,
		RememberForSession: request.RememberForSession && request.Decision == connectorexecution.ConfirmationDecisionConfirmed,
	}
	// The person has decided: the operation and its record must not depend on
	// the browser keeping the request open. Execution keeps its own deadline.
	ctx := context.WithoutCancel(c.Request.Context())
	// The confirmed call's operation card is labelled in the person's language,
	// as the call itself would have been inside the run.
	if locale := confirmationPresentationLocale(c); locale != "" {
		ctx = connectorexecution.WithPresentationLocale(ctx, locale)
	}
	result, err := handler.service.Continue(ctx, command)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	if err := handler.recordResolution(ctx, command, result); err != nil {
		connectorRouteError(c, err)
		return
	}
	projected := connectorRuntimeConfirmationResponse{
		State: result.State, ErrorCode: result.ErrorCode,
		RememberForSession: command.RememberForSession,
	}
	if result.DecisionID != uuid.Nil {
		decisionID := result.DecisionID
		projected.DecisionID = &decisionID
	}
	connectorSuccess(c, http.StatusOK, projected)
}

// recordResolution projects the decision into its host. A DigitalHuman turn
// continues with the operation's result; every other host only records the
// safe terminal, so the provider value never reaches it.
func (handler *connectorRuntimeConfirmationHandler) recordResolution(
	ctx context.Context,
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) error {
	if result.Origin.Surface == connectorcore.TargetTypeDigitalHuman && handler.digitalHuman != nil {
		return handler.digitalHuman.ResumeAfterConnectorConfirmation(ctx, command, result)
	}
	resolution := result
	resolution.ExecutionResult = nil
	return handler.recorder.RecordConnectorConfirmationResolution(ctx, command, resolution)
}

// confirmationPresentationLocale prefers the console's own language cookie,
// which follows the language the person chose in the UI, over the browser's
// Accept-Language.
func confirmationPresentationLocale(c *gin.Context) string {
	if value, err := c.Cookie("locale"); err == nil {
		if locale, ok := appcatalog.ParseLocale(value); ok {
			return string(locale)
		}
	}
	return string(apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language")))
}
