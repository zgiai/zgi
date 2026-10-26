package runtimeworkspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	"github.com/zgiai/zgi/api/internal/infra/webhookhttp"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A Runtime Workspace may send its integrating server webhooks about its end
// users (ADR 0011). Events are written to an outbox in the transaction that
// detects them; a worker delivers them, signed with the endpoint's secret,
// and retries failures with exponential backoff.

// Webhook events.
const (
	WebhookQuotaNear     = "quota.near"
	WebhookQuotaExceeded = "quota.exceeded"
	WebhookCreditsLow    = "credits.low"
	WebhookPlanExpired   = "plan.expired"
	WebhookTest          = "webhook.test"
)

// WebhookEvents are the events an endpoint may subscribe to.
var WebhookEvents = []string{WebhookQuotaNear, WebhookQuotaExceeded, WebhookCreditsLow, WebhookPlanExpired}

// Delivery states.
const (
	WebhookDeliveryPending   = "pending"
	WebhookDeliveryDelivered = "delivered"
	WebhookDeliveryFailed    = "failed"
)

const (
	webhookMaxAttempts      = 8
	webhookFirstRetry       = 30 * time.Second
	webhookMaxRetry         = time.Hour
	webhookLease            = 2 * time.Minute
	webhookDeliveryBatch    = 20
	webhookRetention        = 30 * 24 * time.Hour
	webhookPlanExpiryLookup = 7 * 24 * time.Hour
	webhookAADIntegration   = "runtime_workspace_webhook"
)

var (
	// ErrWebhookNotFound reports a Runtime Workspace without a webhook.
	ErrWebhookNotFound = errors.New("runtime workspace webhook not found")
	// ErrWebhookUnavailable reports that webhook secrets cannot be protected.
	ErrWebhookUnavailable = errors.New("runtime workspace webhook secrets are unavailable")
	// ErrInvalidWebhook reports a malformed webhook endpoint.
	ErrInvalidWebhook = fmt.Errorf("%w: invalid webhook", ErrInvalidWorkspace)
)

// WebhookCipher protects webhook secrets at rest.
type WebhookCipher interface {
	EncryptCredentials(values map[string]string, aad credentialcrypto.CredentialAAD) (string, error)
	DecryptCredentials(envelope string, aad credentialcrypto.CredentialAAD) (map[string]string, error)
}

// WithWebhookCipher lets the service manage webhook secrets.
func WithWebhookCipher(cipher WebhookCipher) ServiceOption {
	return func(s *Service) { s.webhookCipher = cipher }
}

// webhookAllowPrivate lets local development deliver to private addresses.
func webhookAllowPrivate() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ZGI_RUNTIME_WORKSPACE_WEBHOOK_ALLOW_PRIVATE")), "true")
}

// Webhook is a Runtime Workspace's endpoint.
type Webhook struct {
	RuntimeWorkspaceID uuid.UUID      `gorm:"type:uuid;primaryKey"`
	OrganizationID     uuid.UUID      `gorm:"type:uuid;not null"`
	URL                string         `gorm:"column:url;type:varchar(2048);not null"`
	SecretEnvelope     string         `gorm:"type:text;not null"`
	Events             datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	CreditsLowPoints   int64          `gorm:"not null;default:0"`
	Enabled            bool           `gorm:"not null;default:true"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Webhook) TableName() string { return "runtime_workspace_webhooks" }

func (w Webhook) subscribes(event string) bool {
	var events []string
	_ = json.Unmarshal(w.Events, &events)
	return w.Enabled && slices.Contains(events, event)
}

// WebhookDelivery is one event waiting for, or done with, delivery.
type WebhookDelivery struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey"`
	RuntimeWorkspaceID uuid.UUID      `gorm:"type:uuid;not null"`
	SubjectID          *uuid.UUID     `gorm:"type:uuid"`
	Event              string         `gorm:"type:varchar(32);not null"`
	Payload            datatypes.JSON `gorm:"type:jsonb;not null"`
	Status             string         `gorm:"type:varchar(16);not null;default:'pending'"`
	Attempts           int            `gorm:"not null;default:0"`
	NextAttemptAt      time.Time      `gorm:"not null"`
	LastStatus         int            `gorm:"not null;default:0"`
	LastError          string         `gorm:"type:varchar(500);not null;default:''"`
	CreatedAt          time.Time
	DeliveredAt        *time.Time
}

