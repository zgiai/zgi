package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	paymentModel "github.com/zgiai/zgi/api/internal/modules/payment/model"
	paymentRepo "github.com/zgiai/zgi/api/internal/modules/payment/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BillingProvider defines the interface for billing operations.
// In CLOUD mode, this is implemented by RemoteBilling (gRPC to console-api).
// In SELF_HOSTED mode, this is implemented by BillingService (local DB).
type BillingProvider interface {
	PreDeduct(ctx context.Context, bc *BillingContext) error
	Settle(ctx context.Context, bc *BillingContext) error
	CalculateCreditsFromTokens(promptTokens, completionTokens int, modelID uuid.UUID) (inputCredits, outputCredits, totalCredits int64, err error)
	CalculateImageCredits(req *adapter.ImageRequest, modelID uuid.UUID) (int64, error)
	CheckBalance(ctx context.Context, groupID uuid.UUID, ownerID uuid.UUID, estimatedCredits int64) (bool, error)
}

// BillingService handles billing and quota management (local DB implementation)
type BillingService struct {
	db                *gorm.DB
	apiKeyRepo        apikeyrepo.APIKeyRepository
	creditAccountRepo paymentRepo.GroupAICreditAccountRepository
	creditTxRepo      paymentRepo.TransactionRepository
	localRecoverOnce  sync.Once
}

// BillingContext contains billing-related information for a request
type BillingContext struct {
	APIKeyID          string
	OrganizationID    string
	DeductionID       string
	AttemptID         string
	QuotaSubjectType  string
	QuotaSubjectID    string
	AccountID         *uuid.UUID // Internal user ID when using models
	GroupID           *uuid.UUID // Organization ID (formerly Shadow Tenant ID)
	WorkspaceID       string
	AppID             *uuid.UUID // App ID (agent or dataset)
	AppType           *string    // App type: 'agent' or 'dataset'
	SessionID         string
	ConversationID    string
	WorkflowID        string
	WorkflowRunID     string
	NodeID            string
	NodeType          string
	ModelID           uuid.UUID
	ModelSource       PricingModelSource
	ModelName         string // Model name for logging
	ProviderID        uuid.UUID
	ProviderName      string     // Provider name for logging
	RouteID           *uuid.UUID // Selected route ID when the request is routed
	ChannelID         *uuid.UUID // Tenant channel ID, nil if using system provider
	AccountProviderID *uint      // Deprecated, kept for compatibility
	EstimatedCredits  int64      // Estimated credits to deduct
	ActualCredits     int64      // Actual credits used
	PromptTokens      int
	CompletionTokens  int
	TotalTokens       int
	UsageSource       string
	InputCost         decimal.Decimal // Legacy: input credits consumed for logging/RPC compatibility
	OutputCost        decimal.Decimal // Legacy: output credits consumed for logging/RPC compatibility
	TotalCost         decimal.Decimal // Legacy: total credits consumed for logging/RPC compatibility
	InputUSD          decimal.Decimal
	OutputUSD         decimal.Decimal
	TotalUSD          decimal.Decimal
	BillingLane       UsageBillingLane
	UseSystemProvider bool
	IsStreaming       bool
	RequestID         string
	RequestCreatedAt  time.Time
	SettledAt         time.Time
	ResponseTime      int64 // milliseconds
	Status            string
	ErrorMessage      string
	IPAddress         string
	UserAgent         string
}

// NewBillingService creates a new billing service
func NewBillingService(
	db *gorm.DB,
	apiKeyRepo apikeyrepo.APIKeyRepository,
	creditAccountRepo paymentRepo.GroupAICreditAccountRepository,
	creditTxRepo paymentRepo.TransactionRepository,
) *BillingService {
	return &BillingService{
		db:                db,
		apiKeyRepo:        apiKeyRepo,
		creditAccountRepo: creditAccountRepo,
		creditTxRepo:      creditTxRepo,
	}
}

