package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// contextEntrySync writes the protocol messages of one turn into the append-only
// transcript entry table.
//
// Only two things are ever written: a new fact (an appended protocol message) or
// a recomputed effective form (preview). The payload column of an existing row
// is never touched, which is what keeps "the conversation the user saw" and
// "the context the model saw" from drifting apart.
type contextEntrySync struct {
	messageID      uuid.UUID
	conversationID uuid.UUID
	organizationID uuid.UUID
	seq            int
	cseq           int64
	persisted      int
	ordered        []contextEntryOrigin
}

// contextEntryOrigin tracks where one transcript message lives in the durable
// log, in transcript order. CSeq is zero for messages that are not stored (the
// prologue is re-rendered, never logged).
type contextEntryOrigin struct {
	fingerprint string
	cseq        int64
}

// agentContextEntrySync returns the per-turn entry writer, creating it on first
// use. A resumed turn continues the existing seq and cseq spaces instead of
// restarting them, so appends are idempotent under the unique constraints.
func (s *service) agentContextEntrySync(ctx context.Context, prepared *PreparedChat) (*contextEntrySync, error) {
	if s == nil || s.repos == nil || s.repos.TranscriptEntry == nil {
		return nil, nil
	}
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return nil, nil
	}
	if prepared.contextEntries != nil {
		return prepared.contextEntries, nil
	}
	writer := &contextEntrySync{
		messageID:      prepared.Message.ID,
		conversationID: prepared.Message.ConversationID,
		organizationID: prepared.Conversation.OrganizationID,
	}
	seq, cseq, err := s.repos.TranscriptEntry.NextPositions(ctx, writer.conversationID, writer.messageID)
	if err != nil {
		return nil, fmt.Errorf("resolve transcript entry positions: %w", err)
	}
	writer.seq, writer.cseq = seq, cseq
	existing, err := s.repos.TranscriptEntry.ListPreviewsByMessage(ctx, writer.messageID)
	if err != nil {
		return nil, fmt.Errorf("load stored transcript entries: %w", err)
	}
	for _, entry := range existing {
		writer.ordered = append(writer.ordered, contextEntryOrigin{
			fingerprint: protocolMessageFingerprint(entry.Preview),
			cseq:        entry.CSeq,
		})
	}
	writer.persisted = len(existing)
	prepared.contextEntries = writer
	return writer, nil
}

// syncAgentTranscriptEntries persists every protocol message produced so far.
// Call it at each interruption point: before a model call, before a compaction,
// and on the terminal paths.
func (s *service) syncAgentTranscriptEntries(ctx context.Context, prepared *PreparedChat, manager *contextmgr.Manager) {
	if prepared == nil || manager == nil {
		return
	}
	writer, err := s.agentContextEntrySync(ctx, prepared)
	if err != nil {
		logger.WarnContext(ctx, "failed to prepare agent transcript entry writer", "message_id", messageIDString(prepared), err)
		return
	}
	if writer == nil {
		return
	}
	if err := writer.sync(ctx, s.repos, manager.TurnTranscript()); err != nil {
		logger.WarnContext(ctx, "failed to append agent transcript entries", "message_id", messageIDString(prepared), err)
	}
}

func (w *contextEntrySync) sync(ctx context.Context, repos *repository.Repositories, transcript []adapter.Message) error {
	if w == nil || repos == nil || repos.TranscriptEntry == nil {
		return nil
	}
	if len(transcript) == 0 || w.persisted >= len(transcript) {
		return nil
	}
	matched := 0
	for matched < w.persisted && matched < len(transcript) {
		payload, err := protocolMessageToJSON(transcript[matched])
		if err != nil {
			break
		}
		if protocolMessageFingerprint(payload) != w.ordered[matched].fingerprint {
			break
		}
		matched++
	}
	if matched < w.persisted {
		// The live transcript diverged from what is already durable (for example
		// a tool batch that was incomplete when it was first written). The log is
		// append-only, so the past is left alone and the current view is appended
		// as new entries.
		logger.WarnContext(ctx, "agent transcript diverged from stored entries; appending the current view",
			"message_id", w.messageID.String(), "matched", matched, "persisted", w.persisted)
	}

	seq, cseq := w.seq, w.cseq
	entries := make([]*runtimemodel.TranscriptEntry, 0, len(transcript)-w.persisted)
	origins := make([]contextEntryOrigin, 0, len(transcript)-w.persisted)
	for _, message := range transcript[w.persisted:] {
		payload, err := protocolMessageToJSON(message)
		if err != nil {
			return fmt.Errorf("encode transcript message: %w", err)
		}
		kind, storable := transcriptEntryKindForRole(message.Role)
		if !storable {
			origins = append(origins, contextEntryOrigin{fingerprint: protocolMessageFingerprint(payload)})
			continue
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("measure transcript message: %w", err)
		}
		id := uuid.New()
		entry := &runtimemodel.TranscriptEntry{
			ID:             id,
			MessageID:      w.messageID,
			ConversationID: w.conversationID,
			OrganizationID: w.organizationID,
			Seq:            seq,
			CSeq:           cseq,
			Kind:           kind,
			Preview:        pruneTranscriptMessageForWrite(id, payload, kind),
			Payload:        payload,
			ByteSize:       len(encoded),
		}
		entries = append(entries, entry)
		origins = append(origins, contextEntryOrigin{
			fingerprint: protocolMessageFingerprint(payload),
			cseq:        cseq,
		})
		seq++
		cseq++
	}
	if len(entries) > 0 {
		if err := repos.TranscriptEntry.Append(ctx, entries); err != nil {
			return err
		}
	}
	w.ordered = append(w.ordered, origins...)
	w.seq, w.cseq, w.persisted = seq, cseq, len(transcript)
	return nil
}

