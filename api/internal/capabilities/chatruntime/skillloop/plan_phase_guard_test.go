package skillloop

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProjectedPreparationUsesFirstNonEmptyServerResultPathForEquivalentFeishuIdentifiers(t *testing.T) {
	values := projectedExternalPreparationObservedValuesForTarget(
		map[string]interface{}{"users": []interface{}{map[string]interface{}{
			"open_id": "ou_Alice", "user_id": "u_Alice",
		}}},
		"recipient_id",
		[]string{"recipient_id"},
		[]string{"users[].open_id", "users[].user_id"},
	)
	if len(values) != 1 || values[0] != "ou_Alice" {
		t.Fatalf("observed values = %#v, want the first non-empty server path only", values)
	}
	if !projectedExternalPreparationContextMatches(
		map[string]interface{}{"target_argument_paths": []interface{}{"recipient_id", "recipient_type"}},
		map[string]interface{}{"target_arguments": map[string]interface{}{"recipient_type": "open_id"}},
		map[string]interface{}{"arguments": map[string]interface{}{}},
		"recipient_id", "users[].open_id",
	) {
		t.Fatal("matching Feishu recipient_type was not derived from the selected server result path")
	}
	if projectedExternalPreparationContextMatches(
		map[string]interface{}{"target_argument_paths": []interface{}{"recipient_id", "recipient_type"}},
		map[string]interface{}{"target_arguments": map[string]interface{}{"recipient_type": "user_id"}},
		map[string]interface{}{"arguments": map[string]interface{}{}},
		"recipient_id", "users[].open_id",
	) {
		t.Fatal("mismatched Feishu recipient_type was accepted for an open_id result")
	}
}

func TestProjectedPreparationSelectsFeishuIdentifierByServerBoundRecipientType(t *testing.T) {
	connectionBinding := skills.NativeExternalActionConnectionBindingHash("connection-feishu")
	phase := map[string]interface{}{
		"id": "phase-send", "status": "in_progress", operationPlanServerProjectedLedgerEpochKey: "ledger-feishu",
		"expected_action": map[string]interface{}{
			"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
			planExpectedActionServerBindingFingerprintKey: "binding-send",
			"target": map[string]interface{}{"integration_id": "feishu", "action_id": "feishu.message.send_user"},
		},
	}
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{
			map[string]interface{}{
				"integration_id": "feishu", "action_id": "feishu.contact.search", "effect": "read",
				"binding_fingerprint": "binding-search", operationPlanServerProjectedConnectionBindingKey: connectionBinding,
			},
			map[string]interface{}{
				"integration_id": "feishu", "action_id": "feishu.message.send_user", "effect": "external_send",
				"binding_fingerprint": "binding-send", operationPlanServerProjectedConnectionBindingKey: connectionBinding,
				"target_argument_paths": []interface{}{"recipient_id", "recipient_type"},
				"preparation_hints": []interface{}{map[string]interface{}{
					"action_key": "feishu:feishu.contact.search", "relation": "resolve_target",
					"target_arguments": []interface{}{"recipient_id"},
					"result_paths":     []interface{}{"users[].open_id", "users[].user_id"},
				}},
			},
		},
		"operation_plan": map[string]interface{}{"phases": []interface{}{phase}},
	}
	call := map[string]interface{}{
		"integration_id": "feishu", "action_id": "feishu.contact.search", "connection_id": "connection-feishu",
		"arguments": map[string]interface{}{"query": "Alice", "page_size": 10},
	}
	payload := map[string]interface{}{
		"integration_id": "feishu", "action_id": "feishu.contact.search", "operation_status": "completed", "result_count": 1,
		"result": map[string]interface{}{"users": []interface{}{map[string]interface{}{
			"open_id": "ou_Alice", "user_id": "u_Alice",
		}}},
	}

	for _, testCase := range []struct {
		recipientType string
		want          string
	}{
		{recipientType: "open_id", want: "ou_Alice"},
		{recipientType: "user_id", want: "u_Alice"},
	} {
		t.Run(testCase.recipientType, func(t *testing.T) {
			expected := evidenceMapFromAny(phase["expected_action"])
			expected["target_arguments"] = map[string]interface{}{"recipient_type": testCase.recipientType}
			observed := projectedExternalActionObservedPreparationTargets(
				state, call, "phase-send", "ledger-feishu", "binding-send", payload,
			)
			if got := evidenceStringSliceFromAny(observed["recipient_id"]); !reflect.DeepEqual(got, []string{testCase.want}) {
				t.Fatalf("observed recipient = %#v, want %q", observed, testCase.want)
			}
		})
	}

	delete(evidenceMapFromAny(phase["expected_action"]), "target_arguments")
	if observed := projectedExternalActionObservedPreparationTargets(
		state, call, "phase-send", "ledger-feishu", "binding-send", payload,
	); len(observed) != 0 {
		t.Fatalf("recipient without a server-bound recipient_type was trusted: %#v", observed)
	}
}

