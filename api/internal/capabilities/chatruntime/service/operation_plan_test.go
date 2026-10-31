package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func assertAgentManagementModelDecidesOperationPlanForTest(t *testing.T, plan map[string]interface{}) {
	t.Helper()
	if len(plan) == 0 {
		t.Fatal("operation plan is empty")
	}
	if got := stringFromAny(plan["planning_mode"]); got != "phase_only_model_decides" {
		t.Fatalf("planning_mode = %q, want phase_only_model_decides", got)
	}
	if got := stringFromAny(plan["tool_choice_mode"]); got != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("tool_choice_mode = %q, want %q", got, aiChatTurnToolChoiceModelDecides)
	}
	if len(mapSliceFromAny(plan["steps"])) != 0 {
		t.Fatalf("operation plan contains executable steps: %#v", plan["steps"])
	}
	for _, key := range []string{"step_status", "structured_plan", "required_tool_sequence", "args_binding", "planned_tools", "pending_next_action"} {
		if _, exists := plan[key]; exists {
			t.Fatalf("operation plan contains executable field %q: %#v", key, plan[key])
		}
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "current" {
		t.Fatalf("plan_sync_status = %q, want current", got)
	}
}

func TestInvocationEvidenceKeepsOutcomePlanCurrentWithoutUpdatingLegacyStepsOrPhases(t *testing.T) {
	phases := []interface{}{map[string]interface{}{
		"id": "phase-1", "step": "Update the Agent", "status": "in_progress",
	}}
	legacySteps := []interface{}{map[string]interface{}{
		"id": "tool:agent-management/update_agent_config", "status": "pending",
	}}
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":                           operationPlanStatusRunning,
			"phases":                           phases,
			"steps":                            legacySteps,
			"step_status":                      map[string]interface{}{"tool:agent-management/update_agent_config": "pending"},
			"required_tool_sequence":           []interface{}{"agent-management/get_agent_config"},
			"args_binding":                     map[string]interface{}{"agent_id": "legacy-agent"},
			"plan_sync_status":                 "current",
			"evidence_sequence_at_plan_update": 0,
			"evidence_after_last_plan_update":  0,
		},
	}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{map[string]interface{}{
		"kind":       "tool_call",
		"status":     "success",
		"runtime_id": "runtime:tool#7",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  "update_agent_config",
		"arguments":  map[string]interface{}{"agent_id": "model-selected-agent"},
		"result": map[string]interface{}{
			"status": "completed", "effect": "updated", "agent_id": "model-selected-agent",
			"updated_fields": []interface{}{"system_prompt"},
		},
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("plan status = %q, want running until final answer", got)
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "current" {
		t.Fatalf("plan_sync_status = %q, want current", got)
	}
	if got := intValueFromAny(plan["evidence_after_last_plan_update"]); got != 0 {
		t.Fatalf("evidence_after_last_plan_update = %d, want 0", got)
	}
	if got := stringFromAny(mapSliceFromAny(plan["phases"])[0]["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want unchanged in_progress", got)
	}
	if got := stringFromAny(mapFromOperationContext(plan["step_status"])["tool:agent-management/update_agent_config"]); got != "pending" {
		t.Fatalf("legacy step status = %q, want unchanged pending", got)
	}
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 1 || stringFromAny(ledger[0]["tool_name"]) != "update_agent_config" {
		t.Fatalf("evidence ledger = %#v, want one successful update", ledger)
	}
}

func TestInvocationEvidenceDoesNotCompleteUnstructuredPhaseFromToolCall(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-read", "status": "in_progress"},
				map[string]interface{}{"id": "phase-next", "status": "pending"},
			},
		},
	}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{map[string]interface{}{
		"kind":       "tool_call",
		"status":     "success",
		"runtime_id": "runtime:read#1",
		"skill_id":   skills.SkillFileReader,
		"tool_name":  "read_file",
		"arguments":  map[string]interface{}{"plan_phase_id": "phase-read", "file_id": "file-1"},
		"result":     map[string]interface{}{"status": "completed", "content_status": "extracted", "content": "hello", "content_chars": 5, "file_id": "file-1"},
	}})

	phases := mapSliceFromAny(mapFromOperationContext(metadata["operation_plan"])["phases"])
	completion := mapFromOperationContext(phases[0]["completion_action"])
	if got := stringFromAny(phases[0]["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want in_progress without exact acceptance evidence", got)
	}
	if len(completion) != 0 {
		t.Fatalf("completion_action = %#v, want none for unstructured phase", completion)
	}
}

