package integrations

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OAuthClientFlowLocker serializes OAuth client material changes with the
// creation of authorization flows that depend on that material.
type OAuthClientFlowLocker interface {
	WithinOAuthClientFlowLock(
		ctx context.Context,
		organizationID uuid.UUID,
		integrationID string,
		authMethodID string,
		operation func(context.Context) error,
	) error
}

type gormOAuthClientFlowLocker struct {
	db *gorm.DB
}

func newGormOAuthClientFlowLocker(db *gorm.DB) OAuthClientFlowLocker {
	if db == nil {
		return nil
	}
	return &gormOAuthClientFlowLocker{db: db}
}

type oauthClientFlowTransactionContextKey struct{}

func (locker *gormOAuthClientFlowLocker) WithinOAuthClientFlowLock(
	ctx context.Context,
	organizationID uuid.UUID,
	integrationID string,
	authMethodID string,
	operation func(context.Context) error,
) error {
	if locker == nil || locker.db == nil || operation == nil {
		return fmt.Errorf("integration OAuth client-flow lock is unavailable")
	}
	lockKey := organizationID.String() + "/" +
		normalizeOAuthIdentifier(integrationID) + "/" +
		normalizeOAuthIdentifier(authMethodID)
	return locker.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec(
				"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
				lockKey,
			).Error; err != nil {
				return fmt.Errorf("lock integration OAuth client and flow: %w", err)
			}
		}
		lockedContext := context.WithValue(ctx, oauthClientFlowTransactionContextKey{}, tx)
		return operation(lockedContext)
	})
}

func oauthClientFlowDatabase(ctx context.Context, fallback *gorm.DB) (*gorm.DB, bool) {
	if ctx != nil {
		if tx, ok := ctx.Value(oauthClientFlowTransactionContextKey{}).(*gorm.DB); ok && tx != nil {
			return tx.WithContext(ctx), true
		}
	}
	if fallback == nil {
		return nil, false
	}
	return fallback.WithContext(ctx), false
}

func withOAuthClientFlowLock(
	ctx context.Context,
	locker OAuthClientFlowLocker,
	organizationID uuid.UUID,
	integrationID string,
	authMethodID string,
	operation func(context.Context) error,
) error {
	if locker == nil {
		return operation(ctx)
	}
	return locker.WithinOAuthClientFlowLock(
		ctx,
		organizationID,
		integrationID,
		authMethodID,
		operation,
	)
}
