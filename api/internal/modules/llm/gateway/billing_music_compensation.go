package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const musicDeliveryCompensationCode = "MUSIC_DELIVERY_COMPENSATED"

type musicPricingSnapshot struct {
	Operation PricingOperation `json:"operation"`
	Meter     string           `json:"meter"`
	BaseUnit  string           `json:"base_unit"`
	Quantity  int64            `json:"quantity"`
}

type privateMusicDeliveryCompensator interface {
	CompensatePrivateMusicDelivery(ctx context.Context, organizationID uuid.UUID, requestID string) error
}

// CompensatePrivateMusicDelivery refunds one settled private-channel music
// charge after the generated file could not be durably delivered.
func (b *BillingService) CompensatePrivateMusicDelivery(ctx context.Context, organizationID uuid.UUID, requestID string) error {
	requestID = strings.TrimSpace(requestID)
	if b == nil || b.db == nil || organizationID == uuid.Nil || requestID == "" {
		return fmt.Errorf("%w: invalid private music compensation request", adapter.ErrInvalidRequest)
	}

	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("attempt_id = ? AND request_id = ? AND organization_id = ?", requestID, requestID, organizationID).
			First(&attempt).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return adapter.ErrMusicCompensationNotFound
		}
		if err != nil {
			return fmt.Errorf("load private music billing attempt: %w", err)
		}
		if attempt.Lane != billingAttemptLaneLocal {
			return adapter.ErrMusicCompensationNotFound
		}
		switch attempt.Status {
		case billingAttemptStatusCompensated:
			return nil
		case billingAttemptStatusRolledBack, billingAttemptStatusPredeductFailed:
			return adapter.ErrMusicCompensationNotCharged
		case billingAttemptStatusSettled:
		case billingAttemptStatusInit, billingAttemptStatusPre, billingAttemptStatusSettlePending,
			billingAttemptStatusPartial, billingAttemptStatusDeadLetter:
			return adapter.ErrMusicCompensationNotReady
		default:
			return fmt.Errorf("%w: unsupported billing status %q", adapter.ErrMusicCompensationNotReady, attempt.Status)
		}

		bill, err := loadPrivateMusicUsageBill(ctx, tx, attempt)
		if err != nil {
			return err
		}
		subjectEntry, fundEntry, err := loadPrivateMusicBillingEntries(ctx, tx, attempt.AttemptID)
		if err != nil {
			return err
		}
		refundCredits := subjectEntry.ActualAmount
		if fundEntry.ActualAmount != refundCredits || bill.PrivatePoints != refundCredits || bill.TotalPoints != refundCredits || bill.OfficialPoints != 0 {
			return fmt.Errorf("private music compensation amount mismatch for attempt %s", attempt.AttemptID)
		}
		if err := refundPrivateMusicSubject(ctx, tx, attempt, refundCredits); err != nil {
			return err
		}
		if err := b.refundPrivateMusicWallet(ctx, tx, attempt, fundEntry, refundCredits); err != nil {
			return err
		}
		if err := markPrivateMusicEntriesRefunded(ctx, tx, subjectEntry, fundEntry, refundCredits); err != nil {
			return err
		}
		if err := markPrivateMusicUsageBillCompensated(ctx, tx, bill); err != nil {
			return err
		}

		invocationResult := "error"
		errorCode := musicDeliveryCompensationCode
		errorMessage := "generated music could not be delivered"
		result := tx.WithContext(ctx).Model(&BillingAttempt{}).
			Where("attempt_id = ? AND organization_id = ?", attempt.AttemptID, organizationID).
			Updates(map[string]any{
				"status":            billingAttemptStatusCompensated,
				"invocation_result": invocationResult,
				"error_code":        errorCode,
				"error_message":     errorMessage,
				"updated_at":        time.Now(),
			})
		if result.Error != nil {
			return fmt.Errorf("mark private music attempt compensated: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("mark private music attempt compensated affected %d rows", result.RowsAffected)
		}
		return nil
	})
}

