package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	systemservice "github.com/zgiai/zgi/api/internal/modules/system/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type GraphRuntimeHealthHandler struct {
	service *systemservice.GraphRuntimeHealthService
}

func NewGraphRuntimeHealthHandler(service *systemservice.GraphRuntimeHealthService) *GraphRuntimeHealthHandler {
	return &GraphRuntimeHealthHandler{service: service}
}

func (h *GraphRuntimeHealthHandler) GetCapability(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusServiceUnavailable, response.Response{
			Code:    "graph_runtime_unavailable",
			Message: "Knowledge graph runtime is unavailable.",
		})
		return
	}
	response.Success(c, h.service.Capability(c.Request.Context()))
}
