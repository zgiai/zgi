package digitalhuman

import "testing"

// Wrapping the task tools must keep the task service that approval
// continuations apply changes through.
func TestArtifactRuntimeProviderKeepsTaskService(t *testing.T) {
	inner := NewTaskRuntimeProvider(taskRuntimeService{})
	service := &RuntimeService{}
	WithTaskRuntime(NewArtifactRuntimeProvider(inner, nil))(service)
	if service.taskService == nil {
		t.Fatal("task service lost behind the artifact tools")
	}
}
