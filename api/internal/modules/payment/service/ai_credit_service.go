package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/zgiai/ginext/internal/modules/payment/model"
	"github.com/zgiai/ginext/internal/modules/payment/repository"
)

// AICreditService handles AI credit business logic
type AICreditService struct {
	db                     *gorm.DB
	accountRepo            repository.GroupAICreditAccountRepository
	transactionRepo        repository.TransactionRepository // Unified transaction repository
	officialCreditChecker  OfficialCreditChecker
	privateChannelFundRepo repository.PrivateChannelFundRepository
}

// NewAICreditService creates a new AI credit service
func NewAICreditService(
	db *gorm.DB,
	accountRepo repository.GroupAICreditAccountRepository,
	transactionRepo repository.TransactionRepository, // Unified transaction repository
	officialCreditChecker OfficialCreditChecker,
	privateChannelFundRepo repository.PrivateChannelFundRepository,
) *AICreditService {
	if officialCreditChecker == nil {
		officialCreditChecker = NewConsoleOfficialCreditChecker()
	}
	if privateChannelFundRepo == nil {
		privateChannelFundRepo = repository.NewPrivateChannelFundRepository(db)
	}

	return &AICreditService{
		db:                     db,
		accountRepo:            accountRepo,
		transactionRepo:        transactionRepo,
		officialCreditChecker:  officialCreditChecker,
		privateChannelFundRepo: privateChannelFundRepo,
	}
}

func stringPtr(value string) *string {
	return &value
}

// GetOrCreateUserAccount gets or creates a user's AI credit account
func (s *AICreditService) GetOrCreateUserAccount(ctx context.Context, accountID uuid.UUID) (*model.GroupAICreditAccount, error) {
	return s.accountRepo.GetOrCreate(ctx, accountID)
}

// GetUserAccount retrieves a user's AI credit account for a specific group.
// Missing accounts are lazily created with zero purchased credits.
func (s *AICreditService) GetUserAccount(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID) (*model.GroupAICreditAccount, error) {
	account, err := s.accountRepo.GetByAccountIDAndGroupID(ctx, accountID, groupID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return s.getOrCreateAICreditAccount(ctx, accountID, groupID)
		}
		return nil, err
	}
	return account, nil
}

// GetMyAccountOverview returns the account payload for /ai-credits/me.
// Legacy credit fields are preserved in shape but zeroed, while official/private
// balances are returned in dedicated fields.
func (s *AICreditService) GetMyAccountOverview(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID) (*AICreditAccountOverview, error) {
	officialBalance, err := s.officialCreditChecker.GetOfficialBalance(ctx, groupID)
	if err != nil {
		return nil, err
	}

	privateItems, err := s.privateChannelFundRepo.ListByOrganizationID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to load private channel funds: %w", err)
	}

	account, err := s.GetUserAccount(ctx, accountID, groupID)
	if err != nil {
		return nil, err
	}

	privateDetails := make([]PrivateChannelFundDetail, 0, len(privateItems))
	privateTotal := int64(0)
	for _, item := range privateItems {
		privateTotal += item.Balance
		privateDetails = append(privateDetails, PrivateChannelFundDetail{
			ChannelID:   item.ChannelID,
			ChannelName: item.ChannelName,
			Balance:     item.Balance,
			Status:      item.Status,
			Currency:    item.Currency,
			UpdatedAt:   item.UpdatedAt,
		})
	}

	return &AICreditAccountOverview{
		ID:                  account.ID,
		AccountID:           account.AccountID,
		GroupID:             account.GroupID,
		SubscriptionCredits: 0,
		PurchasedCredits:    account.PurchasedCredits,
		TotalEarned:         account.TotalEarned,
		TotalSpent:          account.TotalSpent,
		LastResetAt:         nil,
		NextResetAt:         nil,
		CreatedAt:           account.CreatedAt,
		UpdatedAt:           account.UpdatedAt,
		OfficialAICredits: OfficialAICreditBalance{
			Balance: officialBalance,
			Source:  "console",
		},
		PrivateChannelFunds: PrivateChannelFundSummary{
			Total:    privateTotal,
			Channels: privateDetails,
		},
	}, nil
}

