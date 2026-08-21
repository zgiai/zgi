package digitalhuman

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

const maxChannelWebhookRequestBytes int64 = 1 << 20

// ChannelHandlerService is the authenticated channel configuration and public
// webhook application surface. Implementations must keep provider credentials
// request-scoped and out of returned models.
type ChannelHandlerService interface {
	CreateInstallation(context.Context, Scope, uuid.UUID, CreateChannelInstallationRequest) (*channelruntime.Installation, error)
	ListInstallations(context.Context, Scope, uuid.UUID) ([]*ChannelInstallationView, error)
	UpdateInstallation(context.Context, Scope, uuid.UUID, uuid.UUID, UpdateChannelInstallationRequest) (*channelruntime.Installation, error)
	DeleteInstallation(context.Context, Scope, uuid.UUID, uuid.UUID, DeleteChannelInstallationRequest) error
	CreateBinding(context.Context, Scope, uuid.UUID, CreateChannelBindingRequest) (*ChannelBindingView, error)
	ListBindings(context.Context, Scope, uuid.UUID) ([]*ChannelBindingView, error)
	ListDiscoveries(context.Context, Scope, uuid.UUID) ([]*ChannelConversationDiscovery, error)
	DeleteBinding(context.Context, Scope, uuid.UUID, uuid.UUID) error
	ListEvents(context.Context, Scope, uuid.UUID, ListQuery) (*ChannelEventListResult, error)
	ListDeliveries(context.Context, Scope, uuid.UUID, ListQuery) (*ChannelDeliveryListResult, error)
	RetryDelivery(context.Context, Scope, uuid.UUID, uuid.UUID) error
	HandleWebhook(context.Context, uuid.UUID, channelruntime.WebhookRequest) (*ChannelWebhookResult, error)
}

type ChannelHandler struct {
	service ChannelHandlerService
	base    *Handler
}

func NewChannelHandler(service ChannelHandlerService, projector *apptransport.Projector) *ChannelHandler {
	if service == nil {
		panic("DigitalHuman channel handler requires service")
	}
	if projector == nil {
		panic("DigitalHuman channel handler requires application error projector")
	}
	return &ChannelHandler{service: service, base: &Handler{projector: projector}}
}

// RegisterPublicRoutes intentionally runs before authentication middleware.
// The installation UUID is only an address; verification always uses the
// request-scoped secret referenced by the installation connection.
func (handler *ChannelHandler) RegisterPublicRoutes(router *gin.RouterGroup) {
	router.POST("/webhooks/channels/feishu/:installation_id", handler.HandleFeishuWebhook)
}

func (handler *ChannelHandler) RegisterAuthenticatedRoutes(router *gin.RouterGroup) {
	group := router.Group("/digital-humans/:id/channels")
	group.POST("/installations", handler.CreateInstallation)
	group.GET("/installations", handler.ListInstallations)
	group.PATCH("/installations/:installation_id", handler.UpdateInstallation)
	group.DELETE("/installations/:installation_id", handler.DeleteInstallation)
	group.POST("/bindings", handler.CreateBinding)
	group.GET("/bindings", handler.ListBindings)
	group.GET("/discoveries", handler.ListDiscoveries)
	group.DELETE("/bindings/:binding_id", handler.DeleteBinding)
	group.GET("/events", handler.ListEvents)
	group.GET("/deliveries", handler.ListDeliveries)
	group.POST("/deliveries/:delivery_id/retry", handler.RetryDelivery)
}

func (handler *ChannelHandler) CreateInstallation(c *gin.Context) {
	scope, digitalHumanID, ok := handler.base.scopeAndID(c)
	if !ok {
		return
	}
	var request CreateChannelInstallationRequest
	if !handler.base.decode(c, &request) {
		return
	}
	result, err := handler.service.CreateInstallation(c.Request.Context(), scope, digitalHumanID, request)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, result)
}

func (handler *ChannelHandler) ListInstallations(c *gin.Context) {
	scope, digitalHumanID, ok := handler.base.scopeAndID(c)
	if !ok {
		return
	}
	result, err := handler.service.ListInstallations(c.Request.Context(), scope, digitalHumanID)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (handler *ChannelHandler) UpdateInstallation(c *gin.Context) {
	scope, digitalHumanID, installationID, ok := handler.scopeAndResourceID(c, "installation_id")
	if !ok {
		return
	}
	var request UpdateChannelInstallationRequest
	if !handler.base.decode(c, &request) {
		return
	}
	result, err := handler.service.UpdateInstallation(c.Request.Context(), scope, digitalHumanID, installationID, request)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (handler *ChannelHandler) DeleteInstallation(c *gin.Context) {
	scope, digitalHumanID, installationID, ok := handler.scopeAndResourceID(c, "installation_id")
	if !ok {
		return
	}
	request := DeleteChannelInstallationRequest{
		RemoveBindings: strings.EqualFold(strings.TrimSpace(c.Query("remove_bindings")), "true"),
	}
	if err := handler.service.DeleteInstallation(c.Request.Context(), scope, digitalHumanID, installationID, request); err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"deleted": true})
}