// PreDeduct pre-deducts estimated quota from API key
func (b *BillingService) PreDeduct(ctx context.Context, bc *BillingContext) error {
	usageLane, laneErr := normalizeBillingContextUsageLane(bc)
	if laneErr != nil {
		return laneErr
	}
	useSystemProvider := usageBillingLaneUsesSystemProvider(usageLane)

	err := b.db.Transaction(func(tx *gorm.DB) error {
		if err := b.upsertAttemptInit(ctx, tx, bc); err != nil {
			return fmt.Errorf("failed to init billing attempt: %w", err)
		}

		// 1. Lock and get API key
		var apiKey apikeymodel.TenantAPIKey
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND organization_id = ?", bc.APIKeyID, bc.OrganizationID).
			First(&apiKey).Error; err != nil {
			return err
		}

		// 2. Check API key status - SECURITY: reject inactive keys
		if apiKey.Status != "active" {
			return ErrAPIKeyInactive
		}

		// 3. Pre-deduct subject quota (key/workspace)
		if err := b.preDeductSubjectQuota(ctx, tx, bc, &apiKey); err != nil {
			return fmt.Errorf("failed to pre-deduct subject quota: %w", err)
		}

		if !useSystemProvider {
			if err := b.preDeductPrivateChannelWallet(ctx, tx, bc); err != nil {
				return fmt.Errorf("failed to pre-deduct private channel wallet: %w", err)
			}
		}

		if err := b.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusPre, nil, nil, nil); err != nil {
			return fmt.Errorf("failed to update billing attempt status: %w", err)
		}

		return nil
	})
	if err != nil {
		markErr := b.db.Transaction(func(tx *gorm.DB) error {
			if err := b.upsertAttemptInit(ctx, tx, bc); err != nil {
				return err
			}
			invocation := "error"
			code := "PREDEDUCT_FAILED"
			msg := err.Error()
			return b.updateAttemptStatus(
				ctx,
				tx,
				bc,
				billingAttemptStatusPredeductFailed,
				&invocation,
				&code,
				&msg,
			)
		})
		if markErr != nil {
			return fmt.Errorf("pre-deduct failed: %v (additionally failed to mark attempt: %w)", err, markErr)
		}
		return err
	}
	return nil
}

// CheckBalance checks if the group has sufficient AI credits for the estimated usage.
// Returns true if balance is sufficient, false otherwise.
func (b *BillingService) CheckBalance(ctx context.Context, groupID uuid.UUID, ownerID uuid.UUID, estimatedCredits int64) (bool, error) {
	creditAccount, err := b.creditAccountRepo.GetOrCreateByGroupID(ctx, groupID, ownerID)
	if err != nil {
		return false, fmt.Errorf("failed to get or create credit account: %w", err)
	}
	return creditAccount.CanSpend(estimatedCredits), nil
}