func loadPrivateMusicUsageBill(ctx context.Context, tx *gorm.DB, attempt BillingAttempt) (*UsageBill, error) {
	var bill UsageBill
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("attempt_id = ? AND request_id = ? AND organization_id = ?", attempt.AttemptID, attempt.RequestID, attempt.OrganizationID).
		First(&bill).Error
	if err != nil {
		return nil, fmt.Errorf("load private music usage bill: %w", err)
	}
	if bill.BillingLane != UsageBillingLanePrivate || bill.UseSystemProvider || bill.Status != usageBillStatusSuccess || bill.ChannelID == nil {
		return nil, adapter.ErrMusicCompensationNotCharged
	}
	var snapshot musicPricingSnapshot
	if err := json.Unmarshal(bill.PricingSnapshot, &snapshot); err != nil {
		return nil, fmt.Errorf("%w: invalid music pricing snapshot", adapter.ErrMusicCompensationNotCharged)
	}
	if snapshot.Operation != PricingOperationMusic || snapshot.Meter != meterOutputTrack || snapshot.BaseUnit != baseUnitTrack || snapshot.Quantity != 1 {
		return nil, fmt.Errorf("%w: billing attempt is not music generation", adapter.ErrMusicCompensationNotCharged)
	}
	return &bill, nil
}

func loadPrivateMusicBillingEntries(ctx context.Context, tx *gorm.DB, attemptID string) (*BillingAttemptEntry, *BillingAttemptEntry, error) {
	var entries []BillingAttemptEntry
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("attempt_id = ?", attemptID).
		Find(&entries).Error; err != nil {
		return nil, nil, fmt.Errorf("load private music billing entries: %w", err)
	}
	var subjectEntry, fundEntry *BillingAttemptEntry
	for i := range entries {
		entry := &entries[i]
		switch {
		case entry.EntryType == billingEntryTypeSubject:
			subjectEntry = entry
		case entry.EntryType == billingEntryTypeFund && entry.LedgerType == billingLedgerTypeChannelWallet:
			fundEntry = entry
		}
	}
	if len(entries) != 2 || subjectEntry == nil || fundEntry == nil {
		return nil, nil, fmt.Errorf("private music billing entries are incomplete for attempt %s", attemptID)
	}
	for _, entry := range []*BillingAttemptEntry{subjectEntry, fundEntry} {
		if entry.Status != billingEntryStatusSettled || entry.ActualAmount < 0 || entry.RefundedAmount < 0 ||
			entry.ReservedAmount != entry.ActualAmount+entry.RefundedAmount {
			return nil, nil, fmt.Errorf("private music billing entry is inconsistent for attempt %s", attemptID)
		}
	}
	return subjectEntry, fundEntry, nil
}

func refundPrivateMusicSubject(ctx context.Context, tx *gorm.DB, attempt BillingAttempt, amount int64) error {
	switch attempt.QuotaSubjectType {
	case quotaSubjectTypeAPIKey:
		var apiKey apikeymodel.TenantAPIKey
		if err := tx.WithContext(ctx).Unscoped().
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND organization_id = ?", attempt.QuotaSubjectID, attempt.OrganizationID).
			First(&apiKey).Error; err != nil {
			return fmt.Errorf("load api key for music compensation: %w", err)
		}
		if apiKey.UsedQuota < amount {
			return fmt.Errorf("api key used quota is smaller than music refund")
		}
		apiKey.UsedQuota -= amount
		if apiKey.QuotaLimit != nil {
			apiKey.RemainQuota += amount
		}
		return tx.WithContext(ctx).Unscoped().Save(&apiKey).Error
	case quotaSubjectTypeWorkspace:
		var quota WorkspaceQuota
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND organization_id = ?", attempt.QuotaSubjectID, attempt.OrganizationID).
			First(&quota).Error; err != nil {
			return fmt.Errorf("load workspace quota for music compensation: %w", err)
		}
		if quota.UsedQuota < amount {
			return fmt.Errorf("workspace used quota is smaller than music refund")
		}
		quota.UsedQuota -= amount
		if quota.QuotaLimit != nil {
			quota.RemainQuota += amount
		}
		quota.UpdatedAt = time.Now()
		return tx.WithContext(ctx).Save(&quota).Error
	case quotaSubjectTypeOrganization:
		return nil
	default:
		return fmt.Errorf("unsupported quota subject type %q for music compensation", attempt.QuotaSubjectType)
	}
}

