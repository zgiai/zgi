package inbox

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type taskRegistryStub struct{}

func (taskRegistryStub) Register(string, func(context.Context, *asynq.Task) error) bool { return true }

type taskQueueStub struct {
	ids      []string
	conflict map[string]bool
}

func (q *taskQueueStub) EnqueueTask(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	id := ""
	for _, option := range opts {
		if option.Type() == asynq.TaskIDOpt {
			id = option.Value().(string)
		}
	}
	q.ids = append(q.ids, id)
	if q.conflict[id] {
		return nil, asynq.ErrTaskIDConflict
	}
	return &asynq.TaskInfo{ID: id}, nil
}

func (q *taskQueueStub) GetTaskTypeWithPrefix(taskType string) string { return taskType }

// A delivery's first send stays one queue task; a manual redelivery is a task
// of its own, because the first one stays archived after its retries ran out.
func TestRedeliveryEnqueuesItsOwnTask(t *testing.T) {
	deliveryID := uuid.New()
	first := "inbox-delivery-" + deliveryID.String()
	queue := &taskQueueStub{conflict: map[string]bool{first: true}}
	enqueue := RegisterDeliveryTasks(taskRegistryStub{}, queue, func() *Dispatcher { return nil })

	if err := enqueue(context.Background(), deliveryID, ""); err != nil {
		t.Fatalf("repeated first send = %v, want it to find its own task", err)
	}
	if err := enqueue(context.Background(), deliveryID, "r1"); err != nil {
		t.Fatalf("redelivery = %v", err)
	}
	if len(queue.ids) != 2 || queue.ids[1] == first {
		t.Fatalf("task IDs = %v, want the redelivery to have its own", queue.ids)
	}
	queue.conflict[queue.ids[1]] = true
	if err := enqueue(context.Background(), deliveryID, "r1"); err == nil {
		t.Fatal("a redelivery whose task ID is taken reported success")
	}
}
