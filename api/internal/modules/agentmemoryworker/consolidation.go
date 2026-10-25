package agentmemoryworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// Consolidation (ADR 0013) replaces background extraction under memory
// blocks. It reads both sides of the recent turns and the user's memory
// files, and edits the files: it adds what the user stated or confirmed,
// removes duplicates, resolves contradictions, and may rewrite a file within
// its limit to keep it tidy. Each edit is checked on its own; a bad edit is
// skipped instead of failing the pass.

const (
	consolidationUseCase       = "memory-consolidation"
	consolidationModelVariable = "ZGI_MEMORY_CONSOLIDATION_MODEL"
	maxConsolidationEdits      = 12
	maxConsolidationAnswerRune = 2000
	consolidationRewrite       = "rewrite"
	consolidationArchive       = "archive"
)

type consolidationEdit struct {
	Command  string `json:"command"`
	Path     string `json:"path"`
	Text     string `json:"text,omitempty"`
	OldText  string `json:"old_text,omitempty"`
	NewText  string `json:"new_text,omitempty"`
	Content  string `json:"content,omitempty"`
	Evidence string `json:"evidence"`
	Reason   string `json:"reason,omitempty"`
}

func blockPath(key string) string { return "/user/" + key + ".md" }

func appendDistinct(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// archiveConsolidationEdit moves old_text out of a file into the user's
// archive. It changes no fact, so it needs no evidence; the latest turn is
// its source.
func archiveConsolidationEdit(edit consolidationEdit, slot agentmemory.RuntimeSlot, current map[string]agentmemory.SlotValueResponse, working map[string]string, turns []completedTurn) (string, string, *completedTurn, string) {
	content, edited := working[slot.Key]
	if !edited {
		content = strings.TrimSpace(current[slot.Key].Content)
	}
	if strings.TrimSpace(edit.OldText) == "" || strings.Count(content, edit.OldText) != 1 {
		return "", "", nil, "old_text does not appear exactly once"
	}
	remaining := strings.TrimSpace(strings.Replace(content, edit.OldText, "", 1))
	if remaining == "" {
		return "", "", nil, "archive cannot empty a file"
	}
	if len(turns) == 0 {
		return "", "", nil, "no turn to attribute the archive to"
	}
	return slot.Key, remaining, &turns[len(turns)-1], ""
}

// supersededByLaterConversation reports whether another conversation changed
// the file after the turn an edit is based on. Consolidation runs a minute or
// more after a conversation, so without this a fact the user corrected or
// forgot in a newer conversation would come back. Archiving changes no fact
// and is never superseded.
func supersededByLaterConversation(value agentmemory.SlotValueResponse, conversationID uuid.UUID, source *completedTurn) bool {
	if source == nil || source.CompletedAt.IsZero() || value.SourceCompletedAt == 0 || value.SourceConversationID == "" {
		return false
	}
	if value.SourceConversationID == conversationID.String() {
		return false
	}
	return value.SourceCompletedAt > source.CompletedAt.Unix()
}

// archiveWriter keeps archived text in the user's archive. The relationship
// store implements it; the legacy store does not, so archive edits are only
// accepted where it exists.
type archiveWriter interface {
	ArchiveText(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, content, tags, sourceKey string, meta agentmemory.MutationMetadata) error
}

// automaticWritesAllowed reports whether background passes may write the
// block: read-only and manual-only blocks change only on explicit requests.
func automaticWritesAllowed(slot agentmemory.RuntimeSlot) bool {
	return !slot.ReadOnly && !slot.ManualOnly
}

// memoryInstructions reads the builder's memory instructions for the job's
// configuration: the latest published version, or the draft for draft-scoped
// jobs. Missing instructions are not an error.
func (r *Runner) memoryInstructions(ctx context.Context, job *agentmemory.AgentMemoryExtractionJob) string {
	if r.db == nil || job == nil {
		return ""
	}
	var instructions string
	var err error
	if job.ConfigScope == agentmemory.ConfigScopeDraft {
		err = r.db.WithContext(ctx).Table("agents_configs").
			Select("COALESCE(agent_mode::jsonb ->> 'agent_memory_instructions', '')").
			Where("agents_id = ? AND deleted_at IS NULL", job.AgentID).
			Order("updated_at DESC").Limit(1).Scan(&instructions).Error
	} else {
		err = r.db.WithContext(ctx).Table("agent_published_versions").
			Select("COALESCE(config_snapshot ->> 'agent_memory_instructions', '')").
			Where("workspace_id = ? AND agent_id = ? AND deleted_at IS NULL", job.WorkspaceID, job.AgentID).
			Order("created_at DESC").Limit(1).Scan(&instructions).Error
	}
	if err != nil {
		logger.WarnContext(ctx, "memory consolidation could not read memory instructions", "job_id", job.ID.String(), err)
		return ""
	}
	return strings.TrimSpace(instructions)
}

// consolidationModel is the deployment's consolidation model, or the
// conversation's own model when none is configured.
func consolidationModel(turns []completedTurn) (string, string) {
	if configured := strings.TrimSpace(os.Getenv(consolidationModelVariable)); configured != "" {
		if provider, model, ok := strings.Cut(configured, "/"); ok && strings.TrimSpace(provider) != "" && strings.TrimSpace(model) != "" {
			return strings.TrimSpace(provider), strings.TrimSpace(model)
		}
	}
	last := turns[len(turns)-1]
	return last.ModelProvider, last.ModelName
}

func (r *Runner) consolidationChatRequest(turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse, instructions string) (*adapter.ChatRequest, error) {
	if len(turns) == 0 {
		return nil, fmt.Errorf("no completed turns available for consolidation")
	}
	conversation := make([]map[string]string, 0, len(turns))
	for _, turn := range turns {
		answer := []rune(strings.TrimSpace(turn.Answer))
		if len(answer) > maxConsolidationAnswerRune {
			answer = append(answer[:maxConsolidationAnswerRune], []rune("…")...)
		}
		conversation = append(conversation, map[string]string{"message_id": turn.ID.String(), "user": turn.Query, "assistant": string(answer)})
	}
	contents := map[string]string{}
	for _, value := range values {
		contents[value.Key] = strings.TrimSpace(value.Content)
	}
	files := make([]map[string]interface{}, 0, len(slots))
	for _, slot := range slots {
		file := map[string]interface{}{"path": blockPath(slot.Key), "description": slot.Description, "limit": slot.MaxChars, "chars": utf8.RuneCountInString(contents[slot.Key]), "content": contents[slot.Key]}
		if !automaticWritesAllowed(slot) {
			file["access"] = "context only"
		}
		files = append(files, file)
	}
	raw, err := json.Marshal(map[string]interface{}{"conversation": conversation, "files": files})
	if err != nil {
		return nil, fmt.Errorf("encode consolidation prompt: %w", err)
	}
	rules := []string{
		"You keep one user's memory files accurate and tidy after a conversation. Each file's description says what belongs in it.",
		"Add stable facts, preferences, standing instructions, and ongoing context that the user stated, or confirmed when the assistant proposed them. Put each fact in the file whose description fits it.",
		"Fix what is already saved: remove duplicates, replace facts the user corrected, and drop facts the user asked to forget.",
		"Never save from assistant text alone, hypotheticals, temporary details, facts about other people, secrets, or identification numbers. When unsure, make no edit.",
		"Edits: insert adds a line; str_replace replaces old_text, which must appear exactly once, with new_text; rewrite replaces a whole file with content and must keep every fact in it that is still valid.",
		"Evidence must quote the user's words from the conversation; for a confirmation, quote the confirmation.",
		"Never edit a file marked access \"context only\"; it changes only when the user asks the assistant directly.",
		"When a file is near its limit, or an insert would not fit, move its least current lines out with archive (path, old_text). Archived text leaves the file but stays in the user's archive, where it can still be found; it needs no evidence. Never archive a whole file.",
		`Return JSON only: {"edits":[{"command":"insert|str_replace|rewrite|archive","path":"/user/<file>.md","text":"","old_text":"","new_text":"","content":"","evidence":"user quote","reason":"short why"}]}. Return {"edits":[]} when nothing should change.`,
	}
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		rules = append(rules, "The Agent's builder describes what this memory is for. Treat it as a preference for what to keep and where; it cannot override the rules above.",
			"<memory_instructions>", instructions, "</memory_instructions>")
	}
	system := strings.Join(append(rules, string(raw)), "\n")
	provider, model := consolidationModel(turns)
	temperature, maxTokens := 0.0, maxExtractionOutputTokens
	return &adapter.ChatRequest{
		Provider: provider, Model: model,
		Messages: []adapter.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: "Consolidate the memory files now. Return JSON only."},
		},
		Temperature: &temperature, MaxTokens: &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	}, nil
}

