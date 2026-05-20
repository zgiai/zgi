package logger

import (
	"fmt"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	appconfig "github.com/zgiai/zgi/api/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	logTypeApp   = "app"
	logTypeError = "error"
)

var (
	logInstance      *zap.Logger
	stdLogRedirectMu sync.Mutex
	restoreStdLog    func()
)

// New creates a logger instance without mutating package state.
func New(configs ...*appconfig.Config) (*zap.Logger, error) {
	cfg := appconfig.LogConfig{
		Level:      "debug",
		Filename:   "logs/app.log",
		MaxSize:    100,
		MaxAge:     15,
		MaxBackups: 7,
		Compress:   true,
	}
	if len(configs) > 0 && configs[0] != nil {
		cfg = configs[0].Log
	}

	level := zap.NewAtomicLevelAt(parseLevel(cfg.Level))
	cores := []zapcore.Core{
		newConsoleCore(level),
	}

	fileCore, err := newFileCore(cfg, level)
	if err != nil {
		return nil, err
	}
	if fileCore != nil {
		cores = append(cores, fileCore)
	}

	return zap.New(
		zapcore.NewTee(cores...),
		zap.AddCaller(),
	), nil
}

func newConsoleCore(level zap.AtomicLevel) zapcore.Core {
	return zapcore.NewCore(
		zapcore.NewJSONEncoder(defaultEncoderConfig()),
		zapcore.AddSync(os.Stdout),
		level,
	)
}

func newFileCore(cfg appconfig.LogConfig, level zap.AtomicLevel) (zapcore.Core, error) {
	filename := strings.TrimSpace(cfg.Filename)
	if filename == "" {
		return nil, nil
	}

	cleanName := filepath.Clean(filename)
	if err := os.MkdirAll(filepath.Dir(cleanName), 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	writer := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cleanName,
		MaxSize:    cfg.MaxSize,
		MaxAge:     cfg.MaxAge,
		MaxBackups: cfg.MaxBackups,
		Compress:   cfg.Compress,
	})

	return zapcore.NewCore(
		zapcore.NewJSONEncoder(defaultEncoderConfig()),
		writer,
		level,
	), nil
}

func defaultEncoderConfig() zapcore.EncoderConfig {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.TimeKey = "ts"
	encoderCfg.MessageKey = "msg"
	encoderCfg.LevelKey = "level"
	return encoderCfg
}

func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.DebugLevel
	}
}

// SetLogger replaces the package logger instance.
func SetLogger(l *zap.Logger) {
	logInstance = l
	redirectStandardLog()
}

// Init initializes the logger.
func Init() {
	l, err := New()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	SetLogger(l)
}

// L returns the package logger.
func L() *zap.Logger {
	if logInstance == nil {
		Init()
	}
	return logInstance
}

func helper(callerSkip int) *zap.Logger {
	return L().WithOptions(zap.AddCallerSkip(callerSkip))
}

// Writer returns an io.Writer that writes standard-library logs into zap.
func Writer(logType string) io.Writer {
	return &stdLogWriter{
		logType: strings.TrimSpace(logType),
		level:   zapcore.InfoLevel,
	}
}

// NewStdLogger builds a standard-library logger backed by zap.
func NewStdLogger(logType string) *stdlog.Logger {
	return stdlog.New(Writer(logType), "", 0)
}

// Error logs an error message.
func Error(msg string, args ...interface{}) {
	write(zapcore.ErrorLevel, logTypeError, msg, args...)
}

// Critical logs an error message with stacktrace.
func Critical(msg string, args ...interface{}) {
	writeWithOptions(zapcore.ErrorLevel, logTypeError, nil, true, 2, msg, args...)
}

// Info logs an info message.
func Info(msg string, args ...interface{}) {
	write(zapcore.InfoLevel, logTypeApp, msg, args...)
}

