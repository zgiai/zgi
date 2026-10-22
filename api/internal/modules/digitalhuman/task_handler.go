package digitalhuman

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type TaskHandler struct {
	service TaskHandlerService
	base    *Handler
	pending TaskPendingChangeSource
	files   TaskReportFileSource
}

func NewTaskHandler(service TaskHandlerService, projector *apptransport.Projector) *TaskHandler {
	if service == nil || projector == nil {
		panic("digital human task handler requires service and projector")
	}
	return &TaskHandler{service: service, base: &Handler{projector: projector}}
}

// WithPendingChanges marks personal-view tasks whose proposed change waits for
// the reader's approval, so the task panel can point to the approval card.
func (h *TaskHandler) WithPendingChanges(source TaskPendingChangeSource) *TaskHandler {
	h.pending = source
	return h
}

// WithReportFiles lets reports carry and serve their run's files.
func (h *TaskHandler) WithReportFiles(source TaskReportFileSource) *TaskHandler {
	h.files = source
	return h
}

func (h *TaskHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/digital-humans/:id/tasks")
	group.GET("", h.list)
	group.POST("", h.create)
	group.POST("/schedule-preview", h.previewSchedule)
	group.GET("/:task_id", h.get)
	group.PATCH("/:task_id", h.update)
	group.POST("/:task_id/actions/:action", h.applyAction)
	group.GET("/:task_id/runs", h.runs)
	group.GET("/:task_id/activities", h.activities)
	private := router.Group("/digital-humans/:id/my-tasks")
	private.GET("", h.myList)
	private.POST("", h.myCreate)
	private.GET("/:task_id", h.myGet)
	private.PATCH("/:task_id", h.myUpdate)
	private.POST("/:task_id/actions/:action", h.myApplyAction)
	private.GET("/:task_id/runs", h.myRuns)
	private.GET("/:task_id/activities", h.myActivities)
}

