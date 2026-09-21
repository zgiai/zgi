package usersubject

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAPIEndUserSubjectsAreStableAndConsumerScoped(t *testing.T) {
	workspaceID, consumerID := uuid.New(), uuid.New()
	first, err := AgentAPIEndUserSubject(workspaceID, consumerID, " customer_123 ")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := AgentAPIEndUserSubject(workspaceID, consumerID, "customer_123")
	if err != nil || repeated != first {
		t.Fatalf("repeated subject = %#v, %v; want %#v", repeated, err, first)
	}
	other, err := DigitalHumanAPIEndUserSubject(workspaceID, consumerID, "customer_123")
	if err != nil || other == first {
		t.Fatalf("cross-consumer subject = %#v, %v; want a distinct subject", other, err)
	}
}

func TestAPIEndUserSubjectRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"", "bad\nuser", strings.Repeat("界", 129)} {
		if _, err := AgentAPIEndUserSubject(uuid.New(), uuid.New(), value); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("AgentAPIEndUserSubject(%q) error = %v, want ErrInvalidUser", value, err)
		}
	}
}