func (WebhookDelivery) TableName() string { return "runtime_workspace_webhook_deliveries" }

// WebhookView is a webhook as the console shows it; the secret is never
// shown again after it is created or rotated.
type WebhookView struct {
	Configured          bool       `json:"configured"`
	URL                 string     `json:"url,omitempty"`
	Events              []string   `json:"events"`
	CreditsLowThreshold float64    `json:"credits_low_threshold"`
	Enabled             bool       `json:"enabled"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

// WebhookInput sets a webhook.
type WebhookInput struct {
	URL                 string          `json:"url"`
	Events              []string        `json:"events"`
	CreditsLowThreshold decimal.Decimal `json:"credits_low_threshold"`
	Enabled             *bool           `json:"enabled"`
}

// WebhookSaved is a saved webhook, with its secret when one was created.
type WebhookSaved struct {
	Webhook WebhookView `json:"webhook"`
	Secret  string      `json:"secret,omitempty"`
}

// WebhookDeliveryView is a delivery as the console shows it.
type WebhookDeliveryView struct {
	ID          string     `json:"id"`
	Event       string     `json:"event"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	LastStatus  int        `json:"last_status,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	NextAttempt *time.Time `json:"next_attempt_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}

func webhookView(webhook *Webhook) *WebhookView {
	if webhook == nil {
		return &WebhookView{Events: []string{}}
	}
	var events []string
	_ = json.Unmarshal(webhook.Events, &events)
	if events == nil {
		events = []string{}
	}
	updated := webhook.UpdatedAt
	return &WebhookView{Configured: true, URL: webhook.URL, Events: events, Enabled: webhook.Enabled,
		CreditsLowThreshold: pointsOf(millipoints.FromInt64(webhook.CreditsLowPoints)), UpdatedAt: &updated}
}

func webhookAAD(runtimeWorkspace *RuntimeWorkspace) credentialcrypto.CredentialAAD {
	return credentialcrypto.CredentialAAD{OrganizationID: runtimeWorkspace.OrganizationID, ConnectionID: runtimeWorkspace.LogicalID(),
		IntegrationID: webhookAADIntegration, CredentialVersion: 1}
}

func newWebhookSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return "whsec_" + hex.EncodeToString(raw), nil
}

func validWebhookURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || len(raw) > 2048 {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http" && webhookAllowPrivate()
}

func (s *Service) loadWebhook(ctx context.Context, runtimeWorkspace *RuntimeWorkspace) (*Webhook, error) {
	var webhooks []Webhook
	if err := s.db.WithContext(ctx).Where("runtime_workspace_id = ?", runtimeWorkspace.LogicalID()).Limit(1).Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("load runtime workspace webhook: %w", err)
	}
	if len(webhooks) == 0 {
		return nil, nil
	}
	return &webhooks[0], nil
}

// GetWebhook returns a Runtime Workspace's webhook, or an unconfigured view.
func (s *Service) GetWebhook(ctx context.Context, workspaceID uuid.UUID) (*WebhookView, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, s.endUserLoadError(err)
	}
	webhook, err := s.loadWebhook(ctx, runtimeWorkspace)
	if err != nil {
		return nil, err
	}
	return webhookView(webhook), nil
}