// Settle settles the actual billing after request completion
func (b *BillingService) Settle(ctx context.Context, bc *BillingContext) error {
	usageLane, laneErr := normalizeBillingContextUsageLane(bc)
	if laneErr != nil {
		return laneErr
	}
	useSystemProvider := usageBillingLaneUsesSystemProvider(usageLane)

	alreadyFinalized := false
	// Critical billing operations in transaction
	err := b.db.Transaction(func(tx *gorm.DB) error {
		attemptID, err := b.ensureAttemptID(bc)
		if err != nil {
			return err
		}
		bc.AttemptID = attemptID

		var attempt BillingAttempt
		err = tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("attempt_id = ?", bc.AttemptID).
			First(&attempt).Error
		if err == nil {
			if attempt.Status == billingAttemptStatusSettled || attempt.Status == billingAttemptStatusRolledBack {
				alreadyFinalized = true
				return nil
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load billing attempt for settle: %w", err)
		}

		if err := b.upsertAttemptInit(ctx, tx, bc); err != nil {
			return fmt.Errorf("failed to init billing attempt: %w", err)
		}
		if bc.RequestCreatedAt.IsZero() {
			bc.RequestCreatedAt = time.Now().UTC()
		}
		bc.SettledAt = time.Now().UTC()

		// 1. Settle subject quota update (key/workspace)
		if err := b.settleSubjectQuota(ctx, tx, bc); err != nil {
			return fmt.Errorf("failed to settle subject quota: %w", err)
		}

		fundLedgerType := ""
		fundLedgerRefID := ""

		// 2. Deduct credits only for system provider (payment-on-behalf mode)
		// Tenant channel mode uses tenant's own API key, no credit deduction needed
		if useSystemProvider {
			// System provider: deduct tenant AI credits (payment-on-behalf mode)
			if err := b.deductTenantCredits(ctx, tx, bc); err != nil {
				return fmt.Errorf("failed to deduct tenant credits: %w", err)
			}
			fundLedgerType = billingLedgerTypeOrgFunds
			fundLedgerRefID = bc.OrganizationID
		} else {
			ledgerType, ledgerRefID, err := b.settlePrivateChannelWallet(ctx, tx, bc)
			if err != nil {
				return fmt.Errorf("failed to settle private channel wallet: %w", err)
			}
			fundLedgerType = ledgerType
			fundLedgerRefID = ledgerRefID
		}

		if err := b.updateAttemptEntriesAfterSettle(ctx, tx, bc, fundLedgerType, fundLedgerRefID); err != nil {
			return fmt.Errorf("failed to update attempt entries: %w", err)
		}

		invocation := "success"
		attemptStatus := billingAttemptStatusSettled
		if !billingContextStatusIsSuccess(bc.Status) {
			invocation = "error"
			attemptStatus = billingAttemptStatusRolledBack
		}
		if err := b.updateAttemptStatus(ctx, tx, bc, attemptStatus, &invocation, nil, nil); err != nil {
			return fmt.Errorf("failed to update billing attempt status: %w", err)
		}
		if err := b.upsertUsageBill(ctx, tx, bc, usageBillStatusFromBillingContext(bc.Status), nil, nil); err != nil {
			return fmt.Errorf("failed to upsert usage bill: %w", err)
		}

		return nil
	})

	if err != nil {
		markErr := b.db.Transaction(func(tx *gorm.DB) error {
			if err := b.upsertAttemptInit(ctx, tx, bc); err != nil {
				return err
			}
			if bc.RequestCreatedAt.IsZero() {
				bc.RequestCreatedAt = time.Now().UTC()
			}
			bc.SettledAt = time.Now().UTC()
			invocation := invocationResultFromBillingStatus(bc.Status)
			code := "SETTLE_FAILED"
			msg := err.Error()
			if err := b.updateAttemptStatus(
				ctx,
				tx,
				bc,
				billingAttemptStatusPartial,
				&invocation,
				&code,
				&msg,
			); err != nil {
				return err
			}
			return b.upsertUsageBill(ctx, tx, bc, usageBillStatusPartial, &code, &msg)
		})
		if markErr != nil {
			return fmt.Errorf("settle failed: %v (additionally failed to mark attempt: %w)", err, markErr)
		}
		return err
	}
	if alreadyFinalized {
		return nil
	}

	return nil
}

// settleAPIKeyQuota settles the API key quota
func (b *BillingService) settleAPIKeyQuota(ctx context.Context, tx *gorm.DB, bc *BillingContext) error {
	var apiKey apikeymodel.TenantAPIKey
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND organization_id = ?", bc.APIKeyID, bc.OrganizationID).
		First(&apiKey).Error; err != nil {
		return err
	}

	// If quota_limit is nil (unlimited), only update used_quota for tracking
	if apiKey.QuotaLimit == nil {
		apiKey.UsedQuota += bc.ActualCredits
		return tx.Save(&apiKey).Error
	}

	// Calculate difference (pre-deducted - actual) using credit count
	diff := bc.EstimatedCredits - bc.ActualCredits

	// Subject quota must never go below zero.
	apiKey.RemainQuota = clampQuotaRemainAtZero(apiKey.RemainQuota + diff)
	apiKey.UsedQuota += bc.ActualCredits

	return tx.Save(&apiKey).Error
}

func (b *BillingService) preDeductSubjectQuota(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
	apiKey *apikeymodel.TenantAPIKey,
) error {
	subjectType := strings.TrimSpace(bc.QuotaSubjectType)
	if subjectType == "" {
		return fmt.Errorf("missing quota_subject_type for subject pre-deduct (attempt_id=%s request_id=%s)", strings.TrimSpace(bc.AttemptID), strings.TrimSpace(bc.RequestID))
	}
	if subjectType == quotaSubjectTypeAPIKey {
		apiKeyID := strings.TrimSpace(bc.APIKeyID)
		subjectID := strings.TrimSpace(bc.QuotaSubjectID)
		if apiKeyID == "" {
			return fmt.Errorf("missing api_key_id for key subject pre-deduct (attempt_id=%s request_id=%s)", strings.TrimSpace(bc.AttemptID), strings.TrimSpace(bc.RequestID))
		}
		if subjectID == "" {
			return fmt.Errorf("missing quota_subject_id for key subject pre-deduct (attempt_id=%s request_id=%s)", strings.TrimSpace(bc.AttemptID), strings.TrimSpace(bc.RequestID))
		}
		if subjectID != apiKeyID {
			return fmt.Errorf(
				"quota subject mismatch for key subject pre-deduct: quota_subject_id=%s api_key_id=%s attempt_id=%s request_id=%s",
				subjectID,
				apiKeyID,
				strings.TrimSpace(bc.AttemptID),
				strings.TrimSpace(bc.RequestID),
			)
		}
		if apiKey == nil {
			return fmt.Errorf("api key subject requires loaded api key")
		}
		return b.preDeductAPIKeyQuota(ctx, tx, bc, apiKey)
	}
	if subjectType == quotaSubjectTypeWorkspace {
		return b.preDeductWorkspaceQuota(ctx, tx, bc)
	}
	if subjectType == quotaSubjectTypeOrganization {
		return nil
	}
	return fmt.Errorf("unsupported quota subject type: %s", subjectType)
}

func (b *BillingService) preDeductAPIKeyQuota(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
	apiKey *apikeymodel.TenantAPIKey,
) error {
	// If quota_limit is nil, it means unlimited quota.
	if apiKey.QuotaLimit == nil {
		return nil
	}

	estimatedQuota := bc.EstimatedCredits
	if apiKey.RemainQuota < estimatedQuota {
		return ErrInsufficientQuota
	}

	apiKey.RemainQuota -= estimatedQuota
	return tx.WithContext(ctx).Save(apiKey).Error
}

func (b *BillingService) preDeductWorkspaceQuota(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
) error {
	workspaceID := strings.TrimSpace(bc.QuotaSubjectID)
	if workspaceID == "" {
		return ErrInvalidRequest
	}
	orgID, err := uuid.Parse(bc.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid organization id in billing context: %w", err)
	}

	workspaceQuota, err := b.getOrCreateWorkspaceQuotaForUpdate(ctx, tx, workspaceID, orgID)
	if err != nil {
		return err
	}
	if workspaceQuota.QuotaLimit == nil {
		return nil
	}
	if workspaceQuota.RemainQuota < bc.EstimatedCredits {
		return ErrInsufficientQuota
	}

	workspaceQuota.RemainQuota -= bc.EstimatedCredits
	workspaceQuota.UpdatedAt = time.Now()
	return tx.WithContext(ctx).Save(workspaceQuota).Error
}

func (b *BillingService) settleSubjectQuota(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
) error {
	subjectType := strings.TrimSpace(bc.QuotaSubjectType)
	if subjectType == "" {
		return fmt.Errorf("missing quota_subject_type for subject settle (attempt_id=%s request_id=%s)", strings.TrimSpace(bc.AttemptID), strings.TrimSpace(bc.RequestID))
	}
	if subjectType == quotaSubjectTypeAPIKey {
		apiKeyID := strings.TrimSpace(bc.APIKeyID)
		subjectID := strings.TrimSpace(bc.QuotaSubjectID)
		if apiKeyID == "" {
			return fmt.Errorf("missing api_key_id for key subject settle (attempt_id=%s request_id=%s)", strings.TrimSpace(bc.AttemptID), strings.TrimSpace(bc.RequestID))
		}
		if subjectID == "" {
			return fmt.Errorf("missing quota_subject_id for key subject settle (attempt_id=%s request_id=%s)", strings.TrimSpace(bc.AttemptID), strings.TrimSpace(bc.RequestID))
		}
		if subjectID != apiKeyID {
			return fmt.Errorf(
				"quota subject mismatch for key subject settle: quota_subject_id=%s api_key_id=%s attempt_id=%s request_id=%s",
				subjectID,
				apiKeyID,
				strings.TrimSpace(bc.AttemptID),
				strings.TrimSpace(bc.RequestID),
			)
		}
		return b.settleAPIKeyQuota(ctx, tx, bc)
	}
	if subjectType == quotaSubjectTypeWorkspace {
		return b.settleWorkspaceQuota(ctx, tx, bc)
	}
	if subjectType == quotaSubjectTypeOrganization {
		return nil
	}
	return fmt.Errorf("unsupported quota subject type: %s", subjectType)
}

func (b *BillingService) settleWorkspaceQuota(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
) error {
	workspaceID := strings.TrimSpace(bc.QuotaSubjectID)
	if workspaceID == "" {
		return ErrInvalidRequest
	}
	orgID, err := uuid.Parse(bc.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid organization id in billing context: %w", err)
	}

	workspaceQuota, err := b.getOrCreateWorkspaceQuotaForUpdate(ctx, tx, workspaceID, orgID)
	if err != nil {
		return err
	}

	if workspaceQuota.QuotaLimit == nil {
		workspaceQuota.UsedQuota += bc.ActualCredits
		workspaceQuota.UpdatedAt = time.Now()
		return tx.WithContext(ctx).Save(workspaceQuota).Error
	}

	diff := bc.EstimatedCredits - bc.ActualCredits
	// Subject quota must never go below zero.
	workspaceQuota.RemainQuota = clampQuotaRemainAtZero(workspaceQuota.RemainQuota + diff)
	workspaceQuota.UsedQuota += bc.ActualCredits
	workspaceQuota.UpdatedAt = time.Now()
	return tx.WithContext(ctx).Save(workspaceQuota).Error
}

func clampQuotaRemainAtZero(remain int64) int64 {
	if remain < 0 {
		return 0
	}
	return remain
}

func (b *BillingService) getOrCreateWorkspaceQuotaForUpdate(
	ctx context.Context,
	tx *gorm.DB,
	workspaceID string,
	organizationID uuid.UUID,
) (*WorkspaceQuota, error) {
	var quota WorkspaceQuota
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ?", workspaceID).
		First(&quota).Error
	if err == nil {
		if quota.OrganizationID != organizationID {
			return nil, fmt.Errorf(
				"workspace quota organization mismatch: workspace_id=%s quota_org=%s request_org=%s",
				workspaceID,
				quota.OrganizationID.String(),
				organizationID.String(),
			)
		}
		return &quota, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	quota = WorkspaceQuota{
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		UsedQuota:      0,
		RemainQuota:    0,
		QuotaLimit:     nil, // default unlimited unless explicitly configured
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if createErr := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "workspace_id"}},
			DoNothing: true,
		}).
		Create(&quota).Error; createErr != nil {
		return nil, fmt.Errorf("create workspace quota: %w", createErr)
	}

	quota = WorkspaceQuota{}
	err = tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ?", workspaceID).
		First(&quota).Error
	if err != nil {
		return nil, fmt.Errorf("load workspace quota after create: %w", err)
	}
	if quota.OrganizationID != organizationID {
		return nil, fmt.Errorf(
			"workspace quota organization mismatch: workspace_id=%s quota_org=%s request_org=%s",
			workspaceID,
			quota.OrganizationID.String(),
			organizationID.String(),
		)
	}
	return &quota, nil
}

