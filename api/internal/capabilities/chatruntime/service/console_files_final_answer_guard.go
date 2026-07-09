package service

import (
	"encoding/json"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func consoleFilesFileManagementCreateFinalAnswerGuard(parts *chatRequestParts, metadata map[string]interface{}) skillloop.FinalAnswerGuard {
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		metadataSaveCalls := managedFileSaveCallsFromGeneratedFilesMetadata(metadata)
		successfulSaveCalls := append([]skillloop.SkillToolCallRef{}, metadataSaveCalls...)
		successfulSaveCalls = append(successfulSaveCalls, req.SuccessfulToolCalls...)
		missingTargets := managedFileCreateMissingSaveTargets(parts, metadata, req.SuccessfulToolCalls)
		hasSuccessfulSave := finalAnswerGuardHasSuccessfulFileManagerSaveTool(req) || len(metadataSaveCalls) > 0
		if len(missingTargets) == 0 && hasSuccessfulSave {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if len(missingTargets) > 0 && hasSuccessfulSave {
			message := strings.Join([]string{
				"The user's current files-page request asks to create or save multiple files into File Management.",
				"The following requested file targets have not been saved yet: " + strings.Join(missingTargets, ", ") + ".",
				"Do not say all requested files were created or saved.",
				"Continue the missing file generation/save flow and only report success after file-manager/save_file_to_management succeeds for each target.",
			}, " ")
			systemMessage := message
			if saveArgs := latestUnsavedGeneratedArtifactSaveArgumentsForTargetsFromMetadata(metadata, missingTargets); len(saveArgs) > 0 {
				if encoded, err := json.Marshal(map[string]interface{}{
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
					"arguments": saveArgs,
				}); err == nil {
					systemMessage = strings.Join([]string{
						message,
						"An unsaved generated artifact for a missing requested target already exists.",
						"Do not regenerate it; call file-manager/save_file_to_management with the resolved arguments JSON below.",
						"Resolved generated-file save JSON for tool arguments only; do not reveal internal IDs to the user: " + string(encoded),
					}, " ")
				}
			}
			return skillloop.FinalAnswerGuardResult{
				SkillID:       skills.SkillFileManager,
				ToolName:      "save_file_to_management",
				Message:       message,
				SystemMessage: systemMessage,
			}, true
		}
		if finalAnswerGuardHasAttemptedFileManagerSaveTool(req) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		messageLines := []string{
			"The user's current files-page request explicitly asks to create or save a new file into File Management or the current Files page.",
			"Do not finish by saying this is unsupported.",
			"Load the appropriate artifact-producing skill and file-manager if needed. For each destination file, create one temporary artifact, then call file-manager/save_file_to_management with source_type \"tool_file\", the generated tool_file_id/file_id, and that destination filename.",
			"Use file-generator for normal files and generic SVG/vector files. Use chart-generator only when the user explicitly asks for a chart, graph, data visualization, or a supported chart type.",
			"Keep generated files temporary only when the user did not explicitly ask for File Management, current Files page, save, create, upload, or import as the target.",
			"Only after file-manager/save_file_to_management succeeds may you say the File Management file was created. If approval is required, wait for tool governance instead of asking for a separate natural-language confirmation.",
		}
		systemLines := append([]string{}, messageLines...)
		saveArgs := latestGeneratedArtifactSaveArguments(req.SuccessfulToolCalls)
		saveSourceMessage := "A temporary artifact has already been generated in this turn. Do not generate another file for the same request."
		if len(saveArgs) == 0 {
			saveArgs = latestRecentGeneratedArtifactSaveArguments(parts)
			if len(saveArgs) > 0 {
				saveSourceMessage = "The user is referring to a recent generated/downloadable file from the conversation. Do not generate another file or substitute a visible File Management asset for it."
			}
		}
		if len(saveArgs) > 0 {
			systemLines = append(systemLines,
				saveSourceMessage,
				"Load file-manager if needed, then call call_skill_tool with skill_id \"file-manager\", tool_name \"save_file_to_management\", and the resolved arguments JSON below.",
			)
			if encoded, err := json.Marshal(map[string]interface{}{
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
				"arguments": saveArgs,
			}); err == nil {
				systemLines = append(systemLines, "Resolved generated-file save JSON for tool arguments only; do not reveal internal IDs to the user: "+string(encoded))
			}
		}
		message := strings.Join(messageLines, " ")
		return skillloop.FinalAnswerGuardResult{
			SkillID:       skills.SkillFileManager,
			ToolName:      "save_file_to_management",
			Message:       message,
			SystemMessage: strings.Join(systemLines, " "),
		}, true
	}
}

func fileManagerSaveRequiredToolGuardResult(saveArgs map[string]interface{}) skillloop.FinalAnswerGuardResult {
	messageLines := []string{
		"A temporary artifact has already been generated for this File Management creation request.",
		"Do not generate another file for the same request.",
		"Call file-manager/save_file_to_management with the generated temporary artifact.",
	}
	systemLines := append([]string{}, messageLines...)
	if len(saveArgs) > 0 {
		if encoded, err := json.Marshal(map[string]interface{}{
			"skill_id":  skills.SkillFileManager,
			"tool_name": "save_file_to_management",
			"arguments": saveArgs,
		}); err == nil {
			systemLines = append(systemLines, "Resolved generated-file save JSON for tool arguments only; do not reveal internal IDs to the user: "+string(encoded))
		}
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillFileManager,
		ToolName:      "save_file_to_management",
		Message:       strings.Join(messageLines, " "),
		SystemMessage: strings.Join(systemLines, " "),
	}
}

func continuationGeneratedFilesAlreadySatisfiedGuardResult() skillloop.FinalAnswerGuardResult {
	message := strings.Join([]string{
		"The continuation already has generated file artifacts recorded and no unsaved generated artifact remains.",
		"Do not generate or save another file for the same continuation step.",
		"Continue with the next planned non-generation action, such as refreshing visible files or deleting the frozen target, or provide the final answer if all steps are complete.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillFileGenerator,
		ToolName:      "generate_file",
		Message:       message,
		SystemMessage: message,
	}
}

func consoleFilesContinuationPendingDeleteFinalAnswerGuard(parts *chatRequestParts, metadata map[string]interface{}) skillloop.FinalAnswerGuard {
	if parts == nil || !partsRequestsContinuationWithFallback(parts, "") || !skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) {
		return nil
	}
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if managedFileCreateContinuationSaveFlowActive(parts, metadata) {
			pendingSaveArgs := pendingGeneratedArtifactSaveArgumentCandidates(parts, metadata, req.SuccessfulToolCalls)
			if len(pendingSaveArgs) > 0 {
				result := fileManagerSaveRequiredToolGuardResult(pendingSaveArgs[0])
				prefix := "A generated temporary artifact is still not saved to File Management, so deletion cannot run yet."
				result.Message = prefix + " " + result.Message
				result.SystemMessage = prefix + " Save every requested generated artifact before delete_file or any destructive follow-up step. " + result.SystemMessage
				return result, true
			}
		}
		successfulDeleteCalls := append(successfulMetadataToolCalls(metadata, skills.SkillFileManager, "delete_file"), matchingSkillToolCalls(req.SuccessfulToolCalls, skills.SkillFileManager, "delete_file")...)
		if len(successfulDeleteCalls) > 0 {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return skillloop.FinalAnswerGuardResult{}, false
	}
}

func consoleFilesDeleteAlreadySucceededGuardResult(successfulCalls []skillloop.SkillToolCallRef) skillloop.FinalAnswerGuardResult {
	deletedName := latestSuccessfulDeleteFileName(successfulCalls)
	message := "A file-manager/delete_file call has already succeeded for the frozen deletion target. Do not re-resolve the current third file after deletion and do not ask for another deletion confirmation."
	if deletedName != "" {
		message = "The frozen deletion target " + deletedName + " was already deleted successfully. Do not re-resolve the current third file after deletion and do not ask for another deletion confirmation."
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillFileManager,
		ToolName: "delete_file",
		Message:  message,
		SystemMessage: strings.Join([]string{
			message,
			"Provide the final user-visible answer now.",
			"Summarize the completed txt/svg save and the single completed deletion. Do not call another destructive tool unless the user makes a new explicit request.",
		}, " "),
	}
}

func latestSuccessfulDeleteFileName(calls []skillloop.SkillToolCallRef) string {
	for idx := len(calls) - 1; idx >= 0; idx-- {
		call := calls[idx]
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillFileManager) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "delete_file") {
			continue
		}
		if name := firstNonEmptyString(call.Result["file_name"], call.Result["filename"], call.Result["name"], call.Arguments["filename"], call.Arguments["name"]); name != "" {
			return name
		}
		return strings.TrimSpace(stringFromAny(call.Arguments["file_id"]))
	}
	return ""
}

