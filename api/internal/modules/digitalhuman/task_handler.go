package digitalhuman

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type TaskHandler struct {
	service TaskHandlerService
	base    *Handler
}

func NewTaskHandler(service TaskHandlerService, projector *apptransport.Projector) *TaskHandler {
	if service == nil || projector == nil {
		panic("digital human task handler requires service and projector")
	}
	return &TaskHandler{service: service, base: &Handler{projector: projector}}
}

func (h *TaskHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/digital-humans/:id/tasks")
	group.GET("", h.list)
	group.POST("", h.create)
	group.GET("/:task_id", h.get)
	group.PATCH("/:task_id", h.update)
	group.POST("/:task_id/actions/:action", h.applyAction)
	group.GET("/:task_id/runs", h.runs)
	group.GET("/:task_id/activities", h.activities)
}

func (h *TaskHandler) list(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	query := TaskListQuery{Query: c.Query("query"), Source: c.Query("source"), ScheduleType: TaskScheduleType(c.Query("schedule_type")), Page: intQuery(c, "page"), PageSize: intQuery(c, "page_size")}
	var err error
	query.RunFinishedAfter, err = optionalRFC3339Query(c.Query("run_finished_after"))
	if err != nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.task.list"))
		return
	}
	query.RunFinishedBefore, err = optionalRFC3339Query(c.Query("run_finished_before"))
	if err != nil || query.RunFinishedAfter != nil && query.RunFinishedBefore != nil && !query.RunFinishedBefore.After(*query.RunFinishedAfter) {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.task.list"))
		return
	}
	for _, value := range append(c.QueryArray("status"), c.QueryArray("status[]")...) {
		query.Statuses = append(query.Statuses, TaskWorkStatus(value))
	}
	for _, value := range append(c.QueryArray("priority"), c.QueryArray("priority[]")...) {
		query.Priorities = append(query.Priorities, TaskPriority(value))
	}
	result, err := h.service.ListTasks(c.Request.Context(), scope, id, query)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.task.list"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func optionalRFC3339Query(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (h *TaskHandler) create(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	var req CreateTaskRequest
	if !h.base.decode(c, &req) {
		return
	}
	result, err := h.service.CreateTask(c.Request.Context(), scope, id, req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.task.create"))
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, result)
}

func (h *TaskHandler) get(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	result, err := h.service.GetTask(c.Request.Context(), scope, id, taskID, "")
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.task.get"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}
func (h *TaskHandler) update(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	var req UpdateTaskRequest
	if !h.base.decode(c, &req) {
		return
	}
	result, err := h.service.UpdateTask(c.Request.Context(), scope, id, taskID, req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.task.update"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) applyAction(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	action := TaskAction(strings.TrimSpace(c.Param("action")))
	var req ApplyTaskActionRequest
	if !h.base.decode(c, &req) {
		return
	}
	result, err := h.service.ApplyTaskAction(c.Request.Context(), scope, id, taskID, action, req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.task.action"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) runs(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	result, err := h.service.ListTaskRuns(c.Request.Context(), scope, id, taskID, intQuery(c, "page"), intQuery(c, "page_size"), "")
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.task.runs"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}
func (h *TaskHandler) activities(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	result, err := h.service.ListTaskActivities(c.Request.Context(), scope, id, taskID, intQuery(c, "page"), intQuery(c, "page_size"), "")
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.task.activities"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) ids(c *gin.Context) (Scope, uuid.UUID, uuid.UUID, bool) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	taskID, err := uuid.Parse(strings.TrimSpace(c.Param("task_id")))
	if err != nil || taskID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.task.request"))
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	return scope, id, taskID, true
}
func accountMutation(scope Scope) TaskMutationContext {
	return TaskMutationContext{ActorType: "account", ActorRef: scope.ActorID.String(), OperationID: uuid.New(), Source: "console"}
}
func intQuery(c *gin.Context, key string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(c.Query(key)))
	return value
}
