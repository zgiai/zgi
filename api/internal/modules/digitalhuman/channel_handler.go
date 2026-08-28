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

const (
	maxChannelWebhookMetadataEntries = 64
	maxChannelWebhookMetadataBytes   = 32 << 10
)

// ChannelHandlerService is the authenticated channel configuration and public
// webhook application surface. Implementations must keep provider credentials
// request-scoped and out of returned models.
type ChannelHandlerService interface {
	CreateInstallation(context.Context, Scope, uuid.UUID, CreateChannelInstallationRequest) (*channelruntime.Installation, error)
	ListInstallations(context.Context, Scope, uuid.UUID) ([]*ChannelInstallationView, error)
	UpdateInstallation(context.Context, Scope, uuid.UUID, uuid.UUID, UpdateChannelInstallationRequest) (*channelruntime.Installation, error)
	UpdateInstallationCredential(context.Context, Scope, uuid.UUID, uuid.UUID, UpdateChannelInstallationCredentialRequest) (*channelruntime.Installation, error)
	VerifyInstallation(context.Context, Scope, uuid.UUID, uuid.UUID) (*ChannelInstallationVerification, error)
	StartConnectionTest(context.Context, Scope, uuid.UUID, uuid.UUID, StartChannelConnectionTestRequest) (*ChannelConnectionTest, error)
	GetConnectionTest(context.Context, Scope, uuid.UUID, uuid.UUID, uuid.UUID, channelruntime.ConversationType, time.Time) (*ChannelConnectionTest, error)
	DeleteInstallation(context.Context, Scope, uuid.UUID, uuid.UUID) error
	ListEvents(context.Context, Scope, uuid.UUID, ListQuery) (*ChannelEventListResult, error)
	ListDeliveries(context.Context, Scope, uuid.UUID, ListQuery) (*ChannelDeliveryListResult, error)
	RetryDelivery(context.Context, Scope, uuid.UUID, uuid.UUID) error
	ProbeWebhook(context.Context, string, uuid.UUID) error
	HandleWebhook(context.Context, string, uuid.UUID, channelruntime.WebhookRequest) (*ChannelWebhookResult, error)
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
	router.GET("/webhooks/channels/:provider/:installation_id", handler.ProbeChannelWebhook)
	router.POST("/webhooks/channels/:provider/:installation_id", handler.HandleChannelWebhook)
}

