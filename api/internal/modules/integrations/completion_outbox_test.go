package integrations

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisExecutionCompletionOutboxRoundTripHasNoTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	outbox := NewRedisExecutionCompletionOutbox(client)
	executionID := uuid.New()
	cost := 0.007
	want := PendingExecutionCompletion{
		ExecutionID: executionID,
		Completion: ExecutionCompletion{
			Status:            "succeeded",
			ProviderRequestID: "exa-request",
			DurationMS:        42,
			CostUSD:           &cost,
			ResultCount:       3,
			AttemptCount:      2,
		},
	}
	if err := outbox.Enqueue(context.Background(), want); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	ttl, err := client.TTL(context.Background(), completionOutboxPayloadKey(executionID)).Result()
	if err != nil {
		t.Fatalf("TTL() error = %v", err)
	}
	if ttl != -1 {
		t.Fatalf("payload TTL = %v, want no expiration", ttl)
	}

	claim, err := outbox.Claim(context.Background(), 10)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if claim.ClaimedCount != 1 || len(claim.Items) != 1 || claim.Items[0].ExecutionID != executionID || claim.Items[0].Completion.ProviderRequestID != "exa-request" || claim.Items[0].Completion.CostUSD == nil || *claim.Items[0].Completion.CostUSD != cost {
		t.Fatalf("Claim() = %#v", claim)
	}
	if err := outbox.Delete(context.Background(), executionID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	claim, err = outbox.Claim(context.Background(), 10)
	if err != nil || claim.ClaimedCount != 0 || len(claim.Items) != 0 {
		t.Fatalf("Claim() after delete = %#v, %v", claim, err)
	}
}

