package inbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	"gorm.io/gorm"
)

// Channel owner kinds and types.
const (
	OwnerAccount      = "account"
	OwnerWorkspace    = "workspace"
	OwnerDigitalHuman = "digital_human"

	ChannelEmail   = "email"
	ChannelWebhook = "webhook"

	ChannelPending  = "pending"
	ChannelActive   = "active"
	ChannelDisabled = "disabled"

	// Event keys a channel subscribes to. They name inbox events without
	// their module prefix.
	EventTaskReport = "task_report"
	EventTaskFailed = "task_failed"

	verifyCodeTTL      = 15 * time.Minute
	verifyCodeCooldown = time.Minute
	verifyMaxAttempts  = 5
	maxChannelsPerUser = 20
)

var (
	// ErrChannelNotFound hides channels of other owners.
	ErrChannelNotFound = errors.New("notification channel not found")
	// ErrChannelConflict marks a duplicate target or a channel in the wrong state.
	ErrChannelConflict = errors.New("notification channel conflict")
	// ErrVerificationFailed marks a wrong, expired, or exhausted code.
	ErrVerificationFailed = errors.New("notification channel verification failed")
	// ErrRateLimited asks the caller to wait before sending another code.
	ErrRateLimited = errors.New("notification channel rate limited")
	// ErrDeliveryUnavailable marks a deployment that cannot send this channel type.
	ErrDeliveryUnavailable = errors.New("notification channel delivery unavailable")
)

// Channel is one place notifications are delivered outside the console.
type Channel struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID    *uuid.UUID `gorm:"type:uuid" json:"workspace_id,omitempty"`
	OwnerKind      string     `gorm:"type:varchar(32);not null" json:"owner_kind"`
	OwnerID        uuid.UUID  `gorm:"type:uuid;not null" json:"owner_id"`
	Type           string     `gorm:"type:varchar(16);not null" json:"type"`
	Name           string     `gorm:"type:varchar(128);not null" json:"name"`
	Target         string     `gorm:"type:text;not null" json:"target"`
	SecretEnvelope *string    `gorm:"type:text" json:"-"`
	Events         []string   `gorm:"type:jsonb;serializer:json;not null" json:"events"`
	Status         string     `gorm:"type:varchar(16);not null" json:"status"`
	VerifyCodeHash *string    `gorm:"type:varchar(128)" json:"-"`
	VerifyExpires  *time.Time `gorm:"column:verify_expires_at" json:"-"`
	VerifyAttempts int        `gorm:"not null" json:"-"`
	VerifySentAt   *time.Time `gorm:"column:verify_sent_at" json:"-"`
	VerifiedAt     *time.Time `json:"verified_at,omitempty"`
	CreatedBy      *uuid.UUID `gorm:"type:uuid" json:"-"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName binds channels to their table.
func (Channel) TableName() string { return "notification_channels" }

// ChannelOwner addresses whose channels are managed.
type ChannelOwner struct {
	OrganizationID uuid.UUID
	WorkspaceID    *uuid.UUID
	Kind           string
	ID             uuid.UUID
}

func (o ChannelOwner) valid() bool {
	return o.OrganizationID != uuid.Nil && o.ID != uuid.Nil && (o.Kind == OwnerAccount || o.Kind == OwnerWorkspace || o.Kind == OwnerDigitalHuman)
}

// ChannelInput creates or changes a channel.
type ChannelInput struct {
	Type   string   `json:"type"`
	Name   string   `json:"name"`
	Target string   `json:"target"`
	Events []string `json:"events"`
}

// ChannelUpdate changes a channel's name, events, or whether it is enabled.
type ChannelUpdate struct {
	Name    *string   `json:"name,omitempty"`
	Events  *[]string `json:"events,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
}

// SecretCipher encrypts webhook signing secrets at rest.
type SecretCipher interface {
	EncryptCredentials(credentials map[string]string, aad credentialcrypto.CredentialAAD) (string, error)
	DecryptCredentials(envelope string, aad credentialcrypto.CredentialAAD) (map[string]string, error)
}

// EmailSender sends one HTML email.
type EmailSender func(ctx context.Context, to, subject, html, idempotencyKey string) error

