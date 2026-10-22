package inbox

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// OwnerResolver decides whose channels a request manages. It writes its own
// error response and returns false when the caller may not manage them.
type OwnerResolver func(c *gin.Context) (ChannelOwner, bool)

// ChannelHandler manages notification channels for one kind of owner.
type ChannelHandler struct {
	channels   *Channels
	dispatcher *Dispatcher
	projector  *apptransport.Projector
}

// NewChannelHandler builds a channel handler.
func NewChannelHandler(channels *Channels, dispatcher *Dispatcher, projector *apptransport.Projector) *ChannelHandler {
	if channels == nil || dispatcher == nil || projector == nil {
		panic("notification channel handler requires channels, dispatcher, and projector")
	}
	return &ChannelHandler{channels: channels, dispatcher: dispatcher, projector: projector}
}

// RegisterRoutes serves the channels of the owner resolve returns under path.
func (h *ChannelHandler) RegisterRoutes(router *gin.RouterGroup, path string, resolve OwnerResolver) {
	group := router.Group(path)
	group.GET("", h.handle(resolve, h.list))
	group.POST("", h.handle(resolve, h.create))
	group.PATCH("/:channel_id", h.handle(resolve, h.update))
	group.DELETE("/:channel_id", h.handle(resolve, h.delete))
	group.POST("/:channel_id/verify", h.handle(resolve, h.verify))
	group.POST("/:channel_id/resend-code", h.handle(resolve, h.resendCode))
	group.POST("/:channel_id/rotate-secret", h.handle(resolve, h.rotateSecret))
	group.POST("/:channel_id/test", h.handle(resolve, h.test))
	group.GET("/:channel_id/deliveries", h.handle(resolve, h.deliveries))
	group.POST("/:channel_id/deliveries/:delivery_id/retry", h.handle(resolve, h.retryDelivery))
}

// RegisterChoiceRoute lists the active channels of the owner resolve
// returns, without their targets, for someone choosing where a task reports.
func (h *ChannelHandler) RegisterChoiceRoute(router *gin.RouterGroup, path string, resolve OwnerResolver) {
	router.GET(path, h.handle(resolve, h.choices))
}

// channelChoice is a channel as someone choosing it sees it.
type channelChoice struct {
	ID     uuid.UUID `json:"id"`
	Type   string    `json:"type"`
	Name   string    `json:"name"`
	Events []string  `json:"events"`
}

func (h *ChannelHandler) choices(c *gin.Context, owner ChannelOwner) {
	channels, err := h.channels.List(c.Request.Context(), owner)
	if err != nil {
		h.writeError(c, channelError(err, "inbox.channel.list"))
		return
	}
	items := make([]channelChoice, 0, len(channels))
	for _, channel := range channels {
		if channel.Status == ChannelActive {
			items = append(items, channelChoice{ID: channel.ID, Type: channel.Type, Name: channel.Name, Events: channel.Events})
		}
	}
	writeData(c, gin.H{"items": items})
}

// AccountOwner resolves the signed-in account in its current organization.
func (h *ChannelHandler) AccountOwner(c *gin.Context) (ChannelOwner, bool) {
	organizationID, orgErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	accountID, accountErr := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	if orgErr != nil || accountErr != nil {
		h.writeError(c, apperror.New(appcatalog.CodeAuthenticationRequired, apperror.WithOperation("inbox.channel.owner")))
		return ChannelOwner{}, false
	}
	return ChannelOwner{OrganizationID: organizationID, Kind: OwnerAccount, ID: accountID}, true
}

func (h *ChannelHandler) handle(resolve OwnerResolver, next func(*gin.Context, ChannelOwner)) gin.HandlerFunc {
	return func(c *gin.Context) {
		owner, ok := resolve(c)
		if !ok {
			return
		}
		next(c, owner)
	}
}

// channelView adds the one-time webhook secret to a created channel.
type channelView struct {
	*Channel
	Secret string `json:"secret,omitempty"`
}

func (h *ChannelHandler) list(c *gin.Context, owner ChannelOwner) {
	channels, err := h.channels.List(c.Request.Context(), owner)
	if err != nil {
		h.writeError(c, channelError(err, "inbox.channel.list"))
		return
	}
	writeData(c, gin.H{"items": channels})
}

func (h *ChannelHandler) create(c *gin.Context, owner ChannelOwner) {
	var input ChannelInput
	if err := c.ShouldBindJSON(&input); err != nil {
		h.writeError(c, channelError(ErrInvalidRequest, "inbox.channel.create"))
		return
	}
	actor, _ := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
	channel, secret, err := h.channels.Create(c.Request.Context(), owner, actor, input)
	if err != nil && channel == nil {
		h.writeError(c, channelError(err, "inbox.channel.create"))
		return
	}
	// A pending email channel exists even if its first code failed to send;
	// the owner can request another.
	c.JSON(http.StatusCreated, gin.H{"code": "0", "message": "success", "data": channelView{Channel: channel, Secret: secret}})
}

