package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

func TestTaskOwnerRefRequiresExactlyOneOwnerNamespace(t *testing.T) {
	subject := usersubject.AccountSubject(uuid.New())
	conversationID := uuid.New()
	tests := []struct {
		name  string
		owner TaskOwnerRef
		valid bool
	}{
		{name: "shared", owner: SharedTaskOwner(), valid: true},
		{name: "user", owner: UserTaskOwner(subject), valid: true},
		{name: "conversation", owner: ConversationTaskOwner(conversationID), valid: true},
		{name: "missing kind", owner: TaskOwnerRef{}},
		{name: "mixed", owner: TaskOwnerRef{Kind: TaskOwnerUserSubject, Subject: subject, ConversationID: conversationID}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.owner.Valid(); got != test.valid {
				t.Fatalf("Valid() = %v, want %v", got, test.valid)
			}
		})
	}
}
