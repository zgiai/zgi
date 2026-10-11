package agentexecution

import (
	"github.com/google/uuid"
	"testing"
)

func TestBackgroundIdentityIsVersionedAndConversationScoped(t *testing.T) {
	org, workspace, app, conversation := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, entry := range []string{"channel", "task"} {
		subject := BackgroundSubject(org, workspace, app, conversation, entry)
		scope, err := NewScope(VerifiedSubject{OrganizationID: org, WorkspaceID: workspace, ApplicationType: "digital_human",
			ApplicationID: app, Entry: entry, User: subject, ConversationID: conversation, PublicationRef: "published"})
		if err != nil || scope.Identity.Version != BackgroundIdentityVersion {
			t.Fatalf("%s: scope=%+v err=%v", entry, scope, err)
		}
		for _, other := range []VerifiedSubject{
			{OrganizationID: uuid.New(), WorkspaceID: workspace, ApplicationID: app, ConversationID: conversation, Entry: entry},
			{OrganizationID: org, WorkspaceID: uuid.New(), ApplicationID: app, ConversationID: conversation, Entry: entry},
			{OrganizationID: org, WorkspaceID: workspace, ApplicationID: uuid.New(), ConversationID: conversation, Entry: entry},
			{OrganizationID: org, WorkspaceID: workspace, ApplicationID: app, ConversationID: uuid.New(), Entry: entry},
		} {
			if BackgroundSubject(other.OrganizationID, other.WorkspaceID, other.ApplicationID, other.ConversationID, other.Entry) == subject {
				t.Fatal("background namespaces share an execution subject")
			}
		}
		for _, mutate := range []func(*Identity){
			func(i *Identity) { i.Version = IdentityVersion },
			func(i *Identity) { i.ApplicationType = "agent" },
			func(i *Identity) { i.SubjectKind = "account" },
			func(i *Identity) { i.Entry = "console" },
		} {
			i := *scope.Identity
			mutate(&i)
			if i.Validate() == nil {
				t.Fatalf("accepted conflicting background identity: %+v", i)
			}
		}
		policy, err := ParseEntryAdmission(`{"digital_human:` + entry + `":["` + org.String() + `"]}`)
		if err != nil || !policy.Allows(org.String(), "digital_human", entry) || policy.Allows(org.String(), "agent", entry) {
			t.Fatalf("entry admission: %v", err)
		}
	}
	if BackgroundSubject(org, workspace, app, conversation, "task") == BackgroundSubject(org, workspace, app, conversation, "channel") {
		t.Fatal("channel and scheduler share a writable environment")
	}
}