func (h *ChannelHandler) update(c *gin.Context, owner ChannelOwner) {
	id, ok := h.channelID(c)
	if !ok {
		return
	}
	var update ChannelUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		h.writeError(c, channelError(ErrInvalidRequest, "inbox.channel.update"))
		return
	}
	channel, err := h.channels.Update(c.Request.Context(), owner, id, update)
	if err != nil {
		h.writeError(c, channelError(err, "inbox.channel.update"))
		return
	}
	writeData(c, channel)
}

func (h *ChannelHandler) delete(c *gin.Context, owner ChannelOwner) {
	id, ok := h.channelID(c)
	if !ok {
		return
	}
	if err := h.channels.Delete(c.Request.Context(), owner, id); err != nil {
		h.writeError(c, channelError(err, "inbox.channel.delete"))
		return
	}
	writeData(c, gin.H{"deleted": true})
}

func (h *ChannelHandler) verify(c *gin.Context, owner ChannelOwner) {
	id, ok := h.channelID(c)
	if !ok {
		return
	}
	var request struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, channelError(ErrInvalidRequest, "inbox.channel.verify"))
		return
	}
	channel, err := h.channels.Verify(c.Request.Context(), owner, id, request.Code)
	if err != nil {
		h.writeError(c, channelError(err, "inbox.channel.verify"))
		return
	}
	writeData(c, channel)
}

func (h *ChannelHandler) resendCode(c *gin.Context, owner ChannelOwner) {
	id, ok := h.channelID(c)
	if !ok {
		return
	}
	if err := h.channels.ResendCode(c.Request.Context(), owner, id); err != nil {
		h.writeError(c, channelError(err, "inbox.channel.resend_code"))
		return
	}
	writeData(c, gin.H{"sent": true})
}

func (h *ChannelHandler) rotateSecret(c *gin.Context, owner ChannelOwner) {
	id, ok := h.channelID(c)
	if !ok {
		return
	}
	secret, err := h.channels.RotateSecret(c.Request.Context(), owner, id)
	if err != nil {
		h.writeError(c, channelError(err, "inbox.channel.rotate_secret"))
		return
	}
	writeData(c, gin.H{"secret": secret})
}

func (h *ChannelHandler) test(c *gin.Context, owner ChannelOwner) {
	id, ok := h.channelID(c)
	if !ok {
		return
	}
	err := h.dispatcher.SendTest(c.Request.Context(), owner, id)
	switch {
	case err == nil:
		writeData(c, gin.H{"sent": true})
	case knownChannelError(err):
		h.writeError(c, channelError(err, "inbox.channel.test"))
	default:
		// A failed test send is an answer for the owner, not a server
		// fault: report what the receiving end did.
		writeData(c, gin.H{"sent": false, "error": err.Error()})
	}
}

// deliveries lists the channel's recent deliveries.
func (h *ChannelHandler) deliveries(c *gin.Context, owner ChannelOwner) {
	id, ok := h.channelID(c)
	if !ok {
		return
	}
	items, err := h.dispatcher.ListDeliveries(c.Request.Context(), owner, id, 20)
	if err != nil {
		h.writeError(c, channelError(err, "inbox.channel.deliveries"))
		return
	}
	writeData(c, gin.H{"items": items})
}

// retryDelivery sends a failed delivery again.
func (h *ChannelHandler) retryDelivery(c *gin.Context, owner ChannelOwner) {
	id, ok := h.channelID(c)
	if !ok {
		return
	}
	deliveryID, err := uuid.Parse(strings.TrimSpace(c.Param("delivery_id")))
	if err != nil || deliveryID == uuid.Nil {
		h.writeError(c, channelError(ErrInvalidRequest, "inbox.channel.delivery.retry"))
		return
	}
	if err := h.dispatcher.Redeliver(c.Request.Context(), owner, id, deliveryID); err != nil {
		h.writeError(c, channelError(err, "inbox.channel.delivery.retry"))
		return
	}
	writeData(c, gin.H{"queued": true})
}

func knownChannelError(err error) bool {
	for _, known := range []error{ErrInvalidRequest, ErrChannelNotFound, ErrChannelConflict, ErrVerificationFailed, ErrRateLimited, ErrDeliveryUnavailable} {
		if errors.Is(err, known) {
			return true
		}
	}
	return false
}

func (h *ChannelHandler) channelID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("channel_id")))
	if err != nil || id == uuid.Nil {
		h.writeError(c, channelError(ErrInvalidRequest, "inbox.channel.request"))
		return uuid.Nil, false
	}
	return id, true
}

func (h *ChannelHandler) writeError(c *gin.Context, err error) {
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	presentation := h.projector.Project(err, locale).Presentation
	c.JSON(presentation.HTTPStatus, gin.H{"code": presentation.Code.String(), "message": presentation.Message, "retryable": presentation.Retryable})
}