// Channels manages notification channels.
type Channels struct {
	db     *gorm.DB
	cipher SecretCipher
	email  EmailSender
	now    func() time.Time
}

// NewChannels builds the channel registry. A nil cipher disables webhooks; a
// nil sender disables email.
func NewChannels(db *gorm.DB, cipher SecretCipher, email EmailSender) *Channels {
	if db == nil {
		panic("notification channels require db")
	}
	return &Channels{db: db, cipher: cipher, email: email, now: func() time.Time { return time.Now().UTC() }}
}

// List returns an owner's channels.
func (s *Channels) List(ctx context.Context, owner ChannelOwner) ([]*Channel, error) {
	if !owner.valid() {
		return nil, ErrInvalidRequest
	}
	var channels []*Channel
	err := s.ownerQuery(ctx, owner).Order("created_at").Find(&channels).Error
	if err != nil {
		return nil, fmt.Errorf("list notification channels: %w", err)
	}
	return channels, nil
}

// ActiveWorkspaceChannels reports whether every id is an active team
// channel of the workspace, so a task can report to it.
func (s *Channels) ActiveWorkspaceChannels(ctx context.Context, organizationID, workspaceID uuid.UUID, ids []uuid.UUID) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}
	var count int64
	err := s.db.WithContext(ctx).Model(&Channel{}).
		Where("organization_id = ? AND owner_kind = ? AND owner_id = ? AND status = ? AND id IN ?", organizationID, OwnerWorkspace, workspaceID, ChannelActive, ids).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check workspace notification channels: %w", err)
	}
	return count == int64(len(ids)), nil
}

// Create registers a channel. An email channel stays pending until its owner
// enters the code sent to it. A webhook channel is active at once; its signing
// secret is returned only here.
func (s *Channels) Create(ctx context.Context, owner ChannelOwner, actor uuid.UUID, input ChannelInput) (*Channel, string, error) {
	if !owner.valid() {
		return nil, "", ErrInvalidRequest
	}
	events, err := normalizeEvents(input.Events)
	if err != nil {
		return nil, "", err
	}
	target, err := normalizeTarget(input.Type, input.Target, owner.Kind)
	if err != nil {
		return nil, "", err
	}
	var count int64
	if err := s.ownerQuery(ctx, owner).Count(&count).Error; err != nil {
		return nil, "", fmt.Errorf("count notification channels: %w", err)
	}
	if count >= maxChannelsPerUser {
		return nil, "", ErrChannelConflict
	}
	now := s.now()
	channel := &Channel{
		ID: uuid.New(), OrganizationID: owner.OrganizationID, WorkspaceID: owner.WorkspaceID, OwnerKind: owner.Kind, OwnerID: owner.ID,
		Type: input.Type, Name: strings.TrimSpace(input.Name), Target: target, Events: events, CreatedAt: now, UpdatedAt: now,
	}
	if actor != uuid.Nil {
		channel.CreatedBy = &actor
	}
	if channel.Name == "" {
		channel.Name = target
	}
	var secret string
	switch input.Type {
	case ChannelEmail:
		if s.email == nil {
			return nil, "", ErrDeliveryUnavailable
		}
		channel.Status = ChannelPending
	case ChannelWebhook:
		if s.cipher == nil {
			return nil, "", ErrDeliveryUnavailable
		}
		secret = newSecret()
		envelope, err := s.cipher.EncryptCredentials(map[string]string{"secret": secret}, secretAAD(channel))
		if err != nil {
			return nil, "", fmt.Errorf("encrypt webhook secret: %w", err)
		}
		channel.SecretEnvelope, channel.Status = &envelope, ChannelActive
	}
	if err := s.db.WithContext(ctx).Create(channel).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, "", ErrChannelConflict
		}
		return nil, "", fmt.Errorf("create notification channel: %w", err)
	}
	if channel.Type == ChannelEmail {
		if err := s.sendCode(ctx, channel); err != nil {
			return channel, "", err
		}
	}
	return channel, secret, nil
}

// ResendCode sends a new verification code to a pending email channel.
func (s *Channels) ResendCode(ctx context.Context, owner ChannelOwner, id uuid.UUID) error {
	channel, err := s.load(ctx, owner, id)
	if err != nil {
		return err
	}
	if channel.Type != ChannelEmail || channel.Status != ChannelPending {
		return ErrChannelConflict
	}
	return s.sendCode(ctx, channel)
}