// deductTenantCredits deducts AI credits from tenant (payment-on-behalf mode)
func (b *BillingService) deductTenantCredits(ctx context.Context, tx *gorm.DB, bc *BillingContext) error {
	// Parse tenant ID to UUID
	tenantUUID, err := uuid.Parse(bc.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}

	creditsToDeduct := bc.ActualCredits
	if creditsToDeduct == 0 && (bc.PromptTokens > 0 || bc.CompletionTokens > 0) {
		quote, quoteErr := NewPricingEngine(b.db).QuoteTokens(ctx, pricingModelRefFromBillingContext(bc), bc.PromptTokens, bc.CompletionTokens)
		err = quoteErr
		if err != nil {
			return fmt.Errorf("failed to calculate credits: %w", err)
		}
		creditsToDeduct = quote.TotalCredits
	}
	if creditsToDeduct <= 0 {
		return nil
	}

	// Get account before deduction to record balances
	var accountBefore paymentModel.GroupAICreditAccount
	if err := tx.WithContext(ctx).Where("group_id = ?", tenantUUID).First(&accountBefore).Error; err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Deduct credits from user AI credit account (by group_id which is tenant_id)
	account, err := b.creditAccountRepo.DeductCreditsByGroupID(ctx, tx, tenantUUID, creditsToDeduct)
	if err != nil {
		return fmt.Errorf("failed to deduct credits: %w", err)
	}

	// Create transaction detail (shared across all records in the batch)
	transactionDetail := map[string]interface{}{
		"model_name":        bc.ModelName,
		"provider_name":     bc.ProviderName,
		"prompt_tokens":     bc.PromptTokens,
		"completion_tokens": bc.CompletionTokens,
		"total_tokens":      bc.TotalTokens,
		"request_id":        bc.RequestID,
		"api_key_id":        bc.APIKeyID,
	}

	// Generate shared batch_id for related transactions
	batchID := "BAT-" + uuid.New().String()[:8]
	now := time.Now()
	transactions := []*paymentModel.Transaction{
		{
			BatchID:           batchID,
			GroupID:           account.GroupID,
			CurrencyType:      string(paymentModel.CurrencyTypePurchasedCredits),
			TransactionType:   string(paymentModel.TransactionTypeAIConsumption),
			Amount:            float64(-creditsToDeduct),
			BalanceBefore:     float64(accountBefore.PurchasedCredits),
			BalanceAfter:      float64(account.PurchasedCredits),
			TransactionDetail: transactionDetail,
			CreatedAt:         now,
		},
	}

	return b.creditTxRepo.CreateBatch(ctx, transactions)
}

