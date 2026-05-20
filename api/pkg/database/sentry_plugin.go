package database

import (
	"errors"
	"math/rand"
	"time"

	sentryHelper "github.com/zgiai/ginext/pkg/sentry"
	"gorm.io/gorm"
)

// SentryPlugin is a GORM plugin that reports database errors to Sentry
type SentryPlugin struct {
	// SlowQueryThreshold defines the threshold for slow queries (in milliseconds)
	SlowQueryThreshold time.Duration
	// SlowQuerySampleRate defines the sample rate for slow query reporting (0.0 to 1.0)
	// Default is 1.0 (report all slow queries)
	SlowQuerySampleRate float64
}

// Name returns the plugin name
func (p *SentryPlugin) Name() string {
	return "sentry_plugin"
}

// Initialize initializes the plugin
func (p *SentryPlugin) Initialize(db *gorm.DB) error {
	// Set default slow query threshold if not set
	if p.SlowQueryThreshold == 0 {
		p.SlowQueryThreshold = 1000 * time.Millisecond // 1 second
	}

	// Set default sample rate if not set
	if p.SlowQuerySampleRate == 0 {
		p.SlowQuerySampleRate = 1.0 // Report all slow queries by default
	}

	// Register callback BEFORE query to track start time
	err := db.Callback().Query().Before("gorm:query").Register("sentry:before_query", p.beforeQuery)
	if err != nil {
		return err
	}

	// Register callback for after query
	err = db.Callback().Query().After("gorm:query").Register("sentry:after_query", p.afterQuery)
	if err != nil {
		return err
	}

	// Register callback for after create
	err = db.Callback().Create().After("gorm:create").Register("sentry:after_create", p.afterCreate)
	if err != nil {
		return err
	}

	// Register callback for after update
	err = db.Callback().Update().After("gorm:update").Register("sentry:after_update", p.afterUpdate)
	if err != nil {
		return err
	}

	// Register callback for after delete
	err = db.Callback().Delete().After("gorm:delete").Register("sentry:after_delete", p.afterDelete)
	if err != nil {
		return err
	}

	return nil
}

// beforeQuery is called before a query operation to track start time
func (p *SentryPlugin) beforeQuery(db *gorm.DB) {
	if db.Statement != nil {
		db.Statement.Settings.Store("sentry:start_time", time.Now())
	}
}

// afterQuery is called after a query operation
func (p *SentryPlugin) afterQuery(db *gorm.DB) {
	p.checkError(db, "SELECT")
	p.checkSlowQuery(db)
}

// afterCreate is called after a create operation
func (p *SentryPlugin) afterCreate(db *gorm.DB) {
	p.checkError(db, "INSERT")
}

// afterUpdate is called after an update operation
func (p *SentryPlugin) afterUpdate(db *gorm.DB) {
	p.checkError(db, "UPDATE")
}

// afterDelete is called after a delete operation
func (p *SentryPlugin) afterDelete(db *gorm.DB) {
	p.checkError(db, "DELETE")
}

// checkError checks if there's an error and reports it to Sentry
func (p *SentryPlugin) checkError(db *gorm.DB, operation string) {
	if db.Error != nil && !errors.Is(db.Error, gorm.ErrRecordNotFound) {
		// Get table name
		tableName := "unknown"
		if db.Statement != nil && db.Statement.Table != "" {
			tableName = db.Statement.Table
		}

		// Report to Sentry
		sentryHelper.CaptureDBError(db.Error, operation, tableName, map[string]interface{}{
			"sql":           db.Statement.SQL.String(),
			"rows_affected": db.RowsAffected,
		})
	}
}

// checkSlowQuery checks if the query is slow and reports it
func (p *SentryPlugin) checkSlowQuery(db *gorm.DB) {
	// Skip if no statement
	if db.Statement == nil {
		return
	}

	// Get start time from statement settings
	startTimeVal, ok := db.Statement.Settings.Load("sentry:start_time")
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

		// Report slow query to Sentry
		sentryHelper.CaptureDBError(
			errors.New("slow query detected"),
			"SLOW_QUERY",
			tableName,
			map[string]interface{}{
				"sql":           db.Statement.SQL.String(),
				"duration_ms":   duration.Milliseconds(),
				"threshold_ms":  p.SlowQueryThreshold.Milliseconds(),
				"rows_affected": db.RowsAffected,
				"sample_rate":   p.SlowQuerySampleRate,
			},
		)
	}
}