// Verify activates a pending email channel with the code sent to it.
func (s *Channels) Verify(ctx context.Context, owner ChannelOwner, id uuid.UUID, code string) (*Channel, error) {
	channel, err := s.load(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if channel.Type != ChannelEmail || channel.Status != ChannelPending {
		return nil, ErrChannelConflict
	}
	now := s.now()
	if channel.VerifyCodeHash == nil || channel.VerifyExpires == nil || now.After(*channel.VerifyExpires) || channel.VerifyAttempts >= verifyMaxAttempts {
		return nil, ErrVerificationFailed
	}
	if subtle.ConstantTimeCompare([]byte(codeHash(channel.ID, strings.TrimSpace(code))), []byte(*channel.VerifyCodeHash)) != 1 {
		s.db.WithContext(ctx).Model(&Channel{}).Where("id = ?", channel.ID).Update("verify_attempts", gorm.Expr("verify_attempts + 1"))
		return nil, ErrVerificationFailed
	}
	updates := map[string]any{"status": ChannelActive, "verified_at": now, "verify_code_hash": nil, "verify_expires_at": nil, "updated_at": now}
	if err := s.db.WithContext(ctx).Model(&Channel{}).Where("id = ?", channel.ID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("verify notification channel: %w", err)
	}
	channel.Status, channel.VerifiedAt = ChannelActive, &now
	return channel, nil
}

// Update renames a channel, changes its events, or enables or disables it.
// A pending email channel cannot be enabled before it is verified.
func (s *Channels) Update(ctx context.Context, owner ChannelOwner, id uuid.UUID, update ChannelUpdate) (*Channel, error) {
	channel, err := s.load(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"updated_at": s.now()}
	if update.Name != nil {
		name := strings.TrimSpace(*update.Name)
		if name == "" || len([]rune(name)) > 128 {
			return nil, ErrInvalidRequest
		}
		updates["name"], channel.Name = name, name
	}
	if update.Events != nil {
		events, err := normalizeEvents(*update.Events)
		if err != nil {
			return nil, err
		}
		updates["events"], channel.Events = eventsJSON(events), events
	}
	if update.Enabled != nil {
		switch {
		case channel.Status == ChannelPending:
			return nil, ErrChannelConflict
		case *update.Enabled:
			updates["status"], channel.Status = ChannelActive, ChannelActive
		default:
			updates["status"], channel.Status = ChannelDisabled, ChannelDisabled
		}
	}
	if err := s.db.WithContext(ctx).Model(&Channel{}).Where("id = ?", channel.ID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update notification channel: %w", err)
	}
	return channel, nil
}

// RotateSecret replaces a webhook's signing secret and returns the new one.
func (s *Channels) RotateSecret(ctx context.Context, owner ChannelOwner, id uuid.UUID) (string, error) {
	channel, err := s.load(ctx, owner, id)
	if err != nil {
		return "", err
	}
	if channel.Type != ChannelWebhook || s.cipher == nil {
		return "", ErrChannelConflict
	}
	secret := newSecret()
	envelope, err := s.cipher.EncryptCredentials(map[string]string{"secret": secret}, secretAAD(channel))
	if err != nil {
		return "", fmt.Errorf("encrypt webhook secret: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&Channel{}).Where("id = ?", channel.ID).Updates(map[string]any{"secret_envelope": envelope, "updated_at": s.now()}).Error; err != nil {
		return "", fmt.Errorf("rotate webhook secret: %w", err)
	}
	return secret, nil
}

// Delete removes a channel and its delivery history.
func (s *Channels) Delete(ctx context.Context, owner ChannelOwner, id uuid.UUID) error {
	if _, err := s.load(ctx, owner, id); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where("id = ?", id).Delete(&Channel{}).Error
}

func (s *Channels) webhookSecret(channel *Channel) (string, error) {
	if s.cipher == nil || channel.SecretEnvelope == nil {
		return "", ErrDeliveryUnavailable
	}
	values, err := s.cipher.DecryptCredentials(*channel.SecretEnvelope, secretAAD(channel))
	if err != nil {
		return "", fmt.Errorf("decrypt webhook secret: %w", err)
	}
	return values["secret"], nil
}

func (s *Channels) sendCode(ctx context.Context, channel *Channel) error {
	now := s.now()
	if channel.VerifySentAt != nil && now.Sub(*channel.VerifySentAt) < verifyCodeCooldown {
		return ErrRateLimited
	}
	code, err := sixDigitCode()
	if err != nil {
		return err
	}
	expires := now.Add(verifyCodeTTL)
	hash := codeHash(channel.ID, code)
	if err := s.db.WithContext(ctx).Model(&Channel{}).Where("id = ?", channel.ID).Updates(map[string]any{
		"verify_code_hash": hash, "verify_expires_at": expires, "verify_attempts": 0, "verify_sent_at": now, "updated_at": now,
	}).Error; err != nil {
		return fmt.Errorf("store verification code: %w", err)
	}
	channel.VerifySentAt = &now
	subject := "ZGI notification email verification / 通知邮箱验证"
	body := fmt.Sprintf(`<p>Your ZGI notification email verification code is <strong>%s</strong>. It expires in 15 minutes.</p>
<p>你的 ZGI 通知邮箱验证码是 <strong>%s</strong>，15 分钟内有效。</p>
<p>If you did not request this, ignore this email. / 如果不是你本人操作，请忽略这封邮件。</p>`, code, code)
	if err := s.email(ctx, channel.Target, subject, body, "notification-verify-"+channel.ID.String()+"-"+now.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("%w: %v", ErrDeliveryUnavailable, err)
	}
	return nil
}

func (s *Channels) ownerQuery(ctx context.Context, owner ChannelOwner) *gorm.DB {
	return s.db.WithContext(ctx).Model(&Channel{}).
		Where("organization_id = ? AND owner_kind = ? AND owner_id = ?", owner.OrganizationID, owner.Kind, owner.ID)
}

func (s *Channels) load(ctx context.Context, owner ChannelOwner, id uuid.UUID) (*Channel, error) {
	if !owner.valid() || id == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var channel Channel
	if err := s.ownerQuery(ctx, owner).Where("id = ?", id).First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChannelNotFound
		}
		return nil, fmt.Errorf("load notification channel: %w", err)
	}
	return &channel, nil
}

