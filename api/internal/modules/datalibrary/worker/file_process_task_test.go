package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func TestNewFileProcessTaskBuildsPayload(t *testing.T) {
	requestID := uuid.New()

	task, err := NewFileProcessTask(requestID, nil)
	if err != nil {
		t.Fatalf("NewFileProcessTask: %v", err)
	}

	if task.Type() != TypeDataLibraryFileProcess {
		t.Fatalf("task type=%q", task.Type())
	}

	var payload FileProcessPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ProcessingRequestID != requestID.String() {
		t.Fatalf("processing_request_id=%q", payload.ProcessingRequestID)
	}
}

func TestNewFileProcessTaskRequiresProcessingRequestID(t *testing.T) {
	if _, err := NewFileProcessTask(uuid.Nil, nil); err == nil {
		t.Fatal("expected error for nil processing request id")
	}
}

func TestFileProcessTaskHandlerRejectsInvalidPayload(t *testing.T) {
	handler := NewFileProcessTaskHandler(nil)

	err := handler(context.Background(), asynq.NewTask(TypeDataLibraryFileProcess, []byte("{")))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("malformed payload err=%v", err)
	}

	payload, marshalErr := json.Marshal(FileProcessPayload{ProcessingRequestID: "not-a-uuid"})
	if marshalErr != nil {
		t.Fatalf("marshal payload: %v", marshalErr)
	}
	err = handler(context.Background(), asynq.NewTask(TypeDataLibraryFileProcess, payload))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("invalid uuid err=%v", err)
	}

	payload, marshalErr = json.Marshal(FileProcessPayload{ProcessingRequestID: uuid.NewString()})
	if marshalErr != nil {
		t.Fatalf("marshal payload: %v", marshalErr)
	}
	err = handler(context.Background(), asynq.NewTask(TypeDataLibraryFileProcess, payload))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("nil runner err=%v", err)
	}
}