// boundaryForCompactInput returns the largest cseq that is certainly covered by
// a summary of the given compact input.
//
// It walks the compact input in order and matches fingerprints against the
// recorded origins. An unmatched message contributes nothing, so the result is
// conservative by construction: it can under-claim (a message then appears both
// in the summary and as an entry, which is harmless) but never over-claim (which
// would drop content from the context).
func (w *contextEntrySync) boundaryForCompactInput(messages []adapter.Message) (int64, bool) {
	if w == nil || len(messages) == 0 || len(w.ordered) == 0 {
		return 0, false
	}
	position := 0
	var boundary int64
	found := false
	for _, message := range messages {
		payload, err := protocolMessageToJSON(message)
		if err != nil {
			continue
		}
		fingerprint := protocolMessageFingerprint(payload)
		for position < len(w.ordered) {
			origin := w.ordered[position]
			position++
			if origin.fingerprint != fingerprint {
				continue
			}
			if origin.cseq > boundary {
				boundary = origin.cseq
			}
			found = true
			break
		}
	}
	return boundary, found
}

// persistAgentContextSummary stores one compaction summary together with its
// precise boundary. The boundary is the cseq of the last entry the summarizer
// actually received, so the rebuild slice (cseq > boundary) neither overlaps the
// summary nor leaves a gap.
func (s *service) persistAgentContextSummary(ctx context.Context, prepared *PreparedChat, summary string, boundary int64) {
	if s == nil || s.repos == nil || s.repos.Message == nil || prepared == nil || prepared.Message == nil {
		return
	}
	if strings.TrimSpace(summary) == "" || boundary <= 0 {
		return
	}
	if err := s.repos.Message.UpdateContextSummary(ctx, prepared.Message.ID, summary, boundary); err != nil {
		logger.WarnContext(ctx, "failed to persist agent context summary", "message_id", prepared.Message.ID.String(), "boundary", boundary, err)
	}
}

// loadProjectedConversationContext rebuilds the model-visible conversation
// region of a conversation at a boundary: the newest applicable summary plus
// every entry after it. It reports false when the conversation has no durable
// entries yet, which is the legacy case the caller answers from message rows.
func (s *service) loadProjectedConversationContext(ctx context.Context, conversationID uuid.UUID, throughCSeq int64) ([]adapter.Message, bool, error) {
	if s == nil || s.repos == nil || s.repos.TranscriptEntry == nil || conversationID == uuid.Nil {
		return nil, false, nil
	}
	summary, err := s.repos.Message.ContextSummaryAt(ctx, conversationID, throughCSeq)
	if err != nil {
		return nil, false, err
	}
	after := int64(0)
	if summary != nil {
		after = summary.ThroughSeq
	}
	entries, err := s.repos.TranscriptEntry.ListPreviewsByConversation(ctx, conversationID, after, 0)
	if err != nil {
		return nil, false, err
	}
	if len(entries) == 0 {
		return nil, false, nil
	}
	messages, err := projectContextMessages(summary, entries)
	if err != nil {
		return nil, false, err
	}
	// Projection must be provider-valid: a persisted context may legitimately end
	// in an unmatched tool call when a process died between call and result.
	messages, _ = closeDanglingToolCalls(messages)
	return messages, true, nil
}

// historicalAgentTranscript returns the stored protocol messages of one turn:
// transcript entries when the turn has them, otherwise the legacy metadata
// snapshot written before the entry table existed.
func (s *service) historicalAgentTranscript(ctx context.Context, message *runtimemodel.Message) []adapter.Message {
	if s == nil || message == nil {
		return nil
	}
	if s.repos != nil && s.repos.TranscriptEntry != nil {
		entries, err := s.repos.TranscriptEntry.ListPreviewsByMessage(ctx, message.ID)
		if err == nil && len(entries) > 0 {
			messages, projectErr := projectContextMessages(nil, entries)
			if projectErr == nil {
				messages, _ = closeDanglingToolCalls(messages)
				return messages
			}
			logger.WarnContext(ctx, "failed to project stored transcript entries",
				"message_id", message.ID.String(), projectErr)
		}
	}
	return agentTranscriptFromMetadata(message.Metadata, message.Answer)
}

// appendPersistedAgentTranscript appends one turn's stored protocol messages to
// a continuation request, preferring transcript entries over the legacy
// metadata snapshot.
func (s *service) appendPersistedAgentTranscript(ctx context.Context, request *adapter.ChatRequest, message *runtimemodel.Message) {
	if request == nil || message == nil {
		return
	}
	request.Messages = append(request.Messages, s.historicalAgentTranscript(ctx, message)...)
}

func messageIDString(prepared *PreparedChat) string {
	if prepared == nil || prepared.Message == nil {
		return ""
	}
	return prepared.Message.ID.String()
}
