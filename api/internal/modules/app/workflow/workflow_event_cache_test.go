package workflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestWorkflowCommittedTailReadsContinuousCommittedEvents(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	previous := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() {
		redisutil.SetClient(previous)
		_ = client.Close()
	})

	events := []*workflowpause.RunEventPayload{
		{EventID: "event-1", Sequence: 1, Event: "node_started", SchemaVersion: 2, Data: map[string]interface{}{"node_id": "a"}},
		{EventID: "event-2", Sequence: 2, Event: "node_finished", SchemaVersion: 2, Data: map[string]interface{}{"node_id": "a"}},
	}
	request, ok := prepareWorkflowCommittedTailPublish("run-cache", events)
	if !ok {
		t.Fatal("committed tail request was not prepared")
	}
	publishWorkflowCommittedTailNow(request)

	stored, hit := readWorkflowCommittedTailAfter(context.Background(), "run-cache", 0, 10)
	if !hit || len(stored) != 2 {
		t.Fatalf("cache hit=%v events=%d, want true/2", hit, len(stored))
	}
	if stored[0].Sequence != 1 || stored[1].Sequence != 2 {
		t.Fatalf("cached sequences = %d,%d", stored[0].Sequence, stored[1].Sequence)
	}
}

func TestWorkflowCommittedTailRejectsGap(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	previous := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() {
		redisutil.SetClient(previous)
		_ = client.Close()
	})

	request, ok := prepareWorkflowCommittedTailPublish("run-gap", []*workflowpause.RunEventPayload{
		{EventID: "event-1", Sequence: 1, Event: "node_started", SchemaVersion: 2},
		{EventID: "event-3", Sequence: 3, Event: "node_finished", SchemaVersion: 2},
	})
	if !ok {
		t.Fatal("committed tail request was not prepared")
	}
	publishWorkflowCommittedTailNow(request)
	if events, hit := readWorkflowCommittedTailAfter(context.Background(), "run-gap", 0, 10); hit || events != nil {
		t.Fatalf("gap cache result = %#v hit=%v, want database fallback", events, hit)
	}
}

func TestWorkflowCommittedTailKeepsFirstPayloadForSameSequence(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	previous := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() {
		redisutil.SetClient(previous)
		_ = client.Close()
	})

	first, ok := prepareWorkflowCommittedTailPublish("run-replace", []*workflowpause.RunEventPayload{
		{EventID: "event-1", Sequence: 1, Event: "node_started", SchemaVersion: 2, RecordedAtMS: 100},
	})
	if !ok {
		t.Fatal("first committed tail request was not prepared")
	}
	publishWorkflowCommittedTailNow(first)
	second, ok := prepareWorkflowCommittedTailPublish("run-replace", []*workflowpause.RunEventPayload{
		{EventID: "event-1", Sequence: 1, Event: "node_started", SchemaVersion: 2, RecordedAtMS: 200},
		{EventID: "event-2", Sequence: 2, Event: "node_finished", SchemaVersion: 2, RecordedAtMS: 201},
	})
	if !ok {
		t.Fatal("second committed tail request was not prepared")
	}
	publishWorkflowCommittedTailNow(second)

	if count := client.ZCard(context.Background(), workflowCommittedTailKey("run-replace")).Val(); count != 2 {
		t.Fatalf("committed tail cardinality = %d, want one member per sequence", count)
	}
	stored, hit := readWorkflowCommittedTailAfter(context.Background(), "run-replace", 0, 10)
	if !hit || len(stored) != 2 || stored[0].RecordedAtMS != 100 {
		t.Fatalf("cache hit=%v events=%#v, want immutable first payload followed by sequence 2", hit, stored)
	}
}

func TestWorkflowCommittedTailReadsLegacyDuplicateSequences(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	previous := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() {
		redisutil.SetClient(previous)
		_ = client.Close()
	})

	key := workflowCommittedTailKey("run-legacy-duplicates")
	for _, event := range []workflowpause.RunEventPayload{
		{EventID: "event-1", Sequence: 1, Event: "node_started", SchemaVersion: 2, RecordedAtMS: 100},
		{EventID: "event-1", Sequence: 1, Event: "node_started", SchemaVersion: 2, RecordedAtMS: 200},
		{EventID: "event-2", Sequence: 2, Event: "node_finished", SchemaVersion: 2, RecordedAtMS: 201},
	} {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal legacy event: %v", err)
		}
		if err := client.ZAdd(context.Background(), key, goredis.Z{Score: float64(event.Sequence), Member: string(payload)}).Err(); err != nil {
			t.Fatalf("seed legacy committed tail: %v", err)
		}
	}

	stored, hit := readWorkflowCommittedTailAfter(context.Background(), "run-legacy-duplicates", 0, 10)
	if !hit || len(stored) != 2 || stored[0].Sequence != 1 || stored[1].Sequence != 2 {
		t.Fatalf("legacy duplicate cache hit=%v events=%#v, want continuous deduplicated tail", hit, stored)
	}
}

func TestWorkflowCommittedTailWindowHydratesAdjacentLifecycleEvents(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	previous := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() {
		redisutil.SetClient(previous)
		_ = client.Close()
	})

	db := openWorkflowStopV2TestDB(t)
	if err := db.AutoMigrate(&workflowpause.RunEvent{}); err != nil {
		t.Fatalf("migrate workflow events: %v", err)
	}
	now := time.Now()
	for sequence, eventType := range map[int]string{
		7: workflowpause.EventWorkflowPaused,
		8: workflowpause.EventApprovalResultFilled,
		9: workflowpause.EventWorkflowResumed,
	} {
		if err := db.Create(&workflowpause.RunEvent{
			ID: "event-window-" + eventType, TenantID: "tenant-window", AppID: "app-window",
			WorkflowRunID: "run-window", Sequence: sequence, EventType: eventType,
			EventData: "{}", SchemaVersion: 2, Category: workflowpause.EventCategoryControl,
			CreatedAt: now, OccurredAt: now,
		}).Error; err != nil {
			t.Fatalf("create workflow event %d: %v", sequence, err)
		}
	}

	publishWorkflowCommittedTailWindow(context.Background(), workflowpause.NewService(db), "tenant-window", "run-window", 9)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		stored, hit := readWorkflowCommittedTailAfter(context.Background(), "run-window", 6, 10)
		if hit && len(stored) == 3 {
			if stored[0].Sequence != 7 || stored[1].Sequence != 8 || stored[2].Sequence != 9 {
				t.Fatalf("cached lifecycle sequences = %#v", stored)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("committed lifecycle window was not published to Redis")
}
