package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
)

// plainCipher stands in for the credential keyring in tests.
type plainCipher struct{}

func (plainCipher) EncryptCredentials(values map[string]string, _ credentialcrypto.CredentialAAD) (string, error) {
	encoded, err := json.Marshal(values)
	return string(encoded), err
}

func (plainCipher) DecryptCredentials(envelope string, _ credentialcrypto.CredentialAAD) (map[string]string, error) {
	var values map[string]string
	err := json.Unmarshal([]byte(envelope), &values)
	return values, err
}

type sentEmail struct{ to, subject, body string }

func newChannelFixture(t *testing.T) (*Service, *Channels, *Dispatcher, *[]sentEmail, *[]uuid.UUID) {
	t.Helper()
	service := newTestInbox(t)
	if err := service.db.AutoMigrate(&Channel{}, &Delivery{}); err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{
		`CREATE UNIQUE INDEX idx_notification_channels_target ON notification_channels (owner_kind, owner_id, type, target)`,
		`CREATE UNIQUE INDEX idx_notification_deliveries_unique ON notification_deliveries (notification_id, channel_id)`,
	} {
		if err := service.db.Exec(index).Error; err != nil {
			t.Fatal(err)
		}
	}
	emails := &[]sentEmail{}
	channels := NewChannels(service.db, plainCipher{}, func(_ context.Context, to, subject, body, _ string) error {
		*emails = append(*emails, sentEmail{to, subject, body})
		return nil
	})
	queued := &[]uuid.UUID{}
	dispatcher := NewDispatcher(service.db, channels, func(_ context.Context, id uuid.UUID) error {
		*queued = append(*queued, id)
		return nil
	}, "https://zgi.example", true)
	service.SetDispatcher(dispatcher)
	return service, channels, dispatcher, emails, queued
}

var codePattern = regexp.MustCompile(`<strong>(\d{6})</strong>`)

func TestEmailChannelNeedsTheCodeSentToIt(t *testing.T) {
	_, channels, _, emails, _ := newChannelFixture(t)
	ctx := t.Context()
	owner := ChannelOwner{OrganizationID: uuid.New(), Kind: OwnerAccount, ID: uuid.New()}
	channel, _, err := channels.Create(ctx, owner, owner.ID, ChannelInput{Type: ChannelEmail, Target: "Me@Example.com", Events: []string{EventTaskFailed}})
	if err != nil || channel.Status != ChannelPending || channel.Target != "me@example.com" || len(*emails) != 1 {
		t.Fatalf("create = %#v, %v, emails %d", channel, err, len(*emails))
	}
	code := codePattern.FindStringSubmatch((*emails)[0].body)[1]
	if _, err := channels.Update(ctx, owner, channel.ID, ChannelUpdate{Enabled: boolPointer(true)}); !errors.Is(err, ErrChannelConflict) {
		t.Fatalf("enabling an unverified email: %v", err)
	}
	if err := channels.ResendCode(ctx, owner, channel.ID); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("resend within a minute: %v", err)
	}
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	if _, err := channels.Verify(ctx, owner, channel.ID, wrong); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("wrong code: %v", err)
	}
	stranger := ChannelOwner{OrganizationID: owner.OrganizationID, Kind: OwnerAccount, ID: uuid.New()}
	if _, err := channels.Verify(ctx, stranger, channel.ID, code); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("another account verifying: %v", err)
	}
	verified, err := channels.Verify(ctx, owner, channel.ID, code)
	if err != nil || verified.Status != ChannelActive || verified.VerifiedAt == nil {
		t.Fatalf("verify = %#v, %v", verified, err)
	}
	if _, _, err := channels.Create(ctx, owner, owner.ID, ChannelInput{Type: ChannelEmail, Target: "me@example.com", Events: []string{EventTaskFailed}}); !errors.Is(err, ErrChannelConflict) {
		t.Fatalf("duplicate address: %v", err)
	}
	team := ChannelOwner{OrganizationID: owner.OrganizationID, Kind: OwnerWorkspace, ID: uuid.New()}
	if _, _, err := channels.Create(ctx, team, owner.ID, ChannelInput{Type: ChannelEmail, Target: "team@example.com", Events: []string{EventTaskFailed}}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("team email channel: %v", err)
	}
	if _, _, err := channels.Create(ctx, owner, owner.ID, ChannelInput{Type: ChannelEmail, Target: "x@example.com", Events: []string{"everything"}}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown event: %v", err)
	}
}

