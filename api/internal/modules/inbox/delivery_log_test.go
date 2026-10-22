package inbox

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestDeliveryLogShowsFailuresAndRedelivers(t *testing.T) {
	service, channels, dispatcher, _, queued := newChannelFixture(t)
	ctx := t.Context()
	status := http.StatusInternalServerError
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	defer server.Close()

	organization := uuid.New()
	me := ChannelOwner{OrganizationID: organization, Kind: OwnerAccount, ID: uuid.New()}
	hook, _, err := channels.Create(ctx, me, me.ID, ChannelInput{Type: ChannelWebhook, Target: server.URL, Events: []string{EventTaskReport}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Notify(ctx, NotifyRequest{Recipient: Recipient{OrganizationID: organization, Kind: RecipientAccount, ID: me.ID},
		Event: "digital_human.task_report", SourceType: "digital_human_task_report", SourceID: uuid.New(), Title: "Weekly summary"}); err != nil {
		t.Fatal(err)
	}
	deliveryID := (*queued)[0]
	for range MaxDeliveryAttempts {
		_ = dispatcher.Deliver(ctx, deliveryID)
	}
	items, err := dispatcher.ListDeliveries(ctx, me, hook.ID, 0)
	if err != nil || len(items) != 1 || items[0].Status != DeliveryFailed || items[0].Event != "report.created" || items[0].Title != "Weekly summary" || items[0].LastError == "" {
		t.Fatalf("delivery log = %#v, %v", items, err)
	}

	// Someone else's channel stays out of reach.
	stranger := ChannelOwner{OrganizationID: organization, Kind: OwnerAccount, ID: uuid.New()}
	if _, err := dispatcher.ListDeliveries(ctx, stranger, hook.ID, 0); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("stranger reads the log: %v", err)
	}

	// Once the receiver is fixed, the owner sends it again.
	status = http.StatusOK
	if err := dispatcher.Redeliver(ctx, me, hook.ID, deliveryID); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Deliver(ctx, (*queued)[len(*queued)-1]); err != nil {
		t.Fatal(err)
	}
	items, _ = dispatcher.ListDeliveries(ctx, me, hook.ID, 0)
	if items[0].Status != DeliverySent || items[0].Attempts != 1 {
		t.Fatalf("after redelivery = %#v", items[0])
	}
	if err := dispatcher.Redeliver(ctx, me, hook.ID, deliveryID); !errors.Is(err, ErrChannelConflict) {
		t.Fatalf("redelivering a sent delivery: %v", err)
	}
}
