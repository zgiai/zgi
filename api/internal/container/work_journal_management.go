package container

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

// WorkJournalManagement restores the authenticated viewer's own journal scope.
// History remains readable after a version is unpublished. Management exposes
// no operation state mutation or caller-authored receipt endpoint.
type WorkJournalManagement struct {
	employees backgroundEmployeeReader
	service   func() (*workjournal.Service, error)
}

func (c *ServiceContainer) GetWorkJournalManagement() *WorkJournalManagement {
	return &WorkJournalManagement{employees: c.GetDigitalHumanService(), service: c.GetWorkJournalService}
}
func (s *WorkJournalManagement) ForEmployee(ctx context.Context, scope digitalhuman.Scope, id uuid.UUID) (*workjournal.Service, workjournal.Scope, error) {
	if s == nil || s.employees == nil || s.service == nil || id == uuid.Nil || scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.ActorID == uuid.Nil || (scope.Surface != "" && scope.Surface != digitalhuman.SurfaceConsole) || (scope.PrincipalType != "" && scope.PrincipalType != string(invocation.PrincipalAccount)) || (scope.PrincipalID != uuid.Nil && scope.PrincipalID != scope.ActorID) {
		return nil, workjournal.Scope{}, backgroundDenied(nil)
	}
	detail, err := s.employees.Get(ctx, scope, id)
	if err != nil {
		return nil, workjournal.Scope{}, err
	}
	if detail == nil || detail.Definition == nil || detail.Definition.ID != id || detail.Definition.OrganizationID != scope.OrganizationID || detail.Definition.WorkspaceID != scope.WorkspaceID {
		return nil, workjournal.Scope{}, backgroundDenied(nil)
	}
	journal, err := s.service()
	if err != nil {
		return nil, workjournal.Scope{}, err
	}
	return journal, workjournal.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, OwnerID: scope.ActorID, EmployeeID: id}, nil
}
