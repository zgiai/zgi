package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	registrationProvisioningOutboxPending    = "pending"
	registrationProvisioningOutboxProcessing = "processing"
	registrationProvisioningOutboxCompleted  = "completed"

	registrationProvisioningStepRoute        = 1
	registrationProvisioningStepOrganization = 2
	registrationProvisioningStepGrant        = 3

	registrationProvisioningLeaseDuration = 30 * time.Second
	registrationProvisioningStateTimeout  = 2 * time.Second
)

// RegistrationProvisioningOutbox is the durable handoff between a committed
// cloud registration and its local/remote post-commit provisioning steps.
type RegistrationProvisioningOutbox struct {
	ID             string     `gorm:"column:id;type:uuid;primaryKey"`
	OrganizationID string     `gorm:"column:organization_id;type:uuid;not null;uniqueIndex"`
	AccountID      string     `gorm:"column:account_id;type:uuid;not null"`
	Status         string     `gorm:"column:status;type:varchar(16);not null"`
	CompletedStep  int        `gorm:"column:completed_step;not null"`
	AttemptCount   int        `gorm:"column:attempt_count;not null"`
	NextAttemptAt  time.Time  `gorm:"column:next_attempt_at;not null;index"`
	LeaseOwner     *string    `gorm:"column:lease_owner;type:uuid"`
	LeaseUntil     *time.Time `gorm:"column:lease_until"`
	LastError      string     `gorm:"column:last_error;type:text;not null"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null"`
	CompletedAt    *time.Time `gorm:"column:completed_at"`
}

func (RegistrationProvisioningOutbox) TableName() string {
	return "registration_provisioning_outbox"
}

