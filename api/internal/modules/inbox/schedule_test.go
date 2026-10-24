package inbox

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type sentIM struct {
	connectionID      uuid.UUID
	recipient, text   string
	ownerID           uuid.UUID
	idempotencyKey    string
	organizationScope uuid.UUID
}

type fakeIM struct {
	connections []IMConnection
	sent        []sentIM
	fail        error
}

func (f *fakeIM) Connections(context.Context, uuid.UUID, uuid.UUID) ([]IMConnection, error) {
	return f.connections, nil
}

func (f *fakeIM) Send(_ context.Context, organizationID, ownerID, connectionID uuid.UUID, recipient, text, key string) error {
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, sentIM{connectionID: connectionID, recipient: recipient, text: text, ownerID: ownerID, idempotencyKey: key, organizationScope: organizationID})
	return nil
}

type scheduled struct {
	deliveries []uuid.UUID
	at         []time.Time
	digests    []uuid.UUID
	dues       []time.Time
}

func scheduleFixture(t *testing.T) (*Service, *Channels, *Dispatcher, *[]sentEmail, *[]uuid.UUID, *fakeIM, *scheduled) {
	t.Helper()
	service, channels, dispatcher, emails, queued := newChannelFixture(t)
	if err := service.db.AutoMigrate(&Preferences{}); err != nil {
		t.Fatal(err)
	}
	if err := service.db.Exec(`CREATE TABLE accounts (id text PRIMARY KEY, timezone text)`).Error; err != nil {
		t.Fatal(err)
	}
	im := &fakeIM{}
	channels.UseIMSender(im)
	plan := &scheduled{}
	dispatcher.UseScheduler(Scheduler{
		DeliverAt: func(_ context.Context, id uuid.UUID, at time.Time) error {
			plan.deliveries, plan.at = append(plan.deliveries, id), append(plan.at, at)
			return nil
		},
		Digest: func(_ context.Context, channelID uuid.UUID, due time.Time) error {
			plan.digests, plan.dues = append(plan.digests, channelID), append(plan.dues, due)
			return nil
		},
	})
	return service, channels, dispatcher, emails, queued, im, plan
}

var imCodePattern = regexp.MustCompile(`(\d{6})`)

func TestQuietHoursAndDigestTimes(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 22*60, 8*60
	quiet := Preferences{QuietStart: &start, QuietEnd: &end}
	at := func(hour, minute int) time.Time {
		return time.Date(2026, 10, 23, hour, minute, 0, 0, shanghai).UTC()
	}
	if until := quietUntil(quiet, shanghai, at(23, 30)); !until.Equal(time.Date(2026, 10, 24, 8, 0, 0, 0, shanghai).UTC()) {
		t.Fatalf("quiet at 23:30 lasts until %v", until)
	}
	if until := quietUntil(quiet, shanghai, at(6, 0)); !until.Equal(time.Date(2026, 10, 23, 8, 0, 0, 0, shanghai).UTC()) {
		t.Fatalf("quiet at 06:00 lasts until %v", until)
	}
	if until := quietUntil(quiet, shanghai, at(12, 0)); !until.IsZero() {
		t.Fatalf("noon is quiet until %v", until)
	}
	if until := quietUntil(Preferences{}, shanghai, at(23, 30)); !until.IsZero() {
		t.Fatal("no quiet hours delayed a delivery")
	}
	// 2026-10-23 is a Friday.
	if due := nextDigestAt(DigestDaily, shanghai, at(8, 0)); !due.Equal(at(9, 0)) {
		t.Fatalf("daily digest before nine = %v", due)
	}
	if due := nextDigestAt(DigestDaily, shanghai, at(10, 0)); !due.Equal(time.Date(2026, 10, 24, 9, 0, 0, 0, shanghai).UTC()) {
		t.Fatalf("daily digest after nine = %v", due)
	}
	if due := nextDigestAt(DigestWeekly, shanghai, at(10, 0)); !due.Equal(time.Date(2026, 10, 26, 9, 0, 0, 0, shanghai).UTC()) {
		t.Fatalf("weekly digest = %v", due)
	}
}

func TestIMChannelIsVerifiedThroughTheOwnersConnection(t *testing.T) {
	_, channels, dispatcher, _, _, im, _ := scheduleFixture(t)
	ctx := t.Context()
	owner := ChannelOwner{OrganizationID: uuid.New(), Kind: OwnerAccount, ID: uuid.New()}
	connectionID := uuid.New()
	im.connections = []IMConnection{{ID: connectionID, ConnectorID: "feishu", RecipientKind: "open_id"}}
	if _, _, err := channels.Create(ctx, owner, owner.ID, ChannelInput{Type: ChannelIM, Target: "ou_me", Events: []string{EventTaskReport}}); !errors.Is(err, ErrDeliveryUnavailable) {
		t.Fatalf("an IM channel without a connection: %v", err)
	}
	other := uuid.New()
	if _, _, err := channels.Create(ctx, owner, owner.ID, ChannelInput{Type: ChannelIM, Target: "ou_me", Events: []string{EventTaskReport}, ConnectionID: &other}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("an IM channel with someone else's connection: %v", err)
	}
	team := ChannelOwner{OrganizationID: owner.OrganizationID, Kind: OwnerWorkspace, ID: uuid.New()}
	if _, _, err := channels.Create(ctx, team, owner.ID, ChannelInput{Type: ChannelIM, Target: "ou_me", Events: []string{EventTaskReport}, ConnectionID: &connectionID}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("a team IM channel: %v", err)
	}
	channel, _, err := channels.Create(ctx, owner, owner.ID, ChannelInput{Type: ChannelIM, Target: "ou_me", Events: []string{EventTaskReport}, ConnectionID: &connectionID})
	if err != nil || channel.Status != ChannelPending || len(im.sent) != 1 || im.sent[0].recipient != "ou_me" || im.sent[0].connectionID != connectionID {
		t.Fatalf("create = %#v, %v, sent %#v", channel, err, im.sent)
	}
	code := imCodePattern.FindString(im.sent[0].text)
	if verified, err := channels.Verify(ctx, owner, channel.ID, code); err != nil || verified.Status != ChannelActive {
		t.Fatalf("verify = %#v, %v", verified, err)
	}
	if err := dispatcher.SendTest(ctx, owner, channel.ID); err != nil || len(im.sent) != 2 || !strings.Contains(im.sent[1].text, "测试通知") {
		t.Fatalf("test message = %v, %#v", err, im.sent)
	}
}