func TestProjectedPreparationExactScalarLookupDoesNotRequirePaginationMetadata(t *testing.T) {
	connectionBinding := skills.NativeExternalActionConnectionBindingHash("connection-x")
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{
			map[string]interface{}{
				"integration_id": "x", "action_id": "x.user.get_by_username", "effect": "read",
				"binding_fingerprint": "binding-user-get", operationPlanServerProjectedConnectionBindingKey: connectionBinding,
			},
			map[string]interface{}{
				"integration_id": "x", "action_id": "x.post.list_by_user", "effect": "read",
				"binding_fingerprint": "binding-post-list", operationPlanServerProjectedConnectionBindingKey: connectionBinding,
				"target_argument_paths": []interface{}{"user_id"},
				"preparation_hints": []interface{}{map[string]interface{}{
					"action_key": "x:x.user.get_by_username", "relation": "resolve_target",
					"target_arguments": []interface{}{"user_id"}, "result_paths": []interface{}{"user.id"},
				}},
			},
		},
		"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
			"id": "phase-posts", "status": "in_progress", operationPlanServerProjectedLedgerEpochKey: "ledger-x",
			"expected_action": map[string]interface{}{
				"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
				planExpectedActionServerBindingFingerprintKey: "binding-post-list",
				"target": map[string]interface{}{"integration_id": "x", "action_id": "x.post.list_by_user"},
			},
		}}},
	}
	call := map[string]interface{}{
		"integration_id": "x", "action_id": "x.user.get_by_username", "connection_id": "connection-x",
		"arguments": map[string]interface{}{"username": "alice"},
	}
	payload := map[string]interface{}{
		"integration_id": "x", "action_id": "x.user.get_by_username", "operation_status": "completed", "result_count": 1,
		"result": map[string]interface{}{"user": map[string]interface{}{"id": "2244994945"}},
	}
	observed := projectedExternalActionObservedPreparationTargets(
		state, call, "phase-posts", "ledger-x", "binding-post-list", payload,
	)
	if got := evidenceStringSliceFromAny(observed["user_id"]); !reflect.DeepEqual(got, []string{"2244994945"}) {
		t.Fatalf("exact scalar lookup was not trusted: %#v", observed)
	}

	payload["result"] = map[string]interface{}{
		"user": map[string]interface{}{"id": "2244994945"}, "incomplete": true,
	}
	if observed := projectedExternalActionObservedPreparationTargets(
		state, call, "phase-posts", "ledger-x", "binding-post-list", payload,
	); len(observed) != 0 {
		t.Fatalf("explicitly incomplete scalar lookup was trusted: %#v", observed)
	}
}

func TestProjectedPreparationCollectionStillRequiresGlobalUniquenessProof(t *testing.T) {
	payload := map[string]interface{}{
		"operation_status": "completed", "result_count": 1,
		"result": map[string]interface{}{"chats": []interface{}{map[string]interface{}{"chat_id": "oc_one"}}},
	}
	for _, resultPath := range []string{"chats[].chat_id", "calendars[].calendar_id"} {
		if projectedExternalPreparationResultIsComplete(
			map[string]interface{}{"arguments": map[string]interface{}{}}, payload, 1, 0, resultPath,
		) {
			t.Fatalf("one row at %q on an unbounded response was treated as globally unique", resultPath)
		}
	}
	for _, resultPath := range []string{"user.id", "message.id"} {
		if !projectedExternalPreparationResultIsComplete(
			map[string]interface{}{"arguments": map[string]interface{}{}}, payload, 1, 0, resultPath,
		) {
			t.Fatalf("exact scalar result path %q was treated as a paginated collection", resultPath)
		}
	}
}

func TestNativePreparationHintEvidencePreservesServerResultTransform(t *testing.T) {
	evidence := nativeExternalActionPreparationHintEvidence("github", []skills.NativeExternalActionPreparationHint{{
		ActionID: "github.repository.search", Relation: "resolve_target",
		TargetArguments: []string{"owner", "repo"}, ResultPaths: []string{"repositories[].full_name"},
		ResultTransform: "split_slash_pair",
	}})
	if len(evidence) != 1 || evidenceStringFromAny(evidenceMapFromAny(evidence[0])["result_transform"]) != "split_slash_pair" {
		t.Fatalf("runtime preparation evidence lost transform: %#v", evidence)
	}
}

