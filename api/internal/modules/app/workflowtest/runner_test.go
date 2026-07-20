package workflowtest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type runnerWorkflowServiceStub struct {
	interfaces.WorkflowService
	calls                  []*dto.DraftWorkflowRunRequest
	results                []dto.WorkflowRunResponse
	rawResults             []interface{}
	runErrors              []error
	draft                  interface{}
	createdConversationIDs []string
	nodeExecutions         map[string]*dto.WorkflowRunNodeExecutionListResponse
}

func (s *runnerWorkflowServiceStub) GetDraftWorkflow(ctx context.Context, agentID string, hideSecrets ...bool) (interface{}, error) {
	if s.draft == nil {
		return nil, nil
	}
	return s.draft, nil
}

func (s *runnerWorkflowServiceStub) RunDraftWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error) {
	runReq, ok := req.(*dto.DraftWorkflowRunRequest)
	if ok {
		if len(s.createdConversationIDs) > 0 {
			if convID, _ := runReq.Inputs["sys.conversation_id"].(string); convID == "" {
				runReq.Inputs["sys.conversation_id"] = s.createdConversationIDs[0]
				s.createdConversationIDs = s.createdConversationIDs[1:]
			}
		}
		s.calls = append(s.calls, runReq)
	}
	var result interface{}
	if len(s.rawResults) > 0 {
		result = s.rawResults[0]
		s.rawResults = s.rawResults[1:]
	} else if len(s.results) > 0 {
		result = s.results[0]
		s.results = s.results[1:]
	} else {
		result = dto.WorkflowRunResponse{TaskID: "task", WorkflowRunID: "run"}
	}
	var err error
	if len(s.runErrors) > 0 {
		err = s.runErrors[0]
		s.runErrors = s.runErrors[1:]
	}
	return result, err
}

func (s *runnerWorkflowServiceStub) GetWorkflowRunNodeExecutions(ctx context.Context, tenantID, agentID, runID string) (interface{}, error) {
	if s.nodeExecutions == nil {
		return &dto.WorkflowRunNodeExecutionListResponse{}, nil
	}
	if executions, ok := s.nodeExecutions[runID]; ok {
		return executions, nil
	}
	return &dto.WorkflowRunNodeExecutionListResponse{}, nil
}

func TestWorkflowServiceRunnerMapsAttachmentToStartFileVariable(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "question", "type": "text-input"},
								map[string]interface{}{"variable": "file", "type": "file"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{
					Role:    "user",
					Content: "这个文件的内容是什么？",
					Attachments: []CaseAttachment{
						{Type: "document", TransferMethod: "local_file", UploadFileID: "file-1", Name: "测试文档.docx"},
					},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	require.Equal(t, "这个文件的内容是什么？", workflowService.calls[0].Inputs["question"])
	require.Equal(t, []interface{}{
		map[string]interface{}{
			"type":            "document",
			"transfer_method": "local_file",
			"url":             "",
			"upload_file_id":  "file-1",
			"name":            "测试文档.docx",
		},
	}, workflowService.calls[0].Inputs["sys.files"])
	require.Equal(t, map[string]interface{}{
		"type":            "document",
		"transfer_method": "local_file",
		"url":             "",
		"upload_file_id":  "file-1",
		"name":            "测试文档.docx",
	}, workflowService.calls[0].Inputs["file"])
}

func TestWorkflowServiceRunnerSkipsNonUserTurns(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		results: []dto.WorkflowRunResponse{
			{TaskID: "task-1", WorkflowRunID: "run-1"},
			{TaskID: "task-2", WorkflowRunID: "run-2"},
		},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	result, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{Role: "user", Content: "First user input"},
				{Role: "assistant", Content: "Assistant reply should not be replayed"},
				{Role: "system", Content: "System note should not be replayed"},
				{Role: "user", Content: "Follow-up user input"},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 2)
	require.Equal(t, "First user input", workflowService.calls[0].Inputs["sys.query"])
	require.Equal(t, "Follow-up user input", workflowService.calls[1].Inputs["sys.query"])
	require.Equal(t, 2, result.Outputs["turn_count"])
}

