package agentexecution

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

func TestVerifiedExecutionScopeRequiresExplicitEndUser(t *testing.T) {
	base := VerifiedSubject{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "agent",
		ApplicationID: uuid.New(), Entry: "webapp", User: usersubject.AccountSubject(uuid.New()),
		ConversationID: uuid.New(), PublicationRef: "published-v1"}
	for name, mutate := range map[string]func(*VerifiedSubject){
		"missing user":            func(s *VerifiedSubject) { s.User = usersubject.Ref{} },
		"missing organization":    func(s *VerifiedSubject) { s.OrganizationID = uuid.Nil },
		"missing workspace":       func(s *VerifiedSubject) { s.WorkspaceID = uuid.Nil },
		"missing application":     func(s *VerifiedSubject) { s.ApplicationID = uuid.Nil },
		"missing conversation":    func(s *VerifiedSubject) { s.ConversationID = uuid.Nil },
		"missing publication":     func(s *VerifiedSubject) { s.PublicationRef = "" },
		"api credential fallback": func(s *VerifiedSubject) { s.Entry = "external-api" },
		"anonymous":               func(s *VerifiedSubject) { s.User.Kind = "anonymous" },
		"runtime workspace":       func(s *VerifiedSubject) { s.ApplicationType = "runtime_workspace" },
		"background":              func(s *VerifiedSubject) { s.Entry = "task" },
	} {
		t.Run(name, func(t *testing.T) {
			subject := base
			mutate(&subject)
			if _, err := NewScope(subject); !errors.Is(err, ErrInvalidIdentity) {
				t.Fatalf("identity = %v", err)
			}
		})
	}
	for _, application := range []string{"agent", "digital_human"} {
		for _, entry := range []string{"console", "webapp", "external-api"} {
			subject := base
			subject.ApplicationType, subject.Entry = application, entry
			if entry == "external-api" {
				subject.User.Kind = usersubject.APIEndUser
			}
			scope, err := NewScope(subject)
			if err != nil || scope.SubjectID != subject.User.ID.String() || scope.Identity.ApplicationType != application {
				t.Fatalf("%s/%s = %+v, %v", application, entry, scope, err)
			}
		}
	}
}