// SaveWebhook creates or changes a Runtime Workspace's webhook. Creating one
// returns its secret, once.
func (s *Service) SaveWebhook(ctx context.Context, workspaceID uuid.UUID, input WebhookInput) (*WebhookSaved, error) {
	events := []string{}
	for _, event := range input.Events {
		if !slices.Contains(WebhookEvents, event) {
			return nil, ErrInvalidWebhook
		}
		if !slices.Contains(events, event) {
			events = append(events, event)
		}
	}
	threshold := millipoints.FromDecimalPoints(input.CreditsLowThreshold).Int64()
	if !validWebhookURL(input.URL) || len(events) == 0 || threshold < 0 {
		return nil, ErrInvalidWebhook
	}
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, s.endUserLoadError(err)
	}
	webhook, err := s.loadWebhook(ctx, runtimeWorkspace)
	if err != nil {
		return nil, err
	}
	rawEvents, _ := json.Marshal(events)
	now := s.now()
	saved := &WebhookSaved{}
	if webhook == nil {
		if s.webhookCipher == nil {
			return nil, ErrWebhookUnavailable
		}
		secret, err := newWebhookSecret()
		if err != nil {
			return nil, err
		}
		envelope, err := s.webhookCipher.EncryptCredentials(map[string]string{"secret": secret}, webhookAAD(runtimeWorkspace))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrWebhookUnavailable, err)
		}
		webhook = &Webhook{RuntimeWorkspaceID: runtimeWorkspace.LogicalID(), OrganizationID: runtimeWorkspace.OrganizationID,
			URL: strings.TrimSpace(input.URL), SecretEnvelope: envelope, Events: rawEvents, CreditsLowPoints: threshold,
			Enabled: input.Enabled == nil || *input.Enabled, CreatedAt: now, UpdatedAt: now}
		if err := s.db.WithContext(ctx).Create(webhook).Error; err != nil {
			return nil, fmt.Errorf("create runtime workspace webhook: %w", err)
		}
		saved.Secret = secret
	} else {
		updates := map[string]any{"url": strings.TrimSpace(input.URL), "events": datatypes.JSON(rawEvents), "credits_low_points": threshold, "updated_at": now}
		if input.Enabled != nil {
			updates["enabled"] = *input.Enabled
		}
		if err := s.db.WithContext(ctx).Model(&Webhook{}).Where("runtime_workspace_id = ?", webhook.RuntimeWorkspaceID).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update runtime workspace webhook: %w", err)
		}
		if webhook, err = s.loadWebhook(ctx, runtimeWorkspace); err != nil {
			return nil, err
		}
	}
	saved.Webhook = *webhookView(webhook)
	return saved, nil
}

// RotateWebhookSecret replaces a webhook's secret and returns it, once.
func (s *Service) RotateWebhookSecret(ctx context.Context, workspaceID uuid.UUID) (string, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return "", s.endUserLoadError(err)
	}
	if s.webhookCipher == nil {
		return "", ErrWebhookUnavailable
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return "", err
	}
	envelope, err := s.webhookCipher.EncryptCredentials(map[string]string{"secret": secret}, webhookAAD(runtimeWorkspace))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrWebhookUnavailable, err)
	}
	result := s.db.WithContext(ctx).Model(&Webhook{}).Where("runtime_workspace_id = ?", runtimeWorkspace.LogicalID()).
		Updates(map[string]any{"secret_envelope": envelope, "updated_at": s.now()})
	if result.Error != nil {
		return "", fmt.Errorf("rotate runtime workspace webhook secret: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return "", ErrWebhookNotFound
	}
	return secret, nil
}

// DeleteWebhook removes a Runtime Workspace's webhook and its pending
// deliveries.
func (s *Service) DeleteWebhook(ctx context.Context, workspaceID uuid.UUID) error {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return s.endUserLoadError(err)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM runtime_workspace_webhooks WHERE runtime_workspace_id = ?", runtimeWorkspace.LogicalID()).Error; err != nil {
			return fmt.Errorf("delete runtime workspace webhook: %w", err)
		}
		if err := tx.Exec("DELETE FROM runtime_workspace_webhook_deliveries WHERE runtime_workspace_id = ? AND status = ?", runtimeWorkspace.LogicalID(), WebhookDeliveryPending).Error; err != nil {
			return fmt.Errorf("delete pending webhook deliveries: %w", err)
		}
		return nil
	})
}