func TestWorkflowServiceRunnerMapsAttachmentsToStartFileListVariable(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "files", "type": "file-list"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{
					Role:    "user",
					Content: "比较这两个文件",
					Attachments: []CaseAttachment{
						{Type: "document", TransferMethod: "local_file", UploadFileID: "file-1", Name: "A.docx"},
						{Type: "document", TransferMethod: "local_file", UploadFileID: "file-2", Name: "B.docx"},
					},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	require.Equal(t, []interface{}{
		map[string]interface{}{
			"type":            "document",
			"transfer_method": "local_file",
			"url":             "",
			"upload_file_id":  "file-1",
			"name":            "A.docx",
		},
		map[string]interface{}{
			"type":            "document",
			"transfer_method": "local_file",
			"url":             "",
			"upload_file_id":  "file-2",
			"name":            "B.docx",
		},
	}, workflowService.calls[0].Inputs["files"])
}

func TestWorkflowServiceRunnerUsesUnifiedDraftWorkflowForChatDraft(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: map[string]interface{}{
			"type": "chat",
			"graph": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "file", "type": "file"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{
					Role:    "user",
					Content: "这个文件的内容是什么？",
					Attachments: []CaseAttachment{
						{Type: "document", TransferMethod: "local_file", UploadFileID: "file-1", Name: "测试文档.docx"},
					},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	call := workflowService.calls[0]
	require.Equal(t, "blocking", call.ResponseMode)
	require.Equal(t, "这个文件的内容是什么？", call.Inputs["sys.query"])
	require.Equal(t, "这个文件的内容是什么？", call.Inputs["query"])
	require.Equal(t, "chat", call.Inputs["sys.workflow_type"])
	require.Equal(t, 1, call.Inputs["sys.dialogue_count"])
	require.Equal(t, "", call.Inputs["sys.parent_message_id"])
	require.Equal(t, map[string]interface{}{
		"from_source": "account",
		"invoke_from": "debugger",
	}, call.Inputs["conversation_params"])
	require.Equal(t, map[string]interface{}{
		"type":            "document",
		"transfer_method": "local_file",
		"url":             "",
		"upload_file_id":  "file-1",
		"name":            "测试文档.docx",
	}, call.Inputs["file"])
}

func TestWorkflowServiceRunnerUsesUnifiedDraftWorkflowForTypedChatDraft(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Type: dto.WorkflowTypeChat,
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "query", "type": "text-input"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{Role: "user", Content: "这个文件的内容是什么？"},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	call := workflowService.calls[0]
	require.Equal(t, "blocking", call.ResponseMode)
	require.Equal(t, "这个文件的内容是什么？", call.Inputs["query"])
	require.Equal(t, "chat", call.Inputs["sys.workflow_type"])
	require.Equal(t, 1, call.Inputs["sys.dialogue_count"])
	require.Equal(t, "", call.Inputs["sys.parent_message_id"])
	require.Equal(t, map[string]interface{}{
		"from_source": "account",
		"invoke_from": "debugger",
	}, call.Inputs["conversation_params"])
}

func TestWorkflowServiceRunnerFiltersWorkflowTestMetadataInputs(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "question", "type": "text-input"},
								map[string]interface{}{"variable": "customer", "type": "string"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Content: "question",
			Turns: CaseTurns{{
				Role:    "user",
				Content: "question",
				Attachments: []CaseAttachment{{
					TransferMethod: "local_file",
					UploadFileID:   "generated-file",
					Name:           "fixture.docx",
				}},
				Inputs: JSONMap{
					"customer":              "Acme",
					caseModeInputKey:        "task",
					expectedChecksInputKey:  map[string]interface{}{"output_contains": []interface{}{"ok"}},
					turnExpectationInputKey: "expected",
					turnChecksInputKey:      map[string]interface{}{"conditions": []interface{}{}},
					conversationChecksInputKey: map[string]interface{}{
						"conditions": []interface{}{},
					},
					"__fixture_spec": []interface{}{map[string]interface{}{
						"upload_file_id": "generated-file",
						"name":           "fixture.docx",
					}},
					"__asset_source": "workflow_test_generated",
					"__tags":         []interface{}{"tag"},
				},
			}},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	callInputs := workflowService.calls[0].Inputs
	require.Equal(t, "Acme", callInputs["customer"])
	require.NotContains(t, callInputs, caseModeInputKey)
	require.NotContains(t, callInputs, expectedChecksInputKey)
	require.NotContains(t, callInputs, turnExpectationInputKey)
	require.NotContains(t, callInputs, turnChecksInputKey)
	require.NotContains(t, callInputs, conversationChecksInputKey)
	require.NotContains(t, callInputs, "__fixture_spec")
	require.NotContains(t, callInputs, "__asset_source")
	require.NotContains(t, callInputs, "__tags")
}

func TestWorkflowServiceRunnerAttachesWorkflowTrace(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft:   dto.WorkflowDetail{},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
		nodeExecutions: map[string]*dto.WorkflowRunNodeExecutionListResponse{
			"run-1": {
				Data: []dto.WorkflowRunNodeExecutionResponse{{
					ID:          "exec-1",
					NodeID:      "start",
					NodeType:    "start",
					Title:       "Start",
					Status:      "succeeded",
					Inputs:      json.RawMessage(`{"query":"hello"}`),
					Outputs:     json.RawMessage(`{"answer":"world"}`),
					ElapsedTime: 12,
				}},
			},
		},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	result, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Content: "hello",
		},
	})

	require.NoError(t, err)
	trace, ok := result.Outputs["workflow_trace"].(map[string]interface{})
	require.True(t, ok)
	nodes, ok := trace["nodes"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, nodes, 1)
	require.Equal(t, "start", nodes[0]["node_id"])
	require.Equal(t, map[string]interface{}{"query": "hello"}, nodes[0]["input"])
	require.Equal(t, map[string]interface{}{"answer": "world"}, nodes[0]["output"])
}

