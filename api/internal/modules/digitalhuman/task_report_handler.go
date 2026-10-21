package digitalhuman

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterReportRoutes serves task reports (ADR 0008): the task center feed
// and the reader's own reports. It registers nothing when the task service
// does not store reports.
func (h *TaskHandler) RegisterReportRoutes(router *gin.RouterGroup) {
	if _, ok := h.service.(TaskReportService); !ok {
		return
	}
	shared := router.Group("/digital-humans/:id/task-reports")
	shared.GET("", h.reportList(false))
	shared.GET("/:report_id", h.reportGet(false))
	shared.POST("/:report_id/review", h.reportReview(false))
	private := router.Group("/digital-humans/:id/my-task-reports")
	private.GET("", h.reportList(true))
	private.GET("/:report_id", h.reportGet(true))
	private.POST("/:report_id/review", h.reportReview(true))
}

func (h *TaskHandler) reportAccess(c *gin.Context, mine bool) (TaskAccessContext, Scope, bool) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return TaskAccessContext{}, Scope{}, false
	}
	if mine {
		return userTaskAccess(scope, id), scope, true
	}
	return h.sharedAccess(c, scope, id), scope, true
}

func (h *TaskHandler) reportList(mine bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		access, _, ok := h.reportAccess(c, mine)
		if !ok {
			return
		}
		query, ok := h.reportListQuery(c)
		if !ok {
			return
		}
		result, err := h.service.(TaskReportService).ListTaskReports(c.Request.Context(), access, query)
		if err != nil {
			h.base.writeError(c, taskApplicationError(err, "digital_human.task_report.list"))
			return
		}
		writeDigitalHumanData(c, http.StatusOK, result)
	}
}

func (h *TaskHandler) reportGet(mine bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		access, _, ok := h.reportAccess(c, mine)
		if !ok {
			return
		}
		reportID, ok := h.reportID(c)
		if !ok {
			return
		}
		result, err := h.service.(TaskReportService).GetTaskReport(c.Request.Context(), access, reportID)
		if err != nil {
			h.base.writeError(c, taskApplicationError(err, "digital_human.task_report.get"))
			return
		}
		writeDigitalHumanData(c, http.StatusOK, result)
	}
}

func (h *TaskHandler) reportReview(mine bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		access, scope, ok := h.reportAccess(c, mine)
		if !ok {
			return
		}
		reportID, ok := h.reportID(c)
		if !ok {
			return
		}
		var req ReviewTaskReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task_report.review"))
			return
		}
		result, err := h.service.(TaskReportService).ReviewTaskReport(c.Request.Context(), access, reportID, req, accountMutation(scope))
		if err != nil {
			h.base.writeError(c, taskApplicationError(err, "digital_human.task_report.review"))
			return
		}
		writeDigitalHumanData(c, http.StatusOK, result)
	}
}

func (h *TaskHandler) reportID(c *gin.Context) (uuid.UUID, bool) {
	reportID, err := uuid.Parse(strings.TrimSpace(c.Param("report_id")))
	if err != nil || reportID == uuid.Nil {
		h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task_report.request"))
		return uuid.Nil, false
	}
	return reportID, true
}

// reportListQuery reads task_id, review_state[], outcome[], page, and
// page_size.
func (h *TaskHandler) reportListQuery(c *gin.Context) (TaskReportListQuery, bool) {
	query := TaskReportListQuery{Page: intQuery(c, "page"), PageSize: intQuery(c, "page_size")}
	if raw := strings.TrimSpace(c.Query("task_id")); raw != "" {
		taskID, err := uuid.Parse(raw)
		if err != nil || taskID == uuid.Nil {
			h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task_report.request"))
			return query, false
		}
		query.TaskID = &taskID
	}
	for _, value := range c.QueryArray("review_state[]") {
		switch state := TaskReportReviewState(strings.TrimSpace(value)); state {
		case TaskReportNew, TaskReportAccepted, TaskReportReworkRequested:
			query.ReviewStates = append(query.ReviewStates, state)
		default:
			h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task_report.request"))
			return query, false
		}
	}
	for _, value := range c.QueryArray("outcome[]") {
		switch outcome := TaskReportOutcome(strings.TrimSpace(value)); outcome {
		case TaskReportDone, TaskReportPartial, TaskReportNotDone, TaskReportFailed:
			query.Outcomes = append(query.Outcomes, outcome)
		default:
			h.base.writeError(c, taskApplicationError(ErrInvalidRequest, "digital_human.task_report.request"))
			return query, false
		}
	}
	return query, true
}
