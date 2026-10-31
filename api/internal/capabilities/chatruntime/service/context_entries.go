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
// a recomputed effective form. When context compression changes a tool result,
// the canonical payload and preview are updated together so the original is not
// recoverable from the database.
type contextEntrySync struct {
	messageID      uuid.UUID
	conversationID uuid.UUID
	organizationID uuid.UUID
	head           *uuid.UUID
	persisted      int
	// base is the number of stored entries that precede this run's
	// transcript: the entries a paused turn stored before a continuation, or a
	// legacy parent prefix once it has been written.
	base      int
	ordered   []contextEntryOrigin
	bootstrap []adapter.Message
	// continuation is set when the run resumes a paused turn. Its context
	// manager starts empty, so the turn written here is what resumed the run
	// followed by what the run produced.
	continuation bool
	// resumedAnswer is the answer the paused turn already had, such as a
	// partial answer before a token budget pause. It is not this run's output.
	resumedAnswer string
	// resumePrefix rebuilds a paused turn that has no stored entries: its
	// question, the exchange kept in message metadata, and its partial answer.
	resumePrefix []adapter.Message
}

// contextEntryOrigin tracks where one transcript message lives in the durable
// log, in transcript order. Messages that are not storable (the request
// prologue) are never added to this list.
type contextEntryOrigin struct {
	id          uuid.UUID
	identity    string
	fingerprint string
}

// agentContextEntrySync returns the per-turn entry writer, creating it on first
// use. A resumed turn continues from the message's current head.
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
	if prepared.Message.TranscriptHeadEntryID != nil && *prepared.Message.TranscriptHeadEntryID != uuid.Nil {
		writer.head = cloneUUID(prepared.Message.TranscriptHeadEntryID)
	} else if prepared.Message.ParentID != nil && *prepared.Message.ParentID != uuid.Nil && s.repos.Message != nil {
		parent, parentErr := s.repos.Message.GetScoped(ctx, *prepared.Message.ParentID, prepared.Scope.OrganizationID, prepared.Scope.AccountID)
		if parentErr == nil && parent != nil && parent.TranscriptHeadEntryID != nil {
			writer.head = cloneUUID(parent.TranscriptHeadEntryID)
		}
	}
	if writer.head == nil && prepared.Message.ParentID != nil && s.repos.Message != nil {
		writer.bootstrap = s.legacyParentTranscript(ctx, prepared)
	}
	existing, err := s.repos.TranscriptEntry.ListPreviewsByMessage(ctx, writer.messageID)
	if err != nil {
		return nil, fmt.Errorf("load stored transcript entries: %w", err)
	}
	for _, entry := range existing {
		if entry == nil {
			continue
		}
		writer.ordered = append(writer.ordered, contextEntryOrigin{
			id:          entry.ID,
			identity:    transcriptMessageIdentity(entry.Preview),
			fingerprint: protocolMessageFingerprint(entry.Preview),
		})
	}
	if len(existing) > 0 {
		latest := existing[len(existing)-1]
		if latest != nil && latest.ID != uuid.Nil {
			writer.head = cloneUUID(&latest.ID)
		}
	}
	writer.persisted = len(existing)
	if prepared.Continuation {
		writer.continuation = true
		writer.base = len(existing)
		writer.resumedAnswer = strings.TrimSpace(prepared.Message.Answer)
		if prepared.pausedAnswer != nil {
			writer.resumedAnswer = strings.TrimSpace(*prepared.pausedAnswer)
		}
		if writer.base == 0 {
			writer.resumePrefix = s.pausedTurnWithoutEntries(ctx, prepared.Message, writer.resumedAnswer)
		}
	}
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
	bound := s.persistTurnTranscriptEntries(ctx, prepared, manager.TurnTranscript())
	manager.BindTurnTranscript(bound)
}

