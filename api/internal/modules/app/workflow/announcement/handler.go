package announcement

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetAnnouncement(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	payload, err := h.service.GetByToken(c.Request.Context(), token)
	if err != nil {
		h.handleAnnouncementError(c, err)
		return
	}
	response.Success(c, payload)
}

func (h *Handler) handleAnnouncementError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrAnnouncementNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, ErrAnnouncementExpired):
		response.FailWithMessage(c, response.ErrInvalidParam, ErrAnnouncementExpired.Error())
	default:
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	}
}
