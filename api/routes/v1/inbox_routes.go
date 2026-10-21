package v1

import (
	"context"

	"github.com/gin-gonic/gin"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/inbox"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// InboxRouteDeps are the dependencies of the in-app inbox.
type InboxRouteDeps struct {
	Service                 *inbox.Service
	AccountService          interfaces.AccountService
	ApplicationErrorCatalog *appcatalog.Catalog
}

// RegisterInboxRoutes exposes the signed-in account's inbox.
func RegisterInboxRoutes(v1 *gin.RouterGroup, deps InboxRouteDeps) {
	if deps.Service == nil || deps.AccountService == nil {
		panic("inbox routes require inbox and account services")
	}
	projector, err := apptransport.NewProjector(deps.ApplicationErrorCatalog)
	if err != nil {
		panic("inbox routes require application error catalog")
	}
	group := v1.Group("")
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	inbox.NewHandler(deps.Service, projector).RegisterRoutes(group)
}

// digitalHumanReportInbox puts final DigitalHuman task reports in their
// requester's inbox. A failed run is its own event so the inbox can show it as
// needing attention.
type digitalHumanReportInbox struct{ inbox *inbox.Service }

func (n digitalHumanReportInbox) NotifyTaskReport(ctx context.Context, notice digitalhuman.TaskReportNotice) error {
	kind := inbox.RecipientAccount
	if notice.RecipientKind == inbox.RecipientAPIEndUser {
		kind = inbox.RecipientAPIEndUser
	}
	event := "digital_human.task_report"
	if notice.Outcome == digitalhuman.TaskReportFailed {
		event = "digital_human.task_failed"
	}
	workspaceID := notice.WorkspaceID
	_, err := n.inbox.Notify(ctx, inbox.NotifyRequest{
		Recipient:   inbox.Recipient{OrganizationID: notice.OrganizationID, Kind: kind, ID: notice.RecipientID},
		WorkspaceID: &workspaceID, Event: event, SourceType: "digital_human_task_report", SourceID: notice.ReportID,
		Title: notice.TaskName, Summary: notice.Summary,
		Link: "/console/digital-humans/" + notice.DigitalHumanID.String() + "/tasks?report=" + notice.ReportID.String(),
		Context: map[string]any{
			"digital_human_id": notice.DigitalHumanID.String(), "task_id": notice.TaskID.String(), "outcome": string(notice.Outcome),
		},
	})
	return err
}

var _ digitalhuman.TaskReportNotifier = digitalHumanReportInbox{}
