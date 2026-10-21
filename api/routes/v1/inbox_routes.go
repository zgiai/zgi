package v1

import (
	"context"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/inbox"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/email"
	"gorm.io/gorm"
)

// InboxRouteDeps are the dependencies of the in-app inbox.
type InboxRouteDeps struct {
	Service                 *inbox.Service
	Channels                *inbox.ChannelHandler
	AccountService          interfaces.AccountService
	ApplicationErrorCatalog *appcatalog.Catalog
}

// newNotificationChannels builds notification channels and their delivery.
// Email channels need configured email; webhooks need credential encryption.
// ZGI_NOTIFICATION_WEBHOOK_ALLOW_PRIVATE lets local development deliver
// webhooks to private addresses and must stay off in production.
func newNotificationChannels(inboxService *inbox.Service, db *gorm.DB, cipher inbox.SecretCipher, registry inbox.TaskRegistry, queue inbox.TaskQueue) (*inbox.Channels, *inbox.Dispatcher) {
	channels := inbox.NewChannels(db, cipher, notificationEmailSender())
	consoleURL := ""
	if current := config.Current(); current != nil {
		consoleURL = current.Email.ConsoleWebURL
	}
	allowPrivate := strings.EqualFold(strings.TrimSpace(os.Getenv("ZGI_NOTIFICATION_WEBHOOK_ALLOW_PRIVATE")), "true")
	var dispatcher *inbox.Dispatcher
	enqueue := inbox.RegisterDeliveryTasks(registry, queue, func() *inbox.Dispatcher { return dispatcher })
	dispatcher = inbox.NewDispatcher(db, channels, enqueue, consoleURL, allowPrivate)
	inboxService.SetDispatcher(dispatcher)
	return channels, dispatcher
}

func notificationEmailSender() inbox.EmailSender {
	current := config.Current()
	if current == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(current.Email.MailType)) {
	case "resend":
		if strings.TrimSpace(current.Email.ResendAPIKey) == "" {
			return nil
		}
	case "smtp":
		if strings.TrimSpace(current.Email.SMTPServer) == "" {
			return nil
		}
	default:
		return nil
	}
	return func(ctx context.Context, to, subject, html, idempotencyKey string) error {
		return email.SendEmailWithOptions(ctx, []string{to}, subject, html, "text/html", email.SendOptions{IdempotencyKey: idempotencyKey})
	}
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
	if deps.Channels != nil {
		deps.Channels.RegisterRoutes(group, "/notification-channels", deps.Channels.AccountOwner)
	}
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
			"channel_ids": notice.ChannelIDs,
		},
	})
	return err
}

var _ digitalhuman.TaskReportNotifier = digitalHumanReportInbox{}
