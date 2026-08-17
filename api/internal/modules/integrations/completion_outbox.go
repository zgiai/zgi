package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

const (
	completionOutboxPendingKey    = "zgi:integration:audit-completion:pending"
	completionOutboxProcessingKey = "zgi:integration:audit-completion:processing"
	completionOutboxDeadLetterKey = "zgi:integration:audit-completion:dead-letter"
	completionOutboxKeyBase       = "zgi:integration:audit-completion:"
	completionOutboxSchema        = "zgi.integration_audit_completion.v1"
	completionOutboxLease         = 30 * time.Second
)

var claimExecutionCompletionsScript = redis.NewScript(`
local pending = KEYS[1]
local processing = KEYS[2]
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)
local lease_until = now + tonumber(ARGV[1])
local limit = tonumber(ARGV[2])

local expired = redis.call('ZRANGEBYSCORE', processing, '-inf', now, 'LIMIT', 0, limit)
for _, id in ipairs(expired) do
  redis.call('ZREM', processing, id)
  redis.call('ZADD', pending, now, id)
end

local candidates = redis.call('ZRANGE', pending, 0, limit - 1)
local claimed = {}
for _, id in ipairs(candidates) do
  if redis.call('ZREM', pending, id) == 1 then
    redis.call('ZADD', processing, lease_until, id)
    table.insert(claimed, id)
  end
end
return claimed
`)

var enqueueExecutionCompletionScript = redis.NewScript(`
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)
redis.call('SET', KEYS[1], ARGV[2])
redis.call('ZREM', KEYS[3], ARGV[1])
redis.call('ZADD', KEYS[2], now, ARGV[1])
return 1
`)

type completionOutboxPayload struct {
	SchemaVersion string              `json:"schema_version"`
	Completion    ExecutionCompletion `json:"completion"`
}

type completionDeadLetter struct {
	FailedAt string `json:"failed_at"`
	Reason   string `json:"reason"`
	Payload  string `json:"payload,omitempty"`
}

type PendingExecutionCompletion struct {
	ExecutionID uuid.UUID
	Completion  ExecutionCompletion
}

type ExecutionCompletionClaim struct {
	Items        []PendingExecutionCompletion
	ClaimedCount int
}

type ExecutionCompletionOutbox interface {
	Enqueue(ctx context.Context, pending PendingExecutionCompletion) error
	Claim(ctx context.Context, limit int64) (ExecutionCompletionClaim, error)
	Delete(ctx context.Context, executionID uuid.UUID) error
}

type RedisExecutionCompletionOutbox struct {
	client        *redis.Client
	now           func() time.Time
	leaseDuration time.Duration
}

func NewRedisExecutionCompletionOutbox(client *redis.Client) *RedisExecutionCompletionOutbox {
	return &RedisExecutionCompletionOutbox{
		client:        client,
		now:           time.Now,
		leaseDuration: completionOutboxLease,
	}
}

func (o *RedisExecutionCompletionOutbox) Enqueue(ctx context.Context, pending PendingExecutionCompletion) error {
	if o == nil || o.client == nil {
		return fmt.Errorf("integration audit completion outbox is unavailable")
	}
	if pending.ExecutionID == uuid.Nil {
		return fmt.Errorf("integration audit completion execution id is required")
	}
	payload, err := json.Marshal(completionOutboxPayload{SchemaVersion: completionOutboxSchema, Completion: pending.Completion})
	if err != nil {
		return fmt.Errorf("encode integration audit completion: %w", err)
	}
	_, err = enqueueExecutionCompletionScript.Run(
		ctx,
		o.client,
		[]string{completionOutboxPayloadKey(pending.ExecutionID), completionOutboxPendingKey, completionOutboxProcessingKey},
		pending.ExecutionID.String(),
		payload,
	).Result()
	if err != nil {
		return fmt.Errorf("enqueue integration audit completion: %w", err)
	}
	return nil
}

