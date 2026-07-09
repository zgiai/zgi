package service

import (
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
)

var managedFileTargetPattern = regexp.MustCompile(`(?i)([^\s，,。；;、：:（）()【】\[\]{}"'“”‘’<>]+?\.(?:txt|md|markdown|html|json|csv|svg|pdf|docx|xlsx|pptx))`)

type requestedManagedFileTarget struct {
	Filename  string
	Extension string
}

func managedFileTargetFromArguments(args map[string]interface{}) requestedManagedFileTarget {
	filename := normalizeManagedFileTargetName(firstNonEmptyString(
		args["filename"],
		args["output_filename"],
		args["name"],
		args["file_name"],
	))
	extension := normalizeManagedFileTargetExtension(firstNonEmptyString(
		args["format"],
		args["extension"],
		args["file_type"],
	))
	if extension == "" {
		extension = managedFileTargetExtension(filename)
	}
	if filename != "" && managedFileTargetExtension(filename) == "" && extension != "" {
		filename = filename + "." + extension
	}
	return requestedManagedFileTarget{
		Filename:  filename,
		Extension: extension,
	}
}

func managedFileTargetFromSuccessfulCall(call skillloop.SkillToolCallRef) requestedManagedFileTarget {
	if finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return managedFileTargetFromArguments(map[string]interface{}{
			"filename": firstNonEmptyString(
				call.Result["file_name"],
				call.Result["filename"],
				call.Result["name"],
				call.Arguments["filename"],
			),
			"extension": firstNonEmptyString(
				call.Result["extension"],
				call.Arguments["extension"],
			),
		})
	}
	return managedFileTargetFromArguments(generatedArtifactSaveArguments(call))
}

func managedFileTargetsMatch(left, right requestedManagedFileTarget) bool {
	if left.Filename != "" && right.Filename != "" {
		return left.Filename == right.Filename
	}
	return left.Extension != "" && right.Extension != "" && left.Extension == right.Extension
}

func managedFileTargetMatchesAny(target requestedManagedFileTarget, candidates []requestedManagedFileTarget) bool {
	for _, candidate := range candidates {
		if managedFileTargetsMatch(target, candidate) {
			return true
		}
	}
	return false
}

func managedFileTargetsFromMissingTargetLabels(labels []string) []requestedManagedFileTarget {
	targets := make([]requestedManagedFileTarget, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if strings.HasPrefix(label, "*.") {
			extension := normalizeManagedFileTargetExtension(strings.TrimPrefix(label, "*."))
			if extension != "" {
				targets = append(targets, requestedManagedFileTarget{Extension: extension})
			}
			continue
		}
		filename := normalizeManagedFileTargetName(label)
		if filename == "" {
			continue
		}
		targets = append(targets, requestedManagedFileTarget{
			Filename:  filename,
			Extension: managedFileTargetExtension(filename),
		})
	}
	return targets
}

func requestedManagedFileTargetsFromParts(parts *chatRequestParts) []requestedManagedFileTarget {
	if parts == nil {
		return nil
	}
	return explicitRequestedManagedFileTargetsFromQuery(parts.Query)
}

func requestedManagedFileTargetsFromQuery(query string) []requestedManagedFileTarget {
	targets := explicitRequestedManagedFileTargetsFromQuery(query)
	if len(targets) > 0 {
		return targets
	}
	return implicitRequestedManagedFileTargetsFromQuery(query)
}

func explicitRequestedManagedFileTargetsFromQuery(query string) []requestedManagedFileTarget {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	seen := map[string]struct{}{}
	targets := []requestedManagedFileTarget{}
	for _, match := range managedFileTargetPattern.FindAllString(query, -1) {
		filename := normalizeManagedFileTargetName(match)
		if filename == "" {
			continue
		}
		key := "name:" + filename
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, requestedManagedFileTarget{
			Filename:  filename,
			Extension: managedFileTargetExtension(filename),
		})
	}
	return targets
}

func implicitRequestedManagedFileTargetsFromQuery(query string) []requestedManagedFileTarget {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	text := normalizeConsoleNavigationQuery(query)
	if text == "" || !containsAnySubstring(text, []string{"two files", "2 files", "\u4e24\u4e2a\u6587\u4ef6", "2\u4e2a\u6587\u4ef6", "\u4e00\u4e2a\u6587\u672c", "\u4e00\u4e2asvg"}) {
		return nil
	}
	targets := []requestedManagedFileTarget{}
	if containsAnySubstring(text, []string{"txt", "text file", "\u6587\u672c\u6587\u4ef6"}) {
		targets = append(targets, requestedManagedFileTarget{Extension: "txt"})
	}
	if containsAnySubstring(text, []string{"svg"}) {
		targets = append(targets, requestedManagedFileTarget{Extension: "svg"})
	}
	return targets
}

