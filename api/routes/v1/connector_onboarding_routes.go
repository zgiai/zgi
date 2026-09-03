package v1

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"net/http"
)

type connectorOnboardingService interface {
	Start(context.Context, onboarding.StartCommand) (onboarding.Snapshot, error)
	Get(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (onboarding.Snapshot, error)
	Cancel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64) (onboarding.Snapshot, error)
}

func (h *connectorHandler) registerOnboarding(group *gin.RouterGroup) {
	group.POST("/connectors/:connectorId/onboarding-flows", h.startOnboarding)
	group.POST("/connector-connections/:connectionId/onboarding-flows", h.startOnboarding)
	group.GET("/connectors/onboarding-flows/:flowId", h.getOnboarding)
	group.DELETE("/connectors/onboarding-flows/:flowId", h.cancelOnboarding)
	group.POST("/connectors/onboarding-flows/:flowId/actions", h.onboardingAction)
}
func (h *connectorHandler) startAppCredentialSetup(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	d, ok := h.catalog.Get(c.Param("connectorId"))
	if !ok || !d.IsAppCredentials() {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	h.startKeySetup(c)
}
func (h *connectorHandler) startOnboarding(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var request struct {
		ProtocolVersion  int    `json:"protocol_version"`
		Intent           string `json:"intent"`
		Region           string `json:"region"`
		TemplateID       string `json:"template_id"`
		ExpectedRevision int    `json:"expected_revision"`
	}
	if err := decodeConnectorJSON(c, &request); err != nil {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	if (request.ProtocolVersion != 0 && request.ProtocolVersion != 1) ||
		(request.ProtocolVersion == 1 && (request.Region != "" || request.TemplateID != "" || request.Intent == "create_app" || request.Intent == "update_app")) ||
		(request.ProtocolVersion == 0 && request.Intent != "create_app" && request.Intent != "update_app") {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	command := onboarding.StartCommand{OrganizationID: org, OwnerAccountID: owner, ConnectorID: c.Param("connectorId"), Region: request.Region, TemplateID: request.TemplateID, ExpectedRevision: request.ExpectedRevision, IdempotencyKey: c.GetHeader("Idempotency-Key"), Intent: request.Intent, ProtocolVersion: request.ProtocolVersion}
	if raw := c.Param("connectionId"); raw != "" {
		id, err := parseConnectorUUID(raw)
		if err != nil || (request.Intent != "update_app" && request.Intent != "reauthorize") || request.Region != "" || request.ExpectedRevision < 1 {
			h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
			return
		}
		command.ConnectionID = id
	} else if (request.Intent != "create_app" && request.Intent != "register_bot" && request.Intent != "login_session") || request.ExpectedRevision != 0 {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := h.onboarding.Start(c.Request.Context(), command)
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusAccepted, result)
}

func (h *connectorHandler) messagingMethod(id, method string) bool {
	a := catalog.DefaultMessagingAvailability(id)
	if !a.Enabled {
		return false
	}
	for _, m := range a.Methods {
		if m == method {
			return true
		}
	}
	return false
}
func (h *connectorHandler) projectCatalog(d catalog.Definition, connections []core.PersonalConnection) connectorCatalogItemResponse {
	out := projectCatalogItem(d, connections)
	if !d.IsMessagingCredentials() {
		return out
	}
	a := catalog.DefaultMessagingAvailability(d.ID)
	out.SetupAvailability = &a
	return out
}
func (h *connectorHandler) startCredentialSetup(c *gin.Context) {
	d, ok := h.catalog.Get(c.Param("connectorId"))
	if !ok || !d.IsMessagingCredentials() {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	h.startKeySetup(c)
}
func (h *connectorHandler) onboardingAction(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		h.credentialError(c, err)
		return
	}
	var request struct {
		Action            string            `json:"action"`
		Revision          int64             `json:"expected_revision"`
		ChallengeRevision int64             `json:"challenge_revision"`
		Values            map[string]string `json:"values"`
	}
	defer func() { core.DestroySecretValues(request.Values) }()
	if decodeConnectorJSON(c, &request) != nil {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	service, ok := h.onboarding.(interface {
		Action(context.Context, onboarding.ActionCommand) (onboarding.Snapshot, error)
	})
	if !ok {
		h.credentialError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	result, err := service.Action(c.Request.Context(), onboarding.ActionCommand{OrganizationID: org, OwnerAccountID: owner, FlowID: id, Action: request.Action, ExpectedRevision: request.Revision, ChallengeRevision: request.ChallengeRevision, Values: request.Values})
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusAccepted, result)
}
func (h *connectorHandler) getOnboarding(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		h.credentialError(c, err)
		return
	}
	result, err := h.onboarding.Get(c.Request.Context(), org, owner, id)
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, result)
}
func (h *connectorHandler) cancelOnboarding(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		h.credentialError(c, err)
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if decodeConnectorJSON(c, &request) != nil || request.ExpectedRevision < 1 {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := h.onboarding.Cancel(c.Request.Context(), org, owner, id, request.ExpectedRevision)
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, result)
}
func (h *connectorHandler) credentialError(c *gin.Context, err error) {
	projectable, ok := core.ApplicationError(err)
	if !ok {
		connectorRouteError(c, err)
		return
	}
	if h.errorProjector == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInternal, false, nil))
		return
	}
	projected := h.errorProjector.Project(projectable, apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))).Presentation
	c.Header(apptransport.HeaderApplicationErrorCode, projected.Code.String())
	c.JSON(projected.HTTPStatus, gin.H{"code": string(core.ErrorCodeOf(err)), "message": projected.Message, "retryable": core.ErrorCodeOf(err) == core.ErrorCodeRateLimited})
}
