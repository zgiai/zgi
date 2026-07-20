package database

import (
	"context"
	"errors"
	"math/rand"
	"time"

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
			observability.Tags(map[string]string{
				"db.operation": operation,
				"db.table":     tableName,
			}),
			observability.Attribute("db.rows_affected", db.RowsAffected),
		)
	}
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