func TestFailedInvocationIsRecordedWithoutPermanentlyInvalidatingOutcomePlan(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":           operationPlanStatusRunning,
			"plan_sync_status": "current",
			"phases": []interface{}{map[string]interface{}{
				"id": "phase-update", "status": "in_progress",
			}},
		},
	}
	applyOperationPlanInvocationState(metadata, []map[string]interface{}{map[string]interface{}{
		"kind":       "tool_call",
		"status":     "failed",
		"runtime_id": "runtime:update#1",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  "update_agent_config",
		"error":      "temporary failure",
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["plan_sync_status"]); got != "current" {
		t.Fatalf("plan_sync_status = %q, want current after persisting failure evidence", got)
	}
	if _, exists := plan["plan_revision_reason"]; exists {
		t.Fatalf("plan_revision_reason = %#v, want historical failure not to persist a revision requirement", plan["plan_revision_reason"])
	}
	phase := mapSliceFromAny(plan["phases"])[0]
	if got := stringFromAny(phase["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want failure to leave outcome open", got)
	}
}

func TestInvocationEvidenceUsesGlobalRevisionAcrossLocalToolSequences(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":                           operationPlanStatusRunning,
			"plan_sync_status":                 "current",
			"evidence_revision":                1,
			"evidence_revision_at_plan_update": 1,
			"evidence_sequence_at_plan_update": 1,
			operationPlanEvidenceLedgerKey: []interface{}{map[string]interface{}{
				"kind": "tool_call", "status": "completed", "skill_id": skills.SkillFileReader,
				"tool_name": "read_file", "invocation_id": "read-1", "sequence": 1, "ledger_revision": 1,
				"keys": []interface{}{"tool:file-reader/read_file"},
			}},
		},
	}

	applyOperationPlanInvocationState(metadata, []map[string]interface{}{map[string]interface{}{
		"kind": "tool_call", "status": "success", "runtime_id": "agent-list-1", "sequence": 1,
		"skill_id": skills.SkillAgentManagement, "tool_name": "list_agents",
		"result": map[string]interface{}{"status": "completed", "agents": []interface{}{}},
	}})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := intValueFromAny(plan["evidence_revision"]); got != 2 {
		t.Fatalf("evidence_revision = %d, want 2", got)
	}
	if got := intValueFromAny(plan["evidence_after_last_plan_update"]); got != 0 {
		t.Fatalf("evidence_after_last_plan_update = %d, want 0", got)
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "current" {
		t.Fatalf("plan_sync_status = %q, want current", got)
	}
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 2 || intValueFromAny(ledger[1]["ledger_revision"]) != 2 {
		t.Fatalf("evidence ledger revisions = %#v, want global revisions 1,2", ledger)
	}
}

func TestApplyOperationPlanCompletionVerificationMirrorsTopLevelMetadata(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"version":          operationPlanChecklistVersion,
			"status":           "running",
			"plan_sync_status": "stale",
			"phases": []interface{}{map[string]interface{}{
				"id": "phase-1", "step": "Update Agent", "status": "in_progress",
			}},
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_config": "completed",
			},
			operationPlanEvidenceLedgerKey: []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "completed",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
					"keys":      []interface{}{"tool:agent-management/update_agent_config"},
				},
			},
		},
	}

	applyOperationPlanTerminalCompletionResult(metadata, "pass", "tool evidence is complete", nil, nil, "")

	verification := mapFromOperationContext(metadata["completion_verification"])
	if got := stringFromAny(verification["status"]); got != "incomplete" {
		t.Fatalf("completion_verification.status = %q, want incomplete; metadata=%#v", got, metadata)
	}
	if _, exists := metadata["evidence_ledger"]; exists {
		t.Fatalf("top-level evidence_ledger = %#v, want canonical plan ledger only", metadata["evidence_ledger"])
	}
	ledgerRef := mapFromOperationContext(metadata["evidence_ledger_ref"])
	if got := stringFromAny(ledgerRef["source"]); got != "operation_plan.evidence_ledger" {
		t.Fatalf("evidence_ledger_ref = %#v, want canonical plan reference", ledgerRef)
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(mapSliceFromAny(plan["phases"])[0]["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want finalizer to preserve model phase status", got)
	}
	if got := stringFromAny(plan["plan_sync_status"]); got != "stale" {
		t.Fatalf("plan_sync_status = %q, want finalizer to preserve stale", got)
	}
	if got := stringFromAny(plan["status"]); got == operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, must not complete while phase remains in_progress", got)
	}
}

