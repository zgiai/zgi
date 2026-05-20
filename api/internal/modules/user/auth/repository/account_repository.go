package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	"gorm.io/gorm"
)

type AccountRepository interface {
	GetAccount(ctx context.Context, id string) (*model.Account, error)
	GetAccountByEmail(ctx context.Context, email string) (*model.Account, error)
	GetAccountWithExtensionsByEmail(ctx context.Context, email string) (*model.Account, error)
	CreateAccount(ctx context.Context, account *model.Account) error
	UpdateAccount(ctx context.Context, account *model.Account) error
	DeleteAccount(ctx context.Context, id string) error

	GetGroupRole(ctx context.Context, accountID string, groupID string) (string, error)
	GetGroupRoleByTenantID(ctx context.Context, accountID string, tenantID string) (string, error)
	UpsertOrganizationRole(ctx context.Context, tenantID string, accountID string, role string) error
	GetAccountRoleInEnterpriseGroup(ctx context.Context, accountID string, groupID string) (string, error)
	GetAccountIntegrate(ctx context.Context, accountID string, provider model.AccountIntegrateProvider) (*model.AccountIntegrate, error)
	GetAccountIntegrateByProviderOpenID(ctx context.Context, provider model.AccountIntegrateProvider, openID string) (*model.AccountIntegrate, error)
	GetAccountByNormalizedMobile(ctx context.Context, mobile string) (*model.Account, error)
	CreateAccountIntegrate(ctx context.Context, integrate *model.AccountIntegrate) error
	UpdateAccountIntegrate(ctx context.Context, integrate *model.AccountIntegrate) error
	DeleteAccountIntegrate(ctx context.Context, accountID string, provider model.AccountIntegrateProvider) error

	GetByEmailAndPassword(ctx context.Context, email, password string) (*model.Account, error)
	GetByEmailAndStatus(ctx context.Context, email string, status model.AccountStatus) (*model.Account, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	GetActiveAccounts(ctx context.Context, limit, offset int) ([]*model.Account, error)
	CountByStatus(ctx context.Context, status model.AccountStatus) (int64, error)
	GetAccountsWithExtensions(ctx context.Context, query *gorm.DB, page, limit int) ([]*model.Account, int64, error)

	GetAccountById(ctx context.Context, id string) (*model.Account, error)
	GetAccountsByIds(ctx context.Context, ids []string) ([]*model.Account, error)

	GetDB() *gorm.DB

	GetEnterpriseGroupsByTenantID(ctx context.Context, tenantID string) ([]string, error)

	ExecuteInTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error
	WithTx(tx *gorm.DB) AccountRepository
	SelectAccountAndTenantAccountJoin(ctx context.Context, invitationData map[string]string, tenant workspace_model.Workspace) (*model.AccountAndJoin, error)

	GetAccountContextByAccountID(ctx context.Context, accountID string) (*model.AccountContext, error)
	CreateAccountContext(ctx context.Context, ctxModel *model.AccountContext) error
	UpdateAccountContext(ctx context.Context, ctxModel *model.AccountContext) error
}

type accountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) AccountRepository {
	return &accountRepository{db: db}
}

func (r *accountRepository) GetAccount(ctx context.Context, id string) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *accountRepository) GetAccountsByIds(ctx context.Context, ids []string) ([]*model.Account, error) {
	var accounts []*model.Account
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&accounts).Error
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *accountRepository) GetAccountByEmail(ctx context.Context, email string) (*model.Account, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var account model.Account
	err := r.db.WithContext(ctx).
		Where("LOWER(email) = ?", normalizedEmail).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *accountRepository) CreateAccount(ctx context.Context, account *model.Account) error {
	if account.ID == "" {
		account.ID = uuid.New().String()
	}
	return r.db.WithContext(ctx).Create(account).Error
}

func (r *accountRepository) UpdateAccount(ctx context.Context, account *model.Account) error {
	return r.db.WithContext(ctx).Save(account).Error
}

func (r *accountRepository) DeleteAccount(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Account{}).Error
}

