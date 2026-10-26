package v1

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

type workJournalManagement interface {
	ForEmployee(context.Context, digitalhuman.Scope, uuid.UUID) (*workjournal.Service, workjournal.Scope, error)
}
type workJournalHandler struct{ service workJournalManagement }

func registerWorkJournalRoutes(group *gin.RouterGroup, service workJournalManagement) {
	if group == nil || service == nil {
		return
	}
	h := &workJournalHandler{service: service}
	g := group.Group("/digital-humans/:id/work-journal")
	g.GET("/records", h.records)
	g.GET("/records/:recordID", h.record)
	g.GET("/records/:recordID/history", h.history)
	g.GET("/operations", h.operations)
	g.GET("/operations/:operationID", h.operation)
}
func workJournalRouteScope(c *gin.Context) (digitalhuman.Scope, uuid.UUID, bool) {
	org, e1 := uuid.Parse(util.GetOrganizationID(c))
	workspace, e2 := uuid.Parse(util.GetWorkspaceID(c))
	owner, e3 := uuid.Parse(middleware.GetAccountID(c))
	employee, e4 := uuid.Parse(c.Param("id"))
	if e1 != nil || e2 != nil || e3 != nil || org == uuid.Nil || workspace == uuid.Nil || owner == uuid.Nil {
		connectorAuthError(c)
		return digitalhuman.Scope{}, uuid.Nil, false
	}
	if e4 != nil || employee == uuid.Nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return digitalhuman.Scope{}, uuid.Nil, false
	}
	return digitalhuman.Scope{OrganizationID: org, WorkspaceID: workspace, ActorID: owner}, employee, true
}
func (h *workJournalHandler) restore(c *gin.Context) (*workjournal.Service, workjournal.Scope, bool) {
	scope, id, ok := workJournalRouteScope(c)
	if !ok {
		return nil, workjournal.Scope{}, false
	}
	service, restored, err := h.service.ForEmployee(c.Request.Context(), scope, id)
	if err != nil {
		connectorRouteError(c, err)
		return nil, workjournal.Scope{}, false
	}
	return service, restored, true
}
func journalPageQuery(c *gin.Context) (uuid.UUID, int, bool) {
	limit := 20
	var err error
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return uuid.Nil, 0, false
	}
	var after uuid.UUID
	if raw := c.Query("after_id"); raw != "" {
		after, err = uuid.Parse(raw)
		if err != nil || after == uuid.Nil {
			connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
			return uuid.Nil, 0, false
		}
	}
	return after, limit, true
}
func journalParamID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil || id == uuid.Nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return uuid.Nil, false
	}
	return id, true
}
func (h *workJournalHandler) records(c *gin.Context) {
	after, limit, ok := journalPageQuery(c)
	if !ok {
		return
	}
	s, scope, ok := h.restore(c)
	if !ok {
		return
	}
	rows, err := s.ListRecords(c.Request.Context(), scope, c.Query("kind"), after, limit)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var next *uuid.UUID
	if len(rows) == limit {
		id := rows[len(rows)-1].ID
		next = &id
	}
	response.Success(c, gin.H{"items": rows, "next_after_id": next})
}
func (h *workJournalHandler) record(c *gin.Context) {
	id, ok := journalParamID(c, "recordID")
	if !ok {
		return
	}
	s, scope, ok := h.restore(c)
	if !ok {
		return
	}
	row, err := s.GetRecord(c.Request.Context(), scope, id)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, gin.H{"record": row})
}
func (h *workJournalHandler) history(c *gin.Context) {
	id, ok := journalParamID(c, "recordID")
	if !ok {
		return
	}
	_, limit, ok := journalPageQuery(c)
	if !ok {
		return
	}
	var after uint64
	var err error
	if raw := c.Query("after_revision"); raw != "" {
		after, err = strconv.ParseUint(raw, 10, 64)
		if err != nil {
			connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
			return
		}
	}
	s, scope, ok := h.restore(c)
	if !ok {
		return
	}
	rows, err := s.ListRecordVersions(c.Request.Context(), scope, id, after, limit)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var next *uint64
	if len(rows) == limit {
		revision := rows[len(rows)-1].Revision
		next = &revision
	}
	response.Success(c, gin.H{"items": rows, "next_after_revision": next})
}
func (h *workJournalHandler) operations(c *gin.Context) {
	after, limit, ok := journalPageQuery(c)
	if !ok {
		return
	}
	s, scope, ok := h.restore(c)
	if !ok {
		return
	}
	rows, err := s.ListOperations(c.Request.Context(), scope, c.Query("state"), after, limit)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var next *uuid.UUID
	if len(rows) == limit {
		id := rows[len(rows)-1].ID
		next = &id
	}
	response.Success(c, gin.H{"items": rows, "next_after_id": next})
}
func (h *workJournalHandler) operation(c *gin.Context) {
	id, ok := journalParamID(c, "operationID")
	if !ok {
		return
	}
	s, scope, ok := h.restore(c)
	if !ok {
		return
	}
	row, err := s.GetOperation(c.Request.Context(), scope, id)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, gin.H{"operation": row})
}