func TestApplyOperationPlanCompletionVerificationPassesWhenAllPhasesTerminal(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-1", "status": operationPlanStepStatusCompleted},
				map[string]interface{}{"id": "phase-2", "status": "skipped"},
			},
		},
	}

	applyOperationPlanTerminalCompletionResult(metadata, "pass", "all phases are terminal", nil, nil, "")
	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("plan status = %q, want completed", got)
	}
	if got := stringFromAny(mapFromOperationContext(plan["completion_verification"])["status"]); got != "pass" {
		t.Fatalf("verification status = %q, want pass", got)
	}
}

func TestMainModelFinalDoesNotCompleteRuntimeVerifiedPhase(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"version": operationPlanChecklistVersion,
			"status":  operationPlanStatusRunning,
			"phases": []interface{}{map[string]interface{}{
				"id": "phase-1", "status": operationPlanStepStatusPending,
				"verification_mode": "runtime_effects",
			}},
		},
	}

	applyOperationPlanTerminalCompletionResultWithSource(
		metadata, "pass", "main_model_final", "no active runtime protocol blocker", nil, nil, "",
	)

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("plan status = %q, want running", got)
	}
	if got := stringFromAny(mapSliceFromAny(plan["phases"])[0]["status"]); got != operationPlanStepStatusPending {
		t.Fatalf("phase status = %q, want pending", got)
	}
}

func TestOperationPlanEvidenceLedgerStoresAgentFactsAndPostReadVerification(t *testing.T) {
	plan := map[string]interface{}{
		"status": operationPlanStatusRunning,
	}
	update := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"arguments": map[string]interface{}{
			"agent_id":            "agent-1",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "Write fiction and generate files when needed.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
			"expected_updated_fields": []interface{}{
				"model",
				"system_prompt",
				"file_upload_enabled",
				"enabled_skill_ids",
			},
		},
		"result": map[string]interface{}{
			"status":              "completed",
			"effect":              "updated",
			"agent_id":            "agent-1",
			"agent_name":          "Story Agent",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
			"updated_fields":      []interface{}{"model", "system_prompt", "file_upload_enabled", "enabled_skill_ids"},
		},
	}
	read := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "get_agent_config",
		"result": map[string]interface{}{
			"status":              "completed",
			"agent_id":            "agent-1",
			"agent_name":          "Story Agent",
			"model_provider":      "deepseek",
			"model":               "deepseek-v4-flash",
			"system_prompt":       "Write fiction and generate files when needed.",
			"file_upload_enabled": true,
			"enabled_skill_ids":   []interface{}{"file-generator"},
		},
	}

	operationPlanAppendEvidenceLedgerEntry(plan, update, []string{"tool:agent-management/update_agent_config"})
	operationPlanAppendEvidenceLedgerEntry(plan, read, []string{"tool:agent-management/get_agent_config", "agent:config"})

	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 2 {
		t.Fatalf("evidence_ledger len = %d, want 2; ledger=%#v", len(ledger), ledger)
	}
	updateFacts := mapFromOperationContext(ledger[0]["result_facts"])
	if _, leaked := updateFacts["system_prompt"]; leaked {
		t.Fatalf("update result_facts leaked system_prompt: %#v", updateFacts)
	}
	if got := stringFromAny(updateFacts["agent.config.system_prompt_digest"]); !strings.HasPrefix(got, operationPlanEvidenceDigestPrefix) {
		t.Fatalf("system_prompt digest = %q, want sha256 digest; facts=%#v", got, updateFacts)
	}
	readFacts := mapFromOperationContext(ledger[1]["result_facts"])
	fieldStatus := mapFromOperationContext(readFacts["field_status"])
	if got := stringFromAny(fieldStatus["system_prompt"]); got != "verified" {
		t.Fatalf("field_status.system_prompt = %q, want verified; read facts=%#v", got, readFacts)
	}
	if got := stringFromAny(fieldStatus["model"]); got != "verified" {
		t.Fatalf("field_status.model = %q, want verified; read facts=%#v", got, readFacts)
	}
	if got := stringFromAny(readFacts["agent.config.model"]); got != "deepseek-v4-flash" {
		t.Fatalf("agent.config.model = %q, want deepseek-v4-flash; read facts=%#v", got, readFacts)
	}
	compact := operationPlanCompactEvidenceLedger(plan[operationPlanEvidenceLedgerKey], 10)
	if summary := mapFromOperationContext(compact[1]["result_summary"]); stringFromAny(summary["model"]) != "deepseek-v4-flash" {
		t.Fatalf("compact result_summary = %#v, want model evidence", summary)
	}
}