// ConsumeCredits consumes credits from a user's account
func (s *AICreditService) ConsumeCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, amount int64, relatedID *string, description string, usageDetail map[string]interface{}) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	return repository.Transaction(ctx, s.db, func(txCtx context.Context) error {
		// Get account
		account, err := s.accountRepo.GetByAccountIDAndGroupID(txCtx, accountID, groupID)
		if err != nil {
			return fmt.Errorf("account not found: %w", err)
		}

		if account.PurchasedCredits < amount {
			return fmt.Errorf("insufficient credits")
		}

		purchasedBalanceBefore := account.PurchasedCredits
		balanceBefore := purchasedBalanceBefore

		if err := s.accountRepo.DeductCredits(txCtx, accountID, groupID, amount); err != nil {
			return err
		}

		accountAfter, err := s.accountRepo.GetByAccountIDAndGroupID(txCtx, accountID, groupID)
		if err != nil {
			return fmt.Errorf("failed to get account after deduction: %w", err)
		}

		purchasedBalanceAfter := accountAfter.PurchasedCredits
		balanceAfter := purchasedBalanceAfter

		chineseDesc := "AI模型调用"
		if description != "" {
			chineseDesc = fmt.Sprintf("AI模型调用: %s", description)
		}

		// Create unified transaction record
		transaction := &model.Transaction{
			GroupID:         groupID,
			CurrencyType:    string(model.CurrencyTypePurchasedCredits),
			TransactionType: string(model.TransactionTypeAIConsumption),
			Amount:          float64(-amount), // Negative for spending
			BalanceBefore:   float64(balanceBefore),
			BalanceAfter:    float64(balanceAfter),
			ReferenceType:   stringPtr(model.ReferenceTypeConversation),
			ReferenceID:     relatedID,
			Description:     stringPtr(chineseDesc),
			TransactionDetail: map[string]interface{}{
				"sub_type":                 "ai_usage",
				"purchased_balance_before": purchasedBalanceBefore,
				"purchased_balance_after":  purchasedBalanceAfter,
				"usage_detail":             usageDetail,
			},
		}

		return s.transactionRepo.Create(txCtx, transaction)
	})
}

// RefundCredits refunds purchased credits to a user's account.
func (s *AICreditService) RefundCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, isPurchased bool, amount int64, orderID *string, detail map[string]interface{}) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if !isPurchased {
		return fmt.Errorf("subscription credits have been removed")
	}

	return repository.Transaction(ctx, s.db, func(txCtx context.Context) error {
		account, err := s.accountRepo.GetByAccountIDAndGroupID(txCtx, accountID, groupID)
		if err != nil {
			return fmt.Errorf("account not found: %w", err)
		}

		purchasedBalanceBefore := account.PurchasedCredits
		balanceBefore := purchasedBalanceBefore

		updates := map[string]interface{}{
			"total_spent":       gorm.Expr("total_spent - ?", amount),
			"purchased_credits": gorm.Expr("purchased_credits + ?", amount),
		}

		if err := s.accountRepo.UpdateCredits(txCtx, accountID, groupID, updates); err != nil {
			return err
		}

		// Get account after update
		accountAfter, err := s.accountRepo.GetByAccountIDAndGroupID(txCtx, accountID, groupID)
		if err != nil {
			return fmt.Errorf("failed to get account after update: %w", err)
		}

		purchasedBalanceAfter := accountAfter.PurchasedCredits
		balanceAfter := purchasedBalanceAfter

		// Create unified transaction record
		transaction := &model.Transaction{
			GroupID:         groupID,
			CurrencyType:    string(model.CurrencyTypePurchasedCredits),
			TransactionType: string(model.TransactionTypeOther),
			Amount:          float64(amount),
			BalanceBefore:   float64(balanceBefore),
			BalanceAfter:    float64(balanceAfter),
			ReferenceType:   stringPtr(model.ReferenceTypeOrder),
			ReferenceID:     orderID,
			Description:     stringPtr("点数退款"),
			TransactionDetail: map[string]interface{}{
				"sub_type":                 "refund",
				"purchased_balance_before": purchasedBalanceBefore,
				"purchased_balance_after":  purchasedBalanceAfter,
				"detail":                   detail,
			},
		}

		return s.transactionRepo.Create(txCtx, transaction)
	})
}

// ResetSubscriptionCredits resets subscription credits for a user
func (s *AICreditService) ResetSubscriptionCredits(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID, newCredits int64) error {
	return fmt.Errorf("subscription credits have been removed")
}

// GetTransactionHistory retrieves credit transaction history for a group
// Note: This returns unified Transaction records filtered by credits currency types
func (s *AICreditService) GetTransactionHistory(ctx context.Context, groupID uuid.UUID, currencyType string, limit, offset int) ([]*model.Transaction, error) {
	return s.transactionRepo.ListCreditsByGroupID(ctx, groupID, currencyType, limit, offset)
}

// GetTransactionByID retrieves a transaction by ID
func (s *AICreditService) GetTransactionByID(ctx context.Context, id string) (*model.Transaction, error) {
	return s.transactionRepo.GetByID(ctx, id)
}

// GetAccountBalance retrieves the current balance of a user's account for a specific group
func (s *AICreditService) GetAccountBalance(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID) (int64, int64, int64, error) {
	account, err := s.GetUserAccount(ctx, accountID, groupID)
	if err != nil {
		return 0, 0, 0, err
	}

	return account.GetTotalCredits(), 0, account.GetPurchasedBalance(), nil
}

// getOrCreateAICreditAccount gets or creates a zero-balance AI credit account.
func (s *AICreditService) getOrCreateAICreditAccount(ctx context.Context, accountID uuid.UUID, groupID uuid.UUID) (*model.GroupAICreditAccount, error) {
	account, err := s.accountRepo.GetByAccountIDAndGroupID(ctx, accountID, groupID)
	if err == nil {
		return account, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	newAccount := &model.GroupAICreditAccount{
		AccountID:        accountID,
		GroupID:          groupID,
		PurchasedCredits: 0,
		TotalEarned:      0,
		TotalSpent:       0,
	}

	if err := s.accountRepo.Create(ctx, newAccount); err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return newAccount, nil
}
