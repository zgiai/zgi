package agentmemoryworker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
)

// Preview (ADR 0013) runs one consolidation pass over a sample conversation
// with a draft memory configuration and reports what it would do, without
// writing anything: the edits the model proposed, which were accepted or
// rejected and why, and each file before and after.

const maxPreviewTurns = 12

type PreviewTurn struct {
	User      string `json:"user"`
	Assistant string `json:"assistant,omitempty"`
}

type PreviewRequest struct {
	Slots        []agentmemory.RuntimeSlot
	Instructions string
	Turns        []PreviewTurn
	// Current is the memory before the conversation, by block key.
	Current map[string]string
	// Provider and Model choose the model when no consolidation model is
	// configured for the deployment.
	Provider   string
	Model      string
	AppContext *llmclient.AppContext
}

type PreviewEdit struct {
	Command  string `json:"command"`
	Path     string `json:"path"`
	Text     string `json:"text,omitempty"`
	OldText  string `json:"old_text,omitempty"`
	NewText  string `json:"new_text,omitempty"`
	Content  string `json:"content,omitempty"`
	Evidence string `json:"evidence"`
	Reason   string `json:"reason,omitempty"`
	Status   string `json:"status"`
	// Rejection says which rule rejected the edit.
	Rejection string `json:"rejection,omitempty"`
}

type PreviewFile struct {
	Key    string `json:"key"`
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type PreviewResult struct {
	Provider string        `json:"provider"`
	Model    string        `json:"model"`
	Edits    []PreviewEdit `json:"edits"`
	Files    []PreviewFile `json:"files"`
}

// Preview runs the consolidation pass described above. It needs an LLM client
// and at least one turn with user text.
func Preview(ctx context.Context, client llmclient.LLMClient, req PreviewRequest) (*PreviewResult, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: memory preview needs a model client", agentmemory.ErrInvalidInput)
	}
	if len(req.Turns) == 0 || len(req.Turns) > maxPreviewTurns {
		return nil, fmt.Errorf("%w: memory preview needs between one and %d turns", agentmemory.ErrInvalidInput, maxPreviewTurns)
	}
	now := time.Now().UTC()
	turns := make([]completedTurn, 0, len(req.Turns))
	for index, turn := range req.Turns {
		user := strings.TrimSpace(turn.User)
		if user == "" {
			return nil, fmt.Errorf("%w: preview turn %d has no user text", agentmemory.ErrInvalidInput, index+1)
		}
		turns = append(turns, completedTurn{
			ID: uuid.New(), Query: user, Answer: strings.TrimSpace(turn.Assistant),
			CompletedAt: now, ModelProvider: req.Provider, ModelName: req.Model,
		})
	}
	values := make([]agentmemory.SlotValueResponse, 0, len(req.Slots))
	for _, slot := range req.Slots {
		values = append(values, agentmemory.SlotValueResponse{
			SlotResponse: agentmemory.SlotResponse{Key: slot.Key, Description: slot.Description, MaxChars: slot.MaxChars, ReadOnly: slot.ReadOnly, AutoWrite: !slot.ManualOnly},
			Content:      strings.TrimSpace(req.Current[slot.Key]),
		})
	}
	runner := &Runner{llmClient: client}
	appContext := req.AppContext
	if appContext != nil {
		copied := *appContext
		copied.ModelUseCase, copied.SuppressInvocationContent = consolidationUseCase, true
		appContext = &copied
	}
	edits, err := runner.consolidate(ctx, appContext, turns, req.Slots, values, req.Instructions)
	if err != nil {
		return nil, err
	}
	provider, model := consolidationModel(turns)
	result := &PreviewResult{Provider: provider, Model: model, Edits: make([]PreviewEdit, 0, len(edits))}
	current := make(map[string]agentmemory.SlotValueResponse, len(values))
	for _, value := range values {
		current[value.Key] = value
	}
	working := map[string]string{}
	for _, edit := range edits {
		outcome := PreviewEdit{
			Command: edit.Command, Path: edit.Path, Text: edit.Text, OldText: edit.OldText, NewText: edit.NewText,
			Content: edit.Content, Evidence: edit.Evidence, Reason: edit.Reason, Status: "accepted",
		}
		key, content, _, rejection := applyConsolidationEdit(edit, req.Slots, current, working, turns, map[string]struct{}{})
		if rejection != "" {
			outcome.Status, outcome.Rejection = "rejected", rejection
		} else {
			working[key] = content
		}
		result.Edits = append(result.Edits, outcome)
	}
	for _, slot := range req.Slots {
		before := current[slot.Key].Content
		after, changed := working[slot.Key]
		if !changed {
			after = before
		}
		result.Files = append(result.Files, PreviewFile{Key: slot.Key, Path: blockPath(slot.Key), Before: before, After: after})
	}
	return result, nil
}
