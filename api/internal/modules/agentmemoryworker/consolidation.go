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

func (r *Runner) consolidationChatRequest(turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse) (*adapter.ChatRequest, error) {
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
		files = append(files, map[string]interface{}{"path": blockPath(slot.Key), "description": slot.Description, "limit": slot.MaxChars, "content": contents[slot.Key]})
	}
	raw, err := json.Marshal(map[string]interface{}{"conversation": conversation, "files": files})
	if err != nil {
		return nil, fmt.Errorf("encode consolidation prompt: %w", err)
	}
	system := strings.Join([]string{
		"You keep one user's memory files accurate and tidy after a conversation. Each file's description says what belongs in it.",
		"Add stable facts, preferences, standing instructions, and ongoing context that the user stated, or confirmed when the assistant proposed them. Put each fact in the file whose description fits it.",
		"Fix what is already saved: remove duplicates, replace facts the user corrected, and drop facts the user asked to forget.",
		"Never save from assistant text alone, hypotheticals, temporary details, facts about other people, secrets, or identification numbers. When unsure, make no edit.",
		"Edits: insert adds a line; str_replace replaces old_text, which must appear exactly once, with new_text; rewrite replaces a whole file with content and must keep every fact in it that is still valid.",
		"Evidence must quote the user's words from the conversation; for a confirmation, quote the confirmation.",
		`Return JSON only: {"edits":[{"command":"insert|str_replace|rewrite","path":"/user/<file>.md","text":"","old_text":"","new_text":"","content":"","evidence":"user quote","reason":"short why"}]}. Return {"edits":[]} when nothing should change.`,
		string(raw),
	}, "\n")
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

func (r *Runner) consolidate(ctx context.Context, job *agentmemory.AgentMemoryExtractionJob, scope extractionScope, turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse) ([]consolidationEdit, error) {
	request, err := r.consolidationChatRequest(turns, slots, values)
	if err != nil {
		return nil, err
	}
	resp, err := r.llmClient.AppChat(ctx, &llmclient.AppContext{
		OrganizationID: scope.OrganizationID.String(), WorkspaceID: scope.WorkspaceID.String(), BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID: job.AgentID.String(), AppType: "agent", AccountID: scope.AccountID.String(), SessionID: job.ConversationID.String(),
		ConversationID: job.ConversationID.String(), ModelUseCase: consolidationUseCase, SuppressInvocationContent: true,
	}, request)
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
func (r *Runner) consolidateAndApply(ctx context.Context, job *agentmemory.AgentMemoryExtractionJob, scope extractionScope, turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse) (bool, error) {
	edits, err := r.consolidate(ctx, job, scope, turns, slots, values)
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
	order := []string{}
	for index, edit := range edits {
		key, content, source, reason := applyConsolidationEdit(edit, slots, current, working, turns, turnIDs)
		if reason != "" {
			logger.InfoContext(ctx, "memory consolidation skipped an edit", "job_id", job.ID.String(), "index", index, "command", edit.Command, "reason", reason)
			continue
		}
		if _, seen := working[key]; !seen {
			order = append(order, key)
		}
		working[key], sources[key] = content, *source
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
		})
	}
	if len(mutations) == 0 {
		return false, nil
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