func newRegistrationProvisioningOutbox(accountID, organizationID string) *RegistrationProvisioningOutbox {
	now := time.Now().UTC()
	return &RegistrationProvisioningOutbox{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		AccountID:      accountID,
		Status:         registrationProvisioningOutboxPending,
		NextAttemptAt:  now,
		LastError:      "",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// EnqueueRegistrationProvisioningOutbox durably records cloud provisioning in
// the caller's transaction. Only stable identifiers cross this boundary; the
// worker reloads current account and organization data when each attempt runs.
func EnqueueRegistrationProvisioningOutbox(
	ctx context.Context,
	tx *gorm.DB,
	accountID string,
	organizationID string,
) (*RegistrationProvisioningOutbox, error) {
	if tx == nil {
		return nil, fmt.Errorf("registration provisioning outbox requires transaction")
	}
	entry := newRegistrationProvisioningOutbox(strings.TrimSpace(accountID), strings.TrimSpace(organizationID))
	if strings.TrimSpace(entry.AccountID) == "" || strings.TrimSpace(entry.OrganizationID) == "" {
		return nil, fmt.Errorf("registration provisioning outbox scope is incomplete")
	}
	if err := tx.WithContext(ctx).Create(entry).Error; err != nil {
		return nil, fmt.Errorf("create registration provisioning outbox: %w", err)
	}
	return entry, nil
}

// RegistrationProvisioningOutboxProcessor performs the ordered, idempotent
// cloud provisioning workflow after the account transaction commits.
type RegistrationProvisioningOutboxProcessor struct {
	db                       *gorm.DB
	officialRoutes           interfaces.OfficialRouteBootstrapper
	console                  platformconsole.ConsoleProvider
	organizationRegistration platformconsole.OrganizationRegistrationSyncer
	now                      func() time.Time
	leaseDuration            time.Duration
}

func NewRegistrationProvisioningOutboxProcessor(
	db *gorm.DB,
	officialRoutes interfaces.OfficialRouteBootstrapper,
	consoleProvider platformconsole.ConsoleProvider,
) *RegistrationProvisioningOutboxProcessor {
	organizationRegistration, _ := consoleProvider.(platformconsole.OrganizationRegistrationSyncer)
	return &RegistrationProvisioningOutboxProcessor{
		db:                       db,
		officialRoutes:           officialRoutes,
		console:                  consoleProvider,
		organizationRegistration: organizationRegistration,
		now: func() time.Time {
			return time.Now().UTC()
		},
		leaseDuration: registrationProvisioningLeaseDuration,
	}
}

// ProcessPending claims and processes up to limit due entries. A failed entry
// is released with durable backoff while the rest of the claimed batch runs.
func (p *RegistrationProvisioningOutboxProcessor) ProcessPending(ctx context.Context, limit int) (int, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var processErrors []error
	processed := 0
	for processed < limit {
		entries, err := p.claimPending(ctx, 1)
		if err != nil {
			processErrors = append(processErrors, err)
			break
		}
		if len(entries) == 0 {
			break
		}
		processed++
		if err := p.processClaimed(ctx, &entries[0]); err != nil {
			processErrors = append(processErrors, err)
		}
	}
	return processed, errors.Join(processErrors...)
}

// ProcessOne is used for the best-effort immediate attempt after registration
// commit. Multi-instance safety is the same as the scheduled batch path.
func (p *RegistrationProvisioningOutboxProcessor) ProcessOne(ctx context.Context, id string) error {
	if err := p.validate(); err != nil {
		return err
	}
	entry, err := p.claimOne(ctx, id)
	if err != nil || entry == nil {
		return err
	}
	return p.processClaimed(ctx, entry)
}

func (p *RegistrationProvisioningOutboxProcessor) validate() error {
	if p == nil || p.db == nil {
		return fmt.Errorf("registration provisioning outbox database is not configured")
	}
	if p.officialRoutes == nil {
		return fmt.Errorf("registration provisioning official route service is not configured")
	}
	if p.console == nil || !p.console.IsAvailable() || !strings.EqualFold(p.console.GetMode(), registrationRunModeCloud) {
		return fmt.Errorf("registration provisioning cloud console is not configured")
	}
	if p.organizationRegistration == nil {
		return fmt.Errorf("registration provisioning synchronous organization registration is not configured")
	}
	return nil
}

// ValidateConfiguration verifies that the durable worker has all required
// capabilities before the application begins accepting cloud registrations.
func (p *RegistrationProvisioningOutboxProcessor) ValidateConfiguration() error {
	return p.validate()
}

func (p *RegistrationProvisioningOutboxProcessor) claimPending(ctx context.Context, limit int) ([]RegistrationProvisioningOutbox, error) {
	now := p.now()
	leaseUntil := now.Add(p.leaseDuration)
	leaseOwner := uuid.NewString()
	if p.db.Dialector.Name() == "postgres" {
		var entries []RegistrationProvisioningOutbox
		err := p.db.WithContext(ctx).Raw(`
			WITH candidates AS (
				SELECT id
				FROM registration_provisioning_outbox
				WHERE (status = ? AND next_attempt_at <= ?)
				   OR (status = ? AND lease_until <= ?)
				ORDER BY next_attempt_at ASC, created_at ASC
				FOR UPDATE SKIP LOCKED
				LIMIT ?
			)
			UPDATE registration_provisioning_outbox AS outbox
			SET status = ?, lease_owner = ?, lease_until = ?,
				attempt_count = attempt_count + 1, updated_at = ?
			FROM candidates
			WHERE outbox.id = candidates.id
			RETURNING outbox.*`,
			registrationProvisioningOutboxPending, now,
			registrationProvisioningOutboxProcessing, now,
			limit, registrationProvisioningOutboxProcessing, leaseOwner, leaseUntil, now,
		).Scan(&entries).Error
		if err != nil {
			return nil, fmt.Errorf("claim registration provisioning outbox: %w", err)
		}
		return entries, nil
	}

	var entries []RegistrationProvisioningOutbox
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidates []RegistrationProvisioningOutbox
		if err := tx.Where(
			"(status = ? AND next_attempt_at <= ?) OR (status = ? AND lease_until <= ?)",
			registrationProvisioningOutboxPending, now,
			registrationProvisioningOutboxProcessing, now,
		).Order("next_attempt_at ASC, created_at ASC").Limit(limit).Find(&candidates).Error; err != nil {
			return err
		}
		for i := range candidates {
			result := tx.Model(&RegistrationProvisioningOutbox{}).
				Where("id = ? AND ((status = ? AND next_attempt_at <= ?) OR (status = ? AND lease_until <= ?))",
					candidates[i].ID, registrationProvisioningOutboxPending, now,
					registrationProvisioningOutboxProcessing, now).
				Updates(map[string]interface{}{
					"status": registrationProvisioningOutboxProcessing, "lease_owner": leaseOwner,
					"lease_until": leaseUntil, "attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				candidates[i].Status = registrationProvisioningOutboxProcessing
				candidates[i].LeaseOwner = &leaseOwner
				candidates[i].LeaseUntil = &leaseUntil
				candidates[i].AttemptCount++
				entries = append(entries, candidates[i])
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim registration provisioning outbox: %w", err)
	}
	return entries, nil
}

func (p *RegistrationProvisioningOutboxProcessor) claimOne(ctx context.Context, id string) (*RegistrationProvisioningOutbox, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	entries, err := p.claimPendingByID(ctx, id)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	return &entries[0], nil
}

func (p *RegistrationProvisioningOutboxProcessor) claimPendingByID(ctx context.Context, id string) ([]RegistrationProvisioningOutbox, error) {
	now := p.now()
	leaseUntil := now.Add(p.leaseDuration)
	leaseOwner := uuid.NewString()
	var entry RegistrationProvisioningOutbox
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&entry).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		claimable := (entry.Status == registrationProvisioningOutboxPending && !entry.NextAttemptAt.After(now)) ||
			(entry.Status == registrationProvisioningOutboxProcessing && entry.LeaseUntil != nil && !entry.LeaseUntil.After(now))
		if !claimable {
			entry.ID = ""
			return nil
		}
		result := tx.Model(&RegistrationProvisioningOutbox{}).
			Where("id = ? AND ((status = ? AND next_attempt_at <= ?) OR (status = ? AND lease_until <= ?))",
				id, registrationProvisioningOutboxPending, now, registrationProvisioningOutboxProcessing, now).
			Updates(map[string]interface{}{
				"status": registrationProvisioningOutboxProcessing, "lease_owner": leaseOwner,
				"lease_until": leaseUntil, "attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			entry.ID = ""
			return nil
		}
		entry.Status = registrationProvisioningOutboxProcessing
		entry.LeaseOwner = &leaseOwner
		entry.LeaseUntil = &leaseUntil
		entry.AttemptCount++
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim registration provisioning outbox %s: %w", id, err)
	}
	if entry.ID == "" {
		return nil, nil
	}
	return []RegistrationProvisioningOutbox{entry}, nil
}

func (p *RegistrationProvisioningOutboxProcessor) processClaimed(ctx context.Context, entry *RegistrationProvisioningOutbox) error {
	var organization workspacemodel.Organization
	if err := p.db.WithContext(ctx).Where("id = ?", entry.OrganizationID).First(&organization).Error; err != nil {
		return p.retry(ctx, entry, fmt.Errorf("load registration organization: %w", err), "organization_lookup")
	}
	var account authmodel.Account
	if err := p.db.WithContext(ctx).Where("id = ?", entry.AccountID).First(&account).Error; err != nil {
		return p.retry(ctx, entry, fmt.Errorf("load registration account: %w", err), "account_lookup")
	}
	organizationID, err := uuid.Parse(entry.OrganizationID)
	if err != nil {
		return p.retry(ctx, entry, fmt.Errorf("parse registration organization id: %w", err), "organization_id")
	}
	if entry.CompletedStep < registrationProvisioningStepRoute {
		if err := p.officialRoutes.InitOfficialChannel(ctx, organizationID); err != nil {
			return p.retry(ctx, entry, fmt.Errorf("initialize official route: %w", err), "official_route")
		}
		if err := p.persistStep(ctx, entry, registrationProvisioningStepRoute); err != nil {
			return p.retry(ctx, entry, err, "official_route_checkpoint")
		}
	}
	if entry.CompletedStep < registrationProvisioningStepOrganization {
		if err := p.organizationRegistration.RegisterOrganizationSync(ctx, &platformconsole.RegisterOrganizationRequest{
			OrganizationID: entry.OrganizationID,
			Name:           organization.Name,
			OwnerEmail:     account.Email,
			CreatedAt:      organization.CreatedAt,
		}); err != nil {
			return p.retry(ctx, entry, fmt.Errorf("register organization with console: %w", err), "organization_registration")
		}
		if err := p.persistStep(ctx, entry, registrationProvisioningStepOrganization); err != nil {
			return p.retry(ctx, entry, err, "organization_registration_checkpoint")
		}
	}
	if entry.CompletedStep < registrationProvisioningStepGrant {
		if _, err := p.console.NotifyOfficialSignup(ctx, &platformconsole.NotifyOfficialSignupRequest{
			OrganizationID: entry.OrganizationID,
			AccountID:      entry.AccountID,
		}); err != nil {
			return p.retry(ctx, entry, fmt.Errorf("grant official signup credit: %w", err), "signup_grant")
		}
		if err := p.complete(ctx, entry); err != nil {
			return p.retry(ctx, entry, err, "signup_grant_checkpoint")
		}
	}
	return nil
}

func (p *RegistrationProvisioningOutboxProcessor) persistStep(ctx context.Context, entry *RegistrationProvisioningOutbox, step int) error {
	stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), registrationProvisioningStateTimeout)
	defer cancel()
	now := p.now()
	leaseUntil := now.Add(p.leaseDuration)
	leaseOwner := registrationProvisioningLeaseOwner(entry)
	result := p.db.WithContext(stateCtx).Model(&RegistrationProvisioningOutbox{}).
		Where("id = ? AND status = ? AND lease_owner = ?", entry.ID, registrationProvisioningOutboxProcessing, leaseOwner).
		Updates(map[string]interface{}{
			"completed_step": step, "lease_until": leaseUntil, "last_error": "", "updated_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("persist registration provisioning step %d: %w", step, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("registration provisioning lease lost while persisting step %d", step)
	}
	entry.CompletedStep = step
	entry.LeaseUntil = &leaseUntil
	return nil
}

func (p *RegistrationProvisioningOutboxProcessor) complete(ctx context.Context, entry *RegistrationProvisioningOutbox) error {
	stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), registrationProvisioningStateTimeout)
	defer cancel()
	now := p.now()
	leaseOwner := registrationProvisioningLeaseOwner(entry)
	result := p.db.WithContext(stateCtx).Model(&RegistrationProvisioningOutbox{}).
		Where("id = ? AND status = ? AND lease_owner = ?", entry.ID, registrationProvisioningOutboxProcessing, leaseOwner).
		Updates(map[string]interface{}{
			"completed_step": registrationProvisioningStepGrant, "status": registrationProvisioningOutboxCompleted,
			"lease_owner": nil, "lease_until": nil, "last_error": "", "completed_at": now, "updated_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("complete registration provisioning outbox: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("registration provisioning lease lost while completing")
	}
	entry.CompletedStep = registrationProvisioningStepGrant
	entry.Status = registrationProvisioningOutboxCompleted
	return nil
}

func (p *RegistrationProvisioningOutboxProcessor) retry(ctx context.Context, entry *RegistrationProvisioningOutbox, cause error, step string) error {
	retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), registrationProvisioningStateTimeout)
	defer cancel()
	now := p.now()
	next := now.Add(registrationProvisioningBackoff(entry.AttemptCount))
	safeFailure := registrationProvisioningFailureClass(step, cause)
	safeError := fmt.Errorf("registration provisioning failed: %s", safeFailure)
	leaseOwner := registrationProvisioningLeaseOwner(entry)
	result := p.db.WithContext(retryCtx).Model(&RegistrationProvisioningOutbox{}).
		Where("id = ? AND status = ? AND lease_owner = ?", entry.ID, registrationProvisioningOutboxProcessing, leaseOwner).
		Updates(map[string]interface{}{
			"status": registrationProvisioningOutboxPending, "lease_owner": nil, "lease_until": nil,
			"next_attempt_at": next, "last_error": safeFailure, "updated_at": now,
		})
	if result.Error != nil {
		return errors.Join(safeError, fmt.Errorf("release registration provisioning outbox for retry: %w", result.Error))
	}
	if result.RowsAffected != 1 {
		return errors.Join(safeError, fmt.Errorf("registration provisioning lease lost while scheduling retry"))
	}
	return safeError
}

func registrationProvisioningLeaseOwner(entry *RegistrationProvisioningOutbox) string {
	if entry == nil || entry.LeaseOwner == nil {
		return ""
	}
	return *entry.LeaseOwner
}

func registrationProvisioningFailureClass(step string, cause error) string {
	step = strings.TrimSpace(step)
	if step == "" {
		step = "unknown_step"
	}
	var consoleErr *platformconsole.ConsoleAPIError
	if errors.As(cause, &consoleErr) {
		return step + ":http_" + strconv.Itoa(consoleErr.StatusCode)
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return step + ":timeout"
	}
	if errors.Is(cause, context.Canceled) {
		return step + ":canceled"
	}
	return step + ":failed"
}

func registrationProvisioningBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	delay := time.Second * time.Duration(1<<uint(attempt))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
