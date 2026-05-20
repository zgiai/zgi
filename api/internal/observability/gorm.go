package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const gormSpanInstanceKey = "zgi:otel_gorm_span"

const databaseTracerName = "zgi.database"

type gormTraceConfig struct {
	driver string
	dbName string
}

// InstrumentGORM registers lightweight GORM callbacks for database tracing.
func InstrumentGORM(db *gorm.DB, driver string, dbName string) error {
	if db == nil || !DBEnabled() {
		return nil
	}

	cfg := gormTraceConfig{
		driver: driver,
		dbName: dbName,
	}

	if err := db.Callback().Query().Before("gorm:query").Register("zgi:otel_before_query", beforeGORMSpan("db.query", "SELECT", cfg)); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:after_query").Register("zgi:otel_after_query", afterGORMSpan); err != nil {
		return err
	}
	if err := db.Callback().Create().Before("gorm:create").Register("zgi:otel_before_create", beforeGORMSpan("db.create", "INSERT", cfg)); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:after_create").Register("zgi:otel_after_create", afterGORMSpan); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("zgi:otel_before_update", beforeGORMSpan("db.update", "UPDATE", cfg)); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:after_update").Register("zgi:otel_after_update", afterGORMSpan); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("zgi:otel_before_delete", beforeGORMSpan("db.delete", "DELETE", cfg)); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:after_delete").Register("zgi:otel_after_delete", afterGORMSpan); err != nil {
		return err
	}
	if err := db.Callback().Raw().Before("gorm:raw").Register("zgi:otel_before_raw", beforeGORMSpan("db.raw", "RAW", cfg)); err != nil {
		return err
	}
	if err := db.Callback().Raw().After("gorm:raw").Register("zgi:otel_after_raw", afterGORMSpan); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register("zgi:otel_before_row", beforeGORMSpan("db.row", "ROW", cfg)); err != nil {
		return err
	}
	return db.Callback().Row().After("gorm:row").Register("zgi:otel_after_row", afterGORMSpan)
}

func beforeGORMSpan(spanName string, operation string, cfg gormTraceConfig) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		if tx == nil || tx.Statement == nil {
			return
		}

		ctx := tx.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}

		attrs := []attribute.KeyValue{
			attribute.String("db.system", cfg.driver),
			attribute.String("db.operation", operation),
		}
		if cfg.dbName != "" {
			attrs = append(attrs, attribute.String("zgi.db_name", cfg.dbName))
		}
		if tx.Statement.Table != "" {
			attrs = append(attrs, attribute.String("db.sql.table", tx.Statement.Table))
		}

		ctx, span := otel.Tracer(databaseTracerName).Start(
			ctx,
			spanName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(SanitizeAttributes(attrs)...),
		)
		tx.Statement.Context = ctx
		tx.InstanceSet(gormSpanInstanceKey, span)
	}
}

func afterGORMSpan(tx *gorm.DB) {
	if tx == nil {
		return
	}
	value, ok := tx.InstanceGet(gormSpanInstanceKey)
	if !ok {
		return
	}
	span, ok := value.(trace.Span)
	if !ok {
		return
	}
	EndSpan(span, tx.Error)
}
