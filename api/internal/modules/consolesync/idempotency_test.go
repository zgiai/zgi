package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
)

// Same idempotency key twice: the second command id is refused by the ledger, the stored
// ACK payload comes back, and the first effect is the one that stands.
func TestApplyWithIdempotencyKeyIsAppliedOnce(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	ctx := context.Background()

	first := envelope(CommandAccountStatusSet, `{"status":"inactive"}`)
	first.IdempotencyKey = "org-status-1"
	firstResult, err := applier.Apply(ctx, ptrEnvelope(first))
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Console redelivered the same command under a fresh id, with the same key.
	replay := envelope(CommandAccountStatusSet, `{"status":"active"}`)
	replay.IdempotencyKey = "org-status-1"
	replayResult, err := applier.Apply(ctx, ptrEnvelope(replay))
	if !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("replay = %v, want ErrAlreadyApplied", err)
	}
	if replayResult == nil {
		t.Fatalf("replay result is nil, want the stored ACK payload")
	}
	if replayResult.After != firstResult.After || replayResult.Before != firstResult.Before {
		t.Fatalf("replay payload = %+v, want the stored %+v", replayResult, firstResult)
	}

	// The replay must not have executed: the status written by the first command stands.
	var status string
	if err := db.Table("organizations").Select("status").
		Where("id = ?", testOrganizationID).Scan(&status).Error; err != nil {
		t.Fatalf("read organization status: %v", err)
	}
	if status != "inactive" {
		t.Fatalf("organization status = %q, want inactive (the replay must not re-execute)", status)
	}

	var rows int64
	if err := db.Model(&PreAppliedCommand{}).
		Where("idempotency_key = ?", "org-status-1").Count(&rows).Error; err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("ledger rows for the key = %d, want 1", rows)
	}
}

// A failed apply must not own the idempotency key: the retry under the same key executes
// again, and only once that retry succeeds is a further replay short-circuited.
func TestApplyRetriesIdempotencyKeyAfterFailure(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	ctx := context.Background()

	failed := envelope(CommandAccountStatusSet, `{"status":"bogus"}`)
	failed.IdempotencyKey = "retry-key-1"
	if _, err := applier.Apply(ctx, ptrEnvelope(failed)); err == nil {
		t.Fatalf("an unsupported status must fail")
	}

	// Same key, fresh command id, now valid: the failed row must not short-circuit it.
	retry := envelope(CommandAccountStatusSet, `{"status":"active"}`)
	retry.IdempotencyKey = "retry-key-1"
	retryResult, err := applier.Apply(ctx, ptrEnvelope(retry))
	if err != nil {
		t.Fatalf("retry after a failed apply must execute: %v", err)
	}
	if retryResult == nil || retryResult.After == "" {
		t.Fatalf("retry result = %v, want the fresh payload", retryResult)
	}

	var status string
	if err := db.Table("organizations").Select("status").
		Where("id = ?", testOrganizationID).Scan(&status).Error; err != nil {
		t.Fatalf("read organization status: %v", err)
	}
	if status != "active" {
		t.Fatalf("organization status = %q, want active (the retry must execute)", status)
	}

	// The successful retry now owns the key: a further replay is a no-op with its payload.
	replay := envelope(CommandAccountStatusSet, `{"status":"inactive"}`)
	replay.IdempotencyKey = "retry-key-1"
	replayResult, err := applier.Apply(ctx, ptrEnvelope(replay))
	if !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("replay after a successful retry = %v, want ErrAlreadyApplied", err)
	}
	if replayResult == nil || replayResult.After != retryResult.After {
		t.Fatalf("replay result = %v, want the stored %q", replayResult, retryResult.After)
	}
	if err := db.Table("organizations").Select("status").
		Where("id = ?", testOrganizationID).Scan(&status).Error; err != nil {
		t.Fatalf("read organization status: %v", err)
	}
	if status != "active" {
		t.Fatalf("organization status = %q, want active (the replay must not re-execute)", status)
	}
}

// Different keys with the same payload are different commands: both execute.
func TestApplyWithDistinctIdempotencyKeysProcessesEach(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	ctx := context.Background()

	first := envelope(CommandAccountStatusSet, `{"status":"inactive"}`)
	first.IdempotencyKey = "distinct-key-1"
	if _, err := applier.Apply(ctx, ptrEnvelope(first)); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	second := envelope(CommandAccountStatusSet, `{"status":"active"}`)
	second.IdempotencyKey = "distinct-key-2"
	if _, err := applier.Apply(ctx, ptrEnvelope(second)); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	var status string
	if err := db.Table("organizations").Select("status").
		Where("id = ?", testOrganizationID).Scan(&status).Error; err != nil {
		t.Fatalf("read organization status: %v", err)
	}
	if status != "active" {
		t.Fatalf("organization status = %q, want active (the second key must execute)", status)
	}

	var keys []string
	if err := db.Model(&PreAppliedCommand{}).
		Where("idempotency_key IS NOT NULL").
		Order("idempotency_key").Pluck("idempotency_key", &keys).Error; err != nil {
		t.Fatalf("read ledger keys: %v", err)
	}
	if len(keys) != 2 || keys[0] != "distinct-key-1" || keys[1] != "distinct-key-2" {
		t.Fatalf("ledger keys = %v, want both distinct keys", keys)
	}
}

