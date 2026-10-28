package digitalhuman

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// WithServerWatches serves a task's server watch (ADR 0031).
func (h *TaskHandler) WithServerWatches(watches TaskServerWatchService) *TaskHandler {
	h.watches = watches
	return h
}

func (h *TaskHandler) registerServerWatchRoutes(group *gin.RouterGroup) {
	if h.watches == nil {
		return
	}
	group.GET("/:task_id/server-watch", h.getServerWatch)
	group.PUT("/:task_id/server-watch", h.putServerWatch)
}

func (h *TaskHandler) getServerWatch(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	// The watch is as visible as its task.
	if _, err := h.service.GetTask(c.Request.Context(), h.sharedAccess(c, scope, id), taskID); err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.server_watch.get"))
		return
	}
	watch, err := h.watches.GetServerWatch(c.Request.Context(), scope, id, taskID)
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.server_watch.get"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, watch)
}

func (h *TaskHandler) putServerWatch(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	var req PutServerWatchRequest
	if !h.base.decode(c, &req) {
		return
	}
	task, err := h.service.GetTask(c.Request.Context(), h.sharedAccess(c, scope, id), taskID)
	if err == nil && (task == nil || !task.CanChange) {
		err = ErrTaskReadOnly
	}
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.server_watch.put"))
		return
	}
	watch, err := h.watches.PutServerWatch(c.Request.Context(), scope, id, taskID, req)
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.server_watch.put"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, watch)
}