func TestDigestsHoldReportsAndQuietHoursDelayPersonalChannels(t *testing.T) {
	service, channels, dispatcher, emails, queued, im, plan := scheduleFixture(t)
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
	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 23, 23, 0, 0, 0, shanghai).UTC()
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

	// Two reports of a daily-digest task are held for 09:00 the next day.
	notify("digital_human.task_report", map[string]any{"digest": DigestDaily, "outcome": "done"})
	notify("digital_human.task_report", map[string]any{"digest": DigestDaily, "outcome": "partial"})
	due := time.Date(2026, 10, 24, 9, 0, 0, 0, shanghai).UTC()
	var held []Delivery
	if err := service.db.Where("status = ?", DeliveryHeld).Find(&held).Error; err != nil || len(held) != 2 || !held[0].DigestDueAt.Equal(due) {
		t.Fatalf("held deliveries = %#v, %v", held, err)
	}
	if len(plan.digests) != 2 || plan.digests[0] != channel.ID || !plan.dues[0].Equal(due) || len(*queued) != 0 {
		t.Fatalf("digest schedule = %#v, queued %d", plan, len(*queued))
	}

	// A failure is never held, but quiet hours delay it to 08:00.
	start, end := 22*60, 8*60
	if _, err := channels.SetQuietHours(ctx, owner, &start, &end); err != nil {
		t.Fatal(err)
	}
	notify("digital_human.task_failed", map[string]any{"digest": DigestDaily})
	wake := time.Date(2026, 10, 24, 8, 0, 0, 0, shanghai).UTC()
	if len(plan.deliveries) != 1 || !plan.at[0].Equal(wake) {
		t.Fatalf("quiet delivery schedule = %#v", plan)
	}

	// The digest sends both reports as one message and marks them sent.
	before := len(im.sent)
	if err := dispatcher.SendDigest(ctx, channel.ID, due, false); err != nil {
		t.Fatal(err)
	}
	if len(im.sent) != before+1 || !strings.Contains(im.sent[before].text, "每日任务汇总") || strings.Count(im.sent[before].text, "Daily check") != 2 {
		t.Fatalf("digest message = %#v", im.sent[before:])
	}
	var sent int64
	if err := service.db.Model(&Delivery{}).Where("status = ? AND digest_due_at IS NOT NULL", DeliverySent).Count(&sent).Error; err != nil || sent != 2 {
		t.Fatalf("sent digest deliveries = %d, %v", sent, err)
	}
	if err := dispatcher.SendDigest(ctx, channel.ID, due, false); err != nil || len(im.sent) != before+1 {
		t.Fatal("a digest was sent twice")
	}
	_ = emails
}

func TestQuietHoursArePerAccount(t *testing.T) {
	service, channels, _, _, _, _, _ := scheduleFixture(t)
	ctx := t.Context()
	owner := ChannelOwner{OrganizationID: uuid.New(), Kind: OwnerAccount, ID: uuid.New()}
	if err := service.db.Exec("INSERT INTO accounts (id, timezone) VALUES (?, 'Asia/Shanghai')", owner.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	start, end := 22*60, 8*60
	view, err := channels.SetQuietHours(ctx, owner, &start, &end)
	if err != nil || *view.QuietStart != start || *view.QuietEnd != end || view.Timezone != "Asia/Shanghai" {
		t.Fatalf("set = %#v, %v", view, err)
	}
	same := start
	if _, err := channels.SetQuietHours(ctx, owner, &start, &same); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("an empty window: %v", err)
	}
	if _, err := channels.SetQuietHours(ctx, owner, &start, nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("half a window: %v", err)
	}
	cleared, err := channels.SetQuietHours(ctx, owner, nil, nil)
	if err != nil || cleared.QuietStart != nil {
		t.Fatalf("clear = %#v, %v", cleared, err)
	}
	// An account without a time zone uses UTC.
	unzoned := ChannelOwner{OrganizationID: owner.OrganizationID, Kind: OwnerAccount, ID: uuid.New()}
	if err := service.db.Exec("INSERT INTO accounts (id, timezone) VALUES (?, NULL)", unzoned.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if view, err := channels.GetPreferences(ctx, unzoned); err != nil || view.Timezone != "UTC" {
		t.Fatalf("an account without a time zone = %#v, %v", view, err)
	}
}
