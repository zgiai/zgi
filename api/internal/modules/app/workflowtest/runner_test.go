package workflowtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type runnerWorkflowServiceStub struct {
	interfaces.WorkflowService
	calls   []*dto.DraftWorkflowRunRequest
	results []dto.WorkflowRunResponse
	draft   interface{}
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
		s.calls = append(s.calls, runReq)
	}
	if len(s.results) == 0 {
		return dto.WorkflowRunResponse{TaskID: "task", WorkflowRunID: "run"}, nil
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result, nil
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