// TestWebhook queues a webhook.test event for the endpoint.
func (s *Service) TestWebhook(ctx context.Context, workspaceID uuid.UUID) (*WebhookDeliveryView, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, s.endUserLoadError(err)
	}
	webhook, err := s.loadWebhook(ctx, runtimeWorkspace)
	if err != nil {
		return nil, err
	}
	if webhook == nil {
		return nil, ErrWebhookNotFound
	}
	delivery, err := queueWebhookDelivery(ctx, s.db, runtimeWorkspace.LogicalID(), nil, WebhookTest, map[string]any{"message": "This is a test event."}, s.now())
	if err != nil {
		return nil, err
	}
	view := webhookDeliveryView(*delivery)
	return &view, nil
}

func webhookDeliveryView(delivery WebhookDelivery) WebhookDeliveryView {
	view := WebhookDeliveryView{ID: delivery.ID.String(), Event: delivery.Event, Status: delivery.Status, Attempts: delivery.Attempts,
		LastStatus: delivery.LastStatus, LastError: delivery.LastError, CreatedAt: delivery.CreatedAt, DeliveredAt: delivery.DeliveredAt}
	if delivery.Status == WebhookDeliveryPending {
		next := delivery.NextAttemptAt
		view.NextAttempt = &next
	}
	return view
}

// WebhookDeliveryPage is a page of deliveries, newest first.
type WebhookDeliveryPage struct {
	Items   []WebhookDeliveryView `json:"items"`
	Page    int                   `json:"page"`
	Limit   int                   `json:"limit"`
	HasMore bool                  `json:"has_more"`
}

// ListWebhookDeliveries lists a Runtime Workspace's deliveries, a page at a
// time.
func (s *Service) ListWebhookDeliveries(ctx context.Context, workspaceID uuid.UUID, page, limit int) (*WebhookDeliveryPage, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, s.endUserLoadError(err)
	}
	page = max(page, 1)
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var deliveries []WebhookDelivery
	if err := s.db.WithContext(ctx).Where("runtime_workspace_id = ?", runtimeWorkspace.LogicalID()).
		Order("created_at DESC").Order("id").Limit(limit + 1).Offset((page - 1) * limit).Find(&deliveries).Error; err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	result := &WebhookDeliveryPage{Page: page, Limit: limit, HasMore: len(deliveries) > limit, Items: make([]WebhookDeliveryView, 0, len(deliveries))}
	if result.HasMore {
		deliveries = deliveries[:limit]
	}
	for _, delivery := range deliveries {
		result.Items = append(result.Items, webhookDeliveryView(delivery))
	}
	return result, nil
}