func TestWorkflowServiceRunnerPromotesAnswerFromTrace(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft:   dto.WorkflowDetail{},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
		nodeExecutions: map[string]*dto.WorkflowRunNodeExecutionListResponse{
			"run-1": {
				Data: []dto.WorkflowRunNodeExecutionResponse{{
					ID:       "exec-1",
					NodeID:   "answer_node",
					NodeType: "answer",
					Title:    "Answer",
					Status:   "succeeded",
					Outputs:  json.RawMessage(`{"answer":"hello from agent"}`),
				}},
			},
		},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	result, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID:      "agent-1",
		CaseSnapshot: CaseSnapshot{Content: "hello"},
	})

	require.NoError(t, err)
	require.Equal(t, "hello from agent", result.Outputs["answer"])
}

func TestWorkflowServiceRunnerKeepsScalarWorkflowTraceOutput(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft:   dto.WorkflowDetail{},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
		nodeExecutions: map[string]*dto.WorkflowRunNodeExecutionListResponse{
			"run-1": {
				Data: []dto.WorkflowRunNodeExecutionResponse{{
					ID:      "exec-1",
					NodeID:  "answer",
					Title:   "Answer",
					Status:  "succeeded",
					Outputs: json.RawMessage(`"final answer"`),
				}},
			},
		},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	result, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID:      "agent-1",
		CaseSnapshot: CaseSnapshot{Content: "hello"},
	})

	require.NoError(t, err)
	trace := result.Outputs["workflow_trace"].(map[string]interface{})
	nodes := trace["nodes"].([]map[string]interface{})
	require.Equal(t, map[string]interface{}{"value": "final answer"}, nodes[0]["output"])
	require.Equal(t, "final answer", result.Outputs["answer"])
}

func TestWorkflowServiceRunnerKeepsConversationContextAcrossChatTurns(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Type: dto.WorkflowTypeChat,
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "query", "type": "text-input"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{
			{TaskID: "task-1", WorkflowRunID: "run-1"},
			{TaskID: "task-2", WorkflowRunID: "run-2"},
		},
		createdConversationIDs: []string{"conversation-1"},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	result, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{Role: "user", Content: "first turn"},
				{Role: "user", Content: "follow up"},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 2)
	require.Equal(t, "conversation-1", workflowService.calls[0].Inputs["sys.conversation_id"])
	require.Equal(t, 1, workflowService.calls[0].Inputs["sys.dialogue_count"])
	require.Equal(t, "conversation-1", workflowService.calls[1].Inputs["sys.conversation_id"])
	require.Equal(t, 2, workflowService.calls[1].Inputs["sys.dialogue_count"])
	require.NotContains(t, workflowService.calls[1].Inputs, "sys.parent_message_id")
	turnResults, ok := result.Outputs["turn_results"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, turnResults, 2)
	require.Equal(t, "conversation-1", turnResults[1]["conversation_id"])
}

