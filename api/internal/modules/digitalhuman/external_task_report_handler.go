package digitalhuman

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EndUserNotifications reads and clears an external end user's in-app
// notifications (ADR 0008). It is implemented by the inbox and injected so
// this module does not depend on it.
type EndUserNotifications interface {
	ListEndUserNotifications(ctx context.Context, organizationID uuid.UUID, subjectKind string, subjectID uuid.UUID, unreadOnly bool, page, pageSize int) (any, error)
	MarkEndUserNotificationsRead(ctx context.Context, organizationID uuid.UUID, subjectKind string, subjectID uuid.UUID, ids []uuid.UUID, reportID *uuid.UUID) error
}

// WithEndUserNotifications enables the external notification endpoints.
func (h *ExternalHandler) WithEndUserNotifications(notifications EndUserNotifications) *ExternalHandler {
	h.notifications = notifications
	return h
}

// ExternalTaskReportReviewRequest accepts a report or asks for rework on
// behalf of an API end user.
type ExternalTaskReportReviewRequest struct {
	User     string                 `json:"user"`
	Action   TaskReportReviewAction `json:"action"`
	Feedback string                 `json:"feedback,omitempty"`
}

// registerAPIReportRoutes serves an API end user's reports and notifications.
func (h *ExternalHandler) registerAPIReportRoutes(group *gin.RouterGroup) {
	if _, ok := h.tasks.(TaskReportService); !ok {
		return
	}
	group.GET("/reports", h.ListAPITaskReports)
	group.GET("/reports/:report_id", h.GetAPITaskReport)
	group.POST("/reports/:report_id/review", h.ReviewAPITaskReport)
	group.GET("/reports/:report_id/files/:file_id", h.GetAPITaskReportFile)
	group.GET("/notifications", h.ListAPINotifications)
	group.POST("/notifications/read", h.ReadAPINotifications)
}

// registerWebAppReportRoutes serves a signed-in WebApp user's reports.
func (h *ExternalHandler) registerWebAppReportRoutes(group *gin.RouterGroup) {
	if _, ok := h.tasks.(TaskReportService); !ok {
		return
	}
	group.GET("/task-reports", h.ListWebAppTaskReports)
	group.GET("/task-reports/:report_id", h.GetWebAppTaskReport)
	group.POST("/task-reports/:report_id/review", h.ReviewWebAppTaskReport)
	group.GET("/task-reports/:report_id/files/:file_id", h.GetWebAppTaskReportFile)
}

func (h *ExternalHandler) ListAPITaskReports(c *gin.Context) {
	access, ok := h.apiTaskScope(c, userFromQuery(c))
	if !ok {
		return
	}
	h.listReports(c, access, "digital_human.external.task_report.list")
}

func (h *ExternalHandler) GetAPITaskReport(c *gin.Context) {
	access, ok := h.apiTaskScope(c, userFromQuery(c))
	if !ok {
		return
	}
	h.getReport(c, access, "digital_human.external.task_report.get")
}

// GetAPITaskReportFile downloads a file one of the end user's reports carries.
func (h *ExternalHandler) GetAPITaskReportFile(c *gin.Context) {
	access, ok := h.apiTaskScope(c, userFromQuery(c))
	if !ok {
		return
	}
	h.reportFile(c, access, "digital_human.external.task_report.file")
}

// GetWebAppTaskReportFile downloads a file one of the end user's reports carries.
func (h *ExternalHandler) GetWebAppTaskReportFile(c *gin.Context) {
	access, ok := h.webAppTaskScope(c)
	if !ok {
		return
	}
	h.reportFile(c, access, "digital_human.webapp.task_report.file")
}

func (h *ExternalHandler) reportFile(c *gin.Context, access TaskAccessContext, operation string) {
	reportID, ok := parseExternalReportID(c)
	if !ok {
		h.writeTaskApplicationError(c, ErrInvalidRequest, operation)
		return
	}
	report, err := h.tasks.(TaskReportService).GetTaskReport(c.Request.Context(), access, reportID)
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	serveTaskReportFile(c, h.base, h.runtime, accessScope(access), report)
}

// accessScope is the workspace a task reader works in.
func accessScope(access TaskAccessContext) Scope {
	return Scope{OrganizationID: access.OrganizationID, WorkspaceID: access.WorkspaceID}
}

func (h *ExternalHandler) ReviewAPITaskReport(c *gin.Context) {
	key, _, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var request ExternalTaskReportReviewRequest
	if !h.base.decode(c, &request) {
		return
	}
	access, ok := h.apiTaskScope(c, request.User)
	if !ok {
		return
	}
	h.reviewReport(c, access, ReviewTaskReportRequest{Action: request.Action, Feedback: request.Feedback},
		externalTaskMutation(c, "external-api", key.ID.String()), "digital_human.external.task_report.review")
}

func (h *ExternalHandler) ListWebAppTaskReports(c *gin.Context) {
	access, ok := h.webAppTaskScope(c)
	if !ok {
		return
	}
	h.listReports(c, access, "digital_human.webapp.task_report.list")
}

func (h *ExternalHandler) GetWebAppTaskReport(c *gin.Context) {
	access, ok := h.webAppTaskScope(c)
	if !ok {
		return
	}
	h.getReport(c, access, "digital_human.webapp.task_report.get")
}