func (r *accountRepository) GetGroupRole(ctx context.Context, accountID string, organizationID string) (string, error) {
	var role string
	err := r.db.WithContext(ctx).Table("members").
		Where("account_id = ? AND organization_id = ?", accountID, organizationID).
		Select("role").Row().Scan(&role)
	if err != nil {
		// When using Row().Scan(), "sql: no rows in result set" is returned instead of gorm.ErrRecordNotFound
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "sql: no rows in result set" {
			return "normal", nil
		}
		return "", err
	}
	return role, nil
}

func (r *accountRepository) GetGroupRoleByTenantID(ctx context.Context, accountID string, tenantID string) (string, error) {
	organizationID, found, err := r.resolveOrganizationIDByTenantID(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if !found {
		return "normal", nil
	}

	return r.GetGroupRole(ctx, accountID, organizationID)
}

func (r *accountRepository) GetAccountRoleInEnterpriseGroup(ctx context.Context, accountID string, groupID string) (string, error) {
	return r.GetGroupRole(ctx, accountID, groupID)
}

func (r *accountRepository) GetAccountIntegrate(ctx context.Context, accountID string, provider model.AccountIntegrateProvider) (*model.AccountIntegrate, error) {
	var integration model.AccountIntegrate
	err := r.db.WithContext(ctx).Where("account_id = ? AND provider = ?", accountID, provider).First(&integration).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

func (r *accountRepository) GetAccountIntegrateByProviderOpenID(ctx context.Context, provider model.AccountIntegrateProvider, openID string) (*model.AccountIntegrate, error) {
	var integration model.AccountIntegrate
	err := r.db.WithContext(ctx).
		Where("provider = ? AND open_id = ?", provider, openID).
		First(&integration).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

func (r *accountRepository) GetAccountByNormalizedMobile(ctx context.Context, mobile string) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).
		Where("mobile_e164 = ?", mobile).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *accountRepository) CreateAccountIntegrate(ctx context.Context, integrate *model.AccountIntegrate) error {
	return r.db.WithContext(ctx).Create(integrate).Error
}

func (r *accountRepository) UpdateAccountIntegrate(ctx context.Context, integrate *model.AccountIntegrate) error {
	return r.db.WithContext(ctx).Save(integrate).Error
}

func (r *accountRepository) DeleteAccountIntegrate(ctx context.Context, accountID string, provider model.AccountIntegrateProvider) error {
	return r.db.WithContext(ctx).Where("account_id = ? AND provider = ?", accountID, provider).Delete(&model.AccountIntegrate{}).Error
}

func (r *accountRepository) GetByEmailAndPassword(ctx context.Context, email, password string) (*model.Account, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var account model.Account
	err := r.db.WithContext(ctx).
		Where("LOWER(email) = ? AND password = ?", normalizedEmail, password).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *accountRepository) GetByEmailAndStatus(ctx context.Context, email string, status model.AccountStatus) (*model.Account, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var account model.Account
	err := r.db.WithContext(ctx).
		Where("LOWER(email) = ? AND status = ?", normalizedEmail, status).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *accountRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return false, nil
	}

	var count int64
	err := r.db.WithContext(ctx).
		Unscoped().
		Model(&model.Account{}).
		Where("LOWER(email) = ?", normalizedEmail).
		Count(&count).Error
	return count > 0, err
}

func (r *accountRepository) GetActiveAccounts(ctx context.Context, limit, offset int) ([]*model.Account, error) {
	var accounts []*model.Account
	err := r.db.WithContext(ctx).Where("status = ?", model.AccountStatusActive).Limit(limit).Offset(offset).Find(&accounts).Error
	return accounts, err
}