func TestProjectedPreparationSplitsOneConfirmedGitHubRepositoryServerSide(t *testing.T) {
	connectionBinding := skills.NativeExternalActionConnectionBindingHash("connection-github")
	phaseExpected := map[string]interface{}{
		"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
		planExpectedActionServerBindingFingerprintKey: "binding-create-issue",
		"target": map[string]interface{}{"integration_id": "github", "action_id": "github.issue.create"},
	}
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{
			map[string]interface{}{
				"integration_id": "github", "action_id": "github.repository.search", "effect": "read",
				"binding_fingerprint": "binding-repository-search", operationPlanServerProjectedConnectionBindingKey: connectionBinding,
			},
			map[string]interface{}{
				"integration_id": "github", "action_id": "github.issue.create", "effect": "create",
				"binding_fingerprint": "binding-create-issue", operationPlanServerProjectedConnectionBindingKey: connectionBinding,
				"target_argument_paths": []interface{}{"owner", "repo"},
				"preparation_hints": []interface{}{map[string]interface{}{
					"action_key": "github:github.repository.search", "relation": "resolve_target",
					"target_arguments": []interface{}{"owner", "repo"},
					"result_paths":     []interface{}{"repositories[].full_name"}, "result_transform": "split_slash_pair",
				}},
			},
		},
		"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
			"id": "phase-create", "status": "in_progress", operationPlanServerProjectedLedgerEpochKey: "ledger-github",
			"expected_action": phaseExpected,
		}}},
	}
	call := map[string]interface{}{
		"integration_id": "github", "action_id": "github.repository.search", "connection_id": "connection-github",
		"arguments": map[string]interface{}{"query": "zgi", "per_page": 10},
	}
	payloadFor := func(fullNames []interface{}, total int) map[string]interface{} {
		repositories := make([]interface{}, 0, len(fullNames))
		for _, fullName := range fullNames {
			repositories = append(repositories, map[string]interface{}{"full_name": fullName})
		}
		return map[string]interface{}{
			"integration_id": "github", "action_id": "github.repository.search", "operation_status": "completed",
			"result_count": len(repositories),
			"result": map[string]interface{}{
				"repositories": repositories, "total_count": total, "incomplete_results": false,
			},
		}
	}

	observed := projectedExternalActionObservedPreparationTargets(
		state, call, "phase-create", "ledger-github", "binding-create-issue", payloadFor([]interface{}{"zgiai/zgi"}, 1),
	)
	if !reflect.DeepEqual(evidenceStringSliceFromAny(observed["owner"]), []string{"zgiai"}) ||
		!reflect.DeepEqual(evidenceStringSliceFromAny(observed["repo"]), []string{"zgi"}) {
		t.Fatalf("repository coordinates = %#v, want server-split zgiai/zgi", observed)
	}

	for _, testCase := range []struct {
		name      string
		fullNames []interface{}
		total     int
	}{
		{name: "two repositories", fullNames: []interface{}{"zgiai/zgi", "zgiai/zgi-web"}, total: 2},
		{name: "hidden second repository", fullNames: []interface{}{"zgiai/zgi"}, total: 2},
		{name: "extra slash", fullNames: []interface{}{"zgiai/zgi/extra"}, total: 1},
		{name: "unsafe component", fullNames: []interface{}{"zgiai/../zgi"}, total: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if observed := projectedExternalActionObservedPreparationTargets(
				state, call, "phase-create", "ledger-github", "binding-create-issue", payloadFor(testCase.fullNames, testCase.total),
			); len(observed) != 0 {
				t.Fatalf("invalid repository result was trusted: %#v", observed)
			}
		})
	}

	phaseExpected["target_arguments"] = map[string]interface{}{"owner": "other"}
	if observed := projectedExternalActionObservedPreparationTargets(
		state, call, "phase-create", "ledger-github", "binding-create-issue", payloadFor([]interface{}{"zgiai/zgi"}, 1),
	); len(observed) != 0 {
		t.Fatalf("repository result crossed the existing target context: %#v", observed)
	}
}