func (h *ExternalHandler) ReviewWebAppTaskReport(c *gin.Context) {
	access, ok := h.webAppTaskScope(c)
	if !ok {
		return
	}
	var request ReviewTaskReportRequest
	if !h.base.decode(c, &request) {
		return
	}
	h.reviewReport(c, access, request, externalTaskMutation(c, "webapp", access.Principal.ID), "digital_human.webapp.task_report.review")
}

// ListAPINotifications lists an API end user's notifications: reports and
// failures of the tasks they requested.
func (h *ExternalHandler) ListAPINotifications(c *gin.Context) {
	access, ok := h.apiTaskScope(c, userFromQuery(c))
	if !ok {
		return
	}
	if h.notifications == nil || access.Owner.Subject.ID == uuid.Nil {
		h.writeTaskApplicationError(c, ErrTaskPolicyRejected, "digital_human.external.notification.list")
		return
	}
	result, err := h.notifications.ListEndUserNotifications(c.Request.Context(), access.OrganizationID, string(access.Owner.Subject.Kind), access.Owner.Subject.ID,
		c.Query("unread_only") == "true", intQuery(c, "page"), intQuery(c, "page_size"))
	if err != nil {
		h.writeTaskApplicationError(c, err, "digital_human.external.notification.list")
		return
	}
	writeExternalData(c, http.StatusOK, result)
}

// ReadAPINotifications marks notifications read, by id or by report.
func (h *ExternalHandler) ReadAPINotifications(c *gin.Context) {
	var request struct {
		User     string      `json:"user"`
		IDs      []uuid.UUID `json:"ids"`
		ReportID *uuid.UUID  `json:"report_id"`
	}
	if !h.base.decode(c, &request) {
		return
	}
	access, ok := h.apiTaskScope(c, request.User)
	if !ok {
		return
	}
	if h.notifications == nil || access.Owner.Subject.ID == uuid.Nil || (len(request.IDs) == 0 && request.ReportID == nil) {
		h.writeTaskApplicationError(c, ErrInvalidRequest, "digital_human.external.notification.read")
		return
	}
	if err := h.notifications.MarkEndUserNotificationsRead(c.Request.Context(), access.OrganizationID, string(access.Owner.Subject.Kind), access.Owner.Subject.ID, request.IDs, request.ReportID); err != nil {
		h.writeTaskApplicationError(c, err, "digital_human.external.notification.read")
		return
	}
	writeExternalData(c, http.StatusOK, gin.H{"read": true})
}

func (h *ExternalHandler) listReports(c *gin.Context, access TaskAccessContext, operation string) {
	query := TaskReportListQuery{Page: intQuery(c, "page"), PageSize: intQuery(c, "page_size")}
	if raw := strings.TrimSpace(c.Query("task_id")); raw != "" {
		taskID, err := uuid.Parse(raw)
		if err != nil || taskID == uuid.Nil {
			h.writeTaskApplicationError(c, ErrInvalidRequest, operation)
			return
		}
		query.TaskID = &taskID
	}
	if state := strings.TrimSpace(c.Query("review_state")); state != "" {
		query.ReviewStates = []TaskReportReviewState{TaskReportReviewState(state)}
	}
	result, err := h.tasks.(TaskReportService).ListTaskReports(c.Request.Context(), access, query)
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	for _, report := range result.Items {
		withReportFiles(c, h.runtime, accessScope(access), report)
		hideRunForEndUser(report)
	}
	writeExternalData(c, http.StatusOK, result)
}

func (h *ExternalHandler) getReport(c *gin.Context, access TaskAccessContext, operation string) {
	reportID, ok := parseExternalReportID(c)
	if !ok {
		h.writeTaskApplicationError(c, ErrInvalidRequest, operation)
		return
	}
	result, err := h.tasks.(TaskReportService).GetTaskReport(c.Request.Context(), access, reportID)
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	// Files are resolved from the run before its identifier is hidden.
	result = withReportFiles(c, h.runtime, accessScope(access), result)
	hideRunForEndUser(result)
	writeExternalData(c, http.StatusOK, result)
}

func (h *ExternalHandler) reviewReport(c *gin.Context, access TaskAccessContext, request ReviewTaskReportRequest, mutation TaskMutationContext, operation string) {
	reportID, ok := parseExternalReportID(c)
	if !ok {
		h.writeTaskApplicationError(c, ErrInvalidRequest, operation)
		return
	}
	result, err := h.tasks.(TaskReportService).ReviewTaskReport(c.Request.Context(), access, reportID, request, mutation)
	if err != nil {
		h.writeTaskApplicationError(c, err, operation)
		return
	}
	hideRunForEndUser(result)
	writeExternalData(c, http.StatusOK, result)
}

// hideRunForEndUser keeps internal run identifiers out of the end-user
// contract; the run log is an operator view.
func hideRunForEndUser(report *TaskReport) {
	if report != nil {
		report.DigitalHumanRunID = nil
	}
}

func parseExternalReportID(c *gin.Context) (uuid.UUID, bool) {
	reportID, err := uuid.Parse(strings.TrimSpace(c.Param("report_id")))
	return reportID, err == nil && reportID != uuid.Nil
}