func TestNotificationsReachSubscribedChannelsSigned(t *testing.T) {
	service, channels, dispatcher, emails, queued := newChannelFixture(t)
	ctx := t.Context()
	type received struct {
		signature, timestamp string
		body                 []byte
	}
	var hooks []received
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		hooks = append(hooks, received{r.Header.Get("X-ZGI-Signature"), r.Header.Get("X-ZGI-Timestamp"), body})
		w.WriteHeader(status)
	}))
	defer server.Close()

	organization := uuid.New()
	me := ChannelOwner{OrganizationID: organization, Kind: OwnerAccount, ID: uuid.New()}
	hook, secret, err := channels.Create(ctx, me, me.ID, ChannelInput{Type: ChannelWebhook, Target: server.URL, Events: []string{EventTaskReport, EventTaskFailed}})
	if err != nil || hook.Status != ChannelActive || secret == "" {
		t.Fatalf("webhook = %#v, %q, %v", hook, secret, err)
	}
	mail, _, err := channels.Create(ctx, me, me.ID, ChannelInput{Type: ChannelEmail, Target: "me@example.com", Events: []string{EventTaskFailed}})
	if err != nil {
		t.Fatal(err)
	}
	code := codePattern.FindStringSubmatch((*emails)[0].body)[1]
	if _, err := channels.Verify(ctx, me, mail.ID, code); err != nil {
		t.Fatal(err)
	}
	report := uuid.New()
	notify := func(recipient Recipient, event string, source uuid.UUID, context map[string]any) {
		t.Helper()
		if _, err := service.Notify(ctx, NotifyRequest{Recipient: recipient, Event: event, SourceType: "digital_human_task_report", SourceID: source,
			Title: "Weekly summary", Summary: "Done", Link: "/console/digital-humans/x/tasks?report=" + source.String(), Context: context}); err != nil {
			t.Fatal(err)
		}
	}
	// A report reaches the webhook only; the email subscribes to failures.
	notify(Recipient{OrganizationID: organization, Kind: RecipientAccount, ID: me.ID}, "digital_human.task_report", report, map[string]any{"task_id": "t1", "outcome": "done"})
	if len(*queued) != 1 {
		t.Fatalf("queued deliveries = %d, want the webhook only", len(*queued))
	}
	if err := dispatcher.Deliver(ctx, (*queued)[0]); err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 || hooks[0].signature != "sha256="+SignWebhook(secret, hooks[0].timestamp, hooks[0].body) {
		t.Fatalf("webhook calls = %#v", hooks)
	}
	var payload WebhookPayload
	if err := json.Unmarshal(hooks[0].body, &payload); err != nil || payload.Event != "report.created" || payload.Data["report_id"] != report.String() ||
		payload.Data["link"] != "https://zgi.example/console/digital-humans/x/tasks?report="+report.String() {
		t.Fatalf("payload = %#v, %v", payload, err)
	}
	// A failure reaches both channels; the email links to the console.
	notify(Recipient{OrganizationID: organization, Kind: RecipientAccount, ID: me.ID}, "digital_human.task_failed", uuid.New(), nil)
	if len(*queued) != 3 {
		t.Fatalf("queued deliveries = %d, want 3", len(*queued))
	}
	for _, id := range (*queued)[1:] {
		if err := dispatcher.Deliver(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if last := (*emails)[len(*emails)-1]; last.to != "me@example.com" || !regexp.MustCompile(`https://zgi\.example/console`).MatchString(last.body) {
		t.Fatalf("failure email = %#v", last)
	}

	// An API end user's notification goes to the DigitalHuman's webhook.
	digitalHuman := ChannelOwner{OrganizationID: organization, Kind: OwnerDigitalHuman, ID: uuid.New()}
	if _, _, err := channels.Create(ctx, digitalHuman, me.ID, ChannelInput{Type: ChannelWebhook, Target: server.URL, Events: []string{EventTaskReport}}); err != nil {
		t.Fatal(err)
	}
	notify(Recipient{OrganizationID: organization, Kind: RecipientAPIEndUser, ID: uuid.New()}, "digital_human.task_report", uuid.New(),
		map[string]any{"digital_human_id": digitalHuman.ID.String()})
	if len(*queued) != 4 {
		t.Fatalf("end-user deliveries = %d, want 4", len(*queued))
	}

	// A failing endpoint is retried until the last attempt marks it failed.
	status = http.StatusInternalServerError
	id := (*queued)[3]
	for attempt := 1; attempt < MaxDeliveryAttempts; attempt++ {
		if err := dispatcher.Deliver(ctx, id); err == nil {
			t.Fatalf("attempt %d reported success", attempt)
		}
	}
	if err := dispatcher.Deliver(ctx, id); err != nil {
		t.Fatalf("last attempt asks for another retry: %v", err)
	}
	var delivery Delivery
	if err := service.db.First(&delivery, "id = ?", id).Error; err != nil || delivery.Status != DeliveryFailed || delivery.Attempts != MaxDeliveryAttempts {
		t.Fatalf("delivery after retries = %#v, %v", delivery, err)
	}
}

func boolPointer(value bool) *bool { return &value }

func TestTaskTeamChannelsStayInTheirWorkspace(t *testing.T) {
	service, channels, _, _, queued := newChannelFixture(t)
	ctx := t.Context()
	organization, workspace, elsewhere := uuid.New(), uuid.New(), uuid.New()
	team := func(id uuid.UUID) *Channel {
		t.Helper()
		channel, _, err := channels.Create(ctx, ChannelOwner{OrganizationID: organization, WorkspaceID: &id, Kind: OwnerWorkspace, ID: id}, uuid.New(),
			ChannelInput{Type: ChannelWebhook, Target: "https://hooks.example/" + id.String(), Events: []string{EventTaskReport}})
		if err != nil {
			t.Fatal(err)
		}
		return channel
	}
	ours, theirs := team(workspace), team(elsewhere)
	if ok, err := channels.ActiveWorkspaceChannels(ctx, organization, workspace, []uuid.UUID{ours.ID}); err != nil || !ok {
		t.Fatalf("own channel = %v, %v", ok, err)
	}
	if ok, err := channels.ActiveWorkspaceChannels(ctx, organization, workspace, []uuid.UUID{ours.ID, theirs.ID}); err != nil || ok {
		t.Fatalf("another workspace's channel = %v, %v", ok, err)
	}
	disabled := false
	if _, err := channels.Update(ctx, ChannelOwner{OrganizationID: organization, WorkspaceID: &workspace, Kind: OwnerWorkspace, ID: workspace}, ours.ID, ChannelUpdate{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := channels.ActiveWorkspaceChannels(ctx, organization, workspace, []uuid.UUID{ours.ID}); ok {
		t.Fatal("a disabled channel was accepted")
	}
	enabled := true
	if _, err := channels.Update(ctx, ChannelOwner{OrganizationID: organization, WorkspaceID: &workspace, Kind: OwnerWorkspace, ID: workspace}, ours.ID, ChannelUpdate{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Notify(ctx, NotifyRequest{Recipient: Recipient{OrganizationID: organization, Kind: RecipientAccount, ID: uuid.New()}, WorkspaceID: &workspace,
		Event: "digital_human.task_report", SourceType: "digital_human_task_report", SourceID: uuid.New(), Title: "Weekly", Summary: "Done",
		Context: map[string]any{"channel_ids": []uuid.UUID{ours.ID, theirs.ID}}}); err != nil {
		t.Fatal(err)
	}
	var deliveries []Delivery
	if err := service.db.Find(&deliveries).Error; err != nil || len(*queued) != 1 || len(deliveries) != 1 || deliveries[0].ChannelID != ours.ID {
		t.Fatalf("deliveries = %#v, queued %d, %v", deliveries, len(*queued), err)
	}
}
