package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/zgiai/ginext/internal/modules/payment/model"
)

// TransactionRepository defines the interface for transaction operations
// This is the unified transaction repository for both cash and credits
type TransactionRepository interface {
	Create(ctx context.Context, tx *model.Transaction) error
	CreateBatch(ctx context.Context, txs []*model.Transaction) error
	GetByID(ctx context.Context, id string) (*model.Transaction, error)
	ListByGroupID(ctx context.Context, groupID uuid.UUID, limit, offset int) ([]*model.Transaction, error)
	ListByBatchID(ctx context.Context, batchID string) ([]*model.Transaction, error)

	// Credit transaction methods (for AI credits)
	ListCreditsByGroupID(ctx context.Context, groupID uuid.UUID, currencyType string, limit, offset int) ([]*model.Transaction, error)
	ListCreditsByReferenceID(ctx context.Context, referenceID string) ([]*model.Transaction, error)
	GetCreditsTotalByType(ctx context.Context, groupID uuid.UUID, transactionType string) (float64, error)
}

type transactionRepository struct {
	db *gorm.DB
}

// NewTransactionRepository creates a new transaction repository
func NewTransactionRepository(db *gorm.DB) TransactionRepository {
	return &transactionRepository{db: db}
}

func (r *transactionRepository) Create(ctx context.Context, tx *model.Transaction) error {
	if err := GetDB(ctx, r.db).Create(tx).Error; err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}
	return nil
}

func (r *transactionRepository) CreateBatch(ctx context.Context, txs []*model.Transaction) error {
	if len(txs) == 0 {
		return nil
	}
	if err := GetDB(ctx, r.db).Create(txs).Error; err != nil {
		return fmt.Errorf("failed to create transactions batch: %w", err)
	}
	return nil
}

func (r *transactionRepository) GetByID(ctx context.Context, id string) (*model.Transaction, error) {
	var tx model.Transaction
	if err := GetDB(ctx, r.db).Where("id = ?", id).First(&tx).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	return &tx, nil
}

func (r *transactionRepository) ListByGroupID(ctx context.Context, groupID uuid.UUID, limit, offset int) ([]*model.Transaction, error) {
	var txs []*model.Transaction
	query := GetDB(ctx, r.db).Where("group_id = ?", groupID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	if err := query.Find(&txs).Error; err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}
	return txs, nil
}

func (r *transactionRepository) ListByBatchID(ctx context.Context, batchID string) ([]*model.Transaction, error) {
	var txs []*model.Transaction
	if err := GetDB(ctx, r.db).Where("batch_id = ?", batchID).Order("created_at ASC").Find(&txs).Error; err != nil {
		return nil, fmt.Errorf("failed to list transactions by batch: %w", err)
	}
	return txs, nil
}

// ListCreditsByGroupID returns credit transactions for a group
func (r *transactionRepository) ListCreditsByGroupID(ctx context.Context, groupID uuid.UUID, currencyType string, limit, offset int) ([]*model.Transaction, error) {
	var txs []*model.Transaction
	query := GetDB(ctx, r.db).Where("group_id = ?", groupID)

	if currencyType != "" {
		query = query.Where("currency_type = ?", currencyType)
	} else {
		// Filter to credit transactions only.
		query = query.Where("currency_type IN ?", []string{
			string(model.CurrencyTypeSubscriptionCredits),
			string(model.CurrencyTypePurchasedCredits),
		})
	}

	query = query.Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}

	if err := query.Find(&txs).Error; err != nil {
		return nil, fmt.Errorf("failed to list credit transactions: %w", err)
	}
	return txs, nil
}

// ListCreditsByReferenceID returns credit transactions by reference ID (e.g., order_id)
func (r *transactionRepository) ListCreditsByReferenceID(ctx context.Context, referenceID string) ([]*model.Transaction, error) {
	var txs []*model.Transaction
	if err := GetDB(ctx, r.db).
		Where("reference_id = ?", referenceID).
		Where("currency_type IN ?", []string{
			string(model.CurrencyTypeSubscriptionCredits),
			string(model.CurrencyTypePurchasedCredits),
		}).
		Order("created_at DESC").
		Find(&txs).Error; err != nil {
		return nil, fmt.Errorf("failed to list credit transactions by reference: %w", err)
	}
	return txs, nil
}

// GetCreditsTotalByType calculates total amount for a specific transaction type
func (r *transactionRepository) GetCreditsTotalByType(ctx context.Context, groupID uuid.UUID, transactionType string) (float64, error) {
	var total float64
	if err := GetDB(ctx, r.db).Model(&model.Transaction{}).
		Where("group_id = ? AND transaction_type = ?", groupID, transactionType).
		Where("currency_type IN ?", []string{
			string(model.CurrencyTypeSubscriptionCredits),
			string(model.CurrencyTypePurchasedCredits),
		}).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("failed to calculate credits total: %w", err)
	}
	return total, nil
}
