package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

type runtimeInputDispatcherStub struct {
	callerType string
	batches    []uuid.UUID
}

func (stub *runtimeInputDispatcherStub) OwnsCallerType(callerType string) bool {
	return callerType == stub.callerType
}

func (stub *runtimeInputDispatcherStub) EnqueueRuntimeInputFollowUp(_ context.Context, batchID uuid.UUID) error {
	stub.batches = append(stub.batches, batchID)
	return nil
}

func TestDispatchPreparedRuntimeInputFollowUpRespectsCallerOwnership(t *testing.T) {
	batchID := uuid.New()
	dispatcher := &runtimeInputDispatcherStub{callerType: runtimemodel.ConversationCallerAgent}
	runtime := &service{runtimeInputDispatcher: dispatcher}
	prepared := &PreparedChat{
		Caller:                      Caller{Type: runtimemodel.ConversationCallerDigitalHuman},
		RuntimeInputFollowUpBatchID: &batchID,
	}

	runtime.dispatchPreparedRuntimeInputFollowUp(t.Context(), prepared)
	if len(dispatcher.batches) != 0 {
		t.Fatalf("foreign caller batch was dispatched: %#v", dispatcher.batches)
	}
	prepared.Caller.Type = runtimemodel.ConversationCallerAgent
	runtime.dispatchPreparedRuntimeInputFollowUp(t.Context(), prepared)
	if len(dispatcher.batches) != 1 || dispatcher.batches[0] != batchID {
		t.Fatalf("owned caller batches = %#v, want %s", dispatcher.batches, batchID)
	}
}

func TestRuntimeInputAppliedEventPayloadDescribesConsumptionCheckpoint(t *testing.T) {
	messageID := uuid.New()
	input := &runtimemodel.RuntimeInput{
		ID:                uuid.New(),
		ClientInputID:     uuid.New(),
		ConversationID:    uuid.New(),
		TargetMessageID:   uuid.New(),
		ConsumedMessageID: &messageID,
		Content:           "use a shorter introduction",
		Sequence:          3,
		CreatedAt:         time.Unix(100, 0).UTC(),
	}
	payload := runtimeInputAppliedEventPayload(input, runtimeInputCheckpoint{
		Name:       runtimeInputCheckpointBeforeModelRound,
		ModelRound: 2,
	}, time.Unix(120, 0).UTC())

	if payload["message_id"] != messageID.String() {
		t.Fatalf("message_id = %#v, want %s", payload["message_id"], messageID)
	}
	if payload["content"] != input.Content || payload["checkpoint"] != runtimeInputCheckpointBeforeModelRound {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["status"] != "applied_current_run" {
		t.Fatalf("status = %#v, want applied_current_run", payload["status"])
	}
	if payload["model_round"] != 2 || payload["applied_at"] != int64(120) {
		t.Fatalf("checkpoint metadata = %#v", payload)
	}
}

func TestRuntimeInputFollowUpBatchIDIsDeterministicPerMessage(t *testing.T) {
	messageID := uuid.New()
	first := runtimeInputFollowUpBatchID(messageID)
	second := runtimeInputFollowUpBatchID(messageID)
	if first == uuid.Nil || first != second {
		t.Fatalf("batch IDs = %s and %s, want the same non-zero ID", first, second)
	}
	if first == runtimeInputFollowUpBatchID(uuid.New()) {
		t.Fatal("different messages produced the same follow-up batch ID")
	}
}

func TestRuntimeInputModelMessagesPreserveSubmittedOrder(t *testing.T) {
	items := []*runtimemodel.RuntimeInput{
		{Content: "first supplement", Sequence: 4},
		nil,
		{Content: "   "},
		{Content: "second supplement", Sequence: 5},
	}
	messages := runtimeInputModelMessages(items)
	if len(messages) != 3 {
		t.Fatalf("messages = %#v, want instruction plus two supplements", messages)
	}
	if messages[0].Role != "system" || messages[0].Content != runtimeInputSystemInstruction {
		t.Fatalf("instruction = %#v", messages[0])
	}
	if messages[1].Role != "user" || messages[1].Content != "first supplement" {
		t.Fatalf("first supplement = %#v", messages[1])
	}
	if messages[2].Role != "user" || messages[2].Content != "second supplement" {
		t.Fatalf("second supplement = %#v", messages[2])
	}
}

func TestRuntimeInputContentsSkipEmptyValuesWithoutReordering(t *testing.T) {
	items := []*runtimemodel.RuntimeInput{{Content: "one"}, nil, {Content: "\t"}, {Content: "two"}}
	contents := runtimeInputContents(items)
	if len(contents) != 2 || contents[0] != "one" || contents[1] != "two" {
		t.Fatalf("contents = %#v", contents)
	}
}

func TestRuntimeInputAdmissionModeProjectsFollowUpDisposition(t *testing.T) {
	mode := initialRuntimeInputConsumptionMode(RuntimeInputAdmissionFollowUpOnly)
	if mode == nil || *mode != runtimemodel.RuntimeInputConsumptionFollowUp {
		t.Fatalf("follow-up admission mode = %v", mode)
	}
	if got := runtimeInputDisposition(&runtimemodel.RuntimeInput{ConsumptionMode: mode}); got != RuntimeInputDispositionFollowUp {
		t.Fatalf("follow-up disposition = %q", got)
	}
	if got := runtimeInputDisposition(&runtimemodel.RuntimeInput{}); got != RuntimeInputDispositionCurrentRun {
		t.Fatalf("default disposition = %q", got)
	}
}

func TestPreserveRuntimeInputFollowUpMetadata(t *testing.T) {
	generated := map[string]interface{}{"surface": "agent_console"}
	existing := map[string]interface{}{
		runtimeInputFollowUpBatchIDMetadataKey: "batch-id",
		"transient":                            "do not copy",
	}
	metadata := preserveRuntimeInputFollowUpMetadata(generated, existing)
	if metadata[runtimeInputFollowUpBatchIDMetadataKey] != "batch-id" {
		t.Fatalf("batch ID metadata = %#v", metadata[runtimeInputFollowUpBatchIDMetadataKey])
	}
	if _, ok := metadata["transient"]; ok {
		t.Fatalf("metadata copied unrelated value: %#v", metadata)
	}
}
