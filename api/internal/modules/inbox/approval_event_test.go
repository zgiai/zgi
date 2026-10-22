package inbox

import "testing"

func TestTaskApprovalEventReachesSubscribedChannels(t *testing.T) {
	if got := eventKey("digital_human.task_approval"); got != EventTaskApproval {
		t.Fatalf("eventKey = %q", got)
	}
	if got := webhookEvent("digital_human.task_approval"); got != "task.approval_requested" {
		t.Fatalf("webhookEvent = %q", got)
	}
	if events, err := normalizeEvents([]string{EventTaskApproval, EventTaskReport}); err != nil || len(events) != 2 {
		t.Fatalf("normalizeEvents = %v, %v", events, err)
	}
}
