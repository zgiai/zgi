package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/ginext/internal/modules/llm/channel/dto"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	channelWalletStatusActive = "ACTIVE"
	channelWalletStatusDebt   = "DEBT"

	channelWalletTxTypeAdjust = "adjust"
)

type channelWalletRouteSnapshot struct {
	ID             uuid.UUID        `gorm:"column:id"`
	OrganizationID uuid.UUID        `gorm:"column:organization_id"`
	Type           shared.RouteType `gorm:"column:type"`
	IsOfficial     bool             `gorm:"column:is_official"`
	Balance        decimal.Decimal  `gorm:"column:balance"`
}

func (channelWalletRouteSnapshot) TableName() string {
	return "llm_routes"
}

type channelWalletRecord struct {
	ChannelID      uuid.UUID `gorm:"column:channel_id"`
	OrganizationID uuid.UUID `gorm:"column:organization_id"`
	Balance        int64     `gorm:"column:balance"`
	Status         string    `gorm:"column:status"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (channelWalletRecord) TableName() string {
	return "channel_wallets"
}

type channelWalletTransactionRecord struct {
	ID            uuid.UUID              `gorm:"column:id"`
	ChannelID     uuid.UUID              `gorm:"column:channel_id"`
	AttemptID     *string                `gorm:"column:attempt_id"`
	Type          string                 `gorm:"column:type"`
	Amount        int64                  `gorm:"column:amount"`
	BalanceBefore int64                  `gorm:"column:balance_before"`
	BalanceAfter  int64                  `gorm:"column:balance_after"`
	Metadata      map[string]interface{} `gorm:"column:metadata;type:jsonb;serializer:json"`
	CreatedAt     time.Time              `gorm:"column:created_at"`
}

func (channelWalletTransactionRecord) TableName() string {
	return "channel_wallet_transactions"
}

func (s *channelService) AdjustChannelWallet(
	ctx context.Context,
	organizationID uuid.UUID,
	channelID uuid.UUID,
	req *dto.AdjustChannelWalletRequest,
) (*dto.AdjustChannelWalletResponse, error) {
	if req == nil || req.Amount == 0 {
		return nil, fmt.Errorf("invalid adjustment amount: must not be zero")
	}

	var response dto.AdjustChannelWalletResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		route, err := s.loadPrivateRouteForAdjust(ctx, tx, organizationID, channelID)
		if err != nil {
			return err
		}

		wallet, err := s.getOrCreateChannelWalletForAdjust(ctx, tx, route)
		if err != nil {
			return err
		}

		balanceBefore := wallet.Balance
		balanceAfter := balanceBefore + req.Amount
		newStatus := channelWalletStatusActive
		if balanceAfter < 0 {
			newStatus = channelWalletStatusDebt
		}

		now := time.Now()
		if err := tx.WithContext(ctx).
			Model(&channelWalletRecord{}).
			Where("channel_id = ?", channelID).
			Updates(map[string]interface{}{
				"balance":    balanceAfter,
				"status":     newStatus,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("failed to update channel wallet balance: %w", err)
		}

		if err := s.syncRouteBalanceForAdjust(ctx, tx, organizationID, channelID, balanceAfter, now); err != nil {
			return err
		}

		txRecord, err := s.createChannelWalletAdjustTransaction(
			ctx, tx, organizationID, channelID, req, balanceBefore, balanceAfter, now,
		)
		if err != nil {
			return err
		}

		response = dto.AdjustChannelWalletResponse{
			ChannelID:      channelID,
			OrganizationID: organizationID,
			Amount:         req.Amount,
			BalanceBefore:  balanceBefore,
			BalanceAfter:   balanceAfter,
			Status:         newStatus,
			TransactionID:  txRecord.ID,
			UpdatedAt:      now.Format(time.RFC3339),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (s *channelService) loadPrivateRouteForAdjust(
	ctx context.Context,
	tx *gorm.DB,
	organizationID uuid.UUID,
	channelID uuid.UUID,
) (*channelWalletRouteSnapshot, error) {
	var route channelWalletRouteSnapshot
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND organization_id = ? AND deleted_at IS NULL", channelID, organizationID).
		First(&route).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRouteNotFound
		}
		return nil, fmt.Errorf("failed to load route for adjustment: %w", err)
	}
	if route.IsOfficial || route.Type != shared.RouteTypePrivate {
		return nil, fmt.Errorf("%w: only private channels support wallet adjustment", ErrInvalidRouteType)
	}
	return &route, nil
}

func (s *channelService) getOrCreateChannelWalletForAdjust(
	ctx context.Context,
	tx *gorm.DB,
	route *channelWalletRouteSnapshot,
) (*channelWalletRecord, error) {
	var wallet channelWalletRecord
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("channel_id = ?", route.ID).
		First(&wallet).Error
	if err == nil {
		return &wallet, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to load channel wallet: %w", err)
	}

	initialBalance := route.Balance.Round(0).IntPart()
	initialStatus := channelWalletStatusActive
	if initialBalance < 0 {
		initialStatus = channelWalletStatusDebt
	}

	now := time.Now()
	wallet = channelWalletRecord{
		ChannelID:      route.ID,
		OrganizationID: route.OrganizationID,
		Balance:        initialBalance,
		Status:         initialStatus,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := tx.WithContext(ctx).Create(&wallet).Error; err != nil {
		return nil, fmt.Errorf("failed to create channel wallet: %w", err)
	}
	return &wallet, nil
}

func (s *channelService) syncRouteBalanceForAdjust(
	ctx context.Context,
	tx *gorm.DB,
	organizationID uuid.UUID,
	channelID uuid.UUID,
	balance int64,
	now time.Time,
) error {
	err := tx.WithContext(ctx).
		Model(&channelWalletRouteSnapshot{}).
		Where("id = ? AND organization_id = ?", channelID, organizationID).
		Updates(map[string]interface{}{
			"balance":    decimal.NewFromInt(balance),
			"updated_at": now,
		}).Error
	if err != nil {
		return fmt.Errorf("failed to sync route balance snapshot: %w", err)
	}
	return nil
}

func (s *channelService) createChannelWalletAdjustTransaction(
	ctx context.Context,
	tx *gorm.DB,
	organizationID uuid.UUID,
	channelID uuid.UUID,
	req *dto.AdjustChannelWalletRequest,
	balanceBefore int64,
	balanceAfter int64,
	now time.Time,
) (*channelWalletTransactionRecord, error) {
	metadata := map[string]interface{}{
		"phase":           "manual_adjust",
		"organization_id": organizationID.String(),
	}
	if note := strings.TrimSpace(req.Note); note != "" {
		metadata["note"] = note
	}

	txRecord := &channelWalletTransactionRecord{
		ID:            uuid.New(),
		ChannelID:     channelID,
		Type:          channelWalletTxTypeAdjust,
		Amount:        req.Amount,
		BalanceBefore: balanceBefore,
		BalanceAfter:  balanceAfter,
		Metadata:      metadata,
		CreatedAt:     now,
	}
	if err := tx.WithContext(ctx).Create(txRecord).Error; err != nil {
		return nil, fmt.Errorf("failed to create wallet adjustment transaction: %w", err)
	}
	return txRecord, nil
}
