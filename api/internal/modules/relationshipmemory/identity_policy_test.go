package relationshipmemory

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

func TestAPIEndUserSubjectIsStableTrimmedAndCaseSensitive(t *testing.T) {
	workspaceID, agentID := uuid.New(), uuid.New()
	first, err := usersubject.AgentAPIEndUserSubject(workspaceID, agentID, "  customer_123  ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := usersubject.AgentAPIEndUserSubject(workspaceID, agentID, "customer_123")
	if err != nil {
		t.Fatal(err)
	}
	upper, err := usersubject.AgentAPIEndUserSubject(workspaceID, agentID, "Customer_123")
	if err != nil {
		t.Fatal(err)
	}
	otherAgent, err := usersubject.AgentAPIEndUserSubject(workspaceID, uuid.New(), "customer_123")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("trimmed subject changed: %#v != %#v", first, second)
	}
	if first.ID == upper.ID {
		t.Fatal("API user identity must remain case-sensitive")
	}
	if first.ID == otherAgent.ID {
		t.Fatal("the same user must be isolated across Agents")
	}
}

func TestDigitalHumanAPIEndUserSubjectIsStableAndConsumerScoped(t *testing.T) {
	workspaceID := uuid.New()
	firstDigitalHumanID := uuid.New()
	secondDigitalHumanID := uuid.New()
	first, err := usersubject.DigitalHumanAPIEndUserSubject(workspaceID, firstDigitalHumanID, " customer_123 ")
	if err != nil {
		t.Fatalf("DigitalHumanAPIEndUserSubject() error = %v", err)
	}
	repeated, err := usersubject.DigitalHumanAPIEndUserSubject(workspaceID, firstDigitalHumanID, "customer_123")
	if err != nil {
		t.Fatalf("DigitalHumanAPIEndUserSubject() repeated error = %v", err)
	}
	other, err := usersubject.DigitalHumanAPIEndUserSubject(workspaceID, secondDigitalHumanID, "customer_123")
	if err != nil {
		t.Fatalf("DigitalHumanAPIEndUserSubject() other consumer error = %v", err)
	}
	if first != repeated {
		t.Fatalf("trimmed subjects differ: %#v vs %#v", first, repeated)
	}
	if first.ID == other.ID {
		t.Fatal("subjects for different DigitalHumans must differ")
	}
	if _, err := usersubject.DigitalHumanAPIEndUserSubject(workspaceID, firstDigitalHumanID, "bad\nuser"); !errors.Is(err, usersubject.ErrInvalidUser) {
		t.Fatalf("control character error = %v, want ErrInvalidUser", err)
	}
}

func TestAPIEndUserSubjectValidatesOpaqueUserLimit(t *testing.T) {
	workspaceID, agentID := uuid.New(), uuid.New()
	for _, input := range []string{"", "   ", strings.Repeat("x", 129)} {
		if _, err := usersubject.AgentAPIEndUserSubject(workspaceID, agentID, input); !errors.Is(err, usersubject.ErrInvalidUser) {
			t.Fatalf("APIEndUserSubject(%q) error = %v", input, err)
		}
	}
	if _, err := usersubject.AgentAPIEndUserSubject(workspaceID, agentID, strings.Repeat("界", 128)); err != nil {
		t.Fatalf("128-rune user rejected: %v", err)
	}
}

func TestDecideAccessNeverResolvesChannelsTasksOrAIChat(t *testing.T) {
	subject := usersubject.AccountSubject(uuid.New())
	tests := []struct {
		name     string
		identity InvocationIdentity
		reason   string
	}{
		{"channel private", InvocationIdentity{Surface: SurfaceChannel, ConversationType: ConversationPrivate, UserSubject: &subject}, "external_identity_unresolved"},
		{"channel group", InvocationIdentity{Surface: SurfaceChannel, ConversationType: ConversationGroup, UserSubject: &subject}, "external_identity_unresolved"},
		{"digital human task", InvocationIdentity{Surface: SurfaceDigitalHumanTask, ConversationType: ConversationTask, UserSubject: &subject}, "automated_invocation_has_no_user_subject"},
		{"aichat", InvocationIdentity{Surface: SurfaceAIChat, UserSubject: &subject}, "aichat_memory_disabled"},
		{"missing subject", InvocationIdentity{Surface: SurfaceAgentAPI}, "user_subject_missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := DecideAccess(test.identity)
			if decision.Read || decision.Mutate || decision.Tools || decision.AutoCapture || decision.Reason != test.reason {
				t.Fatalf("decision = %#v", decision)
			}
		})
	}
	allowed := DecideAccess(InvocationIdentity{Surface: SurfaceAgentConsole, UserSubject: &subject})
	if !allowed.Read || !allowed.Mutate || !allowed.Tools || !allowed.AutoCapture || allowed.Reason != "allowed" {
		t.Fatalf("allowed decision = %#v", allowed)
	}
}

func TestDecideAccessRequiresARecognizedSurfaceAndMatchingSubjectKind(t *testing.T) {
	account := usersubject.AccountSubject(uuid.New())
	apiUser := usersubject.Ref{Kind: usersubject.APIEndUser, ID: uuid.New()}
	tests := []struct {
		name     string
		identity InvocationIdentity
		reason   string
	}{
		{"unknown surface", InvocationIdentity{Surface: MemorySurface("unknown"), UserSubject: &account}, "surface_not_allowed"},
		{"account on API", InvocationIdentity{Surface: SurfaceAgentAPI, UserSubject: &account}, "subject_kind_mismatch"},
		{"API user on console", InvocationIdentity{Surface: SurfaceAgentConsole, UserSubject: &apiUser}, "subject_kind_mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := DecideAccess(test.identity)
			if decision.Read || decision.Mutate || decision.Tools || decision.AutoCapture || decision.Reason != test.reason {
				t.Fatalf("DecideAccess() = %#v, want denied reason %q", decision, test.reason)
			}
		})
	}

	if decision := DecideAccess(InvocationIdentity{Surface: SurfaceAgentAPI, UserSubject: &apiUser}); !decision.Read || decision.Reason != "allowed" {
		t.Fatalf("Agent API decision = %#v, want allowed", decision)
	}
}
