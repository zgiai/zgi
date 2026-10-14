package v1

import (
	"context"
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
}

func (registry *recordingApprovalTaskRegistry) Register(
	taskType string,
	_ func(context.Context, *asynq.Task) error,
) bool {
	registry.taskTypes = append(registry.taskTypes, taskType)
	return true
}
