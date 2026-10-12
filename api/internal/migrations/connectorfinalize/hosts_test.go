package connectorfinalize

import (
	"testing"
)

func TestNativePendingWithFormerSkillIDRemainsResumable(t *testing.T) {
	pending := row{"route_receipt": "native-receipt", "skill_id": "external-apps", "tool_name": "send_message", "state": "pending"}
	invocation := row{"kind": "tool_call", "runtime_source": "connector", "skill_id": "external-apps", "tool_name": "send_message", "status": "pending_confirmation", "result": row{"state": "requires_confirmation", "confirmation": pending}}
	metadata := row{"pending_connector_confirmation": pending, "skill_invocations": []any{invocation}, "agent_transcript_version": 1, "agent_transcript": []any{row{"role": "tool", "content": "preserved native result"}}}
	after, err := convertMessageMetadata(metadata, snapshot{}, "message")
	if err != nil {
		t.Fatal(err)
	}
	out := object(after)
	converted := object(out["skill_invocations"].([]any)[0])
	if converted["runtime_source"] != "connector" || converted["skill_id"] != "connector-runtime" || digest(converted["result"]) != digest(invocation["result"]) || digest(out["pending_connector_confirmation"]) != digest(pending) || out["connector_history_version"] != nil || out["connector_action_instances"] != nil || out["agent_transcript_version"] == nil {
		t.Fatal("native continuation was archived or blocked")
	}
}

func TestRetiredCandidatesSealTranscriptReplayWithoutChangingArchivalText(t *testing.T) {
	transcript := []any{row{"role": "assistant", "tool_calls": []any{row{"function": row{"name": "call_skill_tool", "arguments": `{"skill_id":"external-apps","tool_name":"execute_action"}`}}}}}
	metadata := row{"_native_external_action_candidates": []any{row{"action_id": "old"}}, "_native_external_action_projections": []any{row{"action_id": "old"}}, "_native_external_action_intent_matched": true, "_native_external_action_intent_keys": []any{"old"}, "agent_transcript_version": 1, "agent_transcript": transcript, "model_invocations": []any{row{"log_context": "archival request"}}}
	after, err := convertMessageMetadata(metadata, snapshot{}, "message")
	if err != nil {
		t.Fatal(err)
	}
	out := object(after)
	if out["_native_external_action_candidates"] != nil || out["agent_transcript_version"] != nil || object(out["connector_action_instances"])["blocked"] != true || digest(out["agent_transcript"]) != digest(transcript) || digest(out["model_invocations"]) != digest(metadata["model_invocations"]) {
		t.Fatal("retired replay was not sealed, or archival text changed")
	}
	for _, key := range []string{"_native_external_action_projections", "_native_external_action_intent_matched", "_native_external_action_intent_keys"} {
		if out[key] != nil {
			t.Fatal("retired candidate metadata remains")
		}
	}
}

func TestHistoryConversionCannotReplayOrTrustProviderSuccess(t *testing.T) {
	metadata := row{
		"selected_skill_ids":       []any{"external-apps", "ordinary"},
		"skill_invocations":        []any{row{"kind": "tool_call", "skill_id": "external-apps", "tool_name": "execute_action", "runtime_id": "old-1", "arguments": row{"token": "private-secret"}, "result": row{"status": "success", "receipt": "private-receipt"}}},
		"native_skill_diagnostics": row{"tool_bindings": row{"search": row{"skill_id": "external-apps"}}},
		"content":                  "Human text mentions external-apps and execute_action.",
	}
	after, err := convertMessageMetadata(metadata, snapshot{}, "message")
	if err != nil {
		t.Fatal(err)
	}
	out := object(after)
	if out["content"] != metadata["content"] {
		t.Fatal("human content changed")
	}
	if out["selected_skill_ids"].([]any)[0] != "connector-runtime" {
		t.Fatal("machine Skill reference not converted")
	}
	invocation := object(out["skill_invocations"].([]any)[0])
	presentation := object(object(invocation["result"])["connector_presentation"])
	if presentation["display_status"] != "unknown" || invocation["runtime_source"] != "connector_history" || invocation["tool_name"] != "connector_history" {
		t.Fatal("history asserted unproven success or remained executable")
	}
	if invocation["arguments"] != nil || object(invocation["result"])["receipt"] != nil {
		t.Fatal("history leaked execution input")
	}
	if object(out["connector_action_instances"])["blocked"] != true {
		t.Fatal("old turn can mint a new operation")
	}
	if object(metadata["skill_invocations"].([]any)[0])["skill_id"] != "external-apps" {
		t.Fatal("source archive mutated")
	}
}

func TestHostConversionPreservesNewLedgerAndOrdinaryTool(t *testing.T) {
	ledger := row{"version": 1, "instances": row{"server-key": row{"id": "server-id"}}}
	ordinary := row{"skill_id": "filesystem", "tool_name": "read", "arguments": row{"path": "external-apps"}, "result": row{"text": "execute_action"}}
	metadata := row{"connector_action_instances": ledger, "skill_invocations": []any{ordinary, row{"skill_id": "external-apps", "tool_name": "search_web", "runtime_id": "old"}}}
	after, err := convertMessageMetadata(metadata, snapshot{}, "message")
	if err != nil {
		t.Fatal(err)
	}
	out := object(after)
	if digest(out["connector_action_instances"]) != digest(ledger) || digest(out["skill_invocations"].([]any)[0]) != digest(ordinary) {
		t.Fatal("ordinary execution or native ledger changed")
	}
}

func TestDiscardHostScrubKeepsUserTextAndRemovesMachineReferences(t *testing.T) {
	value := row{
		"connector_bindings": []any{row{"id": "old"}},
		"nested":             row{"runtime_type": "integration", "description": "keep this text"},
		"content":            "connector_bindings and execute_action remain user text",
		"ordinary":           row{"tool_name": "read"},
	}
	converted, changed, err := scrubMachineJSON(value)
	if err != nil || !changed {
		t.Fatalf("scrubMachineJSON() changed=%v err=%v", changed, err)
	}
	out := object(converted)
	if out["connector_bindings"] != nil || object(out["nested"])["runtime_type"] != nil {
		t.Fatal("retired machine references remain")
	}
	if out["content"] != value["content"] || object(out["nested"])["description"] != "keep this text" {
		t.Fatal("user text changed")
	}
	if object(out["ordinary"])["tool_name"] != "read" {
		t.Fatal("ordinary tool reference changed")
	}
}

func TestNativeBindingConstraintRejectsWeakenedOrExpandedEnum(t *testing.T) {
	valid := "CHECK (((binding_type)::text = ANY ((ARRAY['skill'::character varying, 'knowledge_dataset'::character varying, 'database'::character varying, 'database_table'::character varying, 'workflow'::character varying])::text[])))"
	if !nativeBindingTypeConstraint(valid) {
		t.Fatal("native enum rejected")
	}
	for _, definition := range []string{"CHECK (true)", valid + " OR true", "CHECK (binding_type <> 'integration_connection')"} {
		if nativeBindingTypeConstraint(definition) {
			t.Fatal("weakened enum accepted")
		}
	}
}