func TestOperationPlanEvidenceUsesAuthoritativeSystemPromptDigestInsteadOfCompactPreview(t *testing.T) {
	fullPrompt := strings.Repeat("existing chapter ", 180) + "第十章 persisted suffix"
	compactPreview := string([]rune(fullPrompt)[:2000]) + "..."
	invocation := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "update_agent_config",
		"result": map[string]interface{}{
			"status":                "completed",
			"updated_fields":        []interface{}{"system_prompt"},
			"system_prompt":         compactPreview,
			"system_prompt_present": true,
			"system_prompt_chars":   len([]rune(fullPrompt)),
			"system_prompt_digest":  operationPlanEvidenceTextDigest(fullPrompt),
		},
	}

	facts := operationPlanEvidenceLedgerResultFacts(invocation)

	if got := stringFromAny(facts["system_prompt_digest"]); got != operationPlanEvidenceTextDigest(fullPrompt) {
		t.Fatalf("system_prompt_digest = %q, want full persisted digest %q", got, operationPlanEvidenceTextDigest(fullPrompt))
	}
	if got := facts["system_prompt_chars"]; got != len([]rune(fullPrompt)) {
		t.Fatalf("system_prompt_chars = %#v, want %d", got, len([]rune(fullPrompt)))
	}
	if got := stringFromAny(facts["system_prompt_digest"]); got == operationPlanEvidenceTextDigest(compactPreview) {
		t.Fatalf("system_prompt_digest = %q, unexpectedly derived from compact preview", got)
	}
	if _, leaked := facts["system_prompt"]; leaked {
		t.Fatalf("result_facts leaked system_prompt: %#v", facts)
	}
}

func TestOperationPlanEvidenceLedgerStoresGeneratedFileFactsAndDedupesManagedSave(t *testing.T) {
	plan := map[string]interface{}{
		"status": operationPlanStatusRunning,
	}
	generate := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
		"arguments": map[string]interface{}{
			"filename":       "story.pdf",
			"format":         "pdf",
			"lifecycle":      "temporary",
			"content_length": 1520,
		},
		"asset_operation_audit": map[string]interface{}{
			"assets": []interface{}{
				map[string]interface{}{
					"name": "story.pdf",
					"type": "file",
					"metadata": map[string]interface{}{
						"format":    "pdf",
						"lifecycle": "temporary",
					},
				},
			},
		},
	}
	save := map[string]interface{}{
		"kind":          "tool_call",
		"status":        "success",
		"skill_id":      skills.SkillFileManager,
		"tool_name":     "save_file_to_management",
		"runtime_id":    "runtime-save-1",
		"invocation_id": "runtime-save-1",
		"result": map[string]interface{}{
			"status":         "completed",
			"target":         "managed_file",
			"file_id":        "managed-1",
			"upload_file_id": "managed-1",
			"filename":       "story.md",
			"source_type":    "tool_file",
		},
	}
	duplicateSave := copyStringAnyMap(save)
	duplicateSave["runtime_id"] = "trace-save-1"
	duplicateSave["invocation_id"] = "trace-save-1"

	operationPlanAppendEvidenceLedgerEntry(plan, generate, []string{"tool:file-generator/generate_file"})
	operationPlanAppendEvidenceLedgerEntry(plan, save, []string{"tool:file-manager/save_file_to_management"})
	operationPlanAppendEvidenceLedgerEntry(plan, duplicateSave, []string{"tool:file-manager/save_file_to_management"})

	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if len(ledger) != 2 {
		t.Fatalf("evidence_ledger len = %d, want generated file plus one managed save; ledger=%#v", len(ledger), ledger)
	}
	generateFacts := mapFromOperationContext(ledger[0]["result_facts"])
	if got := stringFromAny(generateFacts["filename"]); got != "story.pdf" {
		t.Fatalf("generated filename = %q, want story.pdf; facts=%#v", got, generateFacts)
	}
	if got := stringFromAny(generateFacts["file_extension"]); got != "pdf" {
		t.Fatalf("generated file_extension = %q, want pdf; facts=%#v", got, generateFacts)
	}
	if got := stringFromAny(generateFacts["target"]); got != "temporary_artifact" {
		t.Fatalf("generated target = %q, want temporary_artifact; facts=%#v", got, generateFacts)
	}
	if got := stringFromAny(generateFacts["mime_type"]); got != "application/pdf" {
		t.Fatalf("generated mime_type = %q, want application/pdf; facts=%#v", got, generateFacts)
	}
	if got := intValueFromAny(generateFacts["content_length"]); got != 1520 {
		t.Fatalf("generated content_length = %d, want 1520; facts=%#v", got, generateFacts)
	}
	saveFacts := mapFromOperationContext(ledger[1]["result_facts"])
	if got := stringFromAny(saveFacts["file_extension"]); got != "md" {
		t.Fatalf("managed save file_extension = %q, want md; facts=%#v", got, saveFacts)
	}
}