func TestWorkflowServiceRunnerPassesAttachmentToSystemFilesForChatDocumentExtractor(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Type: dto.WorkflowTypeChat,
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type":              "document-extractor",
							"variable_selector": []interface{}{"sys", "files"},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{{
				Role:    "user",
				Content: "请总结附件",
				Attachments: []CaseAttachment{{
					Type:           "document",
					TransferMethod: "local_file",
					UploadFileID:   "file-1",
					Name:           "contract.docx",
				}},
			}},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	require.Equal(t, []interface{}{
		map[string]interface{}{
			"type":            "document",
			"transfer_method": "local_file",
			"url":             "",
			"upload_file_id":  "file-1",
			"name":            "contract.docx",
		},
	}, workflowService.calls[0].Inputs["sys.files"])
}

func TestWorkflowServiceRunnerRejectsLaterChatTurnWithoutRequiredAttachment(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Type: dto.WorkflowTypeChat,
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"id":   "start",
						"data": map[string]interface{}{"type": "start"},
					},
					map[string]interface{}{
						"id": "extract",
						"data": map[string]interface{}{
							"type":              "document-extractor",
							"variable_selector": []interface{}{"sys", "files"},
						},
					},
					map[string]interface{}{
						"id":   "answer",
						"data": map[string]interface{}{"type": "answer"},
					},
				},
				"edges": []interface{}{
					map[string]interface{}{"source": "start", "target": "extract"},
					map[string]interface{}{"source": "extract", "target": "answer"},
				},
			},
		},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	result, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{
					Role:    "user",
					Content: "请总结附件",
					Attachments: []CaseAttachment{{
						Type:           "document",
						TransferMethod: "local_file",
						UploadFileID:   "file-1",
						Name:           "contract.docx",
					}},
				},
				{Role: "user", Content: "请把第三点说得更具体"},
			},
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "第 2 轮缺少附件")
	require.Nil(t, result)
	require.Empty(t, workflowService.calls)
}

func TestWorkflowServiceRunnerStopsOnFailedWorkflowStatusAndPreservesPartialResult(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		rawResults: []interface{}{
			map[string]interface{}{
				"workflow_run_id": "run-1",
				"status":          "failed",
				"error":           "variable not found: [sys files]",
			},
			map[string]interface{}{
				"workflow_run_id": "run-2",
				"status":          "succeeded",
			},
		},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	result, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{Turns: CaseTurns{
			{Role: "user", Content: "first turn"},
			{Role: "user", Content: "follow up"},
		}},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "variable not found: [sys files]")
	require.Len(t, workflowService.calls, 1)
	require.NotNil(t, result)
	require.Equal(t, "run-1", result.WorkflowRunID)
	require.Equal(t, 1, result.Outputs["turn_count"])
	require.Equal(t, 2, result.Outputs["planned_turn_count"])
	turnResults, ok := result.Outputs["turn_results"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, turnResults, 1)
	require.Equal(t, "run-1", turnResults[0]["workflow_run_id"])
}