func finalAnswerGuardHasSuccessfulToolForTargets(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, targetFileIDs []string) bool {
	return finalAnswerGuardHasToolForTargets(req.SuccessfulToolCalls, skillID, toolName, targetFileIDs)
}

func finalAnswerGuardHasAttemptedToolForTargets(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, targetFileIDs []string) bool {
	return finalAnswerGuardHasToolForTargets(req.AttemptedToolCalls, skillID, toolName, targetFileIDs)
}

func finalAnswerGuardHasToolForTargets(calls []skillloop.SkillToolCallRef, skillID string, toolName string, targetFileIDs []string) bool {
	if len(targetFileIDs) == 0 {
		for _, call := range calls {
			if strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) &&
				strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
				return true
			}
		}
		return false
	}
	required := map[string]struct{}{}
	for _, id := range targetFileIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			required[id] = struct{}{}
		}
	}
	if len(required) == 0 {
		return false
	}
	matched := map[string]struct{}{}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
			continue
		}
		actual := skillToolCallFileIDs(call.Arguments)
		for _, got := range actual {
			if _, ok := required[got]; ok {
				matched[got] = struct{}{}
			}
		}
	}
	return len(matched) == len(required)
}

func finalAnswerGuardHasSuccessfulFileManagerSaveTool(req skillloop.FinalAnswerGuardRequest) bool {
	return finalAnswerGuardHasFileManagerSaveCall(req.SuccessfulToolCalls)
}

func finalAnswerGuardHasAttemptedFileManagerSaveTool(req skillloop.FinalAnswerGuardRequest) bool {
	return finalAnswerGuardHasFileManagerSaveCall(req.AttemptedToolCalls)
}

func finalAnswerGuardHasFileManagerSaveCall(calls []skillloop.SkillToolCallRef) bool {
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillFileManager) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(call.ToolName), "save_file_to_management") {
			return true
		}
	}
	return false
}