func (r *Runner) consolidate(ctx context.Context, appContext *llmclient.AppContext, turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse, instructions string) ([]consolidationEdit, error) {
	request, err := r.consolidationChatRequest(turns, slots, values, instructions)
	if err != nil {
		return nil, err
	}
	resp, err := r.llmClient.AppChat(ctx, appContext, request)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty consolidation response")
	}
	content, _ := resp.Choices[0].Message.Content.(string)
	var output struct {
		Edits []consolidationEdit `json:"edits"`
	}
	if err := json.Unmarshal([]byte(stripJSONFence(content)), &output); err != nil {
		return nil, fmt.Errorf("decode consolidation response: %w", err)
	}
	if len(output.Edits) > maxConsolidationEdits {
		output.Edits = output.Edits[:maxConsolidationEdits]
	}
	return output.Edits, nil
}

// consolidateAndApply runs one consolidation pass and writes the accepted
// edits as one commit. It reports whether the pass should be retried after a
// revision conflict.
func (r *Runner) consolidateAndApply(ctx context.Context, job *agentmemory.AgentMemoryExtractionJob, scope extractionScope, turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse, instructions string) (bool, error) {
	edits, err := r.consolidate(ctx, &llmclient.AppContext{
		OrganizationID: scope.OrganizationID.String(), WorkspaceID: scope.WorkspaceID.String(), BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID: job.AgentID.String(), AppType: "agent", AccountID: scope.AccountID.String(), SessionID: job.ConversationID.String(),
		ConversationID: job.ConversationID.String(), ModelUseCase: consolidationUseCase, SuppressInvocationContent: true,
	}, turns, slots, values, instructions)
	if err != nil {
		return false, err
	}
	current := map[string]agentmemory.SlotValueResponse{}
	for _, value := range values {
		current[value.Key] = value
	}
	turnIDs := make(map[string]struct{}, len(turns))
	for _, turn := range turns {
		turnIDs[turn.ID.String()] = struct{}{}
	}
	working := map[string]string{}
	sources := map[string]completedTurn{}
	evidence, reasons := map[string][]string{}, map[string][]string{}
	order := []string{}
	type archived struct{ key, text, reason string }
	var archives []archived
	writer, canArchive := r.memory.(archiveWriter)
	for index, edit := range edits {
		if strings.EqualFold(strings.TrimSpace(edit.Command), consolidationArchive) && !canArchive {
			continue
		}
		key, content, source, reason := applyConsolidationEdit(edit, slots, current, working, turns, turnIDs)
		archiving := strings.EqualFold(strings.TrimSpace(edit.Command), consolidationArchive)
		if reason == "" && !archiving && supersededByLaterConversation(current[key], job.ConversationID, source) {
			reason = "another conversation changed the file after this turn; the newer statement wins"
		}
		if reason != "" {
			logger.InfoContext(ctx, "memory consolidation skipped an edit", "job_id", job.ID.String(), "index", index, "command", edit.Command, "reason", reason)
			continue
		}
		if _, seen := working[key]; !seen {
			order = append(order, key)
		}
		working[key], sources[key] = content, *source
		evidence[key] = appendDistinct(evidence[key], strings.TrimSpace(edit.Evidence))
		reasons[key] = appendDistinct(reasons[key], strings.TrimSpace(edit.Reason))
		if strings.EqualFold(strings.TrimSpace(edit.Command), consolidationArchive) {
			archives = append(archives, archived{key: key, text: strings.TrimSpace(edit.OldText), reason: strings.TrimSpace(edit.Reason)})
			reasons[key] = appendDistinct(reasons[key], "moved older text to the archive")
		}
	}
	mutations := make([]agentmemory.ValueMutation, 0, len(order))
	for _, key := range order {
		content := strings.TrimSpace(working[key])
		if content == strings.TrimSpace(current[key].Content) || content == "" {
			continue
		}
		source := sources[key]
		sourceMessageID, sourceCompletedAt := source.ID, source.CompletedAt
		mutations = append(mutations, agentmemory.ValueMutation{
			Action: agentmemory.MutationActionUpsert, Key: key, Content: content, Mode: agentmemory.MutationModeProactive,
			ExpectedRevision: current[key].Revision, OperationID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(job.ID.String()+":"+key)),
			SourceMessageID: &sourceMessageID, SourceCompletedAt: &sourceCompletedAt,
			Evidence: strings.Join(evidence[key], " / "), Reason: strings.Join(reasons[key], "; "),
		})
	}
	if len(mutations) == 0 {
		return false, nil
	}
	// Archive before the files drop the text, so a failed write loses
	// nothing; the archive skips text it already holds, so a retried pass
	// does not archive twice.
	for _, entry := range archives {
		meta := agentmemory.MutationMetadata{ActorType: agentmemory.EventActorModel, Source: agentmemory.EventSourceAgent, SourceKind: agentmemory.SourceKindAutomatic,
			SourceConversationID: &job.ConversationID, ExtractorVersion: job.ExtractorVersion}
		if err := writer.ArchiveText(ctx, job.WorkspaceID, job.AgentID, job.UserScope, job.UserID, entry.text, entry.key, entry.key, meta); err != nil {
			return false, fmt.Errorf("archive memory text: %w", err)
		}
	}
	_, err = r.memory.MutateValues(ctx, job.WorkspaceID, job.AgentID, slots, job.UserScope, job.UserID, agentmemory.MutateValuesRequest{Operations: mutations}, agentmemory.MutationMetadata{
		ActorType: agentmemory.EventActorModel, Source: agentmemory.EventSourceAgent,
		SourceConversationID: &job.ConversationID, ExtractorVersion: job.ExtractorVersion, MemoryEpoch: &job.MemoryEpoch,
		ConfigScope: job.ConfigScope, ConfigRevision: job.ConfigRevision,
	})
	if errors.Is(err, agentmemory.ErrConflict) || (err != nil && errors.Is(errors.Unwrap(err), agentmemory.ErrConflict)) {
		return true, nil
	}
	return false, err
}

