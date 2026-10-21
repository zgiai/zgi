package inbox

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DeliveryPending = "pending"
	DeliverySent    = "sent"
	DeliveryFailed  = "failed"

	// MaxDeliveryAttempts bounds retries of one delivery.
	MaxDeliveryAttempts = 5
	webhookTimeout      = 10 * time.Second
)

// Delivery tracks one notification sent to one channel.
type Delivery struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	NotificationID uuid.UUID  `gorm:"type:uuid;not null" json:"notification_id"`
	ChannelID      uuid.UUID  `gorm:"type:uuid;not null" json:"channel_id"`
	Status         string     `gorm:"type:varchar(16);not null" json:"status"`
	Attempts       int        `gorm:"not null" json:"attempts"`
	LastError      *string    `gorm:"type:text" json:"last_error,omitempty"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName binds deliveries to their table.
func (Delivery) TableName() string { return "notification_deliveries" }

// DeliveryEnqueuer schedules one delivery attempt in the background.
type DeliveryEnqueuer func(ctx context.Context, deliveryID uuid.UUID) error

// Dispatcher fans inbox notifications out to the recipients' channels.
type Dispatcher struct {
	db         *gorm.DB
	channels   *Channels
	enqueue    DeliveryEnqueuer
	consoleURL string
	client     *http.Client
	now        func() time.Time
}

// NewDispatcher builds a dispatcher. consoleURL turns notification links into
// absolute URLs for email and webhooks. allowPrivateWebhooks is for local
// development only: it lets webhooks reach private addresses.
func NewDispatcher(db *gorm.DB, channels *Channels, enqueue DeliveryEnqueuer, consoleURL string, allowPrivateWebhooks bool) *Dispatcher {
	if db == nil || channels == nil {
		panic("notification dispatcher requires db and channels")
	}
	return &Dispatcher{db: db, channels: channels, enqueue: enqueue, consoleURL: strings.TrimRight(strings.TrimSpace(consoleURL), "/"),
		client: webhookClient(allowPrivateWebhooks), now: func() time.Time { return time.Now().UTC() }}
}

// eventKey maps an inbox event to the key channels subscribe to.
func eventKey(event string) string {
	switch event {
	case "digital_human.task_report":
		return EventTaskReport
	case "digital_human.task_failed":
		return EventTaskFailed
	default:
		return ""
	}
}

// Plan creates a delivery for every active channel that should receive the
// notification and schedules them. Personal channels serve account
// recipients; an API end user's notification goes to the webhook of the
// DigitalHuman whose task it is. Extra channels named by the notification's
// context, such as a team channel chosen for a task, are added when they
// belong to the same organization and subscribe to the event.
func (d *Dispatcher) Plan(ctx context.Context, notification *Notification) error {
	key := eventKey(notification.Event)
	if key == "" {
		return nil
	}
	var candidates []*Channel
	query := d.db.WithContext(ctx).Where("organization_id = ? AND status = ?", notification.OrganizationID, ChannelActive)
	var meta map[string]any
	_ = json.Unmarshal(notification.Context, &meta)
	switch notification.RecipientKind {
	case RecipientAccount:
		if err := query.Where("owner_kind = ? AND owner_id = ?", OwnerAccount, notification.RecipientID).Find(&candidates).Error; err != nil {
			return fmt.Errorf("find notification channels: %w", err)
		}
	case RecipientAPIEndUser:
		digitalHumanID, err := uuid.Parse(fmt.Sprint(meta["digital_human_id"]))
		if err != nil {
			return nil
		}
		if err := query.Where("owner_kind = ? AND owner_id = ?", OwnerDigitalHuman, digitalHumanID).Find(&candidates).Error; err != nil {
			return fmt.Errorf("find notification channels: %w", err)
		}
	}
	if raw, ok := meta["channel_ids"].([]any); ok && len(raw) > 0 {
		ids := make([]uuid.UUID, 0, len(raw))
		for _, value := range raw {
			if id, err := uuid.Parse(fmt.Sprint(value)); err == nil {
				ids = append(ids, id)
			}
		}
		var extra []*Channel
		if err := d.db.WithContext(ctx).Where("organization_id = ? AND status = ? AND id IN ?", notification.OrganizationID, ChannelActive, ids).Find(&extra).Error; err != nil {
			return fmt.Errorf("find task notification channels: %w", err)
		}
		candidates = append(candidates, extra...)
	}
	seen := map[uuid.UUID]bool{}
	now := d.now()
	for _, channel := range candidates {
		if seen[channel.ID] || !slices.Contains(channel.Events, key) {
			continue
		}
		seen[channel.ID] = true
		delivery := &Delivery{ID: uuid.New(), NotificationID: notification.ID, ChannelID: channel.ID, Status: DeliveryPending, CreatedAt: now, UpdatedAt: now}
		result := d.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(delivery)
		if result.Error != nil {
			return fmt.Errorf("create notification delivery: %w", result.Error)
		}
		if result.RowsAffected == 1 && d.enqueue != nil {
			_ = d.enqueue(ctx, delivery.ID)
		}
	}
	return nil
}

// Deliver attempts one delivery. It returns an error while a retry may still
// succeed; the delivery is marked failed after the last attempt.
func (d *Dispatcher) Deliver(ctx context.Context, deliveryID uuid.UUID) error {
	var delivery Delivery
	if err := d.db.WithContext(ctx).First(&delivery, "id = ?", deliveryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if delivery.Status != DeliveryPending {
		return nil
	}
	var notification Notification
	var channel Channel
	if err := d.db.WithContext(ctx).First(&notification, "id = ?", delivery.NotificationID).Error; err != nil {
		return d.finish(ctx, &delivery, fmt.Errorf("notification missing: %w", err), true)
	}
	if err := d.db.WithContext(ctx).First(&channel, "id = ?", delivery.ChannelID).Error; err != nil || channel.Status != ChannelActive {
		return d.finish(ctx, &delivery, errors.New("channel is no longer active"), true)
	}
	var err error
	switch channel.Type {
	case ChannelEmail:
		err = d.sendEmail(ctx, &channel, &notification, &delivery)
	case ChannelWebhook:
		err = d.sendWebhook(ctx, &channel, &notification, &delivery)
	default:
		err = errors.New("unsupported channel type")
	}
	return d.finish(ctx, &delivery, err, false)
}

// SendTest delivers a sample notification to a channel right away.
func (d *Dispatcher) SendTest(ctx context.Context, owner ChannelOwner, channelID uuid.UUID) error {
	channel, err := d.channels.load(ctx, owner, channelID)
	if err != nil {
		return err
	}
	if channel.Status != ChannelActive {
		return ErrChannelConflict
	}
	sample := &Notification{ID: uuid.New(), OrganizationID: channel.OrganizationID, Event: "notification.test", SourceType: "notification_channel", SourceID: channel.ID,
		Title: "ZGI test notification / 测试通知", Summary: "This channel receives ZGI notifications. / 这个渠道可以收到 ZGI 的通知。", Link: "/console", CreatedAt: d.now()}
	delivery := &Delivery{ID: uuid.New()}
	if channel.Type == ChannelEmail {
		return d.sendEmail(ctx, channel, sample, delivery)
	}
	return d.sendWebhook(ctx, channel, sample, delivery)
}

func (d *Dispatcher) finish(ctx context.Context, delivery *Delivery, sendErr error, final bool) error {
	now := d.now()
	updates := map[string]any{"attempts": gorm.Expr("attempts + 1"), "updated_at": now}
	if sendErr == nil {
		updates["status"], updates["sent_at"], updates["last_error"] = DeliverySent, now, nil
	} else {
		message := sendErr.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		updates["last_error"] = message
		if final || delivery.Attempts+1 >= MaxDeliveryAttempts {
			updates["status"] = DeliveryFailed
			final = true
		}
	}
	if err := d.db.WithContext(ctx).Model(&Delivery{}).Where("id = ?", delivery.ID).Updates(updates).Error; err != nil {
		return err
	}
	if sendErr != nil && !final {
		return sendErr
	}
	return nil
}

func (d *Dispatcher) absoluteLink(link string) string {
	if strings.HasPrefix(link, "/") && d.consoleURL != "" {
		return d.consoleURL + link
	}
	return link
}

func eventTitle(event string) (string, string) {
	switch event {
	case "digital_human.task_failed":
		return "Task failed", "任务执行失败"
	case "notification.test":
		return "Test notification", "测试通知"
	default:
		return "Task report", "任务汇报"
	}
}

func (d *Dispatcher) sendEmail(ctx context.Context, channel *Channel, notification *Notification, delivery *Delivery) error {
	if d.channels.email == nil {
		return ErrDeliveryUnavailable
	}
	english, chinese := eventTitle(notification.Event)
	subject := fmt.Sprintf("[ZGI] %s / %s: %s", chinese, english, notification.Title)
	link := d.absoluteLink(notification.Link)
	body := fmt.Sprintf(`<p><strong>%s / %s</strong></p><p>%s</p><p>%s</p>`,
		html.EscapeString(chinese), html.EscapeString(english), html.EscapeString(notification.Title), html.EscapeString(notification.Summary))
	if link != "" {
		body += fmt.Sprintf(`<p><a href="%s">查看详情 / View details</a></p>`, html.EscapeString(link))
	}
	return d.channels.email(ctx, channel.Target, subject, body, "notification-delivery-"+delivery.ID.String())
}

// WebhookPayload is the body of a notification webhook. Only identifiers and
// the outcome line are sent; full content is read through the API.
type WebhookPayload struct {
	ID        string         `json:"id"`
	Event     string         `json:"event"`
	CreatedAt time.Time      `json:"created_at"`
	Data      map[string]any `json:"data"`
}

func webhookEvent(event string) string {
	switch event {
	case "digital_human.task_report":
		return "report.created"
	case "digital_human.task_failed":
		return "task.failed"
	default:
		return event
	}
}

func (d *Dispatcher) sendWebhook(ctx context.Context, channel *Channel, notification *Notification, delivery *Delivery) error {
	secret, err := d.channels.webhookSecret(channel)
	if err != nil {
		return err
	}
	data := map[string]any{"notification_id": notification.ID, "title": notification.Title, "summary": notification.Summary}
	var meta map[string]any
	if json.Unmarshal(notification.Context, &meta) == nil {
		for _, key := range []string{"digital_human_id", "task_id", "outcome"} {
			if value, ok := meta[key]; ok {
				data[key] = value
			}
		}
	}
	if notification.SourceType == "digital_human_task_report" {
		data["report_id"] = notification.SourceID
	}
	if link := d.absoluteLink(notification.Link); link != "" && notification.RecipientKind == RecipientAccount {
		data["link"] = link
	}
	body, err := json.Marshal(WebhookPayload{ID: delivery.ID.String(), Event: webhookEvent(notification.Event), CreatedAt: notification.CreatedAt, Data: data})
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(d.now().Unix(), 10)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.Target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "ZGI-Notifications/1")
	request.Header.Set("X-ZGI-Event", webhookEvent(notification.Event))
	request.Header.Set("X-ZGI-Delivery", delivery.ID.String())
	request.Header.Set("X-ZGI-Timestamp", timestamp)
	request.Header.Set("X-ZGI-Signature", "sha256="+SignWebhook(secret, timestamp, body))
	response, err := d.client.Do(request)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with status %d", response.StatusCode)
	}
	return nil
}

// SignWebhook signs a webhook body: HMAC-SHA256 over "<timestamp>.<body>".
// Receivers recompute it with their secret and compare it to
// X-ZGI-Signature, and reject old timestamps.
func SignWebhook(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

var blockedWebhookPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func publicWebhookAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedWebhookPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// webhookClient dials only public addresses resolved at connect time, so a
// webhook cannot reach internal services, and follows no redirects.
func webhookClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Client{
		Timeout:       webhookTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
				if err != nil {
					return nil, err
				}
				var lastErr error = errors.New("webhook host has no public address")
				for _, addr := range addrs {
					if !allowPrivate && !publicWebhookAddr(addr) {
						continue
					}
					conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.Unmap().String(), port))
					if err == nil {
						return conn, nil
					}
					lastErr = err
				}
				return nil, lastErr
			},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: webhookTimeout,
			DisableKeepAlives:     true,
		},
	}
}
