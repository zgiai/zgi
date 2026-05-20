package conversation

import "testing"

func TestWorkflowMessageStatusDefaultsToCompleted(t *testing.T) {
	if got := workflowMessageStatus(""); got != AgentMessageStatusCompleted {
		t.Fatalf("workflowMessageStatus empty = %q, want %s", got, AgentMessageStatusCompleted)
	}
	if got := workflowMessageStatus(AgentMessageStatusPendingApproval); got != AgentMessageStatusPendingApproval {
		t.Fatalf("workflowMessageStatus explicit = %q, want %s", got, AgentMessageStatusPendingApproval)
	}
}

func TestMessageStatusDefaultsToNormal(t *testing.T) {
	if got := messageStatus(""); got != AgentMessageStatusNormal {
		t.Fatalf("messageStatus empty = %q, want %s", got, AgentMessageStatusNormal)
	}
	if got := messageStatus(AgentMessageStatusCompleted); got != AgentMessageStatusCompleted {
		t.Fatalf("messageStatus explicit = %q, want %s", got, AgentMessageStatusCompleted)
	}
}