// RetryWebhookDelivery sends a failed delivery again.
func (s *Service) RetryWebhookDelivery(ctx context.Context, workspaceID, deliveryID uuid.UUID) error {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return s.endUserLoadError(err)
	}
	result := s.db.WithContext(ctx).Model(&WebhookDelivery{}).
		Where("id = ? AND runtime_workspace_id = ? AND status = ?", deliveryID, runtimeWorkspace.LogicalID(), WebhookDeliveryFailed).
		Updates(map[string]any{"status": WebhookDeliveryPending, "attempts": 0, "next_attempt_at": s.now(), "last_error": ""})
	if result.Error != nil {
		return fmt.Errorf("retry webhook delivery: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

// subscribedWebhook returns the endpoint that wants an event, or nil.
func subscribedWebhook(ctx context.Context, db *gorm.DB, runtimeWorkspaceID uuid.UUID, event string) (*Webhook, error) {
	var webhooks []Webhook
	if err := db.WithContext(ctx).Where("runtime_workspace_id = ? AND enabled", runtimeWorkspaceID).Limit(1).Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("load runtime workspace webhook: %w", err)
	}
	if len(webhooks) == 0 || !webhooks[0].subscribes(event) {
		return nil, nil
	}
	return &webhooks[0], nil
}

// queueWebhookDelivery writes an event to the outbox.
func queueWebhookDelivery(ctx context.Context, db *gorm.DB, runtimeWorkspaceID uuid.UUID, subjectID *uuid.UUID, event string, data map[string]any, now time.Time) (*WebhookDelivery, error) {
	id := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"id": id.String(), "event": event, "created_at": now.UTC().Format(time.RFC3339),
		"runtime_workspace_id": runtimeWorkspaceID.String(), "data": data,
	})
	if err != nil {
		return nil, fmt.Errorf("encode webhook event: %w", err)
	}
	delivery := WebhookDelivery{ID: id, RuntimeWorkspaceID: runtimeWorkspaceID, SubjectID: subjectID, Event: event,
		Payload: datatypes.JSON(payload), Status: WebhookDeliveryPending, NextAttemptAt: now, CreatedAt: now}
	if err := db.WithContext(ctx).Create(&delivery).Error; err != nil {
		return nil, fmt.Errorf("queue webhook event: %w", err)
	}
	return &delivery, nil
}

// notifyEndUser queues an end-user event once per scope and cooldown: the
// first time the state is seen, and again only after the cooldown.
func notifyEndUser(ctx context.Context, db *gorm.DB, user *EndUser, event, scope string, cooldown time.Duration, data map[string]any, now time.Time) error {
	if user == nil {
		return nil
	}
	webhook, err := subscribedWebhook(ctx, db, user.RuntimeWorkspaceID, event)
	if err != nil || webhook == nil {
		return err
	}
	claimed := db.WithContext(ctx).Exec(`
		INSERT INTO runtime_workspace_end_user_notices (subject_id, kind, scope, runtime_workspace_id, notified_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (subject_id, kind, scope) DO UPDATE SET notified_at = EXCLUDED.notified_at
		WHERE runtime_workspace_end_user_notices.notified_at <= ?`,
		user.SubjectID, event, scope, user.RuntimeWorkspaceID, now, now.Add(-cooldown))
	if claimed.Error != nil {
		return fmt.Errorf("record end-user notice: %w", claimed.Error)
	}
	if claimed.RowsAffected == 0 {
		return nil
	}
	subjectID := user.SubjectID
	_, err = queueWebhookDelivery(ctx, db, user.RuntimeWorkspaceID, &subjectID, event, endUserEventData(user, data), now)
	return err
}

func endUserEventData(user *EndUser, data map[string]any) map[string]any {
	out := map[string]any{"user": user.ExternalUserID, "subject_id": user.SubjectID.String()}
	for key, value := range data {
		out[key] = value
	}
	return out
}

// notifyCreditsLow queues credits.low when a balance falls to or below the
// endpoint's threshold.
func notifyCreditsLow(ctx context.Context, db *gorm.DB, user *EndUser, before, after int64, now time.Time) error {
	webhook, err := subscribedWebhook(ctx, db, user.RuntimeWorkspaceID, WebhookCreditsLow)
	if err != nil || webhook == nil || before <= webhook.CreditsLowPoints || after > webhook.CreditsLowPoints {
		return err
	}
	subjectID := user.SubjectID
	_, err = queueWebhookDelivery(ctx, db, user.RuntimeWorkspaceID, &subjectID, WebhookCreditsLow, endUserEventData(user, map[string]any{
		"balance":   json.Number(millipoints.FromInt64(after).DecimalPoints().String()),
		"threshold": json.Number(millipoints.FromInt64(webhook.CreditsLowPoints).DecimalPoints().String()),
	}), now)
	return err
}

