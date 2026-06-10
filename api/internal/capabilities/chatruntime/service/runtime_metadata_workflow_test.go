package service

import "testing"

func TestMergeWorkflowRunMetadataNestsIterationChildrenIntoRounds(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "iteration_started", map[string]interface{}{
		"workflow_run_id": "run-iteration",
		"node_id":         "iteration-node",
		"node_type":       "iteration",
		"title":           "Iterate",
		"created_at":      10,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "iteration_next", map[string]interface{}{
		"workflow_run_id": "run-iteration",
		"node_id":         "iteration-node",
		"node_type":       "iteration",
		"title":           "Iterate",
		"index":           1,
		"created_at":      11,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "node_started", map[string]interface{}{
		"workflow_run_id": "run-iteration",
		"node_id":         "llm-node",
		"node_type":       "llm",
		"title":           "LLM",
		"iteration_id":    "iteration-node",
		"iteration_index": 1,
		"inputs":          map[string]interface{}{"query": "hello"},
		"created_at":      12,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "node_finished", map[string]interface{}{
		"workflow_run_id": "run-iteration",
		"node_id":         "llm-node",
		"node_type":       "llm",
		"title":           "LLM",
		"iteration_id":    "iteration-node",
		"iteration_index": 1,
		"outputs":         map[string]interface{}{"text": "done"},
		"status":          "succeeded",
		"created_at":      13,
	})

	runs, ok := metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one run", metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	nodes, ok := run["nodes"].([]interface{})
	if !ok || len(nodes) != 1 {
		t.Fatalf("nodes = %#v, want one iteration node", run["nodes"])
	}
	iteration, _ := nodes[0].(map[string]interface{})
	rounds, ok := iteration["iteration_rounds"].([]interface{})
	if !ok || len(rounds) != 1 {
		t.Fatalf("iteration_rounds = %#v, want one round", iteration["iteration_rounds"])
	}
	round, _ := rounds[0].(map[string]interface{})
	if round["index"] != 1 {
		t.Fatalf("round = %#v, want index 1", round)
	}
	children, ok := round["nodes"].([]interface{})
	if !ok || len(children) != 1 {
		t.Fatalf("round nodes = %#v, want one child", round["nodes"])
	}
	child, _ := children[0].(map[string]interface{})
	if child["node_id"] != "llm-node" || child["status"] != "succeeded" || child["inputs"] == nil || child["outputs"] == nil {
		t.Fatalf("child = %#v, want merged llm child with inputs and outputs", child)
	}
}

func TestMergeWorkflowRunMetadataNormalizesLoopRoundsAndPreservesTitle(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "loop_started", map[string]interface{}{
		"workflow_run_id": "run-loop",
		"node_id":         "loop-node",
		"node_type":       "loop",
		"title":           "循环",
		"created_at":      10,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "loop_next", map[string]interface{}{
		"workflow_run_id": "run-loop",
		"node_id":         "loop-node",
		"node_type":       "loop",
		"title":           "循环",
		"index":           1,
		"created_at":      11,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "node_finished", map[string]interface{}{
		"workflow_run_id": "run-loop",
		"node_id":         "llm-node",
		"node_type":       "llm",
		"title":           "LLM",
		"loop_id":         "loop-node",
		"loop_index":      0,
		"outputs":         map[string]interface{}{"text": "done"},
		"status":          "succeeded",
		"created_at":      12,
	})

	runs, ok := metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one run", metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	nodes, ok := run["nodes"].([]interface{})
	if !ok || len(nodes) != 1 {
		t.Fatalf("nodes = %#v, want one loop node", run["nodes"])
	}
	loop, _ := nodes[0].(map[string]interface{})
	if loop["title"] != "循环" {
		t.Fatalf("loop = %#v, want original title preserved", loop)
	}
	rounds, ok := loop["loop_rounds"].([]interface{})
	if !ok || len(rounds) != 1 {
		t.Fatalf("loop_rounds = %#v, want one round", loop["loop_rounds"])
	}
	round, _ := rounds[0].(map[string]interface{})
	if round["index"] != 0 {
		t.Fatalf("round = %#v, want normalized index 0", round)
	}
	children, ok := round["nodes"].([]interface{})
	if !ok || len(children) != 1 {
		t.Fatalf("round nodes = %#v, want one child", round["nodes"])
	}
	child, _ := children[0].(map[string]interface{})
	if child["node_id"] != "llm-node" || child["status"] != "succeeded" {
		t.Fatalf("child = %#v, want merged loop child", child)
	}
}

func TestMergeWorkflowRunMetadataAddsIterationRoundDurationsFromCompletedMetadata(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "iteration_started", map[string]interface{}{
		"workflow_run_id": "run-iteration-durations",
		"node_id":         "iteration-node",
		"node_type":       "iteration",
		"title":           "Iterate",
		"created_at":      10,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "iteration_next", map[string]interface{}{
		"workflow_run_id": "run-iteration-durations",
		"node_id":         "iteration-node",
		"node_type":       "iteration",
		"title":           "Iterate",
		"index":           0,
		"created_at":      11,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "node_finished", map[string]interface{}{
		"workflow_run_id": "run-iteration-durations",
		"node_id":         "llm-node",
		"node_type":       "llm",
		"title":           "LLM",
		"iteration_id":    "iteration-node",
		"iteration_index": 0,
		"elapsed_time":    float64(1200),
		"outputs":         map[string]interface{}{"text": "done"},
		"status":          "succeeded",
		"created_at":      12,
		"execution_id":    "llm-node:0",
	})
	metadata = mergeWorkflowRunMetadata(metadata, "iteration_completed", map[string]interface{}{
		"workflow_run_id": "run-iteration-durations",
		"node_id":         "iteration-node",
		"node_type":       "iteration",
		"title":           "Iterate",
		"status":          "succeeded",
		"elapsed_time":    float64(2500),
		"execution_metadata": map[string]interface{}{
			"iteration_duration_list": []float64{1500, 2300},
			"iteration_duration_map":  map[string]float64{"0": 1400, "1": 2200},
		},
		"created_at": 13,
	})

	runs, ok := metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one run", metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	nodes, _ := run["nodes"].([]interface{})
	iteration, _ := nodes[0].(map[string]interface{})
	rounds, ok := iteration["iteration_rounds"].([]interface{})
	if !ok || len(rounds) != 2 {
		t.Fatalf("iteration_rounds = %#v, want two rounds with durations", iteration["iteration_rounds"])
	}
	first, _ := rounds[0].(map[string]interface{})
	if first["elapsed_time"] != float64(1500) {
		t.Fatalf("first round = %#v, want elapsed_time from duration list", first)
	}
	children, ok := first["nodes"].([]interface{})
	if !ok || len(children) != 1 {
		t.Fatalf("first round nodes = %#v, want existing child preserved", first["nodes"])
	}
	second, _ := rounds[1].(map[string]interface{})
	if second["index"] != 1 || second["elapsed_time"] != float64(2300) {
		t.Fatalf("second round = %#v, want index 1 with elapsed_time", second)
	}
}

func TestMergeWorkflowRunMetadataAddsLoopRoundDurationsFromCompletedMetadata(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "loop_started", map[string]interface{}{
		"workflow_run_id": "run-loop-durations",
		"node_id":         "loop-node",
		"node_type":       "loop",
		"title":           "Loop",
		"created_at":      10,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "loop_next", map[string]interface{}{
		"workflow_run_id": "run-loop-durations",
		"node_id":         "loop-node",
		"node_type":       "loop",
		"title":           "Loop",
		"index":           1,
		"created_at":      11,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "loop_completed", map[string]interface{}{
		"workflow_run_id": "run-loop-durations",
		"node_id":         "loop-node",
		"node_type":       "loop",
		"title":           "Loop",
		"status":          "succeeded",
		"elapsed_time":    float64(3500),
		"execution_metadata": map[string]interface{}{
			"loop_duration_map": map[string]float64{"0": 1800, "1": 900},
		},
		"created_at": 13,
	})

	runs, ok := metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one run", metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	nodes, _ := run["nodes"].([]interface{})
	loop, _ := nodes[0].(map[string]interface{})
	rounds, ok := loop["loop_rounds"].([]interface{})
	if !ok || len(rounds) != 2 {
		t.Fatalf("loop_rounds = %#v, want two rounds with durations", loop["loop_rounds"])
	}
	first, _ := rounds[0].(map[string]interface{})
	if first["index"] != 0 || first["elapsed_time"] != float64(1800) {
		t.Fatalf("first round = %#v, want elapsed_time from loop duration map", first)
	}
	second, _ := rounds[1].(map[string]interface{})
	if second["index"] != 1 || second["elapsed_time"] != float64(900) {
		t.Fatalf("second round = %#v, want index 1 with elapsed_time", second)
	}
}

func TestMergeWorkflowRunMetadataKeepsTerminalWorkflowAndContainerState(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "loop_started", map[string]interface{}{
		"workflow_run_id": "run-terminal",
		"node_id":         "loop-node",
		"node_type":       "loop",
		"title":           "循环",
		"created_at":      10,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "loop_completed", map[string]interface{}{
		"workflow_run_id": "run-terminal",
		"node_id":         "loop-node",
		"node_type":       "loop",
		"title":           "循环",
		"status":          "succeeded",
		"elapsed_time":    float64(5.1),
		"outputs":         map[string]interface{}{"result": "ok"},
		"created_at":      20,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "workflow_finished", map[string]interface{}{
		"workflow_run_id": "run-terminal",
		"status":          "succeeded",
		"elapsed_time":    float64(6.2),
		"outputs":         map[string]interface{}{"answer": "done"},
		"created_at":      21,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "node_started", map[string]interface{}{
		"workflow_run_id": "run-terminal",
		"node_id":         "late-llm",
		"node_type":       "llm",
		"title":           "LLM",
		"loop_id":         "loop-node",
		"loop_index":      0,
		"created_at":      22,
	})

	runs, ok := metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one run", metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	if run["status"] != "succeeded" || run["elapsed_time"] != float64(6.2) {
		t.Fatalf("run = %#v, want terminal status and elapsed time preserved", run)
	}
	nodes, _ := run["nodes"].([]interface{})
	loop, _ := nodes[0].(map[string]interface{})
	if loop["title"] != "循环" || loop["status"] != "succeeded" || loop["elapsed_time"] != float64(5.1) {
		t.Fatalf("loop = %#v, want terminal container fields preserved", loop)
	}
	rounds, ok := loop["loop_rounds"].([]interface{})
	if !ok || len(rounds) != 1 {
		t.Fatalf("loop_rounds = %#v, want late child merged into rounds", loop["loop_rounds"])
	}
}

func TestMergeWorkflowRunMetadataStoresApprovalFields(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "workflow_paused", map[string]interface{}{
		"workflow_run_id":  "run-approval",
		"node_id":          "approval-node",
		"node_type":        "approval",
		"status":           "paused",
		"approval_form_id": "form-1",
		"approval_token":   "token-1",
		"approval_url":     "https://example.test/approval",
	})

	runs, ok := metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one run", metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	approval, _ := run["approval"].(map[string]interface{})
	if run["status"] != "pending_approval" || approval["approval_form_id"] != "form-1" || approval["approval_token"] != "token-1" {
		t.Fatalf("run = %#v, want pending approval with safe fields", run)
	}
}

func TestMergeWorkflowRunMetadataStoresQuestionAnswerFields(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "question_answer_requested", map[string]interface{}{
		"workflow_run_id": "run-question",
		"node_id":         "qa-node",
		"node_type":       "question-answer",
		"question":        "Pick one",
		"round":           2,
		"choices": []map[string]interface{}{
			{"id": "yes", "label": "Yes", "value": "yes"},
			{"id": "no", "label": "No", "value": "no"},
		},
	})

	runs, ok := metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one run", metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	question, _ := run["question_answer"].(map[string]interface{})
	if run["status"] != "pending_question" || question["question"] != "Pick one" || question["node_id"] != "qa-node" {
		t.Fatalf("run = %#v, want pending question with question fields", run)
	}
}

func TestMergeWorkflowRunMetadataStoresApprovalLifecycleAndMessages(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "approval_requested", map[string]interface{}{
		"workflow_run_id":  "run-lifecycle",
		"approval_form_id": "form-1",
		"approval_token":   "token-1",
		"created_at":       9,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "approval_result_filled", map[string]interface{}{
		"workflow_run_id": "run-lifecycle",
		"form_id":         "form-1",
		"action":          "approve",
		"inputs":          map[string]interface{}{"comment": "ok"},
		"created_at":      10,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "message", map[string]interface{}{
		"workflow_run_id": "run-lifecycle",
		"answer":          "hello",
		"created_at":      11,
	})
	metadata = mergeWorkflowRunMetadata(metadata, "message_end", map[string]interface{}{
		"workflow_run_id": "run-lifecycle",
		"metadata":        map[string]interface{}{"usage": map[string]interface{}{"total_tokens": 3}},
		"created_at":      12,
	})

	runs, ok := metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one run", metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	approval, _ := run["approval"].(map[string]interface{})
	if approval["approval_token"] != "token-1" || approval["status"] != "submitted" {
		t.Fatalf("approval = %#v, want historical token with submitted status", approval)
	}
	result, _ := run["approval_result"].(map[string]interface{})
	if result["form_id"] != "form-1" || result["action"] != "approve" {
		t.Fatalf("approval_result = %#v, want submitted approval fields", result)
	}
	results, ok := run["approval_results"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("approval_results = %#v, want submitted transcript", run["approval_results"])
	}
	messages, ok := run["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want message and message_end events", run["messages"])
	}
}

func TestMergeWorkflowRunMetadataStoresStoppedAndExpiredStatuses(t *testing.T) {
	stopped := mergeWorkflowRunMetadata(nil, "workflow_stopped", map[string]interface{}{
		"workflow_run_id": "run-stopped",
		"status":          "stopped",
	})
	runs, _ := stopped["workflow_runs"].([]interface{})
	run, _ := runs[0].(map[string]interface{})
	if run["status"] != "stopped" {
		t.Fatalf("run = %#v, want stopped status", run)
	}

	expired := mergeWorkflowRunMetadata(nil, "approval_expired", map[string]interface{}{
		"workflow_run_id": "run-expired",
		"form_id":         "form-1",
		"reason":          "timeout",
	})
	runs, _ = expired["workflow_runs"].([]interface{})
	run, _ = runs[0].(map[string]interface{})
	approvalExpired, _ := run["approval_expired"].(map[string]interface{})
	approval, _ := run["approval"].(map[string]interface{})
	if run["status"] != "expired" || approvalExpired["reason"] != "timeout" {
		t.Fatalf("run = %#v, want expired approval status", run)
	}
	if approval["status"] != "expired" {
		t.Fatalf("approval = %#v, want expired status", approval)
	}
}

func TestWorkflowQuestionUserInputRequestMapsChoices(t *testing.T) {
	request := workflowQuestionUserInputRequest("conversation-1", "message-1", map[string]interface{}{
		"workflow_run_id": "run-question",
		"node_id":         "qa-node",
		"question":        "Pick one",
		"round":           1,
		"choices": []map[string]interface{}{
			{"id": "yes", "label": "Yes", "value": "yes"},
		},
	})

	if request["source"] != "agent_workflow_question_answer" || request["workflow_run_id"] != "run-question" {
		t.Fatalf("request = %#v, want agent workflow question request", request)
	}
	questions, ok := request["questions"].([]interface{})
	if !ok || len(questions) != 1 {
		t.Fatalf("questions = %#v, want one question", request["questions"])
	}
	question, _ := questions[0].(map[string]interface{})
	options, ok := question["options"].([]interface{})
	if !ok || len(options) != 1 {
		t.Fatalf("options = %#v, want one option", question["options"])
	}
	option, _ := options[0].(map[string]interface{})
	if option["option_id"] != "yes" || option["value"] != "yes" || option["label"] != "Yes" {
		t.Fatalf("option = %#v, want stable option mapping", option)
	}
}