// CalculateCost calculates the cost based on token usage and model pricing.
func (b *BillingService) CalculateCost(
	promptTokens, completionTokens int,
	llmModel *llmmodel.LLMModel,
) (inputCost, outputCost, totalCost decimal.Decimal) {
	// Calculate input cost (prompt tokens)
	// InputPrice is cost per million tokens
	inputCost = llmModel.InputPrice.
		Mul(decimal.NewFromInt(int64(promptTokens))).
		Div(decimal.NewFromInt(1000000))

	// Calculate output cost (completion tokens)
	// OutputPrice is cost per million tokens
	outputCost = llmModel.OutputPrice.
		Mul(decimal.NewFromInt(int64(completionTokens))).
		Div(decimal.NewFromInt(1000000))

	// Total cost
	totalCost = inputCost.Add(outputCost)

	return
}

// CalculateTotalTokens calculates the total token count
func (b *BillingService) CalculateTotalTokens(
	promptTokens, completionTokens int,
) int {
	return promptTokens + completionTokens
}

// CalculateCreditsFromTokens calculates credits to deduct based on tokens and model's cost_rate
// Returns: inputCredits, outputCredits, totalCredits, error
func (b *BillingService) CalculateCreditsFromTokens(
	promptTokens,
	completionTokens int,
	modelID uuid.UUID,
) (inputCredits, outputCredits, totalCredits int64, err error) {
	quote, err := NewPricingEngine(b.db).QuoteTokens(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, promptTokens, completionTokens)
	if err != nil {
		return 0, 0, 0, err
	}
	return quote.InputCredits, quote.OutputCredits, quote.TotalCredits, nil
}

// CalculateImageCredits is a compatibility wrapper around PricingEngine.
func (b *BillingService) CalculateImageCredits(req *adapter.ImageRequest, modelID uuid.UUID) (int64, error) {
	quote, err := NewPricingEngine(b.db).QuoteImage(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, req)
	if err != nil {
		return 0, err
	}
	return quote.TotalCredits, nil
}
