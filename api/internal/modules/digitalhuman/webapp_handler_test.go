package digitalhuman

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

func TestWebAppScopeSeparatesAnonymousPrincipalFromAccountMemorySubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	definition := &Definition{
		ID: uuid.New(), WebAppID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
	}
	access := &PublishedAccess{Definition: definition, Version: &Version{ID: uuid.New()}}

	t.Run("anonymous", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Set(webAppAccessContext, access)
		identity := webAppRequestIdentity{ID: uuid.New()}
		context.Set(webAppIdentityContext, identity)
		handler := &ExternalHandler{}
		scope, _, gotIdentity, ok := handler.webAppScope(context)
		if !ok || gotIdentity != identity {
			t.Fatal("anonymous webapp scope was not resolved")
		}
		if scope.UserSubjectID != nil || scope.PrincipalType != string(invocation.PrincipalExternalUser) || scope.ActorID != identity.ID {
			t.Fatalf("anonymous scope = %#v", scope)
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Set(webAppAccessContext, access)
		identity := webAppRequestIdentity{ID: uuid.New(), Authenticated: true}
		context.Set(webAppIdentityContext, identity)
		handler := &ExternalHandler{}
		scope, _, _, ok := handler.webAppScope(context)
		if !ok || scope.UserSubjectID == nil || *scope.UserSubjectID != identity.ID ||
			scope.UserSubjectKind != string(relationshipmemory.UserSubjectAccount) || scope.PrincipalType != string(invocation.PrincipalAccount) {
			t.Fatalf("authenticated scope = %#v", scope)
		}
	})
}
