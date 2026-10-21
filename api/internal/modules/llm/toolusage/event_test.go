package toolusage

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var testOrganizationID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func newTestRepo(t *testing.T) *Repository {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&Event{}); err != nil {
		t.Fatal(err)
	}
	return NewRepository(db)
}
func testEvent() *Event {
	return &Event{OrganizationID: testOrganizationID, RequestID: "req", AttemptID: "attempt", EventID: "event", BillingLane: "private", Tool: ToolWebSearch, Quantity: 2, Unit: "call", Status: StatusSucceeded, UsageSource: UsageSourceProviderReported, DedupeKey: "trusted-key"}
}
func TestDeduplicationAndImmutableFacts(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	first, err := r.Record(ctx, testEvent())
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Record(ctx, testEvent())
	if err != nil || first.ID != second.ID {
		t.Fatalf("duplicate: %v", err)
	}
	conflict := testEvent()
	conflict.Quantity = 3
	if _, err := r.Record(ctx, conflict); !errors.Is(err, ErrFactConflict) {
		t.Fatalf("conflict: %v", err)
	}
	stale := testEvent()
	stale.Quantity = 0
	stale.Status = StatusPending
	stale.UsageSource = UsageSourceMissing
	stored, err := r.Record(ctx, stale)
	if err != nil || stored.Quantity != 2 || stored.Status != StatusSucceeded {
		t.Fatalf("stale changed fact: %+v %v", stored, err)
	}
	rows, err := r.ListByAttempt(ctx, testOrganizationID, "private", "attempt")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows: %v %v", rows, err)
	}
}
func TestPendingFactCanBeCompletedByAProviderObservation(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	pending := testEvent()
	pending.Quantity = 0
	pending.Status = StatusPending
	pending.UsageSource = UsageSourceMissing
	if _, err := r.Record(ctx, pending); err != nil {
		t.Fatal(err)
	}
	completed := testEvent()
	result, err := r.Record(ctx, completed)
	if err != nil || result.Quantity != 2 {
		t.Fatalf("delayed: %v %v", result, err)
	}
	other := testEvent()
	other.BillingLane = "platform"
	if _, err := r.Record(ctx, other); !errors.Is(err, ErrInvalidLane) {
		t.Fatalf("cross lane: %v", err)
	}
}
func TestTenantIsolation(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	first := testEvent()
	if _, err := r.Record(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := testEvent()
	second.OrganizationID = uuid.New()
	if _, err := r.Record(ctx, second); err != nil {
		t.Fatal(err)
	}
	for _, org := range []uuid.UUID{first.OrganizationID, second.OrganizationID} {
		rows, err := r.ListByAttempt(ctx, org, "private", "attempt")
		if err != nil || len(rows) != 1 {
			t.Fatalf("tenant leak: %v %v", rows, err)
		}
	}
}
func TestSessionReuseAndRetryKeys(t *testing.T) {
	first, _ := DedupeKey("provider", "channel", "credential", ToolCodeInterpreter, "request1", "call1", "session")
	reuse, _ := DedupeKey("provider", "channel", "credential", ToolCodeInterpreter, "request2", "call2", "session")
	if first != reuse {
		t.Fatal("session reuse changed identity")
	}
	otherChannel, _ := DedupeKey("provider", "other-channel", "credential", ToolCodeInterpreter, "request2", "call2", "session")
	otherCredential, _ := DedupeKey("provider", "channel", "other-credential", ToolCodeInterpreter, "request2", "call2", "session")
	if first == otherChannel || first == otherCredential {
		t.Fatal("session identity crossed channel or upstream account scope")
	}
	first, _ = DedupeKey("provider", "channel", "credential", ToolWebSearch, "request1", "call1", "")
	retry, _ := DedupeKey("provider", "channel", "credential", ToolWebSearch, "request2", "call2", "")
	if first == retry {
		t.Fatal("new upstream call deduplicated")
	}
}
func TestInvalidFacts(t *testing.T) {
	for _, change := range []func(*Event){func(e *Event) { e.Quantity = -1 }, func(e *Event) { e.Unit = "token" }, func(e *Event) { e.Status = StatusFailed }, func(e *Event) { e.UsageSource = UsageSourceMissing }, func(e *Event) { e.Tool = ToolCodeInterpreter; e.Unit = "session"; e.Quantity = 1 }} {
		e := testEvent()
		change(e)
		if e.Validate() == nil {
			t.Fatalf("invalid accepted: %+v", e)
		}
	}
}