// persistTurnTranscriptEntries writes one turn's protocol messages, the user's
// own question included. Every writer of the durable log goes through here, so
// the log stays a complete conversation record instead of a tool trace.
func (s *service) persistTurnTranscriptEntries(ctx context.Context, prepared *PreparedChat, transcript []adapter.Message) []adapter.Message {
	if prepared == nil {
		return transcript
	}
	writer, err := s.agentContextEntrySync(ctx, prepared)
	if err != nil {
		logger.WarnContext(ctx, "failed to prepare agent transcript entry writer", "message_id", messageIDString(prepared), err)
		return transcript
	}
	if writer == nil {
		return transcript
	}
	turn := turnTranscriptWithQueryAndAnswer(prepared, transcript)
	turnStart := 0
	if strings.TrimSpace(prepared.Message.Query) != "" {
		turnStart = 1
	}
	if writer.continuation {
		prefix := append(append([]adapter.Message(nil), writer.resumePrefix...), prepared.continuationTranscript...)
		turn = appendResumedTurnAnswer(prepared, writer.resumedAnswer, append(prefix, transcript...))
		turnStart = len(prefix)
	}
	bootstrapped := 0
	if writer.persisted == 0 && len(writer.bootstrap) > 0 {
		turn = append(append([]adapter.Message{}, writer.bootstrap...), turn...)
		turnStart += len(writer.bootstrap)
		bootstrapped = len(writer.bootstrap)
	}
	origins, err := writer.syncWithOrigins(ctx, s.repos, turn)
	if err != nil {
		logger.WarnContext(ctx, "failed to append agent transcript entries", "message_id", messageIDString(prepared), err)
		return transcript
	}
	if bootstrapped > 0 {
		// The legacy prefix is stored once. Later checkpoints carry only this
		// turn, so its positions now start after the prefix.
		writer.base += bootstrapped
		writer.bootstrap = nil
	}
	prepared.Message.TranscriptHeadEntryID = cloneUUID(writer.head)
	bound := append([]adapter.Message(nil), transcript...)
	for index := range bound {
		originIndex := turnStart + index
		if originIndex >= 0 && originIndex < len(origins) && origins[originIndex].id != uuid.Nil {
			bound[index].EntryID = origins[originIndex].id.String()
		}
	}
	return bound
}

// legacyParentTranscript converts the main-branch metadata snapshots into a
// one-time in-memory prefix. The prefix is written as entries of the resumed
// message so a legacy conversation immediately joins the parent-chain model.
func (s *service) legacyParentTranscript(ctx context.Context, prepared *PreparedChat) []adapter.Message {
	if s == nil || prepared == nil || prepared.Message == nil || prepared.Message.ParentID == nil || s.repos == nil || s.repos.Message == nil {
		return nil
	}
	var reverse []*runtimemodel.Message
	current := *prepared.Message.ParentID
	seen := map[uuid.UUID]struct{}{}
	for current != uuid.Nil {
		if _, ok := seen[current]; ok {
			return nil
		}
		seen[current] = struct{}{}
		row, err := s.repos.Message.GetScoped(ctx, current, prepared.Scope.OrganizationID, prepared.Scope.AccountID)
		if err != nil || row == nil {
			break
		}
		reverse = append(reverse, row)
		if row.ParentID == nil {
			break
		}
		current = *row.ParentID
	}
	out := make([]adapter.Message, 0, len(reverse)*3)
	for index := len(reverse) - 1; index >= 0; index-- {
		row := reverse[index]
		if strings.TrimSpace(row.Query) != "" {
			out = append(out, adapter.Message{Role: "user", Content: row.Query})
		}
		out = append(out, agentTranscriptFromMetadata(row.Metadata, row.Answer)...)
		if strings.TrimSpace(row.Answer) != "" {
			out = append(out, adapter.Message{Role: "assistant", Content: row.Answer})
		}
	}
	return out
}

// turnTranscriptWithQuery puts the user's question in front of the turn. The
// context manager keeps its transcript to the assistant/tool exchange (its
// callers already carry the current query), but the durable log must also hold
// the question: without it a rebuild would keep the answers and lose what they
// were answering.
func turnTranscriptWithQuery(prepared *PreparedChat, transcript []adapter.Message) []adapter.Message {
	if prepared == nil || prepared.Message == nil {
		return transcript
	}
	query := strings.TrimSpace(prepared.Message.Query)
	if query == "" {
		return transcript
	}
	// The question is stored with the reminders it was sent with, so the next
	// turn replays this turn's request and reads it from the provider's cache
	// (ADR 0043). Transient page context and attachment previews are not stored.
	query = queryWithLanguageReminder(prepared.parts, query)
	var content interface{} = query
	if prepared.parts != nil && len(prepared.parts.TurnReminders) > 0 {
		content = adapter.PrependSystemReminders(prepared.parts.TurnReminders, query)
	}
	// The loop may have merged more guidance after the question, such as the
	// instructions of skills active from the start. Its form is kept only when
	// the question was sent as plain text.
	if sent, ok := prepared.turnUserContent.(string); ok {
		if base, isText := content.(string); isText && strings.HasPrefix(sent, base) {
			content = sent
		}
	}
	messages := make([]adapter.Message, 0, len(transcript)+1)
	messages = append(messages, adapter.Message{Role: "user", Content: content})
	return append(messages, transcript...)
}

