package container

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	journaltools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/workjournal"
)

// The journal is independent of social configuration and available to ordinary
// scheduled employees. Discovery remains lazy and does not touch the database.
func (c *ServiceContainer) GetWorkJournalService() (*workjournal.Service, error) {
	c.workJournalOnce.Do(func() { c.workJournal, c.workJournalErr = workjournal.New(c.db, nil) })
	return c.workJournal, c.workJournalErr
}
func (c *ServiceContainer) newWorkJournalProvider() tools.ToolProvider {
	return journaltools.NewProvider(c.GetWorkJournalService, &workJournalRuntime{container: c})
}

type workJournalRuntime struct{ container *ServiceContainer }

func (r *workJournalRuntime) restore(ctx context.Context, i journaltools.InvocationIdentity) (workjournal.Scope, ActiveBackgroundTaskRun, error) {
	if r == nil || r.container == nil || r.container.db == nil || i.Runtime == nil || i.Runtime.InvokeFrom != tools.ToolInvokeFromDigitalHuman {
		return workjournal.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	org, e1 := uuid.Parse(i.Runtime.TenantID)
	conversation, e2 := uuid.Parse(i.ConversationID)
	employee, e3 := uuid.Parse(i.EmployeeID)
	message, e4 := uuid.Parse(i.MessageID)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || org == uuid.Nil || conversation == uuid.Nil || employee == uuid.Nil || message == uuid.Nil {
		return workjournal.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	var runs []digitalhuman.Run
	err := r.container.db.WithContext(ctx).Select("digital_human_version_id").Where("organization_id = ? AND conversation_id = ? AND message_id = ? AND digital_human_id = ? AND surface = ? AND status = ? AND cancel_requested_at IS NULL", org, conversation, message, employee, digitalhuman.SurfaceTask, digitalhuman.RunStatusRunning).Limit(2).Find(&runs).Error
	if err != nil {
		return workjournal.Scope{}, ActiveBackgroundTaskRun{}, backgroundInternal("restore employee work journal run", err)
	}
	if len(runs) != 1 {
		return workjournal.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	restored, err := r.container.GetBackgroundTaskRunResolver().Restore(ctx, ActiveBackgroundTaskRunRequest{OrganizationID: org, ConversationID: conversation, MessageID: &message, EmployeeID: employee, VersionID: runs[0].DigitalHumanVersionID})
	if err != nil {
		return workjournal.Scope{}, ActiveBackgroundTaskRun{}, err
	}
	if i.ActorID != restored.OwnerID.String() {
		return workjournal.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	return workjournal.Scope{OrganizationID: org, WorkspaceID: restored.WorkspaceID, OwnerID: restored.OwnerID, EmployeeID: employee}, restored, nil
}
func (r *workJournalRuntime) ResolveWorkJournal(ctx context.Context, i journaltools.InvocationIdentity) (journaltools.Session, error) {
	scope, restored, err := r.restore(ctx, i)
	if err != nil {
		return journaltools.Session{}, err
	}
	gateway := &workJournalGateway{restored: restored, resolver: r.container.GetConnectorDigitalHumanRuntimeResolver(), dispatcher: r.container.GetConnectorSurfaceDispatcher(), audit: r.container.GetConnectorAuditRepository()}
	return journaltools.Session{Scope: scope, EmployeeVersionID: restored.Run.DigitalHumanVersionID, Gateway: gateway}, nil
}
