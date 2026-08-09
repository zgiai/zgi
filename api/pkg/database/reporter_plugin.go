package database

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zgiai/zgi/api/internal/observability"
	"gorm.io/gorm"
)

// ReporterPlugin is a GORM plugin that reports database errors through
// ZGIReporter without depending on a specific observability platform.
type ReporterPlugin struct {
	// SlowQueryThreshold defines the threshold for slow queries (in milliseconds)
	SlowQueryThreshold time.Duration
	// SlowQuerySampleRate defines the sample rate for slow query reporting (0.0 to 1.0)
	// Default is 1.0 (report all slow queries)
	SlowQuerySampleRate float64
}

// OperationError marks a failure as originating from an explicit database
// operation while retaining the concrete driver error for classification.
type OperationError struct {
	Operation string
	Cause     error
}

func (e *OperationError) Error() string {
	return e.Operation + ": " + e.Cause.Error()
}

func (e *OperationError) Unwrap() error {
	return e.Cause
}

// WrapOperationError marks an error at the repository/query boundary.
func WrapOperationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &OperationError{Operation: operation, Cause: err}
}

// IsOperationError reports whether an error came from an explicit database
// operation rather than an unrelated network boundary.
func IsOperationError(err error) bool {
	var operationErr *OperationError
	return errors.As(err, &operationErr)
}

// Name returns the plugin name
func (p *ReporterPlugin) Name() string {
	return "zgi_reporter_plugin"
}

// Initialize initializes the plugin
func (p *ReporterPlugin) Initialize(db *gorm.DB) error {
	// Set default slow query threshold if not set
	if p.SlowQueryThreshold == 0 {
		p.SlowQueryThreshold = 1000 * time.Millisecond // 1 second
	}

	// Set default sample rate if not set
	if p.SlowQuerySampleRate == 0 {
		p.SlowQuerySampleRate = 1.0 // Report all slow queries by default
	}

	// Register callback BEFORE query to track start time
	err := db.Callback().Query().Before("gorm:query").Register("zgi_reporter:before_query", p.beforeQuery)
	if err != nil {
		return err
	}

	// Register callback for after query
	err = db.Callback().Query().After("gorm:query").Register("zgi_reporter:after_query", p.afterQuery)
	if err != nil {
		return err
	}

	// Register callback for after create
	err = db.Callback().Create().After("gorm:create").Register("zgi_reporter:after_create", p.afterCreate)
	if err != nil {
		return err
	}

	// Register callback for after update
	err = db.Callback().Update().After("gorm:update").Register("zgi_reporter:after_update", p.afterUpdate)
	if err != nil {
		return err
	}

	// Register callback for after delete
	err = db.Callback().Delete().After("gorm:delete").Register("zgi_reporter:after_delete", p.afterDelete)
	if err != nil {
		return err
	}

	return nil
}

// beforeQuery is called before a query operation to track start time
func (p *ReporterPlugin) beforeQuery(db *gorm.DB) {
	if db.Statement != nil {
		db.Statement.Settings.Store("zgi_reporter:start_time", time.Now())
	}
}

// afterQuery is called after a query operation
func (p *ReporterPlugin) afterQuery(db *gorm.DB) {
	p.checkError(db, "SELECT")
	p.checkSlowQuery(db)
}

// afterCreate is called after a create operation
func (p *ReporterPlugin) afterCreate(db *gorm.DB) {
	p.checkError(db, "INSERT")
}

// afterUpdate is called after an update operation
func (p *ReporterPlugin) afterUpdate(db *gorm.DB) {
	p.checkError(db, "UPDATE")
}

// afterDelete is called after a delete operation
func (p *ReporterPlugin) afterDelete(db *gorm.DB) {
	p.checkError(db, "DELETE")
}

