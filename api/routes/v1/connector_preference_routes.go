package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	connectorpreferences "github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/apperror"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type connectorChatPreferenceService interface {
	Get(context.Context, uuid.UUID, uuid.UUID) (connectorpreferences.Snapshot, error)
	Update(context.Context, connectorpreferences.UpdateCommand) (connectorpreferences.Snapshot, error)
}

type connectorPreferenceRequest struct {
	Mode             connectorpreferences.Mode `json:"mode"`
	ConnectionID     string                    `json:"connection_id,omitempty"`
	ExpectedRevision *int64                    `json:"expected_revision"`
}

func (handler *connectorHandler) preferenceActor(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	organizationID, orgErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	ownerID, ownerErr := uuid.Parse(strings.TrimSpace(middleware.GetAccountID(c)))
	if orgErr != nil || ownerErr != nil || organizationID == uuid.Nil || ownerID == uuid.Nil {
		handler.preferenceError(c, apperror.New(connectorpreferences.AppCodeAccessDenied))
		return uuid.Nil, uuid.Nil, false
	}
	return organizationID, ownerID, true
}

func (handler *connectorHandler) getChatPreferences(c *gin.Context) {
	organizationID, ownerID, ok := handler.preferenceActor(c)
	if !ok {
		return
	}
	snapshot, err := handler.preferences.Get(c.Request.Context(), organizationID, ownerID)
	if err != nil {
		handler.preferenceError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, snapshot)
}

func (handler *connectorHandler) updateChatPreference(c *gin.Context) {
	organizationID, ownerID, ok := handler.preferenceActor(c)
	if !ok {
		return
	}
	var request connectorPreferenceRequest
	if err := decodeConnectorJSON(c, &request); err != nil || request.ExpectedRevision == nil {
		handler.preferenceError(c, apperror.New(connectorpreferences.AppCodeInvalidInput))
		return
	}
	snapshot, err := handler.preferences.Update(c.Request.Context(), connectorpreferences.UpdateCommand{
		OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: c.Param("connectorId"),
		Selection:        connectorpreferences.Selection{Mode: request.Mode, ConnectionID: request.ConnectionID},
		ExpectedRevision: *request.ExpectedRevision, IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
	if err != nil {
		handler.preferenceError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, snapshot)
}

func (handler *connectorHandler) preferenceError(c *gin.Context, err error) {
	if _, ok := apperror.As(err); !ok {
		err = apperror.Wrap(err, connectorpreferences.AppCodeInternal, apperror.WithOperation("connector.preference.http"))
	}
	if handler.errorProjector == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": connectorpreferences.AppCodeInternal.String(), "message": "Unable to load or save new-chat Connector defaults.", "retryable": false})
		return
	}
	projected := handler.errorProjector.Project(err, apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))).Presentation
	c.Header(apptransport.HeaderApplicationErrorCode, projected.Code.String())
	c.JSON(projected.HTTPStatus, gin.H{"code": projected.Code.String(), "message": projected.Message, "retryable": projected.Retryable})
}