func turnTranscriptWithQueryAndAnswer(prepared *PreparedChat, transcript []adapter.Message) []adapter.Message {
	return appendTurnAnswer(prepared, turnTranscriptWithQuery(prepared, transcript))
}

// resumeWith gives the model a message that resumes the paused turn, such as
// the user's answer or an approval decision, and records it for the turn's
// durable transcript. Without it later turns would only see the pause.
func (p *PreparedChat) resumeWith(message adapter.Message) {
	if p == nil || p.LLMRequest == nil {
		return
	}
	p.LLMRequest.Messages = append(p.LLMRequest.Messages, message)
	if p.pausedAnswer == nil && p.Message != nil {
		answer := p.Message.Answer
		p.pausedAnswer = &answer
	}
	recorded := message
	if strings.EqualFold(strings.TrimSpace(recorded.Role), "system") {
		// The durable log keeps user, assistant, and tool messages. A runtime
		// fact the model received as system context is kept as a user-side
		// record so later turns can replay how the pause was resolved.
		recorded.Role = "user"
	}
	p.continuationTranscript = append(p.continuationTranscript, recorded)
}

// appendResumedTurnAnswer adds the turn's answer only when this run produced
// it. The answer the paused turn already had is stored before the pause.
func appendResumedTurnAnswer(prepared *PreparedChat, resumedAnswer string, messages []adapter.Message) []adapter.Message {
	if prepared == nil || prepared.Message == nil || strings.TrimSpace(prepared.Message.Answer) == strings.TrimSpace(resumedAnswer) {
		return messages
	}
	return appendTurnAnswer(prepared, messages)
}

// pausedTurnWithoutEntries rebuilds a paused turn saved before transcript
// entries existed, so a continuation stores it ahead of what resumed it.
func (s *service) pausedTurnWithoutEntries(ctx context.Context, message *runtimemodel.Message, pausedAnswer string) []adapter.Message {
	if message == nil {
		return nil
	}
	out := make([]adapter.Message, 0, 4)
	if query := strings.TrimSpace(message.Query); query != "" {
		out = append(out, adapter.Message{Role: "user", Content: query})
	}
	paused := *message
	paused.Answer = pausedAnswer
	out = append(out, s.historicalAgentTranscript(ctx, &paused)...)
	if answer := strings.TrimSpace(pausedAnswer); answer != "" {
		last := len(out) - 1
		if last < 0 || !strings.EqualFold(strings.TrimSpace(out[last].Role), "assistant") || messageContentText(out[last].Content) != answer || len(out[last].ToolCalls) > 0 {
			out = append(out, adapter.Message{Role: "assistant", Content: answer})
		}
	}
	return out
}

func appendTurnAnswer(prepared *PreparedChat, messages []adapter.Message) []adapter.Message {
	if prepared == nil || prepared.Message == nil || strings.TrimSpace(prepared.Message.Answer) == "" {
		return messages
	}
	answer := strings.TrimSpace(prepared.Message.Answer)
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		if strings.EqualFold(strings.TrimSpace(last.Role), "assistant") && messageContentText(last.Content) == answer && len(last.ToolCalls) == 0 {
			return messages
		}
	}
	return append(messages, adapter.Message{Role: "assistant", Content: answer})
}

