package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

type memoryMessageRecords struct {
	records map[string]map[string]interface{}
	order   []string
	prompts map[string]string
}

func (m *memoryMessageRecords) Upsert(_ context.Context, record *runtimemodel.MessageRecord) error {
	key := record.MessageID.String() + "/" + record.Kind + "/" + record.RecordID
	if existing, ok := m.records[key]; ok {
		for name, value := range record.Payload {
			existing[name] = value
		}
		return nil
	}
	m.records[key] = copyStringAnyMap(record.Payload)
	m.order = append(m.order, key)
	return nil
}

func (m *memoryMessageRecords) ListByMessages(_ context.Context, kind string, ids []uuid.UUID) (map[uuid.UUID][]map[string]interface{}, error) {
	out := map[uuid.UUID][]map[string]interface{}{}
	for _, id := range ids {
		prefix := id.String() + "/" + kind + "/"
		for _, key := range m.order {
			if len(key) > len(prefix) && key[:len(prefix)] == prefix {
				out[id] = append(out[id], m.records[key])
			}
		}
	}
	return out, nil
}

func (m *memoryMessageRecords) Count(_ context.Context, id uuid.UUID, kind string) (int64, error) {
	listed, _ := m.ListByMessages(context.Background(), kind, []uuid.UUID{id})
	return int64(len(listed[id])), nil
}

func (m *memoryMessageRecords) SaveSystemPrompt(_ context.Context, hash, content string) error {
	m.prompts[hash] = content
	return nil
}

func (m *memoryMessageRecords) SystemPrompts(_ context.Context, hashes []string) (map[string]string, error) {
	out := map[string]string{}
	for _, hash := range hashes {
		if content, ok := m.prompts[hash]; ok {
			out[hash] = content
		}
	}
	return out, nil
}

// A model call becomes a record outside the message's metadata, with its
// system prompt stored once, and the runtime log reads it back (ADR 0039).
func TestModelCallsAreRecordedOutsideMessageMetadata(t *testing.T) {
	ctx := context.Background()
	records := &memoryMessageRecords{records: map[string]map[string]interface{}{}, prompts: map[string]string{}}
	svc := &service{repos: &repository.Repositories{MessageRecord: records}}
	message := &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{}}
	prepared := &PreparedChat{Message: message}

	metadata := map[string]interface{}{}
	first := map[string]interface{}{"runtime_id": "call-1", "phase": "model_call", "user_system_prompt": "You are helpful."}
	if !svc.recordModelInvocation(ctx, prepared, metadata, first) {
		t.Fatal("record was not written")
	}
	usage := map[string]interface{}{"runtime_id": "call-1", "usage": map[string]interface{}{"total_tokens": 42}}
	if !svc.recordModelInvocation(ctx, prepared, metadata, usage) {
		t.Fatal("update was not written")
	}
	if _, kept := metadata["model_invocations"]; kept || metadata["model_invocation_count"] != 1 || metadata[runtimeTimelineMetadataKey] == nil {
		t.Fatalf("metadata = %#v, want the timeline and count without the calls", metadata)
	}
	stored := records.records[message.ID.String()+"/model_invocations/call-1"]
	if stored["user_system_prompt"] != nil || stored["user_system_prompt_hash"] == nil || stored["usage"] == nil || stored["phase"] != "model_call" {
		t.Fatalf("stored record = %#v", stored)
	}

	// A message from before keeps its calls in metadata; the log shows both.
	message.Metadata = map[string]interface{}{"model_invocations": []interface{}{map[string]interface{}{"runtime_id": "call-0"}}}
	svc.hydrateModelInvocationRecords(ctx, message)
	calls := modelInvocationsFromMetadata(message.Metadata["model_invocations"])
	if len(calls) != 2 || calls[0]["runtime_id"] != "call-0" || calls[1]["user_system_prompt"] != "You are helpful." || calls[1]["user_system_prompt_hash"] != nil {
		t.Fatalf("hydrated calls = %#v", calls)
	}
}
