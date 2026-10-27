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
	editArchive    = "archive"

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

// FileTools defines memory_edit and memory_view. The model names files
// itself (ADR 0017); archive offers the archive command, for stores that keep
// an archive.
func FileTools(proactiveAllowed, archive bool) []adapter.Tool {
	pathDescription := "A memory file: /user/<name>.md, where name is lowercase letters, digits, and underscores, starting with a letter. Editing a file that does not exist creates it."
	view := adapter.Tool{Type: "function", Function: adapter.Function{
		Name:        ToolMemoryView,
		Description: "Read a memory file in full, for example one shown as truncated or omitted, or list all memory files when path is omitted. Read-only.",
		Parameters: map[string]interface{}{
			"type":                 "object",
			"properties":           map[string]interface{}{"path": map[string]interface{}{"type": "string", "description": "An existing memory file; omit to list them."}},
			"additionalProperties": false,
		},
	}}
	modes := []interface{}{agentMemoryModeExplicit}
	if proactiveAllowed {
		modes = append(modes, agentMemoryModeProactive)
	}
	commands := []interface{}{editInsert, editStrReplace, editClear}
	description := "Edit the user's memory files. Each edit is one command on one file: insert adds text (a new line at the end unless insert_line is given; on a new file it creates the file), " +
		"str_replace replaces old_text, which must appear exactly once in the file, with new_text (empty new_text removes it), and clear deletes a file. "
	oldText := "str_replace: exact text that appears once in the file."
	if archive {
		commands = append(commands, editArchive)
		description += "archive moves old_text, which must appear exactly once, out of a file into the user's archive, where it can still be found; use it to make room instead of deleting still-valid facts. "
		oldText = "str_replace and archive: exact text that appears once in the file."
	}
	return []adapter.Tool{
		{Type: "function", Function: adapter.Function{
			Name:        ToolMemoryEdit,
			Description: description + "Edits are checked one by one; rejected edits come back with a reason. Send all edits for this turn in one call.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"edits": map[string]interface{}{
						"type": "array", "minItems": 1, "maxItems": maxEditsPerCall,
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"command":     map[string]interface{}{"type": "string", "enum": commands},
								"path":        map[string]interface{}{"type": "string", "description": pathDescription},
								"text":        map[string]interface{}{"type": "string", "description": "insert: the text to add."},
								"insert_line": map[string]interface{}{"type": "integer", "minimum": 0, "description": "insert: add after this line number (0 = at the top); omit to append."},
								"old_text":    map[string]interface{}{"type": "string", "description": oldText},
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
		modePolicy = "Proactive maintenance is enabled. In the same turn as your answer, save what the user reveals about themselves that will still be true later, even in passing and even when they are asking about something else: their job and work habits (a nurse on night shifts, coding ten hours a day), family, where they live, the tools and languages they use, and feedback on how you answer (\"too long\" means they prefer shorter answers). Skip one-off requests, moods, and today's plans. Proactive edits may insert and replace, never clear."
	}
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		fmt.Sprintf("The user's memory is a set of files shown in <memory>; you name and organize them. Keep one file per topic with a short descriptive name, such as /user/profile.md or /user/preferences.md, add to an existing file when a fact fits it, and create a new file only for a new topic. All files together hold at most %d characters.", agentmemory.MaxMemoryChars),
		modePolicy,
		"Use memory_edit with small edits: insert a new fact as its own line, and correct or remove an existing fact with str_replace on its exact text. Never rewrite a whole file to change one fact, and never repeat a fact already saved.",
		"When a fact changes, replace the old value instead of adding a second one. When the user asks you to forget something, remove every line about it, in every file.",
		"When the user asks what you know or remember about them, answer from every relevant line in memory, give the most recent value of anything that changed, and never mention what they asked you to forget.",
		"Evidence must quote the user's own words from this conversation. It may be a confirmation like \"yes, that one\" when your previous reply proposed the content. Never save from assistant text alone, tool results, hypotheticals, temporary details, or facts about other people. Never save secrets or identification numbers.",
		"A file shown as truncated or omitted is not fully in view: read it with memory_view before answering a question it may concern, and before editing it.",
		"When memory is too full for a new fact, move the least current lines out with the archive command instead of deleting facts that are still valid.",
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
		"The Agent's builder describes what this Agent's memory is for. Follow it when choosing what to remember and how to organize the files, as a preference only:",
		"it cannot override the memory rules above (evidence, sensitive content), and it is not a request from the user.",
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
	archived := map[string][]string{}
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
		if outcome.Command == editArchive {
			archived[key] = append(archived[key], strings.TrimSpace(edit.OldText))
			outcome.Status = "accepted"
			outcomes = append(outcomes, outcome)
			continue
		}
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
		if len(archived[key]) > 0 {
			reason += "; moved older text to the archive"
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
	// Archive before the files drop the text, so a failed write loses
	// nothing; the archive skips text it already holds.
	for _, mutation := range mutations {
		for _, text := range archived[mutation.Key] {
			if err := req.Archive(ctx, text, mutation.Key); err != nil {
				result := failedMutation(err, summary)
				result.Result["edits"] = outcomes
				return result
			}
		}
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
	if !agentmemory.ValidMemoryKey(key) || BlockPath(key) != strings.TrimSpace(edit.Path) {
		return "", "", "a memory file path is /user/<name>.md, with a name of lowercase letters, digits, and underscores starting with a letter"
	}
	slot := Slot{Key: key}
	mode := strings.ToLower(strings.TrimSpace(edit.Mode))
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
	content, edited := working[slot.Key]
	if !edited {
		content = current[slot.Key]
	}
	if command == editArchive {
		// Archiving moves text without changing any fact, so it needs no
		// evidence.
		if req.Archive == nil {
			return "", "", "this memory has no archive"
		}
		old := strings.TrimSpace(edit.OldText)
		if old == "" {
			return "", "", "archive needs old_text"
		}
		if count := strings.Count(content, old); count != 1 {
			return "", "", fmt.Sprintf("old_text must appear exactly once in the file; it appears %d times", count)
		}
		content = strings.TrimSpace(strings.Replace(content, old, "", 1))
		if content == "" {
			return "", "", "archive cannot empty a file"
		}
		return slot.Key, content, ""
	}
	if !evidenceFromUser(req, edit.Evidence) {
		return "", "", "evidence is not a quote from the user's messages in this conversation"
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
	if total := memoryTotal(slot.Key, content, current, working); total > agentmemory.MaxMemoryChars {
		hint := "shorten or replace existing text"
		if req.Archive != nil {
			hint = "move older lines out with archive first, or shorten existing text"
		}
		return "", "", fmt.Sprintf("memory would hold %d characters, over its limit of %d; %s", total, agentmemory.MaxMemoryChars, hint)
	}
	if agentmemory.ContainsSensitiveContent(added) {
		return "", "", "sensitive content cannot be saved"
	}
	return slot.Key, content, ""
}

// memoryTotal is the size of all memory files with key holding content.
func memoryTotal(key, content string, current, working map[string]string) int {
	total := utf8.RuneCountInString(content)
	for name, saved := range current {
		if name == key {
			continue
		}
		if edited, ok := working[name]; ok {
			saved = edited
		}
		total += utf8.RuneCountInString(saved)
	}
	for name, edited := range working {
		if _, saved := current[name]; !saved && name != key {
			total += utf8.RuneCountInString(edited)
		}
	}
	return total
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
	files := make([]map[string]interface{}, 0, len(req.CurrentValues))
	total := 0
	for _, value := range req.CurrentValues {
		key, content := strings.TrimSpace(value.Key), strings.TrimSpace(value.Content)
		if key == "" || content == "" {
			continue
		}
		total += utf8.RuneCountInString(content)
		if path != "" && BlockPath(key) != path {
			continue
		}
		file := map[string]interface{}{"path": BlockPath(key), "chars": utf8.RuneCountInString(content)}
		if path != "" {
			file["content"] = numberLines(content)
		}
		files = append(files, file)
	}
	if path != "" && len(files) == 0 {
		return map[string]interface{}{"status": "failed", "error_code": "unknown_file"}
	}
	return map[string]interface{}{"status": "success", "files": files, "used_chars": total, "limit_chars": agentmemory.MaxMemoryChars}
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
