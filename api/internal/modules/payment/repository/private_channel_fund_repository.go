package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PrivateChannelFundItem represents a private channel wallet snapshot.
type PrivateChannelFundItem struct {
	ChannelID   uuid.UUID `gorm:"column:channel_id" json:"channel_id"`
	ChannelName string    `gorm:"column:channel_name" json:"channel_name"`
	Balance     int64     `gorm:"column:balance" json:"balance"`
	Status      string    `gorm:"column:status" json:"status"`
	Currency    string    `gorm:"column:currency" json:"currency"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// PrivateChannelFundRepository defines the interface for private channel funds queries.
type PrivateChannelFundRepository interface {
	ListByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]*PrivateChannelFundItem, error)
}

type privateChannelFundRepository struct {
	db *gorm.DB
}

// NewPrivateChannelFundRepository creates a new private channel fund repository.
func NewPrivateChannelFundRepository(db *gorm.DB) PrivateChannelFundRepository {
	return &privateChannelFundRepository{db: db}
}

func (r *privateChannelFundRepository) ListByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]*PrivateChannelFundItem, error) {
	items := make([]*PrivateChannelFundItem, 0)
	db := GetDB(ctx, r.db)

	if !db.Migrator().HasTable("channel_wallets") || !db.Migrator().HasColumn("channel_wallets", "organization_id") {
		return items, nil
	}
	if !db.Migrator().HasTable("llm_routes") {
		return items, nil
	}

	err := db.Table("channel_wallets AS cw").
		Joins(`
			INNER JOIN llm_routes AS routes
				ON routes.id = cw.channel_id
				AND routes.type = 'PRIVATE'
				AND routes.is_official = FALSE
				AND routes.is_enabled = TRUE
				AND routes.deleted_at IS NULL
		`).
		Select(`
			cw.channel_id,
			COALESCE(routes.name, '') AS channel_name,
			cw.balance,
			cw.status,
			COALESCE(routes.currency, 'USD') AS currency,
			cw.updated_at
		`).
		Where("cw.organization_id = ?", organizationID).
		Order("cw.updated_at DESC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list private channel funds: %w", err)
	}

	return items, nil
}
