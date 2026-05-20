package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zgiai/zgi/api/internal/modules/payment/model"
)

// GroupAICreditAccountRepository defines the interface for user AI credit account operations
type GroupAICreditAccountRepository interface {
	Create(ctx context.Context, account *model.GroupAICreditAccount) error
	GetByID(ctx context.Context, id string) (*model.GroupAICreditAccount, error)
	GetByAccountIDAndGroupID(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID) (*model.GroupAICreditAccount, error)
	GetByGroupID(ctx context.Context, groupID uuid.UUID) (*model.GroupAICreditAccount, error)
	GetOrCreate(ctx context.Context, accountID uuid.UUID) (*model.GroupAICreditAccount, error)
	GetOrCreateByGroupID(ctx context.Context, groupID uuid.UUID, accountID uuid.UUID) (*model.GroupAICreditAccount, error)
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	UpdateCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, updates map[string]interface{}) error
	DeductCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, amount int64) error
	DeductCreditsByGroupID(ctx context.Context, tx *gorm.DB, groupID uuid.UUID, amount int64) (*model.GroupAICreditAccount, error)
	ResetSubscriptionCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, newCredits int64) error
}

type groupAICreditAccountRepository struct {
	db *gorm.DB
}

// NewGroupAICreditAccountRepository creates a new user AI credit account repository
func NewGroupAICreditAccountRepository(db *gorm.DB) GroupAICreditAccountRepository {
	return &groupAICreditAccountRepository{db: db}
}

func (r *groupAICreditAccountRepository) Create(ctx context.Context, account *model.GroupAICreditAccount) error {
	if err := GetDB(ctx, r.db).Create(account).Error; err != nil {
		return fmt.Errorf("failed to create user AI credit account: %w", err)
	}
	return nil
}

func (r *groupAICreditAccountRepository) GetByID(ctx context.Context, id string) (*model.GroupAICreditAccount, error) {
	var account model.GroupAICreditAccount
	if err := GetDB(ctx, r.db).Where("id = ?", id).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("failed to get user AI credit account: %w", err)
	}
	return &account, nil
}

func (r *groupAICreditAccountRepository) GetByAccountID(ctx context.Context, accountID uuid.UUID) (*model.GroupAICreditAccount, error) {
	var account model.GroupAICreditAccount
	if err := GetDB(ctx, r.db).Where("account_id = ?", accountID).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("failed to get user AI credit account: %w", err)
	}
	return &account, nil
}

func (r *groupAICreditAccountRepository) GetByAccountIDAndGroupID(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID) (*model.GroupAICreditAccount, error) {
	var account model.GroupAICreditAccount
	if err := GetDB(ctx, r.db).Where("account_id = ? AND group_id = ?", accountID, groupID).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("failed to get user AI credit account: %w", err)
	}
	return &account, nil
}

func (r *groupAICreditAccountRepository) GetByGroupID(ctx context.Context, groupID uuid.UUID) (*model.GroupAICreditAccount, error) {
	var account model.GroupAICreditAccount
	if err := GetDB(ctx, r.db).Where("group_id = ?", groupID).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("failed to get user AI credit account by group ID: %w", err)
	}
	return &account, nil
}

func (r *groupAICreditAccountRepository) GetOrCreate(ctx context.Context, accountID uuid.UUID) (*model.GroupAICreditAccount, error) {
	account, err := r.GetByAccountID(ctx, accountID)
	if err == nil {
		return account, nil
	}

	// Create new account if not found
	newAccount := &model.GroupAICreditAccount{
		AccountID: accountID,
	}

	if err := r.Create(ctx, newAccount); err != nil {
		return nil, err
	}

	return newAccount, nil
}

func (r *groupAICreditAccountRepository) GetOrCreateByGroupID(ctx context.Context, groupID uuid.UUID, accountID uuid.UUID) (*model.GroupAICreditAccount, error) {
	account, err := r.GetByGroupID(ctx, groupID)
	if err == nil {
		return account, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// Create new account if not found
	newAccount := &model.GroupAICreditAccount{
		AccountID: accountID,
		GroupID:   groupID,
	}

	if err := r.Create(ctx, newAccount); err != nil {
		return nil, err
	}

	return newAccount, nil
}

func (r *groupAICreditAccountRepository) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()

	result := GetDB(ctx, r.db).Model(&model.GroupAICreditAccount{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update user AI credit account: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user AI credit account not found")
	}
	return nil
}

func (r *groupAICreditAccountRepository) UpdateCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()

	result := GetDB(ctx, r.db).Model(&model.GroupAICreditAccount{}).Where("account_id = ? AND group_id = ?", accountID, groupID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update credits: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user AI credit account not found")
	}
	return nil
}

func (r *groupAICreditAccountRepository) DeductCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, amount int64) error {
	// Use transaction to ensure atomicity
	return GetDB(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		var account model.GroupAICreditAccount

		// Lock the row for update
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("account_id = ? AND group_id = ?", accountID, groupID).
			First(&account).Error; err != nil {
			return fmt.Errorf("failed to lock account: %w", err)
		}

		if account.PurchasedCredits < amount {
			return fmt.Errorf("insufficient credits")
		}

		updates := map[string]interface{}{
			"updated_at":        time.Now(),
			"total_spent":       gorm.Expr("total_spent + ?", amount),
			"purchased_credits": gorm.Expr("purchased_credits - ?", amount),
		}

		if err := tx.Model(&model.GroupAICreditAccount{}).
			Where("account_id = ? AND group_id = ?", accountID, groupID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to deduct credits: %w", err)
		}

		return nil
	})
}

// DeductCreditsByGroupID deducts credits by group ID with transaction support
func (r *groupAICreditAccountRepository) DeductCreditsByGroupID(ctx context.Context, tx *gorm.DB, groupID uuid.UUID, amount int64) (*model.GroupAICreditAccount, error) {
	var account model.GroupAICreditAccount

	// Lock the row for update
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("group_id = ?", groupID).
		First(&account).Error; err != nil {
		return nil, fmt.Errorf("failed to lock account: %w", err)
	}

	if account.PurchasedCredits < amount {
		return nil, fmt.Errorf("insufficient credits: have %d, need %d", account.GetTotalCredits(), amount)
	}

	account.PurchasedCredits -= amount
	account.TotalSpent += amount
	account.UpdatedAt = time.Now()

	// Save the updated account
	if err := tx.WithContext(ctx).Save(&account).Error; err != nil {
		return nil, fmt.Errorf("failed to deduct credits: %w", err)
	}

	// Return account with before/after balances for transaction recording
	return &account, nil
}

func (r *groupAICreditAccountRepository) ResetSubscriptionCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, newCredits int64) error {
	return nil
}
