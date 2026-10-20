package v1

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/socialmcp"
	"github.com/zgiai/zgi/api/internal/modules/connectors/socialevents"
)

type connectorSocialEventService interface {
	VerifyChallenge(string, string, string) (string, error)
	ReceiveFacebook(context.Context, string, []byte) error
	List(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, int) (socialevents.Page, error)
	Acknowledge(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64) error
}

func (h *connectorHandler) registerSocialWebhook(router *gin.RouterGroup) {
	router.GET("/connectors/social-webhooks/facebook", h.verifySocialWebhook)
	router.POST("/connectors/social-webhooks/facebook", h.receiveSocialWebhook)
}
func (h *connectorHandler) registerSocialCapabilities(group *gin.RouterGroup) {
	group.POST("/connectors/social-media/prepare-image", h.prepareSocialImage)
	group.GET("/connector-connections/:connectionId/social-events", h.listSocialEvents)
	group.POST("/connector-connections/:connectionId/social-events/ack", h.ackSocialEvents)
}
func (h *connectorHandler) verifySocialWebhook(c *gin.Context) {
	socialOAuthHeaders(c)
	if h.socialEvents == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	query, err := c.Request.URL.Query(), error(nil)
	for _, key := range []string{"hub.mode", "hub.verify_token", "hub.challenge"} {
		if len(query[key]) != 1 {
			err = core.NewError(core.ErrorCodeInvalidInput, false, nil)
		}
	}
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	challenge, err := h.socialEvents.VerifyChallenge(query.Get("hub.mode"), query.Get("hub.verify_token"), query.Get("hub.challenge"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	c.String(http.StatusOK, "%s", challenge)
}
func (h *connectorHandler) receiveSocialWebhook(c *gin.Context) {
	socialOAuthHeaders(c)
	if h.socialEvents == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	raw, err := io.ReadAll(c.Request.Body)
	defer clear(raw)
	if err != nil || len(raw) == 0 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	if err = h.socialEvents.ReceiveFacebook(c.Request.Context(), c.GetHeader("X-Hub-Signature-256"), raw); err != nil {
		connectorRouteError(c, err)
		return
	}
	c.String(http.StatusOK, "EVENT_RECEIVED")
}
func (h *connectorHandler) listSocialEvents(c *gin.Context) {
	socialOAuthHeaders(c)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	if h.socialEvents == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	id, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	after, e1 := strconv.ParseInt(c.DefaultQuery("after_id", "0"), 10, 64)
	limit, e2 := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if e1 != nil || e2 != nil || after < 0 || limit < 1 || limit > 100 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	page, err := h.socialEvents.List(c.Request.Context(), org, owner, id, after, limit)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, page)
}
func (h *connectorHandler) ackSocialEvents(c *gin.Context) {
	socialOAuthHeaders(c)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	if h.socialEvents == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	id, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var input struct {
		ThroughID int64 `json:"through_id"`
	}
	if decodeConnectorJSON(c, &input) != nil || input.ThroughID < 1 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	if err = h.socialEvents.Acknowledge(c.Request.Context(), org, owner, id, input.ThroughID); err != nil {
		connectorRouteError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, gin.H{"acknowledged_through_id": input.ThroughID})
}
func (h *connectorHandler) prepareSocialImage(c *gin.Context) {
	socialOAuthHeaders(c)
	if _, _, ok := connectorActor(c); !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
	raw, err := io.ReadAll(c.Request.Body)
	defer clear(raw)
	if err != nil || len(raw) == 0 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	prepared, err := socialmcp.PrepareImage(raw)
	if err != nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	connectorSuccess(c, http.StatusOK, prepared)
}