// Debug logs a debug message.
func Debug(msg string, args ...interface{}) {
	write(zapcore.DebugLevel, logTypeApp, msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...interface{}) {
	write(zapcore.WarnLevel, logTypeApp, msg, args...)
}

// Fatal logs a fatal message and exits the program.
func Fatal(msg string, args ...interface{}) {
	logger := helper(1)
	message, fields := normalizeEntry(msg, args, logTypeError)
	logger.Fatal(message, fields...)
}

// Sync flushes the logger.
func Sync() {
	if logInstance != nil {
		_ = logInstance.Sync()
	}
}

func write(level zapcore.Level, defaultLogType, msg string, args ...interface{}) {
	writeWithOptions(level, defaultLogType, nil, false, 3, msg, args...)
}

func writeWithOptions(level zapcore.Level, defaultLogType string, contextFields []zap.Field, withStack bool, callerSkip int, msg string, args ...interface{}) {
	logger := helper(callerSkip)
	message, fields := normalizeEntry(msg, args, defaultLogType)
	if len(contextFields) > 0 {
		fields = append(append([]zap.Field{}, contextFields...), fields...)
		fields = ensureLogType(fields, defaultLogType)
	}
	if withStack && !hasField(fields, "stacktrace") {
		fields = append(fields, zap.StackSkip("stacktrace", callerSkip))
	}

	switch level {
	case zapcore.DebugLevel:
		logger.Debug(message, fields...)
	case zapcore.InfoLevel:
		logger.Info(message, fields...)
	case zapcore.WarnLevel:
		logger.Warn(message, fields...)
	case zapcore.ErrorLevel:
		logger.Error(message, fields...)
	default:
		logger.Info(message, fields...)
	}
}

func normalizeEntry(msg string, args []interface{}, defaultLogType string) (string, []zap.Field) {
	if len(args) == 0 {
		return msg, []zap.Field{zap.String("log_type", defaultLogType)}
	}

	if fields, ok := buildStructuredFields(args); ok {
		return msg, ensureLogType(fields, defaultLogType)
	}

	if strings.Contains(msg, "%") {
		return fmt.Sprintf(msg, args...), []zap.Field{zap.String("log_type", defaultLogType)}
	}

	if len(args) == 1 {
		return msg, ensureLogType([]zap.Field{zap.Any("value", args[0])}, defaultLogType)
	}

	return msg, ensureLogType([]zap.Field{zap.Any("details", args)}, defaultLogType)
}

func buildStructuredFields(args []interface{}) ([]zap.Field, bool) {
	if len(args) == 1 {
		switch fields := args[0].(type) {
		case map[string]interface{}:
			return mapToFields(fields), true
		}
	}

	fields := make([]zap.Field, 0, len(args))
	for i := 0; i < len(args); {
		switch value := args[i].(type) {
		case zap.Field:
			fields = append(fields, value)
			i++
		case error:
			fields = append(fields, zap.Error(value))
			i++
		case string:
			if i+1 >= len(args) {
				return nil, false
			}
			fields = append(fields, zap.Any(value, args[i+1]))
			i += 2
		default:
			return nil, false
		}
	}

	if len(fields) == 0 {
		return nil, false
	}

	return fields, true
}

func mapToFields(fields map[string]interface{}) []zap.Field {
	zapFields := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		zapFields = append(zapFields, zap.Any(key, value))
	}
	return zapFields
}

func ensureLogType(fields []zap.Field, defaultLogType string) []zap.Field {
	for _, field := range fields {
		if field.Key == "log_type" {
			return fields
		}
	}

	return append(fields, zap.String("log_type", defaultLogType))
}

func hasField(fields []zap.Field, key string) bool {
	for _, field := range fields {
		if field.Key == key {
			return true
		}
	}
	return false
}

func redirectStandardLog() {
	stdLogRedirectMu.Lock()
	defer stdLogRedirectMu.Unlock()

	if restoreStdLog != nil {
		restoreStdLog()
		restoreStdLog = nil
	}

	restore, err := zap.RedirectStdLogAt(L().With(zap.String("log_type", logTypeApp)), zapcore.InfoLevel)
	if err != nil {
		return
	}
	restoreStdLog = restore
	stdlog.SetFlags(0)
}

type stdLogWriter struct {
	logType string
	level   zapcore.Level
}

func (w *stdLogWriter) Write(p []byte) (int, error) {
	message := strings.TrimSpace(string(p))
	if message == "" {
		return len(p), nil
	}

	write(w.level, fallbackLogType(w.logType), message)
	return len(p), nil
}

func fallbackLogType(logType string) string {
	if strings.TrimSpace(logType) == "" {
		return logTypeApp
	}
	return filepath.Clean(logType)
}