func (w *contextEntrySync) syncWithOrigins(ctx context.Context, repos *repository.Repositories, transcript []adapter.Message) ([]contextEntryOrigin, error) {
	if w == nil || repos == nil || repos.TranscriptEntry == nil || len(transcript) == 0 {
		return nil, nil
	}
	// Encode the live view once. It answers both questions this function asks:
	// which stored previews are stale, and which messages are new.
	payloads := make([]map[string]interface{}, len(transcript))
	sizes := make([]int, len(transcript))
	fingerprints := make([]string, len(transcript))
	for index, message := range transcript {
		payload, err := protocolMessageToJSON(message)
		if err != nil {
			return nil, fmt.Errorf("encode transcript message: %w", err)
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("measure transcript message: %w", err)
		}
		payloads[index], sizes[index] = payload, len(encoded)
		fingerprints[index] = protocolMessageFingerprint(payload)
	}
	// Positions are relative to base: a continuation's transcript starts
	// after the entries its paused run already stored.
	persisted := w.persisted - w.base
	if persisted < 0 {
		persisted = 0
	}
	stored := persisted
	if stored > len(transcript) {
		stored = len(transcript)
	}
	matched := 0
	for matched < stored && fingerprints[matched] == w.ordered[w.base+matched].fingerprint {
		matched++
	}
	if matched < stored {
		w.refreshRewrittenPreviews(ctx, repos, payloads, sizes, fingerprints, matched, stored)
	}
	if persisted >= len(transcript) {
		origins := make([]contextEntryOrigin, len(transcript))
		copy(origins, w.ordered[w.base:w.base+len(transcript)])
		return origins, nil
	}

	head := cloneUUID(w.head)
	entries := make([]*runtimemodel.TranscriptEntry, 0, len(transcript)-persisted)
	origins := make([]contextEntryOrigin, 0, len(transcript)-persisted)
	for index := persisted; index < len(transcript); index++ {
		kind, storable := transcriptEntryKindForRole(transcript[index].Role)
		if !storable {
			origins = append(origins, contextEntryOrigin{fingerprint: fingerprints[index]})
			continue
		}
		id := uuid.New()
		entries = append(entries, &runtimemodel.TranscriptEntry{
			ID:             id,
			MessageID:      w.messageID,
			ConversationID: w.conversationID,
			OrganizationID: w.organizationID,
			ParentEntryID:  cloneUUID(head),
			EventID:        fmt.Sprintf("%s:%d:%s", w.messageID, w.base+index, fingerprints[index]),
			Kind:           kind,
			Preview:        pruneTranscriptMessageForWrite(id, payloads[index], kind),
			Payload:        payloads[index],
			ByteSize:       sizes[index],
		})
		origins = append(origins, contextEntryOrigin{
			id:          id,
			identity:    transcriptMessageIdentity(payloads[index]),
			fingerprint: fingerprints[index],
		})
		head = &id
	}
	if len(entries) > 0 {
		newHead, err := repos.TranscriptEntry.AppendForMessage(ctx, w.messageID, w.head, entries)
		if err != nil {
			return nil, err
		}
		if newHead != uuid.Nil {
			w.head = cloneUUID(&newHead)
		}
	}
	w.ordered = append(w.ordered, origins...)
	w.persisted = w.base + len(transcript)
	return append([]contextEntryOrigin(nil), w.ordered[w.base:w.base+len(transcript)]...), nil
}

// refreshRewrittenPreviews replaces the canonical stored form of entries whose
// live counterpart changed in place, for example a tool result compressed by a
// later context-budget decision.
//
// A position whose live message is a *different* message is not a rewrite but a
// divergence. The log is append-only, so the stored entry is kept and reported.
func (w *contextEntrySync) refreshRewrittenPreviews(ctx context.Context, repos *repository.Repositories, payloads []map[string]interface{}, sizes []int, fingerprints []string, from, to int) {
	for index := from; index < to; index++ {
		origin := w.ordered[w.base+index]
		if origin.id == uuid.Nil {
			continue
		}
		live := transcriptMessageIdentity(payloads[index])
		if live == "" || live != origin.identity {
			logger.WarnContext(ctx, "agent transcript diverged from stored entries; stored entries are kept",
				"message_id", w.messageID.String(), "position", index, "stored", origin.identity, "live", live)
			continue
		}
		kind, _ := transcriptEntryKindForRole(messageContentText(payloads[index]["role"]))
		if err := repos.TranscriptEntry.UpdatePreview(ctx, origin.id, pruneTranscriptMessageForWrite(origin.id, payloads[index], kind), sizes[index]); err != nil {
			logger.WarnContext(ctx, "failed to refresh a stored transcript preview",
				"message_id", w.messageID.String(), "entry_id", origin.id.String(), err)
			continue
		}
		w.ordered[w.base+index].fingerprint = fingerprints[index]
	}
}