func TestWorkflowServiceRunnerMapsAttachmentsToMultipleSingleFileVariables(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "primary_doc", "type": "file-input"},
								map[string]interface{}{"variable": "reference_doc", "type": "file"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{
					Role:    "user",
					Content: "分别读取两个文档",
					Attachments: []CaseAttachment{
						{Type: "document", TransferMethod: "local_file", UploadFileID: "file-1", Name: "主文档.docx"},
						{Type: "document", TransferMethod: "local_file", UploadFileID: "file-2", Name: "参考文档.docx"},
					},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	require.Equal(t, map[string]interface{}{
		"type":            "document",
		"transfer_method": "local_file",
		"url":             "",
		"upload_file_id":  "file-1",
		"name":            "主文档.docx",
	}, workflowService.calls[0].Inputs["primary_doc"])
	require.Equal(t, map[string]interface{}{
		"type":            "document",
		"transfer_method": "local_file",
		"url":             "",
		"upload_file_id":  "file-2",
		"name":            "参考文档.docx",
	}, workflowService.calls[0].Inputs["reference_doc"])
}

func TestWorkflowServiceRunnerDoesNotOverrideExplicitFileInput(t *testing.T) {
	explicitFile := map[string]interface{}{
		"type":            "document",
		"transfer_method": "local_file",
		"url":             "",
		"upload_file_id":  "explicit-file",
		"name":            "手动指定.docx",
	}
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "file", "type": "file"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{
					Role:    "user",
					Content: "读取指定文件",
					Inputs: map[string]interface{}{
						"file": explicitFile,
					},
					Attachments: []CaseAttachment{
						{Type: "document", TransferMethod: "local_file", UploadFileID: "attached-file", Name: "附件.docx"},
					},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	require.Equal(t, explicitFile, workflowService.calls[0].Inputs["file"])
	require.Equal(t, []interface{}{
		map[string]interface{}{
			"type":            "document",
			"transfer_method": "local_file",
			"url":             "",
			"upload_file_id":  "attached-file",
			"name":            "附件.docx",
		},
	}, workflowService.calls[0].Inputs["#files#"])
}

func TestWorkflowServiceRunnerRunsAttachmentOnlyTurn(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "file", "type": "file"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{{TaskID: "task-1", WorkflowRunID: "run-1"}},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	_, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{
					Role: "user",
					Attachments: []CaseAttachment{
						{Type: "document", TransferMethod: "local_file", UploadFileID: "file-1", Name: "测试文档.docx"},
					},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 1)
	require.Equal(t, "", workflowService.calls[0].Inputs["sys.query"])
	require.Equal(t, "", workflowService.calls[0].Inputs["input1"])
	require.Equal(t, map[string]interface{}{
		"type":            "document",
		"transfer_method": "local_file",
		"url":             "",
		"upload_file_id":  "file-1",
		"name":            "测试文档.docx",
	}, workflowService.calls[0].Inputs["file"])
	require.Len(t, workflowService.calls[0].Files, 1)
}

func TestWorkflowServiceRunnerAggregatesMultiTurnOutputs(t *testing.T) {
	workflowService := &runnerWorkflowServiceStub{
		draft: dto.WorkflowDetail{
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"data": map[string]interface{}{
							"type": "start",
							"variables": []interface{}{
								map[string]interface{}{"variable": "question", "type": "text-input"},
							},
						},
					},
				},
			},
		},
		results: []dto.WorkflowRunResponse{
			{TaskID: "task-1", WorkflowRunID: "run-1"},
			{TaskID: "task-2", WorkflowRunID: "run-2"},
		},
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: workflowService,
		WorkspaceID:     "workspace-1",
		AccountID:       "account-1",
	}

	result, err := runner.RunCase(context.Background(), RunCaseRequest{
		AgentID: "agent-1",
		CaseSnapshot: CaseSnapshot{
			Turns: CaseTurns{
				{Role: "user", Content: "第一轮问题"},
				{Role: "user", Content: "第二轮追问"},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, workflowService.calls, 2)
	require.Equal(t, "run-2", result.WorkflowRunID)
	require.Equal(t, 2, result.Outputs["turn_count"])
	require.Equal(t, "run-2", result.Outputs["workflow_run_id"])
	turnResults, ok := result.Outputs["turn_results"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, turnResults, 2)
	require.Equal(t, 1, turnResults[0]["turn_index"])
	require.Equal(t, "第一轮问题", turnResults[0]["content"])
	require.Equal(t, "run-1", turnResults[0]["workflow_run_id"])
	require.Equal(t, 2, turnResults[1]["turn_index"])
	require.Equal(t, "第二轮追问", turnResults[1]["content"])
	require.Equal(t, "run-2", turnResults[1]["workflow_run_id"])
}