func (handler *ChannelHandler) RegisterAuthenticatedRoutes(router *gin.RouterGroup) {
	group := router.Group("/digital-humans/:id/channels")
	group.POST("/installations", handler.CreateInstallation)
	group.GET("/installations", handler.ListInstallations)
	group.PATCH("/installations/:installation_id", handler.UpdateInstallation)
	group.PUT("/installations/:installation_id/credential", handler.UpdateInstallationCredential)
	group.POST("/installations/:installation_id/verify", handler.VerifyInstallation)
	group.POST("/installations/:installation_id/connection-tests", handler.StartConnectionTest)
	group.GET("/installations/:installation_id/connection-tests/:connection_test_id", handler.GetConnectionTest)
	group.DELETE("/installations/:installation_id", handler.DeleteInstallation)
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

func (handler *ChannelHandler) UpdateInstallationCredential(c *gin.Context) {
	scope, digitalHumanID, installationID, ok := handler.scopeAndResourceID(c, "installation_id")
	if !ok {
		return
	}
	var request UpdateChannelInstallationCredentialRequest
	if !handler.base.decode(c, &request) {
		return
	}
	result, err := handler.service.UpdateInstallationCredential(c.Request.Context(), scope, digitalHumanID, installationID, request)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (handler *ChannelHandler) VerifyInstallation(c *gin.Context) {
	scope, digitalHumanID, installationID, ok := handler.scopeAndResourceID(c, "installation_id")
	if !ok {
		return
	}
	result, err := handler.service.VerifyInstallation(c.Request.Context(), scope, digitalHumanID, installationID)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (handler *ChannelHandler) StartConnectionTest(c *gin.Context) {
	scope, digitalHumanID, installationID, ok := handler.scopeAndResourceID(c, "installation_id")
	if !ok {
		return
	}
	var request StartChannelConnectionTestRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		handler.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.channel.connection_test.start"))
		return
	}
	result, err := handler.service.StartConnectionTest(c.Request.Context(), scope, digitalHumanID, installationID, request)
	if err != nil {
		handler.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (handler *ChannelHandler) GetConnectionTest(c *gin.Context) {
	scope, digitalHumanID, installationID, ok := handler.scopeAndResourceID(c, "installation_id")
	if !ok {
		return
	}
	testID, err := uuid.Parse(strings.TrimSpace(c.Param("connection_test_id")))
	if err != nil || testID == uuid.Nil {
		handler.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.channel.connection_test.get"))
		return
	}
	startedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(c.Query("started_at")))
	if err != nil {
		handler.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.channel.connection_test.get"))
		return
	}
	conversationType := channelruntime.ConversationType(strings.TrimSpace(c.Query("conversation_type")))
	result, err := handler.service.GetConnectionTest(
		c.Request.Context(), scope, digitalHumanID, installationID, testID, conversationType, startedAt,
	)
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
	if err := handler.service.DeleteInstallation(c.Request.Context(), scope, digitalHumanID, installationID); err != nil {
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

func (handler *ChannelHandler) HandleChannelWebhook(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	installationID, err := uuid.Parse(strings.TrimSpace(c.Param("installation_id")))
	if provider == "" || err != nil || installationID == uuid.Nil {
		writeChannelWebhookError(c, http.StatusUnauthorized, "channel.webhook.unauthorized", "Webhook authentication failed.")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChannelWebhookRequestBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeChannelWebhookError(c, http.StatusBadRequest, "channel.webhook.invalid", "Invalid webhook request.")
		return
	}
	headers, query, ok := boundedWebhookMetadata(c.Request.Header, c.Request.URL.Query())
	if !ok {
		writeChannelWebhookError(c, http.StatusBadRequest, "channel.webhook.invalid", "Invalid webhook request.")
		return
	}
	result, err := handler.service.HandleWebhook(c.Request.Context(), provider, installationID, channelruntime.WebhookRequest{
		Body: body, Headers: headers, Query: query, ReceivedAt: time.Now().UTC(),
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
	writeChannelWebhookResponse(c, result)
}

// ProbeChannelWebhook supports provider consoles that perform an unauthenticated
// reachability check before saving a robot message-receiving address. It never
// reads credentials or creates an inbound event.
func (handler *ChannelHandler) ProbeChannelWebhook(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	installationID, err := uuid.Parse(strings.TrimSpace(c.Param("installation_id")))
	if provider == "" || err != nil || installationID == uuid.Nil {
		writeChannelWebhookError(c, http.StatusUnauthorized, "channel.webhook.unauthorized", "Webhook authentication failed.")
		return
	}
	if err := handler.service.ProbeWebhook(c.Request.Context(), provider, installationID); err != nil {
		switch {
		case errors.Is(err, ErrChannelInstallationNotFound):
			writeChannelWebhookError(c, http.StatusUnauthorized, "channel.webhook.unauthorized", "Webhook authentication failed.")
		default:
			writeChannelWebhookError(c, http.StatusServiceUnavailable, "channel.webhook.unavailable", "Webhook processing is temporarily unavailable.")
		}
		return
	}
	// Some provider consoles treat an empty 200 response as an unsuccessful
	// reachability probe. Return an explicit acknowledgement while keeping the
	// endpoint read-only.
	c.String(http.StatusOK, "success")
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

func writeChannelWebhookResponse(c *gin.Context, result *ChannelWebhookResult) {
	if result == nil {
		c.Status(http.StatusOK)
		return
	}
	response := result.Response
	status := response.StatusCode
	if status < 100 || status > 599 {
		status = http.StatusOK
	}
	for key, values := range response.Headers {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	if len(response.Body) == 0 {
		c.Status(status)
		return
	}
	c.Status(status)
	_, _ = c.Writer.Write(response.Body)
}

func boundedWebhookMetadata(headers http.Header, query map[string][]string) (http.Header, map[string][]string, bool) {
	if len(headers)+len(query) > maxChannelWebhookMetadataEntries {
		return nil, nil, false
	}
	resultHeaders := make(http.Header, len(headers))
	resultQuery := make(map[string][]string, len(query))
	total := 0
	copyValues := func(target map[string][]string, key string, values []string) bool {
		total += len(key)
		cloned := make([]string, len(values))
		for index, value := range values {
			total += len(value)
			if total > maxChannelWebhookMetadataBytes {
				return false
			}
			cloned[index] = value
		}
		target[key] = cloned
		return total <= maxChannelWebhookMetadataBytes
	}
	for key, values := range headers {
		if !copyValues(resultHeaders, key, values) {
			return nil, nil, false
		}
	}
	for key, values := range query {
		if !copyValues(resultQuery, key, values) {
			return nil, nil, false
		}
	}
	return resultHeaders, resultQuery, true
}
