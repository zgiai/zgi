package v1

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hibiken/asynq"
	workflow "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
)

func TestRegisterApprovalTaskHandlersIncludesConnectorResume(t *testing.T) {
	registry := &recordingApprovalTaskRegistry{}
	registerApprovalTaskHandlers(
		registry,
		nil,
		&approvalruntime.Service{},
		&workflow.WorkflowHandler{},
		approvalruntime.NewResumeHosts(),
	)

	want := map[string]bool{
		approvalruntime.TypeApprovalResume:  false,
		approvalruntime.TypeQuestionResume:  false,
		approvalruntime.TypeConnectorResume: false,
	}
	for _, taskType := range registry.taskTypes {
		if _, expected := want[taskType]; expected {
			want[taskType] = true
		}
	}
	for taskType, registered := range want {
		if !registered {
			t.Fatalf("task handler %q was not registered; got %v", taskType, registry.taskTypes)
		}
	}
}

type recordingApprovalTaskRegistry struct {
	taskTypes []string
	handlers  map[string]func(context.Context, *asynq.Task) error
}

func (registry *recordingApprovalTaskRegistry) Register(
	taskType string,
	handler func(context.Context, *asynq.Task) error,
) bool {
	registry.taskTypes = append(registry.taskTypes, taskType)
	if registry.handlers == nil {
		registry.handlers = map[string]func(context.Context, *asynq.Task) error{}
	}
	registry.handlers[taskType] = handler
	return true
}

type recordingResumeHost struct {
	requests []approvalruntime.ResumeRequest
}

func (h *recordingResumeHost) ResumeWorkflow(_ context.Context, req approvalruntime.ResumeRequest) (bool, error) {
	h.requests = append(h.requests, req)
	return true, nil
}

func TestQuestionResumeTasksLetTheConversationHostFollowTheWorkflow(t *testing.T) {
	registry := &recordingApprovalTaskRegistry{}
	hosts := approvalruntime.NewResumeHosts()
	host := &recordingResumeHost{}
	hosts.Register(host)
	registerApprovalTaskHandlers(registry, nil, &approvalruntime.Service{}, &workflow.WorkflowHandler{}, hosts)

	payload, err := json.Marshal(approvalruntime.QuestionResumeTaskPayload{WorkflowRunID: "run-1", Inputs: map[string]interface{}{"query": "Q4"}})
	if err != nil {
		t.Fatal(err)
	}
	handler := registry.handlers[approvalruntime.TypeQuestionResume]
	if handler == nil {
		t.Fatal("question resume handler was not registered")
	}
	if err := handler(context.Background(), asynq.NewTask(approvalruntime.TypeQuestionResume, payload)); err != nil {
		t.Fatalf("question resume task error = %v", err)
	}
	if len(host.requests) != 1 || host.requests[0].WorkflowRunID != "run-1" || host.requests[0].QuestionInputs["query"] != "Q4" {
		t.Fatalf("host requests = %#v, want the question resume handed to the conversation host", host.requests)
	}
}