func (b *BillingService) refundPrivateMusicWallet(
	ctx context.Context,
	tx *gorm.DB,
	attempt BillingAttempt,
	fundEntry *BillingAttemptEntry,
	amount int64,
) error {
	channelID, err := uuid.Parse(fundEntry.LedgerRefID)
	if err != nil || attempt.RouteID == nil || *attempt.RouteID != channelID {
		return fmt.Errorf("private music channel identity is invalid")
	}
	var wallet ChannelWallet
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("channel_id = ? AND organization_id = ?", channelID, attempt.OrganizationID).
		First(&wallet).Error; err != nil {
		return fmt.Errorf("load channel wallet for music compensation: %w", err)
	}
	before := wallet.Balance
	after := before + amount
	status := channelWalletStatusActive
	if after < 0 {
		status = channelWalletStatusDebt
	}
	result := tx.WithContext(ctx).Model(&ChannelWallet{}).
		Where("channel_id = ? AND organization_id = ?", channelID, attempt.OrganizationID).
		Updates(map[string]any{"balance": after, "status": status, "updated_at": time.Now()})
	if result.Error != nil {
		return fmt.Errorf("refund private music channel wallet: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("refund private music channel wallet affected %d rows", result.RowsAffected)
	}
	if err := b.syncRouteBalanceSnapshot(ctx, tx, channelID, after); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	return b.createChannelWalletTransaction(
		ctx,
		tx,
		channelID,
		attempt.AttemptID,
		channelWalletTxTypeRefund,
		amount,
		before,
		after,
		map[string]any{
			"phase":            billingPhaseCompensate,
			"reason":           "music_delivery_failed",
			"request_id":       attempt.RequestID,
			"attempt_id":       attempt.AttemptID,
			"refunded_credits": amount,
		},
	)
}

func markPrivateMusicEntriesRefunded(
	ctx context.Context,
	tx *gorm.DB,
	subjectEntry *BillingAttemptEntry,
	fundEntry *BillingAttemptEntry,
	amount int64,
) error {
	for _, entry := range []*BillingAttemptEntry{subjectEntry, fundEntry} {
		result := tx.WithContext(ctx).Model(&BillingAttemptEntry{}).
			Where("id = ? AND attempt_id = ?", entry.ID, entry.AttemptID).
			Updates(map[string]any{
				"refunded_amount": entry.RefundedAmount + amount,
				"status":          billingEntryStatusRefunded,
				"updated_at":      time.Now(),
			})
		if result.Error != nil {
			return fmt.Errorf("mark private music billing entry refunded: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("mark private music billing entry refunded affected %d rows", result.RowsAffected)
		}
	}
	return nil
}

func markPrivateMusicUsageBillCompensated(ctx context.Context, tx *gorm.DB, bill *UsageBill) error {
	errorCode := musicDeliveryCompensationCode
	errorMessage := "generated music could not be delivered"
	result := tx.WithContext(ctx).Model(&UsageBill{}).
		Where("attempt_id = ?", bill.AttemptID).
		Updates(map[string]any{
			"status":          usageBillStatusFailed,
			"official_points": 0,
			"private_points":  0,
			"total_points":    0,
			"error_code":      errorCode,
			"error_message":   errorMessage,
		})
	if result.Error != nil {
		return fmt.Errorf("mark private music usage bill compensated: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("mark private music usage bill compensated affected %d rows", result.RowsAffected)
	}
	return nil
}