func missingRequestedManagedFileSaveTargets(parts *chatRequestParts, calls []skillloop.SkillToolCallRef) []string {
	if parts == nil {
		return nil
	}
	targets := requestedManagedFileTargetsFromParts(parts)
	if len(targets) <= 1 {
		return nil
	}
	savedNames := map[string]int{}
	savedExtensions := map[string]int{}
	for _, call := range calls {
		if !finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
			continue
		}
		name := savedManagedFileName(call)
		if name == "" {
			continue
		}
		savedNames[name]++
		if ext := managedFileTargetExtension(name); ext != "" {
			savedExtensions[ext]++
		}
	}
	missing := []string{}
	for _, target := range targets {
		if target.Filename != "" {
			if savedNames[target.Filename] > 0 {
				continue
			}
			missing = append(missing, target.Filename)
			continue
		}
		if target.Extension != "" {
			if savedExtensions[target.Extension] > 0 {
				savedExtensions[target.Extension]--
				continue
			}
			missing = append(missing, "*."+target.Extension)
		}
	}
	return missing
}

func savedManagedFileName(call skillloop.SkillToolCallRef) string {
	if !finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return ""
	}
	return normalizeManagedFileTargetName(firstNonEmptyString(
		call.Result["file_name"],
		call.Result["filename"],
		call.Result["name"],
		call.Arguments["filename"],
		call.Arguments["output_filename"],
	))
}

func fileManagerSaveToolFileID(call skillloop.SkillToolCallRef) string {
	if !finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(
		call.Arguments["tool_file_id"],
		call.Arguments["file_id"],
		call.Result["source_tool_file_id"],
		call.Result["source_file_id"],
		call.Result["tool_file_id"],
	))
}

func fileManagerSaveArgumentsToolFileID(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(
		args["tool_file_id"],
		args["file_id"],
		args["source_tool_file_id"],
		args["source_file_id"],
	))
}

func normalizeManagedFileTargetName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, " \t\r\n\"'`.,，。;；:：!！?？)）]】}》>“”‘’")
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return strings.ToLower(strings.TrimSpace(name))
}

func managedFileTargetExtension(filename string) string {
	filename = normalizeManagedFileTargetName(filename)
	if filename == "" {
		return ""
	}
	idx := strings.LastIndex(filename, ".")
	if idx < 0 || idx == len(filename)-1 {
		return ""
	}
	ext := strings.TrimPrefix(filename[idx+1:], ".")
	if ext == "markdown" {
		return "md"
	}
	return ext
}

func normalizeManagedFileTargetExtension(extension string) string {
	extension = strings.ToLower(strings.TrimSpace(extension))
	extension = strings.TrimPrefix(extension, ".")
	if extension == "markdown" {
		return "md"
	}
	return extension
}

func generatedArtifactSaveArguments(call skillloop.SkillToolCallRef) map[string]interface{} {
	if finalAnswerGuardHasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return nil
	}
	if !toolCallResultLooksLikeGeneratedArtifact(call) {
		return nil
	}
	toolFileID := strings.TrimSpace(firstNonEmptyString(call.Result["tool_file_id"], call.Result["file_id"]))
	if toolFileID == "" {
		return nil
	}
	filename := strings.TrimSpace(firstNonEmptyString(
		call.Result["filename"],
		call.Result["name"],
		call.Arguments["filename"],
		call.Arguments["output_filename"],
	))
	args := map[string]interface{}{
		"source_type":  "tool_file",
		"tool_file_id": toolFileID,
	}
	if filename != "" {
		args["filename"] = filename
	}
	return args
}

func generatedArtifactMapSaveArguments(artifact map[string]interface{}) map[string]interface{} {
	if len(artifact) == 0 {
		return nil
	}
	if strings.TrimSpace(stringFromAny(artifact["upload_file_id"])) != "" ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(artifact["target"])), "managed_file") {
		return nil
	}
	toolFileID := strings.TrimSpace(firstNonEmptyString(artifact["tool_file_id"], artifact["file_id"]))
	if toolFileID == "" {
		return nil
	}
	args := map[string]interface{}{
		"source_type":  "tool_file",
		"tool_file_id": toolFileID,
	}
	if filename := strings.TrimSpace(firstNonEmptyString(artifact["filename"], artifact["name"])); filename != "" {
		args["filename"] = filename
	}
	return args
}

func shouldReuseRecentGeneratedArtifactForManagedCreate(parts *chatRequestParts) bool {
	if parts == nil || len(parts.RecentGeneratedArtifacts) == 0 ||
		!turnTaskContractRequestsManagedFileCreate(parts, nil, "") {
		return false
	}
	return !modelTurnIntentRequestsTemporaryFileArtifact(parts.ModelTurnIntent)
}
