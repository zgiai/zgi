package inbox

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestInbox(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:inbox-"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Notification{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX idx_inbox_notifications_source ON inbox_notifications (recipient_kind, recipient_id, event, source_type, source_id)`).Error; err != nil {
		t.Fatal(err)
	}
	return NewService(db)
}

func TestInboxStoresEachEventOnceAndTracksReadState(t *testing.T) {
	service := newTestInbox(t)
	ctx := t.Context()
	me := Recipient{OrganizationID: uuid.New(), Kind: RecipientAccount, ID: uuid.New()}
	someoneElse := Recipient{OrganizationID: me.OrganizationID, Kind: RecipientAccount, ID: uuid.New()}
	report := uuid.New()
	notify := func(recipient Recipient, event string, source uuid.UUID) {
		t.Helper()
		if _, err := service.Notify(ctx, NotifyRequest{Recipient: recipient, Event: event, SourceType: "digital_human_task_report", SourceID: source,
			Title: "Weekly summary", Summary: "Done", Link: "/console", Context: map[string]any{"outcome": "done"}}); err != nil {
			t.Fatal(err)
		}
	}
	notify(me, "digital_human.task_report", report)
	notify(me, "digital_human.task_report", report) // a redelivered event is ignored
	notify(me, "digital_human.task_failed", uuid.New())
	notify(someoneElse, "digital_human.task_report", report)

	list, err := service.List(ctx, me, ListQuery{})
	if err != nil || list.Total != 2 || list.UnreadCount != 2 || string(list.Items[0].Context) == "" {
		t.Fatalf("list = %#v, %v", list, err)
	}
	if err := service.MarkSourceRead(ctx, me, "digital_human_task_report", report); err != nil {
		t.Fatal(err)
	}
	if count, err := service.UnreadCount(ctx, me); err != nil || count != 1 {
		t.Fatalf("unread after opening the report = %d, %v", count, err)
	}
	unread, err := service.List(ctx, me, ListQuery{UnreadOnly: true})
	if err != nil || unread.Total != 1 || unread.Items[0].Event != "digital_human.task_failed" {
		t.Fatalf("unread list = %#v, %v", unread, err)
	}
	// Marking someone else's notification does nothing.
	if err := service.MarkRead(ctx, someoneElse, []uuid.UUID{unread.Items[0].ID}); err != nil {
		t.Fatal(err)
	}
	if count, _ := service.UnreadCount(ctx, me); count != 1 {
		t.Fatalf("another reader changed my inbox: %d", count)
	}
	if err := service.MarkAllRead(ctx, me); err != nil {
		t.Fatal(err)
	}
	if count, _ := service.UnreadCount(ctx, me); count != 0 {
		t.Fatalf("unread after read-all = %d", count)
	}
	if count, _ := service.UnreadCount(ctx, someoneElse); count != 1 {
		t.Fatalf("read-all touched another recipient: %d", count)
	}
	if _, err := service.Notify(ctx, NotifyRequest{Recipient: Recipient{OrganizationID: me.OrganizationID, Kind: "channel", ID: uuid.New()}, Event: "x", SourceType: "y", SourceID: uuid.New()}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unsupported recipient: %v", err)
	}
}