func TestRedisExecutionCompletionOutboxClaimUsesLease(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	outbox := NewRedisExecutionCompletionOutbox(client)
	redisNow := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	server.SetTime(redisNow)
	outbox.leaseDuration = time.Minute
	executionID := uuid.New()
	if err := outbox.Enqueue(context.Background(), PendingExecutionCompletion{
		ExecutionID: executionID,
		Completion:  ExecutionCompletion{Status: "succeeded"},
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	first, err := outbox.Claim(context.Background(), 1)
	if err != nil || first.ClaimedCount != 1 || len(first.Items) != 1 {
		t.Fatalf("first Claim() = %#v, %v", first, err)
	}
	second, err := outbox.Claim(context.Background(), 1)
	if err != nil || second.ClaimedCount != 0 {
		t.Fatalf("second Claim() before lease expiry = %#v, %v", second, err)
	}

	server.SetTime(redisNow.Add(time.Minute + time.Millisecond))
	reclaimed, err := outbox.Claim(context.Background(), 1)
	if err != nil || reclaimed.ClaimedCount != 1 || len(reclaimed.Items) != 1 || reclaimed.Items[0].ExecutionID != executionID {
		t.Fatalf("Claim() after lease expiry = %#v, %v", reclaimed, err)
	}
}

func TestRedisExecutionCompletionOutboxLeaseIgnoresAPIInstanceClockSkew(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	firstOutbox := NewRedisExecutionCompletionOutbox(client)
	secondOutbox := NewRedisExecutionCompletionOutbox(client)
	base := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	firstOutbox.now = func() time.Time { return base }
	secondOutbox.now = func() time.Time { return base.Add(24 * time.Hour) }
	executionID := uuid.New()
	if err := firstOutbox.Enqueue(context.Background(), PendingExecutionCompletion{
		ExecutionID: executionID,
		Completion:  ExecutionCompletion{Status: "succeeded"},
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	first, err := firstOutbox.Claim(context.Background(), 1)
	if err != nil || first.ClaimedCount != 1 {
		t.Fatalf("first Claim() = %#v, %v", first, err)
	}
	second, err := secondOutbox.Claim(context.Background(), 1)
	if err != nil || second.ClaimedCount != 0 {
		t.Fatalf("clock-skewed second Claim() = %#v, %v", second, err)
	}
}

func TestRedisExecutionCompletionOutboxClaimIsExclusiveAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	firstOutbox := NewRedisExecutionCompletionOutbox(client)
	secondOutbox := NewRedisExecutionCompletionOutbox(client)
	executionID := uuid.New()
	if err := firstOutbox.Enqueue(context.Background(), PendingExecutionCompletion{
		ExecutionID: executionID,
		Completion:  ExecutionCompletion{Status: "succeeded"},
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan ExecutionCompletionClaim, 2)
	errorsCh := make(chan error, 2)
	for _, outbox := range []*RedisExecutionCompletionOutbox{firstOutbox, secondOutbox} {
		wg.Add(1)
		go func(outbox *RedisExecutionCompletionOutbox) {
			defer wg.Done()
			claim, err := outbox.Claim(context.Background(), 1)
			results <- claim
			errorsCh <- err
		}(outbox)
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
	}
	claimed := 0
	for result := range results {
		claimed += result.ClaimedCount
	}
	if claimed != 1 {
		t.Fatalf("claimed count across instances = %d, want 1", claimed)
	}
}

func TestRedisExecutionCompletionOutboxDeadLettersCorruptPayload(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	outbox := NewRedisExecutionCompletionOutbox(client)
	executionID := uuid.New()
	ctx := context.Background()
	if err := client.Set(ctx, completionOutboxPayloadKey(executionID), "not-json", 0).Err(); err != nil {
		t.Fatalf("seed payload: %v", err)
	}
	if err := client.ZAdd(ctx, completionOutboxPendingKey, redis.Z{Member: executionID.String()}).Err(); err != nil {
		t.Fatalf("seed pending index: %v", err)
	}

	claim, err := outbox.Claim(ctx, 1)
	if err == nil || !strings.Contains(err.Error(), "decode integration audit completion") {
		t.Fatalf("Claim() error = %v", err)
	}
	if claim.ClaimedCount != 1 || len(claim.Items) != 0 {
		t.Fatalf("Claim() = %#v", claim)
	}
	deadLetter, err := client.HGet(ctx, completionOutboxDeadLetterKey, executionID.String()).Result()
	if err != nil || !strings.Contains(deadLetter, "payload is not valid JSON") || !strings.Contains(deadLetter, "not-json") {
		t.Fatalf("dead letter = %q, %v", deadLetter, err)
	}
	if exists := client.Exists(ctx, completionOutboxPayloadKey(executionID)).Val(); exists != 0 {
		t.Fatalf("corrupt payload still exists")
	}
}

func TestExecutorReconcilesPersistedCompletion(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	outbox := NewRedisExecutionCompletionOutbox(client)
	executionID := uuid.New()
	completion := ExecutionCompletion{Status: "failed", ErrorCode: ErrorCodeTimeout, DurationMS: 20, AttemptCount: 3}
	if err := outbox.Enqueue(context.Background(), PendingExecutionCompletion{ExecutionID: executionID, Completion: completion}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	audit := &testAudit{}
	executor := NewExecutor(NewRegistry(), audit, &testQuota{}, nil, []byte("audit-key"), time.Second).WithCompletionOutbox(outbox)
	claimed, err := executor.reconcilePendingCompletions(context.Background(), 10)
	if err != nil {
		t.Fatalf("reconcilePendingCompletions() error = %v", err)
	}
	if claimed != 1 {
		t.Fatalf("reconcilePendingCompletions() claimed = %d", claimed)
	}
	if audit.completeCalls != 1 || audit.completion.ErrorCode != ErrorCodeTimeout {
		t.Fatalf("reconciled completion = %#v", audit.completion)
	}
	claim, err := outbox.Claim(context.Background(), 10)
	if err != nil || claim.ClaimedCount != 0 {
		t.Fatalf("pending after reconciliation = %#v, %v", claim, err)
	}
}

func TestExecutorReconcilesValidCompletionWhenClaimAlsoDeadLettersCorruption(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	outbox := NewRedisExecutionCompletionOutbox(client)
	validID := uuid.New()
	if err := outbox.Enqueue(context.Background(), PendingExecutionCompletion{
		ExecutionID: validID,
		Completion:  ExecutionCompletion{Status: "succeeded"},
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	corruptID := uuid.New()
	ctx := context.Background()
	if err := client.Set(ctx, completionOutboxPayloadKey(corruptID), "not-json", 0).Err(); err != nil {
		t.Fatalf("seed corrupt payload: %v", err)
	}
	if err := client.ZAdd(ctx, completionOutboxPendingKey, redis.Z{Score: -1, Member: corruptID.String()}).Err(); err != nil {
		t.Fatalf("seed corrupt index: %v", err)
	}

	audit := &testAudit{}
	executor := NewExecutor(NewRegistry(), audit, &testQuota{}, nil, []byte("audit-key"), time.Second).WithCompletionOutbox(outbox)
	claimed, err := executor.reconcilePendingCompletions(ctx, 10)
	if err == nil || claimed != 2 {
		t.Fatalf("reconcilePendingCompletions() = %d, %v", claimed, err)
	}
	if audit.completeCalls != 1 || audit.completedID != validID {
		t.Fatalf("valid completion was not reconciled: calls=%d id=%s", audit.completeCalls, audit.completedID)
	}
}

type completionCountingAudit struct {
	mu        sync.Mutex
	completed map[uuid.UUID]ExecutionCompletion
}

func (a *completionCountingAudit) Create(context.Context, *ExecutionRecord) error { return nil }

func (a *completionCountingAudit) Complete(_ context.Context, id uuid.UUID, completion ExecutionCompletion) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.completed == nil {
		a.completed = make(map[uuid.UUID]ExecutionCompletion)
	}
	a.completed[id] = completion
	return nil
}

func (a *completionCountingAudit) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.completed)
}

func TestExecutorCompletionRecoveryDrainsMoreThanOneBatch(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	outbox := NewRedisExecutionCompletionOutbox(client)
	for i := 0; i < completionRecoveryBatchSize+7; i++ {
		if err := outbox.Enqueue(context.Background(), PendingExecutionCompletion{
			ExecutionID: uuid.New(),
			Completion:  ExecutionCompletion{Status: "succeeded", ResultCount: i},
		}); err != nil {
			t.Fatalf("Enqueue(%d) error = %v", i, err)
		}
	}
	audit := &completionCountingAudit{}
	executor := NewExecutor(NewRegistry(), audit, &testQuota{}, nil, []byte("audit-key"), time.Second).WithCompletionOutbox(outbox)

	if err := executor.drainPendingCompletions(context.Background()); err != nil {
		t.Fatalf("drainPendingCompletions() error = %v", err)
	}
	if got := audit.count(); got != completionRecoveryBatchSize+7 {
		t.Fatalf("completed count = %d, want %d", got, completionRecoveryBatchSize+7)
	}
	claim, err := outbox.Claim(context.Background(), 1)
	if err != nil || claim.ClaimedCount != 0 {
		t.Fatalf("remaining completion = %#v, %v", claim, err)
	}
}