func TestTurnStrategyAllowedSkillIDsKeepsPlannedToolsWithRouteHint(t *testing.T) {
	strategy := &AIChatTurnStrategy{
		Intent:        "manage_agent_asset",
		RouteRequired: true,
		PlannedTools: []AIChatTurnStrategyTool{
			{SkillID: skills.SkillAgentManagement, ToolName: "update_agent_config"},
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
		},
	}

	allowed := turnStrategyAllowedSkillIDs(strategy)
	for _, want := range []string{skills.SkillAgentManagement} {
		if _, ok := allowed[want]; !ok {
			t.Fatalf("allowed skills = %#v, missing %s", allowed, want)
		}
	}
}

func TestAgentCapabilityDefinitionsForPromptExposeMVPContracts(t *testing.T) {
	definitions := agentManagementCapabilityDefinitionsForPrompt()
	if len(definitions) == 0 {
		t.Fatal("agentManagementCapabilityDefinitionsForPrompt() = empty, want prompt-facing capability model")
	}
	byID := map[string]map[string]interface{}{}
	for _, definition := range definitions {
		id := stringFromAny(definition["capability_id"])
		if id != "" {
			byID[id] = definition
		}
	}

	model := byID[agentCapabilityModelSelection]
	for _, field := range []string{"model_provider", "model"} {
		if !stringSliceContainsFold(stringSliceFromAny(model["required_config_fields"]), field) {
			t.Fatalf("model capability definition = %#v, missing required field %s", model, field)
		}
	}
	if stringFromAny(model["candidate_tool"]) != "list_available_models" {
		t.Fatalf("model capability definition = %#v, want list_available_models candidate tool", model)
	}
	for _, want := range []string{"model_provider", "model"} {
		if !strings.Contains(fmt.Sprint(model["verify_by"]), want) {
			t.Fatalf("model capability verify_by = %#v, want %s evidence", model["verify_by"], want)
		}
	}

	systemPrompt := byID[agentCapabilitySystemPrompt]
	if !stringSliceContainsFold(stringSliceFromAny(systemPrompt["required_config_fields"]), "system_prompt") {
		t.Fatalf("system-prompt capability definition = %#v, want system_prompt field", systemPrompt)
	}
	if !strings.Contains(fmt.Sprint(systemPrompt["not_sufficient"]), "tool_binding_or_resource_access_claim") {
		t.Fatalf("system-prompt not_sufficient = %#v, want resource-access boundary", systemPrompt["not_sufficient"])
	}

	fileUpload := byID[agentCapabilityAcceptUploaded]
	if !stringSliceContainsFold(stringSliceFromAny(fileUpload["required_config_fields"]), "file_upload_enabled") {
		t.Fatalf("file-upload capability definition = %#v, want file_upload_enabled config field", fileUpload)
	}
	if stringSliceContainsFold(stringSliceFromAny(fileUpload["required_config_fields"]), "enabled_skill_ids") {
		t.Fatalf("file-upload capability definition = %#v, should not require skill binding", fileUpload)
	}

	memory := byID[agentCapabilityMemory]
	if !stringSliceContainsFold(stringSliceFromAny(memory["required_config_fields"]), "agent_memory_enabled") {
		t.Fatalf("memory capability definition = %#v, want agent_memory_enabled config field", memory)
	}
	if !strings.Contains(fmt.Sprint(memory["not_sufficient"]), "system_prompt_only") {
		t.Fatalf("memory not_sufficient = %#v, want prompt-only boundary", memory["not_sufficient"])
	}

	suggestedQuestions := byID[agentCapabilitySuggestedQuestion]
	if !stringSliceContainsFold(stringSliceFromAny(suggestedQuestions["required_config_fields"]), "suggested_questions") {
		t.Fatalf("suggested-questions capability definition = %#v, want suggested_questions config field", suggestedQuestions)
	}
	if !strings.Contains(fmt.Sprint(suggestedQuestions["verify_by"]), "suggested_questions") {
		t.Fatalf("suggested-questions verify_by = %#v, want suggested_questions evidence", suggestedQuestions["verify_by"])
	}

	skillBacked := byID[agentCapabilitySkillBacked]
	if !stringSliceContainsFold(stringSliceFromAny(skillBacked["required_config_fields"]), "enabled_skill_ids") {
		t.Fatalf("skill-backed capability definition = %#v, want enabled_skill_ids config field", skillBacked)
	}
	if got := stringFromAny(skillBacked["candidate_tool"]); got != "list_agent_skill_candidates" {
		t.Fatalf("skill-backed candidate_tool = %q, want list_agent_skill_candidates; definition=%#v", got, skillBacked)
	}
	if !strings.Contains(fmt.Sprint(skillBacked["examples"]), "file generation") {
		t.Fatalf("skill-backed examples = %#v, want file generation example", skillBacked["examples"])
	}

	for _, want := range []struct {
		capabilityID string
		field        string
	}{
		{agentCapabilityKnowledgeBinding, "knowledge_dataset_ids"},
		{agentCapabilityDatabaseBinding, "database_bindings"},
		{agentCapabilityWorkflowBinding, "workflow_bindings"},
	} {
		definition := byID[want.capabilityID]
		if len(stringSliceFromAny(definition["candidate_tools"])) == 0 {
			t.Fatalf("%s definition = %#v, want candidate_tools for binding resolution", want.capabilityID, definition)
		}
	}
	for _, capabilityID := range []string{
		agentCapabilityModelSelection,
		agentCapabilitySystemPrompt,
		agentCapabilityAcceptUploaded,
		agentCapabilityMemory,
		agentCapabilitySuggestedQuestion,
		agentCapabilitySkillBacked,
		agentCapabilityKnowledgeBinding,
		agentCapabilityDatabaseBinding,
		agentCapabilityWorkflowBinding,
	} {
		definition := byID[capabilityID]
		if len(definition) == 0 {
			t.Fatalf("missing MVP capability definition %s; definitions=%#v", capabilityID, definitions)
		}
		for _, key := range []string{"meaning", "enable_by", "not_sufficient", "verify_by"} {
			if value, ok := definition[key]; !ok || fmt.Sprint(value) == "" || fmt.Sprint(value) == "[]" {
				t.Fatalf("%s definition missing %s semantic contract: %#v", capabilityID, key, definition)
			}
		}
	}
}