func TestProjectedPreparationObservationRejectsCrossResourceContext(t *testing.T) {
	connectionBinding := skills.NativeExternalActionConnectionBindingHash("connection-github")
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{
			map[string]interface{}{
				"integration_id": "github", "action_id": "github.issue.list", "effect": "read",
				"binding_fingerprint": "binding-list", operationPlanServerProjectedConnectionBindingKey: connectionBinding,
			},
			map[string]interface{}{
				"integration_id": "github", "action_id": "github.issue.update", "effect": "write",
				"binding_fingerprint": "binding-update", operationPlanServerProjectedConnectionBindingKey: connectionBinding,
				"target_argument_paths": []interface{}{"owner", "repo", "issue_number"},
				"preparation_hints": []interface{}{map[string]interface{}{
					"action_key": "github:github.issue.list", "relation": "resolve_target",
					"target_arguments": []interface{}{"issue_number"}, "result_paths": []interface{}{"issues[].number"},
				}},
			},
		},
		"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
			"id": "phase-issue", "status": "in_progress", operationPlanServerProjectedLedgerEpochKey: "ledger-issue",
			"expected_action": map[string]interface{}{
				"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
				planExpectedActionServerBindingFingerprintKey: "binding-update",
				"target":           map[string]interface{}{"integration_id": "github", "action_id": "github.issue.update"},
				"target_arguments": map[string]interface{}{"owner": "octo", "repo": "repo-a"},
			},
		}}},
	}
	payload := map[string]interface{}{
		"integration_id": "github", "action_id": "github.issue.list", "operation_status": "completed", "result_count": 1,
		"result": map[string]interface{}{"issues": []interface{}{map[string]interface{}{"number": "42"}}},
	}
	crossRepoCall := map[string]interface{}{
		"integration_id": "github", "action_id": "github.issue.list", "connection_id": "connection-github",
		"arguments": map[string]interface{}{"owner": "octo", "repo": "repo-b", "per_page": 10},
	}
	if observed := projectedExternalActionObservedPreparationTargets(
		state, crossRepoCall, "phase-issue", "ledger-issue", "binding-update", payload,
	); len(observed) != 0 {
		t.Fatalf("cross-repository issue number was trusted: %#v", observed)
	}
	sameRepoCall := copyStringAnyMap(crossRepoCall)
	sameRepoCall["arguments"] = map[string]interface{}{"owner": "octo", "repo": "repo-a", "per_page": 10}
	if observed := projectedExternalActionObservedPreparationTargets(
		state, sameRepoCall, "phase-issue", "ledger-issue", "binding-update", payload,
	); !reflect.DeepEqual(evidenceStringSliceFromAny(observed["issue_number"]), []string{"42"}) {
		t.Fatalf("same-repository issue number was not trusted: %#v", observed)
	}
}

func TestProjectedPreparationObservationRequiresConfirmedProviderSuccessAndExactConnection(t *testing.T) {
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	searchProjection := projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search")
	searchProjection.Binding.IntentMatched = false
	searchProjection.Binding.IntentGroup = ""
	searchProjection.Binding.IntentTokens = nil
	sendProjection := projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send")
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{searchProjection, sendProjection}, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	sendFingerprint := sendProjection.Binding.BindingFingerprint
	snapshot := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
		"id": "phase-send", "status": "in_progress", operationPlanServerProjectedLedgerEpochKey: "ledger-1",
		"expected_action": map[string]interface{}{
			"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
			planExpectedActionServerBindingFingerprintKey: sendFingerprint,
			"target": map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
		},
	}}}}
	state := runtimeStateForRun(RunRequest{NativeToolSet: &toolSet, RuntimeStateSnapshot: func() map[string]interface{} { return snapshot }})
	callArguments := map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.contact.search", "connection_id": "connection-wecom",
		"arguments": map[string]interface{}{"query": "Alice"},
	}
	providerResult := map[string]interface{}{"members": []interface{}{map[string]interface{}{"recipient_ref": "wm-alice"}}}
	for _, status := range []string{"failed", "partial_failed", "outcome_unknown", "pending", "executing"} {
		if observed := projectedExternalActionObservedPreparationTargets(
			state, callArguments, "phase-send", "ledger-1", sendFingerprint,
			map[string]interface{}{
				"integration_id": "wecom", "action_id": "wecom.contact.search",
				"operation_status": status, "result_count": 1, "result": providerResult,
			},
		); len(observed) != 0 {
			t.Fatalf("status %q produced trusted target evidence: %#v", status, observed)
		}
	}
	if observed := projectedExternalActionObservedPreparationTargets(
		state, callArguments, "phase-send", "ledger-1", sendFingerprint,
		map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.contact.search",
			"operation_status": "completed", "result_count": 1, "result": providerResult,
		},
	); len(evidenceStringSliceFromAny(observed["recipient_ref"])) != 1 {
		t.Fatalf("confirmed matching-connection result was not observed: %#v", observed)
	}
	wrongConnection := copyStringAnyMap(callArguments)
	wrongConnection["connection_id"] = "connection-wecom-other"
	if observed := projectedExternalActionObservedPreparationTargets(
		state, wrongConnection, "phase-send", "ledger-1", sendFingerprint,
		map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.contact.search",
			"operation_status": "completed", "result_count": 1, "result": providerResult,
		},
	); len(observed) != 0 {
		t.Fatalf("different-connection preparation result was trusted: %#v", observed)
	}
	if observed := projectedExternalActionObservedPreparationTargets(
		state, callArguments, "phase-send", "ledger-1", sendFingerprint,
		map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.contact.search",
			"operation_status": "completed", "result_count": 1,
			"result": map[string]interface{}{
				projectedExternalObservedPreparationTargetsKey: map[string]interface{}{"recipient_ref": []interface{}{"wm-forged"}},
			},
		},
	); len(observed) != 0 {
		t.Fatalf("provider-injected private evidence key was trusted: %#v", observed)
	}
}

