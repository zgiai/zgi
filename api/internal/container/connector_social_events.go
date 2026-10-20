package container

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/socialevents"
)

func (c *ServiceContainer) GetConnectorSocialEventsService() *socialevents.Service {
	if c.connectorSocialEvents == nil {
		service, err := socialevents.New(c.db, c.GetConnectorCredentialSealer(), socialevents.Options{FacebookAppSecret: c.config.Connectors.SocialWebhookFacebookAppSecret, FacebookVerifyToken: c.config.Connectors.SocialWebhookFacebookVerifyToken})
		if err != nil {
			panic(fmt.Sprintf("initialize social event receiver: %v", err))
		}
		c.connectorSocialEvents = service
	}
	return c.connectorSocialEvents
}

// Defer event-store construction until an authorized native invocation. Provider
// construction and candidate verification must not query connection storage.
type connectorSocialEventStore struct{ container *ServiceContainer }

func (s *connectorSocialEventStore) List(ctx context.Context, org, owner, conn uuid.UUID, after int64, limit int) (socialevents.Page, error) {
	return s.container.GetConnectorSocialEventsService().List(ctx, org, owner, conn, after, limit)
}
func (s *connectorSocialEventStore) Acknowledge(ctx context.Context, org, owner, conn uuid.UUID, through int64) error {
	return s.container.GetConnectorSocialEventsService().Acknowledge(ctx, org, owner, conn, through)
}
