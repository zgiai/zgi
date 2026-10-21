package digitalhuman

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

// ExternalTaskCreateRequest keeps the public API's user field alongside the
// product task fields. The user value is consumed only by the subject resolver.
type ExternalTaskCreateRequest struct {
	User                    string           `json:"user"`
	Name                    string           `json:"name"`
	Goal                    string           `json:"goal"`
	SuccessCriteria         []string         `json:"success_criteria,omitempty"`
	Priority                TaskPriority     `json:"priority,omitempty"`
	Schedule                TaskSchedule     `json:"schedule"`
	DeliveryMode            TaskDeliveryMode `json:"delivery_mode,omitempty"`
	ExecutionTimeoutSeconds int              `json:"execution_timeout_seconds,omitempty"`
	MaxAttempts             int              `json:"max_attempts,omitempty"`
}

func (r ExternalTaskCreateRequest) taskRequest() CreateTaskRequest {
	return CreateTaskRequest{
		Name:                    r.Name,
		Goal:                    r.Goal,
		SuccessCriteria:         r.SuccessCriteria,
		Priority:                r.Priority,
		Schedule:                r.Schedule,
		DeliveryMode:            r.DeliveryMode,
		ExecutionTimeoutSeconds: r.ExecutionTimeoutSeconds,
		MaxAttempts:             r.MaxAttempts,
	}
}

type ExternalTaskUpdateRequest struct {
	User                    string            `json:"user"`
	ExpectedRevision        int64             `json:"expected_revision"`
	Name                    *string           `json:"name,omitempty"`
	Goal                    *string           `json:"goal,omitempty"`
	SuccessCriteria         *[]string         `json:"success_criteria,omitempty"`
	Priority                *TaskPriority     `json:"priority,omitempty"`
	Schedule                *TaskSchedule     `json:"schedule,omitempty"`
	DeliveryMode            *TaskDeliveryMode `json:"delivery_mode,omitempty"`
	ExecutionTimeoutSeconds *int              `json:"execution_timeout_seconds,omitempty"`
	MaxAttempts             *int              `json:"max_attempts,omitempty"`
}

func (r ExternalTaskUpdateRequest) taskRequest() UpdateTaskRequest {
	return UpdateTaskRequest{
		ExpectedRevision:        r.ExpectedRevision,
		Name:                    r.Name,
		Goal:                    r.Goal,
		SuccessCriteria:         r.SuccessCriteria,
		Priority:                r.Priority,
		Schedule:                r.Schedule,
		DeliveryMode:            r.DeliveryMode,
		ExecutionTimeoutSeconds: r.ExecutionTimeoutSeconds,
		MaxAttempts:             r.MaxAttempts,
	}
}

