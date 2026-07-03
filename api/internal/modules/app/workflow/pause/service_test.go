package pause

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAppendEventPayloadSerializesSequencesPerWorkflowRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&RunEvent{}); err != nil {
		t.Fatalf("migrate run events: %v", err)
	}
	service := NewService(db)
	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := service.AppendEventPayload(context.Background(), AppendEventParams{
				TenantID:      "tenant-1",
				AppID:         "app-1",
				WorkflowRunID: "run-1",
				EventType:     EventNodeStarted,
				EventData:     map[string]interface{}{"index": index},
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("append event: %v", err)
		}
	}
	payload, err := service.ListEvents(context.Background(), "tenant-1", "run-1", 0, count)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(payload.Events) != count {
		t.Fatalf("events = %d, want %d", len(payload.Events), count)
	}
	for index, event := range payload.Events {
		want := index + 1
		if event.Sequence != want {
			t.Fatalf("event[%d].sequence = %d, want %d; events=%s", index, event.Sequence, want, sequencesDebug(payload.Events))
		}
	}
}

func sequencesDebug(events []RunEventPayload) string {
	values := make([]string, 0, len(events))
	for _, event := range events {
		values = append(values, fmt.Sprintf("%d", event.Sequence))
	}
	return "[" + strings.Join(values, ",") + "]"
}