func (h *TaskHandler) list(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	query := TaskListQuery{Query: c.Query("query"), Source: c.Query("source"), ScheduleType: TaskScheduleType(c.Query("schedule_type")), Page: intQuery(c, "page"), PageSize: intQuery(c, "page_size"),
		IncludeUnreviewed: c.Query("include_unreviewed") == "true"}
	var err error
	query.RunFinishedAfter, err = optionalRFC3339Query(c.Query("run_finished_after"))
	if err != nil {
		h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task.list"))
		return
	}
	query.RunFinishedBefore, err = optionalRFC3339Query(c.Query("run_finished_before"))
	if err != nil || query.RunFinishedAfter != nil && query.RunFinishedBefore != nil && !query.RunFinishedBefore.After(*query.RunFinishedAfter) {
		h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task.list"))
		return
	}
	for _, value := range append(c.QueryArray("status"), c.QueryArray("status[]")...) {
		query.Statuses = append(query.Statuses, TaskWorkStatus(value))
	}
	for _, value := range append(c.QueryArray("priority"), c.QueryArray("priority[]")...) {
		query.Priorities = append(query.Priorities, TaskPriority(value))
	}
	for _, value := range append(c.QueryArray("requester_kind"), c.QueryArray("requester_kind[]")...) {
		query.RequesterKinds = append(query.RequesterKinds, TaskRequesterKind(value))
	}
	query.RequesterID = c.Query("requester_id")
	result, err := h.service.ListTasks(c.Request.Context(), h.sharedAccess(c, scope, id), query)
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.list"))
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

func (h *TaskHandler) previewSchedule(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	var req TaskSchedulePreviewRequest
	if !h.base.decode(c, &req) {
		return
	}
	result, err := h.service.PreviewTaskSchedule(c.Request.Context(), h.sharedAccess(c, scope, id), req)
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.schedule_preview"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
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
	result, err := h.service.CreateTask(c.Request.Context(), h.sharedAccess(c, scope, id), req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.create"))
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, result)
}

func (h *TaskHandler) get(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	result, err := h.service.GetTask(c.Request.Context(), h.sharedAccess(c, scope, id), taskID)
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.get"))
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
	result, err := h.service.UpdateTask(c.Request.Context(), h.sharedAccess(c, scope, id), taskID, req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.update"))
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
	result, err := h.service.ApplyTaskAction(c.Request.Context(), h.sharedAccess(c, scope, id), taskID, action, req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.action"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) runs(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	result, err := h.service.ListTaskRuns(c.Request.Context(), h.sharedAccess(c, scope, id), taskID, intQuery(c, "page"), intQuery(c, "page_size"))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.runs"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}
func (h *TaskHandler) activities(c *gin.Context) {
	scope, id, taskID, ok := h.ids(c)
	if !ok {
		return
	}
	result, err := h.service.ListTaskActivities(c.Request.Context(), h.sharedAccess(c, scope, id), taskID, intQuery(c, "page"), intQuery(c, "page_size"))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.activities"))
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
		h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task.request"))
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	return scope, id, taskID, true
}

func (h *TaskHandler) myList(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	query := TaskListQuery{Query: c.Query("query"), Source: c.Query("source"), ScheduleType: TaskScheduleType(c.Query("schedule_type")), Page: intQuery(c, "page"), PageSize: intQuery(c, "page_size"),
		IncludeUnreviewed: c.Query("include_unreviewed") == "true"}
	var err error
	query.RunFinishedAfter, err = optionalRFC3339Query(c.Query("run_finished_after"))
	if err != nil {
		h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task.list"))
		return
	}
	query.RunFinishedBefore, err = optionalRFC3339Query(c.Query("run_finished_before"))
	if err != nil || query.RunFinishedAfter != nil && query.RunFinishedBefore != nil && !query.RunFinishedBefore.After(*query.RunFinishedAfter) {
		h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task.list"))
		return
	}
	for _, value := range append(c.QueryArray("status"), c.QueryArray("status[]")...) {
		query.Statuses = append(query.Statuses, TaskWorkStatus(value))
	}
	for _, value := range append(c.QueryArray("priority"), c.QueryArray("priority[]")...) {
		query.Priorities = append(query.Priorities, TaskPriority(value))
	}
	result, err := h.service.ListTasks(c.Request.Context(), userTaskAccess(scope, id), query)
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.list"))
		return
	}
	if result != nil {
		withPendingChanges(c.Request.Context(), h.pending, scope, id, result.Items)
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) myCreate(c *gin.Context) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	var req CreateTaskRequest
	if !h.base.decode(c, &req) {
		return
	}
	result, err := h.service.CreateTask(c.Request.Context(), userTaskAccess(scope, id), req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.create"))
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, result)
}

func (h *TaskHandler) myGet(c *gin.Context) {
	scope, id, taskID, ok := h.myIDs(c)
	if !ok {
		return
	}
	result, err := h.service.GetTask(c.Request.Context(), userTaskAccess(scope, id), taskID)
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.get"))
		return
	}
	withPendingChanges(c.Request.Context(), h.pending, scope, id, []*Task{result})
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) myUpdate(c *gin.Context) {
	scope, id, taskID, ok := h.myIDs(c)
	if !ok {
		return
	}
	var req UpdateTaskRequest
	if !h.base.decode(c, &req) {
		return
	}
	result, err := h.service.UpdateTask(c.Request.Context(), userTaskAccess(scope, id), taskID, req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.update"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) myApplyAction(c *gin.Context) {
	scope, id, taskID, ok := h.myIDs(c)
	if !ok {
		return
	}
	var req ApplyTaskActionRequest
	if !h.base.decode(c, &req) {
		return
	}
	result, err := h.service.ApplyTaskAction(c.Request.Context(), userTaskAccess(scope, id), taskID, TaskAction(strings.TrimSpace(c.Param("action"))), req, accountMutation(scope))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.action"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) myRuns(c *gin.Context) {
	scope, id, taskID, ok := h.myIDs(c)
	if !ok {
		return
	}
	result, err := h.service.ListTaskRuns(c.Request.Context(), userTaskAccess(scope, id), taskID, intQuery(c, "page"), intQuery(c, "page_size"))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.runs"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) myActivities(c *gin.Context) {
	scope, id, taskID, ok := h.myIDs(c)
	if !ok {
		return
	}
	result, err := h.service.ListTaskActivities(c.Request.Context(), userTaskAccess(scope, id), taskID, intQuery(c, "page"), intQuery(c, "page_size"))
	if err != nil {
		h.base.writeError(c, taskApplicationError(err, "digital_human.task.activities"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *TaskHandler) myIDs(c *gin.Context) (Scope, uuid.UUID, uuid.UUID, bool) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	taskID, err := uuid.Parse(strings.TrimSpace(c.Param("task_id")))
	if err != nil || taskID == uuid.Nil {
		h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task.request"))
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	return scope, id, taskID, true
}
func accountMutation(scope Scope) TaskMutationContext {
	return TaskMutationContext{ActorType: "account", ActorRef: scope.ActorID.String(), OperationID: uuid.New(), Source: "console"}
}

// sharedAccess is the task center view. The route permission check records
// whether the caller may manage every task of the DigitalHuman.
func (h *TaskHandler) sharedAccess(c *gin.Context, scope Scope, digitalHumanID uuid.UUID) TaskAccessContext {
	access := sharedTaskAccess(scope, digitalHumanID)
	access.Viewer.Manager = c.GetBool(TaskManagerContextKey)
	return access
}

func sharedTaskAccess(scope Scope, digitalHumanID uuid.UUID) TaskAccessContext {
	return TaskAccessContext{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    scope.WorkspaceID,
		DigitalHumanID: digitalHumanID,
		Principal:      invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: scope.ActorID.String()},
		Owner:          SharedTaskOwner(),
		Surface:        SurfaceConsole,
		Viewer:         TaskViewer{Mode: TaskViewTaskCenter},
	}
}

func userTaskAccess(scope Scope, digitalHumanID uuid.UUID) TaskAccessContext {
	if scope.ActorID == uuid.Nil {
		return TaskAccessContext{}
	}
	subject := usersubject.AccountSubject(scope.ActorID)
	return TaskAccessContext{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    scope.WorkspaceID,
		DigitalHumanID: digitalHumanID,
		Principal:      invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: scope.ActorID.String()},
		Owner:          UserTaskOwner(subject),
		Surface:        SurfaceConsole,
		Viewer:         TaskViewer{Mode: TaskViewMember},
	}
}
func intQuery(c *gin.Context, key string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(c.Query(key)))
	return value
}