// Commands without a key keep their previous behaviour: they all execute, store NULL and
// never collide with each other or with keyed rows.
func TestApplyWithoutIdempotencyKeyKeepsWorking(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	ctx := context.Background()

	first := envelope(CommandAccountStatusSet, `{"status":"inactive"}`)
	if _, err := applier.Apply(ctx, ptrEnvelope(first)); err != nil {
		t.Fatalf("first keyless apply: %v", err)
	}

	// A repeated command id is still refused exactly as before.
	if _, err := applier.Apply(ctx, ptrEnvelope(first)); !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("repeated command id = %v, want ErrAlreadyApplied", err)
	}

	second := envelope(CommandAccountStatusSet, `{"status":"active"}`)
	if _, err := applier.Apply(ctx, ptrEnvelope(second)); err != nil {
		t.Fatalf("second keyless apply: %v", err)
	}

	var status string
	if err := db.Table("organizations").Select("status").
		Where("id = ?", testOrganizationID).Scan(&status).Error; err != nil {
		t.Fatalf("read organization status: %v", err)
	}
	if status != "active" {
		t.Fatalf("organization status = %q, want active", status)
	}

	var keyed, total int64
	if err := db.Model(&PreAppliedCommand{}).
		Where("idempotency_key IS NOT NULL").Count(&keyed).Error; err != nil {
		t.Fatalf("count keyed rows: %v", err)
	}
	if err := db.Model(&PreAppliedCommand{}).Count(&total).Error; err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if keyed != 0 || total != 2 {
		t.Fatalf("ledger keyed/total = %d/%d, want 0/2", keyed, total)
	}
}

// End to end through the sync pipeline: a replay under a fresh command id is ACKed as
// applied with the stored payload instead of being executed a second time.
func TestReplayedIdempotencyKeyAcksTheStoredResult(t *testing.T) {
	db := newConsolesyncTestDB(t)
	seedOrganization(t, db, "active")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	first := CommandEnvelope{
		ID:             uuid.NewString(),
		Seq:            1,
		Type:           CommandAccountStatusSet,
		Payload:        `{"status":"inactive"}`,
		IdempotencyKey: "pipeline-key-1",
	}
	replay := CommandEnvelope{
		ID:             uuid.NewString(),
		Seq:            2,
		Type:           CommandAccountStatusSet,
		Payload:        `{"status":"active"}`,
		IdempotencyKey: "pipeline-key-1",
	}
	commandsData, err := json.Marshal(PullCommandsResponse{
		Commands:       []CommandEnvelope{first, replay},
		DesiredVersion: 1,
	})
	if err != nil {
		t.Fatalf("marshal commands: %v", err)
	}

	calls := make(chan capturedCall, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- captureConsoleCall(r)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case apiPrefix + pathHeartbeat:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"desired_version":1}}`))
		case apiPrefix + pathCommands:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":` + string(commandsData) + `}`))
		case apiPrefix + pathDesiredState:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"version":1,"account_status":"active"}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
		}
	}))
	defer server.Close()

	service := NewService(db, Config{
		Enabled:        true,
		Edition:        "CLOUD",
		BaseURL:        server.URL,
		InstanceID:     "instance-1",
		InstanceSecret: "secret",
		Interval:       45 * time.Second,
		StaleAfter:     5 * time.Minute,
		HTTPTimeout:    5 * time.Second,
		ReconcileEvery: time.Minute,
	}, "test")
	if service == nil {
		t.Fatalf("NewService() returned nil for a valid cloud config")
	}
	t.Cleanup(func() {
		consolecred.Global().SetStale(false)
		consolecred.Global().Clear()
	})
	consolecred.Configure(service.cfg.InstanceID, service.cfg.InstanceSecret)

	if err := service.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() = %v", err)
	}

	firstAck := decodeAck(t, receiveCallForPath(t, calls, apiPrefix+pathCommands+"/"+first.ID+"/ack"))
	if firstAck.Status != StatusApplied {
		t.Fatalf("first ack status = %q, want applied", firstAck.Status)
	}
	replayAck := decodeAck(t, receiveCallForPath(t, calls, apiPrefix+pathCommands+"/"+replay.ID+"/ack"))
	if replayAck.Status != StatusApplied {
		t.Fatalf("replay ack status = %q, want applied", replayAck.Status)
	}
	if replayAck.After != firstAck.After {
		t.Fatalf("replay ack after = %q, want the stored %q", replayAck.After, firstAck.After)
	}

	var status string
	if err := db.Table("organizations").Select("status").
		Where("id = ?", testOrganizationID).Scan(&status).Error; err != nil {
		t.Fatalf("read organization status: %v", err)
	}
	if status != "inactive" {
		t.Fatalf("organization status = %q, want inactive (the replay must not re-execute)", status)
	}
}

func decodeAck(t *testing.T, call capturedCall) AckRequest {
	t.Helper()
	var ack AckRequest
	if err := json.Unmarshal(call.Body, &ack); err != nil {
		t.Fatalf("decode ack request %s: %v", call.Body, err)
	}
	return ack
}