func TestAgentConfigFieldDescriptorsDriveCanonicalFields(t *testing.T) {
	for _, descriptor := range agentManagementConfigFieldDescriptors() {
		t.Run(descriptor.field, func(t *testing.T) {
			if descriptor.field == "" {
				t.Fatalf("config field descriptor has empty field: %#v", descriptor)
			}
			if got := operationPlanAgentConfigCanonicalField(descriptor.field); got != descriptor.field {
				t.Fatalf("operationPlanAgentConfigCanonicalField(%q) = %q, want %q", descriptor.field, got, descriptor.field)
			}
			for _, alias := range descriptor.aliases {
				if got := operationPlanAgentConfigCanonicalField(alias); got != descriptor.field {
					t.Fatalf("operationPlanAgentConfigCanonicalField(%q) = %q, want %q", alias, got, descriptor.field)
				}
			}
		})
	}
}

func TestSkillLoopRuntimeStateSnapshotEmbedsOperationLedgerInExecutionLedger(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "Use the selected page resource.",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
		OperationLedger: map[string]interface{}{
			"version": operationLedgerVersion,
			"status":  operationLedgerStatusObserved,
			"resources": []map[string]interface{}{{
				"id":   "file-1",
				"type": "file",
				"name": "visible.md",
			}},
			"capabilities": []map[string]interface{}{{
				"id":   "file.read",
				"name": "read_file",
			}},
			"risk_summary": map[string]interface{}{
				"level": "low",
			},
		},
	}
	metadata := streamingMessageMetadataWithTaskID(parts, "task-operation-ledger")
	prepared := &PreparedChat{
		parts: parts,
		Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: metadata,
		},
	}

	evidence := skillLoopRuntimeStateSnapshot(prepared)()
	if ledger := mapFromOperationContext(evidence["operation_ledger"]); len(ledger) == 0 {
		t.Fatalf("operation_ledger evidence = %#v, want map", evidence["operation_ledger"])
	}
	executionLedger := mapFromOperationContext(evidence["execution_ledger"])
	operationLedger := mapFromOperationContext(executionLedger["operation_ledger"])
	if len(operationLedger) == 0 {
		t.Fatalf("execution_ledger.operation_ledger = %#v, want operation ledger", executionLedger["operation_ledger"])
	}
	if operationLedger["version"] != operationLedgerVersion {
		t.Fatalf("operation ledger version = %#v, want %q", operationLedger["version"], operationLedgerVersion)
	}
	resources := mapSliceFromAny(operationLedger["resources"])
	if len(resources) != 1 || resources[0]["name"] != "visible.md" {
		t.Fatalf("operation ledger resources = %#v, want selected visible.md resource", operationLedger["resources"])
	}
	capabilities := mapSliceFromAny(operationLedger["capabilities"])
	if len(capabilities) != 1 || capabilities[0]["name"] != "read_file" {
		t.Fatalf("operation ledger capabilities = %#v, want read_file capability", operationLedger["capabilities"])
	}
}