func TestProjectedPreparationObservationRejectsIncompleteOrPaginatedSingleRows(t *testing.T) {
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	searchProjection := projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search")
	searchProjection.Binding.IntentMatched = false
	searchProjection.Binding.IntentGroup = ""
	searchProjection.Binding.IntentTokens = nil
	sendProjection := projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send")
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{searchProjection, sendProjection}, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	sendFingerprint := sendProjection.Binding.BindingFingerprint
	snapshot := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
		"id": "phase-send", "status": "in_progress", operationPlanServerProjectedLedgerEpochKey: "ledger-1",
		"expected_action": map[string]interface{}{
			"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
			planExpectedActionServerBindingFingerprintKey: sendFingerprint,
			"target": map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
		},
	}}}}
	state := runtimeStateForRun(RunRequest{NativeToolSet: &toolSet, RuntimeStateSnapshot: func() map[string]interface{} { return snapshot }})
	baseCall := map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.contact.search", "connection_id": "connection-wecom",
		"arguments": map[string]interface{}{"query": "Alice", "max_results": 10},
	}
	baseResult := map[string]interface{}{"members": []interface{}{map[string]interface{}{"recipient_ref": "wm-alice"}}}
	basePayload := map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.contact.search", "operation_status": "completed", "result_count": 1,
		"result": baseResult,
	}

	tests := []struct {
		name          string
		arguments     map[string]interface{}
		resultOverlay map[string]interface{}
		payload       map[string]interface{}
		wantObserved  bool
	}{
		{name: "complete unique result", wantObserved: true},
		{name: "wecom full max results page", arguments: map[string]interface{}{"max_results": 1}},
		{name: "feishu has more", arguments: map[string]interface{}{"page_size": 1}, resultOverlay: map[string]interface{}{"has_more": true}},
		{name: "gmail next page token", resultOverlay: map[string]interface{}{"next_page_token": "page-2"}},
		{name: "gmail estimate exceeds returned", resultOverlay: map[string]interface{}{"result_size_estimate": 42}},
		{name: "nested total exceeds returned", resultOverlay: map[string]interface{}{"metadata": map[string]interface{}{"total_count": 2}}},
		{name: "content truncated", resultOverlay: map[string]interface{}{"content_truncated": true}},
		{name: "business row cannot prove no-more", arguments: map[string]interface{}{"max_results": 1}, resultOverlay: map[string]interface{}{
			"members": []interface{}{map[string]interface{}{"recipient_ref": "wm-alice", "has_more": false}},
		}},
		{name: "non first cursor", arguments: map[string]interface{}{"cursor": "cursor-2"}},
		{name: "non first page", arguments: map[string]interface{}{"page": 2}},
		{name: "missing facade result count", payload: map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.contact.search", "operation_status": "completed", "result": baseResult,
		}},
		{name: "explicit provider no-more proves full single page", arguments: map[string]interface{}{"max_results": 1}, resultOverlay: map[string]interface{}{"has_more": false}, wantObserved: true},
		{name: "incomplete false is neutral on a full page", arguments: map[string]interface{}{"max_results": 1}, resultOverlay: map[string]interface{}{"incomplete_results": false}},
		{name: "malformed incomplete flag fails closed", resultOverlay: map[string]interface{}{"incomplete_results": "false"}},
		{name: "full page contradictory zero total", arguments: map[string]interface{}{"max_results": 1}, resultOverlay: map[string]interface{}{"incomplete_results": false, "total_count": 0}},
		{name: "full page exact total proves uniqueness", arguments: map[string]interface{}{"max_results": 1}, resultOverlay: map[string]interface{}{"incomplete_results": false, "total_count": 1}, wantObserved: true},
		{name: "full page larger total is incomplete", arguments: map[string]interface{}{"max_results": 1}, resultOverlay: map[string]interface{}{"incomplete_results": false, "total_count": 2}},
		{name: "conflicting authoritative totals fail closed", resultOverlay: map[string]interface{}{"total_count": 1, "metadata": map[string]interface{}{"total": 2}}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			call := copyStringAnyMap(baseCall)
			business := map[string]interface{}{"query": "Alice", "max_results": 10}
			for key, value := range testCase.arguments {
				business[key] = value
			}
			call["arguments"] = business
			payload := copyStringAnyMap(basePayload)
			result := copyStringAnyMap(baseResult)
			for key, value := range testCase.resultOverlay {
				result[key] = value
			}
			payload["result"] = result
			if testCase.payload != nil {
				payload = testCase.payload
			}
			observed := projectedExternalActionObservedPreparationTargets(
				state, call, "phase-send", "ledger-1", sendFingerprint, payload,
			)
			if got := len(evidenceStringSliceFromAny(observed["recipient_ref"])); (got == 1) != testCase.wantObserved {
				t.Fatalf("observed=%#v, wantObserved=%v", observed, testCase.wantObserved)
			}
		})
	}
}

