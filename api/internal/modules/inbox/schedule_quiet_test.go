package inbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDigestsWaitForQuietHoursAndUnscheduledDeliveriesFail(t *testing.T) {
	service, channels, dispatcher, _, _, im, plan := scheduleFixture(t)
	ctx := t.Context()
	owner := ChannelOwner{OrganizationID: uuid.New(), Kind: OwnerAccount, ID: uuid.New()}
	if err := service.db.Exec("INSERT INTO accounts (id, timezone) VALUES (?, 'Asia/Shanghai')", owner.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	connectionID := uuid.New()
	im.connections = []IMConnection{{ID: connectionID, ConnectorID: "dingtalk", RecipientKind: "user_id"}}
	channel, _, err := channels.Create(ctx, owner, owner.ID, ChannelInput{Type: ChannelIM, Target: "user1", Events: []string{EventTaskReport, EventTaskFailed}, ConnectionID: &connectionID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := channels.Verify(ctx, owner, channel.ID, imCodePattern.FindString(im.sent[0].text)); err != nil {
		t.Fatal(err)
	}
	start, end := 8*60, 10*60
	if _, err := channels.SetQuietHours(ctx, owner, &start, &end); err != nil {
		t.Fatal(err)
	}
	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 23, 7, 0, 0, 0, shanghai).UTC()
	dispatcher.now = func() time.Time { return now }
	recipient := Recipient{OrganizationID: owner.OrganizationID, Kind: RecipientAccount, ID: owner.ID}
	notify := func(event string, context map[string]any) *Notification {
		t.Helper()
		notification, err := service.Notify(ctx, NotifyRequest{Recipient: recipient, Event: event, SourceType: "digital_human_task_report", SourceID: uuid.New(),
			Title: "Daily check", Summary: "All good", Link: "/console", Context: context})
		if err != nil {
			t.Fatal(err)
		}
		return notification
	}

	// The 09:00 digest falls inside 08:00-10:00 quiet hours and waits for 10:00.
	notify("digital_human.task_report", map[string]any{"digest": DigestDaily})
	if wake := time.Date(2026, 10, 23, 10, 0, 0, 0, shanghai).UTC(); len(plan.dues) != 1 || !plan.dues[0].Equal(wake) {
		t.Fatalf("digest during quiet hours is due %v", plan.dues)
	}

	// A delivery no task will pick up is failed where it can be sent again.
	dispatcher.UseScheduler(Scheduler{
		DeliverAt: func(context.Context, uuid.UUID, time.Time) error { return errors.New("queue unavailable") },
		Digest:    func(context.Context, uuid.UUID, time.Time) error { return errors.New("queue unavailable") },
	})
	notify("digital_human.task_report", map[string]any{"digest": DigestDaily})
	var failed []Delivery
	if err := service.db.Where("status = ?", DeliveryFailed).Find(&failed).Error; err != nil || len(failed) != 1 ||
		failed[0].LastError == nil || *failed[0].LastError != "schedule delivery: queue unavailable" {
		t.Fatalf("unscheduled deliveries = %#v, %v", failed, err)
	}
	if err := dispatcher.Redeliver(ctx, owner, channel.ID, failed[0].ID); err != nil {
		t.Fatalf("redeliver an unscheduled delivery: %v", err)
	}
}