// transcriptMessageIdentity names the protocol message an entry is about, so a
// rewritten form can be told apart from a different message at the same
// position. Only protocol ids are used: content may legitimately change.
// Messages without an id have no identity, and are never rewritten in place.
func transcriptMessageIdentity(payload map[string]interface{}) string {
	if id := strings.TrimSpace(messageContentText(payload["tool_call_id"])); id != "" {
		return "tool:" + id
	}
	calls, ok := payload["tool_calls"].([]interface{})
	if !ok {
		return ""
	}
	for _, call := range calls {
		fields, ok := call.(map[string]interface{})
		if !ok {
			continue
		}
		if id := strings.TrimSpace(messageContentText(fields["id"])); id != "" {
			return "assistant:" + id
		}
	}
	return ""
}

// conversationRegion is the model-visible conversation built from the durable
// transcript log: the newest applicable summary plus every entry after its
// boundary. Each replayed message carries the conversation position of its
// entry, so a later compaction can name the exact boundary it summarized.
type conversationRegion struct {
	Messages []adapter.Message
}

// projectTranscriptChain rebuilds context from the parent-linked transcript.
// The newest summary is a checkpoint appended at the then-current head; its
// coverage boundary may therefore be older than the summary's parent. We keep
// the summary plus every non-summary entry between that boundary and head.
func (s *service) projectTranscriptChain(ctx context.Context, headID uuid.UUID, branchByID map[uuid.UUID]*runtimemodel.Message, includeImages bool) (conversationRegion, bool, error) {
	if s == nil || s.repos == nil || s.repos.TranscriptEntry == nil || headID == uuid.Nil {
		return conversationRegion{}, false, nil
	}
	chain, err := s.repos.TranscriptEntry.LoadPreviewChain(ctx, headID, nil, 0)
	if err != nil {
		return conversationRegion{}, false, err
	}
	if len(chain) == 0 {
		return conversationRegion{}, false, nil
	}
	latestSummary := -1
	boundaryIndex := -1
	for index, entry := range chain {
		if entry == nil || entry.Kind != runtimemodel.TranscriptEntryKindSummary {
			continue
		}
		latestSummary = index
		if entry.CoversThroughEntryID == nil || *entry.CoversThroughEntryID == uuid.Nil {
			return conversationRegion{}, false, fmt.Errorf("transcript summary %s has no coverage boundary", entry.ID)
		}
		for boundary := index + 1; boundary < len(chain); boundary++ {
			if chain[boundary] != nil && chain[boundary].ID == *entry.CoversThroughEntryID {
				boundaryIndex = boundary
				break
			}
		}
		if boundaryIndex < 0 {
			return conversationRegion{}, false, fmt.Errorf("transcript summary %s boundary is not reachable", entry.ID)
		}
		break
	}

	selected := chain
	var summary *runtimemodel.TranscriptEntryPreview
	if latestSummary >= 0 {
		summary = chain[latestSummary]
		selected = chain[:boundaryIndex] // coverage boundary itself is represented by the summary
	}
	messages := make([]adapter.Message, 0, len(selected)+1)
	if summary != nil {
		content := messageContentText(summary.Preview["content"])
		if strings.TrimSpace(content) == "" {
			return conversationRegion{}, false, fmt.Errorf("transcript summary %s has empty content", summary.ID)
		}
		messages = append(messages, adapter.Message{Role: "user", Content: content, EntryID: summary.ID.String()})
	}
	for index := len(selected) - 1; index >= 0; index-- {
		entry := selected[index]
		if entry == nil || entry.Kind == runtimemodel.TranscriptEntryKindSummary || len(entry.Preview) == 0 {
			continue
		}
		message, decodeErr := protocolMessageFromJSON(entry.Preview)
		if decodeErr != nil {
			return conversationRegion{}, false, fmt.Errorf("project transcript entry %s: %w", entry.ID, decodeErr)
		}
		message.EntryID = entry.ID.String()
		if entry.Kind == runtimemodel.TranscriptEntryKindUser {
			if row := branchByID[entry.MessageID]; row != nil && strings.TrimSpace(row.Query) == strings.TrimSpace(messageContentText(entry.Preview["content"])) {
				if rendered, renderErr := s.historicalUserMessage(ctx, row, includeImages); renderErr == nil && rendered != nil {
					rendered.EntryID = entry.ID.String()
					message = *rendered
				}
			}
		}
		messages = append(messages, message)
	}
	if len(messages) == 0 {
		return conversationRegion{}, false, nil
	}
	messages, _ = closeDanglingToolCalls(messages)
	return conversationRegion{Messages: messages}, true, nil
}

