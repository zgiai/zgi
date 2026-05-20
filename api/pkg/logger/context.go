package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextFieldsKey struct{}

// WithFields attaches structured logging fields to a context.
func WithFields(ctx context.Context, fields ...zap.Field) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(fields) == 0 {
		return ctx
	}

	merged := append(fieldsFromContext(ctx), fields...)
	copied := make([]zap.Field, len(merged))
	copy(copied, merged)
	return context.WithValue(ctx, contextFieldsKey{}, copied)
}

// DebugContext logs a debug message with fields extracted from context.
func DebugContext(ctx context.Context, msg string, args ...interface{}) {
	writeWithOptions(zapcore.DebugLevel, logTypeApp, fieldsFromContext(ctx), false, 2, msg, args...)
}

// InfoContext logs an info message with fields extracted from context.
func InfoContext(ctx context.Context, msg string, args ...interface{}) {
	writeWithOptions(zapcore.InfoLevel, logTypeApp, fieldsFromContext(ctx), false, 2, msg, args...)
}

// WarnContext logs a warning message with fields extracted from context.
func WarnContext(ctx context.Context, msg string, args ...interface{}) {
	writeWithOptions(zapcore.WarnLevel, logTypeApp, fieldsFromContext(ctx), false, 2, msg, args...)
}

// ErrorContext logs an error message with fields extracted from context.
func ErrorContext(ctx context.Context, msg string, args ...interface{}) {
	writeWithOptions(zapcore.ErrorLevel, logTypeError, fieldsFromContext(ctx), false, 2, msg, args...)
}

// CriticalContext logs an error message with fields extracted from context and a stacktrace.
func CriticalContext(ctx context.Context, msg string, args ...interface{}) {
	writeWithOptions(zapcore.ErrorLevel, logTypeError, fieldsFromContext(ctx), true, 2, msg, args...)
}

func fieldsFromContext(ctx context.Context) []zap.Field {
	if ctx == nil {
		return nil
	}

	fields, _ := ctx.Value(contextFieldsKey{}).([]zap.Field)
	if len(fields) == 0 {
		return nil
	}

	copied := make([]zap.Field, len(fields))
	copy(copied, fields)
	return copied
}