func TestHandleProgressiveSkillCallTreatsMismatchedPlanPhaseAsAdvisory(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	call := planPhaseGuardToolCall(t, "phase-navigation")
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-guard", "msg-phase-guard", "", "auto", &adapter.ChatRequest{}),
		resolved,
		call,
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"phases": []interface{}{map[string]interface{}{
					"id": "phase-navigation", "status": "in_progress",
					"expected_action": map[string]interface{}{"skill_id": "console-navigator", "tool_name": "navigate"},
				}},
			},
		},
		1,
		nil,
	)

	if result.recoverable {
		t.Fatalf("result = %#v, want advisory phase association to execute", result)
	}
	if echoTool.calls != 1 {
		t.Fatalf("runtime calls = %d, want one", echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "phase-navigation" {
		t.Fatalf("plan_phase_id = %q, want preserved advisory association", got)
	}
}

func TestHandleProgressiveSkillCallInfersUniqueStructuredPlanPhase(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	call := planPhaseGuardToolCall(t, "")
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-infer", "msg-phase-infer", "", "auto", &adapter.ChatRequest{}),
		resolved,
		call,
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"phases": []interface{}{map[string]interface{}{
					"id": "phase-business", "status": "pending",
					"expected_action": map[string]interface{}{"skill_id": "phase-guard-skill", "tool_name": "echo_value"},
				}},
			},
		},
		1,
		nil,
	)

	if result.recoverable || echoTool.calls != 1 {
		t.Fatalf("result = %#v calls=%d, want one successful runtime call", result, echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "phase-business" {
		t.Fatalf("plan_phase_id = %q, want inferred phase-business", got)
	}
}

func TestHandleProgressiveSkillCallBindsExplicitCurrentUnstructuredPhase(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-bind", "msg-phase-bind", "", "auto", &adapter.ChatRequest{}),
		resolved,
		planPhaseGuardToolCall(t, "phase-business"),
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-business", "status": "in_progress"},
			map[string]interface{}{"id": "phase-next", "status": "pending"},
		}}},
		1,
		nil,
	)

	if result.recoverable || echoTool.calls != 1 {
		t.Fatalf("result = %#v calls=%d, want explicit current phase to execute", result, echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "phase-business" {
		t.Fatalf("plan_phase_id = %q, want phase-business", got)
	}
}

func TestHandleProgressiveSkillCallAllowsExplicitFutureOutcomePhase(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-future", "msg-phase-future", "", "auto", &adapter.ChatRequest{}),
		resolved,
		planPhaseGuardToolCall(t, "phase-next"),
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-current", "status": "in_progress"},
			map[string]interface{}{"id": "phase-next", "status": "pending"},
		}}},
		1,
		nil,
	)

	if result.recoverable {
		t.Fatalf("result = %#v, want future outcome phase association to execute", result)
	}
	if echoTool.calls != 1 {
		t.Fatalf("runtime calls = %d, want one", echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "phase-next" {
		t.Fatalf("plan_phase_id = %q, want phase-next", got)
	}
}

func TestHandleProgressiveSkillCallDropsUnknownPhaseAssociationWithoutBlockingTool(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-stale", "msg-phase-stale", "", "auto", &adapter.ChatRequest{}),
		resolved,
		planPhaseGuardToolCall(t, "phase-from-old-plan"),
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-current", "status": "in_progress"},
		}}},
		1,
		nil,
	)

	if result.recoverable || echoTool.calls != 1 {
		t.Fatalf("result = %#v calls=%d, want stale phase hint dropped and tool executed", result, echoTool.calls)
	}
	if got := evidenceStringFromAny(result.trace.Arguments["plan_phase_id"]); got != "" {
		t.Fatalf("plan_phase_id = %q, want stale association omitted", got)
	}
}

func TestHandleProgressiveSkillCallKeepsMixedPlanUnstructuredPhaseUsable(t *testing.T) {
	runner, resolved, echoTool := newPlanPhaseGuardTestRunner(t)
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-phase-mixed", "msg-phase-mixed", "", "auto", &adapter.ChatRequest{}),
		resolved,
		planPhaseGuardToolCall(t, ""),
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{"phase-guard-skill": {}},
		map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-current", "status": "in_progress"},
			map[string]interface{}{
				"id": "phase-future", "status": "pending",
				"expected_action": map[string]interface{}{"skill_id": "another-skill", "tool_name": "another_tool"},
			},
		}}},
		1,
		nil,
	)

	if result.recoverable || echoTool.calls != 1 {
		t.Fatalf("result = %#v calls=%d, want mixed plan call to execute", result, echoTool.calls)
	}
}