// persistAgentContextSummary stores one compaction summary as an append-only
// entry with a parent-chain boundary.
func (s *service) persistAgentContextSummary(ctx context.Context, prepared *PreparedChat, summary, boundaryID string) {
	if s == nil || s.repos == nil || s.repos.Message == nil || prepared == nil || prepared.Message == nil {
		return
	}
	if strings.TrimSpace(summary) == "" {
		return
	}
	if boundary, err := uuid.Parse(strings.TrimSpace(boundaryID)); err == nil && boundary != uuid.Nil && s.repos.TranscriptEntry != nil {
		head := cloneUUID(prepared.Message.TranscriptHeadEntryID)
		newHead, appendErr := s.repos.TranscriptEntry.AppendSummaryForMessage(ctx, prepared.Message.ID, head, &boundary, contextmgr.GetCompactUserSummaryMessage(contextmgr.FormatCompactSummary(summary), false, "", true), "summary:"+boundary.String())
		if appendErr != nil {
			logger.WarnContext(ctx, "failed to append agent context summary entry", "message_id", prepared.Message.ID.String(), "boundary_entry_id", boundary.String(), appendErr)
			return
		}
		if newHead != uuid.Nil {
			prepared.Message.TranscriptHeadEntryID = cloneUUID(&newHead)
		}
		return
	}
}

// historicalAgentTranscript returns the stored protocol messages of one turn:
// transcript entries when the turn has them, otherwise the legacy metadata
// snapshot written before the entry table existed.
//
// The turn's own user entry is dropped: this replay is appended to a request
// that already carries the current question, and repeating it would insert a
// second user message in the middle of the assistant/tool exchange.
func (s *service) historicalAgentTranscript(ctx context.Context, message *runtimemodel.Message) []adapter.Message {
	if s == nil || message == nil {
		return nil
	}
	if s.repos != nil && s.repos.TranscriptEntry != nil {
		entries, err := s.repos.TranscriptEntry.ListPreviewsByMessage(ctx, message.ID)
		if err == nil && len(entries) > 0 {
			messages, projectErr := projectContextMessages(transcriptExchangeEntries(entries))
			if projectErr == nil {
				messages, _ = closeDanglingToolCalls(messages)
				return dropDuplicatedFinalAnswer(messages, message.Answer)
			}
			logger.WarnContext(ctx, "failed to project stored transcript entries",
				"message_id", message.ID.String(), projectErr)
		}
	}
	return agentTranscriptFromMetadata(message.Metadata, message.Answer)
}

// dropDuplicatedFinalAnswer mirrors the legacy transcript rule
// (normalizeAgentTranscript): callers append the turn's stored answer
// themselves, so a trailing plain assistant message must not be replayed a
// second time. The entry log keeps that answer as a fact; only this per-turn
// replay drops it.
func dropDuplicatedFinalAnswer(messages []adapter.Message, answer string) []adapter.Message {
	if strings.TrimSpace(answer) == "" || len(messages) == 0 {
		return messages
	}
	last := messages[len(messages)-1]
	if strings.EqualFold(strings.TrimSpace(last.Role), "assistant") && len(last.ToolCalls) == 0 {
		return messages[:len(messages)-1]
	}
	return messages
}

// transcriptExchangeEntries keeps everything but the user's own question: the
// user entries that open the turn. A continuation records how a pause was
// resolved (an approval, an answer, a connector result) as a user-side entry
// later in the turn; those stay, or a resumed turn forgets what it learned.
func transcriptExchangeEntries(entries []*runtimemodel.TranscriptEntryPreview) []*runtimemodel.TranscriptEntryPreview {
	out := make([]*runtimemodel.TranscriptEntryPreview, 0, len(entries))
	opened := false
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Kind == runtimemodel.TranscriptEntryKindUser && !opened {
			continue
		}
		opened = true
		out = append(out, entry)
	}
	return out
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

func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	copy := *value
	return &copy
}
