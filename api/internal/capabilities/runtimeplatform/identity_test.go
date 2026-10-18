package runtimeplatform

import (
	"testing"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestGrantPinsExecutionIdentityAndNeverImpersonatesAgent(t *testing.T) {
	scope := testScope()
	scope.AgentID = ""
	scope.ExecutionIdentity = execution.Identity{Version: 1, WorkspaceID: "workspace", ApplicationType: "digital_human",
		ApplicationID: "digital-human", Entry: "webapp", SubjectKind: "account", ConversationID: scope.ConversationID, PublicationRef: "published-v1"}
	session, token, err := NewSession(t.Context(), scope, []Action{ExportFile}, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for name, mutate := range map[string]func(*Scope){
		"application": func(s *Scope) { s.ExecutionIdentity.ApplicationID = "other" },
		"entry":       func(s *Scope) { s.ExecutionIdentity.Entry = "console" },
		"workspace":   func(s *Scope) { s.ExecutionIdentity.WorkspaceID = "other" },
		"publication": func(s *Scope) { s.ExecutionIdentity.PublicationRef = "published-v2" },
		"legacy":      func(s *Scope) { s.ExecutionIdentity = execution.Identity{} },
	} {
		t.Run(name, func(t *testing.T) {
			other := scope
			mutate(&other)
			if _, err := session.Execute(t.Context(), token, other, testRequest()); !apperror.IsCode(err, catalog.CodeAuthenticationRequired) {
				t.Fatalf("cross-identity grant = %v", err)
			}
		})
	}
	if _, err := session.Execute(t.Context(), token, scope, testRequest()); err != nil {
		t.Fatal(err)
	}
	scope.AgentID = "pretend-agent"
	if _, _, err := NewSession(t.Context(), scope, []Action{ExportFile}, testOptions()); !apperror.IsCode(err, catalog.CodeRequestInvalid) {
		t.Fatalf("DH Agent impersonation accepted: %v", err)
	}
}
