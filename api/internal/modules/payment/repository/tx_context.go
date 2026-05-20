package repository

import (
	"context"

	"gorm.io/gorm"
)

// txKey is the context key for storing transaction
type txKey struct{}

// WithTx returns a new context with the transaction stored
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// GetDB returns the transaction from context if exists, otherwise returns the default db
// This allows repository methods to automatically use the transaction when available
func GetDB(ctx context.Context, defaultDB *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return defaultDB.WithContext(ctx)
}

// HasTx checks if there is a transaction in the context
func HasTx(ctx context.Context) bool {
	_, ok := ctx.Value(txKey{}).(*gorm.DB)
	return ok
}

// Transaction executes fn within a transaction.
// If there's already a transaction in context, GORM will automatically use SAVEPOINT for nested transaction.
// This allows methods to declare their own Transaction without worrying about nesting.
//
// Usage in service:
//
//	func (s *Service) DoSomething(ctx context.Context) error {
//	    return repository.Transaction(ctx, s.db, func(ctx context.Context) error {
//	        // All operations here use the same transaction
//	        // Nested Transaction calls will use SAVEPOINT automatically
//	        return s.otherService.DoOtherThing(ctx) // This can also call Transaction
//	    })
//	}
func Transaction(ctx context.Context, db *gorm.DB, fn func(ctx context.Context) error) error {
	// Get current DB (might be an existing transaction from context, or the default db)
	currentDB := GetDB(ctx, db)

	return currentDB.Transaction(func(tx *gorm.DB) error {
		// Store the transaction in context for downstream operations
		txCtx := WithTx(ctx, tx)
		return fn(txCtx)
	})
}