// checkError checks if there's an error and reports it through ZGI Reporter.
func (p *ReporterPlugin) checkError(db *gorm.DB, operation string) {
	if db.Error != nil && !errors.Is(db.Error, gorm.ErrRecordNotFound) {
		ctx := context.Background()
		// Get table name
		tableName := "unknown"
		if db.Statement != nil {
			ctx = db.Statement.Context
			if db.Statement.Table != "" {
				tableName = db.Statement.Table
			}
		}

		observability.CaptureError(ctx, "database.operation.failed", db.Error,
			observability.WithErrorClassification(classifyDatabaseError(db.Error)),
			observability.Tags(map[string]string{
				"db.operation": operation,
				"db.table":     tableName,
			}),
			observability.Attribute("db.rows_affected", db.RowsAffected),
		)
	}
}

func classifyDatabaseError(err error) observability.ErrorClassification {
	classification := observability.ErrorClassification{
		Category: observability.ErrorCategoryDatabase,
		Source:   observability.ErrorSourceZGI,
		Code:     "database_operation_failed",
	}
	if errors.Is(err, context.DeadlineExceeded) {
		classification.Category = observability.ErrorCategoryTimeout
		classification.Source = observability.ErrorSourceInfrastructure
		classification.Code = "database_timeout"
		classification.Retryable = true
		return classification
	}

	// A connection error may wrap a PostgreSQL rejection such as invalid
	// credentials. Preserve that SQLSTATE before falling back to a generic
	// transport classification.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr != nil && pgErr.Code != "" {
		classification.Code = "postgres_" + pgErr.Code
		class := pgErr.Code
		if len(class) > 2 {
			class = class[:2]
		}
		switch strings.ToUpper(class) {
		case "08", "40", "53", "57", "58":
			classification.Source = observability.ErrorSourceInfrastructure
			classification.Retryable = true
		}
		return classification
	}

	var connectErr *pgconn.ConnectError
	if errors.As(err, &connectErr) {
		classification.Source = observability.ErrorSourceInfrastructure
		classification.Code = "postgres_connection_failed"
		classification.Retryable = true
		return classification
	}
	var networkErr *net.OpError
	if errors.As(err, &networkErr) {
		classification.Source = observability.ErrorSourceInfrastructure
		classification.Code = "database_transport_failed"
		classification.Retryable = true
		return classification
	}
	return classification
}

// ClassifyError exposes the database ownership taxonomy to callers that
// surface database failures through a higher-level operation. Keeping one
// classifier prevents the same outage from being assigned to contradictory
// owners by the database and domain layers.
func ClassifyError(err error) observability.ErrorClassification {
	return classifyDatabaseError(err)
}

// checkSlowQuery checks if the query is slow and reports it
func (p *ReporterPlugin) checkSlowQuery(db *gorm.DB) {
	// Skip if no statement
	if db.Statement == nil {
		return
	}

	// Get start time from statement settings
	startTimeVal, ok := db.Statement.Settings.Load("zgi_reporter:start_time")
	if !ok {
		return
	}

	startTime, ok := startTimeVal.(time.Time)
	if !ok {
		return
	}

	// Calculate duration
	duration := time.Since(startTime)

	// Check if query is slow
	if duration > p.SlowQueryThreshold {
		// Apply sampling: only report a percentage of slow queries
		// This reduces overhead in high-traffic scenarios
		if rand.Float64() > p.SlowQuerySampleRate {
			return
		}

		// Get table name
		tableName := "unknown"
		if db.Statement.Table != "" {
			tableName = db.Statement.Table
		}

		observability.CaptureError(
			db.Statement.Context,
			"database.query.slow",
			errors.New("slow query detected"),
			observability.WithLevel(observability.LevelWarning),
			observability.WithErrorClassification(observability.ErrorClassification{
				Category: observability.ErrorCategoryDatabase,
				Source:   observability.ErrorSourceZGI,
				Code:     "slow_query",
			}),
			observability.Tags(map[string]string{
				"db.operation": "SLOW_QUERY",
				"db.table":     tableName,
			}),
			observability.Attributes(map[string]any{
				"duration_ms":   duration.Milliseconds(),
				"threshold_ms":  p.SlowQueryThreshold.Milliseconds(),
				"rows_affected": db.RowsAffected,
				"sample_rate":   p.SlowQuerySampleRate,
			}),
		)
	}
}
