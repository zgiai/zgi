package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// Digests and quiet hours (ADR 0008 stage 4).
const (
	DigestDaily  = "daily"
	DigestWeekly = "weekly"

	// DigestTaskType sends the held reports of one channel for one digest.
	DigestTaskType = "inbox:notification:digest"

	digestHour       = 9
	digestMaxReports = 50
)

// Preferences are an account's notification settings in one organization.
type Preferences struct {
	OrganizationID uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	AccountID      uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	// QuietStart and QuietEnd are minutes after midnight in the account's
	// time zone; both are nil when quiet hours are off.
	QuietStart *int      `json:"quiet_start"`
	QuietEnd   *int      `json:"quiet_end"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName binds preferences to their table.
func (Preferences) TableName() string { return "notification_preferences" }

// PreferencesView is the preferences with the time zone they apply in.
type PreferencesView struct {
	QuietStart *int   `json:"quiet_start"`
	QuietEnd   *int   `json:"quiet_end"`
	Timezone   string `json:"timezone"`
}

// GetPreferences reads an account's notification preferences.
func (s *Channels) GetPreferences(ctx context.Context, owner ChannelOwner) (*PreferencesView, error) {
	if !owner.valid() || owner.Kind != OwnerAccount {
		return nil, ErrInvalidRequest
	}
	preferences, location, err := loadPreferences(ctx, s.db, owner.OrganizationID, owner.ID)
	if err != nil {
		return nil, err
	}
	return &PreferencesView{QuietStart: preferences.QuietStart, QuietEnd: preferences.QuietEnd, Timezone: location.String()}, nil
}

// SetQuietHours sets or, with both nil, clears an account's quiet hours.
func (s *Channels) SetQuietHours(ctx context.Context, owner ChannelOwner, start, end *int) (*PreferencesView, error) {
	if !owner.valid() || owner.Kind != OwnerAccount || (start == nil) != (end == nil) {
		return nil, ErrInvalidRequest
	}
	if start != nil && (*start < 0 || *start > 1439 || *end < 0 || *end > 1439 || *start == *end) {
		return nil, ErrInvalidRequest
	}
	row := Preferences{OrganizationID: owner.OrganizationID, AccountID: owner.ID, QuietStart: start, QuietEnd: end, UpdatedAt: s.now()}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, fmt.Errorf("save notification preferences: %w", err)
	}
	return s.GetPreferences(ctx, owner)
}

// loadPreferences reads an account's preferences and time zone; UTC when
// the account has none.
func loadPreferences(ctx context.Context, db *gorm.DB, organizationID, accountID uuid.UUID) (Preferences, *time.Location, error) {
	preferences := Preferences{OrganizationID: organizationID, AccountID: accountID}
	var rows []Preferences
	if err := db.WithContext(ctx).Where("organization_id = ? AND account_id = ?", organizationID, accountID).Limit(1).Find(&rows).Error; err != nil {
		return preferences, time.UTC, fmt.Errorf("load notification preferences: %w", err)
	}
	if len(rows) == 1 {
		preferences = rows[0]
	}
	var account struct{ Timezone sql.NullString }
	if err := db.WithContext(ctx).Table("accounts").Select("timezone").Where("id = ?", accountID).Limit(1).Scan(&account).Error; err != nil {
		return preferences, time.UTC, fmt.Errorf("load account time zone: %w", err)
	}
	location := time.UTC
	if zone := strings.TrimSpace(account.Timezone.String); account.Timezone.Valid && zone != "" {
		if loaded, err := time.LoadLocation(zone); err == nil {
			location = loaded
		}
	}
	return preferences, location, nil
}

// quietUntil is when an account's quiet hours end if now is inside them,
// and the zero time otherwise. A window may cross midnight.
func quietUntil(preferences Preferences, location *time.Location, now time.Time) time.Time {
	if preferences.QuietStart == nil || preferences.QuietEnd == nil {
		return time.Time{}
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	start, end := *preferences.QuietStart, *preferences.QuietEnd
	inside := start < end && minute >= start && minute < end || start > end && (minute >= start || minute < end)
	if !inside {
		return time.Time{}
	}
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	until := day.Add(time.Duration(end) * time.Minute)
	if !until.After(local) {
		until = until.AddDate(0, 0, 1)
	}
	return until.UTC()
}

// nextDigestAt is when a digest of the given kind is next sent: 09:00, and
// on Monday for a weekly digest, in the recipient's time zone.
func nextDigestAt(kind string, location *time.Location, now time.Time) time.Time {
	local := now.In(location)
	at := time.Date(local.Year(), local.Month(), local.Day(), digestHour, 0, 0, 0, location)
	if !at.After(local) {
		at = at.AddDate(0, 0, 1)
	}
	if kind == DigestWeekly {
		for at.Weekday() != time.Monday {
			at = at.AddDate(0, 0, 1)
		}
	}
	return at.UTC()
}

// personalChannel reports whether quiet hours apply to a channel: its
// owner's own email and IM.
func personalChannel(channel *Channel) bool {
	return channel.OwnerKind == OwnerAccount && (channel.Type == ChannelEmail || channel.Type == ChannelIM)
}

// Scheduler schedules deliveries for later.
type Scheduler struct {
	// DeliverAt sends a pending delivery at a time.
	DeliverAt func(ctx context.Context, deliveryID uuid.UUID, at time.Time) error
	// Digest sends a channel's held reports of one digest at its time.
	Digest func(ctx context.Context, channelID uuid.UUID, due time.Time) error
}

// RegisterScheduledTasks registers the digest handler and returns the
// scheduler the dispatcher uses. Deliveries run by the delivery handler
// RegisterDeliveryTasks registers.
func RegisterScheduledTasks(registry TaskRegistry, queue TaskQueue, dispatcher func() *Dispatcher) Scheduler {
	if registry == nil || queue == nil || dispatcher == nil {
		panic("notification scheduled tasks require registry, queue, and dispatcher")
	}
	deliveryType := queue.GetTaskTypeWithPrefix(DeliveryTaskType)
	digestType := queue.GetTaskTypeWithPrefix(DigestTaskType)
	if !registry.Register(digestType, func(ctx context.Context, task *asynq.Task) error {
		var payload digestPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil || payload.ChannelID == uuid.Nil || payload.Due.IsZero() {
			return fmt.Errorf("invalid digest task: %w", asynq.SkipRetry)
		}
		retried, _ := asynq.GetRetryCount(ctx)
		return dispatcher().SendDigest(ctx, payload.ChannelID, payload.Due, retried+1 >= MaxDeliveryAttempts)
	}) {
		panic("notification digest task handler is already registered")
	}
	enqueue := func(task *asynq.Task, id string, at time.Time) error {
		_, err := queue.EnqueueTask(task, asynq.Queue("default"), asynq.MaxRetry(MaxDeliveryAttempts-1), asynq.Timeout(time.Minute),
			asynq.TaskID(id), asynq.ProcessAt(at))
		if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) && !errors.Is(err, asynq.ErrDuplicateTask) {
			return fmt.Errorf("schedule notification: %w", err)
		}
		return nil
	}
	return Scheduler{
		DeliverAt: func(_ context.Context, deliveryID uuid.UUID, at time.Time) error {
			return enqueue(asynq.NewTask(deliveryType, []byte(deliveryID.String())), "inbox-delivery-"+deliveryID.String(), at)
		},
		Digest: func(_ context.Context, channelID uuid.UUID, due time.Time) error {
			body, err := json.Marshal(digestPayload{ChannelID: channelID, Due: due})
			if err != nil {
				return err
			}
			return enqueue(asynq.NewTask(digestType, body), fmt.Sprintf("inbox-digest-%s-%d", channelID, due.Unix()), due)
		},
	}
}

type digestPayload struct {
	ChannelID uuid.UUID `json:"channel_id"`
	Due       time.Time `json:"due"`
}

// SendDigest sends the reports held for a channel's digest due at due as one
// message, and marks them sent, or failed after the last attempt.
func (d *Dispatcher) SendDigest(ctx context.Context, channelID uuid.UUID, due time.Time, final bool) error {
	var held []Delivery
	if err := d.db.WithContext(ctx).Where("channel_id = ? AND status = ? AND digest_due_at = ?", channelID, DeliveryHeld, due).
		Order("created_at").Find(&held).Error; err != nil {
		return fmt.Errorf("load held deliveries: %w", err)
	}
	if len(held) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(held))
	notificationIDs := make([]uuid.UUID, 0, len(held))
	for _, delivery := range held {
		ids = append(ids, delivery.ID)
		notificationIDs = append(notificationIDs, delivery.NotificationID)
	}
	var channel Channel
	if err := d.db.WithContext(ctx).First(&channel, "id = ?", channelID).Error; err != nil || channel.Status != ChannelActive {
		return d.finishDigest(ctx, ids, errors.New("channel is no longer active"), true)
	}
	var notifications []Notification
	if err := d.db.WithContext(ctx).Where("id IN ?", notificationIDs).Order("created_at").Find(&notifications).Error; err != nil {
		return fmt.Errorf("load digest notifications: %w", err)
	}
	digest := DigestDaily
	if held[0].Digest != nil {
		digest = *held[0].Digest
	}
	var err error
	switch channel.Type {
	case ChannelEmail:
		err = d.sendDigestEmail(ctx, &channel, digest, notifications, due)
	case ChannelIM:
		err = d.sendDigestIM(ctx, &channel, digest, notifications, due)
	case ChannelWebhook:
		err = d.sendDigestWebhook(ctx, &channel, digest, notifications, due)
	default:
		err = errors.New("unsupported channel type")
	}
	return d.finishDigest(ctx, ids, err, final)
}

func (d *Dispatcher) finishDigest(ctx context.Context, ids []uuid.UUID, sendErr error, final bool) error {
	now := d.now()
	updates := map[string]any{"attempts": gorm.Expr("attempts + 1"), "updated_at": now}
	switch {
	case sendErr == nil:
		updates["status"], updates["sent_at"], updates["last_error"] = DeliverySent, now, nil
	case final:
		message := sendErr.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		updates["status"], updates["last_error"] = DeliveryFailed, message
	}
	if err := d.db.WithContext(ctx).Model(&Delivery{}).Where("id IN ?", ids).Updates(updates).Error; err != nil {
		return err
	}
	if sendErr != nil && !final {
		return sendErr
	}
	return nil
}

func digestTitles(kind string) (string, string) {
	if kind == DigestWeekly {
		return "Weekly task digest", "每周任务汇总"
	}
	return "Daily task digest", "每日任务汇总"
}

func outcomeOf(notification Notification) string {
	var meta map[string]any
	_ = json.Unmarshal(notification.Context, &meta)
	return fmt.Sprint(meta["outcome"])
}

func digestLines(notifications []Notification) []string {
	lines := make([]string, 0, len(notifications))
	for index, notification := range notifications {
		if index == digestMaxReports {
			lines = append(lines, fmt.Sprintf("… +%d", len(notifications)-digestMaxReports))
			break
		}
		line := notification.Title
		if summary := strings.TrimSpace(notification.Summary); summary != "" {
			line += ": " + summary
		}
		lines = append(lines, line)
	}
	return lines
}

func (d *Dispatcher) sendDigestEmail(ctx context.Context, channel *Channel, kind string, notifications []Notification, due time.Time) error {
	if d.channels.email == nil {
		return ErrDeliveryUnavailable
	}
	english, chinese := digestTitles(kind)
	subject := fmt.Sprintf("[ZGI] %s / %s (%d)", chinese, english, len(notifications))
	body := fmt.Sprintf(`<p><strong>%s / %s</strong></p><ul>`, html.EscapeString(chinese), html.EscapeString(english))
	for _, line := range digestLines(notifications) {
		body += "<li>" + html.EscapeString(line) + "</li>"
	}
	body += "</ul>"
	if link := d.absoluteLink("/console/digital-humans"); link != "" {
		body += fmt.Sprintf(`<p><a href="%s">查看报告 / View reports</a></p>`, html.EscapeString(link))
	}
	return d.channels.email(ctx, channel.Target, subject, body, fmt.Sprintf("notification-digest-%s-%d", channel.ID, due.Unix()))
}

func (d *Dispatcher) sendDigestIM(ctx context.Context, channel *Channel, kind string, notifications []Notification, due time.Time) error {
	if d.channels.im == nil || channel.ConnectionID == nil {
		return ErrDeliveryUnavailable
	}
	english, chinese := digestTitles(kind)
	text := fmt.Sprintf("%s / %s (%d)\n- %s", chinese, english, len(notifications), strings.Join(digestLines(notifications), "\n- "))
	if link := d.absoluteLink("/console/digital-humans"); link != "" {
		text += "\n" + link
	}
	return d.channels.im.Send(ctx, channel.OrganizationID, channel.OwnerID, *channel.ConnectionID, channel.Target, text,
		fmt.Sprintf("notification-digest-%s-%d", channel.ID, due.Unix()))
}

func (d *Dispatcher) sendDigestWebhook(ctx context.Context, channel *Channel, kind string, notifications []Notification, due time.Time) error {
	reports := make([]map[string]any, 0, len(notifications))
	for _, notification := range notifications {
		report := map[string]any{"notification_id": notification.ID, "title": notification.Title, "summary": notification.Summary,
			"outcome": outcomeOf(notification), "created_at": notification.CreatedAt}
		if notification.SourceType == "digital_human_task_report" {
			report["report_id"] = notification.SourceID
		}
		reports = append(reports, report)
	}
	digest := &Notification{ID: uuid.NewSHA1(channel.ID, []byte(fmt.Sprint(due.Unix()))), Event: "task.digest", CreatedAt: due}
	return d.postWebhook(ctx, channel, digest, fmt.Sprintf("digest-%s-%d", channel.ID, due.Unix()), map[string]any{"digest": kind, "reports": reports})
}

func (d *Dispatcher) sendIM(ctx context.Context, channel *Channel, notification *Notification, delivery *Delivery) error {
	if d.channels.im == nil || channel.ConnectionID == nil {
		return ErrDeliveryUnavailable
	}
	english, chinese := eventTitle(notification.Event)
	text := fmt.Sprintf("%s / %s: %s", chinese, english, notification.Title)
	if summary := strings.TrimSpace(notification.Summary); summary != "" {
		text += "\n" + summary
	}
	if link := d.absoluteLink(notification.Link); link != "" {
		text += "\n" + link
	}
	return d.channels.im.Send(ctx, channel.OrganizationID, channel.OwnerID, *channel.ConnectionID, channel.Target, text, "notification-delivery-"+delivery.ID.String())
}