// WebhookWorker reports expired plans and delivers queued events. Deliveries
// are leased with SKIP LOCKED, so every API instance may run it.
type WebhookWorker struct {
	db        *gorm.DB
	cipher    WebhookCipher
	client    *http.Client
	now       func() time.Time
	lastPurge time.Time
}

// NewWebhookWorker builds the worker.
func NewWebhookWorker(db *gorm.DB, cipher WebhookCipher) *WebhookWorker {
	return &WebhookWorker{db: db, cipher: cipher, client: webhookhttp.Client(webhookAllowPrivate()), now: func() time.Time { return time.Now().UTC() }}
}

// RunOnce reports newly expired plans, delivers what is due, and purges old
// deliveries and notices.
func (w *WebhookWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.db == nil {
		return nil
	}
	if err := w.reportExpiredPlans(ctx); err != nil {
		return err
	}
	for {
		delivered, err := w.deliverDue(ctx)
		if err != nil || delivered < webhookDeliveryBatch {
			if err == nil {
				err = w.purge(ctx)
			}
			return err
		}
	}
}

func (w *WebhookWorker) reportExpiredPlans(ctx context.Context) error {
	now := w.now()
	var expired []EndUser
	if err := w.db.WithContext(ctx).Raw(`
		SELECT e.* FROM runtime_workspace_end_users e
		JOIN runtime_workspace_webhooks w ON w.runtime_workspace_id = e.runtime_workspace_id AND w.enabled AND w.events @> ?::jsonb
		WHERE e.plan_key <> '' AND e.status <> 'erased' AND e.plan_expires_at <= ? AND e.plan_expires_at > ?
			AND NOT EXISTS (SELECT 1 FROM runtime_workspace_end_user_notices n
				WHERE n.subject_id = e.subject_id AND n.kind = ? AND n.scope = e.plan_key || ':' || floor(extract(epoch FROM e.plan_expires_at))::bigint)
		LIMIT 100`, `["`+WebhookPlanExpired+`"]`, now, now.Add(-webhookPlanExpiryLookup), WebhookPlanExpired).Scan(&expired).Error; err != nil {
		return fmt.Errorf("find expired end-user plans: %w", err)
	}
	for index := range expired {
		user := &expired[index]
		scope := user.PlanKey + ":" + strconv.FormatInt(user.PlanExpiresAt.Unix(), 10)
		data := map[string]any{"plan": user.PlanKey, "expired_at": user.PlanExpiresAt.UTC().Format(time.RFC3339)}
		if err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return notifyEndUser(ctx, tx, user, WebhookPlanExpired, scope, webhookPlanExpiryLookup*2, data, now)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *WebhookWorker) deliverDue(ctx context.Context) (int, error) {
	now := w.now()
	var due []WebhookDelivery
	err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_attempt_at <= ?", WebhookDeliveryPending, now).
			Order("next_attempt_at").Limit(webhookDeliveryBatch).Find(&due).Error; err != nil {
			return fmt.Errorf("claim webhook deliveries: %w", err)
		}
		if len(due) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, 0, len(due))
		for _, delivery := range due {
			ids = append(ids, delivery.ID)
		}
		return tx.Model(&WebhookDelivery{}).Where("id IN ?", ids).Update("next_attempt_at", now.Add(webhookLease)).Error
	})
	if err != nil {
		return 0, err
	}
	webhooks := map[uuid.UUID]*Webhook{}
	for _, delivery := range due {
		webhook, ok := webhooks[delivery.RuntimeWorkspaceID]
		if !ok {
			var rows []Webhook
			if err := w.db.WithContext(ctx).Where("runtime_workspace_id = ?", delivery.RuntimeWorkspaceID).Limit(1).Find(&rows).Error; err != nil {
				return 0, fmt.Errorf("load runtime workspace webhook: %w", err)
			}
			if len(rows) == 1 {
				webhook = &rows[0]
			}
			webhooks[delivery.RuntimeWorkspaceID] = webhook
		}
		status, sendErr := w.send(ctx, webhook, delivery)
		if err := w.finish(ctx, delivery, status, sendErr); err != nil {
			return 0, err
		}
	}
	return len(due), nil
}