func TestSkillLoopCompletionPageContextEvidenceHonorsNoNavigationRequest(t *testing.T) {
	context := map[string]interface{}{
		"resources": []interface{}{
			map[string]interface{}{
				"resource_type": "page",
				"href":          "/console/agents/agent-1/agent",
			},
			map[string]interface{}{
				"resource_type": "agent",
				"id":            "agent-1",
				"title":         "Visible Agent One",
				"href":          "/console/agents/agent-1/agent",
				"selected":      true,
			},
		},
	}
	parts := &chatRequestParts{
		Query:               "\u8bf7\u4fee\u6539\u5f53\u524d\u667a\u80fd\u4f53\u7684\u63cf\u8ff0\uff0c\u4e0d\u8981\u5207\u6362\u5230\u5176\u4ed6\u9875\u9762",
		Surface:             aiChatSurfaceContextualSidebar,
		RuntimeContext:      "route=/console/agents/agent-1/agent",
		SkillMode:           skillModeAuto,
		SkillIDs:            []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
		RawOperationContext: context,
		OperationContext:    context,
	}

	evidence := skillLoopCompletionPageContextEvidence(parts)
	if got := stringFromAny(evidence["current_page"]); got != "/console/agents/agent-1/agent" {
		t.Fatalf("current_page = %q, want current Agent detail route; evidence=%#v", got, evidence)
	}
	if target := mapFromOperationContext(evidence["resolved_target_from_user_request"]); len(target) > 0 {
		t.Fatalf("resolved_target_from_user_request = %#v, want omitted for explicit no-navigation request", target)
	}
}

func TestContextualConsoleAgentsSkillMessageFiltersUnavailableResolvedTools(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "open the Agent page",
			Surface:        aiChatSurfaceContextualSidebar,
			SkillMode:      skillModeAuto,
			SkillIDs:       []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
			RuntimeContext: "route=/console/agents",
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "page",
						"href":          "/console/agents",
					},
					map[string]interface{}{
						"resource_type": "agent",
						"id":            "agent-1",
						"title":         "Customer Support Agent",
						"href":          "/console/agents/agent-1/agent",
					},
				},
			},
		},
	}
	prepared.parts.OperationContext = prepared.parts.RawOperationContext
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{
			Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
			Tools: []skills.SkillToolDefinition{
				{Name: "get_agent_config"},
				{Name: "update_agent_config"},
				{Name: "list_available_models"},
			},
		},
	}}

	message, ok := contextualConsoleAgentsSkillMessageForResolved(prepared, resolved)
	if !ok {
		t.Fatal("contextualConsoleAgentsSkillMessageForResolved() = false, want message")
	}
	content := stringFromAny(message.Content)
	for _, unexpected := range []string{`"tool_name":"list_agents"`, `"tool_name":"get_agent"`, `"tool_name":"navigate"`} {
		if strings.Contains(content, unexpected) {
			t.Fatalf("message content exposes unavailable tool %s: %s", unexpected, content)
		}
	}
	for _, want := range []string{
		`"tool_name":"get_agent_config"`,
		`"tool_name":"update_agent_config"`,
		"tools array in Agent management JSON is the authoritative callable tool list",
		"Because list_agents is absent from the tools array",
		"Because get_agent is absent from the tools array",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("message content missing %q: %s", want, content)
		}
	}
}

func TestSkillLoopToolEffectClassificationUsesGovernanceManifest(t *testing.T) {
	if !skillLoopToolLooksReadOnly(skills.SkillInternalKnowledge, "retrieve_knowledge") {
		t.Fatal("retrieve_knowledge was not classified as read-only from governance manifest")
	}
	if skillLoopToolLooksAssetMutation(skills.SkillInternalKnowledge, "retrieve_knowledge") {
		t.Fatal("retrieve_knowledge was classified as mutation, want read-only")
	}
	if !skillLoopToolLooksAssetMutation(skills.SkillAgentWorkflow, "run_agent_workflow") {
		t.Fatal("run_agent_workflow was not classified as governed mutation/invoke")
	}
}

