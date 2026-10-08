package platformrelay

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func newTestBroker(t *testing.T) (*Broker, string) {
	t.Helper()
	broker := New()
	t.Cleanup(broker.Close)
	token, err := broker.OpenJob(t.Context(), "job-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return broker, token
}

func request() agentexecution.PlatformRequest {
	return agentexecution.PlatformRequest{ID: "r1", Action: "file.export", Arguments: json.RawMessage(`{"path":"out.txt"}`)}
}

func TestJobGrantIsolationAndClose(t *testing.T) {
	broker, token := newTestBroker(t)
	if _, err := broker.Submit("job-2", token, request()); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("cross Job accepted: %v", err)
	}
	if _, err := broker.Submit("job-1", strings.Repeat("x", 43), request()); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("wrong token accepted: %v", err)
	}
	if _, err := broker.Submit("job-1", token, request()); err != nil {
		t.Fatal(err)
	}
	broker.CloseJob("job-1")
	if _, err := broker.Receipt("job-1", token, "r1"); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("closed Job accepted: %v", err)
	}
	newToken, err := broker.OpenJob(t.Context(), "job-1", time.Minute)
	if err != nil || newToken == token {
		t.Fatalf("new grant err=%v same token=%v", err, newToken == token)
	}
	if _, err := broker.Submit("job-1", token, request()); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("old token accepted after reconstruction: %v", err)
	}
}

func TestDuplicateAndResponseDelivery(t *testing.T) {
	broker, token := newTestBroker(t)
	var group sync.WaitGroup
	errorsFound := make(chan error, 8)
	for range 8 {
		group.Go(func() {
			_, err := broker.Submit("job-1", token, request())
			if err != nil {
				errorsFound <- err
			}
		})
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	pending, err := broker.Pending("job-1")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	pending[0].Arguments[0] = 'x'
	if again, err := broker.Pending("job-1"); err != nil || len(again) != 0 {
		t.Fatalf("indeterminate claim replayed: %+v err=%v", again, err)
	}
	if receipt, err := broker.Receipt("job-1", token, "r1"); err != nil || receipt.Status != "running" {
		t.Fatalf("claim status=%+v err=%v", receipt, err)
	}
	response := agentexecution.PlatformResponse{ID: "r1", Status: "completed", Data: json.RawMessage(`{"file_id":"file-1"}`)}
	if err := broker.Resolve("job-1", response); err != nil {
		t.Fatal(err)
	}
	if err := broker.Resolve("job-1", response); err != nil {
		t.Fatal(err)
	}
	got, err := broker.Submit("job-1", token, request())
	if err != nil || got.Status != "completed" || string(got.Data) != string(response.Data) {
		t.Fatalf("response=%+v err=%v", got, err)
	}
	got.Data[0] = 'x'
	again, err := broker.Receipt("job-1", token, "r1")
	if err != nil || string(again.Data) != string(response.Data) {
		t.Fatalf("receipt ownership=%+v err=%v", again, err)
	}
	if pending, err := broker.Pending("job-1"); err != nil || len(pending) != 0 {
		t.Fatalf("completed request requeued: %+v err=%v", pending, err)
	}
	changed := request()
	changed.Arguments = json.RawMessage(`{"path":"other.txt"}`)
	if _, err := broker.Submit("job-1", token, changed); !errors.Is(err, agentexecution.ErrIdempotencyConflict) {
		t.Fatalf("changed request accepted: %v", err)
	}
	response.Data = json.RawMessage(`{"file_id":"other"}`)
	if err := broker.Resolve("job-1", response); !errors.Is(err, agentexecution.ErrIdempotencyConflict) {
		t.Fatalf("changed result accepted: %v", err)
	}
}

func TestBrokerCloseCannotReopen(t *testing.T) {
	broker, _ := newTestBroker(t)
	broker.Close()
	if _, err := broker.OpenJob(t.Context(), "new-job", time.Minute); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("closed broker admitted job: %v", err)
	}
}

func TestGrantCancellationAndRequestBounds(t *testing.T) {
	broker, token := newTestBroker(t)
	for i := range MaxRequests {
		r := request()
		r.ID = strings.Repeat("r", i+1)
		if _, err := broker.Submit("job-1", token, r); err != nil {
			t.Fatal(err)
		}
	}
	r := request()
	r.ID = "overflow"
	if _, err := broker.Submit("job-1", token, r); !errors.Is(err, agentexecution.ErrOutputLimit) {
		t.Fatalf("overflow=%v", err)
	}
	parent, cancel := context.WithCancel(t.Context())
	token, err := broker.OpenJob(parent, "cancel-job", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := broker.Submit("cancel-job", token, request()); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("canceled job=%v", err)
	}
}

func TestRelayRejectsUnsafeRequestsAndDiagnosticErrors(t *testing.T) {
	broker, token := newTestBroker(t)
	for _, action := range []string{"http.get", "shell.exec", "admin.call"} {
		r := request()
		r.Action = action
		if _, err := broker.Submit("job-1", token, r); !errors.Is(err, agentexecution.ErrInvalidRequest) {
			t.Fatalf("action=%s err=%v", action, err)
		}
	}
	if _, err := broker.Submit("job-1", token, request()); err != nil {
		t.Fatal(err)
	}
	for _, response := range []agentexecution.PlatformResponse{
		{ID: "r1", Status: "failed", ErrorCode: "https://user:password@private"},
		{ID: "r1", Status: "failed", ErrorCode: "internal_error", Data: json.RawMessage(`"secret"`)},
		{ID: "r1", Status: "completed", ErrorCode: "internal_error"},
		{ID: "r1", Status: "completed", Data: json.RawMessage(strings.Repeat(" ", MaxResponseBytes+1))},
	} {
		if err := broker.Resolve("job-1", response); !errors.Is(err, agentexecution.ErrInvalidRequest) {
			t.Fatalf("unsafe response accepted: status=%s err=%v", response.Status, err)
		}
	}
}