func (o *RedisExecutionCompletionOutbox) Claim(ctx context.Context, limit int64) (ExecutionCompletionClaim, error) {
	if o == nil || o.client == nil {
		return ExecutionCompletionClaim{}, fmt.Errorf("integration audit completion outbox is unavailable")
	}
	if limit <= 0 {
		return ExecutionCompletionClaim{}, nil
	}
	leaseDuration := o.leaseDuration
	if leaseDuration <= 0 {
		leaseDuration = completionOutboxLease
	}
	ids, err := claimExecutionCompletionsScript.Run(
		ctx,
		o.client,
		[]string{completionOutboxPendingKey, completionOutboxProcessingKey},
		leaseDuration.Milliseconds(),
		limit,
	).StringSlice()
	if err != nil {
		return ExecutionCompletionClaim{}, fmt.Errorf("claim integration audit completions: %w", err)
	}

	claim := ExecutionCompletionClaim{
		Items:        make([]PendingExecutionCompletion, 0, len(ids)),
		ClaimedCount: len(ids),
	}
	var claimErr error
	for _, rawID := range ids {
		executionID, parseErr := uuid.Parse(strings.TrimSpace(rawID))
		if parseErr != nil {
			deadLetterErr := o.deadLetter(ctx, rawID, nil, "invalid execution id")
			claimErr = errors.Join(claimErr, fmt.Errorf("decode integration audit completion %q: %w", rawID, parseErr), deadLetterErr)
			continue
		}
		payload, getErr := o.client.Get(ctx, completionOutboxPayloadKey(executionID)).Bytes()
		if getErr == redis.Nil {
			deadLetterErr := o.deadLetter(ctx, rawID, nil, "payload is missing")
			claimErr = errors.Join(claimErr, fmt.Errorf("decode integration audit completion %s: payload is missing", rawID), deadLetterErr)
			continue
		}
		if getErr != nil {
			claimErr = errors.Join(claimErr, fmt.Errorf("read integration audit completion %s: %w", rawID, getErr))
			continue
		}
		var decoded completionOutboxPayload
		if decodeErr := json.Unmarshal(payload, &decoded); decodeErr != nil {
			deadLetterErr := o.deadLetter(ctx, rawID, payload, "payload is not valid JSON")
			claimErr = errors.Join(claimErr, fmt.Errorf("decode integration audit completion %s: %w", rawID, decodeErr), deadLetterErr)
			continue
		}
		if decoded.SchemaVersion != completionOutboxSchema {
			deadLetterErr := o.deadLetter(ctx, rawID, payload, "unsupported schema version")
			claimErr = errors.Join(claimErr, fmt.Errorf("decode integration audit completion %s: unsupported schema version %q", rawID, decoded.SchemaVersion), deadLetterErr)
			continue
		}
		claim.Items = append(claim.Items, PendingExecutionCompletion{ExecutionID: executionID, Completion: decoded.Completion})
	}
	return claim, claimErr
}

func (o *RedisExecutionCompletionOutbox) Delete(ctx context.Context, executionID uuid.UUID) error {
	if o == nil || o.client == nil {
		return fmt.Errorf("integration audit completion outbox is unavailable")
	}
	_, err := o.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, completionOutboxPayloadKey(executionID))
		pipe.ZRem(ctx, completionOutboxPendingKey, executionID.String())
		pipe.ZRem(ctx, completionOutboxProcessingKey, executionID.String())
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete integration audit completion: %w", err)
	}
	return nil
}

func (o *RedisExecutionCompletionOutbox) deadLetter(ctx context.Context, rawID string, payload []byte, reason string) error {
	record, err := json.Marshal(completionDeadLetter{
		FailedAt: o.now().UTC().Format(time.RFC3339Nano),
		Reason:   reason,
		Payload:  string(payload),
	})
	if err != nil {
		return fmt.Errorf("encode integration audit completion dead letter: %w", err)
	}
	_, err = o.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, completionOutboxDeadLetterKey, rawID, record)
		pipe.Del(ctx, completionOutboxKeyBase+rawID)
		pipe.ZRem(ctx, completionOutboxPendingKey, rawID)
		pipe.ZRem(ctx, completionOutboxProcessingKey, rawID)
		return nil
	})
	if err != nil {
		return fmt.Errorf("dead-letter integration audit completion %q: %w", rawID, err)
	}
	return nil
}

func completionOutboxPayloadKey(executionID uuid.UUID) string {
	return completionOutboxKeyBase + executionID.String()
}
