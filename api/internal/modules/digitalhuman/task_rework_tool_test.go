package digitalhuman

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type reworkTaskService struct {
	taskRuntimeService
	reports  []*TaskReport
	reviewed *ReviewTaskReportRequest
	reviewID uuid.UUID
}

func (s *reworkTaskService) SubmitTaskReport(context.Context, TaskAccessContext, SubmitTaskReportRequest) (*TaskReport, error) {
	return nil, ErrInvalidRequest
}

func (s *reworkTaskService) ListTaskReports(_ context.Context, _ TaskAccessContext, query TaskReportListQuery) (*TaskReportList, error) {
	if query.TaskID == nil {
		return nil, ErrInvalidRequest
	}
	return &TaskReportList{Items: s.reports}, nil
}

func (s *reworkTaskService) GetTaskReport(context.Context, TaskAccessContext, uuid.UUID) (*TaskReport, error) {
	return nil, ErrTaskNotFound
}

func (s *reworkTaskService) ReviewTaskReport(_ context.Context, _ TaskAccessContext, reportID uuid.UUID, req ReviewTaskReportRequest, _ TaskMutationContext) (*TaskReport, error) {
	s.reviewed, s.reviewID = &req, reportID
	return &TaskReport{ID: reportID, ReviewState: TaskReportReworkRequested}, nil
}

func TestTheDigitalHumanReworksAReportFromTheConversation(t *testing.T) {
	run, event := taskRuntimeFixture(invocation.OriginChannel)
	taskID, retried, final := uuid.New(), uuid.New(), uuid.New()
	service := &reworkTaskService{reports: []*TaskReport{{ID: retried, TaskID: taskID}, {ID: final, TaskID: taskID, Final: true}}}
	preparation, err := NewTaskRuntimeProvider(service).Prepare(t.Context(), run, event)
	if err != nil {
		t.Fatal(err)
	}
	var rework *adapter.Tool
	handlerIndex := -1
	for index, tool := range preparation.Tools {
		if tool.Definition.Function.Name == taskReworkToolName {
			rework, handlerIndex = &preparation.Tools[index].Definition, index
		}
	}
	if rework == nil || !strings.Contains(preparation.StableGuidance, taskReworkToolName) ||
		!strings.Contains(preparation.StableGuidance, "Never ask the user to reply with a keyword") {
		t.Fatalf("conversation tools = %#v", preparation.Tools)
	}
	result := preparation.Tools[handlerIndex].Handler(t.Context(), adapter.ToolCall{ID: "rework", Function: adapter.FunctionCall{
		Name: taskReworkToolName, Arguments: `{"task_id":"` + taskID.String() + `","feedback":"补充北京的数据"}`,
	}})
	if result.Status == "failed" || service.reviewed == nil || service.reviewID != final ||
		service.reviewed.Action != TaskReportRework || service.reviewed.Feedback != "补充北京的数据" {
		t.Fatalf("rework = %#v, reviewed %v %#v", result, service.reviewID, service.reviewed)
	}
	empty := preparation.Tools[handlerIndex].Handler(t.Context(), adapter.ToolCall{ID: "empty", Function: adapter.FunctionCall{
		Name: taskReworkToolName, Arguments: `{"task_id":"` + taskID.String() + `","feedback":" "}`,
	}})
	if empty.Status != "failed" {
		t.Fatalf("rework without feedback = %#v", empty)
	}
}