func (handler *ChannelHandler) CreateBinding(c *gin.Context) {
	scope, digitalHumanID, ok := handler.base.scopeAndID(c)
	if !ok {
		return
	}
	var request CreateChannelBindingRequest
	if !handler.base.decode(c, &request) {
		return
	}
	result, err := handler.service.CreateBinding(c.Request.Context(), scope, digitalHumanID, request)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, result)
}

func (handler *ChannelHandler) ListBindings(c *gin.Context) {
	scope, digitalHumanID, ok := handler.base.scopeAndID(c)
	if !ok {
		return
	}
	result, err := handler.service.ListBindings(c.Request.Context(), scope, digitalHumanID)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (handler *ChannelHandler) ListDiscoveries(c *gin.Context) {
	scope, digitalHumanID, ok := handler.base.scopeAndID(c)
	if !ok {
		return
	}
	result, err := handler.service.ListDiscoveries(c.Request.Context(), scope, digitalHumanID)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (handler *ChannelHandler) DeleteBinding(c *gin.Context) {
	scope, digitalHumanID, bindingID, ok := handler.scopeAndResourceID(c, "binding_id")
	if !ok {
		return
	}
	if err := handler.service.DeleteBinding(c.Request.Context(), scope, digitalHumanID, bindingID); err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"deleted": true})
}

func (handler *ChannelHandler) ListEvents(c *gin.Context) {
	handler.list(c, func(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query ListQuery) (any, error) {
		return handler.service.ListEvents(ctx, scope, digitalHumanID, query)
	})
}

func (handler *ChannelHandler) ListDeliveries(c *gin.Context) {
	handler.list(c, func(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query ListQuery) (any, error) {
		return handler.service.ListDeliveries(ctx, scope, digitalHumanID, query)
	})
}

func (handler *ChannelHandler) RetryDelivery(c *gin.Context) {
	scope, digitalHumanID, deliveryID, ok := handler.scopeAndResourceID(c, "delivery_id")
	if !ok {
		return
	}
	if err := handler.service.RetryDelivery(c.Request.Context(), scope, digitalHumanID, deliveryID); err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusAccepted, gin.H{"queued": true})
}

func (handler *ChannelHandler) HandleFeishuWebhook(c *gin.Context) {
	installationID, err := uuid.Parse(strings.TrimSpace(c.Param("installation_id")))
	if err != nil || installationID == uuid.Nil {
		writeChannelWebhookError(c, http.StatusUnauthorized, "channel.webhook.unauthorized", "Webhook authentication failed.")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChannelWebhookRequestBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeChannelWebhookError(c, http.StatusBadRequest, "channel.webhook.invalid", "Invalid webhook request.")
		return
	}
	result, err := handler.service.HandleWebhook(c.Request.Context(), installationID, channelruntime.WebhookRequest{
		Body: body, Timestamp: c.GetHeader("X-Lark-Request-Timestamp"), Nonce: c.GetHeader("X-Lark-Request-Nonce"),
		Signature: c.GetHeader("X-Lark-Signature"), ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		switch {
		case errors.Is(err, channelruntime.ErrWebhookUnauthorized), errors.Is(err, ErrChannelInstallationNotFound):
			writeChannelWebhookError(c, http.StatusUnauthorized, "channel.webhook.unauthorized", "Webhook authentication failed.")
		case errors.Is(err, channelruntime.ErrInvalidWebhook), errors.Is(err, channelruntime.ErrUnsupportedEvent):
			writeChannelWebhookError(c, http.StatusBadRequest, "channel.webhook.invalid", "Invalid webhook request.")
		default:
			writeChannelWebhookError(c, http.StatusServiceUnavailable, "channel.webhook.unavailable", "Webhook processing is temporarily unavailable.")
		}
		return
	}
	if result != nil && result.Challenge != "" {
		c.JSON(http.StatusOK, gin.H{"challenge": result.Challenge})
		return
	}
	// Provider-facing acknowledgements intentionally omit internal event/run IDs.
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

func (handler *ChannelHandler) list(c *gin.Context, action func(context.Context, Scope, uuid.UUID, ListQuery) (any, error)) {
	scope, digitalHumanID, ok := handler.base.scopeAndID(c)
	if !ok {
		return
	}
	query, err := parseListQuery(c)
	if err != nil {
		handler.base.writeError(c, applicationError(err, "digital_human.channel.list"))
		return
	}
	result, err := action(c.Request.Context(), scope, digitalHumanID, query)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (handler *ChannelHandler) scopeAndResourceID(c *gin.Context, parameter string) (Scope, uuid.UUID, uuid.UUID, bool) {
	scope, digitalHumanID, ok := handler.base.scopeAndID(c)
	if !ok {
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	resourceID, err := uuid.Parse(strings.TrimSpace(c.Param(parameter)))
	if err != nil || resourceID == uuid.Nil {
		handler.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.channel.request"))
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	return scope, digitalHumanID, resourceID, true
}

func writeChannelWebhookError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}