func (w *WebhookWorker) send(ctx context.Context, webhook *Webhook, delivery WebhookDelivery) (int, error) {
	if webhook == nil || !webhook.Enabled {
		return 0, errors.New("webhook is disabled or removed")
	}
	if w.cipher == nil {
		return 0, ErrWebhookUnavailable
	}
	var runtimeWorkspace RuntimeWorkspace
	if err := w.db.WithContext(ctx).Where("runtime_id = ?", webhook.RuntimeWorkspaceID).First(&runtimeWorkspace).Error; err != nil {
		return 0, fmt.Errorf("load runtime workspace: %w", err)
	}
	values, err := w.cipher.DecryptCredentials(webhook.SecretEnvelope, webhookAAD(&runtimeWorkspace))
	if err != nil {
		return 0, errors.New("webhook secret cannot be read")
	}
	secret := values["secret"]
	credentialcrypto.DestroyMap(values)
	timestamp := strconv.FormatInt(w.now().Unix(), 10)
	body := []byte(delivery.Payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "ZGI-Webhooks/1")
	request.Header.Set("X-ZGI-Event", delivery.Event)
	request.Header.Set("X-ZGI-Delivery", delivery.ID.String())
	request.Header.Set("X-ZGI-Timestamp", timestamp)
	request.Header.Set("X-ZGI-Signature", "sha256="+webhookhttp.Sign(secret, timestamp, body))
	response, err := w.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("webhook request failed: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("webhook responded with status %d", response.StatusCode)
	}
	return response.StatusCode, nil
}

// webhookRetryDelay is the wait after a failed attempt: 30s doubling, up to
// an hour.
func webhookRetryDelay(attempts int) time.Duration {
	delay := webhookFirstRetry
	for i := 1; i < attempts && delay < webhookMaxRetry; i++ {
		delay *= 2
	}
	return min(delay, webhookMaxRetry)
}

func (w *WebhookWorker) finish(ctx context.Context, delivery WebhookDelivery, status int, sendErr error) error {
	now := w.now()
	attempts := delivery.Attempts + 1
	updates := map[string]any{"attempts": attempts, "last_status": status}
	if sendErr == nil {
		updates["status"], updates["delivered_at"], updates["last_error"] = WebhookDeliveryDelivered, now, ""
	} else {
		message := sendErr.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		updates["last_error"] = message
		if attempts >= webhookMaxAttempts {
			updates["status"] = WebhookDeliveryFailed
		} else {
			updates["next_attempt_at"] = now.Add(webhookRetryDelay(attempts))
		}
	}
	if err := w.db.WithContext(ctx).Model(&WebhookDelivery{}).Where("id = ?", delivery.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("record webhook delivery: %w", err)
	}
	return nil
}

func (w *WebhookWorker) purge(ctx context.Context) error {
	now := w.now()
	if now.Sub(w.lastPurge) < time.Hour {
		return nil
	}
	w.lastPurge = now
	if err := w.db.WithContext(ctx).Exec("DELETE FROM runtime_workspace_webhook_deliveries WHERE status <> ? AND created_at < ?", WebhookDeliveryPending, now.Add(-webhookRetention)).Error; err != nil {
		return fmt.Errorf("purge webhook deliveries: %w", err)
	}
	if err := w.db.WithContext(ctx).Exec("DELETE FROM runtime_workspace_end_user_notices WHERE notified_at < ?", now.Add(-webhookRetention)).Error; err != nil {
		return fmt.Errorf("purge end-user notices: %w", err)
	}
	return nil
}
