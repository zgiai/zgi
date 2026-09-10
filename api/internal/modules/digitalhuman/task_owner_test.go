package digitalhuman

import (
	"encoding/json"
	"strings"
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

func TestTaskResponseExposesOwnerKindWithoutOwnerIdentifiers(t *testing.T) {
	payload, err := json.Marshal(Task{
		ID:             uuid.New(),
		DigitalHumanID: uuid.New(),
		OwnerKind:      TaskOwnerUserSubject,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	if !strings.Contains(body, `"owner_kind":"user_subject"`) {
		t.Fatalf("task response does not expose owner kind: %s", body)
	}
	for _, forbidden := range []string{"owner_subject_id", "owner_conversation_id"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("task response exposes %q: %s", forbidden, body)
		}
	}
}
