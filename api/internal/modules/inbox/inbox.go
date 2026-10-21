// Package inbox is the in-app notification inbox (ADR 0008). It stores one
// notification per recipient and source event, with read state. Other
// notification channels, such as email or a webhook, deliver the same events
// elsewhere; the inbox is always on and is the fallback when nothing else is
// bound or a delivery fails.
package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Recipient kinds match user subject kinds.
const (
	RecipientAccount    = "account"
	RecipientAPIEndUser = "api_end_user"
)

// ErrInvalidRequest marks malformed inbox input.
var ErrInvalidRequest = errors.New("invalid inbox request")

// Notification is one inbox entry.
type Notification struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID    *uuid.UUID     `gorm:"type:uuid" json:"workspace_id,omitempty"`
	RecipientKind  string         `gorm:"type:varchar(32);not null" json:"-"`
	RecipientID    uuid.UUID      `gorm:"type:uuid;not null" json:"-"`
	Event          string         `gorm:"type:varchar(64);not null" json:"event"`
	SourceType     string         `gorm:"type:varchar(64);not null" json:"source_type"`
	SourceID       uuid.UUID      `gorm:"type:uuid;not null" json:"source_id"`
	Title          string         `gorm:"type:text;not null" json:"title"`
	Summary        string         `gorm:"type:text;not null" json:"summary"`
	Link           string         `gorm:"type:text;not null" json:"link"`
	Context        datatypes.JSON `gorm:"type:jsonb;not null" json:"context"`
	ReadAt         *time.Time     `json:"read_at,omitempty"`
	CreatedAt      time.Time      `gorm:"not null" json:"created_at"`
}

// TableName binds notifications to their table.
func (Notification) TableName() string { return "inbox_notifications" }

// Recipient addresses one person's inbox within an organization.
type Recipient struct {
	OrganizationID uuid.UUID
	Kind           string
	ID             uuid.UUID
}

func (r Recipient) valid() bool {
	return r.OrganizationID != uuid.Nil && r.ID != uuid.Nil && (r.Kind == RecipientAccount || r.Kind == RecipientAPIEndUser)
}

// NotifyRequest records one event for one recipient.
type NotifyRequest struct {
	Recipient   Recipient
	WorkspaceID *uuid.UUID
	Event       string
	SourceType  string
	SourceID    uuid.UUID
	Title       string
	Summary     string
	Link        string
	Context     map[string]any
}

// ListQuery pages a recipient's inbox, newest first.
type ListQuery struct {
	UnreadOnly bool
	Page       int
	PageSize   int
}

// List is one page of notifications with the recipient's unread count.
type List struct {
	Items       []*Notification `json:"items"`
	Total       int64           `json:"total"`
	UnreadCount int64           `json:"unread_count"`
	Page        int             `json:"page"`
	PageSize    int             `json:"page_size"`
}

// Service stores and serves inbox notifications.
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

// NewService builds an inbox backed by db.
func NewService(db *gorm.DB) *Service {
	if db == nil {
		panic("inbox requires db")
	}
	return &Service{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Notify stores a notification once per recipient, event, and source.
func (s *Service) Notify(ctx context.Context, req NotifyRequest) (*Notification, error) {
	if !req.Recipient.valid() || strings.TrimSpace(req.Event) == "" || strings.TrimSpace(req.SourceType) == "" || req.SourceID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	contextValue := datatypes.JSON(`{}`)
	if len(req.Context) > 0 {
		encoded, err := json.Marshal(req.Context)
		if err != nil {
			return nil, fmt.Errorf("encode inbox context: %w", err)
		}
		contextValue = encoded
	}
	notification := &Notification{
		ID: uuid.New(), OrganizationID: req.Recipient.OrganizationID, WorkspaceID: req.WorkspaceID,
		RecipientKind: req.Recipient.Kind, RecipientID: req.Recipient.ID, Event: strings.TrimSpace(req.Event),
		SourceType: strings.TrimSpace(req.SourceType), SourceID: req.SourceID, Title: strings.TrimSpace(req.Title),
		Summary: strings.TrimSpace(req.Summary), Link: strings.TrimSpace(req.Link), Context: contextValue, CreatedAt: s.now(),
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "recipient_kind"}, {Name: "recipient_id"}, {Name: "event"}, {Name: "source_type"}, {Name: "source_id"}},
		DoNothing: true,
	}).Create(notification).Error; err != nil {
		return nil, fmt.Errorf("store inbox notification: %w", err)
	}
	return notification, nil
}

func (s *Service) recipientQuery(ctx context.Context, recipient Recipient) *gorm.DB {
	return s.db.WithContext(ctx).Model(&Notification{}).
		Where("organization_id = ? AND recipient_kind = ? AND recipient_id = ?", recipient.OrganizationID, recipient.Kind, recipient.ID)
}

// List pages a recipient's notifications.
func (s *Service) List(ctx context.Context, recipient Recipient, query ListQuery) (*List, error) {
	if !recipient.valid() {
		return nil, ErrInvalidRequest
	}
	page, pageSize := query.Page, query.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	base := s.recipientQuery(ctx, recipient)
	if query.UnreadOnly {
		base = base.Where("read_at IS NULL")
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count inbox notifications: %w", err)
	}
	var items []*Notification
	if err := base.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list inbox notifications: %w", err)
	}
	unread, err := s.UnreadCount(ctx, recipient)
	if err != nil {
		return nil, err
	}
	return &List{Items: items, Total: total, UnreadCount: unread, Page: page, PageSize: pageSize}, nil
}

// UnreadCount counts a recipient's unread notifications.
func (s *Service) UnreadCount(ctx context.Context, recipient Recipient) (int64, error) {
	if !recipient.valid() {
		return 0, ErrInvalidRequest
	}
	var count int64
	if err := s.recipientQuery(ctx, recipient).Where("read_at IS NULL").Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count unread inbox notifications: %w", err)
	}
	return count, nil
}

// MarkRead marks the given notifications of a recipient as read. Unknown or
// foreign identifiers are ignored.
func (s *Service) MarkRead(ctx context.Context, recipient Recipient, ids []uuid.UUID) error {
	if !recipient.valid() || len(ids) == 0 || len(ids) > 100 {
		return ErrInvalidRequest
	}
	return s.recipientQuery(ctx, recipient).Where("id IN ? AND read_at IS NULL", ids).Update("read_at", s.now()).Error
}

// MarkSourceRead marks every notification about one source as read, for
// example when the recipient opens the report it points to.
func (s *Service) MarkSourceRead(ctx context.Context, recipient Recipient, sourceType string, sourceID uuid.UUID) error {
	if !recipient.valid() || strings.TrimSpace(sourceType) == "" || sourceID == uuid.Nil {
		return ErrInvalidRequest
	}
	return s.recipientQuery(ctx, recipient).Where("source_type = ? AND source_id = ? AND read_at IS NULL", sourceType, sourceID).Update("read_at", s.now()).Error
}

// MarkAllRead marks all of a recipient's notifications as read.
func (s *Service) MarkAllRead(ctx context.Context, recipient Recipient) error {
	if !recipient.valid() {
		return ErrInvalidRequest
	}
	return s.recipientQuery(ctx, recipient).Where("read_at IS NULL").Update("read_at", s.now()).Error
}