func TestResolveOperationPlanPhaseForProjectedActionReadsBusinessArgumentsEnvelope(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		projectedExternalActionPlanTestPhase("phase-alice", "pending", "recipient_ref", "alice"),
		projectedExternalActionPlanTestPhase("phase-bob", "pending", "recipient_ref", "bob"),
	})

	phaseID, enforce, err := resolveOperationPlanPhaseForSkillCall(
		state,
		"",
		skills.SkillExternalApps,
		"execute_action",
		map[string]interface{}{
			"integration_id": "wecom",
			"action_id":      "wecom.message.send",
			"arguments":      map[string]interface{}{"recipient_ref": "bob", "content": "hello"},
		},
	)
	if err != nil || !enforce || phaseID != "phase-bob" {
		t.Fatalf("resolveOperationPlanPhaseForSkillCall() = %q, %v, %v; want phase-bob, true, nil", phaseID, enforce, err)
	}
}

func TestProjectedExternalActionPlanIssueRequiresCompleteCanonicalLedger(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		map[string]interface{}{"id": "phase-a", "status": "in_progress", "required": true},
		map[string]interface{}{"id": "phase-b", "status": "pending", "required": true},
	})
	arguments := map[string]interface{}{
		"integration_id": "wecom",
		"action_id":      "wecom.message.send",
		"arguments":      map[string]interface{}{"recipient_ref": "alice", "content": "one"},
	}
	if issue := projectedExternalActionPlanIssue(state, "phase-a", skills.SkillExternalApps, "execute_action", arguments); issue == "" {
		t.Fatal("projectedExternalActionPlanIssue() = empty, want incomplete-ledger rejection")
	}

	state["operation_plan"] = map[string]interface{}{"phases": []interface{}{
		projectedExternalActionPlanTestPhase("phase-a", "in_progress", "recipient_ref", "alice"),
		projectedExternalActionPlanTestPhase("phase-b", "pending", "recipient_ref", "bob"),
	}}
	if issue := projectedExternalActionPlanIssue(state, "phase-a", skills.SkillExternalApps, "execute_action", arguments); issue != "" {
		t.Fatalf("projectedExternalActionPlanIssue() = %q, want complete canonical ledger accepted", issue)
	}
}

func TestProjectedExternalActionPlanIssueUsesPhaseIDForSameActionAndTarget(t *testing.T) {
	state := projectedExternalActionPlanTestState([]interface{}{
		projectedExternalActionPlanTestPhase("phase-first", "in_progress", "recipient_ref", "alice"),
		projectedExternalActionPlanTestPhase("phase-second", "pending", "recipient_ref", "alice"),
	})
	arguments := map[string]interface{}{
		"integration_id": "wecom",
		"action_id":      "wecom.message.send",
		"arguments":      map[string]interface{}{"recipient_ref": "alice", "content": "first"},
	}
	if issue := projectedExternalActionPlanIssue(state, "", skills.SkillExternalApps, "execute_action", arguments); issue == "" {
		t.Fatal("projectedExternalActionPlanIssue() = empty without phase ID, want ambiguity rejection")
	}
	if issue := projectedExternalActionPlanIssue(state, "phase-first", skills.SkillExternalApps, "execute_action", arguments); issue != "" {
		t.Fatalf("projectedExternalActionPlanIssue() = %q with exact phase ID, want accepted", issue)
	}
}

func TestProjectedExternalActionPlanIssueAllowsExplicitOptionalProjectedAction(t *testing.T) {
	phase := projectedExternalActionPlanTestPhase("phase-optional", "pending", "recipient_ref", "alice")
	phase["outcome_id"] = "outcome-optional"
	delete(phase, "required")
	state := projectedExternalActionPlanTestState([]interface{}{phase})
	state["operation_plan"].(map[string]interface{})["outcomes"] = []interface{}{map[string]interface{}{
		"id": "outcome-optional", "required": false, "verification_mode": "runtime_effects",
	}}
	arguments := map[string]interface{}{
		"integration_id": "wecom",
		"action_id":      "wecom.message.send",
		"arguments":      map[string]interface{}{"recipient_ref": "alice", "content": "optional"},
	}
	if issue := projectedExternalActionPlanIssue(
		state, "phase-optional", skills.SkillExternalApps, "execute_action", arguments,
	); issue != "" {
		t.Fatalf("optional projected Action was incorrectly prohibited: %q", issue)
	}
}

