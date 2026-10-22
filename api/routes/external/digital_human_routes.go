package external

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/inbox"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// RegisterDigitalHumanExternalRoutes keeps DigitalHuman's independently
// authenticated routes outside the legacy Agent API-key middleware group.
func RegisterDigitalHumanExternalRoutes(
	router *gin.RouterGroup,
	runtime *digitalhuman.RuntimeService,
	access *digitalhuman.AccessService,
	tasks digitalhuman.TaskHandlerService,
	fileService interfaces.FileService,
	catalog *appcatalog.Catalog,
	notifications *inbox.Service,
) {
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		panic("digital human external routes require application error projector")
	}
	var sessions *digitalhuman.WebAppSessionSigner
	if current := config.Current(); current != nil {
		sessions, _ = digitalhuman.NewWebAppSessionSigner(current.JWT.Secret)
	}
	handler := digitalhuman.NewExternalHandler(runtime, access, tasks, fileService, projector, sessions)
	if notifications != nil {
		handler.WithEndUserNotifications(endUserInbox{inbox: notifications})
	}
	handler.RegisterAPIRoutes(router)
	handler.RegisterWebAppRoutes(router)
}

// endUserInbox lets API end users read the inbox entries of their own tasks.
type endUserInbox struct{ inbox *inbox.Service }

func (n endUserInbox) ListEndUserNotifications(ctx context.Context, organizationID uuid.UUID, subjectKind string, subjectID uuid.UUID, unreadOnly bool, page, pageSize int) (any, error) {
	return n.inbox.List(ctx, inbox.Recipient{OrganizationID: organizationID, Kind: subjectKind, ID: subjectID}, inbox.ListQuery{UnreadOnly: unreadOnly, Page: page, PageSize: pageSize})
}

func (n endUserInbox) EndUserUnreadCount(ctx context.Context, organizationID uuid.UUID, subjectKind string, subjectID uuid.UUID) (int64, error) {
	return n.inbox.UnreadCount(ctx, inbox.Recipient{OrganizationID: organizationID, Kind: subjectKind, ID: subjectID})
}

func (n endUserInbox) MarkAllEndUserNotificationsRead(ctx context.Context, organizationID uuid.UUID, subjectKind string, subjectID uuid.UUID) error {
	return n.inbox.MarkAllRead(ctx, inbox.Recipient{OrganizationID: organizationID, Kind: subjectKind, ID: subjectID})
}

func (n endUserInbox) MarkEndUserNotificationsRead(ctx context.Context, organizationID uuid.UUID, subjectKind string, subjectID uuid.UUID, ids []uuid.UUID, reportID *uuid.UUID) error {
	recipient := inbox.Recipient{OrganizationID: organizationID, Kind: subjectKind, ID: subjectID}
	if reportID != nil {
		if err := n.inbox.MarkSourceRead(ctx, recipient, "digital_human_task_report", *reportID); err != nil {
			return err
		}
	}
	if len(ids) > 0 {
		return n.inbox.MarkRead(ctx, recipient, ids)
	}
	return nil
}
