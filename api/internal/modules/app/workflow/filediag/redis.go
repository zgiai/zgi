package filediag

import (
	"context"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/pkg/logger"
	zredis "github.com/zgiai/zgi/api/pkg/redis"
	"go.uber.org/zap"
)

const (
	redisStreamKey     = "zgi:workflow:file_diagnostics"
	redisStreamMaxLen  = 10000
	redisStreamTTL     = 7 * 24 * time.Hour
	redisWriteTimeout  = 100 * time.Millisecond
	redisTimestampUnit = int64(time.Millisecond)
)

// AppendError records workflow file diagnostics in Redis when Redis is available.
// It is intentionally best-effort and must never block or fail workflow execution.
func AppendError(_ context.Context, event string, message string, fields map[string]string) {
	client := zredis.GetClient()
	if client == nil {
		return
	}

	now := time.Now()
	values := map[string]interface{}{
		"event":        event,
		"message":      message,
		"timestamp":    now.UTC().Format(time.RFC3339Nano),
		"timestamp_ms": strconv.FormatInt(now.UnixNano()/redisTimestampUnit, 10),
	}
	for key, value := range fields {
		values[key] = value
	}

	go appendErrorValues(client, event, values)
}

func appendErrorValues(client *goredis.Client, event string, values map[string]interface{}) {
	writeCtx, cancel := context.WithTimeout(context.Background(), redisWriteTimeout)
	defer cancel()

	_, err := client.Pipelined(writeCtx, func(pipe goredis.Pipeliner) error {
		pipe.XAdd(writeCtx, &goredis.XAddArgs{
			Stream: redisStreamKey,
			MaxLen: redisStreamMaxLen,
			Approx: true,
			Values: values,
		})
		pipe.Expire(writeCtx, redisStreamKey, redisStreamTTL)
		return nil
	})
	if err != nil {
		logger.Debug("failed to append workflow file diagnostic to Redis",
			zap.String("event", event),
			zap.Error(err),
		)
	}
}