func (r *accountRepository) GetAccountContextByAccountID(ctx context.Context, accountID string) (*model.AccountContext, error) {
	var ctxModel model.AccountContext
	err := r.db.WithContext(ctx).Where("account_id = ?", accountID).First(&ctxModel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &ctxModel, nil
}

func (r *accountRepository) CreateAccountContext(ctx context.Context, ctxModel *model.AccountContext) error {
	now := time.Now()
	ctxModel.CreatedAt = now
	ctxModel.UpdatedAt = now
	return r.db.WithContext(ctx).Create(ctxModel).Error
}

func (r *accountRepository) UpdateAccountContext(ctx context.Context, ctxModel *model.AccountContext) error {
	ctxModel.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(ctxModel).Error
}

func (r *accountRepository) CountByStatus(ctx context.Context, status model.AccountStatus) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Account{}).Where("status = ?", status).Count(&count).Error
	return count, err
}

func (r *accountRepository) GetAccountsWithExtensions(ctx context.Context, query *gorm.DB, page, limit int) ([]*model.Account, int64, error) {
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count accounts: %w", err)
	}

	offset := (page - 1) * limit
	var accounts []*model.Account

	if err := query.Offset(offset).Limit(limit).Find(&accounts).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch accounts: %w", err)
	}

	return accounts, total, nil
}

func (r *accountRepository) GetAccountById(ctx context.Context, id string) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *accountRepository) GetAccountWithExtensionsByEmail(ctx context.Context, email string) (*model.Account, error) {
	return r.GetAccountByEmail(ctx, email)
}

func (r *accountRepository) GetDB() *gorm.DB {
	return r.db
}

func (r *accountRepository) UpsertOrganizationRole(ctx context.Context, tenantID string, accountID string, role string) error {
	organizationID, found, err := r.resolveOrganizationIDByTenantID(ctx, tenantID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("tenant %s not found in any enterprise group", tenantID)
	}

	result := r.db.WithContext(ctx).
		Table("members").
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		Update("role", role)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return r.db.WithContext(ctx).
			Table("members").
			Create(map[string]interface{}{
				"organization_id": organizationID,
				"account_id":      accountID,
				"role":            role,
			}).Error
	}

	return nil
}

func (r *accountRepository) GetEnterpriseGroupsByTenantID(ctx context.Context, tenantID string) ([]string, error) {
	organizationID, found, err := r.resolveOrganizationIDByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !found {
		return []string{}, nil
	}
	return []string{organizationID}, nil
}

func (r *accountRepository) resolveOrganizationIDByTenantID(ctx context.Context, tenantID string) (organizationID string, found bool, err error) {
	err = r.db.WithContext(ctx).
		Table("workspaces").
		Where("id = ? AND organization_id IS NOT NULL", tenantID).
		Select("organization_id").
		Row().
		Scan(&organizationID)
	if err == nil {
		return organizationID, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, err
	}

	var count int64
	if err := r.db.WithContext(ctx).
		Table("organizations").
		Where("id = ?", tenantID).
		Count(&count).Error; err != nil {
		return "", false, err
	}
	if count == 0 {
		return "", false, nil
	}
	return tenantID, true, nil
}

func (r *accountRepository) ExecuteInTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	tx := r.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

func (r *accountRepository) WithTx(tx *gorm.DB) AccountRepository {
	return NewAccountRepository(tx)
}

func (r *accountRepository) SelectAccountAndTenantAccountJoin(ctx context.Context, invitationData map[string]string, workspace workspace_model.Workspace) (*model.AccountAndJoin, error) {
	type AccountWithRole struct {
		model.Account
		Role string `gorm:"column:role"`
	}

	var result AccountWithRole

	err := r.db.WithContext(ctx).
		Model(&model.Account{}).
		Select("accounts.*, workspace_members.role").
		Joins("JOIN workspace_members ON accounts.id = workspace_members.account_id").
		Where("accounts.email = ? AND workspace_members.workspace_id = ?", invitationData["email"], workspace.ID).
		Scan(&result).Error

	if err != nil {
		return nil, err
	}

	if result.Email == "" {
		return nil, gorm.ErrRecordNotFound
	}

	accountAndJoin := &model.AccountAndJoin{
		Account: result.Account,
		Role:    result.Role,
	}

	return accountAndJoin, nil
}
