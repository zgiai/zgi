package toolusage

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func observerIdentity() Identity {
	return Identity{OrganizationID: testOrganizationID, RequestID: "req", AttemptID: "attempt", Lane: LanePrivate, ModelName: "test", Provider: "provider", ChannelID: uuid.MustParse("00000000-0000-0000-0000-000000000003")}
}
func TestObserverNeverCountsRequestedOrFunctionTools(t *testing.T) {
	o := NewObserver(observerIdentity(), []byte(`{"tools":[{"type":"web_search_preview"},{"type":"file_search"},{"type":"code_interpreter"}]}`))
	o.Observe([]byte(`{"id":"resp_1","output":[{"id":"fn_1","type":"function_call","name":"web_search","status":"completed"}]}`))
	events, err := o.Events(true, false)
	if err != nil || len(events) != 3 {
		t.Fatalf("events: %v %v", events, err)
	}
	for _, e := range events {
		if e.Quantity != 0 || e.UsageSource != UsageSourceMissing {
			t.Fatalf("fabricated usage: %+v", e)
		}
	}
}
func TestObserverUsesProviderCounterOnce(t *testing.T) {
	o := NewObserver(observerIdentity(), nil)
	body := []byte(`{"id":"resp_1","output":[{"id":"ws_1","type":"web_search_call","status":"completed"},{"id":"ws_2","type":"web_search_call","status":"completed"}],"tool_usage":{"web_search":{"num_requests":2}}}`)
	o.Observe(body)
	o.Observe(body)
	events, err := o.Events(true, false)
	if err != nil || len(events) != 1 || events[0].Quantity != 2 || events[0].UsageSource != UsageSourceProviderReported {
		t.Fatalf("counter: %+v %v", events, err)
	}
}
func TestObserverCountsUniqueCompletedSearchesAndFailures(t *testing.T) {
	o := NewObserver(observerIdentity(), nil)
	o.Observe([]byte(`{"type":"response.created","response":{"type":"response","id":"resp_1"}}`))
	for _, body := range []string{`{"item":{"id":"fs_1","type":"file_search_call","status":"completed"}}`, `{"item":{"id":"fs_1","type":"file_search_call","status":"completed"}}`, `{"item":{"id":"fs_2","type":"file_search_call","status":"completed"}}`, `{"item":{"id":"fs_failed","type":"file_search_call","status":"failed"}}`} {
		o.Observe([]byte(body))
	}
	events, err := o.Events(true, false)
	if err != nil || len(events) != 1 || events[0].Quantity != 2 {
		t.Fatalf("calls: %+v %v", events, err)
	}
}
func TestObserverDelayedMissingFactCanBeConfirmed(t *testing.T) {
	r := newTestRepo(t)
	o := NewObserver(observerIdentity(), []byte(`{"tools":[{"type":"web_search"}]}`))
	o.Observe([]byte(`{"id":"resp_1","type":"response"}`))
	if err := o.Record(context.Background(), r, true, false); err != nil {
		t.Fatal(err)
	}
	o.Observe([]byte(`{"id":"resp_1","type":"response","tool_usage":{"web_search":{"num_requests":3}}}`))
	if err := o.Record(context.Background(), r, true, false); err != nil {
		t.Fatal(err)
	}
	events, err := r.ListByAttempt(context.Background(), testOrganizationID, LanePrivate, "attempt")
	if err != nil || len(events) != 1 || events[0].Quantity != 3 {
		t.Fatalf("delayed: %+v %v", events, err)
	}
}
func TestObserverSessionReusePersistsOnce(t *testing.T) {
	r := newTestRepo(t)
	for _, body := range []string{`{"id":"resp_1","output":[{"id":"ci_1","type":"code_interpreter_call","status":"completed","container_id":"session-1"}]}`, `{"id":"resp_2","output":[{"id":"ci_2","type":"code_interpreter_call","status":"completed","container_id":"session-1"}]}`} {
		o := NewObserver(observerIdentity(), nil)
		o.Observe([]byte(body))
		if err := o.Record(context.Background(), r, true, false); err != nil {
			t.Fatal(err)
		}
	}
	events, err := r.ListByAttempt(context.Background(), testOrganizationID, LanePrivate, "attempt")
	if err != nil || len(events) != 1 || events[0].Quantity != 1 {
		t.Fatalf("session: %+v %v", events, err)
	}
}
func TestStreamObserverPreservesBytesAndRecordsFinalFacts(t *testing.T) {
	o := NewObserver(observerIdentity(), nil)
	input := "event: response.created\ndata: {\"response\":{\"type\":\"response\",\"id\":\"resp_1\"}}\n\nevent: response.completed\ndata: {\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"web_search_call\",\"id\":\"ws_1\",\"status\":\"completed\"}]}}\n\n"
	finished := 0
	body := WrapStream(io.NopCloser(strings.NewReader(input)), o, func(failed bool) {
		if failed {
			t.Error("clean stream failed")
		}
		finished++
	})
	output, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(output) != input || finished != 1 {
		t.Fatalf("stream: %v %d", err, finished)
	}
	events, err := o.Events(true, false)
	if err != nil || len(events) != 1 || events[0].Quantity != 1 {
		t.Fatalf("stream events: %+v %v", events, err)
	}
}

func TestPendingExpiryAndLateFact(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	o := NewObserver(observerIdentity(), []byte(`{"tools":[{"type":"web_search"}]}`))
	o.Observe([]byte(`{"id":"resp_expire","type":"response"}`))
	if err := o.Record(ctx, r, true, false); err != nil {
		t.Fatal(err)
	}
	n, err := r.ExpirePending(ctx, time.Now().Add(time.Second))
	if err != nil || n != 1 {
		t.Fatalf("expire: %d %v", n, err)
	}
	events, _ := r.ListByAttempt(ctx, testOrganizationID, ownedLane, "attempt")
	if events[0].Status != StatusFailed || events[0].Quantity != 0 {
		t.Fatalf("expired: %+v", events)
	}
	o.Observe([]byte(`{"id":"resp_expire","type":"response","tool_usage":{"web_search":{"num_requests":2}}}`))
	if err := o.Record(ctx, r, true, false); err != nil {
		t.Fatal(err)
	}
	events, _ = r.ListByAttempt(ctx, testOrganizationID, ownedLane, "attempt")
	if len(events) != 1 || events[0].Quantity != 2 || events[0].Status != StatusSucceeded {
		t.Fatalf("late fact: %+v", events)
	}
}

func TestFailedCodeSessionIsFinalAndFree(t *testing.T) {
	o := NewObserver(observerIdentity(), nil)
	o.Observe([]byte(`{"id":"resp_fail","output":[{"id":"ci_failed","type":"code_interpreter_call","status":"failed"}]}`))
	events, err := o.Events(true, false)
	if err != nil || len(events) != 1 || events[0].Quantity != 0 || events[0].Status != StatusFailed {
		t.Fatalf("failed code: %+v %v", events, err)
	}
}

func TestSameSessionAcrossChannelsDoesNotCreateSecondFact(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		identity := observerIdentity()
		identity.ChannelID = uuid.New()
		identity.RequestID = uuid.NewString()
		o := NewObserver(identity, nil)
		o.Observe([]byte(`{"id":"resp_channel","output":[{"id":"ci_session","type":"code_interpreter_call","status":"completed","container_id":"shared_session"}]}`))
		if err := o.Record(ctx, r, true, false); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := r.db.Model(&Event{}).Where("session_id = ?", "shared_session").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("cross-channel session: %d %v", count, err)
	}
}