func TestValidateProjectedExternalActionPlanSnapshotRejectsUnexecutedTerminalPhase(t *testing.T) {
	for _, status := range []string{"completed", "skipped", "failed"} {
		t.Run(status, func(t *testing.T) {
			state := projectedExternalActionPlanTestState(nil)
			phase := projectedExternalActionPlanTestPhase("phase-new", status, "recipient_ref", "alice")
			if err := validateProjectedExternalActionPlanSnapshot([]map[string]interface{}{phase}, state); err == nil {
				t.Fatalf("new projected phase with status %q was accepted without runtime evidence", status)
			}
		})
	}

	state := projectedExternalActionPlanTestState(nil)
	phase := projectedExternalActionPlanTestPhase("phase-new", "in_progress", "recipient_ref", "alice")
	if err := validateProjectedExternalActionPlanSnapshot([]map[string]interface{}{phase}, state); err != nil {
		t.Fatalf("new open projected phase was rejected: %v", err)
	}
}

func TestServerExternalCapabilityRejectsUnrelatedModelSelectedActionWithoutIntentEvidence(t *testing.T) {
	baselinePhase := map[string]interface{}{
		"id": "phase-send", "outcome_id": "outcome-send", "step": "Send message", "status": "in_progress",
	}
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.calendar.delete",
			"binding_fingerprint": "binding-calendar-delete", "intent_matched": false,
			"intent_group": "calendar.delete", "effect": "delete",
			"target_argument_paths": []interface{}{"calendar_id"},
		}},
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{baselinePhase},
			"outcomes": []interface{}{map[string]interface{}{
				"id": "outcome-send", "goal": "Send a message", "required": true,
				"verification_mode": "runtime_effects", "capabilities": []interface{}{"external-apps"},
			}},
		},
	}
	next := copyStringAnyMap(baselinePhase)
	next["expected_action"] = map[string]interface{}{
		"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
		planExpectedActionServerProjectionKey:         "wecom_calendar_delete",
		planExpectedActionServerBindingFingerprintKey: "binding-calendar-delete",
		"target": map[string]interface{}{
			"integration_id": "wecom", "action_id": "wecom.calendar.delete",
		},
		"target_arguments": map[string]interface{}{"calendar_id": "calendar-1"},
	}
	if err := validateProjectedExternalActionPlanSnapshot([]map[string]interface{}{next}, state); err == nil {
		t.Fatal("server external send outcome accepted unrelated calendar.delete alias without intent evidence")
	}
}

func projectedExternalActionPlanTestState(phases []interface{}) map[string]interface{} {
	return map[string]interface{}{
		runtimeStateNativeExternalActionProjectionsKey: []interface{}{map[string]interface{}{
			"tool_name": "wecom_send_message", "integration_id": "wecom", "action_id": "wecom.message.send",
			"effect": "external_send", "target_argument_paths": []interface{}{"recipient_ref"},
			"preparation_action_keys": []interface{}{"wecom:wecom.contact.search"},
		}},
		"operation_plan": map[string]interface{}{"phases": phases},
	}
}

func projectedExternalActionPlanTestPhase(id string, status string, targetPath string, targetValue string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "step": id, "status": status, "required": true,
		"expected_action": map[string]interface{}{
			"skill_id":                            skills.SkillExternalApps,
			"tool_name":                           "execute_action",
			"projected_tool_name":                 "wecom_send_message",
			planExpectedActionServerProjectionKey: "wecom_send_message",
			"target": map[string]interface{}{
				"integration_id": "wecom",
				"action_id":      "wecom.message.send",
			},
			"target_arguments": map[string]interface{}{targetPath: targetValue},
		},
	}
}

func newPlanPhaseGuardTestRunner(t *testing.T) (*Runner, *skills.ResolvedSkills, *runnerProtocolEchoTool) {
	t.Helper()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "phase-guard-skill", `---
name: phase-guard-skill
description: Test operation-plan phase guard.
when_to_use: Use for phase guard tests.
provider_type: builtin
provider_id: protocol_batch
runtime_type: tool
tools:
  - echo_value
---

# Phase Guard

Call echo_value once.
`)
	echoTool := &runnerProtocolEchoTool{}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerProtocolEchoProvider{tool: echoTool}); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"phase-guard-skill"})
	if err != nil {
		t.Fatalf("resolve skill: %v", err)
	}
	return &Runner{SkillRuntime: runtime}, resolved, echoTool
}

func planPhaseGuardToolCall(t *testing.T, phaseID string) adapter.ToolCall {
	t.Helper()
	payload := map[string]interface{}{
		"skill_id":  "phase-guard-skill",
		"tool_name": "echo_value",
		"arguments": map[string]interface{}{"value": "hello"},
	}
	if phaseID != "" {
		payload["plan_phase_id"] = phaseID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.ToolCall{
		ID:   "call-phase-guard",
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      skills.MetaToolCallSkillTool,
			Arguments: string(encoded),
		},
	}
}
