package inbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// maxDeliveryLogItems bounds the recent deliveries shown for one channel.
const maxDeliveryLogItems = 50

// DeliveryLogItem is one recent delivery to a channel, as its owner sees it.
type DeliveryLogItem struct {
	ID        uuid.UUID  `json:"id"`
	Event     string     `json:"event"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Attempts  int        `json:"attempts"`
	LastError string     `json:"last_error,omitempty"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ListDeliveries returns a channel's most recent deliveries, newest first, so
// its owner can see what reached the receiver and what failed.
func (d *Dispatcher) ListDeliveries(ctx context.Context, owner ChannelOwner, channelID uuid.UUID, limit int) ([]DeliveryLogItem, error) {
	if _, err := d.channels.load(ctx, owner, channelID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxDeliveryLogItems {
		limit = 20
	}
	var rows []struct {
		Delivery
		NotificationEvent string
		NotificationTitle string
	}
	if err := d.db.WithContext(ctx).Table("notification_deliveries").
		Select("notification_deliveries.*, inbox_notifications.event AS notification_event, inbox_notifications.title AS notification_title").
		Joins("JOIN inbox_notifications ON inbox_notifications.id = notification_deliveries.notification_id").
		Where("notification_deliveries.channel_id = ?", channelID).
		Order("notification_deliveries.created_at DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list notification deliveries: %w", err)
	}
	items := make([]DeliveryLogItem, 0, len(rows))
	for _, row := range rows {
		item := DeliveryLogItem{ID: row.ID, Event: webhookEvent(row.NotificationEvent), Title: row.NotificationTitle, Status: row.Status,
			Attempts: row.Attempts, SentAt: row.SentAt, CreatedAt: row.CreatedAt}
		if row.LastError != nil {
			item.LastError = *row.LastError
		}
		items = append(items, item)
	}
	return items, nil
}

// Redeliver sends a failed delivery again from the first attempt, once the
// owner has fixed the receiving end.
func (d *Dispatcher) Redeliver(ctx context.Context, owner ChannelOwner, channelID, deliveryID uuid.UUID) error {
	channel, err := d.channels.load(ctx, owner, channelID)
	if err != nil {
		return err
	}
	if channel.Status != ChannelActive {
		return ErrChannelConflict
	}
	result := d.db.WithContext(ctx).Model(&Delivery{}).
		Where("id = ? AND channel_id = ? AND status = ?", deliveryID, channelID, DeliveryFailed).
		Updates(map[string]any{"status": DeliveryPending, "attempts": 0, "last_error": nil, "updated_at": d.now()})
	if result.Error != nil {
		return fmt.Errorf("reset notification delivery: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var existing Delivery
		if err := d.db.WithContext(ctx).Select("id").First(&existing, "id = ? AND channel_id = ?", deliveryID, channelID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChannelNotFound
		}
		return ErrChannelConflict
	}
	if d.enqueue != nil {
		return d.enqueue(ctx, deliveryID)
	}
	return nil
}
