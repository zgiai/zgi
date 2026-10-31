package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	userSystemPromptKey     = "user_system_prompt"
	userSystemPromptHashKey = "user_system_prompt_hash"
)

func (s *service) messageRecordsEnabled() bool {
	return s != nil && s.repos != nil && s.repos.MessageRecord != nil
}

// recordModelInvocation stores one model call as a message record instead of
// appending it to the message's metadata (ADR 0039). The metadata keeps the
// call's place in the runtime timeline and the number of calls. The system
// prompt is stored once by hash. It reports false when the record could not be
// written, so the caller can keep the call in metadata instead.
func (s *service) recordModelInvocation(ctx context.Context, prepared *PreparedChat, metadata map[string]interface{}, invocation map[string]interface{}) bool {
	if !s.messageRecordsEnabled() || prepared == nil || prepared.Message == nil {
		return false
	}
	ensureInvocationTimelineTime(invocation, time.Now())
	recordID := strings.TrimSpace(stringFromAny(invocation["runtime_id"]))
	if recordID == "" {
		recordID = uuid.NewString()
		invocation["runtime_id"] = recordID
	}
	payload := compactSkillInvocation(invocation)
	if prompt := strings.TrimSpace(stringFromAny(payload[userSystemPromptKey])); prompt != "" {
		digest := sha256.Sum256([]byte(prompt))
		hash := hex.EncodeToString(digest[:])
		if err := s.repos.MessageRecord.SaveSystemPrompt(ctx, hash, prompt); err != nil {
			logger.WarnContext(ctx, "failed to store a model call's system prompt", "message_id", prepared.Message.ID.String(), err)
			return false
		}
		delete(payload, userSystemPromptKey)
		payload[userSystemPromptHashKey] = hash
	}
	timeline := copyStringAnyMap(metadata)
	upsertRuntimeTimeline(timeline, invocation, runtimemodel.MessageRecordKindModelInvocation)
	payload["sequence"] = invocation["sequence"]
	record := &runtimemodel.MessageRecord{
		MessageID:      prepared.Message.ID,
		OrganizationID: preparedOrganizationID(prepared),
		Kind:           runtimemodel.MessageRecordKindModelInvocation,
		RecordID:       recordID,
		Sequence:       intValueFromAny(invocation["sequence"]),
		Payload:        payload,
	}
	if err := s.repos.MessageRecord.Upsert(ctx, record); err != nil {
		logger.WarnContext(ctx, "failed to store a model call record", "message_id", prepared.Message.ID.String(), err)
		return false
	}
	for key, value := range timeline {
		metadata[key] = value
	}
	count, err := s.repos.MessageRecord.Count(ctx, prepared.Message.ID, runtimemodel.MessageRecordKindModelInvocation)
	if err == nil {
		metadata["model_invocation_count"] = int(count) + len(modelInvocationsFromMetadata(metadata[runtimemodel.MessageRecordKindModelInvocation]))
	}
	return true
}

func preparedOrganizationID(prepared *PreparedChat) uuid.UUID {
	if prepared.Conversation != nil && prepared.Conversation.OrganizationID != uuid.Nil {
		return prepared.Conversation.OrganizationID
	}
	return prepared.Scope.OrganizationID
}

// hydrateModelInvocationRecords puts each message's model call records back
// into its metadata under model_invocations, after any calls a message still
// keeps there from before ADR 0039, with their system prompts restored. Only
// the runtime log paths, which show the calls and never write the message
// back, use it.
func (s *service) hydrateModelInvocationRecords(ctx context.Context, messages ...*runtimemodel.Message) {
	if !s.messageRecordsEnabled() || len(messages) == 0 {
		return
	}
	ids := make([]uuid.UUID, 0, len(messages))
	for _, message := range messages {
		if message != nil {
			ids = append(ids, message.ID)
		}
	}
	records, err := s.repos.MessageRecord.ListByMessages(ctx, runtimemodel.MessageRecordKindModelInvocation, ids)
	if err != nil {
		logger.WarnContext(ctx, "failed to load model call records", err)
		return
	}
	hashes := map[string]struct{}{}
	for _, payloads := range records {
		for _, payload := range payloads {
			if hash := strings.TrimSpace(stringFromAny(payload[userSystemPromptHashKey])); hash != "" {
				hashes[hash] = struct{}{}
			}
		}
	}
	prompts := map[string]string{}
	if len(hashes) > 0 {
		list := make([]string, 0, len(hashes))
		for hash := range hashes {
			list = append(list, hash)
		}
		if prompts, err = s.repos.MessageRecord.SystemPrompts(ctx, list); err != nil {
			logger.WarnContext(ctx, "failed to load model call system prompts", err)
			prompts = map[string]string{}
		}
	}
	for _, message := range messages {
		if message == nil || len(records[message.ID]) == 0 {
			continue
		}
		stored := modelInvocationsFromMetadata(message.Metadata[runtimemodel.MessageRecordKindModelInvocation])
		seen := make(map[string]struct{}, len(stored))
		for _, invocation := range stored {
			seen[strings.TrimSpace(stringFromAny(invocation["runtime_id"]))] = struct{}{}
		}
		for _, payload := range records[message.ID] {
			if _, duplicate := seen[strings.TrimSpace(stringFromAny(payload["runtime_id"]))]; duplicate {
				continue
			}
			invocation := copyStringAnyMap(payload)
			if prompt, ok := prompts[strings.TrimSpace(stringFromAny(invocation[userSystemPromptHashKey]))]; ok {
				invocation[userSystemPromptKey] = prompt
			}
			delete(invocation, userSystemPromptHashKey)
			stored = append(stored, invocation)
		}
		if message.Metadata == nil {
			message.Metadata = map[string]interface{}{}
		}
		message.Metadata[runtimemodel.MessageRecordKindModelInvocation] = skillInvocationsToInterfaceSlice(stored)
	}
}