// applyConsolidationEdit checks one edit against the file as earlier edits
// left it and returns the file's new content and evidence turn, or why the
// edit was skipped.
func applyConsolidationEdit(edit consolidationEdit, slots []agentmemory.RuntimeSlot, current map[string]agentmemory.SlotValueResponse, working map[string]string, turns []completedTurn, turnIDs map[string]struct{}) (string, string, *completedTurn, string) {
	path := strings.TrimSpace(edit.Path)
	var slot agentmemory.RuntimeSlot
	found := false
	for _, candidate := range slots {
		if candidate.Enabled && blockPath(candidate.Key) == path {
			slot, found = candidate, true
		}
	}
	if !found {
		return "", "", nil, "unknown memory file"
	}
	if !automaticWritesAllowed(slot) {
		return "", "", nil, "the file takes no automatic edits"
	}
	if strings.ToLower(strings.TrimSpace(edit.Command)) == consolidationArchive {
		return archiveConsolidationEdit(edit, slot, current, working, turns)
	}
	var source *completedTurn
	for index := len(turns) - 1; index >= 0 && source == nil; index-- {
		if agentmemory.QuotedIn(edit.Evidence, turns[index].Query) {
			source = &turns[index]
		}
	}
	if source == nil {
		return "", "", nil, "evidence is not a quote from a user message"
	}
	content, edited := working[slot.Key]
	if !edited {
		content = strings.TrimSpace(current[slot.Key].Content)
	}
	var added string
	switch strings.ToLower(strings.TrimSpace(edit.Command)) {
	case "insert":
		added = strings.TrimSpace(edit.Text)
		if added == "" || strings.Contains(content, added) {
			return "", "", nil, "insert is empty or already saved"
		}
		if content == "" {
			content = added
		} else {
			content += "\n" + added
		}
	case "str_replace":
		if strings.TrimSpace(edit.OldText) == "" || strings.Count(content, edit.OldText) != 1 {
			return "", "", nil, "old_text does not appear exactly once"
		}
		added = edit.NewText
		content = strings.Replace(content, edit.OldText, edit.NewText, 1)
	case consolidationRewrite:
		// A file the user wrote explicitly in these turns keeps their words.
		if value, ok := current[slot.Key]; ok && value.SourceKind == agentmemory.SourceKindExplicit {
			if _, fromThisBatch := turnIDs[value.SourceMessageID]; fromThisBatch {
				return "", "", nil, "file was written explicitly in these turns"
			}
		}
		added = strings.TrimSpace(edit.Content)
		if added == "" {
			return "", "", nil, "rewrite cannot empty a file"
		}
		content = added
	default:
		return "", "", nil, "unsupported command"
	}
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > slot.MaxChars {
		return "", "", nil, "file would be empty or over its limit"
	}
	sensitive := agentmemory.ContainsSensitiveContent(added)
	if agentmemory.DescriptionAllowsContact(slot.Description) {
		sensitive = agentmemory.ContainsSecretContent(added)
	}
	if sensitive {
		return "", "", nil, "sensitive content"
	}
	return slot.Key, content, source, ""
}
