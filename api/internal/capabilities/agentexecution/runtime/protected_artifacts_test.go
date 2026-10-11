package executionruntime

import (
	"testing"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProtectedArtifactPersisterFreezesOwnerAndMessageWithoutPublicURL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&tool_file.ToolFile{}); err != nil {
		t.Fatal(err)
	}
	manager := tool_file.NewToolFileManager(db, &agentExecutionTestStorage{LocalStorage: storage.NewLocalStorage(t.TempDir())})
	scope := execution.Scope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString(), Identity: &execution.Identity{
		Version: 1, WorkspaceID: uuid.NewString(), ApplicationType: "digital_human", ApplicationID: uuid.NewString(), Entry: "external-api", SubjectKind: "api_end_user", ConversationID: uuid.NewString(), PublicationRef: "published-v1"}}
	messageID := uuid.NewString()
	persister, err := NewProtectedArtifactPersister(manager, scope, messageID)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := persister.Persist(t.Context(), scope, scope.Identity.ConversationID, "artifacts/result.csv", []byte("a,b\n1,2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if artifact["access_mode"] != "authenticated" || artifact["message_id"] != messageID || artifact["filename"] != "result.csv" || artifact["url"] != nil || artifact["download_url"] != nil {
		t.Fatalf("artifact = %#v", artifact)
	}
	file, err := manager.GetToolFileByID(t.Context(), artifact["file_id"].(string))
	if err != nil || *file.ExecutionIdentity != *scope.Identity || *file.ExecutionMessageID != messageID || file.UserID != scope.SubjectID || file.TenantID != scope.OrganizationID {
		t.Fatalf("stored owner = %+v, %v", file, err)
	}
	for _, invalid := range []string{"../outside", "/etc/passwd", "a\\b", "."} {
		if _, err := persister.Persist(t.Context(), scope, scope.Identity.ConversationID, invalid, nil); err == nil {
			t.Fatalf("accepted path: %s", invalid)
		}
	}
	scope.Identity.PublicationRef = "mutated"
	if _, err := persister.Persist(t.Context(), scope, scope.Identity.ConversationID, "result.txt", nil); err == nil {
		t.Fatal("owner mutation affected bound persister")
	}
	if _, err := (defaultAgentExecutionArtifactPersister{}).Persist(t.Context(), scope, scope.Identity.ConversationID, "result.txt", nil); err == nil {
		t.Fatal("protected caller fell back to legacy persistence")
	}
}