type ExternalTaskActionRequest struct {
	User             string `json:"user"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type WebAppTaskUpdateRequest struct {
	ExpectedRevision        int64             `json:"expected_revision"`
	Name                    *string           `json:"name,omitempty"`
	Goal                    *string           `json:"goal,omitempty"`
	SuccessCriteria         *[]string         `json:"success_criteria,omitempty"`
	Priority                *TaskPriority     `json:"priority,omitempty"`
	Schedule                *TaskSchedule     `json:"schedule,omitempty"`
	DeliveryMode            *TaskDeliveryMode `json:"delivery_mode,omitempty"`
	ExecutionTimeoutSeconds *int              `json:"execution_timeout_seconds,omitempty"`
	MaxAttempts             *int              `json:"max_attempts,omitempty"`
}

func (r WebAppTaskUpdateRequest) taskRequest() UpdateTaskRequest {
	return UpdateTaskRequest{
		ExpectedRevision:        r.ExpectedRevision,
		Name:                    r.Name,
		Goal:                    r.Goal,
		SuccessCriteria:         r.SuccessCriteria,
		Priority:                r.Priority,
		Schedule:                r.Schedule,
		DeliveryMode:            r.DeliveryMode,
		ExecutionTimeoutSeconds: r.ExecutionTimeoutSeconds,
		MaxAttempts:             r.MaxAttempts,
	}
}

type WebAppTaskActionRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

func (h *ExternalHandler) ListAPITasks(c *gin.Context) {
	scope, ok := h.apiTaskScope(c, userFromQuery(c))
	if !ok {
		return
	}
	h.listTasks(c, scope, "digital_human.external.task.list")
}

func (h *ExternalHandler) CreateAPITask(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var request ExternalTaskCreateRequest
	if !h.base.decode(c, &request) {
		return
	}
	scope, ok := h.apiUserScope(c, key, access, request.User)
	if !ok {
		return
	}
	result, err := h.tasks.CreateTask(c.Request.Context(), externalTaskAccess(scope, access.Definition.ID, SurfaceAPI), request.taskRequest(), externalTaskMutation(c, "external-api", key.ID.String()))
	h.writeTaskResult(c, http.StatusCreated, result, err, "digital_human.external.task.create")
}

func (h *ExternalHandler) GetAPITask(c *gin.Context) {
	scope, taskID, ok := h.apiTaskID(c)
	if !ok {
		return
	}
	h.getTask(c, scope, taskID, "digital_human.external.task.get")
}

func (h *ExternalHandler) UpdateAPITask(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var request ExternalTaskUpdateRequest
	if !h.base.decode(c, &request) {
		return
	}
	scope, ok := h.apiUserScope(c, key, access, request.User)
	if !ok {
		return
	}
	taskID, ok := parseExternalTaskID(c)
	if !ok {
		return
	}
	result, err := h.tasks.UpdateTask(c.Request.Context(), externalTaskAccess(scope, access.Definition.ID, SurfaceAPI), taskID, request.taskRequest(), externalTaskMutation(c, "external-api", key.ID.String()))
	h.writeTaskResult(c, http.StatusOK, result, err, "digital_human.external.task.update")
}

func (h *ExternalHandler) ApplyAPITaskAction(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var request ExternalTaskActionRequest
	if !h.base.decode(c, &request) {
		return
	}
	scope, ok := h.apiUserScope(c, key, access, request.User)
	if !ok {
		return
	}
	taskID, ok := parseExternalTaskID(c)
	if !ok {
		return
	}
	result, err := h.tasks.ApplyTaskAction(c.Request.Context(), externalTaskAccess(scope, access.Definition.ID, SurfaceAPI), taskID, TaskAction(strings.TrimSpace(c.Param("action"))), ApplyTaskActionRequest{ExpectedRevision: request.ExpectedRevision}, externalTaskMutation(c, "external-api", key.ID.String()))
	h.writeTaskResult(c, http.StatusOK, result, err, "digital_human.external.task.action")
}

func (h *ExternalHandler) ListAPITaskRuns(c *gin.Context) {
	scope, taskID, ok := h.apiTaskID(c)
	if !ok {
		return
	}
	h.listTaskRuns(c, scope, taskID, "digital_human.external.task.runs")
}

func (h *ExternalHandler) ListAPITaskActivities(c *gin.Context) {
	scope, taskID, ok := h.apiTaskID(c)
	if !ok {
		return
	}
	h.listTaskActivities(c, scope, taskID, "digital_human.external.task.activities")
}

func (h *ExternalHandler) ListWebAppTasks(c *gin.Context) {
	scope, ok := h.webAppTaskScope(c)
	if !ok {
		return
	}
	h.listTasks(c, scope, "digital_human.webapp.task.list")
}

func (h *ExternalHandler) CreateWebAppTask(c *gin.Context) {
	scope, ok := h.webAppTaskScope(c)
	if !ok {
		return
	}
	var request CreateTaskRequest
	if !h.base.decode(c, &request) {
		return
	}
	result, err := h.tasks.CreateTask(c.Request.Context(), scope, request, externalTaskMutation(c, "webapp", scope.Principal.ID))
	h.writeTaskResult(c, http.StatusCreated, result, err, "digital_human.webapp.task.create")
}

func (h *ExternalHandler) GetWebAppTask(c *gin.Context) {
	scope, taskID, ok := h.webAppTaskID(c)
	if !ok {
		return
	}
	h.getTask(c, scope, taskID, "digital_human.webapp.task.get")
}

func (h *ExternalHandler) UpdateWebAppTask(c *gin.Context) {
	scope, ok := h.webAppTaskScope(c)
	if !ok {
		return
	}
	var request WebAppTaskUpdateRequest
	if !h.base.decode(c, &request) {
		return
	}
	taskID, ok := parseExternalTaskID(c)
	if !ok {
		return
	}
	result, err := h.tasks.UpdateTask(c.Request.Context(), scope, taskID, request.taskRequest(), externalTaskMutation(c, "webapp", scope.Principal.ID))
	h.writeTaskResult(c, http.StatusOK, result, err, "digital_human.webapp.task.update")
}

func (h *ExternalHandler) ApplyWebAppTaskAction(c *gin.Context) {
	scope, ok := h.webAppTaskScope(c)
	if !ok {
		return
	}
	var request WebAppTaskActionRequest
	if !h.base.decode(c, &request) {
		return
	}
	taskID, ok := parseExternalTaskID(c)
	if !ok {
		return
	}
	result, err := h.tasks.ApplyTaskAction(c.Request.Context(), scope, taskID, TaskAction(strings.TrimSpace(c.Param("action"))), ApplyTaskActionRequest{ExpectedRevision: request.ExpectedRevision}, externalTaskMutation(c, "webapp", scope.Principal.ID))
	h.writeTaskResult(c, http.StatusOK, result, err, "digital_human.webapp.task.action")
}

func (h *ExternalHandler) ListWebAppTaskRuns(c *gin.Context) {
	scope, taskID, ok := h.webAppTaskID(c)
	if !ok {
		return
	}
	h.listTaskRuns(c, scope, taskID, "digital_human.webapp.task.runs")
}

func (h *ExternalHandler) ListWebAppTaskActivities(c *gin.Context) {
	scope, taskID, ok := h.webAppTaskID(c)
	if !ok {
		return
	}
	h.listTaskActivities(c, scope, taskID, "digital_human.webapp.task.activities")
}

func (h *ExternalHandler) apiTaskID(c *gin.Context) (TaskAccessContext, uuid.UUID, bool) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return TaskAccessContext{}, uuid.Nil, false
	}
	scope, ok := h.apiUserScope(c, key, access, userFromQuery(c))
	if !ok {
		return TaskAccessContext{}, uuid.Nil, false
	}
	taskID, ok := parseExternalTaskID(c)
	if !ok {
		return TaskAccessContext{}, uuid.Nil, false
	}
	return externalTaskAccess(scope, access.Definition.ID, SurfaceAPI), taskID, true
}

func (h *ExternalHandler) webAppTaskID(c *gin.Context) (TaskAccessContext, uuid.UUID, bool) {
	scope, ok := h.webAppTaskScope(c)
	if !ok {
		return TaskAccessContext{}, uuid.Nil, false
	}
	taskID, ok := parseExternalTaskID(c)
	if !ok {
		return TaskAccessContext{}, uuid.Nil, false
	}
	return scope, taskID, true
}

func (h *ExternalHandler) apiTaskScope(c *gin.Context, user string) (TaskAccessContext, bool) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return TaskAccessContext{}, false
	}
	scope, ok := h.apiUserScope(c, key, access, user)
	if !ok {
		return TaskAccessContext{}, false
	}
	return externalTaskAccess(scope, access.Definition.ID, SurfaceAPI), true
}

func (h *ExternalHandler) webAppTaskScope(c *gin.Context) (TaskAccessContext, bool) {
	scope, access, identity, ok := h.webAppScope(c)
	if !ok {
		return TaskAccessContext{}, false
	}
	if !identity.Authenticated || scope.UserSubjectID == nil || *scope.UserSubjectID == uuid.Nil {
		// Anonymous landing pages can chat, but task ownership requires an
		// authenticated account subject. Keep this an authentication response so
		// clients can show the login flow rather than a generic policy error.
		h.writeAuthError(c, false)
		return TaskAccessContext{}, false
	}
	return externalTaskAccess(scope, access.Definition.ID, SurfaceWebApp), true
}

func externalTaskAccess(scope Scope, digitalHumanID uuid.UUID, surface string) TaskAccessContext {
	owner := TaskOwnerRef{}
	if scope.UserSubjectID != nil && *scope.UserSubjectID != uuid.Nil {
		owner = UserTaskOwner(usersubject.Ref{Kind: usersubject.Kind(scope.UserSubjectKind), ID: *scope.UserSubjectID})
	}
	return TaskAccessContext{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    scope.WorkspaceID,
		DigitalHumanID: digitalHumanID,
		Principal:      invocation.PrincipalRef{Type: invocation.PrincipalType(scope.PrincipalType), ID: scope.PrincipalID.String()},
		Owner:          owner,
		Surface:        surface,
		Viewer:         TaskViewer{Mode: TaskViewExternal},
	}
}

func externalTaskMutation(c *gin.Context, source, actor string) TaskMutationContext {
	operationID := uuid.New()
	if raw := strings.TrimSpace(c.GetHeader("Idempotency-Key")); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil && parsed != uuid.Nil {
			operationID = parsed
		}
	}
	// Task activities have a deliberately small, stable actor type enum. Keep
	// the invocation surface in Source for audit/debugging, but persist all
	// public external surfaces as the supported `external` actor type. Writing
	// values such as `webapp` or `external-api` directly would violate the
	// database constraint and turn an otherwise valid mutation into a 500.
	return TaskMutationContext{ActorType: "external", ActorRef: actor, OperationID: operationID, Source: source}
}

func parseExternalTaskID(c *gin.Context) (uuid.UUID, bool) {
	taskID, err := uuid.Parse(strings.TrimSpace(c.Param("task_id")))
	if err != nil || taskID == uuid.Nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code":      AppCodeTaskInvalidRequest.String(),
			"message":   "The task identifier is invalid.",
			"retryable": false,
		})
		return uuid.Nil, false
	}
	return taskID, true
}

func (h *ExternalHandler) listTasks(c *gin.Context, access TaskAccessContext, operation string) {
	query, err := externalTaskListQuery(c)
	if err != nil {
		h.writeTaskApplicationError(c, ErrInvalidRequest, operation)
		return
	}
	result, err := h.tasks.ListTasks(c.Request.Context(), access, query)
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	writeExternalData(c, http.StatusOK, result)
}

func (h *ExternalHandler) getTask(c *gin.Context, access TaskAccessContext, taskID uuid.UUID, operation string) {
	result, err := h.tasks.GetTask(c.Request.Context(), access, taskID)
	h.writeTaskResult(c, http.StatusOK, result, err, operation)
}

func (h *ExternalHandler) listTaskRuns(c *gin.Context, access TaskAccessContext, taskID uuid.UUID, operation string) {
	result, err := h.tasks.ListTaskRuns(c.Request.Context(), access, taskID, intQuery(c, "page"), intQuery(c, "page_size"))
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	writeExternalData(c, http.StatusOK, result)
}

func (h *ExternalHandler) listTaskActivities(c *gin.Context, access TaskAccessContext, taskID uuid.UUID, operation string) {
	result, err := h.tasks.ListTaskActivities(c.Request.Context(), access, taskID, intQuery(c, "page"), intQuery(c, "page_size"))
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	for _, item := range result.Items {
		if item != nil {
			// Actor references are trusted audit data (for example an API key
			// id), not part of the public task ownership contract.
			item.ActorRef = ""
		}
	}
	writeExternalData(c, http.StatusOK, result)
}

func (h *ExternalHandler) writeTaskResult(c *gin.Context, status int, result *Task, err error, operation string) {
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	writeExternalData(c, status, result)
}

func (h *ExternalHandler) writeTaskApplicationError(c *gin.Context, err error, operation string) {
	h.base.writeError(c, taskApplicationError(err, operation))
}

func externalTaskListQuery(c *gin.Context) (TaskListQuery, error) {
	query := TaskListQuery{Query: c.Query("query"), Source: c.Query("source"), ScheduleType: TaskScheduleType(c.Query("schedule_type")), Page: intQuery(c, "page"), PageSize: intQuery(c, "page_size")}
	var err error
	query.RunFinishedAfter, err = optionalRFC3339Query(c.Query("run_finished_after"))
	if err != nil {
		return TaskListQuery{}, err
	}
	query.RunFinishedBefore, err = optionalRFC3339Query(c.Query("run_finished_before"))
	if err != nil || query.RunFinishedAfter != nil && query.RunFinishedBefore != nil && !query.RunFinishedBefore.After(*query.RunFinishedAfter) {
		return TaskListQuery{}, ErrInvalidRequest
	}
	for _, value := range append(c.QueryArray("status"), c.QueryArray("status[]")...) {
		query.Statuses = append(query.Statuses, TaskWorkStatus(value))
	}
	for _, value := range append(c.QueryArray("priority"), c.QueryArray("priority[]")...) {
		query.Priorities = append(query.Priorities, TaskPriority(value))
	}
	return query, nil
}