func TestSkillLoopCompletionOperationResultSummaryPrefersLatestToolCallOverPlanClientAction(t *testing.T) {
	summary := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              operationPlanStatusRunning,
			"pending_next_action": "continue_from_phase_success_criteria",
			"tool_result": map[string]interface{}{
				"kind":      "client_action",
				"status":    "succeeded",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "create_agent",
				"result_summary": map[string]interface{}{
					"status": "succeeded",
					"effect": "create",
				},
			},
		},
		"tool_results": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"result_summary": map[string]interface{}{
					"status":         "completed",
					"effect":         "updated",
					"agent_id":       "agent-1",
					"agent_name":     "Smoke Agent",
					"model_provider": "deepseek",
					"model":          "deepseek-chat",
					"updated_fields": []interface{}{"model_provider", "model", "enabled_skill_ids"},
				},
			},
		},
		"client_actions": []interface{}{
			map[string]interface{}{
				"kind":      "client_action",
				"status":    "succeeded",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "create_agent",
				"result_summary": map[string]interface{}{
					"effect": "create",
				},
			},
		},
	}

	operationSummary := skillLoopCompletionOperationResultSummary(summary)
	latest := mapFromOperationContext(operationSummary["latest_tool_result"])
	if got := stringFromAny(latest["tool_name"]); got != "update_agent_config" {
		t.Fatalf("latest_tool_result.tool_name = %q, want update_agent_config; summary=%#v", got, operationSummary)
	}
	if got := stringFromAny(operationSummary["tool_name"]); got != "update_agent_config" {
		t.Fatalf("operation_result_summary.tool_name = %q, want update_agent_config; summary=%#v", got, operationSummary)
	}
	if got := stringFromAny(operationSummary["agent_id"]); got != "agent-1" {
		t.Fatalf("operation_result_summary.agent_id = %q, want agent-1; summary=%#v", got, operationSummary)
	}
	if got := stringFromAny(operationSummary["status"]); got != operationPlanStatusRunning {
		t.Fatalf("operation_result_summary.status = %q, want running while plan is still running; summary=%#v", got, operationSummary)
	}
}

func assertAgentManagementModelDecidesExecutionForTest(t *testing.T, strategy *AIChatTurnStrategy) {
	t.Helper()
	if strategy == nil {
		t.Fatal("strategy = nil, want model-decides Agent-management execution")
	}
	if got := strings.TrimSpace(strategy.ToolChoiceMode); got != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("ToolChoiceMode = %q, want %q; strategy=%#v", got, aiChatTurnToolChoiceModelDecides, strategy)
	}
	if len(strategy.PlannedTools) != 0 {
		t.Fatalf("planned tools = %#v, want model-decides without hard scripted tools", strategy.PlannedTools)
	}
}

func TestOperationPlanRejectedAgentDeleteIsTerminalForContinuation(t *testing.T) {
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agents")
	plan := map[string]interface{}{
		"version":             operationPlanVersion,
		"status":              "rejected",
		"pending_next_action": "none",
		"intent":              "manage_agent_asset",
		"original_user_goal":  "delete the first two visible agents",
		"steps": []interface{}{map[string]interface{}{
			"id":        deleteStepID,
			"title":     "Delete visible agents",
			"status":    "rejected",
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "delete_agents",
		}},
		"step_status": map[string]interface{}{
			deleteStepID: "rejected",
		},
	}

	if operationPlanHasIncompleteWork(plan) {
		t.Fatalf("operationPlanHasIncompleteWork(%#v) = true, want rejected plan to be terminal", plan)
	}
}

func stringSliceContains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func mustUUIDForTest(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if id == uuid.Nil {
		t.Fatal("uuid.New returned nil")
	}
	return id
}

func TestCompactUnsavedOperationPlanGeneratedFilesUsesSaveInvocationEvidence(t *testing.T) {
	files := []map[string]interface{}{{
		"target":       "temporary_artifact",
		"file_id":      "tool-1",
		"tool_file_id": "tool-1",
		"filename":     "report.pdf",
		"extension":    ".pdf",
	}}
	saveCalls := []skillloop.SkillToolCallRef{{
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-1",
			"filename":     "report.pdf",
		},
		Result: map[string]interface{}{
			"status":              "completed",
			"source_tool_file_id": "tool-1",
			"target":              "managed_file",
		},
	}}

	if unsaved := compactUnsavedOperationPlanGeneratedFiles(files, saveCalls); len(unsaved) != 0 {
		t.Fatalf("unsaved files = %#v, want save invocation evidence to satisfy the temporary artifact", unsaved)
	}
}
