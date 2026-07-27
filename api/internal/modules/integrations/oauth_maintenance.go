package integrations

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	oauthMaintenanceBatchSize        = 100
	oauthMaintenanceInterval         = 5 * time.Minute
	oauthMaintenanceOperationTimeout = 20 * time.Second
	oauthTerminalRetention           = 24 * time.Hour
	oauthMaintenanceAdvisoryLockID   = int64(0x5a47494f41555448) // "ZGIOAUTH"
)

type OAuthArtifactMaintenanceResult struct {
	LeaseAcquired bool
	ExpiredFlows  int64
	ClearedFlows  int64
	DeletedFlows  int64
	DeletedStates int64
}

type OAuthArtifactMaintenanceRepository interface {
	MaintainOAuthArtifacts(context.Context, time.Time, time.Time, int) (OAuthArtifactMaintenanceResult, error)
}

// MaintainOAuthArtifacts performs one bounded cleanup batch. PostgreSQL uses a
// transaction-scoped advisory lease, so it is safe for every API instance to
// run this method on the same schedule.
func (repository *GormOAuthFlowRepository) MaintainOAuthArtifacts(
	ctx context.Context,
	now time.Time,
	terminalBefore time.Time,
	batchSize int,
) (OAuthArtifactMaintenanceResult, error) {
	if repository == nil || repository.db == nil {
		return OAuthArtifactMaintenanceResult{}, fmt.Errorf("integration OAuth maintenance repository is unavailable")
	}
	now = now.UTC()
	terminalBefore = terminalBefore.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if terminalBefore.IsZero() || !terminalBefore.Before(now) {
		terminalBefore = now.Add(-oauthTerminalRetention)
	}
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = oauthMaintenanceBatchSize
	}
	result := OAuthArtifactMaintenanceResult{}
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			var acquired bool
			if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", oauthMaintenanceAdvisoryLockID).Scan(&acquired).Error; err != nil {
				return fmt.Errorf("acquire integration OAuth maintenance lease: %w", err)
			}
			if !acquired {
				return nil
			}
		}
		result.LeaseAcquired = true

		expiredFlowIDs := tx.Model(&IntegrationOAuthFlow{}).
			Select("id").
			Where("status = ? AND expires_at <= ?", OAuthFlowPending, now).
			Order("expires_at ASC").
			Limit(batchSize)
		expired := tx.Model(&IntegrationOAuthFlow{}).
			Where("id IN (?) AND status = ?", expiredFlowIDs, OAuthFlowPending).
			Updates(map[string]any{
				"status": OAuthFlowExpired, "failure_code": ErrorCodeAuthInvalid,
				"completed_at": now, "encrypted_flow_token": "", "updated_at": now,
			})
		if expired.Error != nil {
			return fmt.Errorf("expire integration OAuth flows: %w", expired.Error)
		}
		result.ExpiredFlows = expired.RowsAffected

		terminalSecretIDs := tx.Model(&IntegrationOAuthFlow{}).
			Select("id").
			Where("status <> ? AND encrypted_flow_token <> ''", OAuthFlowPending).
			Order("completed_at ASC NULLS FIRST").
			Limit(batchSize)
		cleared := tx.Model(&IntegrationOAuthFlow{}).
			Where("id IN (?) AND status <> ?", terminalSecretIDs, OAuthFlowPending).
			Updates(map[string]any{"encrypted_flow_token": "", "updated_at": now})
		if cleared.Error != nil {
			return fmt.Errorf("erase terminal integration OAuth flow secrets: %w", cleared.Error)
		}
		result.ClearedFlows = cleared.RowsAffected

		staleStateIDs := tx.Model(&IntegrationOAuthState{}).
			Select("id").
			Where("status = ? OR expires_at <= ?", OAuthStateConsumed, now).
			Order("expires_at ASC").
			Limit(batchSize)
		deletedStates := tx.Where("id IN (?)", staleStateIDs).Delete(&IntegrationOAuthState{})
		if deletedStates.Error != nil {
			return fmt.Errorf("delete stale integration OAuth states: %w", deletedStates.Error)
		}
		result.DeletedStates = deletedStates.RowsAffected

		terminalFlowIDs := tx.Model(&IntegrationOAuthFlow{}).
			Select("id").
			Where("status <> ? AND completed_at IS NOT NULL AND completed_at <= ?", OAuthFlowPending, terminalBefore).
			Order("completed_at ASC").
			Limit(batchSize)
		deletedFlows := tx.Where("id IN (?)", terminalFlowIDs).Delete(&IntegrationOAuthFlow{})
		if deletedFlows.Error != nil {
			return fmt.Errorf("delete terminal integration OAuth flows: %w", deletedFlows.Error)
		}
		result.DeletedFlows = deletedFlows.RowsAffected
		return nil
	})
	if err != nil {
		return OAuthArtifactMaintenanceResult{}, err
	}
	return result, nil
}

func (service *OAuthFlowService) MaintainOAuthArtifacts(ctx context.Context) (OAuthArtifactMaintenanceResult, error) {
	if service == nil || service.maintenance == nil {
		return OAuthArtifactMaintenanceResult{}, nil
	}
	now := time.Now().UTC()
	return service.maintenance.MaintainOAuthArtifacts(
		ctx,
		now,
		now.Add(-oauthTerminalRetention),
		oauthMaintenanceBatchSize,
	)
}

// RunOAuthMaintenance removes one-time authorization material and old terminal
// flows for the API process lifetime. The repository lease prevents concurrent
// workers from processing the same batch.
func (service *OAuthFlowService) RunOAuthMaintenance(ctx context.Context) {
	if service == nil || service.maintenance == nil {
		return
	}
	run := func() {
		operationCtx, cancel := context.WithTimeout(ctx, oauthMaintenanceOperationTimeout)
		defer cancel()
		if _, err := service.MaintainOAuthArtifacts(operationCtx); err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "failed to maintain integration OAuth artifacts", err)
		}
	}
	run()
	ticker := time.NewTicker(oauthMaintenanceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
