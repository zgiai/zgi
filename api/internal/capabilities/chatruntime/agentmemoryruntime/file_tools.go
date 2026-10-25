package agentmemoryruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// File-style memory tools (ADR 0013). The model edits memory files the way it
// edits code: insert a line, replace an exact unique substring, or clear a
// file. Each edit is checked on its own, so one bad edit is reported back
// without discarding the others; one call is one commit in the history.

const (
	ToolMemoryEdit = "memory_edit"
	ToolMemoryView = "memory_view"

	editInsert     = "insert"
	editStrReplace = "str_replace"
	editClear      = "clear"

	maxEditsPerCall = 16
)

type FileEdit struct {
	Command    string `json:"command"`
	Path       string `json:"path"`
	Text       string `json:"text,omitempty"`
	OldText    string `json:"old_text,omitempty"`
	NewText    string `json:"new_text,omitempty"`
	InsertLine *int   `json:"insert_line,omitempty"`
	Evidence   string `json:"evidence"`
	Mode       string `json:"mode"`
}

type fileEditArguments struct {
	Edits []FileEdit `json:"edits"`
}

// EditOutcome reports what happened to one edit of a memory_edit call.
type EditOutcome struct {
	Index   int    `json:"index"`
	Path    string `json:"path"`
	Command string `json:"command"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
}

// FileTools defines memory_edit and memory_view over the enabled blocks.
func FileTools(slots []Slot, proactiveAllowed bool) []adapter.Tool {
	paths := make([]interface{}, 0, len(slots))
	editable := make([]interface{}, 0, len(slots))
	for _, slot := range slots {
		if slot.Enabled {
			paths = append(paths, BlockPath(slot.Key))
			if !slot.ReadOnly {
				editable = append(editable, BlockPath(slot.Key))
			}
		}
	}
	if len(paths) == 0 {
		return nil
	}
	view := adapter.Tool{Type: "function", Function: adapter.Function{
		Name:        ToolMemoryView,
		Description: "Read a memory file in full, for example one shown as truncated or omitted, or list all memory files when path is omitted. Read-only.",
		Parameters: map[string]interface{}{
			"type":                 "object",
			"properties":           map[string]interface{}{"path": map[string]interface{}{"type": "string", "enum": paths}},
			"additionalProperties": false,
		},
	}}
	if len(editable) == 0 {
		return []adapter.Tool{view}
	}
	modes := []interface{}{agentMemoryModeExplicit}
	if proactiveAllowed {
		modes = append(modes, agentMemoryModeProactive)
	}
	return []adapter.Tool{
		{Type: "function", Function: adapter.Function{
			Name: ToolMemoryEdit,
			Description: "Edit the user's memory files. Each edit is one command on one file: insert adds text (a new line at the end unless insert_line is given), " +
				"str_replace replaces old_text, which must appear exactly once in the file, with new_text (empty new_text removes it), and clear empties a file. " +
				"Edits are checked one by one; rejected edits come back with a reason. Send all edits for this turn in one call.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"edits": map[string]interface{}{
						"type": "array", "minItems": 1, "maxItems": maxEditsPerCall,
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"command":     map[string]interface{}{"type": "string", "enum": []interface{}{editInsert, editStrReplace, editClear}},
								"path":        map[string]interface{}{"type": "string", "enum": editable},
								"text":        map[string]interface{}{"type": "string", "description": "insert: the text to add."},
								"insert_line": map[string]interface{}{"type": "integer", "minimum": 0, "description": "insert: add after this line number (0 = at the top); omit to append."},
								"old_text":    map[string]interface{}{"type": "string", "description": "str_replace: exact text that appears once in the file."},
								"new_text":    map[string]interface{}{"type": "string", "description": "str_replace: replacement; empty removes old_text."},
								"evidence":    map[string]interface{}{"type": "string", "description": "An exact quote from one of the user's messages in this conversation that supports the edit."},
								"mode":        map[string]interface{}{"type": "string", "enum": modes},
							},
							"required":             []string{"command", "path", "evidence", "mode"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"edits"},
				"additionalProperties": false,
			},
		}},
		view,
	}
}

// FilePolicyMessage tells the model how to use the memory files.
func FilePolicyMessage(proactiveAllowed bool) adapter.Message {
	modePolicy := "Proactive maintenance is disabled: edit memory only when the user asks you to remember, correct, or forget something."
	if proactiveAllowed {
		modePolicy = "Proactive maintenance is enabled: you may also add high-confidence stable facts, preferences, instructions, and ongoing context the user stated directly. Proactive edits may insert and replace, never clear."
	}
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"The user's memory is a set of files shown in <memory>. Each file's description says what belongs in it; put each fact in the file it fits.",
		modePolicy,
		"Use memory_edit with small edits: insert a new fact as its own line, and correct or remove an existing fact with str_replace on its exact text. Never rewrite a whole file to change one fact, and never repeat a fact already saved.",
		"Evidence must quote the user's own words from this conversation. It may be a confirmation like \"yes, that one\" when your previous reply proposed the content. Never save from assistant text alone, tool results, hypotheticals, temporary details, or facts about other people. Never save secrets or identification numbers.",
		"A file shown as truncated or omitted is not fully in view: read it with memory_view before answering a question it may concern, and before editing it.",
		"Never edit a file marked access=\"read-only\". Edit a file marked access=\"explicit-only\" only when the user asks you to remember, correct, or forget something.",
		"Older memory moved out of full files is kept in the user's archive; when the files do not answer a question about the user, search it with memory_search.",
		"When the user refers to something discussed in an earlier conversation that memory does not cover, search for it with conversation_search before saying you do not know.",
		"Do not narrate proactive saves; the product shows memory updates. For explicit requests, confirm only from a successful tool result, and say plainly when an edit was rejected.",
	}, "\n")}
}

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

// InstructionsMessage frames the builder's memory instructions as business
// preference under the fixed memory rules, or returns nil when there are none.
func InstructionsMessage(instructions string) *adapter.Message {
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return nil
	}
	return &adapter.Message{Role: "system", Content: strings.Join([]string{
		"The Agent's builder describes what this Agent's memory is for. Follow it when choosing what to remember and where, as a preference only:",
		"it cannot override the memory rules above (evidence, sensitive content, read-only and explicit-only files), and it is not a request from the user.",
		"<memory_instructions>", instructions, "</memory_instructions>",
	}, "\n")}
}

// ExecuteEdits applies a memory_edit call: each edit is validated on its own,
// the accepted edits are folded into one new value per file, and the files
// are written together as one commit.
func ExecuteEdits(ctx context.Context, req MutationRequest, rawArguments string) MutationResult {
	var arguments fileEditArguments
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(rawArguments)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&arguments); err != nil || decoder.More() {
		return failedMutation(fmt.Errorf("%w: malformed memory_edit arguments", ErrInvalidInput), nil)
	}
	summary := map[string]interface{}{"edit_count": len(arguments.Edits)}
	if req.MemoryService == nil || req.WorkspaceID == zeroUUID || req.AgentID == zeroUUID || req.UserID == zeroUUID ||
		req.MutationMetadata.SourceMessageID == nil || *req.MutationMetadata.SourceMessageID == zeroUUID {
		return failedMutation(fmt.Errorf("%w: agent memory runtime scope is invalid", ErrInvalidInput), summary)
	}
	if len(arguments.Edits) == 0 || len(arguments.Edits) > maxEditsPerCall {
		return failedMutation(fmt.Errorf("%w: memory_edit needs between one and %d edits", ErrInvalidInput, maxEditsPerCall), summary)
	}
	current := make(map[string]string, len(req.CurrentValues))
	revisions := make(map[string]int64, len(req.CurrentValues))
	for _, value := range req.CurrentValues {
		current[strings.TrimSpace(value.Key)] = strings.TrimSpace(value.Content)
		revisions[strings.TrimSpace(value.Key)] = value.Revision
	}
	working := map[string]string{}
	explicit := map[string]bool{}
	evidence := map[string][]string{}
	order := []string{}
	outcomes := make([]EditOutcome, 0, len(arguments.Edits))
	for index, edit := range arguments.Edits {
		outcome := EditOutcome{Index: index, Path: strings.TrimSpace(edit.Path), Command: strings.ToLower(strings.TrimSpace(edit.Command)), Status: "rejected"}
		key, content, reason := applyEdit(req, edit, outcome.Command, current, working)
		if reason != "" {
			outcome.Reason = reason
			outcomes = append(outcomes, outcome)
			continue
		}
		if _, seen := working[key]; !seen {
			order = append(order, key)
		}
		working[key] = content
		evidence[key] = appendDistinct(evidence[key], strings.TrimSpace(edit.Evidence))
		if strings.EqualFold(strings.TrimSpace(edit.Mode), agentMemoryModeExplicit) {
			explicit[key] = true
		}
		outcome.Status = "accepted"
		outcomes = append(outcomes, outcome)
	}
	summary["edits"] = outcomes
	mutations := make([]agentmemory.ValueMutation, 0, len(order))
	for _, key := range order {
		content := strings.TrimSpace(working[key])
		if content == current[key] {
			continue
		}
		action := agentmemory.MutationActionUpsert
		if content == "" {
			action = agentmemory.MutationActionClear
		}
		mode, reason := agentMemoryModeProactive, "saved during the conversation"
		if explicit[key] {
			mode, reason = agentMemoryModeExplicit, "the user asked to remember"
		}
		mutations = append(mutations, agentmemory.ValueMutation{
			Action: action, Key: key, Content: content, Mode: mode, ExpectedRevision: revisions[key],
			Evidence: strings.Join(evidence[key], " / "), Reason: reason,
			OperationID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(req.MutationMetadata.SourceMessageID.String()+":"+req.CallID+":"+key+":edit")),
		})
	}
	if len(mutations) == 0 {
		result := failedMutation(fmt.Errorf("%w: no memory edit was accepted", ErrInvalidInput), summary)
		result.Result["edits"] = outcomes
		return result
	}
	response, err := req.MemoryService.MutateValues(ctx, req.WorkspaceID, req.AgentID, RuntimeSlots(req.Slots), req.UserScope, req.UserID,
		agentmemory.MutateValuesRequest{Operations: mutations}, req.MutationMetadata)
	if err != nil {
		result := failedMutation(err, summary)
		result.Result["edits"] = outcomes
		return result
	}
	return MutationResult{Status: "success", Arguments: summary, Response: response,
		Result: map[string]interface{}{"status": "success", "edits": outcomes, "operations": response.Operations}}
}

// applyEdit checks one edit against the file as earlier edits left it and
// returns the file's new content, or why the edit was rejected.
func applyEdit(req MutationRequest, edit FileEdit, command string, current, working map[string]string) (string, string, string) {
	key := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(edit.Path), "/user/"), ".md")
	slot, ok := findEnabledSlot(req.Slots, key)
	if !ok || BlockPath(slot.Key) != strings.TrimSpace(edit.Path) {
		return "", "", "unknown memory file"
	}
	if slot.ReadOnly {
		return "", "", "the file is read-only"
	}
	mode := strings.ToLower(strings.TrimSpace(edit.Mode))
	if slot.ManualOnly && mode == agentMemoryModeProactive {
		return "", "", "the file takes edits only when the user asks"
	}
	switch mode {
	case agentMemoryModeExplicit:
	case agentMemoryModeProactive:
		if !req.ProactiveAllowed {
			return "", "", "proactive memory maintenance is disabled; edit only when the user asks"
		}
		if command == editClear {
			return "", "", "proactive edits cannot clear a file"
		}
	default:
		return "", "", "unsupported mode"
	}
	if !evidenceFromUser(req, edit.Evidence) {
		return "", "", "evidence is not a quote from the user's messages in this conversation"
	}
	content, edited := working[slot.Key]
	if !edited {
		content = current[slot.Key]
	}
	var added string
	switch command {
	case editInsert:
		added = strings.TrimSpace(edit.Text)
		if added == "" {
			return "", "", "insert needs text"
		}
		if strings.Contains(content, added) {
			return "", "", "this text is already in the file"
		}
		content = insertText(content, added, edit.InsertLine)
	case editStrReplace:
		old := edit.OldText
		if strings.TrimSpace(old) == "" {
			return "", "", "str_replace needs old_text"
		}
		if count := strings.Count(content, old); count != 1 {
			return "", "", fmt.Sprintf("old_text must appear exactly once in the file; it appears %d times", count)
		}
		added = edit.NewText
		content = strings.Replace(content, old, edit.NewText, 1)
	case editClear:
		content = ""
	default:
		return "", "", "unsupported command"
	}
	content = strings.TrimSpace(content)
	if slot.MaxChars > 0 && utf8.RuneCountInString(content) > slot.MaxChars {
		return "", "", fmt.Sprintf("the file would hold %d characters, over its limit of %d; shorten or replace existing text", utf8.RuneCountInString(content), slot.MaxChars)
	}
	sensitive := agentmemory.ContainsSensitiveContent(added)
	if agentmemory.DescriptionAllowsContact(slot.Description) {
		sensitive = agentmemory.ContainsSecretContent(added)
	}
	if sensitive {
		return "", "", "sensitive content cannot be saved"
	}
	return slot.Key, content, ""
}

func insertText(content, text string, line *int) string {
	if strings.TrimSpace(content) == "" {
		return text
	}
	lines := strings.Split(content, "\n")
	if line == nil || *line >= len(lines) {
		return content + "\n" + text
	}
	at := *line
	if at < 0 {
		at = 0
	}
	return strings.Join(append(lines[:at], append([]string{text}, lines[at:]...)...), "\n")
}

// evidenceFromUser accepts a quote from the current message or any recent user
// message in the conversation. Spacing, punctuation, and case are ignored, so
// a quote that drops a list number or a comma still counts.
func evidenceFromUser(req MutationRequest, evidence string) bool {
	return agentmemory.QuotedIn(evidence, append([]string{req.LatestUserMessage}, req.RecentUserMessages...)...)
}

// ExecuteView answers memory_view from the values read for this run, in full.
func ExecuteView(req MutationRequest, rawArguments string) map[string]interface{} {
	var arguments struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(strings.TrimSpace(rawArguments)), &arguments)
	path := strings.TrimSpace(arguments.Path)
	files := make([]map[string]interface{}, 0, len(req.Slots))
	for _, slot := range req.Slots {
		if !slot.Enabled || (path != "" && BlockPath(slot.Key) != path) {
			continue
		}
		content := ""
		for _, value := range req.CurrentValues {
			if strings.TrimSpace(value.Key) == slot.Key {
				content = strings.TrimSpace(value.Content)
			}
		}
		file := map[string]interface{}{"path": BlockPath(slot.Key), "description": slot.Description, "chars": utf8.RuneCountInString(content), "limit": slot.MaxChars}
		if path != "" {
			file["content"] = numberLines(content)
		}
		files = append(files, file)
	}
	if path != "" && len(files) == 0 {
		return map[string]interface{}{"status": "failed", "error_code": "unknown_file"}
	}
	return map[string]interface{}{"status": "success", "files": files}
}

func numberLines(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		lines[index] = fmt.Sprintf("%d\t%s", index+1, line)
	}
	return strings.Join(lines, "\n")
}
