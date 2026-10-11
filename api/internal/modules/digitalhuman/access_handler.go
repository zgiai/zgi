package digitalhuman

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// AccessHandler exposes operator-only external access settings and keys.
type AccessHandler struct {
	service *AccessService
	base    *Handler
}

func NewAccessHandler(service *AccessService, projector *apptransport.Projector) *AccessHandler {
	if service == nil || projector == nil {
		panic("digital human access handler requires service and projector")
	}
	return &AccessHandler{service: service, base: &Handler{projector: projector}}
}

func (h *AccessHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/digital-humans/:id")
	group.GET("/access", h.GetSettings)
	group.PATCH("/access", h.UpdateSettings)
	group.GET("/api-keys", h.ListAPIKeys)
	group.POST("/api-keys", h.CreateAPIKey)
	group.PATCH("/api-keys/:api_key_id", h.UpdateAPIKey)
	group.DELETE("/api-keys/:api_key_id", h.DeleteAPIKey)
}

func (h *AccessHandler) GetSettings(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	settings, err := h.service.GetSettings(c.Request.Context(), scope, id)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.access.get"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, settings)
}

func (h *AccessHandler) UpdateSettings(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	var request AccessSettingsUpdate
	if !h.base.decode(c, &request) {
		return
	}
	settings, err := h.service.UpdateSettings(c.Request.Context(), scope, id, request)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.access.update"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, settings)
}

func (h *AccessHandler) ListAPIKeys(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	keys, err := h.service.ListAPIKeys(c.Request.Context(), scope, id)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.api_key.list"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"api_keys": keys, "total": len(keys)})
}

func (h *AccessHandler) CreateAPIKey(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	var request APIKeyCreateRequest
	if !h.base.decode(c, &request) {
		return
	}
	created, err := h.service.CreateAPIKey(c.Request.Context(), scope, id, request)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.api_key.create"))
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, created)
}

func (h *AccessHandler) UpdateAPIKey(c *gin.Context) {
	scope, id, keyID, ok := h.scopeAndKeyID(c)
	if !ok {
		return
	}
	var request APIKeyUpdateRequest
	if !h.base.decode(c, &request) {
		return
	}
	key, err := h.service.UpdateAPIKey(c.Request.Context(), scope, id, keyID, request)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.api_key.update"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, key)
}

func (h *AccessHandler) DeleteAPIKey(c *gin.Context) {
	scope, id, keyID, ok := h.scopeAndKeyID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteAPIKey(c.Request.Context(), scope, id, keyID); err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.api_key.delete"))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AccessHandler) scopeAndKeyID(c *gin.Context) (Scope, uuid.UUID, uuid.UUID, bool) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	keyID, err := uuid.Parse(c.Param("api_key_id"))
	if err != nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.api_key.parse"))
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	return scope, id, keyID, true
}