func normalizeEvents(events []string) ([]string, error) {
	result := make([]string, 0, len(events))
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event != EventTaskReport && event != EventTaskFailed {
			return nil, ErrInvalidRequest
		}
		if !slices.Contains(result, event) {
			result = append(result, event)
		}
	}
	if len(result) == 0 {
		return nil, ErrInvalidRequest
	}
	return result, nil
}

func normalizeTarget(channelType, target, ownerKind string) (string, error) {
	target = strings.TrimSpace(target)
	switch channelType {
	case ChannelEmail:
		// Email reaches a person, so only personal channels use it.
		address, err := mail.ParseAddress(target)
		if err != nil || ownerKind != OwnerAccount || address.Address != target || len(target) > 254 {
			return "", ErrInvalidRequest
		}
		return strings.ToLower(target), nil
	case ChannelWebhook:
		parsed, err := url.Parse(target)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || len(target) > 2048 {
			return "", ErrInvalidRequest
		}
		return parsed.String(), nil
	default:
		return "", ErrInvalidRequest
	}
}

func secretAAD(channel *Channel) credentialcrypto.CredentialAAD {
	return credentialcrypto.CredentialAAD{OrganizationID: channel.OrganizationID, ConnectionID: channel.ID, IntegrationID: "notification_webhook", CredentialVersion: 1}
}

func newSecret() string {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("read random secret: %v", err))
	}
	return "whsec_" + hex.EncodeToString(buffer)
}

func sixDigitCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate verification code: %w", err)
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func codeHash(channelID uuid.UUID, code string) string {
	sum := sha256.Sum256([]byte(channelID.String() + ":" + code))
	return hex.EncodeToString(sum[:])
}

func eventsJSON(events []string) string {
	return `["` + strings.Join(events, `","`) + `"]`
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint") || errors.Is(err, gorm.ErrDuplicatedKey)
}
