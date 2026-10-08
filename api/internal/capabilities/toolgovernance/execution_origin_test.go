package toolgovernance

import (
	"testing"

	"github.com/google/uuid"
)

func TestExecutionOriginIsImmutableAndPartOfFrozenHash(t *testing.T) {
	o := ExecutionOrigin{OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), ApplicationType: "digital_human", ApplicationID: uuid.NewString(),
		Entry: "external-api", SubjectKind: "api_end_user", SubjectID: uuid.NewString(), ConversationID: uuid.NewString(), MessageID: uuid.NewString(), PublicationRef: uuid.NewString()}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	f := NewFrozenInvocation(FrozenInvocationRequest{ExecutionOrigin: &o, SkillID: "test", ToolName: "write", Arguments: map[string]interface{}{"value": 1}})
	o.SubjectID = uuid.NewString()
	if f.ExecutionOrigin.SubjectID == o.SubjectID || !FrozenInvocationHashMatches(f) {
		t.Fatal("caller mutation changed frozen origin")
	}
	f.ExecutionOrigin.PublicationRef = uuid.NewString()
	if FrozenInvocationHashMatches(f) {
		t.Fatal("publication tampering did not invalidate frozen hash")
	}
	for _, change := range []func(*ExecutionOrigin){
		func(o *ExecutionOrigin) { o.Entry = "channel" }, func(o *ExecutionOrigin) { o.ApplicationType = "runtime_workspace" },
		func(o *ExecutionOrigin) { o.SubjectID = "" }, func(o *ExecutionOrigin) { o.PublicationRef = "latest" },
		func(o *ExecutionOrigin) { o.SubjectKind = "account" },
	} {
		invalid := o
		change(&invalid)
		if invalid.Validate() == nil {
			t.Fatalf("accepted origin: %+v", invalid)
		}
	}
}
